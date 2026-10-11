package controllers

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	ctrlhandler "sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	frpv1alpha1 "github.com/zufardhiyaulhaq/frp-operator/api/v1alpha1"
	"github.com/zufardhiyaulhaq/frp-operator/pkg/client/status"
	"github.com/zufardhiyaulhaq/frp-operator/pkg/frps"
)

// ServerReconciler runs frps for each Server: one pod, its config, a public and a web Service.
type ServerReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
	// MaxPorts caps the ports of a Server's public Service (--server-max-ports).
	MaxPorts int
	// LoadBalancerError returns the latest SyncLoadBalancerFailed message for a Service, or "".
	LoadBalancerError func(ctx context.Context, namespace, service string) (string, error)
	// ServiceMonitorAvailable reports whether the cluster serves monitoring.coreos.com/v1 ServiceMonitor.
	ServiceMonitorAvailable func() bool
}

//+kubebuilder:rbac:groups=frp.zufardhiyaulhaq.com,resources=servers,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=frp.zufardhiyaulhaq.com,resources=servers/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=frp.zufardhiyaulhaq.com,resources=servers/finalizers,verbs=update
//+kubebuilder:rbac:groups="",resources=events,verbs=create;patch;list
//+kubebuilder:rbac:groups=monitoring.coreos.com,resources=servicemonitors,verbs=get;list;watch;create;update;patch;delete

func (r *ServerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)
	srv := &frpv1alpha1.Server{}
	if err := r.Get(ctx, req.NamespacedName, srv); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	res, err := frps.Resolve(ctx, r.Client, srv, r.MaxPorts)
	if err != nil {
		// The running pod keeps its last good configuration; say why the spec is not applied.
		msg := fmt.Sprintf("invalid configuration: %v", err)
		log.Error(err, "invalid server configuration")
		r.Recorder.Event(srv, corev1.EventTypeWarning, frps.ReasonInvalidConfig, msg)
		setServerCondition(srv, status.ConditionTypeConfigSync, metav1.ConditionFalse, frps.ReasonInvalidConfig, msg)
		if statusErr := r.Status().Update(ctx, srv); statusErr != nil {
			log.Error(statusErr, "failed to update server status")
		}
		return ctrl.Result{}, err
	}

	newGeneration := srv.Status.ObservedGeneration != srv.Generation
	if newGeneration {
		r.warnAboutSpec(srv)
	}
	if res.NewToken && srv.Status.ConnectionSecret != "" {
		r.Recorder.Eventf(srv, corev1.EventTypeWarning, frps.ReasonTokenRotated,
			"Secret %s was missing, so a new token was generated: copy it to every connected cluster", frps.ConnectionSecretName(srv))
	}

	if err := r.applySecret(ctx, srv, frps.BuildConnectionSecret(srv, res.Token, res.ConnectionCA)); err != nil {
		return ctrl.Result{}, err
	}
	if res.Dashboard != nil && res.GeneratedDashboard {
		if err := r.applySecret(ctx, srv, frps.BuildDashboardSecret(srv, *res.Dashboard)); err != nil {
			return ctrl.Result{}, err
		}
	}

	pod, pending, err := r.reconcilePod(ctx, srv, res)
	if err != nil {
		return ctrl.Result{}, err
	}

	// While a Manual restart is pending, frps still listens on its old ports: keep the Services as
	// they are (only create a missing one) and move them when the new pod starts.
	svc, err := r.applyService(ctx, srv, frps.BuildService(srv, res.Ports), pending)
	if err != nil {
		return ctrl.Result{}, err
	}
	webService := frps.BuildWebService(srv)
	switch {
	case pending:
	case res.Ports.WebServer:
		_, err = r.applyService(ctx, srv, webService, false)
	default:
		err = r.deleteIfExists(ctx, webService)
	}
	if err != nil {
		return ctrl.Result{}, err
	}
	if err := r.reconcileServiceMonitor(ctx, srv, newGeneration); err != nil {
		return ctrl.Result{}, err
	}
	if pending && !hasServerReason(srv, status.ConditionTypeConfigSync, frps.ReasonRestartPending) {
		r.Recorder.Eventf(srv, corev1.EventTypeNormal, frps.ReasonRestartPending,
			"The configuration changed: delete pod %s to apply it (restartPolicy Manual)", frps.PodName(srv))
	}

	address := frps.Address(svc)
	lbError := ""
	if address == "" && svc.Spec.Type == corev1.ServiceTypeLoadBalancer && r.LoadBalancerError != nil {
		if lbError, err = r.LoadBalancerError(ctx, svc.Namespace, svc.Name); err != nil {
			log.Error(err, "failed to read load balancer events")
			lbError = ""
		}
	}
	setServerStatus(srv, res, pod, pending, address, lbError)
	if err := r.Status().Update(ctx, srv); err != nil {
		return ctrl.Result{}, err
	}
	if pod == nil {
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}
	return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
}

// reconcilePod returns the running pod (nil while frps restarts) and whether a Manual restart is
// pending. The config Secret is written only when a pod is about to start, so a running pod's
// mounted config never changes under it — a crash restart under Manual keeps the old config.
func (r *ServerReconciler) reconcilePod(ctx context.Context, srv *frpv1alpha1.Server, res *frps.Resolved) (*corev1.Pod, bool, error) {
	desired := frps.BuildPod(srv, res.Ports)
	desired.Annotations[frps.ConfigHashAnnotation] = frps.ConfigHash(res.TOML, res.Mounted, desired.Spec)
	if err := controllerutil.SetControllerReference(srv, desired, r.Scheme); err != nil {
		return nil, false, err
	}

	current := &corev1.Pod{}
	err := r.Get(ctx, client.ObjectKeyFromObject(desired), current)
	if apierrors.IsNotFound(err) {
		if err := r.applySecret(ctx, srv, frps.BuildConfigSecret(srv, res.TOML)); err != nil {
			return nil, false, err
		}
		if err := r.Create(ctx, desired); err != nil {
			return nil, false, err
		}
		r.Recorder.Eventf(srv, corev1.EventTypeNormal, frps.ReasonPodCreated, "Started frps pod %s", desired.Name)
		return desired, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if current.DeletionTimestamp != nil {
		return nil, false, nil
	}
	// A terminated pod (evicted, node shutdown) is never restarted by the kubelet: replace it,
	// under either restart policy.
	if current.Status.Phase == corev1.PodFailed || current.Status.Phase == corev1.PodSucceeded {
		if err := r.Delete(ctx, current); err != nil && !apierrors.IsNotFound(err) {
			return nil, false, err
		}
		r.Recorder.Eventf(srv, corev1.EventTypeWarning, frps.ReasonPodFailed, "frps pod %s %s (%s %s): starting a new one",
			current.Name, current.Status.Phase, current.Status.Reason, current.Status.Message)
		return nil, false, nil
	}
	if current.Annotations[frps.ConfigHashAnnotation] == desired.Annotations[frps.ConfigHashAnnotation] {
		return current, false, nil
	}
	if frps.RestartPolicy(srv) == frpv1alpha1.RestartPolicyManual {
		return current, true, nil
	}
	if err := r.Delete(ctx, current); err != nil && !apierrors.IsNotFound(err) {
		return nil, false, err
	}
	r.Recorder.Event(srv, corev1.EventTypeNormal, frps.ReasonRestarting, "The configuration changed: restarting frps")
	return nil, false, nil
}

func setServerStatus(srv *frpv1alpha1.Server, res *frps.Resolved, pod *corev1.Pod, pending bool, address, lbError string) {
	srv.Status.Address = address
	srv.Status.ConnectionSecret = frps.ConnectionSecretName(srv)
	srv.Status.ExposedPorts = &frpv1alpha1.ServerStatus_ExposedPorts{TCP: res.Ports.AllowTCP, UDP: res.Ports.AllowUDP}
	srv.Status.TLS = nil
	if len(res.ServerNames) > 0 {
		srv.Status.TLS = &frpv1alpha1.ServerStatus_TLS{ServerNames: res.ServerNames}
	}
	srv.Status.ObservedGeneration = srv.Generation

	switch {
	case pod == nil:
		setServerCondition(srv, status.ConditionTypeConfigSync, metav1.ConditionFalse, frps.ReasonRestarting, "Restarting frps with the new configuration")
	case pending:
		setServerCondition(srv, status.ConditionTypeConfigSync, metav1.ConditionFalse, frps.ReasonRestartPending,
			fmt.Sprintf("restartPolicy is Manual: delete pod %s to apply the new configuration", pod.Name))
	default:
		setServerCondition(srv, status.ConditionTypeConfigSync, metav1.ConditionTrue, frps.ReasonConfigApplied, "frps runs the current configuration")
	}

	ready, problem := false, ""
	if pod != nil {
		ready, problem = frps.PodState(pod)
	}
	switch {
	case problem != "":
		setServerCondition(srv, status.ConditionTypeReady, metav1.ConditionFalse, frps.ReasonPodFailed, problem)
	case !ready:
		setServerCondition(srv, status.ConditionTypeReady, metav1.ConditionFalse, frps.ReasonPodStarting, "Waiting for the frps pod to be ready")
	case address == "" && lbError != "":
		setServerCondition(srv, status.ConditionTypeReady, metav1.ConditionFalse, frps.ReasonLoadBalancerFailed, lbError)
	case address == "":
		setServerCondition(srv, status.ConditionTypeReady, metav1.ConditionFalse, frps.ReasonAddressPending, "Waiting for the load balancer to assign an address")
	default:
		setServerCondition(srv, status.ConditionTypeReady, metav1.ConditionTrue, frps.ReasonReady, "frps is reachable at "+address)
	}
}

// warnAboutSpec emits the spec's Warning events once per generation.
func (r *ServerReconciler) warnAboutSpec(srv *frpv1alpha1.Server) {
	if t := srv.Spec.Bind.TLS; t == nil || t.Certificate == nil {
		r.Recorder.Event(srv, corev1.EventTypeWarning, frps.ReasonTunnelTLSUnverified,
			"bind.tls.certificate is not set: frps uses a random certificate, so clients cannot verify the server they connect to")
	}
	if frps.DashboardInsecure(srv) {
		r.Recorder.Event(srv, corev1.EventTypeWarning, frps.ReasonDashboardInsecure,
			"The dashboard Service is reachable outside the cluster without dashboard.tls: its login and admin API travel in clear text")
	}
}

func (r *ServerReconciler) reconcileServiceMonitor(ctx context.Context, srv *frpv1alpha1.Server, newGeneration bool) error {
	want := srv.Spec.Metrics != nil && srv.Spec.Metrics.ServiceMonitor
	if r.ServiceMonitorAvailable == nil || !r.ServiceMonitorAvailable() {
		if want && newGeneration {
			r.Recorder.Event(srv, corev1.EventTypeWarning, frps.ReasonServiceMonitorUnavailable,
				"metrics.serviceMonitor is set, but the ServiceMonitor CRD (monitoring.coreos.com/v1) is not installed")
		}
		return nil
	}
	desired := frps.BuildServiceMonitor(srv)
	if !want {
		return r.deleteIfExists(ctx, desired)
	}
	current := &unstructured.Unstructured{}
	current.SetGroupVersionKind(desired.GroupVersionKind())
	current.SetNamespace(desired.GetNamespace())
	current.SetName(desired.GetName())
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, current, func() error {
		current.SetLabels(desired.GetLabels())
		current.Object["spec"] = desired.Object["spec"]
		return controllerutil.SetControllerReference(srv, current, r.Scheme)
	})
	return err
}

func (r *ServerReconciler) applySecret(ctx context.Context, srv *frpv1alpha1.Server, desired *corev1.Secret) error {
	secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: desired.Name, Namespace: desired.Namespace}}
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, secret, func() error {
		secret.Labels = desired.Labels
		if secret.Type == "" {
			secret.Type = desired.Type
		}
		secret.Data = desired.Data
		return controllerutil.SetControllerReference(srv, secret, r.Scheme)
	})
	return err
}

// applyService patches a Service in place (it is never recreated, so its IP survives) and clears
// the fields the API server rejects after a type or traffic-policy change.
// With keepExisting, an existing Service is returned unchanged.
func (r *ServerReconciler) applyService(ctx context.Context, srv *frpv1alpha1.Server, desired *corev1.Service, keepExisting bool) (*corev1.Service, error) {
	svc := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: desired.Name, Namespace: desired.Namespace}}
	if keepExisting {
		err := r.Get(ctx, client.ObjectKeyFromObject(svc), svc)
		if err == nil {
			return svc, nil
		}
		if !apierrors.IsNotFound(err) {
			return nil, err
		}
	}
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, svc, func() error {
		svc.Labels = desired.Labels
		if svc.Annotations == nil {
			svc.Annotations = map[string]string{}
		}
		for k, v := range desired.Annotations {
			svc.Annotations[k] = v
		}
		svc.Spec.Type = desired.Spec.Type
		svc.Spec.Selector = desired.Spec.Selector
		svc.Spec.Ports = frps.KeepNodePorts(svc.Spec.Ports, desired.Spec.Ports, desired.Spec.Type)
		svc.Spec.ExternalTrafficPolicy = desired.Spec.ExternalTrafficPolicy
		svc.Spec.LoadBalancerSourceRanges = desired.Spec.LoadBalancerSourceRanges
		if desired.Spec.Type == corev1.ServiceTypeLoadBalancer {
			if svc.Spec.LoadBalancerClass == nil { // immutable once set
				svc.Spec.LoadBalancerClass = desired.Spec.LoadBalancerClass
			}
		} else {
			svc.Spec.LoadBalancerClass = nil
			svc.Spec.AllocateLoadBalancerNodePorts = nil
		}
		if desired.Spec.ExternalTrafficPolicy != corev1.ServiceExternalTrafficPolicyLocal {
			svc.Spec.HealthCheckNodePort = 0
		}
		return controllerutil.SetControllerReference(srv, svc, r.Scheme)
	})
	return svc, err
}

func (r *ServerReconciler) deleteIfExists(ctx context.Context, obj client.Object) error {
	if err := r.Delete(ctx, obj); err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	return nil
}

func setServerCondition(srv *frpv1alpha1.Server, condType string, st metav1.ConditionStatus, reason, message string) {
	meta.SetStatusCondition(&srv.Status.Conditions, metav1.Condition{
		Type: condType, Status: st, Reason: reason, Message: message, ObservedGeneration: srv.Generation,
	})
}

func hasServerReason(srv *frpv1alpha1.Server, condType, reason string) bool {
	c := meta.FindStatusCondition(srv.Status.Conditions, condType)
	return c != nil && c.Reason == reason
}

// serversForSecret enqueues the Servers in the Secret's namespace that read or own it, so a
// rotated token or certificate takes effect without waiting for the next requeue.
func (r *ServerReconciler) serversForSecret(ctx context.Context, obj client.Object) []reconcile.Request {
	servers := &frpv1alpha1.ServerList{}
	if err := r.List(ctx, servers, client.InNamespace(obj.GetNamespace())); err != nil {
		return nil
	}
	var requests []reconcile.Request
	for i := range servers.Items {
		if slices.Contains(frps.SecretNames(&servers.Items[i]), obj.GetName()) {
			requests = append(requests, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(&servers.Items[i])})
		}
	}
	return requests
}

// latestEventMessage returns the message of the most recent event, or "".
func latestEventMessage(events []corev1.Event) string {
	if len(events) == 0 {
		return ""
	}
	at := func(e corev1.Event) time.Time {
		if !e.LastTimestamp.IsZero() {
			return e.LastTimestamp.Time
		}
		return e.EventTime.Time
	}
	sort.Slice(events, func(i, j int) bool { return at(events[i]).Before(at(events[j])) })
	return events[len(events)-1].Message
}

// SetupWithManager sets up the controller with the Manager.
func (r *ServerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if r.Recorder == nil {
		r.Recorder = mgr.GetEventRecorderFor("server-controller")
	}
	if r.LoadBalancerError == nil {
		clientset, err := kubernetes.NewForConfig(mgr.GetConfig())
		if err != nil {
			return fmt.Errorf("failed to create clientset: %w", err)
		}
		// Read live with a field selector: caching every Event in the cluster would be costly.
		r.LoadBalancerError = func(ctx context.Context, namespace, service string) (string, error) {
			events, err := clientset.CoreV1().Events(namespace).List(ctx, metav1.ListOptions{
				FieldSelector: "involvedObject.kind=Service,involvedObject.name=" + service + ",reason=SyncLoadBalancerFailed",
			})
			if err != nil {
				return "", err
			}
			return latestEventMessage(events.Items), nil
		}
	}
	if r.ServiceMonitorAvailable == nil {
		mapper := mgr.GetRESTMapper()
		r.ServiceMonitorAvailable = func() bool {
			_, err := mapper.RESTMapping(schema.GroupKind{Group: "monitoring.coreos.com", Kind: "ServiceMonitor"}, "v1")
			return err == nil
		}
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&frpv1alpha1.Server{}).
		Owns(&corev1.Pod{}).
		Owns(&corev1.Service{}).
		Watches(&corev1.Secret{}, ctrlhandler.EnqueueRequestsFromMapFunc(r.serversForSecret)).
		Complete(r)
}
