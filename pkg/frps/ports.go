// Package frps builds what the operator runs for a Server: frps.toml, the frps pod and its
// Services. Only Resolve talks to the API server (it reads Secrets); everything else is pure.
package frps

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	v1 "github.com/zufardhiyaulhaq/frp-operator/api/v1alpha1"
)

// WebServerPort is where frps serves the dashboard, admin API and /metrics inside the pod.
const WebServerPort int32 = 7500

// LoadBalancerClass is the class the operator's own LoadBalancer controller claims. A Server's
// Service must not use it, or the operator would try to back its own frps with a ServerPool.
const LoadBalancerClass = "frp.zufardhiyaulhaq.com/frp"

// Ports is a Server's validated port layout. A zero port is off.
type Ports struct {
	Bind, KCP, QUIC     int32
	HTTP, HTTPS, TCPMux int32
	AllowTCP, AllowUDP  []int32 // sorted, unique
	WebServer           bool    // dashboard or metrics: frps listens on WebServerPort
}

// PortSpan is one entry of frps's allowPorts.
type PortSpan struct{ Start, End int32 }

// ParsePortRange parses "443" or "8000-8010".
func ParsePortRange(r v1.PortRange) (int32, int32, error) {
	first, last, isRange := strings.Cut(string(r), "-")
	start, err := strconv.Atoi(first)
	if err != nil {
		return 0, 0, fmt.Errorf("port range %q: %w", r, err)
	}
	end := start
	if isRange {
		if end, err = strconv.Atoi(last); err != nil {
			return 0, 0, fmt.Errorf("port range %q: %w", r, err)
		}
	}
	if start < 1 || end > 65535 || start > end {
		return 0, 0, fmt.Errorf("port range %q must be within 1-65535 with start <= end", r)
	}
	return int32(start), int32(end), nil
}

func expand(field string, ranges []v1.PortRange) ([]int32, error) {
	var ports []int32
	for _, r := range ranges {
		start, end, err := ParsePortRange(r)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", field, err)
		}
		for p := start; p <= end; p++ {
			ports = append(ports, p)
		}
	}
	slices.Sort(ports)
	return slices.Compact(ports), nil
}

// WebServerEnabled reports whether frps runs its web server (dashboard or metrics).
func WebServerEnabled(spec *v1.ServerSpec) bool {
	return (spec.Dashboard != nil && spec.Dashboard.Enabled) || (spec.Metrics != nil && spec.Metrics.Enabled)
}

// Validate checks a Server spec and returns its port layout. maxPorts caps the public Service.
func Validate(spec *v1.ServerSpec, maxPorts int) (*Ports, error) {
	if s := spec.Service; s != nil {
		if s.Type != "" && s.Type != string(corev1.ServiceTypeLoadBalancer) && s.Type != string(corev1.ServiceTypeClusterIP) {
			return nil, fmt.Errorf("service.type %q is not supported: use LoadBalancer or ClusterIP (NodePort remaps ports, and frps opens each remote port itself)", s.Type)
		}
		if s.LoadBalancerClass == LoadBalancerClass {
			return nil, fmt.Errorf("service.loadBalancerClass %q is the operator's own ServerPool class", s.LoadBalancerClass)
		}
	}
	if m := spec.Metrics; m != nil && m.ServiceMonitor && !m.Enabled {
		return nil, fmt.Errorf("metrics.serviceMonitor requires metrics.enabled")
	}
	if _, err := spec.Log.LevelOrDefault(); err != nil {
		return nil, err
	}

	p := &Ports{Bind: spec.Bind.Port, KCP: deref(spec.Bind.KCPPort), QUIC: deref(spec.Bind.QUICPort), WebServer: WebServerEnabled(spec)}
	if p.Bind == 0 {
		return nil, fmt.Errorf("bind.port is required")
	}
	if v := spec.Vhost; v != nil {
		p.HTTP, p.HTTPS, p.TCPMux = deref(v.HTTPPort), deref(v.HTTPSPort), deref(v.TCPMuxPort)
	}
	if a := spec.AllowPorts; a != nil {
		var err error
		if p.AllowTCP, err = expand("allowPorts.tcp", a.TCP); err != nil {
			return nil, err
		}
		if p.AllowUDP, err = expand("allowPorts.udp", a.UDP); err != nil {
			return nil, err
		}
	}

	tcp := []portUse{{p.Bind, "bind.port"}, {p.HTTP, "vhost.httpPort"}, {p.HTTPS, "vhost.httpsPort"}, {p.TCPMux, "vhost.tcpmuxPort"}}
	if p.WebServer {
		tcp = append(tcp, portUse{WebServerPort, "the dashboard/metrics web server"})
	}
	for _, port := range p.AllowTCP {
		tcp = append(tcp, portUse{port, "allowPorts.tcp"})
	}
	udp := []portUse{{p.KCP, "bind.kcpPort"}, {p.QUIC, "bind.quicPort"}}
	for _, port := range p.AllowUDP {
		udp = append(udp, portUse{port, "allowPorts.udp"})
	}
	if err := noOverlap("TCP", tcp); err != nil {
		return nil, err
	}
	if err := noOverlap("UDP", udp); err != nil {
		return nil, err
	}

	if n := len(p.ServicePorts()); n > maxPorts {
		return nil, fmt.Errorf("the public Service would have %d ports, more than --server-max-ports=%d", n, maxPorts)
	}
	return p, nil
}

type portUse struct {
	port int32
	what string
}

// noOverlap rejects a port used twice within one protocol; TCP and UDP are separate in frps.
func noOverlap(proto string, uses []portUse) error {
	seen := map[int32]string{}
	for _, u := range uses {
		if u.port == 0 {
			continue
		}
		if u.port < 1 || u.port > 65535 {
			return fmt.Errorf("%s %d is outside 1-65535", u.what, u.port)
		}
		if prev, ok := seen[u.port]; ok {
			return fmt.Errorf("%s port %d is used by both %s and %s", proto, u.port, prev, u.what)
		}
		seen[u.port] = u.what
	}
	return nil
}

// ServicePorts are the public Service's ports, each with its own protocol.
func (p *Ports) ServicePorts() []corev1.ServicePort {
	var out []corev1.ServicePort
	add := func(name string, port int32, proto corev1.Protocol) {
		if port != 0 {
			out = append(out, corev1.ServicePort{Name: name, Port: port, TargetPort: intstr.FromInt32(port), Protocol: proto})
		}
	}
	add("bind", p.Bind, corev1.ProtocolTCP)
	add("kcp", p.KCP, corev1.ProtocolUDP)
	add("quic", p.QUIC, corev1.ProtocolUDP)
	add("http", p.HTTP, corev1.ProtocolTCP)
	add("https", p.HTTPS, corev1.ProtocolTCP)
	add("tcpmux", p.TCPMux, corev1.ProtocolTCP)
	for _, port := range p.AllowTCP {
		add(fmt.Sprintf("tcp-%d", port), port, corev1.ProtocolTCP)
	}
	for _, port := range p.AllowUDP {
		add(fmt.Sprintf("udp-%d", port), port, corev1.ProtocolUDP)
	}
	return out
}

// AllowRanges is frps's allowPorts: the union of the TCP and UDP ports, merged into spans. With
// no ports it is the empty span {2, 1}, because an empty allowPorts makes frps allow every port.
func (p *Ports) AllowRanges() []PortSpan {
	ports := slices.Concat(p.AllowTCP, p.AllowUDP)
	slices.Sort(ports)
	ports = slices.Compact(ports)
	if len(ports) == 0 {
		return []PortSpan{{Start: 2, End: 1}}
	}
	spans := []PortSpan{{Start: ports[0], End: ports[0]}}
	for _, port := range ports[1:] {
		if last := &spans[len(spans)-1]; port == last.End+1 {
			last.End = port
		} else {
			spans = append(spans, PortSpan{Start: port, End: port})
		}
	}
	return spans
}

func deref(p *int32) int32 {
	if p == nil {
		return 0
	}
	return *p
}
