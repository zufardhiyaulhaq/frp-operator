package builder

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type PodBuilder struct {
	Name      string
	Namespace string
	Image     string
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

func (n *PodBuilder) Build() (*corev1.Pod, error) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      n.Name + "-frpc",
			Namespace: n.Namespace,
			Labels:    n.BuildLabels(),
			Annotations: map[string]string{
				"sidecar.istio.io/inject":                "false",
				"linkerd.io/inject":                      "disabled",
				"kuma.io/sidecar-injection":              "disabled",
				"appmesh.k8s.aws/sidecarInjectorWebhook": "disabled",
				"injector.nsm.nginx.com/auto-inject":     "false",
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:    "frpc",
					Image:   n.Image,
					Command: []string{"-c", "/frp/config.ini"},
					Ports: []corev1.ContainerPort{
						{ContainerPort: int32(4040)},
					},
					VolumeMounts: []corev1.VolumeMount{
						{
							Name:      n.Name + "-frpc-config",
							MountPath: "/frp",
						},
					},
				},
			},
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
