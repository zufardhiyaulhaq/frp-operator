package loadbalancer

import (
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	frpv1alpha1 "github.com/zufardhiyaulhaq/frp-operator/api/v1alpha1"
)

func svc(ns, name string) *corev1.Service {
	return &corev1.Service{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: name}}
}

func TestIsClaimed(t *testing.T) {
	class := LoadBalancerClass
	other := "metallb.io/metallb"
	cases := []struct {
		name string
		svc  corev1.ServiceSpec
		want bool
	}{
		{"lb with our class", corev1.ServiceSpec{Type: corev1.ServiceTypeLoadBalancer, LoadBalancerClass: &class}, true},
		{"lb without class", corev1.ServiceSpec{Type: corev1.ServiceTypeLoadBalancer}, false},
		{"lb other class", corev1.ServiceSpec{Type: corev1.ServiceTypeLoadBalancer, LoadBalancerClass: &other}, false},
		{"clusterip with our class", corev1.ServiceSpec{Type: corev1.ServiceTypeClusterIP, LoadBalancerClass: &class}, false},
	}
	for _, c := range cases {
		s := svc("a", "b")
		s.Spec = c.svc
		if got := IsClaimed(s); got != c.want {
			t.Errorf("%s: IsClaimed = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestParseServerList(t *testing.T) {
	if got := ParseServerList(" sg-01, sg-02 ,,sg-03 "); len(got) != 3 || got[0] != "sg-01" || got[2] != "sg-03" {
		t.Fatalf("got %v", got)
	}
	if got := ParseServerList(""); got != nil {
		t.Fatalf("empty: got %v, want nil", got)
	}
	if got := ParseServerList("sg-01,sg-02,sg-01"); len(got) != 2 || got[0] != "sg-01" || got[1] != "sg-02" {
		t.Fatalf("duplicates: got %v, want [sg-01, sg-02]", got)
	}
}

func TestAllocatedRoundTrip(t *testing.T) {
	refs := []ServerRef{{"prod", "sg-01"}, {"prod", "sg-02"}}
	s := FormatAllocated(refs)
	if s != "prod/sg-01,prod/sg-02" {
		t.Fatalf("format: %q", s)
	}
	back, err := ParseAllocated(s)
	if err != nil || len(back) != 2 || back[1] != refs[1] {
		t.Fatalf("parse: %v %v", back, err)
	}
	if got, err := ParseAllocated(""); err != nil || got != nil {
		t.Fatalf("empty: %v %v", got, err)
	}
	if _, err := ParseAllocated("sg-01"); err == nil {
		t.Fatal("expected error for missing pool")
	}
}

func TestSafeName(t *testing.T) {
	if got := SafeName("short"); got != "short" {
		t.Fatalf("short: %q", got)
	}
	long := strings.Repeat("a", 70)
	got := SafeName(long)
	if len(got) != 63 || !strings.HasPrefix(got, strings.Repeat("a", 54)+"-") {
		t.Fatalf("long: %q (len %d)", got, len(got))
	}
	if SafeName(long) != got || SafeName(long+"b") == got {
		t.Fatal("hash must be stable and depend on the full name")
	}
}

func TestGeneratedNames(t *testing.T) {
	s := svc("default", "web")
	ref := ServerRef{"prod", "sg-01"}
	if got := ClientName(s, ref, frpv1alpha1.ClientModePerService); got != "lb-default-web-sg-01" {
		t.Fatalf("client per-service: %q", got)
	}
	if got := ClientName(s, ref, frpv1alpha1.ClientModeShared); got != "pool-prod-sg-01" {
		t.Fatalf("client shared: %q", got)
	}
	if got := ClientID(s, ref, frpv1alpha1.ClientModePerService); got != "lb/default/web/sg-01" {
		t.Fatalf("clientID per-service: %q", got)
	}
	if got := ClientID(s, ref, frpv1alpha1.ClientModeShared); got != "pool/prod/sg-01" {
		t.Fatalf("clientID shared: %q", got)
	}
	if got := UpstreamName(s, ref, 443, corev1.ProtocolTCP); got != "lb-default-web-sg-01-443" {
		t.Fatalf("upstream tcp: %q", got)
	}
	if got := UpstreamName(s, ref, 53, corev1.ProtocolUDP); got != "lb-default-web-sg-01-53-udp" {
		t.Fatalf("upstream udp: %q", got)
	}
	if got := UpstreamName(s, ref, 53, corev1.ProtocolTCP); got != "lb-default-web-sg-01-53" {
		t.Fatalf("upstream tcp same port: %q", got)
	}
	if got := ServiceHost(s); got != "web.default.svc.cluster.local" {
		t.Fatalf("host: %q", got)
	}
	if got := ServiceKey(s); got != "default/web" {
		t.Fatalf("key: %q", got)
	}
}

func TestPortRanges(t *testing.T) {
	ranges, err := ParsePortRanges([]frpv1alpha1.PortRange{"443", "8000-9000"})
	if err != nil {
		t.Fatal(err)
	}
	for port, want := range map[int32]bool{443: true, 444: false, 8000: true, 8500: true, 9000: true, 9001: false} {
		if got := PortAllowed(ranges, port); got != want {
			t.Errorf("port %d: got %v want %v", port, got, want)
		}
	}
	for _, bad := range []frpv1alpha1.PortRange{"abc", "9000-8000", "70000", "0"} {
		if _, err := ParsePortRanges([]frpv1alpha1.PortRange{bad}); err == nil {
			t.Errorf("%q: expected error", bad)
		}
	}
	empty, _ := ParsePortRanges(nil)
	if len(empty) != 0 {
		t.Fatal("nil input must yield no ranges")
	}
}
