// Package podtemplate applies a ClientSpec_PodTemplate to a pod; Clients and Servers share it.
package podtemplate

import (
	corev1 "k8s.io/api/core/v1"

	frpv1alpha1 "github.com/zufardhiyaulhaq/frp-operator/api/v1alpha1"
)

// Apply merges t into pod: labels and annotations are added (t wins), the first container gets
// t's resources and env, and the scheduling and security fields replace the pod's.
func Apply(pod *corev1.Pod, t *frpv1alpha1.ClientSpec_PodTemplate) {
	if t == nil {
		return
	}
	if len(t.Labels) > 0 && pod.Labels == nil {
		pod.Labels = map[string]string{}
	}
	for k, v := range t.Labels {
		pod.Labels[k] = v
	}
	if len(t.Annotations) > 0 && pod.Annotations == nil {
		pod.Annotations = map[string]string{}
	}
	for k, v := range t.Annotations {
		pod.Annotations[k] = v
	}
	if len(pod.Spec.Containers) > 0 {
		c := &pod.Spec.Containers[0]
		if t.Resources != nil {
			c.Resources = *t.Resources
		}
		if t.Env != nil {
			c.Env = t.Env
		}
	}
	if t.NodeSelector != nil {
		pod.Spec.NodeSelector = t.NodeSelector
	}
	if t.Tolerations != nil {
		pod.Spec.Tolerations = t.Tolerations
	}
	if t.Affinity != nil {
		pod.Spec.Affinity = t.Affinity
	}
	if t.ServiceAccountName != "" {
		pod.Spec.ServiceAccountName = t.ServiceAccountName
	}
	if t.ImagePullSecrets != nil {
		pod.Spec.ImagePullSecrets = t.ImagePullSecrets
	}
	if t.PriorityClassName != "" {
		pod.Spec.PriorityClassName = t.PriorityClassName
	}
	if t.SecurityContext != nil {
		pod.Spec.SecurityContext = t.SecurityContext
	}
}
