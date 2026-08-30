package loadbalancer

import (
	"strconv"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	frpv1alpha1 "github.com/zufardhiyaulhaq/frp-operator/api/v1alpha1"
)

func baseLabels(ref ServerRef) map[string]string {
	return map[string]string{
		LabelManagedBy: ManagedByValue,
		LabelPool:      ref.Pool,
		LabelServer:    ref.Server,
	}
}

func serviceLabels(labels map[string]string, svc *corev1.Service) map[string]string {
	labels[LabelServiceUID] = string(svc.UID)
	labels[LabelServiceNamespace] = svc.Namespace
	labels[LabelServiceName] = svc.Name
	return labels
}

// UpstreamLabels are the labels every generated Upstream carries; they are the allocation record.
func UpstreamLabels(svc *corev1.Service, ref ServerRef, port corev1.ServicePort) map[string]string {
	labels := serviceLabels(baseLabels(ref), svc)
	labels[LabelPort] = strconv.Itoa(int(port.Port))
	labels[LabelProtocol] = string(protocolOf(port))
	return labels
}

func protocolOf(port corev1.ServicePort) corev1.Protocol {
	if port.Protocol == "" {
		return corev1.ProtocolTCP
	}
	return port.Protocol
}

// BuildClient renders the Client for one pool server, in the pool's namespace.
func BuildClient(pool *frpv1alpha1.ServerPool, server frpv1alpha1.ServerPoolServer, svc *corev1.Service, ref ServerRef) *frpv1alpha1.Client {
	mode := pool.Spec.ClientMode
	labels := baseLabels(ref)
	if mode != frpv1alpha1.ClientModeShared {
		labels = serviceLabels(labels, svc)
	}
	clientID := ClientID(svc, ref, mode)
	c := &frpv1alpha1.Client{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ClientName(svc, ref, mode),
			Namespace: pool.Namespace,
			Labels:    labels,
		},
		Spec: frpv1alpha1.ClientSpec{
			ClientID: &clientID,
			Server: frpv1alpha1.ClientSpec_Server{
				Host:           server.Host,
				Port:           server.Port,
				Protocol:       server.TransportProtocol,
				Authentication: server.Authentication,
				AdminServer:    server.AdminServer,
				TLS:            server.TLS,
				Transport:      server.Transport,
			},
		},
	}
	if pool.Spec.ClientTemplate != nil {
		c.Spec.PodTemplate = pool.Spec.ClientTemplate.PodTemplate
	}
	return c
}

// BuildUpstream renders the Upstream for one Service port on one server.
func BuildUpstream(pool *frpv1alpha1.ServerPool, svc *corev1.Service, port corev1.ServicePort, ref ServerRef) *frpv1alpha1.Upstream {
	u := &frpv1alpha1.Upstream{
		ObjectMeta: metav1.ObjectMeta{
			Name:      UpstreamName(svc, ref, port.Port, protocolOf(port)),
			Namespace: pool.Namespace,
			Labels:    UpstreamLabels(svc, ref, port),
			Annotations: map[string]string{
				AnnotationServiceCreated: svc.CreationTimestamp.UTC().Format(time.RFC3339),
			},
		},
		Spec: frpv1alpha1.UpstreamSpec{
			Client: ClientName(svc, ref, pool.Spec.ClientMode),
		},
	}
	host := ServiceHost(svc)
	if protocolOf(port) == corev1.ProtocolUDP {
		u.Spec.UDP = &frpv1alpha1.UpstreamSpec_UDP{
			Host:   host,
			Port:   int(port.Port),
			Server: frpv1alpha1.UpstreamSpec_UDP_Server{Port: int(port.Port)},
		}
	} else {
		u.Spec.TCP = &frpv1alpha1.UpstreamSpec_TCP{
			Host:   host,
			Port:   int(port.Port),
			Server: frpv1alpha1.UpstreamSpec_TCP_Server{Port: int(port.Port)},
		}
	}
	return u
}

// AllocationFromUpstream reads the allocation record back from a generated Upstream.
func AllocationFromUpstream(u *frpv1alpha1.Upstream) (Allocation, bool) {
	l := u.Labels
	if l[LabelManagedBy] != ManagedByValue {
		return Allocation{}, false
	}
	port, err := strconv.Atoi(l[LabelPort])
	if err != nil {
		return Allocation{}, false
	}
	created, _ := time.Parse(time.RFC3339, u.Annotations[AnnotationServiceCreated])
	return Allocation{
		Pool:           l[LabelPool],
		Server:         l[LabelServer],
		Port:           int32(port),
		Protocol:       corev1.Protocol(l[LabelProtocol]),
		ServiceKey:     l[LabelServiceNamespace] + "/" + l[LabelServiceName],
		ServiceCreated: created,
	}, true
}
