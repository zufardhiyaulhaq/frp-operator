package builder

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	frpv1alpha1 "github.com/zufardhiyaulhaq/frp-operator/api/v1alpha1"
)

type PodBuilder struct {
	Name           string
	Namespace      string
	Image          string
	PodTemplate    *frpv1alpha1.ClientSpec_PodTemplate
	TLSSecret      string
	TLSCAConfigMap string
}

func NewPodBuilder() *PodBuilder {
	return &PodBuilder{}
}

func (n *PodBuilder) SetName(name string) *PodBuilder {
	n.Name = name
	return n
}

func (n *PodBuilder) SetNamespace(namespace string) *PodBuilder {
	n.Namespace = namespace
	return n
}

func (n *PodBuilder) SetImage(image string) *PodBuilder {
	n.Image = image
	return n
}

func (n *PodBuilder) SetPodTemplate(podTemplate *frpv1alpha1.ClientSpec_PodTemplate) *PodBuilder {
	n.PodTemplate = podTemplate
	return n
}

func (n *PodBuilder) SetTLSSecret(tlsSecret string) *PodBuilder {
	n.TLSSecret = tlsSecret
	return n
}

func (n *PodBuilder) SetTLSCAConfigMap(tlsCAConfigMap string) *PodBuilder {
	n.TLSCAConfigMap = tlsCAConfigMap
	return n
}

func (n *PodBuilder) Build() (*corev1.Pod, error) {
	// Build base labels and annotations
	labels := n.BuildLabels()
	annotations := map[string]string{
		"sidecar.istio.io/inject":                "false",
		"linkerd.io/inject":                      "disabled",
		"kuma.io/sidecar-injection":              "disabled",
		"appmesh.k8s.aws/sidecarInjectorWebhook": "disabled",
		"injector.nsm.nginx.com/auto-inject":     "false",
	}

	// Merge PodTemplate labels and annotations
	if n.PodTemplate != nil {
		for k, v := range n.PodTemplate.Labels {
			labels[k] = v
		}
		for k, v := range n.PodTemplate.Annotations {
			annotations[k] = v
		}
	}

	// Build container
	container := corev1.Container{
		Name:    "frpc",
		Image:   n.Image,
		Command: []string{"frpc", "-c", "/frp/config.toml"},
		Ports: []corev1.ContainerPort{
			{ContainerPort: int32(4040)},
		},
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      n.Name + "-frpc-config",
				MountPath: "/frp",
			},
		},
	}

	// Apply container resources from PodTemplate
	if n.PodTemplate != nil && n.PodTemplate.Resources != nil {
		container.Resources = *n.PodTemplate.Resources
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:        n.Name + "-frpc",
			Namespace:   n.Namespace,
			Labels:      labels,
			Annotations: annotations,
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{container},
			Volumes: []corev1.Volume{
				{
					Name: n.Name + "-frpc-config",
					VolumeSource: corev1.VolumeSource{
						ConfigMap: &corev1.ConfigMapVolumeSource{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: n.Name + "-frpc-config",
							},
						},
					},
				},
			},
		},
	}

	// Add TLS volumes and mounts if configured
	if n.TLSSecret != "" {
		pod.Spec.Volumes = append(pod.Spec.Volumes, corev1.Volume{
			Name: "tls-certs",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: n.TLSSecret,
				},
			},
		})
		pod.Spec.Containers[0].VolumeMounts = append(
			pod.Spec.Containers[0].VolumeMounts,
			corev1.VolumeMount{
				Name:      "tls-certs",
				MountPath: "/etc/frp/tls",
				ReadOnly:  true,
			},
		)
	}

	// Add TLS CA ConfigMap volume if configured separately
	if n.TLSCAConfigMap != "" && n.TLSSecret == "" {
		pod.Spec.Volumes = append(pod.Spec.Volumes, corev1.Volume{
			Name: "tls-ca",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: n.TLSCAConfigMap,
					},
				},
			},
		})
		pod.Spec.Containers[0].VolumeMounts = append(
			pod.Spec.Containers[0].VolumeMounts,
			corev1.VolumeMount{
				Name:      "tls-ca",
				MountPath: "/etc/frp/tls",
				ReadOnly:  true,
			},
		)
	}

	// Apply PodTemplate fields to pod spec
	if n.PodTemplate != nil {
		if n.PodTemplate.NodeSelector != nil {
			pod.Spec.NodeSelector = n.PodTemplate.NodeSelector
		}
		if n.PodTemplate.Tolerations != nil {
			pod.Spec.Tolerations = n.PodTemplate.Tolerations
		}
		if n.PodTemplate.Affinity != nil {
			pod.Spec.Affinity = n.PodTemplate.Affinity
		}
		if n.PodTemplate.ServiceAccountName != "" {
			pod.Spec.ServiceAccountName = n.PodTemplate.ServiceAccountName
		}
		if n.PodTemplate.ImagePullSecrets != nil {
			pod.Spec.ImagePullSecrets = n.PodTemplate.ImagePullSecrets
		}
		if n.PodTemplate.PriorityClassName != "" {
			pod.Spec.PriorityClassName = n.PodTemplate.PriorityClassName
		}
		if n.PodTemplate.SecurityContext != nil {
			pod.Spec.SecurityContext = n.PodTemplate.SecurityContext
		}
	}

	return pod, nil
}

func (n *PodBuilder) BuildLabels() map[string]string {
	var labels = map[string]string{
		"app.kubernetes.io/name":       n.Name,
		"app.kubernetes.io/managed-by": "frp-operator",
		"app.kubernetes.io/created-by": n.Name,
	}

	return labels
}
