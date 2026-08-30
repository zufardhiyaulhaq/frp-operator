package controllers

import (
	"context"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/event"

	frpv1alpha1 "github.com/zufardhiyaulhaq/frp-operator/api/v1alpha1"
	"github.com/zufardhiyaulhaq/frp-operator/pkg/client/handler"
	"github.com/zufardhiyaulhaq/frp-operator/pkg/client/models"
	"github.com/zufardhiyaulhaq/frp-operator/pkg/loadbalancer"
)

const opNS = "frp-operator"

func lbScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	if err := frpv1alpha1.AddToScheme(s); err != nil {
		t.Fatal(err)
	}
	if err := corev1.AddToScheme(s); err != nil {
		t.Fatal(err)
	}
	return s
}

func lbPool(name string, mode string, servers ...string) *frpv1alpha1.ServerPool {
	p := &frpv1alpha1.ServerPool{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: opNS},
		Spec:       frpv1alpha1.ServerPoolSpec{AllocationPolicy: frpv1alpha1.AllocationPolicyAuto, ClientMode: mode},
	}
	for _, s := range servers {
		p.Spec.Servers = append(p.Spec.Servers, frpv1alpha1.ServerPoolServer{
			Name: s, Host: s + ".example", Port: 7000, PublicAddress: "203.0.113." + s[len(s)-1:],
			Authentication: frpv1alpha1.ClientSpec_Server_Authentication{
				Token: &frpv1alpha1.ClientSpec_Server_Authentication_Token{Secret: frpv1alpha1.Secret{Name: "tok", Key: "token"}},
			},
		})
	}
	return p
}

func lbService(ns, name string, ports ...int32) *corev1.Service {
	class := loadbalancer.LoadBalancerClass
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns, UID: types.UID("uid-" + name), CreationTimestamp: metav1.Now()},
		Spec:       corev1.ServiceSpec{Type: corev1.ServiceTypeLoadBalancer, LoadBalancerClass: &class},
	}
	for _, p := range ports {
		svc.Spec.Ports = append(svc.Spec.Ports, corev1.ServicePort{Name: "p", Port: p, Protocol: corev1.ProtocolTCP})
	}
	return svc
}

func tokenSecret() *corev1.Secret {
	return &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "tok", Namespace: opNS}, Data: map[string][]byte{"token": []byte("x")}}
}

func newLBReconciler(t *testing.T, objs ...ctrlclient.Object) (*ServiceReconciler, ctrlclient.Client) {
	t.Helper()
	c := fake.NewClientBuilder().WithScheme(lbScheme(t)).WithObjects(objs...).
		WithStatusSubresource(&corev1.Service{}, &frpv1alpha1.ServerPool{}, &frpv1alpha1.Client{}).Build()
	r := &ServiceReconciler{
		Client: c, Scheme: c.Scheme(), Recorder: record.NewFakeRecorder(200), OperatorNamespace: opNS,
		StatusFunc: func(models.Config) ([]handler.ProxyStatus, error) { return nil, nil },
	}
	return r, c
}

// reconcileUntilStable runs Reconcile up to n times (finalizer add → allocate → status).
func reconcileUntilStable(t *testing.T, r *ServiceReconciler, svc *corev1.Service, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		if _, err := r.Reconcile(context.TODO(), ctrl.Request{NamespacedName: types.NamespacedName{Namespace: svc.Namespace, Name: svc.Name}}); err != nil {
			t.Fatalf("reconcile %d: %v", i, err)
		}
	}
}

func getService(t *testing.T, c ctrlclient.Client, ns, name string) *corev1.Service {
	t.Helper()
	svc := &corev1.Service{}
	if err := c.Get(context.TODO(), types.NamespacedName{Namespace: ns, Name: name}, svc); err != nil {
		t.Fatal(err)
	}
	return svc
}

func listManagedUpstreams(t *testing.T, c ctrlclient.Client) []frpv1alpha1.Upstream {
	t.Helper()
	list := &frpv1alpha1.UpstreamList{}
	if err := c.List(context.TODO(), list, ctrlclient.InNamespace(opNS), ctrlclient.MatchingLabels{loadbalancer.LabelManagedBy: loadbalancer.ManagedByValue}); err != nil {
		t.Fatal(err)
	}
	return list.Items
}

func TestServiceReconcile_IgnoresUnclaimed(t *testing.T) {
	svc := lbService("default", "web", 443)
	svc.Spec.LoadBalancerClass = nil
	r, c := newLBReconciler(t, svc, lbPool("p", frpv1alpha1.ClientModePerService, "s1"), tokenSecret())
	reconcileUntilStable(t, r, svc, 2)
	if got := getService(t, c, "default", "web"); len(got.Finalizers) != 0 {
		t.Fatalf("unclaimed service must not get a finalizer: %v", got.Finalizers)
	}
	if n := len(listManagedUpstreams(t, c)); n != 0 {
		t.Fatalf("generated %d upstreams for unclaimed service", n)
	}
}

func TestServiceReconcile_GeneratesClientAndUpstreams(t *testing.T) {
	svc := lbService("default", "web", 80, 443)
	r, c := newLBReconciler(t, svc, lbPool("p", frpv1alpha1.ClientModePerService, "s1"), tokenSecret())
	reconcileUntilStable(t, r, svc, 3)

	got := getService(t, c, "default", "web")
	if got.Annotations[loadbalancer.AnnotationAllocated] != "p/s1" {
		t.Fatalf("allocated annotation: %q", got.Annotations[loadbalancer.AnnotationAllocated])
	}
	if len(got.Finalizers) != 1 || got.Finalizers[0] != loadbalancer.Finalizer {
		t.Fatalf("finalizer: %v", got.Finalizers)
	}
	client := &frpv1alpha1.Client{}
	if err := c.Get(context.TODO(), types.NamespacedName{Namespace: opNS, Name: "lb-default-web-s1"}, client); err != nil {
		t.Fatalf("client: %v", err)
	}
	if client.Spec.Server.Host != "s1.example" {
		t.Fatalf("client host: %s", client.Spec.Server.Host)
	}
	ups := listManagedUpstreams(t, c)
	if len(ups) != 2 {
		t.Fatalf("upstreams: %d", len(ups))
	}
	// Client not Ready yet → no ingress
	if len(got.Status.LoadBalancer.Ingress) != 0 {
		t.Fatalf("ingress must wait for Ready client: %+v", got.Status.LoadBalancer.Ingress)
	}
}

func markClientReady(t *testing.T, c ctrlclient.Client, name string) {
	t.Helper()
	client := &frpv1alpha1.Client{}
	if err := c.Get(context.TODO(), types.NamespacedName{Namespace: opNS, Name: name}, client); err != nil {
		t.Fatal(err)
	}
	client.Status.Conditions = []metav1.Condition{{Type: "Ready", Status: metav1.ConditionTrue, Reason: "PodRunning", LastTransitionTime: metav1.Now()}}
	if err := c.Status().Update(context.TODO(), client); err != nil {
		t.Fatal(err)
	}
}

func TestServiceReconcile_IngressGatedOnReadyAndRunning(t *testing.T) {
	svc := lbService("default", "web", 443)
	r, c := newLBReconciler(t, svc, lbPool("p", frpv1alpha1.ClientModePerService, "s1"), tokenSecret())
	reconcileUntilStable(t, r, svc, 3)
	markClientReady(t, c, "lb-default-web-s1")

	// Ready but proxy not running → still no ingress, and item 5 of the final-review fix wave:
	// the frp_loadbalancer_service metric must read "pending" (bound was selected, but there is
	// no working ingress yet), not "bound".
	r.StatusFunc = func(models.Config) ([]handler.ProxyStatus, error) {
		return []handler.ProxyStatus{{Name: "lb-default-web-s1-443", Type: "tcp", Status: "start error", Err: "port already used"}}, nil
	}
	reconcileUntilStable(t, r, svc, 1)
	if got := getService(t, c, "default", "web"); len(got.Status.LoadBalancer.Ingress) != 0 {
		t.Fatalf("ingress with proxy in start error: %+v", got.Status.LoadBalancer.Ingress)
	}
	if v, ok := gaugeValue(t, "frp_loadbalancer_service", map[string]string{"namespace": "default", "service": "web", "state": "pending"}); !ok || v != 1 {
		t.Fatalf("want state=pending metric while ingress is empty, got ok=%v v=%v", ok, v)
	}
	if _, ok := gaugeValue(t, "frp_loadbalancer_service", map[string]string{"namespace": "default", "service": "web", "state": "bound"}); ok {
		t.Fatalf("must not report state=bound while ingress is empty")
	}

	r.StatusFunc = func(models.Config) ([]handler.ProxyStatus, error) {
		return []handler.ProxyStatus{{Name: "lb-default-web-s1-443", Type: "tcp", Status: "running"}}, nil
	}
	reconcileUntilStable(t, r, svc, 1)
	got := getService(t, c, "default", "web")
	if len(got.Status.LoadBalancer.Ingress) != 1 || got.Status.LoadBalancer.Ingress[0].IP != "203.0.113.1" {
		t.Fatalf("ingress: %+v", got.Status.LoadBalancer.Ingress)
	}
	// Item 6 of the final-review fix wave: an IP ingress entry must carry IPMode: Proxy (the frps
	// server's public address is delivered to the node/pod, not acting as a real LB VIP).
	if mode := got.Status.LoadBalancer.Ingress[0].IPMode; mode == nil || *mode != corev1.LoadBalancerIPModeProxy {
		t.Fatalf("want IPMode Proxy on an IP ingress entry, got %v", mode)
	}
	if v, ok := gaugeValue(t, "frp_loadbalancer_service", map[string]string{"namespace": "default", "service": "web", "state": "bound"}); !ok || v != 1 {
		t.Fatalf("want state=bound metric once ingress is populated, got ok=%v v=%v", ok, v)
	}
}

func TestServiceReconcile_HostnamePublicAddress(t *testing.T) {
	svc := lbService("default", "web", 443)
	p := lbPool("p", frpv1alpha1.ClientModePerService, "s1")
	p.Spec.Servers[0].PublicAddress = "lb.example.com"
	r, c := newLBReconciler(t, svc, p, tokenSecret())
	reconcileUntilStable(t, r, svc, 3)
	markClientReady(t, c, "lb-default-web-s1")
	r.StatusFunc = func(models.Config) ([]handler.ProxyStatus, error) {
		return []handler.ProxyStatus{{Name: "lb-default-web-s1-443", Status: "running"}}, nil
	}
	reconcileUntilStable(t, r, svc, 1)
	got := getService(t, c, "default", "web")
	if len(got.Status.LoadBalancer.Ingress) != 1 || got.Status.LoadBalancer.Ingress[0].Hostname != "lb.example.com" || got.Status.LoadBalancer.Ingress[0].IP != "" {
		t.Fatalf("ingress: %+v", got.Status.LoadBalancer.Ingress)
	}
	// Item 6 of the final-review fix wave: IPMode is only meaningful for IP entries; a Hostname
	// entry must not have it set.
	if mode := got.Status.LoadBalancer.Ingress[0].IPMode; mode != nil {
		t.Fatalf("want no IPMode on a Hostname ingress entry, got %v", *mode)
	}
}

func TestServiceReconcile_SecondServiceSamePortPendingThenTakesOver(t *testing.T) {
	a := lbService("default", "a", 443)
	b := lbService("default", "b", 443)
	b.CreationTimestamp = metav1.NewTime(a.CreationTimestamp.Add(1))
	r, c := newLBReconciler(t, a, b, lbPool("p", frpv1alpha1.ClientModePerService, "s1"), tokenSecret())
	reconcileUntilStable(t, r, a, 3)
	reconcileUntilStable(t, r, b, 3)

	if got := getService(t, c, "default", "b"); got.Annotations[loadbalancer.AnnotationAllocated] != "" {
		t.Fatalf("b must be pending, got %q", got.Annotations[loadbalancer.AnnotationAllocated])
	}
	if n := len(listManagedUpstreams(t, c)); n != 1 {
		t.Fatalf("upstreams: %d, want 1", n)
	}

	// delete a → finalizer path cleans up
	if err := c.Delete(context.TODO(), getService(t, c, "default", "a")); err != nil {
		t.Fatal(err)
	}
	reconcileUntilStable(t, r, a, 2)
	if err := c.Get(context.TODO(), types.NamespacedName{Namespace: "default", Name: "a"}, &corev1.Service{}); err == nil {
		t.Fatal("a should be gone after finalizer removal")
	}
	if n := len(listManagedUpstreams(t, c)); n != 0 {
		t.Fatalf("a's upstreams not cleaned: %d", n)
	}

	reconcileUntilStable(t, r, b, 2)
	if got := getService(t, c, "default", "b"); got.Annotations[loadbalancer.AnnotationAllocated] != "p/s1" {
		t.Fatalf("b must bind after a is gone, got %q", got.Annotations[loadbalancer.AnnotationAllocated])
	}
}

func TestServiceReconcile_StickyAndReallocateWhenServerRemoved(t *testing.T) {
	svc := lbService("default", "web", 443)
	p := lbPool("p", frpv1alpha1.ClientModePerService, "s1", "s2")
	r, c := newLBReconciler(t, svc, p, tokenSecret())
	reconcileUntilStable(t, r, svc, 3)
	if got := getService(t, c, "default", "web"); got.Annotations[loadbalancer.AnnotationAllocated] != "p/s1" {
		t.Fatalf("initial: %q", got.Annotations[loadbalancer.AnnotationAllocated])
	}

	// A new pool sorting earlier appears → must stay on p/s1
	if err := c.Create(context.TODO(), lbPool("a", frpv1alpha1.ClientModePerService, "a1")); err != nil {
		t.Fatal(err)
	}
	reconcileUntilStable(t, r, svc, 2)
	if got := getService(t, c, "default", "web"); got.Annotations[loadbalancer.AnnotationAllocated] != "p/s1" {
		t.Fatalf("sticky violated: %q", got.Annotations[loadbalancer.AnnotationAllocated])
	}

	// remove s1 from pool p → reallocate (to a/a1, the first fit)
	cur := &frpv1alpha1.ServerPool{}
	if err := c.Get(context.TODO(), types.NamespacedName{Namespace: opNS, Name: "p"}, cur); err != nil {
		t.Fatal(err)
	}
	cur.Spec.Servers = cur.Spec.Servers[1:]
	if err := c.Update(context.TODO(), cur); err != nil {
		t.Fatal(err)
	}
	reconcileUntilStable(t, r, svc, 3)
	got := getService(t, c, "default", "web")
	if got.Annotations[loadbalancer.AnnotationAllocated] != "a/a1" {
		t.Fatalf("reallocation: %q", got.Annotations[loadbalancer.AnnotationAllocated])
	}
	for _, u := range listManagedUpstreams(t, c) {
		if u.Labels[loadbalancer.LabelServer] == "s1" {
			t.Fatalf("stale upstream on removed server: %s", u.Name)
		}
	}

	rec := r.Recorder.(*record.FakeRecorder)
	close(rec.Events)
	var sawBound, sawReallocated bool
	for e := range rec.Events {
		if strings.Contains(e, EventReasonBound) {
			sawBound = true
		}
		if strings.Contains(e, EventReasonServerReallocated) {
			sawReallocated = true
		}
	}
	if !sawBound {
		t.Fatal("expected a Bound event on initial binding")
	}
	if !sawReallocated {
		t.Fatal("expected a ServerReallocated event when s1 was removed from the pool")
	}
}

// TestServiceReconcile_RemovingServerAnnotationCollapsesToOne covers item 4 of the final-review
// fix wave: a multi-server binding only exists because the server annotation pinned that exact
// set. Removing the annotation must not be treated as "still valid" (boundStillValid used to
// return true whenever request.Servers was empty, regardless of how many servers were bound) —
// it must fall through to re-selection, which collapses the binding to a single server and tears
// down the other server's generated objects.
func TestServiceReconcile_RemovingServerAnnotationCollapsesToOne(t *testing.T) {
	svc := lbService("default", "web", 443)
	svc.Annotations = map[string]string{loadbalancer.AnnotationServer: "s1,s2"}
	pool := lbPool("p", frpv1alpha1.ClientModePerService, "s1", "s2")
	r, c := newLBReconciler(t, svc, pool, tokenSecret())
	reconcileUntilStable(t, r, svc, 3)

	got := getService(t, c, "default", "web")
	if got.Annotations[loadbalancer.AnnotationAllocated] != "p/s1,p/s2" {
		t.Fatalf("must bind both pinned servers: %q", got.Annotations[loadbalancer.AnnotationAllocated])
	}

	cur := getService(t, c, "default", "web")
	delete(cur.Annotations, loadbalancer.AnnotationServer)
	if err := c.Update(context.TODO(), cur); err != nil {
		t.Fatal(err)
	}
	reconcileUntilStable(t, r, svc, 3)

	got = getService(t, c, "default", "web")
	refs, err := loadbalancer.ParseAllocated(got.Annotations[loadbalancer.AnnotationAllocated])
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) != 1 {
		t.Fatalf("must collapse to exactly one server after removing the server annotation, got %q", got.Annotations[loadbalancer.AnnotationAllocated])
	}

	ups := listManagedUpstreams(t, c)
	if len(ups) != 1 {
		t.Fatalf("only the remaining server's upstream must survive, got %d: %v", len(ups), ups)
	}
	if ups[0].Labels[loadbalancer.LabelServer] != refs[0].Server {
		t.Fatalf("surviving upstream must belong to the surviving server %q, got %q", refs[0].Server, ups[0].Labels[loadbalancer.LabelServer])
	}
}

func TestServiceReconcile_SharedClientLifecycle(t *testing.T) {
	a := lbService("default", "a", 80)
	b := lbService("default", "b", 443)
	r, c := newLBReconciler(t, a, b, lbPool("p", frpv1alpha1.ClientModeShared, "s1"), tokenSecret())
	reconcileUntilStable(t, r, a, 3)
	reconcileUntilStable(t, r, b, 3)

	list := &frpv1alpha1.ClientList{}
	if err := c.List(context.TODO(), list, ctrlclient.InNamespace(opNS)); err != nil {
		t.Fatal(err)
	}
	if len(list.Items) != 1 || list.Items[0].Name != "pool-p-s1" {
		t.Fatalf("shared mode must create exactly one client: %+v", list.Items)
	}

	if err := c.Delete(context.TODO(), getService(t, c, "default", "a")); err != nil {
		t.Fatal(err)
	}
	reconcileUntilStable(t, r, a, 2)
	if err := c.Get(context.TODO(), types.NamespacedName{Namespace: opNS, Name: "pool-p-s1"}, &frpv1alpha1.Client{}); err != nil {
		t.Fatal("shared client must survive while b still uses it")
	}
	if err := c.Delete(context.TODO(), getService(t, c, "default", "b")); err != nil {
		t.Fatal(err)
	}
	reconcileUntilStable(t, r, b, 2)
	if err := c.Get(context.TODO(), types.NamespacedName{Namespace: opNS, Name: "pool-p-s1"}, &frpv1alpha1.Client{}); err == nil {
		t.Fatal("shared client must be deleted with its last upstream")
	}
}

func TestServiceReconcile_ClassRemovedCleansUp(t *testing.T) {
	svc := lbService("default", "web", 443)
	r, c := newLBReconciler(t, svc, lbPool("p", frpv1alpha1.ClientModePerService, "s1"), tokenSecret())
	reconcileUntilStable(t, r, svc, 3)

	cur := getService(t, c, "default", "web")
	cur.Spec.LoadBalancerClass = nil
	cur.Spec.Type = corev1.ServiceTypeClusterIP
	if err := c.Update(context.TODO(), cur); err != nil {
		t.Fatal(err)
	}
	reconcileUntilStable(t, r, svc, 2)
	got := getService(t, c, "default", "web")
	if len(got.Finalizers) != 0 || got.Annotations[loadbalancer.AnnotationAllocated] != "" || len(got.Status.LoadBalancer.Ingress) != 0 {
		t.Fatalf("not cleaned: fin=%v ann=%q ing=%v", got.Finalizers, got.Annotations[loadbalancer.AnnotationAllocated], got.Status.LoadBalancer.Ingress)
	}
	if n := len(listManagedUpstreams(t, c)); n != 0 {
		t.Fatalf("upstreams left: %d", n)
	}
}

func TestServiceReconcile_PoolStatusProjection(t *testing.T) {
	svc := lbService("default", "web", 443)
	r, c := newLBReconciler(t, svc, lbPool("p", frpv1alpha1.ClientModePerService, "s1"), tokenSecret())
	reconcileUntilStable(t, r, svc, 3)
	p := &frpv1alpha1.ServerPool{}
	if err := c.Get(context.TODO(), types.NamespacedName{Namespace: opNS, Name: "p"}, p); err != nil {
		t.Fatal(err)
	}
	if len(p.Status.Servers) != 1 || len(p.Status.Servers[0].AllocatedPorts) != 1 ||
		p.Status.Servers[0].AllocatedPorts[0].Port != 443 || p.Status.Servers[0].AllocatedPorts[0].Service != "default/web" {
		t.Fatalf("pool status: %+v", p.Status)
	}
}

// TestServiceReconcile_RaceLoserCleansUp simulates two Services that both already produced an
// Upstream for the same server/port (e.g. from a stale cache during a real race) and are both
// annotated as bound to it. The sticky path must resolve this with Winner(): the older Service
// (a) keeps its binding and objects, the younger one (b) gets its generated objects cleaned up
// and goes Pending, instead of both deadlocking in Pending forever.
func TestServiceReconcile_RaceLoserCleansUp(t *testing.T) {
	a := lbService("default", "a", 443)
	b := lbService("default", "b", 443)
	b.CreationTimestamp = metav1.NewTime(a.CreationTimestamp.Add(1))
	a.Finalizers = []string{loadbalancer.Finalizer}
	b.Finalizers = []string{loadbalancer.Finalizer}
	a.Annotations = map[string]string{loadbalancer.AnnotationAllocated: "p/s1"}
	b.Annotations = map[string]string{loadbalancer.AnnotationAllocated: "p/s1"}

	pool := lbPool("p", frpv1alpha1.ClientModePerService, "s1", "s2")
	ref := loadbalancer.ServerRef{Pool: "p", Server: "s1"}
	upA := loadbalancer.BuildUpstream(pool, a, a.Spec.Ports[0], ref)
	upB := loadbalancer.BuildUpstream(pool, b, b.Spec.Ports[0], ref)

	r, c := newLBReconciler(t, a, b, pool, tokenSecret(), upA, upB)

	reconcileUntilStable(t, r, b, 1)
	ups := listManagedUpstreams(t, c)
	if len(ups) != 1 || ups[0].Name != upA.Name {
		t.Fatalf("b's upstream must be cleaned up after losing the race: %v", ups)
	}
	if got := getService(t, c, "default", "b"); len(got.Status.LoadBalancer.Ingress) != 0 {
		t.Fatalf("b must be pending: %+v", got.Status.LoadBalancer.Ingress)
	}

	reconcileUntilStable(t, r, a, 1)
	if got := getService(t, c, "default", "a"); got.Annotations[loadbalancer.AnnotationAllocated] != "p/s1" {
		t.Fatalf("a must keep its binding: %q", got.Annotations[loadbalancer.AnnotationAllocated])
	}
	ups = listManagedUpstreams(t, c)
	found := false
	for _, u := range ups {
		if u.Name == upA.Name {
			found = true
		}
	}
	if !found {
		t.Fatalf("a's upstream must survive: %v", ups)
	}

	// Fix round 2, item 4: b must not stay wedged on the binding it just lost — losing must also
	// clear its allocated-server annotation so the next reconcile falls through to Allocate and
	// picks the other available server (s2) instead of retrying the same lost server forever.
	reconcileUntilStable(t, r, b, 2)
	if got := getService(t, c, "default", "b"); got.Annotations[loadbalancer.AnnotationAllocated] != "p/s2" {
		t.Fatalf("b must re-bind to the other server after losing the race, got %q", got.Annotations[loadbalancer.AnnotationAllocated])
	}
}

// TestServiceReconcile_PoolDeletedCleansUp covers the spec's "Binding and stickiness" rule: when
// the bound pool is gone and no re-selection is possible, generated objects must be deleted even
// though the Service stays Pending with its allocated-server annotation intact (so a returning
// pool/server can re-bind).
func TestServiceReconcile_PoolDeletedCleansUp(t *testing.T) {
	svc := lbService("default", "web", 443)
	pool := lbPool("p", frpv1alpha1.ClientModePerService, "s1")
	r, c := newLBReconciler(t, svc, pool, tokenSecret())
	reconcileUntilStable(t, r, svc, 3)
	if got := getService(t, c, "default", "web"); got.Annotations[loadbalancer.AnnotationAllocated] != "p/s1" {
		t.Fatalf("precondition: %q", got.Annotations[loadbalancer.AnnotationAllocated])
	}

	if err := c.Delete(context.TODO(), pool); err != nil {
		t.Fatal(err)
	}
	reconcileUntilStable(t, r, svc, 1)

	got := getService(t, c, "default", "web")
	if got.Annotations[loadbalancer.AnnotationAllocated] != "p/s1" {
		t.Fatalf("annotation must be kept per spec: %q", got.Annotations[loadbalancer.AnnotationAllocated])
	}
	if len(got.Status.LoadBalancer.Ingress) != 0 {
		t.Fatalf("ingress must be cleared: %+v", got.Status.LoadBalancer.Ingress)
	}
	if n := len(listManagedUpstreams(t, c)); n != 0 {
		t.Fatalf("upstreams not cleaned: %d", n)
	}
	clients := &frpv1alpha1.ClientList{}
	if err := c.List(context.TODO(), clients, ctrlclient.InNamespace(opNS)); err != nil {
		t.Fatal(err)
	}
	if len(clients.Items) != 0 {
		t.Fatalf("clients not cleaned: %+v", clients.Items)
	}
}

// TestServiceReconcile_SCTPRejected covers I4: adding an unsupported-protocol port to an already
// bound Service must not disturb the existing binding, and must not create an upstream for the
// rejected port.
func TestServiceReconcile_SCTPRejected(t *testing.T) {
	svc := lbService("default", "web", 443)
	r, c := newLBReconciler(t, svc, lbPool("p", frpv1alpha1.ClientModePerService, "s1"), tokenSecret())
	reconcileUntilStable(t, r, svc, 3)

	cur := getService(t, c, "default", "web")
	cur.Spec.Ports = append(cur.Spec.Ports, corev1.ServicePort{Name: "sctp", Port: 9999, Protocol: corev1.ProtocolSCTP})
	if err := c.Update(context.TODO(), cur); err != nil {
		t.Fatal(err)
	}
	reconcileUntilStable(t, r, svc, 1)

	got := getService(t, c, "default", "web")
	if got.Annotations[loadbalancer.AnnotationAllocated] != "p/s1" {
		t.Fatalf("existing binding must be untouched: %q", got.Annotations[loadbalancer.AnnotationAllocated])
	}
	ups := listManagedUpstreams(t, c)
	if len(ups) != 1 {
		t.Fatalf("no upstream must be created for the rejected SCTP port: %v", ups)
	}
	for _, u := range ups {
		if u.Labels[loadbalancer.LabelPort] == "9999" {
			t.Fatalf("sctp upstream must not exist: %s", u.Name)
		}
	}
}

// TestServiceReconcile_TCPAndUDPSamePort covers M9: a TCP and a UDP port with the same number
// must not collide on the generated Upstream name.
func TestServiceReconcile_TCPAndUDPSamePort(t *testing.T) {
	svc := lbService("default", "dns")
	svc.Spec.Ports = []corev1.ServicePort{
		{Name: "tcp", Port: 53, Protocol: corev1.ProtocolTCP},
		{Name: "udp", Port: 53, Protocol: corev1.ProtocolUDP},
	}
	r, c := newLBReconciler(t, svc, lbPool("p", frpv1alpha1.ClientModePerService, "s1"), tokenSecret())
	reconcileUntilStable(t, r, svc, 3)

	ups := listManagedUpstreams(t, c)
	if len(ups) != 2 {
		t.Fatalf("want 2 upstreams, got %d: %v", len(ups), ups)
	}
	names := map[string]bool{}
	for _, u := range ups {
		names[u.Name] = true
	}
	if !names["lb-default-dns-s1-53"] || !names["lb-default-dns-s1-53-udp"] {
		t.Fatalf("expected distinct tcp/udp upstream names, got %v", names)
	}
}

// TestServiceReconcile_PoolReadyTransitions covers C2: the pool's Ready condition must actually
// transition (True -> False), not get stuck on its first written value forever.
func TestServiceReconcile_PoolReadyTransitions(t *testing.T) {
	svc := lbService("default", "web", 443)
	pool := lbPool("p", frpv1alpha1.ClientModePerService, "s1")
	secret := tokenSecret()
	r, c := newLBReconciler(t, svc, pool, secret)
	reconcileUntilStable(t, r, svc, 3)

	got := &frpv1alpha1.ServerPool{}
	if err := c.Get(context.TODO(), types.NamespacedName{Namespace: opNS, Name: "p"}, got); err != nil {
		t.Fatal(err)
	}
	if !poolConditionIs(got, metav1.ConditionTrue) {
		t.Fatalf("pool must start Ready: %+v", got.Status.Conditions)
	}

	if err := c.Delete(context.TODO(), secret); err != nil {
		t.Fatal(err)
	}
	reconcileUntilStable(t, r, svc, 1)

	got = &frpv1alpha1.ServerPool{}
	if err := c.Get(context.TODO(), types.NamespacedName{Namespace: opNS, Name: "p"}, got); err != nil {
		t.Fatal(err)
	}
	if !poolConditionIs(got, metav1.ConditionFalse) {
		t.Fatalf("pool must become NotReady after its token secret is deleted: %+v", got.Status.Conditions)
	}
}

// TestServiceReconcile_PoolReadyRejectsBadServerName covers item 2 of the final-review fix wave:
// poolReady must independently re-validate server names against the same pattern/length the CRD
// enforces (`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`, max 40 chars), since a pool applied against stale
// CRDs (or restored from a backup predating the validation) can bypass admission entirely.
func TestServiceReconcile_PoolReadyRejectsBadServerName(t *testing.T) {
	svc := lbService("default", "web", 443)
	pool := lbPool("p", frpv1alpha1.ClientModePerService, "SG_01")
	r, c := newLBReconciler(t, svc, pool, tokenSecret())
	reconcileUntilStable(t, r, svc, 2)

	got := &frpv1alpha1.ServerPool{}
	if err := c.Get(context.TODO(), types.NamespacedName{Namespace: opNS, Name: "p"}, got); err != nil {
		t.Fatal(err)
	}
	if !poolConditionIs(got, metav1.ConditionFalse) {
		t.Fatalf("pool with server name %q must be NotReady: %+v", "SG_01", got.Status.Conditions)
	}
}

func poolConditionIs(p *frpv1alpha1.ServerPool, want metav1.ConditionStatus) bool {
	for _, c := range p.Status.Conditions {
		if c.Type == "Ready" {
			return c.Status == want
		}
	}
	return false
}

// TestServiceReconcile_EventsNotRepeated covers I6: a recurring Warning (here, ClientNotReady on
// a Service whose Client never becomes Ready) must be reported once, not on every reconcile.
func TestServiceReconcile_EventsNotRepeated(t *testing.T) {
	svc := lbService("default", "web", 443)
	r, c := newLBReconciler(t, svc, lbPool("p", frpv1alpha1.ClientModePerService, "s1"), tokenSecret())
	reconcileUntilStable(t, r, svc, 3)
	_ = c

	rec := r.Recorder.(*record.FakeRecorder)
	close(rec.Events)
	count := 0
	for e := range rec.Events {
		if strings.Contains(e, EventReasonClientNotReady) {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("ClientNotReady must be deduplicated across reconciles, got %d", count)
	}
}

// TestServiceReconcile_EventsNotRepeated_MultiPort covers fix-round-2 item 1: warnOnce must
// track a *set* of already-emitted messages per Service, not a single last value — otherwise a
// Service with two ports both in "start error" alternates between two distinct ProxyStartError
// messages and re-emits both of them on every reconcile forever (each message looking "new"
// relative to whichever message happened to be recorded last).
func TestServiceReconcile_EventsNotRepeated_MultiPort(t *testing.T) {
	svc := lbService("default", "web", 80, 443)
	r, c := newLBReconciler(t, svc, lbPool("p", frpv1alpha1.ClientModePerService, "s1"), tokenSecret())
	reconcileUntilStable(t, r, svc, 3)
	markClientReady(t, c, "lb-default-web-s1")

	r.StatusFunc = func(models.Config) ([]handler.ProxyStatus, error) {
		return []handler.ProxyStatus{
			{Name: "lb-default-web-s1-80", Type: "tcp", Status: "start error", Err: "port already used"},
			{Name: "lb-default-web-s1-443", Type: "tcp", Status: "start error", Err: "port already used"},
		}, nil
	}
	reconcileUntilStable(t, r, svc, 3)

	rec := r.Recorder.(*record.FakeRecorder)
	close(rec.Events)
	count := 0
	for e := range rec.Events {
		if strings.Contains(e, EventReasonProxyStartError) {
			count++
		}
	}
	if count != 2 {
		t.Fatalf("want exactly 2 distinct ProxyStartError events (one per port, each deduplicated across reconciles), got %d", count)
	}
}

// TestServiceReconcile_PartialBindingKeepsDedupArmed covers fix-round-2 item 2: clearEventDedup
// must only fire once every bound ref has a live ingress entry (len(ingress) == len(refs)), not
// merely "some entry exists" — otherwise a multi-server pinned Service that is only partially
// ready re-arms the same ClientNotReady Warning for its still-unready server on every reconcile.
func TestServiceReconcile_PartialBindingKeepsDedupArmed(t *testing.T) {
	svc := lbService("default", "web", 443)
	svc.Annotations = map[string]string{loadbalancer.AnnotationServer: "s1,s2"}
	pool := lbPool("p", frpv1alpha1.ClientModePerService, "s1", "s2")
	r, c := newLBReconciler(t, svc, pool, tokenSecret())
	reconcileUntilStable(t, r, svc, 3)

	got := getService(t, c, "default", "web")
	if got.Annotations[loadbalancer.AnnotationAllocated] != "p/s1,p/s2" {
		t.Fatalf("must bind both pinned servers: %q", got.Annotations[loadbalancer.AnnotationAllocated])
	}

	// Start counting from a clean slate: s1 becomes Ready+running, s2 never does, so this
	// Service can never reach len(ingress) == len(refs) for the rest of the test.
	markClientReady(t, c, "lb-default-web-s1")
	r.StatusFunc = func(models.Config) ([]handler.ProxyStatus, error) {
		return []handler.ProxyStatus{{Name: "lb-default-web-s1-443", Status: "running"}}, nil
	}
	r.lastEvent = nil
	r.Recorder = record.NewFakeRecorder(200)

	reconcileUntilStable(t, r, svc, 3)

	rec := r.Recorder.(*record.FakeRecorder)
	close(rec.Events)
	count := 0
	for e := range rec.Events {
		if strings.Contains(e, EventReasonClientNotReady) {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("ClientNotReady for the still-unready server must stay deduplicated across a partial binding, got %d", count)
	}
}

// TestServiceReconcile_ProxyStatusFallbackKeepsResolvedAdminCreds covers fix-round-2 item 3:
// proxyStatus's fallback on a models.NewConfig error must keep whatever NewConfig already
// resolved (including AdminPort/AdminUsername/AdminPassword sourced from the Client's own
// adminServer Secrets, which are read before the token Secret lookup that fails) instead of
// discarding it for the package defaults.
func TestServiceReconcile_ProxyStatusFallbackKeepsResolvedAdminCreds(t *testing.T) {
	svc := lbService("default", "web", 443)
	pool := lbPool("p", frpv1alpha1.ClientModePerService, "s1")
	adminSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "admin-creds", Namespace: opNS},
		Data:       map[string][]byte{"username": []byte("custom-admin"), "password": []byte("custom-pass")},
	}
	pool.Spec.Servers[0].AdminServer = &frpv1alpha1.ClientSpec_Server_AdminServer{
		Port:     7500,
		Username: &frpv1alpha1.ClientSpec_Server_AdminServer_Username{Secret: frpv1alpha1.Secret{Name: "admin-creds", Key: "username"}},
		Password: &frpv1alpha1.ClientSpec_Server_AdminServer_Password{Secret: frpv1alpha1.Secret{Name: "admin-creds", Key: "password"}},
	}
	// Deliberately do not create the "tok" token Secret referenced by lbPool: models.NewConfig
	// will fail on it, but proxyStatus must still use the adminServer-resolved
	// username/password/port it already read before that failure.
	r, c := newLBReconciler(t, svc, pool, adminSecret)
	reconcileUntilStable(t, r, svc, 3)
	markClientReady(t, c, "lb-default-web-s1")

	var captured models.Config
	r.StatusFunc = func(cfg models.Config) ([]handler.ProxyStatus, error) {
		captured = cfg
		return []handler.ProxyStatus{{Name: "lb-default-web-s1-443", Status: "running"}}, nil
	}
	reconcileUntilStable(t, r, svc, 1)

	if captured.Common.AdminUsername != "custom-admin" {
		t.Fatalf("AdminUsername must come from the resolved adminServer secret, not the package default: %q", captured.Common.AdminUsername)
	}
	if captured.Common.AdminPassword != "custom-pass" {
		t.Fatalf("AdminPassword must come from the resolved adminServer secret, not the package default: %q", captured.Common.AdminPassword)
	}
	if captured.Common.AdminPort != 7500 {
		t.Fatalf("AdminPort must come from the resolved adminServer.Port, not the package default: %d", captured.Common.AdminPort)
	}
}

// TestWarnOnce_ReemitsAfterInterval covers item 3 of the final-review fix wave: a Warning
// suppressed by warnOnce must be re-emitted once eventRepeatInterval has passed since it was last
// recorded, instead of being silenced forever. The reconciler's clock is overridden via `now` so
// the test can advance time without sleeping.
func TestWarnOnce_ReemitsAfterInterval(t *testing.T) {
	svc := lbService("default", "web", 443)
	r, _ := newLBReconciler(t, svc)
	rec := record.NewFakeRecorder(200)
	r.Recorder = rec

	cur := time.Now()
	r.now = func() time.Time { return cur }

	r.warnOnce(svc, EventReasonClientNotReady, "still not ready")
	r.warnOnce(svc, EventReasonClientNotReady, "still not ready")
	r.warnOnce(svc, EventReasonClientNotReady, "still not ready")

	// Not yet past eventRepeatInterval: still only one event.
	cur = cur.Add(eventRepeatInterval - time.Second)
	r.warnOnce(svc, EventReasonClientNotReady, "still not ready")

	// Past eventRepeatInterval: must re-emit.
	cur = cur.Add(2 * time.Second)
	r.warnOnce(svc, EventReasonClientNotReady, "still not ready")

	close(rec.Events)
	count := 0
	for e := range rec.Events {
		if strings.Contains(e, EventReasonClientNotReady) {
			count++
		}
	}
	if count != 2 {
		t.Fatalf("want 1 initial event + 1 re-emitted after eventRepeatInterval, got %d", count)
	}
}

func TestClaimedServiceRequests(t *testing.T) {
	claimed := lbService("default", "web", 443)
	other := lbService("default", "other", 80)
	other.Spec.LoadBalancerClass = nil
	r, _ := newLBReconciler(t, claimed, other)
	reqs := r.ClaimedServiceRequests(context.TODO())
	if len(reqs) != 1 || reqs[0].Name != "web" || reqs[0].Namespace != "default" {
		t.Fatalf("got %v", reqs)
	}
}

func TestPredicate_OnlyClaimedOrFinalized(t *testing.T) {
	claimed := lbService("default", "web", 443)
	plain := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "x", Namespace: "default"}}
	finalized := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "y", Namespace: "default", Finalizers: []string{loadbalancer.Finalizer}}}
	if !serviceOfInterest(claimed) || serviceOfInterest(plain) || !serviceOfInterest(finalized) {
		t.Fatal("predicate mismatch")
	}
}
func TestClientStatusChanged(t *testing.T) {
	baseConditions := []metav1.Condition{
		{Type: "Ready", Status: metav1.ConditionFalse, Reason: "NotReady", Message: "not ready", LastTransitionTime: metav1.NewTime(time.Now())},
	}

	t.Run("same conditions, LastTransitionTime differs", func(t *testing.T) {
		oldC := &frpv1alpha1.Client{Status: frpv1alpha1.ClientStatus{Conditions: baseConditions}}
		newConditions := append([]metav1.Condition(nil), baseConditions...)
		newConditions[0].LastTransitionTime = metav1.NewTime(time.Now().Add(time.Hour))
		newC := &frpv1alpha1.Client{Status: frpv1alpha1.ClientStatus{Conditions: newConditions}}
		if clientStatusChanged(event.UpdateEvent{ObjectOld: oldC, ObjectNew: newC}) {
			t.Fatal("expected no change when only LastTransitionTime differs")
		}
	})

	t.Run("Ready flips False to True", func(t *testing.T) {
		oldC := &frpv1alpha1.Client{Status: frpv1alpha1.ClientStatus{Conditions: baseConditions}}
		newConditions := []metav1.Condition{
			{Type: "Ready", Status: metav1.ConditionTrue, Reason: "Ready", Message: "ready", LastTransitionTime: metav1.NewTime(time.Now())},
		}
		newC := &frpv1alpha1.Client{Status: frpv1alpha1.ClientStatus{Conditions: newConditions}}
		if !clientStatusChanged(event.UpdateEvent{ObjectOld: oldC, ObjectNew: newC}) {
			t.Fatal("expected change when Ready flips False to True")
		}
	})

	t.Run("only Status.Message differs", func(t *testing.T) {
		oldC := &frpv1alpha1.Client{Status: frpv1alpha1.ClientStatus{Conditions: baseConditions, Message: "old message"}}
		newC := &frpv1alpha1.Client{Status: frpv1alpha1.ClientStatus{Conditions: baseConditions, Message: "new message"}}
		if !clientStatusChanged(event.UpdateEvent{ObjectOld: oldC, ObjectNew: newC}) {
			t.Fatal("expected change when only Status.Message differs")
		}
	})
}
