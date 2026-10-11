package frps

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	v1 "github.com/zufardhiyaulhaq/frp-operator/api/v1alpha1"
)

func ptr[T any](v T) *T { return &v }

func TestParsePortRange(t *testing.T) {
	for in, want := range map[v1.PortRange][2]int32{"443": {443, 443}, "8000-8010": {8000, 8010}, "1-65535": {1, 65535}} {
		start, end, err := ParsePortRange(in)
		if err != nil || start != want[0] || end != want[1] {
			t.Errorf("ParsePortRange(%q) = %d, %d, %v; want %v", in, start, end, err, want)
		}
	}
	for _, in := range []v1.PortRange{"0", "70000", "10-5", "abc", "1-x"} {
		if _, _, err := ParsePortRange(in); err == nil {
			t.Errorf("ParsePortRange(%q) returned no error", in)
		}
	}
}

func TestValidate_ExpandsAllowPortsByProtocol(t *testing.T) {
	spec := &v1.ServerSpec{
		Bind:       v1.ServerSpec_Bind{Port: 7000, KCPPort: ptr(int32(7000))},
		AllowPorts: &v1.ServerSpec_AllowPorts{TCP: []v1.PortRange{"8000-8002", "8001"}, UDP: []v1.PortRange{"5353"}},
	}
	p, err := Validate(spec, 50)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(p.AllowTCP, []int32{8000, 8001, 8002}) || !reflect.DeepEqual(p.AllowUDP, []int32{5353}) {
		t.Fatalf("AllowTCP = %v, AllowUDP = %v", p.AllowTCP, p.AllowUDP)
	}
	var got []string
	for _, sp := range p.ServicePorts() {
		got = append(got, fmt.Sprintf("%s:%d/%s->%d", sp.Name, sp.Port, sp.Protocol, sp.TargetPort.IntVal))
	}
	want := []string{"bind:7000/TCP->7000", "kcp:7000/UDP->7000", "tcp-8000:8000/TCP->8000", "tcp-8001:8001/TCP->8001", "tcp-8002:8002/TCP->8002", "udp-5353:5353/UDP->5353"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ServicePorts = %v, want %v", got, want)
	}
}

func TestValidate_SameNumberOnTCPAndUDPIsFine(t *testing.T) {
	spec := &v1.ServerSpec{
		Bind:       v1.ServerSpec_Bind{Port: 7000, KCPPort: ptr(int32(7000))},
		AllowPorts: &v1.ServerSpec_AllowPorts{TCP: []v1.PortRange{"5353"}, UDP: []v1.PortRange{"5353"}},
	}
	if _, err := Validate(spec, 50); err != nil {
		t.Fatal(err)
	}
}

func TestValidate_Rejects(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*v1.ServerSpec)
		want   string
	}{
		{"https on the bind port", func(s *v1.ServerSpec) { s.Vhost = &v1.ServerSpec_Vhost{HTTPSPort: ptr(int32(7000))} },
			"TCP port 7000 is used by both bind.port and vhost.httpsPort"},
		{"kcp and quic share a port", func(s *v1.ServerSpec) { s.Bind.KCPPort, s.Bind.QUICPort = ptr(int32(7001)), ptr(int32(7001)) },
			"UDP port 7001 is used by both bind.kcpPort and bind.quicPort"},
		{"allowPorts claims the web server port", func(s *v1.ServerSpec) {
			s.Dashboard = &v1.ServerSpec_Dashboard{Enabled: true}
			s.AllowPorts = &v1.ServerSpec_AllowPorts{TCP: []v1.PortRange{"7500"}}
		}, "TCP port 7500 is used by both the dashboard/metrics web server and allowPorts.tcp"},
		{"too many ports", func(s *v1.ServerSpec) { s.AllowPorts = &v1.ServerSpec_AllowPorts{TCP: []v1.PortRange{"8000-8049"}} },
			"51 ports, more than --server-max-ports=50"},
		{"bad range", func(s *v1.ServerSpec) { s.AllowPorts = &v1.ServerSpec_AllowPorts{UDP: []v1.PortRange{"9-1"}} },
			"allowPorts.udp"},
		{"NodePort", func(s *v1.ServerSpec) { s.Service = &v1.ServerSpec_Service{Type: "NodePort"} },
			`service.type "NodePort" is not supported`},
		{"the operator's own class", func(s *v1.ServerSpec) { s.Service = &v1.ServerSpec_Service{LoadBalancerClass: LoadBalancerClass} },
			"operator's own ServerPool class"},
		{"serviceMonitor without metrics", func(s *v1.ServerSpec) { s.Metrics = &v1.ServerSpec_Metrics{ServiceMonitor: true} },
			"metrics.serviceMonitor requires metrics.enabled"},
		{"no bind port", func(s *v1.ServerSpec) { s.Bind.Port = 0 }, "bind.port is required"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			spec := &v1.ServerSpec{Bind: v1.ServerSpec_Bind{Port: 7000}}
			tc.mutate(spec)
			_, err := Validate(spec, 50)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Validate() error = %v, want it to contain %q", err, tc.want)
			}
		})
	}
}

func TestAllowRanges(t *testing.T) {
	p := &Ports{AllowTCP: []int32{8000, 8001, 8002, 9000}, AllowUDP: []int32{8003, 9000}}
	if got, want := p.AllowRanges(), []PortSpan{{8000, 8003}, {9000, 9000}}; !reflect.DeepEqual(got, want) {
		t.Errorf("AllowRanges = %v, want %v", got, want)
	}
	// No ports: an empty range, because an empty allowPorts makes frps allow every port.
	if got, want := (&Ports{}).AllowRanges(), []PortSpan{{2, 1}}; !reflect.DeepEqual(got, want) {
		t.Errorf("AllowRanges = %v, want %v", got, want)
	}
}
