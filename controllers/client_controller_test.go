package controllers

import (
	"context"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	ctrlmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"

	frpv1alpha1 "github.com/zufardhiyaulhaq/frp-operator/api/v1alpha1"
	"github.com/zufardhiyaulhaq/frp-operator/pkg/client/handler"
	"github.com/zufardhiyaulhaq/frp-operator/pkg/metrics"
)

func countFamily(t *testing.T, name string) int {
	t.Helper()
	families, err := ctrlmetrics.Registry.Gather()
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range families {
		if f.GetName() == name {
			return len(f.GetMetric())
		}
	}
	return 0
}

// gaugeValue returns the value of the first series in family whose labels are a superset
// of the given labels, and whether such a series was found.
func gaugeValue(t *testing.T, family string, labels map[string]string) (float64, bool) {
	t.Helper()
	families, err := ctrlmetrics.Registry.Gather()
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range families {
		if f.GetName() != family {
			continue
		}
		for _, m := range f.GetMetric() {
			got := map[string]string{}
			for _, l := range m.GetLabel() {
				got[l.GetName()] = l.GetValue()
			}
			match := true
			for k, v := range labels {
				if got[k] != v {
					match = false
					break
				}
			}
			if match {
				return m.GetGauge().GetValue(), true
			}
		}
	}
	return 0, false
}

func TestReconcile_DeletedClientRemovesMetrics(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := frpv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	r := &ClientReconciler{Client: fake.NewClientBuilder().WithScheme(scheme).Build(), Scheme: scheme}

	c := &frpv1alpha1.Client{}
	c.Namespace, c.Name = "ns1", "gone"
	metrics.RecordClient(c, "ns1/gone", frpcImage)
	metrics.RecordProxies("ns1", "gone", []handler.ProxyStatus{{Name: "p", Type: "tcp", Status: "running"}}, nil)
	if n := countFamily(t, "frp_proxy_info"); n != 1 {
		t.Fatalf("precondition: frp_proxy_info = %d, want 1", n)
	}

	_, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Namespace: "ns1", Name: "gone"}})
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	for _, name := range []string{"frp_client_info", "frp_client_ready", "frp_proxy_status", "frp_proxy_info", "frp_client_admin_up"} {
		if n := countFamily(t, name); n != 0 {
			t.Errorf("%s still has %d series after Client deletion", name, n)
		}
	}
}

// TestReconcile_PodNotRunningPrunesProxyMetrics covers finding 2: when the pod is not
// Running, the early-return branch must refresh frp_client_admin_up and prune the stale
// frp_proxy_* series left behind by a previous, healthy reconcile — otherwise they stay
// frozen green while the pod is actually down.
func TestReconcile_PodNotRunningPrunesProxyMetrics(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := frpv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}

	ns, name := "ns2", "flaky"
	t.Cleanup(func() { metrics.DeleteClient(ns, name) })

	c := &frpv1alpha1.Client{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
		Spec: frpv1alpha1.ClientSpec{
			Server: frpv1alpha1.ClientSpec_Server{
				Host: "example.com",
				Port: 7000,
				Authentication: frpv1alpha1.ClientSpec_Server_Authentication{
					Token: &frpv1alpha1.ClientSpec_Server_Authentication_Token{
						Secret: frpv1alpha1.Secret{Name: "frp-token", Key: "token"},
					},
				},
			},
		},
	}
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "frp-token", Namespace: ns},
		Data:       map[string][]byte{"token": []byte("shh")},
	}
	// Pre-existing pod, same image as desired (skips the image-roll branch) but not yet Running.
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: name + "-frpc", Namespace: ns},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{Name: "frpc", Image: frpcImage}},
		},
		Status: corev1.PodStatus{Phase: corev1.PodPending},
	}

	r := &ClientReconciler{
		Client:   fake.NewClientBuilder().WithScheme(scheme).WithObjects(c, secret, pod).Build(),
		Scheme:   scheme,
		Recorder: record.NewFakeRecorder(10),
	}

	// Simulate a previous, healthy reconcile that left admin_up=1 and a proxy series behind.
	metrics.RecordProxies(ns, name, []handler.ProxyStatus{{Name: "p", Type: "tcp", Status: "running"}}, nil)
	if v, ok := gaugeValue(t, "frp_client_admin_up", map[string]string{"namespace": ns, "client": name}); !ok || v != 1 {
		t.Fatalf("precondition: admin_up = %v (ok=%v), want 1", v, ok)
	}
	if n := countFamily(t, "frp_proxy_info"); n != 1 {
		t.Fatalf("precondition: frp_proxy_info = %d, want 1", n)
	}

	if _, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Namespace: ns, Name: name}}); err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	if v, ok := gaugeValue(t, "frp_client_admin_up", map[string]string{"namespace": ns, "client": name}); !ok || v != 0 {
		t.Errorf("admin_up after pod-not-running reconcile = %v (ok=%v), want 0", v, ok)
	}
	if n := countFamily(t, "frp_proxy_info"); n != 0 {
		t.Errorf("frp_proxy_info = %d after pod-not-running reconcile, want 0 (stale series must be pruned)", n)
	}
}

// An invalid spec (here: a Secret key that does not exist) must not leave the Client looking
// synced: the last good config keeps running, but ConfigSynced and frp_client_config_synced
// have to report that the current spec is not applied.
func TestReconcile_InvalidConfigMarksConfigNotSynced(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := frpv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}

	ns, name := "ns3", "broken"
	t.Cleanup(func() { metrics.DeleteClient(ns, name) })

	c := &frpv1alpha1.Client{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
		Spec: frpv1alpha1.ClientSpec{
			Server: frpv1alpha1.ClientSpec_Server{
				Host: "example.com",
				Port: 7000,
				Authentication: frpv1alpha1.ClientSpec_Server_Authentication{
					Token: &frpv1alpha1.ClientSpec_Server_Authentication_Token{
						Secret: frpv1alpha1.Secret{Name: "frp-token", Key: "no-such-key"},
					},
				},
			},
		},
		Status: frpv1alpha1.ClientStatus{
			Phase: "Running",
			Conditions: []metav1.Condition{{
				Type: "ConfigSynced", Status: metav1.ConditionTrue, Reason: "ConfigReloaded",
				Message: "Configuration synchronized", LastTransitionTime: metav1.Now(),
			}},
		},
	}
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "frp-token", Namespace: ns},
		Data:       map[string][]byte{"token": []byte("shh")},
	}
	recorder := record.NewFakeRecorder(10)
	k8s := fake.NewClientBuilder().WithScheme(scheme).WithObjects(c, secret).WithStatusSubresource(c).Build()
	r := &ClientReconciler{Client: k8s, Scheme: scheme, Recorder: recorder}

	// Simulate the previous, healthy reconcile.
	metrics.RecordClient(c, ns+"/"+name, frpcImage)
	if v, ok := gaugeValue(t, "frp_client_config_synced", map[string]string{"namespace": ns, "client": name}); !ok || v != 1 {
		t.Fatalf("precondition: config_synced = %v (ok=%v), want 1", v, ok)
	}

	_, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Namespace: ns, Name: name}})
	if err == nil {
		t.Fatal("reconcile error = nil, want the invalid config error so the request is retried")
	}

	got := &frpv1alpha1.Client{}
	if err := k8s.Get(context.Background(), types.NamespacedName{Namespace: ns, Name: name}, got); err != nil {
		t.Fatal(err)
	}
	cond := meta.FindStatusCondition(got.Status.Conditions, "ConfigSynced")
	if cond == nil || cond.Status != metav1.ConditionFalse || cond.Reason != "InvalidConfig" || !strings.Contains(cond.Message, `key "no-such-key"`) {
		t.Errorf("ConfigSynced = %+v, want False/InvalidConfig mentioning the missing key", cond)
	}
	if v, ok := gaugeValue(t, "frp_client_config_synced", map[string]string{"namespace": ns, "client": name}); !ok || v != 0 {
		t.Errorf("config_synced = %v (ok=%v), want 0", v, ok)
	}
	select {
	case event := <-recorder.Events:
		if !strings.HasPrefix(event, "Warning InvalidConfig") {
			t.Errorf("event = %q, want a Warning InvalidConfig event", event)
		}
	default:
		t.Error("no event recorded, want a Warning InvalidConfig event")
	}
}

func TestSetCondition_KeepsTransitionTimeWhenStatusUnchanged(t *testing.T) {
	r := &ClientReconciler{}
	earlier := metav1.NewTime(time.Now().Add(-time.Hour).Truncate(time.Second))
	c := &frpv1alpha1.Client{Status: frpv1alpha1.ClientStatus{Conditions: []metav1.Condition{{
		Type: "ConfigSynced", Status: metav1.ConditionFalse, Reason: "InvalidConfig", Message: "old", LastTransitionTime: earlier,
	}}}}

	r.setCondition(c, "ConfigSynced", metav1.ConditionFalse, "InvalidConfig", "new")
	cond := meta.FindStatusCondition(c.Status.Conditions, "ConfigSynced")
	if !cond.LastTransitionTime.Equal(&earlier) || cond.Message != "new" {
		t.Errorf("unchanged status: LastTransitionTime = %v, Message = %q; want %v and %q", cond.LastTransitionTime, cond.Message, earlier, "new")
	}

	r.setCondition(c, "ConfigSynced", metav1.ConditionTrue, "ConfigReloaded", "ok")
	cond = meta.FindStatusCondition(c.Status.Conditions, "ConfigSynced")
	if cond.LastTransitionTime.Equal(&earlier) {
		t.Error("status changed but LastTransitionTime was not updated")
	}
}
