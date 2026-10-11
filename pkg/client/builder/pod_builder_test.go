package builder

import (
	"reflect"
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

func TestPodBuilder_WithEnv(t *testing.T) {
	pt := &frpv1alpha1.ClientSpec_PodTemplate{
		Env: []corev1.EnvVar{
			{Name: "GOMEMLIMIT", Value: "80MiB"},
			{
				Name: "MEM_LIMIT",
				ValueFrom: &corev1.EnvVarSource{
					ResourceFieldRef: &corev1.ResourceFieldSelector{Resource: "limits.memory"},
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

	env := pod.Spec.Containers[0].Env
	if len(env) != 2 {
		t.Fatalf("Expected 2 env vars, got %d", len(env))
	}
	if env[0].Name != "GOMEMLIMIT" || env[0].Value != "80MiB" {
		t.Errorf("Expected GOMEMLIMIT=80MiB, got %s=%s", env[0].Name, env[0].Value)
	}
	if env[1].ValueFrom == nil || env[1].ValueFrom.ResourceFieldRef == nil ||
		env[1].ValueFrom.ResourceFieldRef.Resource != "limits.memory" {
		t.Errorf("Expected MEM_LIMIT to reference limits.memory, got %+v", env[1])
	}
}

func TestPodBuilder_NoEnvByDefault(t *testing.T) {
	pod, err := NewPodBuilder().
		SetName("test").
		SetNamespace("default").
		SetImage("fatedier/frpc:v0.65.0").
		SetPodTemplate(&frpv1alpha1.ClientSpec_PodTemplate{}).
		Build()
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if len(pod.Spec.Containers[0].Env) != 0 {
		t.Errorf("Expected no env vars, got %v", pod.Spec.Containers[0].Env)
	}
}

// projectedTLSFiles maps each file in the pod's tls-certs volume to "<kind>:<name>/<key>".
func projectedTLSFiles(t *testing.T, pod *corev1.Pod) map[string]string {
	t.Helper()
	for _, v := range pod.Spec.Volumes {
		if v.Name != "tls-certs" {
			continue
		}
		if v.Projected == nil {
			t.Fatalf("tls-certs volume is not projected: %+v", v.VolumeSource)
		}
		got := map[string]string{}
		for _, src := range v.Projected.Sources {
			switch {
			case src.Secret != nil:
				for _, it := range src.Secret.Items {
					got[it.Path] = "secret:" + src.Secret.Name + "/" + it.Key
				}
			case src.ConfigMap != nil:
				for _, it := range src.ConfigMap.Items {
					got[it.Path] = "configmap:" + src.ConfigMap.Name + "/" + it.Key
				}
			}
		}
		return got
	}
	return nil
}

func TestPodBuilder_TLSFilesFromSeparateSources(t *testing.T) {
	pod, err := NewPodBuilder().SetName("c").SetNamespace("ns").SetImage("img").
		SetTLS(&frpv1alpha1.ClientSpec_Server_TLS{
			Enable:        true,
			CertFile:      &frpv1alpha1.SecretRef{Secret: frpv1alpha1.Secret{Name: "client-cert", Key: "cert.pem"}},
			KeyFile:       &frpv1alpha1.SecretRef{Secret: frpv1alpha1.Secret{Name: "client-cert", Key: "key.pem"}},
			TrustedCAFile: &frpv1alpha1.ConfigMapOrSecretRef{Secret: &frpv1alpha1.Secret{Name: "server-ca", Key: "ca.crt"}},
		}).Build()
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"tls.crt": "secret:client-cert/cert.pem",
		"tls.key": "secret:client-cert/key.pem",
		"ca.crt":  "secret:server-ca/ca.crt",
	}
	if got := projectedTLSFiles(t, pod); !reflect.DeepEqual(got, want) {
		t.Errorf("TLS files = %v, want %v", got, want)
	}
	mounted := false
	for _, m := range pod.Spec.Containers[0].VolumeMounts {
		if m.Name == "tls-certs" && m.MountPath == "/etc/frp/tls" && m.ReadOnly {
			mounted = true
		}
	}
	if !mounted {
		t.Errorf("tls-certs is not mounted read-only at /etc/frp/tls: %+v", pod.Spec.Containers[0].VolumeMounts)
	}
}

func TestPodBuilder_TLSCAFromConfigMapWithClientCertificate(t *testing.T) {
	pod, err := NewPodBuilder().SetName("c").SetNamespace("ns").SetImage("img").
		SetTLS(&frpv1alpha1.ClientSpec_Server_TLS{
			Enable:        true,
			CertFile:      &frpv1alpha1.SecretRef{Secret: frpv1alpha1.Secret{Name: "client-cert", Key: "tls.crt"}},
			KeyFile:       &frpv1alpha1.SecretRef{Secret: frpv1alpha1.Secret{Name: "client-cert", Key: "tls.key"}},
			TrustedCAFile: &frpv1alpha1.ConfigMapOrSecretRef{ConfigMap: &frpv1alpha1.ConfigMapRef{Name: "server-ca", Key: "root.pem"}},
		}).Build()
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"tls.crt": "secret:client-cert/tls.crt",
		"tls.key": "secret:client-cert/tls.key",
		"ca.crt":  "configmap:server-ca/root.pem",
	}
	if got := projectedTLSFiles(t, pod); !reflect.DeepEqual(got, want) {
		t.Errorf("TLS files = %v, want %v", got, want)
	}
}

func TestPodBuilder_NoTLSVolumeWithoutFiles(t *testing.T) {
	pod, err := NewPodBuilder().SetName("c").SetNamespace("ns").SetImage("img").
		SetTLS(&frpv1alpha1.ClientSpec_Server_TLS{Enable: true}).Build()
	if err != nil {
		t.Fatal(err)
	}
	if files := projectedTLSFiles(t, pod); files != nil {
		t.Errorf("unexpected tls-certs volume with files %v", files)
	}
}
