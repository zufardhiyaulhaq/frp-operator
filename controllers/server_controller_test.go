package controllers

import (
	"context"
	"reflect"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	frpv1alpha1 "github.com/zufardhiyaulhaq/frp-operator/api/v1alpha1"
	"github.com/zufardhiyaulhaq/frp-operator/pkg/frps"
	"github.com/zufardhiyaulhaq/frp-operator/pkg/frps/frpstest"
)

const serverNS = "frp"

func ptrOf[T any](v T) *T { return &v }

type serverFixture struct {
	t   *testing.T
	r   *ServerReconciler
	c   client.Client
	rec *record.FakeRecorder
}

func newServerFixture(t *testing.T, objs ...client.Object) *serverFixture {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := frpv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).WithStatusSubresource(&frpv1alpha1.Server{}, &corev1.Pod{}, &corev1.Service{}).Build()
	rec := record.NewFakeRecorder(100)
	return &serverFixture{t: t, c: c, rec: rec, r: &ServerReconciler{Client: c, Scheme: scheme, Recorder: rec, MaxPorts: 50}}
}

func (f *serverFixture) reconcile() error {
	_, err := f.r.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Namespace: serverNS, Name: "public"}})
	return err
}

func (f *serverFixture) mustReconcile() {
	f.t.Helper()
	if err := f.reconcile(); err != nil {
		f.t.Fatalf("reconcile: %v", err)
	}
}

func (f *serverFixture) get(name string, obj client.Object) {
	f.t.Helper()
	if err := f.c.Get(context.Background(), types.NamespacedName{Namespace: serverNS, Name: name}, obj); err != nil {
		f.t.Fatalf("get %s: %v", name, err)
	}
}

func (f *serverFixture) exists(name string, obj client.Object) bool {
	return f.c.Get(context.Background(), types.NamespacedName{Namespace: serverNS, Name: name}, obj) == nil
}

func (f *serverFixture) update(obj client.Object) {
	f.t.Helper()
	if err := f.c.Update(context.Background(), obj); err != nil {
		f.t.Fatal(err)
	}
}

func (f *serverFixture) server() *frpv1alpha1.Server {
	s := &frpv1alpha1.Server{}
	f.get("public", s)
	return s
}

// updateSpec changes the Server's spec and bumps its generation, as the API server would.
func (f *serverFixture) updateSpec(mutate func(*frpv1alpha1.ServerSpec)) {
	f.t.Helper()
	s := f.server()
	mutate(&s.Spec)
	s.Generation++
	f.update(s)
}

func (f *serverFixture) pod() *corev1.Pod {
	p := &corev1.Pod{}
	f.get("public-frps", p)
	return p
}

func (f *serverFixture) podExists() bool { return f.exists("public-frps", &corev1.Pod{}) }

func (f *serverFixture) secretData(name, key string) string {
	s := &corev1.Secret{}
	f.get(name, s)
	return string(s.Data[key])
}

func (f *serverFixture) markPodReady() {
	p := f.pod()
	p.Status = corev1.PodStatus{Phase: corev1.PodRunning, Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue}}}
	f.updateStatus(p)
}

// updateStatus writes obj's status (Pods and Services have a status subresource, like the API server).
func (f *serverFixture) updateStatus(obj client.Object) {
	f.t.Helper()
	if err := f.c.Status().Update(context.Background(), obj); err != nil {
		f.t.Fatal(err)
	}
}

// events drains the recorded events ("<type> <reason> <message>").
func (f *serverFixture) events() []string {
	var out []string
	for {
		select {
		case e := <-f.rec.Events:
			out = append(out, e)
		default:
			return out
		}
	}
}

func hasEvent(events []string, eventType, reason string) bool {
	for _, e := range events {
		if strings.HasPrefix(e, eventType+" "+reason+" ") {
			return true
		}
	}
	return false
}

func (f *serverFixture) expectCondition(condType string, status metav1.ConditionStatus, reason string) *metav1.Condition {
	f.t.Helper()
	c := meta.FindStatusCondition(f.server().Status.Conditions, condType)
	if c == nil || c.Status != status || c.Reason != reason {
		f.t.Fatalf("%s = %+v, want %s/%s", condType, c, status, reason)
	}
	return c
}

func testServer(mutate func(*frpv1alpha1.ServerSpec)) *frpv1alpha1.Server {
	s := &frpv1alpha1.Server{
		ObjectMeta: metav1.ObjectMeta{Name: "public", Namespace: serverNS, Generation: 1},
		Spec: frpv1alpha1.ServerSpec{
			Bind:       frpv1alpha1.ServerSpec_Bind{Port: 7000},
			AllowPorts: &frpv1alpha1.ServerSpec_AllowPorts{TCP: []frpv1alpha1.PortRange{"8000"}},
			Authentication: &frpv1alpha1.ServerSpec_Authentication{
				Token: &frpv1alpha1.ClientSpec_Server_Authentication_Token{Secret: frpv1alpha1.Secret{Name: "token", Key: "token"}},
			},
			Service: &frpv1alpha1.ServerSpec_Service{Type: "ClusterIP"},
		},
	}
	if mutate != nil {
		mutate(&s.Spec)
	}
	return s
}

func serverTokenSecret(value string) *corev1.Secret {
	return &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "token", Namespace: serverNS}, Data: map[string][]byte{"token": []byte(value)}}
}

func certSecret(t *testing.T, name string, dnsNames ...string) *corev1.Secret {
	t.Helper()
	crt, key, err := frpstest.SelfSigned(dnsNames...)
	if err != nil {
		t.Fatal(err)
	}
	return &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: serverNS},
		Data: map[string][]byte{"tls.crt": crt, "tls.key": key, "ca.crt": crt}}
}

func TestServerReconcile_CreatesFrpsObjects(t *testing.T) {
	f := newServerFixture(t, testServer(nil), serverTokenSecret("s3cret"))
	f.mustReconcile()

	if toml := f.secretData("public-frps-config", "frps.toml"); !strings.Contains(toml, `auth.token = "s3cret"`) || !strings.Contains(toml, "{ single = 8000 }") {
		t.Errorf("frps.toml =\n%s", toml)
	}
	if got := f.secretData("public-connection", "token"); got != "s3cret" {
		t.Errorf("connection token = %q", got)
	}
	svc := &corev1.Service{}
	f.get("public-frps", svc)
	if svc.Spec.Type != corev1.ServiceTypeClusterIP || len(svc.Spec.Ports) != 2 || len(svc.OwnerReferences) != 1 {
		t.Errorf("service = %+v", svc)
	}
	pod := f.pod()
	if pod.Annotations[frps.ConfigHashAnnotation] == "" || len(pod.OwnerReferences) != 1 || pod.OwnerReferences[0].Name != "public" {
		t.Errorf("pod = %+v", pod.ObjectMeta)
	}
	s := f.server()
	if s.Status.Address != "public-frps.frp.svc" || s.Status.ConnectionSecret != "public-connection" || s.Status.ObservedGeneration != 1 {
		t.Errorf("status = %+v", s.Status)
	}
	if s.Status.ExposedPorts == nil || !reflect.DeepEqual(s.Status.ExposedPorts.TCP, []int32{8000}) {
		t.Errorf("exposedPorts = %+v", s.Status.ExposedPorts)
	}
	f.expectCondition("ConfigSynced", metav1.ConditionTrue, frps.ReasonConfigApplied)
	f.expectCondition("Ready", metav1.ConditionFalse, frps.ReasonPodStarting)
}

func TestServerReconcile_GeneratedTokenIsStable(t *testing.T) {
	f := newServerFixture(t, testServer(func(s *frpv1alpha1.ServerSpec) { s.Authentication = nil }))
	f.mustReconcile()
	first := f.secretData("public-connection", "token")
	if len(first) != 64 {
		t.Fatalf("generated token = %q", first)
	}
	hash := f.pod().Annotations[frps.ConfigHashAnnotation]

	f.mustReconcile()
	if again := f.secretData("public-connection", "token"); again != first {
		t.Errorf("token changed between reconciles: %q -> %q", first, again)
	}
	if got := f.pod().Annotations[frps.ConfigHashAnnotation]; got != hash {
		t.Error("frps was restarted although nothing changed")
	}
	if hasEvent(f.events(), "Warning", frps.ReasonTokenRotated) {
		t.Error("TokenRotated for a stable token")
	}
}

func TestServerReconcile_DeletedConnectionSecretRotatesToken(t *testing.T) {
	f := newServerFixture(t, testServer(func(s *frpv1alpha1.ServerSpec) { s.Authentication = nil }))
	f.mustReconcile()
	first := f.secretData("public-connection", "token")
	if err := f.c.Delete(context.Background(), &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "public-connection", Namespace: serverNS}}); err != nil {
		t.Fatal(err)
	}
	f.events()

	f.mustReconcile()
	if second := f.secretData("public-connection", "token"); second == first {
		t.Error("the token was not rotated")
	}
	if !hasEvent(f.events(), "Warning", frps.ReasonTokenRotated) {
		t.Error("no TokenRotated event")
	}
	if f.podExists() {
		t.Error("frps kept running with the old token")
	}
}

func TestServerReconcile_InvalidConfigKeepsRunningPod(t *testing.T) {
	f := newServerFixture(t, testServer(nil), serverTokenSecret("s3cret"))
	f.mustReconcile()
	hash := f.pod().Annotations[frps.ConfigHashAnnotation]
	token := &corev1.Secret{}
	f.get("token", token)
	token.Data = map[string][]byte{"other": []byte("x")}
	f.update(token)

	if err := f.reconcile(); err == nil {
		t.Fatal("reconcile succeeded with a missing token key")
	}
	if c := f.expectCondition("ConfigSynced", metav1.ConditionFalse, frps.ReasonInvalidConfig); !strings.Contains(c.Message, `key "token" not found`) {
		t.Errorf("message = %q", c.Message)
	}
	if !hasEvent(f.events(), "Warning", frps.ReasonInvalidConfig) {
		t.Error("no InvalidConfig event")
	}
	if got := f.pod().Annotations[frps.ConfigHashAnnotation]; got != hash {
		t.Error("the running pod was touched")
	}
}

func TestServerReconcile_AutomaticRestartOnChange(t *testing.T) {
	f := newServerFixture(t, testServer(nil), serverTokenSecret("s3cret"))
	f.mustReconcile()
	f.updateSpec(func(s *frpv1alpha1.ServerSpec) { s.AllowPorts.TCP = append(s.AllowPorts.TCP, "8001") })

	f.mustReconcile()
	if f.podExists() {
		t.Fatal("the pod was not deleted after a config change")
	}
	f.expectCondition("ConfigSynced", metav1.ConditionFalse, frps.ReasonRestarting)

	f.mustReconcile()
	if toml := f.secretData("public-frps-config", "frps.toml"); !strings.Contains(toml, "{ start = 8000, end = 8001 }") {
		t.Errorf("frps.toml =\n%s", toml)
	}
	if !f.podExists() {
		t.Fatal("the pod was not recreated")
	}
	f.expectCondition("ConfigSynced", metav1.ConditionTrue, frps.ReasonConfigApplied)
}

func TestServerReconcile_ManualRestartWaitsForPodDeletion(t *testing.T) {
	f := newServerFixture(t, testServer(func(s *frpv1alpha1.ServerSpec) { s.RestartPolicy = frpv1alpha1.RestartPolicyManual }), serverTokenSecret("s3cret"))
	f.mustReconcile()
	oldConfig := f.secretData("public-frps-config", "frps.toml")
	f.events()
	f.updateSpec(func(s *frpv1alpha1.ServerSpec) { s.AllowPorts.TCP = []frpv1alpha1.PortRange{"9000"} })

	f.mustReconcile()
	if !f.podExists() {
		t.Fatal("Manual: the pod was deleted")
	}
	if got := f.secretData("public-frps-config", "frps.toml"); got != oldConfig {
		t.Error("Manual: the mounted config changed while the restart is pending")
	}
	f.expectCondition("ConfigSynced", metav1.ConditionFalse, frps.ReasonRestartPending)
	if !hasEvent(f.events(), "Normal", frps.ReasonRestartPending) {
		t.Error("no RestartPending event")
	}

	if err := f.c.Delete(context.Background(), f.pod()); err != nil {
		t.Fatal(err)
	}
	f.mustReconcile()
	if toml := f.secretData("public-frps-config", "frps.toml"); !strings.Contains(toml, "{ single = 9000 }") {
		t.Errorf("frps.toml =\n%s", toml)
	}
	f.expectCondition("ConfigSynced", metav1.ConditionTrue, frps.ReasonConfigApplied)
}

func TestServerReconcile_CertificateRotationRestartsPod(t *testing.T) {
	f := newServerFixture(t, testServer(func(s *frpv1alpha1.ServerSpec) {
		s.Bind.TLS = &frpv1alpha1.ServerSpec_Bind_TLS{Certificate: &frpv1alpha1.SecretNameRef{Secret: frpv1alpha1.SecretName{Name: "frps-cert"}}}
	}), serverTokenSecret("s3cret"), certSecret(t, "frps-cert", "frps.example.com"))
	f.mustReconcile()
	if tls := f.server().Status.TLS; tls == nil || !reflect.DeepEqual(tls.ServerNames, []string{"frps.example.com"}) {
		t.Fatalf("status.tls = %+v", tls)
	}
	if f.secretData("public-connection", "ca.crt") == "" {
		t.Error("the connection Secret has no ca.crt")
	}

	cert := &corev1.Secret{}
	f.get("frps-cert", cert)
	cert.Data = certSecret(t, "frps-cert", "frps.example.com").Data
	f.update(cert)
	f.mustReconcile()
	if f.podExists() {
		t.Fatal("frps kept serving the old certificate")
	}
}

func TestServerReconcile_ServiceTypeChangeClearsLoadBalancerFields(t *testing.T) {
	f := newServerFixture(t, testServer(func(s *frpv1alpha1.ServerSpec) {
		s.Service = &frpv1alpha1.ServerSpec_Service{Type: "LoadBalancer", ExternalTrafficPolicy: "Local", LoadBalancerClass: "example.com/lb"}
	}), serverTokenSecret("s3cret"))
	f.mustReconcile()

	// What the API server adds to a LoadBalancer Service.
	svc := &corev1.Service{}
	f.get("public-frps", svc)
	for i := range svc.Spec.Ports {
		svc.Spec.Ports[i].NodePort = int32(30000 + i)
	}
	svc.Spec.AllocateLoadBalancerNodePorts = ptrOf(true)
	svc.Spec.HealthCheckNodePort = 31000
	f.update(svc)

	f.mustReconcile() // LoadBalancer → LoadBalancer keeps the node ports
	f.get("public-frps", svc)
	if svc.Spec.Ports[0].NodePort != 30000 {
		t.Errorf("node port reallocated: %+v", svc.Spec.Ports)
	}

	f.updateSpec(func(s *frpv1alpha1.ServerSpec) { s.Service = &frpv1alpha1.ServerSpec_Service{Type: "ClusterIP"} })
	f.mustReconcile()
	f.get("public-frps", svc)
	if svc.Spec.Type != corev1.ServiceTypeClusterIP || svc.Spec.ExternalTrafficPolicy != "" || svc.Spec.AllocateLoadBalancerNodePorts != nil ||
		svc.Spec.HealthCheckNodePort != 0 || svc.Spec.LoadBalancerClass != nil {
		t.Errorf("spec = %+v", svc.Spec)
	}
	for _, p := range svc.Spec.Ports {
		if p.NodePort != 0 {
			t.Errorf("ClusterIP port kept node port %d", p.NodePort)
		}
	}
}

func TestServerReconcile_ReadyNeedsPodAndAddress(t *testing.T) {
	f := newServerFixture(t, testServer(func(s *frpv1alpha1.ServerSpec) { s.Service = &frpv1alpha1.ServerSpec_Service{Type: "LoadBalancer"} }), serverTokenSecret("s3cret"))
	f.mustReconcile()
	f.expectCondition("Ready", metav1.ConditionFalse, frps.ReasonPodStarting)

	f.markPodReady()
	f.mustReconcile()
	f.expectCondition("Ready", metav1.ConditionFalse, frps.ReasonAddressPending)

	svc := &corev1.Service{}
	f.get("public-frps", svc)
	svc.Status.LoadBalancer.Ingress = []corev1.LoadBalancerIngress{{IP: "203.0.113.10"}}
	f.updateStatus(svc)
	f.mustReconcile()
	f.expectCondition("Ready", metav1.ConditionTrue, frps.ReasonReady)
	if got := f.server().Status.Address; got != "203.0.113.10" {
		t.Errorf("address = %q", got)
	}
}

func TestServerReconcile_LoadBalancerFailed(t *testing.T) {
	f := newServerFixture(t, testServer(func(s *frpv1alpha1.ServerSpec) { s.Service = &frpv1alpha1.ServerSpec_Service{Type: "LoadBalancer"} }), serverTokenSecret("s3cret"))
	f.r.LoadBalancerError = func(_ context.Context, namespace, service string) (string, error) {
		if namespace != serverNS || service != "public-frps" {
			t.Errorf("LoadBalancerError(%s, %s)", namespace, service)
		}
		return "mixed protocol is not supported for LoadBalancer", nil
	}
	f.mustReconcile()
	f.markPodReady()
	f.mustReconcile()
	if c := f.expectCondition("Ready", metav1.ConditionFalse, frps.ReasonLoadBalancerFailed); !strings.Contains(c.Message, "mixed protocol") {
		t.Errorf("message = %q", c.Message)
	}
}

func TestServerReconcile_SpecWarningsOncePerGeneration(t *testing.T) {
	f := newServerFixture(t, testServer(func(s *frpv1alpha1.ServerSpec) {
		s.Dashboard = &frpv1alpha1.ServerSpec_Dashboard{Enabled: true, Service: &frpv1alpha1.ServerSpec_Dashboard_Service{Type: "LoadBalancer"}}
	}), serverTokenSecret("s3cret"))
	f.mustReconcile()
	events := f.events()
	if !hasEvent(events, "Warning", frps.ReasonTunnelTLSUnverified) || !hasEvent(events, "Warning", frps.ReasonDashboardInsecure) {
		t.Errorf("events = %v", events)
	}
	f.mustReconcile()
	if events := f.events(); hasEvent(events, "Warning", frps.ReasonTunnelTLSUnverified) {
		t.Errorf("warned again for the same generation: %v", events)
	}
}

func TestServerReconcile_ServiceMonitorUnavailable(t *testing.T) {
	f := newServerFixture(t, testServer(func(s *frpv1alpha1.ServerSpec) {
		s.Metrics = &frpv1alpha1.ServerSpec_Metrics{Enabled: true, ServiceMonitor: true}
	}), serverTokenSecret("s3cret"))
	f.r.ServiceMonitorAvailable = func() bool { return false }
	f.mustReconcile()
	if !hasEvent(f.events(), "Warning", frps.ReasonServiceMonitorUnavailable) {
		t.Error("no ServiceMonitorUnavailable event")
	}
	f.expectCondition("ConfigSynced", metav1.ConditionTrue, frps.ReasonConfigApplied)
}

func TestServerReconcile_WebServiceFollowsDashboard(t *testing.T) {
	f := newServerFixture(t, testServer(func(s *frpv1alpha1.ServerSpec) { s.Dashboard = &frpv1alpha1.ServerSpec_Dashboard{Enabled: true} }), serverTokenSecret("s3cret"))
	f.mustReconcile()
	if !f.exists("public-frps-web", &corev1.Service{}) {
		t.Fatal("no web Service")
	}
	if user := f.secretData("public-dashboard", "username"); user != "admin" {
		t.Errorf("dashboard username = %q", user)
	}
	f.updateSpec(func(s *frpv1alpha1.ServerSpec) { s.Dashboard = nil })
	f.mustReconcile()
	if f.exists("public-frps-web", &corev1.Service{}) {
		t.Error("the web Service outlived the dashboard")
	}
}

func TestServersForSecret(t *testing.T) {
	f := newServerFixture(t, testServer(nil))
	reqs := f.r.serversForSecret(context.Background(), &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "token", Namespace: serverNS}})
	if len(reqs) != 1 || reqs[0].Name != "public" {
		t.Errorf("requests = %v", reqs)
	}
	if reqs := f.r.serversForSecret(context.Background(), &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "unrelated", Namespace: serverNS}}); len(reqs) != 0 {
		t.Errorf("requests for an unrelated Secret = %v", reqs)
	}
}

func TestFrpsImageMatchesFrpcRelease(t *testing.T) {
	_, frpcTag, _ := strings.Cut(frpcImage, ":")
	_, frpsTag, _ := strings.Cut(frps.Image, ":")
	if frpcTag != frpsTag {
		t.Errorf("frpc %s and frps %s are different frp releases", frpcImage, frps.Image)
	}
}

// An evicted frps pod (phase Failed) never restarts on its own: it must be replaced, under either
// restart policy.
func TestServerReconcile_EvictedPodIsReplaced(t *testing.T) {
	for _, policy := range []string{frpv1alpha1.RestartPolicyAutomatic, frpv1alpha1.RestartPolicyManual} {
		t.Run(policy, func(t *testing.T) {
			f := newServerFixture(t, testServer(func(s *frpv1alpha1.ServerSpec) { s.RestartPolicy = policy }), serverTokenSecret("s3cret"))
			f.mustReconcile()
			p := f.pod()
			p.Status = corev1.PodStatus{Phase: corev1.PodFailed, Reason: "Evicted", Message: "The node was low on resource: memory."}
			f.updateStatus(p)

			f.mustReconcile()
			if f.podExists() {
				t.Fatal("the evicted frps pod was kept")
			}
			f.mustReconcile()
			if !f.podExists() {
				t.Fatal("frps was not started again after the eviction")
			}
		})
	}
}

// Under restartPolicy Manual the running frps keeps its old ports until its pod is deleted, so
// the Services must keep pointing at them until then.
func TestServerReconcile_ManualKeepsServicesUntilRestart(t *testing.T) {
	f := newServerFixture(t, testServer(func(s *frpv1alpha1.ServerSpec) { s.RestartPolicy = frpv1alpha1.RestartPolicyManual }), serverTokenSecret("s3cret"))
	f.mustReconcile()
	bindPort := func() int32 {
		svc := &corev1.Service{}
		f.get("public-frps", svc)
		for _, p := range svc.Spec.Ports {
			if p.Name == "bind" {
				return p.Port
			}
		}
		return 0
	}

	f.updateSpec(func(s *frpv1alpha1.ServerSpec) {
		s.Bind.Port = 7443
		s.Dashboard = &frpv1alpha1.ServerSpec_Dashboard{Enabled: true}
	})
	f.mustReconcile()
	if got := bindPort(); got != 7000 {
		t.Errorf("bind port = %d while the restart is pending, want the running frps's 7000", got)
	}
	if f.exists("public-frps-web", &corev1.Service{}) {
		t.Error("the web Service appeared before frps runs its web server")
	}

	if err := f.c.Delete(context.Background(), f.pod()); err != nil {
		t.Fatal(err)
	}
	f.mustReconcile()
	if got := bindPort(); got != 7443 {
		t.Errorf("bind port = %d after the restart, want 7443", got)
	}
	if !f.exists("public-frps-web", &corev1.Service{}) {
		t.Error("no web Service after the restart")
	}
}

// A log-level change is a config change: Automatic restarts frps, Manual waits for the pod to be
// deleted and keeps the running config untouched.
func TestServerReconcile_LogLevelChange(t *testing.T) {
	t.Run("Automatic", func(t *testing.T) {
		f := newServerFixture(t, testServer(nil), serverTokenSecret("s3cret"))
		f.mustReconcile()
		f.updateSpec(func(s *frpv1alpha1.ServerSpec) { s.Log = &frpv1alpha1.LogSpec{Level: "debug"} })
		f.mustReconcile()
		if f.podExists() {
			t.Fatal("frps kept running at the old log level")
		}
		f.mustReconcile()
		if toml := f.secretData("public-frps-config", "frps.toml"); !strings.Contains(toml, `log.level = "debug"`) {
			t.Errorf("frps.toml =\n%s", toml)
		}
	})
	t.Run("Manual", func(t *testing.T) {
		f := newServerFixture(t, testServer(func(s *frpv1alpha1.ServerSpec) { s.RestartPolicy = frpv1alpha1.RestartPolicyManual }), serverTokenSecret("s3cret"))
		f.mustReconcile()
		before := f.secretData("public-frps-config", "frps.toml")
		f.updateSpec(func(s *frpv1alpha1.ServerSpec) { s.Log = &frpv1alpha1.LogSpec{Level: "debug"} })
		f.mustReconcile()
		if !f.podExists() {
			t.Fatal("Manual: frps was restarted")
		}
		if got := f.secretData("public-frps-config", "frps.toml"); got != before {
			t.Error("Manual: the running config changed before the pod was deleted")
		}
		f.expectCondition("ConfigSynced", metav1.ConditionFalse, frps.ReasonRestartPending)
	})
}
