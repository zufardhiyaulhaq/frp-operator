package podtemplate

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	frpv1alpha1 "github.com/zufardhiyaulhaq/frp-operator/api/v1alpha1"
)

func TestApply_MergesIntoPod(t *testing.T) {
	runAsUser := int64(1000)
	pod := &corev1.Pod{}
	pod.Labels = map[string]string{"app": "frps", "keep": "yes"}
	pod.Spec.Containers = []corev1.Container{{Name: "frps"}}
	pod.Spec.SecurityContext = &corev1.PodSecurityContext{Sysctls: []corev1.Sysctl{{Name: "x", Value: "1"}}}

	Apply(pod, &frpv1alpha1.ClientSpec_PodTemplate{
		Labels:            map[string]string{"app": "mine", "team": "net"},
		Annotations:       map[string]string{"a": "b"},
		Resources:         &corev1.ResourceRequirements{Limits: corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("64Mi")}},
		Env:               []corev1.EnvVar{{Name: "GOMEMLIMIT", Value: "60MiB"}},
		NodeSelector:      map[string]string{"zone": "a"},
		PriorityClassName: "high",
		SecurityContext:   &corev1.PodSecurityContext{RunAsUser: &runAsUser},
	})

	if pod.Labels["app"] != "mine" || pod.Labels["team"] != "net" || pod.Labels["keep"] != "yes" {
		t.Errorf("labels = %v", pod.Labels)
	}
	if pod.Annotations["a"] != "b" {
		t.Errorf("annotations = %v", pod.Annotations)
	}
	c := pod.Spec.Containers[0]
	if c.Resources.Limits.Memory().String() != "64Mi" || len(c.Env) != 1 {
		t.Errorf("container = %+v", c)
	}
	if pod.Spec.NodeSelector["zone"] != "a" || pod.Spec.PriorityClassName != "high" {
		t.Errorf("spec = %+v", pod.Spec)
	}
	if pod.Spec.SecurityContext.Sysctls != nil || *pod.Spec.SecurityContext.RunAsUser != 1000 {
		t.Errorf("securityContext = %+v, want the template's to replace the pod's", pod.Spec.SecurityContext)
	}
}

func TestApply_NilTemplate(t *testing.T) {
	pod := &corev1.Pod{}
	pod.Spec.Containers = []corev1.Container{{Name: "frps"}}
	Apply(pod, nil)
	if pod.Labels != nil || pod.Spec.SecurityContext != nil {
		t.Errorf("pod changed: %+v", pod)
	}
}
