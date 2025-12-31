package builder

import (
	"github.com/zufardhiyaulhaq/frp-operator/pkg/client/models"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

type ServiceBuilder struct {
	Name        string
	Namespace   string
	AdminPort   int
	VisitorPort []int
}

func NewServiceBuilder() *ServiceBuilder {
	return &ServiceBuilder{
		AdminPort: models.DEFAULT_ADMIN_PORT,
	}
}

func (n *ServiceBuilder) SetName(name string) *ServiceBuilder {
	n.Name = name
	return n
}

func (n *ServiceBuilder) SetNamespace(namespace string) *ServiceBuilder {
	n.Namespace = namespace
	return n
}

func (n *ServiceBuilder) SetAdminPort(adminPort int) *ServiceBuilder {
	n.AdminPort = adminPort
	return n
}

func (n *ServiceBuilder) AddVisitorPort(visitorPort int) *ServiceBuilder {
	n.VisitorPort = append(n.VisitorPort, visitorPort)
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
					Port:     int32(n.AdminPort),
					TargetPort: intstr.IntOrString{
						Type:   0,
						IntVal: int32(n.AdminPort),
					},
				},
			},
			Type: "ClusterIP",
		},
	}

	for _, port := range n.VisitorPort {
		servicePort := corev1.ServicePort{
			Name:     "tcp-visitor-" + string(port),
			Protocol: corev1.ProtocolTCP,
			Port:     int32(port),
			TargetPort: intstr.IntOrString{
				Type:   0,
				IntVal: int32(port),
			},
		}
		Service.Spec.Ports = append(Service.Spec.Ports, servicePort)
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
