package loadbalancer

import (
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	frpv1alpha1 "github.com/zufardhiyaulhaq/frp-operator/api/v1alpha1"
)

func pool(name, policy string, allowed []frpv1alpha1.PortRange, servers ...frpv1alpha1.ServerPoolServer) frpv1alpha1.ServerPool {
	return frpv1alpha1.ServerPool{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "frp-operator"},
		Spec:       frpv1alpha1.ServerPoolSpec{AllocationPolicy: policy, AllowedPorts: allowed, Servers: servers},
	}
}

func server(name string, allowed ...frpv1alpha1.PortRange) frpv1alpha1.ServerPoolServer {
	return frpv1alpha1.ServerPoolServer{Name: name, Host: name + ".example", Port: 7000, PublicAddress: "1.2.3.4", AllowedPorts: allowed}
}

func tcp(port int32) corev1.ServicePort {
	return corev1.ServicePort{Name: "p", Port: port, Protocol: corev1.ProtocolTCP}
}

func alloc(pool, server string, port int32, svc string) Allocation {
	return Allocation{Pool: pool, Server: server, Port: port, Protocol: corev1.ProtocolTCP, ServiceKey: svc}
}

func TestAllocate_FirstFitDeterministic(t *testing.T) {
	pools := []frpv1alpha1.ServerPool{
		pool("zeta", frpv1alpha1.AllocationPolicyAuto, nil, server("z1")),
		pool("alpha", frpv1alpha1.AllocationPolicyAuto, nil, server("a1"), server("a2")),
	}
	req := Request{ServiceKey: "default/web", Ports: []corev1.ServicePort{tcp(443)}}
	refs, reason := Allocate(req, pools, nil)
	if reason != nil || len(refs) != 1 || refs[0] != (ServerRef{"alpha", "a1"}) {
		t.Fatalf("got %v %v", refs, reason)
	}
	// a1:443 taken → a2
	refs, _ = Allocate(req, pools, []Allocation{alloc("alpha", "a1", 443, "x/y")})
	if refs[0] != (ServerRef{"alpha", "a2"}) {
		t.Fatalf("got %v", refs)
	}
	// alpha full → zeta
	refs, _ = Allocate(req, pools, []Allocation{alloc("alpha", "a1", 443, "x/y"), alloc("alpha", "a2", 443, "x/z")})
	if refs[0] != (ServerRef{"zeta", "z1"}) {
		t.Fatalf("got %v", refs)
	}
	// everything full → NoServerAvailable
	_, reason = Allocate(req, pools, []Allocation{alloc("alpha", "a1", 443, "x/y"), alloc("alpha", "a2", 443, "x/z"), alloc("zeta", "z1", 443, "x/w")})
	if reason == nil || reason.Reason != ReasonNoServerAvailable {
		t.Fatalf("got %v", reason)
	}
}

func TestAllocate_OwnAllocationsDoNotBlock(t *testing.T) {
	pools := []frpv1alpha1.ServerPool{pool("p", frpv1alpha1.AllocationPolicyAuto, nil, server("s1"))}
	req := Request{ServiceKey: "default/web", Ports: []corev1.ServicePort{tcp(443)}}
	refs, reason := Allocate(req, pools, []Allocation{alloc("p", "s1", 443, "default/web")})
	if reason != nil || refs[0] != (ServerRef{"p", "s1"}) {
		t.Fatalf("got %v %v", refs, reason)
	}
}

func TestAllocate_MultiPortMustFitOneServer(t *testing.T) {
	pools := []frpv1alpha1.ServerPool{pool("p", frpv1alpha1.AllocationPolicyAuto, nil, server("s1"), server("s2"))}
	req := Request{ServiceKey: "default/web", Ports: []corev1.ServicePort{tcp(80), tcp(443)}}
	refs, reason := Allocate(req, pools, []Allocation{alloc("p", "s1", 443, "x/y")})
	if reason != nil || refs[0] != (ServerRef{"p", "s2"}) {
		t.Fatalf("got %v %v", refs, reason)
	}
}

func TestAllocate_ProtocolIsPartOfKey(t *testing.T) {
	pools := []frpv1alpha1.ServerPool{pool("p", frpv1alpha1.AllocationPolicyAuto, nil, server("s1"))}
	udp := corev1.ServicePort{Name: "dns", Port: 53, Protocol: corev1.ProtocolUDP}
	req := Request{ServiceKey: "default/dns", Ports: []corev1.ServicePort{udp}}
	if _, reason := Allocate(req, pools, []Allocation{alloc("p", "s1", 53, "x/y")}); reason != nil {
		t.Fatalf("TCP 53 must not block UDP 53: %v", reason)
	}
}

func TestAllocate_ExplicitPoolsSkippedUnlessNamed(t *testing.T) {
	pools := []frpv1alpha1.ServerPool{pool("prod", frpv1alpha1.AllocationPolicyExplicit, nil, server("s1"))}
	req := Request{ServiceKey: "default/web", Ports: []corev1.ServicePort{tcp(443)}}
	if _, reason := Allocate(req, pools, nil); reason == nil || reason.Reason != ReasonNoServerAvailable {
		t.Fatalf("got %v", reason)
	}
	req.Pool = "prod"
	if refs, reason := Allocate(req, pools, nil); reason != nil || refs[0] != (ServerRef{"prod", "s1"}) {
		t.Fatalf("got %v %v", refs, reason)
	}
	req.Pool = "missing"
	if _, reason := Allocate(req, pools, nil); reason == nil || reason.Reason != ReasonPoolNotFound {
		t.Fatalf("got %v", reason)
	}
}

func TestAllocate_PinnedServers(t *testing.T) {
	pools := []frpv1alpha1.ServerPool{
		pool("a", frpv1alpha1.AllocationPolicyAuto, nil, server("sg-01"), server("sg-02")),
		pool("b", frpv1alpha1.AllocationPolicyAuto, nil, server("sg-01")),
		pool("x", frpv1alpha1.AllocationPolicyExplicit, nil, server("sg-09")),
	}
	base := Request{ServiceKey: "default/web", Ports: []corev1.ServicePort{tcp(443)}}

	// unambiguous: only pool a has both
	req := base
	req.Servers = []string{"sg-01", "sg-02"}
	refs, reason := Allocate(req, pools, nil)
	if reason != nil || len(refs) != 2 || refs[0] != (ServerRef{"a", "sg-01"}) || refs[1] != (ServerRef{"a", "sg-02"}) {
		t.Fatalf("got %v %v", refs, reason)
	}
	// ambiguous: sg-01 in a and b
	req.Servers = []string{"sg-01"}
	if _, reason := Allocate(req, pools, nil); reason == nil || reason.Reason != ReasonAmbiguousServer {
		t.Fatalf("got %v", reason)
	}
	// resolved by pool annotation
	req.Pool = "b"
	if refs, reason := Allocate(req, pools, nil); reason != nil || refs[0] != (ServerRef{"b", "sg-01"}) {
		t.Fatalf("got %v %v", refs, reason)
	}
	// explicit pool server without pool annotation → not found
	req = base
	req.Servers = []string{"sg-09"}
	if _, reason := Allocate(req, pools, nil); reason == nil || reason.Reason != ReasonServerNotFound {
		t.Fatalf("got %v", reason)
	}
	// named pool lacks the server
	req.Pool = "a"
	if _, reason := Allocate(req, pools, nil); reason == nil || reason.Reason != ReasonServerNotFound {
		t.Fatalf("got %v", reason)
	}
	// all-or-nothing: one of two pinned servers busy
	req = base
	req.Servers = []string{"sg-01", "sg-02"}
	_, reason = Allocate(req, pools, []Allocation{alloc("a", "sg-02", 443, "x/y")})
	if reason == nil || reason.Reason != ReasonPortUnavailable {
		t.Fatalf("got %v", reason)
	}
}

func TestAllocate_AllowedPorts(t *testing.T) {
	pools := []frpv1alpha1.ServerPool{pool("p", frpv1alpha1.AllocationPolicyAuto, []frpv1alpha1.PortRange{"443"},
		server("web"), server("any", "1-65535"))}
	req := Request{ServiceKey: "default/x", Ports: []corev1.ServicePort{tcp(8080)}}
	refs, reason := Allocate(req, pools, nil)
	if reason != nil || refs[0] != (ServerRef{"p", "any"}) {
		t.Fatalf("pool default must exclude web: %v %v", refs, reason)
	}
	req.Servers = []string{"web"}
	if _, reason := Allocate(req, pools, nil); reason == nil || reason.Reason != ReasonPortNotAllowed {
		t.Fatalf("pinned: got %v", reason)
	}
}

func TestAllocate_UnsupportedProtocol(t *testing.T) {
	pools := []frpv1alpha1.ServerPool{pool("p", frpv1alpha1.AllocationPolicyAuto, nil, server("s1"))}
	req := Request{ServiceKey: "default/x", Ports: []corev1.ServicePort{{Port: 1, Protocol: corev1.ProtocolSCTP}}}
	if _, reason := Allocate(req, pools, nil); reason == nil || reason.Reason != ReasonUnsupportedProtocol {
		t.Fatalf("got %v", reason)
	}
}

func TestWinner(t *testing.T) {
	old := Allocation{ServiceKey: "b/b", ServiceCreated: time.Unix(100, 0)}
	young := Allocation{ServiceKey: "a/a", ServiceCreated: time.Unix(200, 0)}
	if Winner(old, young).ServiceKey != "b/b" || Winner(young, old).ServiceKey != "b/b" {
		t.Fatal("older service must win")
	}
	tie := Allocation{ServiceKey: "a/a", ServiceCreated: time.Unix(100, 0)}
	if Winner(old, tie).ServiceKey != "a/a" {
		t.Fatal("ties break on smaller key")
	}
}

func TestAllocate_PinnedDuplicatesCollapse(t *testing.T) {
	pools := []frpv1alpha1.ServerPool{pool("p", frpv1alpha1.AllocationPolicyAuto, nil, server("sg-01"))}
	req := Request{ServiceKey: "default/web", Ports: []corev1.ServicePort{tcp(443)}, Servers: ParseServerList("sg-01,sg-01")}
	refs, reason := Allocate(req, pools, nil)
	if reason != nil || len(refs) != 1 || refs[0] != (ServerRef{"p", "sg-01"}) {
		t.Fatalf("got %v %v", refs, reason)
	}
}

func TestAllocate_EmptyPolicyIsAuto(t *testing.T) {
	pools := []frpv1alpha1.ServerPool{pool("p", "", nil, server("s1"))}
	req := Request{ServiceKey: "default/web", Ports: []corev1.ServicePort{tcp(443)}}
	refs, reason := Allocate(req, pools, nil)
	if reason != nil || len(refs) != 1 || refs[0] != (ServerRef{"p", "s1"}) {
		t.Fatalf("got %v %v", refs, reason)
	}
}
