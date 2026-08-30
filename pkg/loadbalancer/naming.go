package loadbalancer

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"

	corev1 "k8s.io/api/core/v1"

	frpv1alpha1 "github.com/zufardhiyaulhaq/frp-operator/api/v1alpha1"
)

const (
	LoadBalancerClass = "frp.zufardhiyaulhaq.com/frp"

	AnnotationServerPool     = "frp.zufardhiyaulhaq.com/server-pool"
	AnnotationServer         = "frp.zufardhiyaulhaq.com/server"
	AnnotationAllocated      = "frp.zufardhiyaulhaq.com/allocated-server"
	AnnotationServiceCreated = "frp.zufardhiyaulhaq.com/service-created" // RFC3339, on generated Upstreams

	Finalizer = "frp.zufardhiyaulhaq.com/loadbalancer"

	LabelManagedBy        = "frp.zufardhiyaulhaq.com/managed-by"
	ManagedByValue        = "loadbalancer"
	LabelServiceUID       = "frp.zufardhiyaulhaq.com/service-uid"
	LabelServiceNamespace = "frp.zufardhiyaulhaq.com/service-namespace"
	LabelServiceName      = "frp.zufardhiyaulhaq.com/service-name"
	LabelPool             = "frp.zufardhiyaulhaq.com/pool"
	LabelServer           = "frp.zufardhiyaulhaq.com/server"
	LabelPort             = "frp.zufardhiyaulhaq.com/port"
	LabelProtocol         = "frp.zufardhiyaulhaq.com/protocol"

	maxNameLength = 63
)

// ServerRef identifies one server inside one pool.
type ServerRef struct {
	Pool   string
	Server string
}

func (r ServerRef) String() string { return r.Pool + "/" + r.Server }

// IsClaimed reports whether the Service is a LoadBalancer with our loadBalancerClass.
func IsClaimed(svc *corev1.Service) bool {
	return svc.Spec.Type == corev1.ServiceTypeLoadBalancer &&
		svc.Spec.LoadBalancerClass != nil && *svc.Spec.LoadBalancerClass == LoadBalancerClass
}

// ParseServerList splits a comma-separated annotation value, trimming blanks. Duplicate names are dropped, keeping first-occurrence order. Empty → nil.
func ParseServerList(s string) []string {
	var out []string
	seen := make(map[string]bool)
	for _, part := range strings.Split(s, ",") {
		if p := strings.TrimSpace(part); p != "" && !seen[p] {
			out = append(out, p)
			seen[p] = true
		}
	}
	return out
}

// ParseAllocated parses "pool/server,pool/server". Empty → nil, nil.
func ParseAllocated(s string) ([]ServerRef, error) {
	var refs []ServerRef
	for _, item := range ParseServerList(s) {
		pool, server, ok := strings.Cut(item, "/")
		if !ok || pool == "" || server == "" {
			return nil, fmt.Errorf("invalid allocated-server entry %q, want pool/server", item)
		}
		refs = append(refs, ServerRef{Pool: pool, Server: server})
	}
	return refs, nil
}

func FormatAllocated(refs []ServerRef) string {
	parts := make([]string, 0, len(refs))
	for _, r := range refs {
		parts = append(parts, r.String())
	}
	return strings.Join(parts, ",")
}

// SafeName truncates names longer than 63 characters to 54 chars + "-" + 8 hex chars of
// sha256(full name), so they stay valid object names and label values.
func SafeName(name string) string {
	if len(name) <= maxNameLength {
		return name
	}
	sum := sha256.Sum256([]byte(name))
	return name[:54] + "-" + hex.EncodeToString(sum[:])[:8]
}

func ClientName(svc *corev1.Service, ref ServerRef, mode string) string {
	if mode == frpv1alpha1.ClientModeShared {
		return SafeName("pool-" + ref.Pool + "-" + ref.Server)
	}
	return SafeName("lb-" + svc.Namespace + "-" + svc.Name + "-" + ref.Server)
}

func ClientID(svc *corev1.Service, ref ServerRef, mode string) string {
	if mode == frpv1alpha1.ClientModeShared {
		return "pool/" + ref.Pool + "/" + ref.Server
	}
	return "lb/" + svc.Namespace + "/" + svc.Name + "/" + ref.Server
}

// UpstreamName includes protocol so a TCP and a UDP upstream on the same port do not collide;
// TCP keeps the historical name, UDP gets a "-udp" suffix.
func UpstreamName(svc *corev1.Service, ref ServerRef, port int32, protocol corev1.Protocol) string {
	name := "lb-" + svc.Namespace + "-" + svc.Name + "-" + ref.Server + "-" + strconv.Itoa(int(port))
	if protocol == corev1.ProtocolUDP {
		name += "-udp"
	}
	return SafeName(name)
}

func ServiceHost(svc *corev1.Service) string {
	return svc.Name + "." + svc.Namespace + ".svc.cluster.local"
}

func ServiceKey(svc *corev1.Service) string { return svc.Namespace + "/" + svc.Name }

// ParsePortRanges converts "443" / "8000-9000" entries to inclusive [lo, hi] pairs.
func ParsePortRanges(ranges []frpv1alpha1.PortRange) ([][2]int32, error) {
	out := make([][2]int32, 0, len(ranges))
	for _, r := range ranges {
		lo, hi, isRange := strings.Cut(string(r), "-")
		if !isRange {
			hi = lo
		}
		l, err := parsePort(lo)
		if err != nil {
			return nil, fmt.Errorf("allowedPorts %q: %w", r, err)
		}
		h, err := parsePort(hi)
		if err != nil {
			return nil, fmt.Errorf("allowedPorts %q: %w", r, err)
		}
		if l > h {
			return nil, fmt.Errorf("allowedPorts %q: start greater than end", r)
		}
		out = append(out, [2]int32{l, h})
	}
	return out, nil
}

func parsePort(s string) (int32, error) {
	n, err := strconv.Atoi(s)
	if err != nil || n < 1 || n > 65535 {
		return 0, fmt.Errorf("%q is not a port in 1-65535", s)
	}
	return int32(n), nil
}

// PortAllowed reports whether port falls inside any range. An empty range list allows nothing;
// callers treat "no allowedPorts configured" as allow-all before calling this.
func PortAllowed(ranges [][2]int32, port int32) bool {
	for _, r := range ranges {
		if port >= r[0] && port <= r[1] {
			return true
		}
	}
	return false
}
