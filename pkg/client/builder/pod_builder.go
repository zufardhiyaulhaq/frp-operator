package builder

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	frpv1alpha1 "github.com/zufardhiyaulhaq/frp-operator/api/v1alpha1"
	"github.com/zufardhiyaulhaq/frp-operator/pkg/podtemplate"
)

type PodBuilder struct {
	Name        string
	Namespace   string
	Image       string
	PodTemplate *frpv1alpha1.ClientSpec_PodTemplate
	TLS         *frpv1alpha1.ClientSpec_Server_TLS
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

// SetTLS mounts the Client's TLS files at /etc/frp/tls as tls.crt, tls.key and ca.crt, each read
// from its own Secret (or ConfigMap, for the CA) and key.
func (n *PodBuilder) SetTLS(tls *frpv1alpha1.ClientSpec_Server_TLS) *PodBuilder {
	n.TLS = tls
	return n
}

// tlsProjection lists the projected-volume sources for the TLS files that are set.
func tlsProjection(t *frpv1alpha1.ClientSpec_Server_TLS) []corev1.VolumeProjection {
	if t == nil {
		return nil
	}
	var sources []corev1.VolumeProjection
	secret := func(ref frpv1alpha1.Secret, path string) {
		sources = append(sources, corev1.VolumeProjection{Secret: &corev1.SecretProjection{
			LocalObjectReference: corev1.LocalObjectReference{Name: ref.Name},
			Items:                []corev1.KeyToPath{{Key: ref.Key, Path: path}},
		}})
	}
	if t.CertFile != nil {
		secret(t.CertFile.Secret, "tls.crt")
	}
	if t.KeyFile != nil {
		secret(t.KeyFile.Secret, "tls.key")
	}
	if ca := t.TrustedCAFile; ca != nil {
		switch {
		case ca.Secret != nil:
			secret(*ca.Secret, "ca.crt")
		case ca.ConfigMap != nil:
			sources = append(sources, corev1.VolumeProjection{ConfigMap: &corev1.ConfigMapProjection{
				LocalObjectReference: corev1.LocalObjectReference{Name: ca.ConfigMap.Name},
				Items:                []corev1.KeyToPath{{Key: ca.ConfigMap.Key, Path: "ca.crt"}},
			}})
		}
	}
	return sources
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

	// Add the TLS files, each from its own reference
	if sources := tlsProjection(n.TLS); len(sources) > 0 {
		pod.Spec.Volumes = append(pod.Spec.Volumes, corev1.Volume{
			Name:         "tls-certs",
			VolumeSource: corev1.VolumeSource{Projected: &corev1.ProjectedVolumeSource{Sources: sources}},
		})
		pod.Spec.Containers[0].VolumeMounts = append(pod.Spec.Containers[0].VolumeMounts, corev1.VolumeMount{
			Name:      "tls-certs",
			MountPath: "/etc/frp/tls",
			ReadOnly:  true,
		})
	}

	podtemplate.Apply(pod, n.PodTemplate)

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
