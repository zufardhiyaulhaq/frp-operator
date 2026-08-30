package controllers

import (
	"context"
	"fmt"
	"net"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	ctrlbuilder "sigs.k8s.io/controller-runtime/pkg/builder"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	ctrlhandler "sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	frpv1alpha1 "github.com/zufardhiyaulhaq/frp-operator/api/v1alpha1"
	"github.com/zufardhiyaulhaq/frp-operator/pkg/client/handler"
	"github.com/zufardhiyaulhaq/frp-operator/pkg/client/models"
	"github.com/zufardhiyaulhaq/frp-operator/pkg/client/status"
	"github.com/zufardhiyaulhaq/frp-operator/pkg/loadbalancer"
	"github.com/zufardhiyaulhaq/frp-operator/pkg/metrics"
)

const (
	EventReasonBound             = "Bound"
	EventReasonServerReallocated = "ServerReallocated"
	EventReasonPoolDeleted       = "PoolDeleted"
	EventReasonClientNotReady    = "ClientNotReady"
	EventReasonProxyStartError   = "ProxyStartError"

	lbRequeue = 30 * time.Second

	// eventRepeatInterval is how long warnOnce suppresses a previously-emitted Warning message
	// before re-emitting it. Without this, a Warning suppressed early in an outage would never be
	// seen again by anyone who starts watching Events later (Kubernetes Events themselves expire
	// after ~1h by default), so a still-ongoing problem could appear to have gone silent.
	eventRepeatInterval = 10 * time.Minute
)

// serverNameRE mirrors the ServerPoolServer.Name kubebuilder validation
// (+kubebuilder:validation:Pattern=`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`, +kubebuilder:validation:MaxLength=40)
// so a pool that somehow bypassed CRD validation (e.g. applied against stale CRDs, or restored
// from a backup taken before validation was added) is still caught here and reported NotReady
// instead of producing generated object names controller-gen assumed were already safe.
var serverNameRE = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`)

const maxServerNameLength = 40

// ServiceReconciler binds LoadBalancer Services with our loadBalancerClass to ServerPool servers.
type ServiceReconciler struct {
	ctrlclient.Client
	Scheme            *runtime.Scheme
	Recorder          record.EventRecorder
	OperatorNamespace string
	// StatusFunc reads frpc proxy status; nil means handler.Status. Tests override it.
	StatusFunc func(models.Config) ([]handler.ProxyStatus, error)
	// now returns the current time; nil means time.Now. Tests override it to advance the clock
	// without sleeping, to exercise eventRepeatInterval re-emission.
	now func() time.Time

	// lastEventMu guards lastEvent, which deduplicates recurring Warning events (ClientNotReady,
	// ProxyStartError, Pending reasons) so a persistent problem is reported once instead of
	// every ~30s reconcile. Keyed by Service, holding the set of "reason: message" strings
	// already emitted for it, each mapped to when it was last emitted — a set, not a single last
	// value, because a multi-port or multi-server Service can have several distinct Warning
	// messages live at once (e.g. two ports both in "start error" with different proxy names) and
	// each must still only be reported once per eventRepeatInterval, not have the second message
	// keep re-arming the first. The recorded time lets a still-ongoing problem be re-emitted every
	// eventRepeatInterval instead of being silenced forever.
	lastEventMu sync.Mutex
	lastEvent   map[types.NamespacedName]map[string]time.Time
}

func (r *ServiceReconciler) clock() time.Time {
	if r.now != nil {
		return r.now()
	}
	return time.Now()
}

//+kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;update;patch
//+kubebuilder:rbac:groups="",resources=services/status,verbs=get;update;patch
//+kubebuilder:rbac:groups="",resources=events,verbs=create;patch
//+kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch
//+kubebuilder:rbac:groups=frp.zufardhiyaulhaq.com,resources=serverpools,verbs=get;list;watch
//+kubebuilder:rbac:groups=frp.zufardhiyaulhaq.com,resources=serverpools/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=frp.zufardhiyaulhaq.com,resources=clients;upstreams,verbs=get;list;watch;create;update;patch;delete

// warnOnce emits a Warning event for svc only if reason+": "+message has not already been
// recorded for this Service within the last eventRepeatInterval, so a recurring condition
// (ClientNotReady, ProxyStartError, a Pending reason) is reported periodically instead of on
// every ~30s reconcile. Distinct messages for the same Service (e.g. one per port) are tracked
// independently, so a second message does not suppress or reset an earlier, still-relevant one.
func (r *ServiceReconciler) warnOnce(svc *corev1.Service, reason, message string) {
	key := types.NamespacedName{Namespace: svc.Namespace, Name: svc.Name}
	full := reason + ": " + message
	now := r.clock()

	r.lastEventMu.Lock()
	if r.lastEvent == nil {
		r.lastEvent = map[types.NamespacedName]map[string]time.Time{}
	}
	seen := r.lastEvent[key]
	if last, ok := seen[full]; ok && now.Sub(last) < eventRepeatInterval {
		r.lastEventMu.Unlock()
		return
	}
	if seen == nil {
		seen = map[string]time.Time{}
		r.lastEvent[key] = seen
	}
	seen[full] = now
	r.lastEventMu.Unlock()

	r.Recorder.Event(svc, corev1.EventTypeWarning, reason, message)
}

// clearEventDedup drops the last-seen Warning for svc, so a problem that recurs later (after the
// Service became fully bound, or after it was deleted and recreated) is reported again.
func (r *ServiceReconciler) clearEventDedup(svc *corev1.Service) {
	key := types.NamespacedName{Namespace: svc.Namespace, Name: svc.Name}
	r.lastEventMu.Lock()
	delete(r.lastEvent, key)
	r.lastEventMu.Unlock()
}

func (r *ServiceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx).WithValues("service", req.NamespacedName)

	svc := &corev1.Service{}
	if err := r.Get(ctx, req.NamespacedName, svc); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	claimed := loadbalancer.IsClaimed(svc)
	hasFinalizer := controllerutil.ContainsFinalizer(svc, loadbalancer.Finalizer)

	if !svc.DeletionTimestamp.IsZero() || !claimed {
		if !hasFinalizer {
			return ctrl.Result{}, nil
		}
		r.clearEventDedup(svc)
		metrics.DeleteLoadBalancer(svc.Namespace, svc.Name)
		if err := r.cleanupService(ctx, svc, nil, nil); err != nil {
			return ctrl.Result{}, err
		}
		if err := r.updatePoolStatuses(ctx); err != nil {
			log.Error(err, "failed to update pool status")
		}
		// releasingClass: the loadBalancerClass was removed/changed on a live (non-deleting) Service,
		// as opposed to the Service itself being deleted.
		releasingClass := !claimed && svc.DeletionTimestamp.IsZero()
		if releasingClass {
			delete(svc.Annotations, loadbalancer.AnnotationAllocated)
		}
		controllerutil.RemoveFinalizer(svc, loadbalancer.Finalizer)
		// Persist metadata (annotations, finalizers) via a regular Update first. A subsequent
		// Status().Update would otherwise overwrite our local copy's non-status fields with what
		// is currently stored (mirrors real apiserver /status subresource semantics), silently
		// reverting any annotation/finalizer changes made before it.
		if err := r.Update(ctx, svc); err != nil && !apierrors.IsNotFound(err) {
			return ctrl.Result{}, err
		}
		if releasingClass {
			svc.Status.LoadBalancer = corev1.LoadBalancerStatus{}
			if err := r.Status().Update(ctx, svc); err != nil && !apierrors.IsNotFound(err) {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	if !hasFinalizer {
		controllerutil.AddFinalizer(svc, loadbalancer.Finalizer)
		if err := r.Update(ctx, svc); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	pools, err := r.listPools(ctx)
	if err != nil {
		return ctrl.Result{}, err
	}
	allocs, err := r.listAllocations(ctx)
	if err != nil {
		return ctrl.Result{}, err
	}
	request := requestFor(svc)

	// Reject unsupported protocols (e.g. SCTP) before even considering the sticky path — the
	// existing binding, if any, must not be disturbed by a newly-added bad port.
	if reason := loadbalancer.ValidateProtocols(request); reason != nil {
		r.warnOnce(svc, reason.Reason, reason.Message)
		return r.setPending(ctx, svc)
	}

	bound, err := loadbalancer.ParseAllocated(svc.Annotations[loadbalancer.AnnotationAllocated])
	if err != nil {
		log.Info("ignoring malformed allocated-server annotation", "error", err.Error())
		bound = nil
	}

	refs, reason := r.resolveBinding(ctx, svc, request, bound, pools, allocs)
	if reason != nil {
		r.warnOnce(svc, reason.Reason, reason.Message)
		return r.setPending(ctx, svc)
	}

	// Drop generated objects on servers we are no longer bound to (reallocation / annotation edit).
	if err := r.cleanupService(ctx, svc, refs, pools); err != nil {
		return ctrl.Result{}, err
	}
	if err := r.applyBinding(ctx, svc, request, refs, pools); err != nil {
		return ctrl.Result{}, err
	}
	if lost := r.raceCheck(ctx, svc, refs, request); lost != nil {
		r.warnOnce(svc, lost.Reason, lost.Message)
		if err := r.cleanupService(ctx, svc, nil, pools); err != nil {
			return ctrl.Result{}, err
		}
		return r.setPending(ctx, svc)
	}

	want := loadbalancer.FormatAllocated(refs)
	if svc.Annotations[loadbalancer.AnnotationAllocated] != want {
		if svc.Annotations == nil {
			svc.Annotations = map[string]string{}
		}
		svc.Annotations[loadbalancer.AnnotationAllocated] = want
		if err := r.Update(ctx, svc); err != nil {
			return ctrl.Result{}, err
		}
		r.Recorder.Event(svc, corev1.EventTypeNormal, EventReasonBound, "bound to "+want)
	}

	ingress := r.ingressFor(ctx, svc, request, refs, pools)
	// state reflects whether the Service actually has a working ingress for every bound ref, not
	// merely that a binding was selected: a Service can be bound (refs chosen, annotation written)
	// while still waiting on its Client to become Ready or its proxies to start, and that state
	// must read "pending" in the metric, matching what kubectl shows (EXTERNAL-IP <pending>).
	state := "pending"
	if len(ingress) == len(refs) {
		// Fully bound (every ref produced an ingress entry, not just some of them): forget any
		// previously-suppressed Warning so a future recurrence is reported again instead of
		// staying silenced forever. A partial match (e.g. a multi-server pinned Service where
		// only some servers are ready) must not clear this — the still-unready server(s) would
		// otherwise re-arm the same Warning on every reconcile.
		r.clearEventDedup(svc)
		state = "bound"
	}
	if !sameIngress(svc.Status.LoadBalancer.Ingress, ingress) {
		svc.Status.LoadBalancer.Ingress = ingress
		if err := r.Status().Update(ctx, svc); err != nil {
			return ctrl.Result{}, err
		}
	}
	metrics.RecordLoadBalancer(svc.Namespace, svc.Name, refs[0].Pool, refs[0].Server, state)
	if err := r.updatePoolStatuses(ctx); err != nil {
		log.Error(err, "failed to update pool status")
	}
	return ctrl.Result{RequeueAfter: lbRequeue}, nil
}

// sameIngress compares only IP and Hostname per entry. On Kubernetes >= 1.30 the apiserver
// defaults ingress[].ipMode, which reflect.DeepEqual would then see as a perpetual diff between
// what we last wrote and what we read back, causing a Status().Update on every reconcile.
func sameIngress(a, b []corev1.LoadBalancerIngress) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].IP != b[i].IP || a[i].Hostname != b[i].Hostname {
			return false
		}
	}
	return true
}

func requestFor(svc *corev1.Service) loadbalancer.Request {
	return loadbalancer.Request{
		ServiceKey: loadbalancer.ServiceKey(svc),
		Ports:      svc.Spec.Ports,
		Pool:       strings.TrimSpace(svc.Annotations[loadbalancer.AnnotationServerPool]),
		Servers:    loadbalancer.ParseServerList(svc.Annotations[loadbalancer.AnnotationServer]),
	}
}

// resolveBinding keeps an existing valid binding (sticky) or runs selection.
func (r *ServiceReconciler) resolveBinding(ctx context.Context, svc *corev1.Service, request loadbalancer.Request,
	bound []loadbalancer.ServerRef, pools []frpv1alpha1.ServerPool, allocs []loadbalancer.Allocation) ([]loadbalancer.ServerRef, *loadbalancer.Reason) {

	if len(bound) > 0 && boundStillValid(bound, request, pools) {
		mine := loadbalancer.Allocation{ServiceKey: request.ServiceKey, ServiceCreated: svc.CreationTimestamp.Time}
		for _, ref := range bound {
			pool := loadbalancer.FindPool(pools, ref.Pool)
			server := loadbalancer.FindServer(*pool, ref.Server)

			filtered, lost := stickyFilterAllocs(mine, request, ref, allocs)
			if lost != nil {
				// Genuine race for a port I already hold, and I lost it: my objects are stale,
				// clean them up now instead of deadlocking both sides in Pending forever (the
				// winner keeps its objects and ingress; I go back through allocation next time).
				if err := r.cleanupService(ctx, svc, nil, pools); err != nil {
					log.FromContext(ctx).Error(err, "failed to clean up after losing a port race")
				}
				// Also drop the allocated-server annotation: leaving it in place would keep
				// boundStillValid true forever (the pool/server I lost still exist), wedging me
				// on a binding I no longer hold any objects for and never re-selecting. Clearing
				// it lets the next reconcile fall through to Allocate and pick another server.
				if _, ok := svc.Annotations[loadbalancer.AnnotationAllocated]; ok {
					delete(svc.Annotations, loadbalancer.AnnotationAllocated)
					if err := r.Update(ctx, svc); err != nil {
						log.FromContext(ctx).Error(err, "failed to clear allocated-server annotation after losing a port race")
					}
				}
				return nil, lost
			}
			if reason := loadbalancer.Fit(request, *pool, *server, filtered); reason != nil {
				// Either a config problem (allowedPorts) or a conflict on a port I do not hold
				// yet (e.g. newly added to the Service): go Pending without touching my
				// existing, still-valid objects.
				return nil, reason
			}
		}
		return bound, nil
	}

	refs, reason := loadbalancer.Allocate(request, pools, allocs)
	if reason != nil {
		if len(bound) > 0 {
			r.warnOnce(svc, EventReasonPoolDeleted,
				fmt.Sprintf("bound server(s) %s no longer exist; %s", loadbalancer.FormatAllocated(bound), reason.Message))
			// The pool (or every bound server in it) is gone and no re-selection is possible:
			// the generated objects for the old binding can never become valid again, so delete
			// them now rather than leaving them orphaned. The allocated-server annotation is
			// left as-is so a returning pool/server can re-bind (sticky).
			if err := r.cleanupService(ctx, svc, nil, pools); err != nil {
				log.FromContext(ctx).Error(err, "failed to clean up after pool/server deletion")
			}
		}
		return nil, reason
	}
	if len(bound) > 0 {
		// One-shot transition (not a recurring Pending reason): once refs are picked, the
		// allocated-server annotation is updated below and the next reconcile takes the sticky
		// path, so this does not repeat on its own — still routed through warnOnce for safety.
		r.warnOnce(svc, EventReasonServerReallocated,
			fmt.Sprintf("%s -> %s", loadbalancer.FormatAllocated(bound), loadbalancer.FormatAllocated(refs)))
	}
	return refs, nil
}

// stickyFilterAllocs drops, from allocs, any foreign allocation on ref that mine has already won
// a race against (so Fit does not fail on it — the loser cleans itself up on its own reconcile).
// It returns a non-nil Reason when mine loses a genuine race: a foreign allocation exists for a
// port mine itself also holds (an Upstream of mine already exists for it), and mine's
// ServiceCreated does not win the tie-break. A foreign allocation on a port mine does not hold
// yet (e.g. a port just added to the Service) is left in the returned slice untouched, so Fit
// reports it as a plain PortUnavailable/Pending without triggering any cleanup.
func stickyFilterAllocs(mine loadbalancer.Allocation, request loadbalancer.Request, ref loadbalancer.ServerRef,
	allocs []loadbalancer.Allocation) ([]loadbalancer.Allocation, *loadbalancer.Reason) {

	filtered := make([]loadbalancer.Allocation, 0, len(allocs))
	for _, a := range allocs {
		if a.Pool != ref.Pool || a.Server != ref.Server || a.ServiceKey == mine.ServiceKey {
			filtered = append(filtered, a)
			continue
		}
		// a is a foreign allocation on my bound server. Only ports I am actually requesting
		// matter for the race check; anything else is irrelevant to Fit too.
		requested := false
		for _, p := range request.Ports {
			if a.Port == p.Port && a.Protocol == protocolOrTCP(p) {
				requested = true
				break
			}
		}
		if !requested {
			filtered = append(filtered, a)
			continue
		}
		iHoldIt := false
		for _, o := range allocs {
			if o.ServiceKey == mine.ServiceKey && o.Pool == ref.Pool && o.Server == ref.Server && o.Port == a.Port && o.Protocol == a.Protocol {
				iHoldIt = true
				break
			}
		}
		if !iHoldIt {
			// Case (b): a port I do not hold yet is blocked by someone else. Keep it so Fit
			// reports PortUnavailable; do not touch my existing objects.
			filtered = append(filtered, a)
			continue
		}
		// Case (a): a genuine race for a port I already hold.
		if loadbalancer.Winner(mine, a).ServiceKey == mine.ServiceKey {
			// I win: drop the foreign allocation so Fit does not fail on it.
			continue
		}
		return nil, &loadbalancer.Reason{Reason: loadbalancer.ReasonPortUnavailable,
			Message: fmt.Sprintf("port %d/%s on %s won by %s", a.Port, a.Protocol, ref, a.ServiceKey)}
	}
	return filtered, nil
}

// boundStillValid: every bound server still exists, and if the user pins servers the pinned set
// must equal the bound set (so annotation edits are honoured).
func boundStillValid(bound []loadbalancer.ServerRef, request loadbalancer.Request, pools []frpv1alpha1.ServerPool) bool {
	for _, ref := range bound {
		pool := loadbalancer.FindPool(pools, ref.Pool)
		if pool == nil || loadbalancer.FindServer(*pool, ref.Server) == nil {
			return false
		}
		if request.Pool != "" && request.Pool != ref.Pool {
			return false
		}
	}
	if len(request.Servers) == 0 {
		// The server annotation was removed (or never set) while bound to more than one server.
		// A multi-server binding only ever exists because the user pinned that exact set via the
		// server annotation; once it's gone there is no longer any basis for holding more than
		// one server, so fall through to re-selection (which collapses to a single server) rather
		// than treating the old pinned set as still valid.
		return len(bound) <= 1
	}
	if len(request.Servers) != len(bound) {
		return false
	}
	want := map[string]bool{}
	for _, s := range request.Servers {
		want[s] = true
	}
	for _, ref := range bound {
		if !want[ref.Server] {
			return false
		}
	}
	return true
}

func (r *ServiceReconciler) listPools(ctx context.Context) ([]frpv1alpha1.ServerPool, error) {
	list := &frpv1alpha1.ServerPoolList{}
	if err := r.List(ctx, list, ctrlclient.InNamespace(r.OperatorNamespace)); err != nil {
		return nil, err
	}
	return list.Items, nil
}

func (r *ServiceReconciler) listManagedUpstreams(ctx context.Context, extra ...ctrlclient.ListOption) ([]frpv1alpha1.Upstream, error) {
	list := &frpv1alpha1.UpstreamList{}
	opts := append([]ctrlclient.ListOption{
		ctrlclient.InNamespace(r.OperatorNamespace),
		ctrlclient.MatchingLabels{loadbalancer.LabelManagedBy: loadbalancer.ManagedByValue},
	}, extra...)
	if err := r.List(ctx, list, opts...); err != nil {
		return nil, err
	}
	return list.Items, nil
}

func (r *ServiceReconciler) listAllocations(ctx context.Context) ([]loadbalancer.Allocation, error) {
	ups, err := r.listManagedUpstreams(ctx)
	if err != nil {
		return nil, err
	}
	var allocs []loadbalancer.Allocation
	for i := range ups {
		if a, ok := loadbalancer.AllocationFromUpstream(&ups[i]); ok {
			allocs = append(allocs, a)
		}
	}
	return allocs, nil
}

// applyBinding ensures Clients and Upstreams for every bound server and port.
func (r *ServiceReconciler) applyBinding(ctx context.Context, svc *corev1.Service, request loadbalancer.Request,
	refs []loadbalancer.ServerRef, pools []frpv1alpha1.ServerPool) error {

	wantUpstreams := map[string]bool{}
	for _, ref := range refs {
		pool := loadbalancer.FindPool(pools, ref.Pool)
		server := loadbalancer.FindServer(*pool, ref.Server)

		desiredClient := loadbalancer.BuildClient(pool, *server, svc, ref)
		if err := r.ensureClient(ctx, desiredClient); err != nil {
			return err
		}
		for _, port := range request.Ports {
			desired := loadbalancer.BuildUpstream(pool, svc, port, ref)
			wantUpstreams[desired.Name] = true
			if err := r.ensureUpstream(ctx, desired); err != nil {
				return err
			}
		}
	}
	// Remove upstreams of this Service for ports that no longer exist.
	existing, err := r.listManagedUpstreams(ctx, ctrlclient.MatchingLabels{loadbalancer.LabelServiceUID: string(svc.UID)})
	if err != nil {
		return err
	}
	for i := range existing {
		if !wantUpstreams[existing[i].Name] {
			if err := r.Delete(ctx, &existing[i]); err != nil && !apierrors.IsNotFound(err) {
				return err
			}
		}
	}
	return nil
}

func (r *ServiceReconciler) ensureClient(ctx context.Context, desired *frpv1alpha1.Client) error {
	current := &frpv1alpha1.Client{}
	err := r.Get(ctx, types.NamespacedName{Namespace: desired.Namespace, Name: desired.Name}, current)
	if apierrors.IsNotFound(err) {
		return r.Create(ctx, desired)
	}
	if err != nil {
		return err
	}
	if reflect.DeepEqual(current.Spec, desired.Spec) && reflect.DeepEqual(current.Labels, desired.Labels) {
		return nil
	}
	current.Spec = desired.Spec
	current.Labels = desired.Labels
	return r.Update(ctx, current)
}

func (r *ServiceReconciler) ensureUpstream(ctx context.Context, desired *frpv1alpha1.Upstream) error {
	current := &frpv1alpha1.Upstream{}
	err := r.Get(ctx, types.NamespacedName{Namespace: desired.Namespace, Name: desired.Name}, current)
	if apierrors.IsNotFound(err) {
		return r.Create(ctx, desired)
	}
	if err != nil {
		return err
	}
	if reflect.DeepEqual(current.Spec, desired.Spec) && reflect.DeepEqual(current.Labels, desired.Labels) &&
		reflect.DeepEqual(current.Annotations, desired.Annotations) {
		return nil
	}
	current.Spec = desired.Spec
	current.Labels = desired.Labels
	current.Annotations = desired.Annotations
	return r.Update(ctx, current)
}

// cleanupService deletes generated objects of svc that are not on one of keep. keep == nil
// deletes everything. pools is used to compute, for a kept ref, the Client name its pool's
// *current* clientMode dictates (a PerService/Shared flip on a server we stay bound to must not
// leak the previous mode's Client); pools may be nil when keep is nil. Every managed Upstream is
// snapshotted once up front and never re-listed, so the "is this Client still referenced"
// decision below cannot depend on observing this call's own writes.
func (r *ServiceReconciler) cleanupService(ctx context.Context, svc *corev1.Service, keep []loadbalancer.ServerRef, pools []frpv1alpha1.ServerPool) error {
	keepSet := map[loadbalancer.ServerRef]bool{}
	for _, ref := range keep {
		keepSet[ref] = true
	}
	// Desired Client name for every kept ref, using the pool's current clientMode.
	keepClientNames := map[string]bool{}
	for _, ref := range keep {
		if pool := loadbalancer.FindPool(pools, ref.Pool); pool != nil {
			keepClientNames[loadbalancer.ClientName(svc, ref, pool.Spec.ClientMode)] = true
		}
	}

	allUpstreams, err := r.listManagedUpstreams(ctx)
	if err != nil {
		return err
	}
	// simulatedClient predicts what u's Spec.Client will be once this reconcile pass finishes:
	// "" once its Upstream is deleted below, the new desired name once applyBinding repoints a
	// kept-but-clientMode-changed Upstream, or its current value for anything untouched by this
	// call (including every other Service's Upstreams).
	simulatedClient := func(u frpv1alpha1.Upstream) string {
		if u.Labels[loadbalancer.LabelServiceUID] != string(svc.UID) {
			return u.Spec.Client
		}
		ref := loadbalancer.ServerRef{Pool: u.Labels[loadbalancer.LabelPool], Server: u.Labels[loadbalancer.LabelServer]}
		if !keepSet[ref] {
			return ""
		}
		if pool := loadbalancer.FindPool(pools, ref.Pool); pool != nil {
			return loadbalancer.ClientName(svc, ref, pool.Spec.ClientMode)
		}
		return u.Spec.Client
	}

	touchedClients := map[string]bool{}
	for i := range allUpstreams {
		u := allUpstreams[i]
		if u.Labels[loadbalancer.LabelServiceUID] != string(svc.UID) {
			continue
		}
		ref := loadbalancer.ServerRef{Pool: u.Labels[loadbalancer.LabelPool], Server: u.Labels[loadbalancer.LabelServer]}
		if keepSet[ref] {
			if want := simulatedClient(u); u.Spec.Client != want {
				// clientMode flipped for this still-bound ref: applyBinding is about to
				// repoint this Upstream to want, so the old Client name may now be orphaned.
				touchedClients[u.Spec.Client] = true
			}
			continue
		}
		touchedClients[u.Spec.Client] = true
		if err := r.Delete(ctx, &u); err != nil && !apierrors.IsNotFound(err) {
			return err
		}
	}
	// Per-service Clients (PerService mode) whose name does not match a kept ref's current
	// desired name — covers both "ref no longer kept" and "ref kept but clientMode flipped".
	clients := &frpv1alpha1.ClientList{}
	if err := r.List(ctx, clients, ctrlclient.InNamespace(r.OperatorNamespace),
		ctrlclient.MatchingLabels{loadbalancer.LabelManagedBy: loadbalancer.ManagedByValue, loadbalancer.LabelServiceUID: string(svc.UID)}); err != nil {
		return err
	}
	for i := range clients.Items {
		if keepClientNames[clients.Items[i].Name] {
			continue
		}
		if err := r.Delete(ctx, &clients.Items[i]); err != nil && !apierrors.IsNotFound(err) {
			return err
		}
		delete(touchedClients, clients.Items[i].Name)
	}
	// Shared Clients: delete when nothing will reference them once this pass finishes, per the
	// allUpstreams snapshot taken above (never re-listed).
	for name := range touchedClients {
		inUse := false
		for _, u := range allUpstreams {
			if simulatedClient(u) == name {
				inUse = true
				break
			}
		}
		if inUse {
			continue
		}
		c := &frpv1alpha1.Client{}
		if err := r.Get(ctx, types.NamespacedName{Namespace: r.OperatorNamespace, Name: name}, c); err != nil {
			if apierrors.IsNotFound(err) {
				continue
			}
			return err
		}
		if c.Labels[loadbalancer.LabelManagedBy] != loadbalancer.ManagedByValue {
			continue
		}
		if err := r.Delete(ctx, c); err != nil && !apierrors.IsNotFound(err) {
			return err
		}
	}
	return nil
}

// raceCheck resolves two Services that both created Upstreams for the same server/port.
func (r *ServiceReconciler) raceCheck(ctx context.Context, svc *corev1.Service, refs []loadbalancer.ServerRef, request loadbalancer.Request) *loadbalancer.Reason {
	allocs, err := r.listAllocations(ctx)
	if err != nil {
		return nil
	}
	mine := loadbalancer.ServiceKey(svc)
	for _, ref := range refs {
		for _, port := range request.Ports {
			var own *loadbalancer.Allocation
			var others []loadbalancer.Allocation
			for i := range allocs {
				a := allocs[i]
				if a.Pool != ref.Pool || a.Server != ref.Server || a.Port != port.Port || string(a.Protocol) != string(protocolOrTCP(port)) {
					continue
				}
				if a.ServiceKey == mine {
					own = &allocs[i]
				} else {
					others = append(others, a)
				}
			}
			if own == nil {
				continue
			}
			for _, o := range others {
				if loadbalancer.Winner(*own, o).ServiceKey != mine {
					return &loadbalancer.Reason{Reason: loadbalancer.ReasonPortUnavailable,
						Message: fmt.Sprintf("port %d/%s on %s won by %s", port.Port, protocolOrTCP(port), ref, o.ServiceKey)}
				}
			}
		}
	}
	return nil
}

func protocolOrTCP(p corev1.ServicePort) corev1.Protocol {
	if p.Protocol == "" {
		return corev1.ProtocolTCP
	}
	return p.Protocol
}

// ingressFor returns one ingress entry per bound server whose Client is Ready and whose
// proxies for this Service are all running. Servers that are not ready emit an Event and are omitted.
func (r *ServiceReconciler) ingressFor(ctx context.Context, svc *corev1.Service, request loadbalancer.Request,
	refs []loadbalancer.ServerRef, pools []frpv1alpha1.ServerPool) []corev1.LoadBalancerIngress {

	var ingress []corev1.LoadBalancerIngress
	for _, ref := range refs {
		pool := loadbalancer.FindPool(pools, ref.Pool)
		server := loadbalancer.FindServer(*pool, ref.Server)
		clientName := loadbalancer.ClientName(svc, ref, pool.Spec.ClientMode)

		client := &frpv1alpha1.Client{}
		if err := r.Get(ctx, types.NamespacedName{Namespace: r.OperatorNamespace, Name: clientName}, client); err != nil {
			r.warnOnce(svc, EventReasonClientNotReady, fmt.Sprintf("%s: client %s: %v", ref, clientName, err))
			continue
		}
		if !clientReady(client) {
			r.warnOnce(svc, EventReasonClientNotReady, fmt.Sprintf("%s: client %s is not Ready: %s", ref, clientName, client.Status.Message))
			continue
		}
		proxies, err := r.proxyStatus(ctx, client)
		if err != nil {
			r.warnOnce(svc, EventReasonClientNotReady, fmt.Sprintf("%s: cannot read frpc status: %v", ref, err))
			continue
		}
		byName := map[string]handler.ProxyStatus{}
		for _, p := range proxies {
			byName[p.Name] = p
		}
		ready := true
		for _, port := range request.Ports {
			name := loadbalancer.UpstreamName(svc, ref, port.Port, protocolOrTCP(port))
			p, ok := byName[name]
			if !ok || p.Status != "running" {
				ready = false
				msg := fmt.Sprintf("port %d on %s: proxy %s status %q", port.Port, ref, name, p.Status)
				if p.Err != "" {
					msg += ": " + p.Err
				}
				r.warnOnce(svc, EventReasonProxyStartError, msg)
			}
		}
		if !ready {
			continue
		}
		if net.ParseIP(server.PublicAddress) != nil {
			// IPMode: Proxy tells kube-proxy/consumers that traffic to this IP is delivered to
			// the node or pod, not the IP itself acting as a real load-balancer VIP — which is
			// what an frps server's public address actually is here. sameIngress ignores this
			// field, so setting it does not cause a spurious Status().Update loop.
			ingress = append(ingress, corev1.LoadBalancerIngress{IP: server.PublicAddress, IPMode: ptr.To(corev1.LoadBalancerIPModeProxy)})
		} else {
			ingress = append(ingress, corev1.LoadBalancerIngress{Hostname: server.PublicAddress})
		}
	}
	return ingress
}

func clientReady(c *frpv1alpha1.Client) bool {
	for _, cond := range c.Status.Conditions {
		if cond.Type == status.ConditionTypeReady && cond.Status == metav1.ConditionTrue {
			return true
		}
	}
	return false
}

func (r *ServiceReconciler) proxyStatus(ctx context.Context, client *frpv1alpha1.Client) ([]handler.ProxyStatus, error) {
	cfg, err := models.NewConfig(r.Client, client, nil, nil)
	if err != nil {
		// NewConfig can fail for reasons unrelated to whether frpc itself is reachable (e.g. a
		// token Secret that was deleted after the Client was created) — do not blank a
		// healthy Service's ingress over that. NewConfig still returns cfg populated up to the
		// point of failure (including AdminPort/AdminUsername/AdminPassword, which are resolved
		// from their own Secrets earlier and independently of the token lookup that failed), so
		// keep it and only backfill whichever admin fields are still zero-valued with the
		// defaults, instead of discarding everything NewConfig already resolved.
		log.FromContext(ctx).Info("proxyStatus: NewConfig failed, using its partially-populated config", "client", client.Name, "error", err.Error())
		if cfg.Common.AdminPort == 0 {
			cfg.Common.AdminPort = models.DEFAULT_ADMIN_PORT
		}
		if cfg.Common.AdminUsername == "" {
			cfg.Common.AdminUsername = models.DEFAULT_ADMIN_USERNAME
		}
		if cfg.Common.AdminPassword == "" {
			cfg.Common.AdminPassword = models.DEFAULT_ADMIN_PASSWORD
		}
	}
	cfg.Common.AdminAddress = client.Name + "-frpc." + client.Namespace + ".svc"
	fn := r.StatusFunc
	if fn == nil {
		fn = handler.Status
	}
	return fn(cfg)
}

func (r *ServiceReconciler) setPending(ctx context.Context, svc *corev1.Service) (ctrl.Result, error) {
	metrics.RecordLoadBalancer(svc.Namespace, svc.Name, "", "", "pending")
	if len(svc.Status.LoadBalancer.Ingress) != 0 {
		svc.Status.LoadBalancer.Ingress = nil
		if err := r.Status().Update(ctx, svc); err != nil {
			return ctrl.Result{}, err
		}
	}
	if err := r.updatePoolStatuses(ctx); err != nil {
		log.FromContext(ctx).Error(err, "failed to update pool status")
	}
	return ctrl.Result{RequeueAfter: lbRequeue}, nil
}

// updatePoolStatuses rewrites status.servers[].allocatedPorts and the Ready condition of every pool.
func (r *ServiceReconciler) updatePoolStatuses(ctx context.Context) error {
	pools, err := r.listPools(ctx)
	if err != nil {
		return err
	}
	allocs, err := r.listAllocations(ctx)
	if err != nil {
		return err
	}
	for i := range pools {
		pool := &pools[i]
		// Copy, not alias: setPoolCondition below mutates desired.Conditions in place, and an
		// alias of pool.Status.Conditions would make the sameConditionsIgnoringTime comparison
		// further down always see the (already-mutated) two sides as equal, so the Ready
		// condition would never actually transition after its first write.
		desired := frpv1alpha1.ServerPoolStatus{Conditions: append([]metav1.Condition(nil), pool.Status.Conditions...)}
		for _, s := range pool.Spec.Servers {
			st := frpv1alpha1.ServerPoolServerStatus{Name: s.Name}
			for _, a := range allocs {
				if a.Pool == pool.Name && a.Server == s.Name {
					st.AllocatedPorts = append(st.AllocatedPorts, frpv1alpha1.AllocatedPort{Port: a.Port, Protocol: string(a.Protocol), Service: a.ServiceKey})
				}
			}
			sort.Slice(st.AllocatedPorts, func(x, y int) bool {
				if st.AllocatedPorts[x].Port != st.AllocatedPorts[y].Port {
					return st.AllocatedPorts[x].Port < st.AllocatedPorts[y].Port
				}
				return st.AllocatedPorts[x].Protocol < st.AllocatedPorts[y].Protocol
			})
			metrics.SetPoolAllocated(pool.Name, s.Name, len(st.AllocatedPorts))
			desired.Servers = append(desired.Servers, st)
		}
		readyMsg, readyStatus := r.poolReady(ctx, pool)
		desired.Conditions = setPoolCondition(desired.Conditions, metav1.Condition{
			Type: status.ConditionTypeReady, Status: readyStatus, Reason: "Validated", Message: readyMsg,
		})
		if reflect.DeepEqual(pool.Status.Servers, desired.Servers) && sameConditionsIgnoringTime(pool.Status.Conditions, desired.Conditions) {
			continue
		}
		pool.Status = desired
		if err := r.Status().Update(ctx, pool); err != nil && !apierrors.IsNotFound(err) {
			return err
		}
	}
	return nil
}

// poolReady validates server names are unique, allowedPorts parse, and token Secrets exist.
func (r *ServiceReconciler) poolReady(ctx context.Context, pool *frpv1alpha1.ServerPool) (string, metav1.ConditionStatus) {
	seen := map[string]bool{}
	if _, err := loadbalancer.ParsePortRanges(pool.Spec.AllowedPorts); err != nil {
		return err.Error(), metav1.ConditionFalse
	}
	for _, s := range pool.Spec.Servers {
		if seen[s.Name] {
			return fmt.Sprintf("duplicate server name %q", s.Name), metav1.ConditionFalse
		}
		seen[s.Name] = true
		if len(s.Name) > maxServerNameLength || !serverNameRE.MatchString(s.Name) {
			return fmt.Sprintf("server name %q must match %s and be at most %d characters", s.Name, serverNameRE.String(), maxServerNameLength), metav1.ConditionFalse
		}
		if _, err := loadbalancer.ParsePortRanges(s.AllowedPorts); err != nil {
			return fmt.Sprintf("server %s: %v", s.Name, err), metav1.ConditionFalse
		}
		if s.Authentication.Token != nil {
			sec := &corev1.Secret{}
			if err := r.Get(ctx, types.NamespacedName{Namespace: pool.Namespace, Name: s.Authentication.Token.Secret.Name}, sec); err != nil {
				return fmt.Sprintf("server %s: token secret %q: %v", s.Name, s.Authentication.Token.Secret.Name, err), metav1.ConditionFalse
			}
		}
	}
	return "pool is valid", metav1.ConditionTrue
}

func setPoolCondition(conds []metav1.Condition, c metav1.Condition) []metav1.Condition {
	c.LastTransitionTime = metav1.Now()
	for i := range conds {
		if conds[i].Type == c.Type {
			if conds[i].Status == c.Status {
				c.LastTransitionTime = conds[i].LastTransitionTime
			}
			conds[i] = c
			return conds
		}
	}
	return append(conds, c)
}

func sameConditionsIgnoringTime(a, b []metav1.Condition) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Type != b[i].Type || a[i].Status != b[i].Status || a[i].Reason != b[i].Reason || a[i].Message != b[i].Message {
			return false
		}
	}
	return true
}

func serviceOfInterest(obj ctrlclient.Object) bool {
	svc, ok := obj.(*corev1.Service)
	if !ok {
		return false
	}
	return loadbalancer.IsClaimed(svc) || controllerutil.ContainsFinalizer(svc, loadbalancer.Finalizer)
}

// ClaimedServiceRequests enqueues every Service we own; used when a pool or a generated object changes.
func (r *ServiceReconciler) ClaimedServiceRequests(ctx context.Context) []reconcile.Request {
	list := &corev1.ServiceList{}
	if err := r.List(ctx, list); err != nil {
		return nil
	}
	var reqs []reconcile.Request
	for i := range list.Items {
		if serviceOfInterest(&list.Items[i]) {
			reqs = append(reqs, reconcile.Request{NamespacedName: types.NamespacedName{Namespace: list.Items[i].Namespace, Name: list.Items[i].Name}})
		}
	}
	return reqs
}

// clientStatusChanged reports whether a generated Client's observable status changed in a way
// that should re-trigger reconciliation of its owning Service. ClientReconciler.setCondition
// re-stamps LastTransitionTime on every ~30s reconcile even when nothing else changed, so
// conditions are compared ignoring that field; Status.Message and Labels are still compared
// exactly.
func clientStatusChanged(e event.UpdateEvent) bool {
	oldC, ok1 := e.ObjectOld.(*frpv1alpha1.Client)
	newC, ok2 := e.ObjectNew.(*frpv1alpha1.Client)
	if !ok1 || !ok2 {
		return false
	}
	return !sameConditionsIgnoringTime(oldC.Status.Conditions, newC.Status.Conditions) ||
		oldC.Status.Message != newC.Status.Message ||
		!reflect.DeepEqual(oldC.Labels, newC.Labels)
}

func (r *ServiceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Recorder = mgr.GetEventRecorderFor("service-controller")
	all := ctrlhandler.EnqueueRequestsFromMapFunc(func(ctx context.Context, _ ctrlclient.Object) []reconcile.Request {
		return r.ClaimedServiceRequests(ctx)
	})
	managed := predicate.NewPredicateFuncs(func(obj ctrlclient.Object) bool {
		return obj.GetNamespace() == r.OperatorNamespace &&
			obj.GetLabels()[loadbalancer.LabelManagedBy] == loadbalancer.ManagedByValue
	})
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Service{}, ctrlbuilder.WithPredicates(predicate.NewPredicateFuncs(serviceOfInterest))).
		Watches(&frpv1alpha1.ServerPool{}, all).
		Watches(&frpv1alpha1.Upstream{}, all, ctrlbuilder.WithPredicates(managed)).
		Watches(&frpv1alpha1.Client{}, all, ctrlbuilder.WithPredicates(managed, predicate.Funcs{
			// Only status/label changes of generated Clients matter (Ready flips); spec updates come from us.
			UpdateFunc: clientStatusChanged,
		})).
		Complete(r)
}
