package builder

import (
	"github.com/zufardhiyaulhaq/frp-operator/pkg/client/models"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

type ServiceBuilder struct {
	Name      string
	Namespace string
}

func NewServiceBuilder() *ServiceBuilder {
	return &ServiceBuilder{}
}

func (n *ServiceBuilder) SetName(name string) *ServiceBuilder {
	n.Name = name
	return n
}

func (n *ServiceBuilder) SetNamespace(namespace string) *ServiceBuilder {
	n.Namespace = namespace
	return n
}

func (n *ServiceBuilder) Build() (*corev1.Service, error) {
	Service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      n.Name + "-frpc",
			Namespace: n.Namespace,
			Labels:    n.BuildLabels(),
		},
		Spec: corev1.ServiceSpec{
			Selector: n.BuildLabels(),
			Ports: []corev1.ServicePort{
				{
					Name:     "http-api",
					Protocol: corev1.ProtocolTCP,
					Port:     models.DEFAULT_ADMIN_PORT,
					TargetPort: intstr.IntOrString{
						Type:   0,
						IntVal: models.DEFAULT_ADMIN_PORT,
					},
				},
			},
			Type: "ClusterIP",
		},
	}

	return Service, nil
}

func (n *ServiceBuilder) BuildLabels() map[string]string {
	var labels = map[string]string{
		"app.kubernetes.io/name":       n.Name,
		"app.kubernetes.io/managed-by": "frp-operator",
		"app.kubernetes.io/created-by": n.Name,
	}

	return labels
}
