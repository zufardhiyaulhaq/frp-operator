package builder

import (
	"testing"

	frpv1alpha1 "github.com/zufardhiyaulhaq/frp-operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

func TestDaemonSetBuilder_Basic(t *testing.T) {
	ds, err := NewDaemonSetBuilder().
		SetName("test").
		SetNamespace("default").
		SetImage("fatedier/frpc:v0.65.0").
		Build()

	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	if ds.Name != "test-frpc" {
		t.Errorf("Expected daemonset name test-frpc, got %s", ds.Name)
	}
	if ds.Namespace != "default" {
		t.Errorf("Expected namespace default, got %s", ds.Namespace)
	}
	if len(ds.Spec.Template.Spec.Containers) != 1 {
		t.Errorf("Expected 1 container, got %d", len(ds.Spec.Template.Spec.Containers))
	}
	if ds.Spec.Template.Spec.Containers[0].Image != "fatedier/frpc:v0.65.0" {
		t.Errorf("Expected image fatedier/frpc:v0.65.0, got %s", ds.Spec.Template.Spec.Containers[0].Image)
	}

	// Selector must use the operator-managed identity labels and must be a
	// subset of the pod template labels — otherwise the DaemonSet will not
	// pick up the pods it created.
	if ds.Spec.Selector == nil {
		t.Fatalf("Expected non-nil selector")
	}
	for k, v := range ds.Spec.Selector.MatchLabels {
		if ds.Spec.Template.Labels[k] != v {
			t.Errorf("Selector label %s=%s not present in pod template labels", k, v)
		}
	}

	if ds.Spec.Template.Labels["app.kubernetes.io/name"] != "test" {
		t.Errorf("Expected pod template label app.kubernetes.io/name=test")
	}
	if ds.Spec.Template.Labels["app.kubernetes.io/managed-by"] != "frp-operator" {
		t.Errorf("Expected pod template label app.kubernetes.io/managed-by=frp-operator")
	}

	if ds.Spec.Template.Annotations["sidecar.istio.io/inject"] != "false" {
		t.Errorf("Expected Istio sidecar injection disabled on pod template")
	}
}

func TestDaemonSetBuilder_WithPodTemplate(t *testing.T) {
	pt := &frpv1alpha1.ClientSpec_PodTemplate{
		Resources: &corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU: resource.MustParse("50m"),
			},
		},
		NodeSelector: map[string]string{
			"node-role.kubernetes.io/worker": "",
		},
		Tolerations: []corev1.Toleration{
			{Key: "dedicated", Operator: corev1.TolerationOpEqual, Value: "frp", Effect: corev1.TaintEffectNoSchedule},
		},
		Labels: map[string]string{
			"custom-label": "value",
		},
		ServiceAccountName: "frp-sa",
	}

	ds, err := NewDaemonSetBuilder().
		SetName("test").
		SetNamespace("default").
		SetImage("fatedier/frpc:v0.65.0").
		SetPodTemplate(pt).
		Build()

	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	if ds.Spec.Template.Spec.NodeSelector["node-role.kubernetes.io/worker"] != "" {
		t.Errorf("Expected node selector to be propagated to pod template")
	}
	if len(ds.Spec.Template.Spec.Tolerations) != 1 {
		t.Errorf("Expected 1 toleration, got %d", len(ds.Spec.Template.Spec.Tolerations))
	}
	if ds.Spec.Template.Spec.ServiceAccountName != "frp-sa" {
		t.Errorf("Expected ServiceAccountName frp-sa")
	}
	if ds.Spec.Template.Labels["custom-label"] != "value" {
		t.Errorf("Expected custom label to be merged into pod template labels")
	}
	// Selector must NOT include user-supplied labels — those can change
	// across template updates and would break selector immutability.
	if _, present := ds.Spec.Selector.MatchLabels["custom-label"]; present {
		t.Errorf("Custom label leaked into DaemonSet selector — selector must stay stable")
	}
}

func TestDaemonSetBuilder_TLSSecretMount(t *testing.T) {
	ds, err := NewDaemonSetBuilder().
		SetName("test").
		SetNamespace("default").
		SetImage("fatedier/frpc:v0.65.0").
		SetTLSSecret("frp-tls").
		Build()

	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	var found bool
	for _, v := range ds.Spec.Template.Spec.Volumes {
		if v.Name == "tls-certs" {
			found = true
			if v.Secret == nil || v.Secret.SecretName != "frp-tls" {
				t.Errorf("Expected tls-certs volume backed by Secret frp-tls")
			}
		}
	}
	if !found {
		t.Errorf("Expected tls-certs volume on pod template")
	}
}

func TestDaemonSetBuilder_ConfigVolumePresent(t *testing.T) {
	ds, err := NewDaemonSetBuilder().
		SetName("test").
		SetNamespace("default").
		SetImage("fatedier/frpc:v0.65.0").
		Build()

	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	var configVolume *corev1.Volume
	for i := range ds.Spec.Template.Spec.Volumes {
		if ds.Spec.Template.Spec.Volumes[i].Name == "test-frpc-config" {
			configVolume = &ds.Spec.Template.Spec.Volumes[i]
		}
	}
	if configVolume == nil {
		t.Fatalf("Expected test-frpc-config ConfigMap volume")
	}
	if configVolume.ConfigMap == nil || configVolume.ConfigMap.Name != "test-frpc-config" {
		t.Errorf("Expected ConfigMap source named test-frpc-config")
	}
}
