package builder

import (
	"testing"

	frpv1alpha1 "github.com/zufardhiyaulhaq/frp-operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

func TestPodBuilder_Basic(t *testing.T) {
	pod, err := NewPodBuilder().
		SetName("test").
		SetNamespace("default").
		SetImage("fatedier/frpc:v0.65.0").
		Build()

	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	// Check basic properties
	if pod.Name != "test-frpc" {
		t.Errorf("Expected pod name test-frpc, got %s", pod.Name)
	}
	if pod.Namespace != "default" {
		t.Errorf("Expected namespace default, got %s", pod.Namespace)
	}
	if len(pod.Spec.Containers) != 1 {
		t.Errorf("Expected 1 container, got %d", len(pod.Spec.Containers))
	}
	if pod.Spec.Containers[0].Image != "fatedier/frpc:v0.65.0" {
		t.Errorf("Expected image fatedier/frpc:v0.65.0, got %s", pod.Spec.Containers[0].Image)
	}

	// Check default labels
	if pod.Labels["app.kubernetes.io/name"] != "test" {
		t.Errorf("Expected label app.kubernetes.io/name=test")
	}
	if pod.Labels["app.kubernetes.io/managed-by"] != "frp-operator" {
		t.Errorf("Expected label app.kubernetes.io/managed-by=frp-operator")
	}

	// Check default annotations (service mesh disabled)
	if pod.Annotations["sidecar.istio.io/inject"] != "false" {
		t.Errorf("Expected Istio sidecar injection disabled")
	}
	if pod.Annotations["linkerd.io/inject"] != "disabled" {
		t.Errorf("Expected Linkerd injection disabled")
	}
}

func TestPodBuilder_WithPodTemplate(t *testing.T) {
	pt := &frpv1alpha1.ClientSpec_PodTemplate{
		Resources: &corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("100m"),
				corev1.ResourceMemory: resource.MustParse("64Mi"),
			},
			Limits: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("500m"),
				corev1.ResourceMemory: resource.MustParse("256Mi"),
			},
		},
		NodeSelector: map[string]string{
			"kubernetes.io/os": "linux",
		},
		Labels: map[string]string{
			"custom-label": "value",
		},
		Annotations: map[string]string{
			"custom-annotation": "value",
		},
		ServiceAccountName: "frp-sa",
		PriorityClassName:  "high-priority",
	}

	pod, err := NewPodBuilder().
		SetName("test").
		SetNamespace("default").
		SetImage("fatedier/frpc:v0.65.0").
		SetPodTemplate(pt).
		Build()

	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	// Check resources
	if pod.Spec.Containers[0].Resources.Requests.Cpu().String() != "100m" {
		t.Errorf("Expected CPU request 100m, got %s", pod.Spec.Containers[0].Resources.Requests.Cpu().String())
	}
	if pod.Spec.Containers[0].Resources.Limits.Memory().String() != "256Mi" {
		t.Errorf("Expected memory limit 256Mi, got %s", pod.Spec.Containers[0].Resources.Limits.Memory().String())
	}

	// Check node selector
	if pod.Spec.NodeSelector["kubernetes.io/os"] != "linux" {
		t.Errorf("Expected node selector kubernetes.io/os=linux")
	}

	// Check custom labels are merged
	if pod.Labels["custom-label"] != "value" {
		t.Errorf("Expected custom-label=value")
	}
	// Check default labels still present
	if pod.Labels["app.kubernetes.io/name"] != "test" {
		t.Errorf("Expected default label app.kubernetes.io/name=test to be preserved")
	}

	// Check custom annotations are merged
	if pod.Annotations["custom-annotation"] != "value" {
		t.Errorf("Expected custom-annotation=value")
	}
	// Check default annotations still present
	if pod.Annotations["sidecar.istio.io/inject"] != "false" {
		t.Errorf("Expected default annotation sidecar.istio.io/inject=false to be preserved")
	}

	// Check service account
	if pod.Spec.ServiceAccountName != "frp-sa" {
		t.Errorf("Expected ServiceAccountName frp-sa, got %s", pod.Spec.ServiceAccountName)
	}

	// Check priority class
	if pod.Spec.PriorityClassName != "high-priority" {
		t.Errorf("Expected PriorityClassName high-priority, got %s", pod.Spec.PriorityClassName)
	}
}

func TestPodBuilder_WithTolerations(t *testing.T) {
	pt := &frpv1alpha1.ClientSpec_PodTemplate{
		Tolerations: []corev1.Toleration{
			{
				Key:      "dedicated",
				Operator: corev1.TolerationOpEqual,
				Value:    "frp",
				Effect:   corev1.TaintEffectNoSchedule,
			},
		},
	}

	pod, err := NewPodBuilder().
		SetName("test").
		SetNamespace("default").
		SetImage("fatedier/frpc:v0.65.0").
		SetPodTemplate(pt).
		Build()

	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	if len(pod.Spec.Tolerations) != 1 {
		t.Errorf("Expected 1 toleration, got %d", len(pod.Spec.Tolerations))
	}
	if pod.Spec.Tolerations[0].Key != "dedicated" {
		t.Errorf("Expected toleration key 'dedicated', got %s", pod.Spec.Tolerations[0].Key)
	}
}

func TestPodBuilder_WithAffinity(t *testing.T) {
	pt := &frpv1alpha1.ClientSpec_PodTemplate{
		Affinity: &corev1.Affinity{
			NodeAffinity: &corev1.NodeAffinity{
				RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
					NodeSelectorTerms: []corev1.NodeSelectorTerm{
						{
							MatchExpressions: []corev1.NodeSelectorRequirement{
								{
									Key:      "zone",
									Operator: corev1.NodeSelectorOpIn,
									Values:   []string{"us-west-1a"},
								},
							},
						},
					},
				},
			},
		},
	}

	pod, err := NewPodBuilder().
		SetName("test").
		SetNamespace("default").
		SetImage("fatedier/frpc:v0.65.0").
		SetPodTemplate(pt).
		Build()

	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	if pod.Spec.Affinity == nil {
		t.Errorf("Expected affinity to be set")
	}
	if pod.Spec.Affinity.NodeAffinity == nil {
		t.Errorf("Expected node affinity to be set")
	}
}

func TestPodBuilder_WithImagePullSecrets(t *testing.T) {
	pt := &frpv1alpha1.ClientSpec_PodTemplate{
		ImagePullSecrets: []corev1.LocalObjectReference{
			{Name: "my-registry-secret"},
		},
	}

	pod, err := NewPodBuilder().
		SetName("test").
		SetNamespace("default").
		SetImage("fatedier/frpc:v0.65.0").
		SetPodTemplate(pt).
		Build()

	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	if len(pod.Spec.ImagePullSecrets) != 1 {
		t.Errorf("Expected 1 image pull secret, got %d", len(pod.Spec.ImagePullSecrets))
	}
	if pod.Spec.ImagePullSecrets[0].Name != "my-registry-secret" {
		t.Errorf("Expected image pull secret 'my-registry-secret', got %s", pod.Spec.ImagePullSecrets[0].Name)
	}
}

func TestPodBuilder_WithSecurityContext(t *testing.T) {
	runAsNonRoot := true
	runAsUser := int64(1000)
	pt := &frpv1alpha1.ClientSpec_PodTemplate{
		SecurityContext: &corev1.PodSecurityContext{
			RunAsNonRoot: &runAsNonRoot,
			RunAsUser:    &runAsUser,
		},
	}

	pod, err := NewPodBuilder().
		SetName("test").
		SetNamespace("default").
		SetImage("fatedier/frpc:v0.65.0").
		SetPodTemplate(pt).
		Build()

	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	if pod.Spec.SecurityContext == nil {
		t.Errorf("Expected security context to be set")
	}
	if *pod.Spec.SecurityContext.RunAsNonRoot != true {
		t.Errorf("Expected RunAsNonRoot=true")
	}
	if *pod.Spec.SecurityContext.RunAsUser != 1000 {
		t.Errorf("Expected RunAsUser=1000, got %d", *pod.Spec.SecurityContext.RunAsUser)
	}
}

func TestPodBuilder_NilPodTemplate(t *testing.T) {
	pod, err := NewPodBuilder().
		SetName("test").
		SetNamespace("default").
		SetImage("fatedier/frpc:v0.65.0").
		SetPodTemplate(nil).
		Build()

	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	// Should still build successfully with defaults
	if pod.Name != "test-frpc" {
		t.Errorf("Expected pod name test-frpc, got %s", pod.Name)
	}
	if pod.Spec.ServiceAccountName != "" {
		t.Errorf("Expected empty service account name when PodTemplate is nil")
	}
}

func TestPodBuilder_LabelAnnotationOverride(t *testing.T) {
	pt := &frpv1alpha1.ClientSpec_PodTemplate{
		Labels: map[string]string{
			"app.kubernetes.io/name": "custom-name", // Override default
		},
		Annotations: map[string]string{
			"sidecar.istio.io/inject": "true", // Override default
		},
	}

	pod, err := NewPodBuilder().
		SetName("test").
		SetNamespace("default").
		SetImage("fatedier/frpc:v0.65.0").
		SetPodTemplate(pt).
		Build()

	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	// Custom values should override defaults
	if pod.Labels["app.kubernetes.io/name"] != "custom-name" {
		t.Errorf("Expected custom label to override default, got %s", pod.Labels["app.kubernetes.io/name"])
	}
	if pod.Annotations["sidecar.istio.io/inject"] != "true" {
		t.Errorf("Expected custom annotation to override default, got %s", pod.Annotations["sidecar.istio.io/inject"])
	}
}
