package loadbalancer

import (
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	frpv1alpha1 "github.com/zufardhiyaulhaq/frp-operator/api/v1alpha1"
)

func testPool(mode string) *frpv1alpha1.ServerPool {
	proto := "kcp"
	return &frpv1alpha1.ServerPool{
		ObjectMeta: metav1.ObjectMeta{Name: "prod", Namespace: "frp-operator"},
		Spec: frpv1alpha1.ServerPoolSpec{
			ClientMode: mode,
			Servers: []frpv1alpha1.ServerPoolServer{{
				Name: "sg-01", Host: "10.0.0.1", Port: 7000, PublicAddress: "1.2.3.4",
				TransportProtocol: &proto,
				Authentication: frpv1alpha1.ClientSpec_Server_Authentication{
					Token: &frpv1alpha1.ClientSpec_Server_Authentication_Token{Secret: frpv1alpha1.Secret{Name: "tok", Key: "token"}},
				},
				Transport: &frpv1alpha1.ClientSpec_Server_Transport{WireProtocol: "v2"},
			}},
			ClientTemplate: &frpv1alpha1.ServerPoolClientTemplate{
				PodTemplate: &frpv1alpha1.ClientSpec_PodTemplate{NodeSelector: map[string]string{"zone": "a"}},
			},
		},
	}
}

func testService() *corev1.Service {
	return &corev1.Service{ObjectMeta: metav1.ObjectMeta{
		Name: "web", Namespace: "default", UID: "uid-1",
		CreationTimestamp: metav1.NewTime(time.Date(2026, 8, 30, 0, 0, 0, 0, time.UTC)),
	}}
}

func TestBuildClient_PerService(t *testing.T) {
	p := testPool(frpv1alpha1.ClientModePerService)
	c := BuildClient(p, p.Spec.Servers[0], testService(), ServerRef{"prod", "sg-01"})
	if c.Namespace != "frp-operator" || c.Name != "lb-default-web-sg-01" {
		t.Fatalf("meta: %s/%s", c.Namespace, c.Name)
	}
	if c.Spec.ClientID == nil || *c.Spec.ClientID != "lb/default/web/sg-01" {
		t.Fatalf("clientID: %v", c.Spec.ClientID)
	}
	if c.Spec.Server.Host != "10.0.0.1" || c.Spec.Server.Port != 7000 || *c.Spec.Server.Protocol != "kcp" ||
		c.Spec.Server.Authentication.Token.Secret.Name != "tok" || c.Spec.Server.Transport.WireProtocol != "v2" {
		t.Fatalf("server spec not copied: %+v", c.Spec.Server)
	}
	if c.Spec.PodTemplate == nil || c.Spec.PodTemplate.NodeSelector["zone"] != "a" {
		t.Fatalf("podTemplate not applied: %+v", c.Spec.PodTemplate)
	}
	for k, v := range map[string]string{LabelManagedBy: ManagedByValue, LabelPool: "prod", LabelServer: "sg-01",
		LabelServiceUID: "uid-1", LabelServiceNamespace: "default", LabelServiceName: "web"} {
		if c.Labels[k] != v {
			t.Errorf("label %s = %q, want %q", k, c.Labels[k], v)
		}
	}
}

func TestBuildClient_Shared(t *testing.T) {
	p := testPool(frpv1alpha1.ClientModeShared)
	c := BuildClient(p, p.Spec.Servers[0], testService(), ServerRef{"prod", "sg-01"})
	if c.Name != "pool-prod-sg-01" || *c.Spec.ClientID != "pool/prod/sg-01" {
		t.Fatalf("%s %v", c.Name, c.Spec.ClientID)
	}
	if _, ok := c.Labels[LabelServiceUID]; ok {
		t.Fatal("shared client must not carry a service-uid label")
	}
}

func TestBuildUpstream_TCPAndUDP(t *testing.T) {
	p := testPool(frpv1alpha1.ClientModePerService)
	s := testService()
	ref := ServerRef{"prod", "sg-01"}
	u := BuildUpstream(p, s, corev1.ServicePort{Name: "https", Port: 443, Protocol: corev1.ProtocolTCP}, ref)
	if u.Namespace != "frp-operator" || u.Name != "lb-default-web-sg-01-443" || u.Spec.Client != "lb-default-web-sg-01" {
		t.Fatalf("meta: %s/%s client=%s", u.Namespace, u.Name, u.Spec.Client)
	}
	if u.Spec.TCP == nil || u.Spec.TCP.Host != "web.default.svc.cluster.local" || u.Spec.TCP.Port != 443 || u.Spec.TCP.Server.Port != 443 || u.Spec.UDP != nil {
		t.Fatalf("tcp spec: %+v", u.Spec)
	}
	if u.Annotations[AnnotationServiceCreated] != "2026-08-30T00:00:00Z" {
		t.Fatalf("created annotation: %q", u.Annotations[AnnotationServiceCreated])
	}
	if u.Labels[LabelPort] != "443" || u.Labels[LabelProtocol] != "TCP" || u.Labels[LabelServiceUID] != "uid-1" {
		t.Fatalf("labels: %v", u.Labels)
	}

	u = BuildUpstream(p, s, corev1.ServicePort{Name: "dns", Port: 53, Protocol: corev1.ProtocolUDP}, ref)
	if u.Spec.UDP == nil || u.Spec.UDP.Server.Port != 53 || u.Spec.TCP != nil {
		t.Fatalf("udp spec: %+v", u.Spec)
	}
	if u.Name != "lb-default-web-sg-01-53-udp" {
		t.Fatalf("udp name must not collide with a tcp upstream on the same port: %q", u.Name)
	}

	// A TCP upstream on the same port number as the UDP one above must get a distinct name.
	tcpSamePort := BuildUpstream(p, s, corev1.ServicePort{Name: "dns-tcp", Port: 53, Protocol: corev1.ProtocolTCP}, ref)
	if tcpSamePort.Name != "lb-default-web-sg-01-53" || tcpSamePort.Name == u.Name {
		t.Fatalf("tcp/udp name collision on port 53: tcp=%q udp=%q", tcpSamePort.Name, u.Name)
	}

	// Shared mode: upstream references the shared client
	p.Spec.ClientMode = frpv1alpha1.ClientModeShared
	u = BuildUpstream(p, s, corev1.ServicePort{Port: 443, Protocol: corev1.ProtocolTCP}, ref)
	if u.Spec.Client != "pool-prod-sg-01" {
		t.Fatalf("shared client ref: %s", u.Spec.Client)
	}
}

func TestAllocationFromUpstream(t *testing.T) {
	p := testPool(frpv1alpha1.ClientModePerService)
	u := BuildUpstream(p, testService(), corev1.ServicePort{Port: 443, Protocol: corev1.ProtocolTCP}, ServerRef{"prod", "sg-01"})
	a, ok := AllocationFromUpstream(u)
	if !ok || a.Pool != "prod" || a.Server != "sg-01" || a.Port != 443 || a.Protocol != corev1.ProtocolTCP ||
		a.ServiceKey != "default/web" || a.ServiceCreated.Year() != 2026 {
		t.Fatalf("got %+v ok=%v", a, ok)
	}
	if _, ok := AllocationFromUpstream(&frpv1alpha1.Upstream{}); ok {
		t.Fatal("unmanaged upstream must be ignored")
	}
}
