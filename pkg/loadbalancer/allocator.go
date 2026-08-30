package loadbalancer

import (
	"fmt"
	"sort"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"

	frpv1alpha1 "github.com/zufardhiyaulhaq/frp-operator/api/v1alpha1"
)

const (
	ReasonPortUnavailable     = "PortUnavailable"
	ReasonPortNotAllowed      = "PortNotAllowed"
	ReasonServerNotFound      = "ServerNotFound"
	ReasonPoolNotFound        = "PoolNotFound"
	ReasonAmbiguousServer     = "AmbiguousServer"
	ReasonNoServerAvailable   = "NoServerAvailable"
	ReasonUnsupportedProtocol = "UnsupportedProtocol"
)

// Request is what a Service asks for.
type Request struct {
	ServiceKey string // "<namespace>/<name>"
	Ports      []corev1.ServicePort
	Pool       string   // server-pool annotation, "" when absent
	Servers    []string // server annotation, nil when absent
}

// Allocation is one port held on one server by one Service (derived from a generated Upstream).
type Allocation struct {
	Pool           string
	Server         string
	Port           int32
	Protocol       corev1.Protocol
	ServiceKey     string
	ServiceCreated time.Time
}

// Reason explains why a Service cannot be bound; Reason is used as the Kubernetes Event reason.
type Reason struct {
	Reason  string
	Message string
}

func (r *Reason) Error() string { return r.Reason + ": " + r.Message }

func FindPool(pools []frpv1alpha1.ServerPool, name string) *frpv1alpha1.ServerPool {
	for i := range pools {
		if pools[i].Name == name {
			return &pools[i]
		}
	}
	return nil
}

func FindServer(pool frpv1alpha1.ServerPool, name string) *frpv1alpha1.ServerPoolServer {
	for i := range pool.Spec.Servers {
		if pool.Spec.Servers[i].Name == name {
			return &pool.Spec.Servers[i]
		}
	}
	return nil
}

// ValidateProtocols reports the first port in req that frpc cannot proxy (anything but TCP/UDP).
// Shared by Allocate and by callers that must reject an unsupported protocol before even
// considering the sticky (already-bound) path.
func ValidateProtocols(req Request) *Reason {
	for _, p := range req.Ports {
		if p.Protocol != corev1.ProtocolTCP && p.Protocol != corev1.ProtocolUDP && p.Protocol != "" {
			return &Reason{ReasonUnsupportedProtocol, fmt.Sprintf("port %d uses protocol %s; only TCP and UDP are supported", p.Port, p.Protocol)}
		}
	}
	return nil
}

// Allocate selects servers for req. See the spec's "Selection" section.
func Allocate(req Request, pools []frpv1alpha1.ServerPool, allocs []Allocation) ([]ServerRef, *Reason) {
	if reason := ValidateProtocols(req); reason != nil {
		return nil, reason
	}

	sorted := append([]frpv1alpha1.ServerPool(nil), pools...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Name < sorted[j].Name })

	var candidates []frpv1alpha1.ServerPool
	if req.Pool != "" {
		p := FindPool(sorted, req.Pool)
		if p == nil {
			return nil, &Reason{ReasonPoolNotFound, fmt.Sprintf("ServerPool %q not found", req.Pool)}
		}
		candidates = []frpv1alpha1.ServerPool{*p}
	} else {
		for _, p := range sorted {
			if p.Spec.AllocationPolicy == "" || p.Spec.AllocationPolicy == frpv1alpha1.AllocationPolicyAuto {
				candidates = append(candidates, p)
			}
		}
	}

	if len(req.Servers) > 0 {
		return allocatePinned(req, candidates, allocs)
	}

	for _, p := range candidates {
		for _, s := range p.Spec.Servers {
			if Fit(req, p, s, allocs) == nil {
				return []ServerRef{{Pool: p.Name, Server: s.Name}}, nil
			}
		}
	}
	names := make([]string, 0, len(candidates))
	for _, p := range candidates {
		names = append(names, p.Name)
	}
	return nil, &Reason{ReasonNoServerAvailable, fmt.Sprintf("no server in pools [%s] can provide ports %s", strings.Join(names, ", "), portList(req.Ports))}
}

func allocatePinned(req Request, candidates []frpv1alpha1.ServerPool, allocs []Allocation) ([]ServerRef, *Reason) {
	var matches []frpv1alpha1.ServerPool
	for _, p := range candidates {
		all := true
		for _, name := range req.Servers {
			if FindServer(p, name) == nil {
				all = false
				break
			}
		}
		if all {
			matches = append(matches, p)
		}
	}
	switch {
	case len(matches) == 0 && req.Pool != "":
		return nil, &Reason{ReasonServerNotFound, fmt.Sprintf("servers [%s] are not all present in ServerPool %q", strings.Join(req.Servers, ", "), req.Pool)}
	case len(matches) == 0:
		return nil, &Reason{ReasonServerNotFound, fmt.Sprintf("servers [%s] not found together in any Auto ServerPool (set the server-pool annotation if they are in an Explicit pool)", strings.Join(req.Servers, ", "))}
	case len(matches) > 1:
		names := make([]string, 0, len(matches))
		for _, p := range matches {
			names = append(names, p.Name)
		}
		return nil, &Reason{ReasonAmbiguousServer, fmt.Sprintf("servers [%s] exist in pools [%s]; set the server-pool annotation", strings.Join(req.Servers, ", "), strings.Join(names, ", "))}
	}
	p := matches[0]
	refs := make([]ServerRef, 0, len(req.Servers))
	for _, name := range req.Servers {
		if r := Fit(req, p, *FindServer(p, name), allocs); r != nil {
			return nil, r
		}
		refs = append(refs, ServerRef{Pool: p.Name, Server: name})
	}
	return refs, nil
}

// Fit reports why server cannot provide every port in req, or nil when it can.
// Allocations owned by req.ServiceKey itself never block.
func Fit(req Request, pool frpv1alpha1.ServerPool, server frpv1alpha1.ServerPoolServer, allocs []Allocation) *Reason {
	allowed := server.AllowedPorts
	if len(allowed) == 0 {
		allowed = pool.Spec.AllowedPorts
	}
	ranges, err := ParsePortRanges(allowed)
	if err != nil {
		return &Reason{ReasonPortNotAllowed, fmt.Sprintf("%s/%s: %v", pool.Name, server.Name, err)}
	}
	ref := ServerRef{Pool: pool.Name, Server: server.Name}
	for _, p := range req.Ports {
		if len(ranges) > 0 && !PortAllowed(ranges, p.Port) {
			return &Reason{ReasonPortNotAllowed, fmt.Sprintf("port %d is not in allowedPorts of %s", p.Port, ref)}
		}
		proto := p.Protocol
		if proto == "" {
			proto = corev1.ProtocolTCP
		}
		for _, a := range allocs {
			if a.Pool == pool.Name && a.Server == server.Name && a.Port == p.Port && a.Protocol == proto && a.ServiceKey != req.ServiceKey {
				return &Reason{ReasonPortUnavailable, fmt.Sprintf("port %d/%s already allocated on %s by %s", p.Port, proto, ref, a.ServiceKey)}
			}
		}
	}
	return nil
}

// Winner decides which of two Services racing for the same port keeps it.
func Winner(a, b Allocation) Allocation {
	if a.ServiceCreated.Before(b.ServiceCreated) {
		return a
	}
	if b.ServiceCreated.Before(a.ServiceCreated) {
		return b
	}
	if a.ServiceKey < b.ServiceKey {
		return a
	}
	return b
}

func portList(ports []corev1.ServicePort) string {
	parts := make([]string, 0, len(ports))
	for _, p := range ports {
		parts = append(parts, fmt.Sprintf("%d/%s", p.Port, p.Protocol))
	}
	return "[" + strings.Join(parts, ", ") + "]"
}
