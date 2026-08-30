{% raw %}
# LoadBalancer Services backed by FRP — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A `Service type=LoadBalancer` with `loadBalancerClass: frp.zufardhiyaulhaq.com/frp` is bound to one (or, when pinned, several) frps server(s) from a `ServerPool`; the operator generates the `Client`/`Upstream` CRs and writes the server's public address to `status.loadBalancer.ingress`.

**Architecture:** New namespaced `ServerPool` CRD in the operator namespace. A pure-Go allocator (`pkg/loadbalancer`) picks servers deterministically from pools using existing generated `Upstream`s (labelled) as the allocation record. A new `ServiceReconciler` owns claimed Services via a finalizer, generates `Client`/`Upstream` objects in the operator namespace, and gates the ingress entry on the generated Client being `Ready` with every proxy `running`. `ClientReconciler` is not modified.

**Tech Stack:** Go 1.23, controller-runtime v0.18.4, k8s.io/api v0.30.1, prometheus/client_golang v1.16, Helm chart, controller-gen (`make manifests generate`).

**Spec:** `docs/superpowers/specs/2026-08-30-loadbalancer-service-design.md`

## Global Constraints

- Run Go with `PATH=$HOME/.asdf/installs/golang/1.23.12/go/bin:$PATH` in front of every `go`/`make` command.
- **Never run `make lint` / golangci-lint** (OOMs on this machine). Use `go build ./... && go vet ./... && gofmt -l .` instead.
- **Do not commit.** The user commits. Every "Commit" step below is replaced by: run `git status --short` and confirm only the expected files changed. Leave the working tree dirty.
- `loadBalancerClass` value is exactly `frp.zufardhiyaulhaq.com/frp`.
- Annotations: `frp.zufardhiyaulhaq.com/server-pool`, `frp.zufardhiyaulhaq.com/server`, `frp.zufardhiyaulhaq.com/allocated-server`. Finalizer: `frp.zufardhiyaulhaq.com/loadbalancer`. Generated-object label `frp.zufardhiyaulhaq.com/managed-by: loadbalancer`.
- `allowedPorts` entries match `^\d+(-\d+)?$`.
- Generated names: `lb-<svc-ns>-<svc-name>-<server>` (Client, PerService), `pool-<pool>-<server>` (Client, Shared), `lb-<svc-ns>-<svc-name>-<server>-<port>` (Upstream); anything > 63 chars is truncated to 54 chars + `-` + first 8 hex chars of sha256 of the full name.
- Service port `TCP` → TCP upstream, `UDP` → UDP upstream, anything else → `UnsupportedProtocol`; `remotePort` = Service port; Upstream `host` = `<svc>.<ns>.svc.cluster.local`.
- Selection: pools sorted by name, servers in spec order, first fit; one server unless `server` annotation lists several; all-or-nothing for pinned lists; no fallback from an `Explicit` pool.
- Ingress is written only when the generated Client has condition `Ready=True` **and** `handler.Status` reports every Upstream of that Service on that server as `running`.
- Chart: version `1.9.0`, appVersion `0.11.0`, `operator.tag: v0.11.0`; CRDs synced into `charts/frp-operator/crds/crds.yaml` (append a fourth document).
- Release notes in `docs/releases/v0.11.0.md`; docs under `docs/superpowers/**` containing `{{`/`{%` must be wrapped in `{% raw %}` … `{% endraw %}`.

---

## File map

| File | Responsibility |
|---|---|
| `api/v1alpha1/serverpool_types.go` | `ServerPool` CRD types |
| `pkg/loadbalancer/naming.go` | constants, annotation parse/format, generated names, port ranges |
| `pkg/loadbalancer/allocator.go` | pure selection logic (`Allocate`, `Fit`, `Winner`) |
| `pkg/loadbalancer/builder.go` | `ServerPoolServer` + Service port → `Client` / `Upstream` objects |
| `controllers/service_controller.go` | `ServiceReconciler`: watches, finalizer, apply/delete, status, Events |
| `pkg/metrics/loadbalancer.go` | `frp_serverpool_allocated_ports`, `frp_loadbalancer_service` |
| `main.go` | flags `--enable-loadbalancer-controller`, `--operator-namespace` |
| `charts/frp-operator/…` | CRD, RBAC, env, values, version |
| `examples/loadbalancer/` | pool + three Services |
| docs | README section, AGENTS.md, release notes, dashboard row |

---

### Task 1: ServerPool CRD

**Files:**
- Create: `api/v1alpha1/serverpool_types.go`
- Generated: `api/v1alpha1/zz_generated.deepcopy.go`, `config/crd/bases/frp.zufardhiyaulhaq.com_serverpools.yaml`, `config/rbac/role.yaml`
- Modify: `charts/frp-operator/crds/crds.yaml` (append)

**Interfaces:**
- Produces: `frpv1alpha1.ServerPool`, `ServerPoolSpec{AllocationPolicy, ClientMode string; AllowedPorts []PortRange; Servers []ServerPoolServer; ClientTemplate *ServerPoolClientTemplate}`, `ServerPoolServer{Name, Host string; Port int; PublicAddress string; AllowedPorts []PortRange; TransportProtocol *string; Authentication ClientSpec_Server_Authentication; AdminServer *ClientSpec_Server_AdminServer; TLS *ClientSpec_Server_TLS; Transport *ClientSpec_Server_Transport}`, `ServerPoolStatus{Servers []ServerPoolServerStatus; Conditions []metav1.Condition}`, `ServerPoolServerStatus{Name string; AllocatedPorts []AllocatedPort}`, `AllocatedPort{Port int32; Protocol string; Service string}`, `PortRange string`.

- [ ] **Step 1: Write the types**

```go
// api/v1alpha1/serverpool_types.go
package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PortRange is a single port ("443") or an inclusive range ("8000-9000").
// +kubebuilder:validation:Pattern=`^\d+(-\d+)?$`
type PortRange string

const (
	AllocationPolicyAuto     = "Auto"
	AllocationPolicyExplicit = "Explicit"
	ClientModePerService     = "PerService"
	ClientModeShared         = "Shared"
)

// ServerPoolSpec defines a set of frps servers that LoadBalancer Services can be bound to.
type ServerPoolSpec struct {
	// +kubebuilder:validation:Enum=Auto;Explicit
	// +kubebuilder:default=Auto
	// +optional
	// AllocationPolicy: Auto pools are eligible for Services without a server-pool annotation;
	// Explicit pools are used only when a Service names them.
	AllocationPolicy string `json:"allocationPolicy,omitempty"`
	// +kubebuilder:validation:Enum=PerService;Shared
	// +kubebuilder:default=PerService
	// +optional
	// ClientMode: PerService creates one frpc Client per Service per server; Shared creates one
	// Client per server shared by every Service bound to it.
	ClientMode string `json:"clientMode,omitempty"`
	// +optional
	// AllowedPorts restricts which Service ports may be claimed on servers of this pool.
	// Empty means any port. A server's own allowedPorts replaces this list.
	AllowedPorts []PortRange `json:"allowedPorts,omitempty"`
	// +kubebuilder:validation:MinItems=1
	Servers []ServerPoolServer `json:"servers"`
	// +optional
	// ClientTemplate is applied to every Client generated from this pool.
	ClientTemplate *ServerPoolClientTemplate `json:"clientTemplate,omitempty"`
}

// ServerPoolServer is one frps server. Fields mirror Client.spec.server.
type ServerPoolServer struct {
	// +kubebuilder:validation:MinLength=1
	// Name is the stable key used in annotations and generated resource names.
	Name string `json:"name"`
	// Host is the address frpc dials.
	Host string `json:"host"`
	Port int    `json:"port"`
	// +kubebuilder:validation:MinLength=1
	// PublicAddress (IP or hostname) is written to Service status.loadBalancer.ingress.
	PublicAddress string `json:"publicAddress"`
	// +optional
	// AllowedPorts replaces the pool-level allowedPorts for this server.
	AllowedPorts []PortRange `json:"allowedPorts,omitempty"`
	// +kubebuilder:validation:Enum=tcp;kcp;quic;websocket;wss
	// +optional
	// TransportProtocol is the frpc→frps transport (Client.spec.server.protocol).
	TransportProtocol *string `json:"transportProtocol,omitempty"`
	Authentication    ClientSpec_Server_Authentication `json:"authentication"`
	// +optional
	AdminServer *ClientSpec_Server_AdminServer `json:"adminServer,omitempty"`
	// +optional
	TLS *ClientSpec_Server_TLS `json:"tls,omitempty"`
	// +optional
	Transport *ClientSpec_Server_Transport `json:"transport,omitempty"`
}

type ServerPoolClientTemplate struct {
	// +optional
	PodTemplate *ClientSpec_PodTemplate `json:"podTemplate,omitempty"`
}

type AllocatedPort struct {
	Port     int32  `json:"port"`
	Protocol string `json:"protocol"`
	// Service is "<namespace>/<name>" of the bound Service.
	Service string `json:"service"`
}

type ServerPoolServerStatus struct {
	Name string `json:"name"`
	// +optional
	AllocatedPorts []AllocatedPort `json:"allocatedPorts,omitempty"`
}

// ServerPoolStatus is a projection of current allocations; generated Upstreams are the source of truth.
type ServerPoolStatus struct {
	// +optional
	Servers []ServerPoolServerStatus `json:"servers,omitempty"`
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:printcolumn:name="Policy",type=string,JSONPath=`.spec.allocationPolicy`
//+kubebuilder:printcolumn:name="Mode",type=string,JSONPath=`.spec.clientMode`
//+kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// ServerPool is the Schema for the serverpools API
type ServerPool struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ServerPoolSpec   `json:"spec,omitempty"`
	Status ServerPoolStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// ServerPoolList contains a list of ServerPool
type ServerPoolList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ServerPool `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ServerPool{}, &ServerPoolList{})
}
```

- [ ] **Step 2: Generate**

Run: `PATH=$HOME/.asdf/installs/golang/1.23.12/go/bin:$PATH make generate manifests`
Expected: `api/v1alpha1/zz_generated.deepcopy.go` gains `ServerPool*` DeepCopy funcs; `config/crd/bases/frp.zufardhiyaulhaq.com_serverpools.yaml` exists with `pattern: ^\d+(-\d+)?$` under `allowedPorts.items`.

- [ ] **Step 3: Sync CRD into the chart**

Run: `printf '\n' >> charts/frp-operator/crds/crds.yaml && cat config/crd/bases/frp.zufardhiyaulhaq.com_serverpools.yaml >> charts/frp-operator/crds/crds.yaml`
Then check: `grep -c '^---' charts/frp-operator/crds/crds.yaml` → `4`; `grep -n 'name: serverpools.frp' charts/frp-operator/crds/crds.yaml` → one hit. Ensure no blank-line duplication of `---` (the generated file starts with `---`).

- [ ] **Step 4: Build**

Run: `PATH=$HOME/.asdf/installs/golang/1.23.12/go/bin:$PATH go build ./... && go vet ./api/...`
Expected: no output.

- [ ] **Step 5: Confirm changed files**

`git status --short` shows: `api/v1alpha1/serverpool_types.go` (new), `zz_generated.deepcopy.go`, `config/crd/bases/frp.zufardhiyaulhaq.com_serverpools.yaml` (new), `config/rbac/role.yaml` (may be unchanged until Task 5), `charts/frp-operator/crds/crds.yaml`.

---

### Task 2: Naming, annotations, port ranges (`pkg/loadbalancer/naming.go`)

**Files:**
- Create: `pkg/loadbalancer/naming.go`, `pkg/loadbalancer/naming_test.go`

**Interfaces:**
- Produces (all in package `loadbalancer`):
  - constants `LoadBalancerClass`, `AnnotationServerPool`, `AnnotationServer`, `AnnotationAllocated`, `AnnotationServiceCreated`, `Finalizer`, `LabelManagedBy`, `ManagedByValue`, `LabelServiceUID`, `LabelServiceNamespace`, `LabelServiceName`, `LabelPool`, `LabelServer`, `LabelPort`, `LabelProtocol`
  - `type ServerRef struct{ Pool, Server string }`; `func (r ServerRef) String() string` → `pool/server`
  - `func IsClaimed(svc *corev1.Service) bool`
  - `func ParseServerList(s string) []string`
  - `func ParseAllocated(s string) ([]ServerRef, error)`; `func FormatAllocated(refs []ServerRef) string`
  - `func SafeName(name string) string`
  - `func ClientName(svc *corev1.Service, ref ServerRef, mode string) string`
  - `func ClientID(svc *corev1.Service, ref ServerRef, mode string) string`
  - `func UpstreamName(svc *corev1.Service, ref ServerRef, port int32) string`
  - `func ServiceHost(svc *corev1.Service) string`
  - `func ServiceKey(svc *corev1.Service) string` → `ns/name`
  - `func ParsePortRanges(ranges []frpv1alpha1.PortRange) ([][2]int32, error)`; `func PortAllowed(ranges [][2]int32, port int32) bool`

- [ ] **Step 1: Write the failing tests**

```go
// pkg/loadbalancer/naming_test.go
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
	if got := UpstreamName(s, ref, 443); got != "lb-default-web-sg-01-443" {
		t.Fatalf("upstream: %q", got)
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
```

- [ ] **Step 2: Run to verify failure**

Run: `PATH=$HOME/.asdf/installs/golang/1.23.12/go/bin:$PATH go test ./pkg/loadbalancer/`
Expected: build failure (undefined symbols).

- [ ] **Step 3: Implement**

```go
// pkg/loadbalancer/naming.go
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

// ParseServerList splits a comma-separated annotation value, trimming blanks. Empty → nil.
func ParseServerList(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		if p := strings.TrimSpace(part); p != "" {
			out = append(out, p)
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

func UpstreamName(svc *corev1.Service, ref ServerRef, port int32) string {
	return SafeName("lb-" + svc.Namespace + "-" + svc.Name + "-" + ref.Server + "-" + strconv.Itoa(int(port)))
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
```

- [ ] **Step 4: Run tests**

Run: `PATH=$HOME/.asdf/installs/golang/1.23.12/go/bin:$PATH go test ./pkg/loadbalancer/ -v`
Expected: all PASS.

- [ ] **Step 5: Confirm changed files** — `git status --short` shows only the two new files.

---

### Task 3: Allocator (`pkg/loadbalancer/allocator.go`)

**Files:**
- Create: `pkg/loadbalancer/allocator.go`, `pkg/loadbalancer/allocator_test.go`

**Interfaces:**
- Consumes: Task 2 (`ServerRef`, `ParsePortRanges`, `PortAllowed`), Task 1 types.
- Produces:
  - `type Request struct{ ServiceKey string; Ports []corev1.ServicePort; Pool string; Servers []string }`
  - `type Allocation struct{ Pool, Server string; Port int32; Protocol corev1.Protocol; ServiceKey string; ServiceCreated time.Time }`
  - `type Reason struct{ Reason, Message string }` (implements `error`)
  - reason constants `ReasonPortUnavailable = "PortUnavailable"`, `ReasonPortNotAllowed = "PortNotAllowed"`, `ReasonServerNotFound = "ServerNotFound"`, `ReasonPoolNotFound = "PoolNotFound"`, `ReasonAmbiguousServer = "AmbiguousServer"`, `ReasonNoServerAvailable = "NoServerAvailable"`, `ReasonUnsupportedProtocol = "UnsupportedProtocol"`
  - `func Allocate(req Request, pools []frpv1alpha1.ServerPool, allocs []Allocation) ([]ServerRef, *Reason)`
  - `func Fit(req Request, pool frpv1alpha1.ServerPool, server frpv1alpha1.ServerPoolServer, allocs []Allocation) *Reason`
  - `func FindServer(pool frpv1alpha1.ServerPool, name string) *frpv1alpha1.ServerPoolServer`
  - `func FindPool(pools []frpv1alpha1.ServerPool, name string) *frpv1alpha1.ServerPool`
  - `func Winner(a, b Allocation) Allocation` — older `ServiceCreated`, then smaller `ServiceKey`.

- [ ] **Step 1: Write the failing tests**

```go
// pkg/loadbalancer/allocator_test.go
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
```

- [ ] **Step 2: Run to verify failure** — `go test ./pkg/loadbalancer/` → undefined `Allocate` etc.

- [ ] **Step 3: Implement**

```go
// pkg/loadbalancer/allocator.go
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

// Allocate selects servers for req. See the spec's "Selection" section.
func Allocate(req Request, pools []frpv1alpha1.ServerPool, allocs []Allocation) ([]ServerRef, *Reason) {
	for _, p := range req.Ports {
		if p.Protocol != corev1.ProtocolTCP && p.Protocol != corev1.ProtocolUDP && p.Protocol != "" {
			return nil, &Reason{ReasonUnsupportedProtocol, fmt.Sprintf("port %d uses protocol %s; only TCP and UDP are supported", p.Port, p.Protocol)}
		}
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
```

- [ ] **Step 4: Run tests** — `go test ./pkg/loadbalancer/ -v` → all PASS.
- [ ] **Step 5: Confirm changed files** — only the two new files.

---

### Task 4: Object builders (`pkg/loadbalancer/builder.go`)

**Files:**
- Create: `pkg/loadbalancer/builder.go`, `pkg/loadbalancer/builder_test.go`

**Interfaces:**
- Consumes: Tasks 1–3.
- Produces:
  - `func BuildClient(pool *frpv1alpha1.ServerPool, server frpv1alpha1.ServerPoolServer, svc *corev1.Service, ref ServerRef) *frpv1alpha1.Client` — namespace = pool namespace; name/clientID per mode; labels (`managed-by`, pool, server; plus service-uid/namespace/name in PerService mode).
  - `func BuildUpstream(pool *frpv1alpha1.ServerPool, svc *corev1.Service, port corev1.ServicePort, ref ServerRef) *frpv1alpha1.Upstream` — TCP or UDP; labels for pool/server/port/protocol/service-*; annotation `AnnotationServiceCreated` = `svc.CreationTimestamp` RFC3339.
  - `func AllocationFromUpstream(u *frpv1alpha1.Upstream) (Allocation, bool)` — from labels/annotation; false when not a managed Upstream.
  - `func UpstreamLabels(svc *corev1.Service, ref ServerRef, port corev1.ServicePort) map[string]string`

- [ ] **Step 1: Write the failing tests**

```go
// pkg/loadbalancer/builder_test.go
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
```

- [ ] **Step 2: Run to verify failure** — undefined `BuildClient`.

- [ ] **Step 3: Implement**

```go
// pkg/loadbalancer/builder.go
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
			Name:      UpstreamName(svc, ref, port.Port),
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
```

- [ ] **Step 4: Run tests** — `go test ./pkg/loadbalancer/ -v` → PASS.
- [ ] **Step 5: Confirm changed files** — only the two new files.

---

### Task 5: ServiceReconciler — claim, allocate, generate, cleanup

**Files:**
- Create: `controllers/service_controller.go`, `controllers/service_controller_test.go`
- Modify: `main.go` (register reconciler + flags), `config/rbac/role.yaml` (regenerated)

**Interfaces:**
- Consumes: Tasks 1–4; `handler.Status(models.Config)`, `models.NewConfig`, `status.ConditionTypeReady`.
- Produces: `controllers.ServiceReconciler{Client; Scheme; Recorder; OperatorNamespace string; StatusFunc func(models.Config) ([]handler.ProxyStatus, error)}` with `Reconcile` and `SetupWithManager`; exported event reasons `EventReasonServerReallocated = "ServerReallocated"`, `EventReasonPoolDeleted = "PoolDeleted"`, `EventReasonClientNotReady = "ClientNotReady"`, `EventReasonProxyStartError = "ProxyStartError"`, `EventReasonBound = "Bound"`.

This task implements the whole reconcile loop including readiness gating and pool status (the spec's steps 1–8); Task 6 adds the watches for pools/generated objects and the race check. Tests in this task use `StatusFunc` to fake frpc.

- [ ] **Step 1: Write the failing tests**

```go
// controllers/service_controller_test.go
package controllers

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	frpv1alpha1 "github.com/zufardhiyaulhaq/frp-operator/api/v1alpha1"
	"github.com/zufardhiyaulhaq/frp-operator/pkg/client/handler"
	"github.com/zufardhiyaulhaq/frp-operator/pkg/client/models"
	"github.com/zufardhiyaulhaq/frp-operator/pkg/loadbalancer"
)

const opNS = "frp-operator"

func lbScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	if err := frpv1alpha1.AddToScheme(s); err != nil {
		t.Fatal(err)
	}
	if err := corev1.AddToScheme(s); err != nil {
		t.Fatal(err)
	}
	return s
}

func lbPool(name string, mode string, servers ...string) *frpv1alpha1.ServerPool {
	p := &frpv1alpha1.ServerPool{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: opNS},
		Spec:       frpv1alpha1.ServerPoolSpec{AllocationPolicy: frpv1alpha1.AllocationPolicyAuto, ClientMode: mode},
	}
	for _, s := range servers {
		p.Spec.Servers = append(p.Spec.Servers, frpv1alpha1.ServerPoolServer{
			Name: s, Host: s + ".example", Port: 7000, PublicAddress: "203.0.113." + s[len(s)-1:],
			Authentication: frpv1alpha1.ClientSpec_Server_Authentication{
				Token: &frpv1alpha1.ClientSpec_Server_Authentication_Token{Secret: frpv1alpha1.Secret{Name: "tok", Key: "token"}},
			},
		})
	}
	return p
}

func lbService(ns, name string, ports ...int32) *corev1.Service {
	class := loadbalancer.LoadBalancerClass
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns, UID: types.UID("uid-" + name), CreationTimestamp: metav1.Now()},
		Spec:       corev1.ServiceSpec{Type: corev1.ServiceTypeLoadBalancer, LoadBalancerClass: &class},
	}
	for _, p := range ports {
		svc.Spec.Ports = append(svc.Spec.Ports, corev1.ServicePort{Name: "p", Port: p, Protocol: corev1.ProtocolTCP})
	}
	return svc
}

func tokenSecret() *corev1.Secret {
	return &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "tok", Namespace: opNS}, Data: map[string][]byte{"token": []byte("x")}}
}

func newLBReconciler(t *testing.T, objs ...ctrlclient.Object) (*ServiceReconciler, ctrlclient.Client) {
	t.Helper()
	c := fake.NewClientBuilder().WithScheme(lbScheme(t)).WithObjects(objs...).
		WithStatusSubresource(&corev1.Service{}, &frpv1alpha1.ServerPool{}, &frpv1alpha1.Client{}).Build()
	r := &ServiceReconciler{
		Client: c, Scheme: c.Scheme(), Recorder: record.NewFakeRecorder(50), OperatorNamespace: opNS,
		StatusFunc: func(models.Config) ([]handler.ProxyStatus, error) { return nil, nil },
	}
	return r, c
}

// reconcileUntilStable runs Reconcile up to n times (finalizer add → allocate → status).
func reconcileUntilStable(t *testing.T, r *ServiceReconciler, svc *corev1.Service, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		if _, err := r.Reconcile(context.TODO(), ctrl.Request{NamespacedName: types.NamespacedName{Namespace: svc.Namespace, Name: svc.Name}}); err != nil {
			t.Fatalf("reconcile %d: %v", i, err)
		}
	}
}

func getService(t *testing.T, c ctrlclient.Client, ns, name string) *corev1.Service {
	t.Helper()
	svc := &corev1.Service{}
	if err := c.Get(context.TODO(), types.NamespacedName{Namespace: ns, Name: name}, svc); err != nil {
		t.Fatal(err)
	}
	return svc
}

func listManagedUpstreams(t *testing.T, c ctrlclient.Client) []frpv1alpha1.Upstream {
	t.Helper()
	list := &frpv1alpha1.UpstreamList{}
	if err := c.List(context.TODO(), list, ctrlclient.InNamespace(opNS), ctrlclient.MatchingLabels{loadbalancer.LabelManagedBy: loadbalancer.ManagedByValue}); err != nil {
		t.Fatal(err)
	}
	return list.Items
}

func TestServiceReconcile_IgnoresUnclaimed(t *testing.T) {
	svc := lbService("default", "web", 443)
	svc.Spec.LoadBalancerClass = nil
	r, c := newLBReconciler(t, svc, lbPool("p", frpv1alpha1.ClientModePerService, "s1"), tokenSecret())
	reconcileUntilStable(t, r, svc, 2)
	if got := getService(t, c, "default", "web"); len(got.Finalizers) != 0 {
		t.Fatalf("unclaimed service must not get a finalizer: %v", got.Finalizers)
	}
	if n := len(listManagedUpstreams(t, c)); n != 0 {
		t.Fatalf("generated %d upstreams for unclaimed service", n)
	}
}

func TestServiceReconcile_GeneratesClientAndUpstreams(t *testing.T) {
	svc := lbService("default", "web", 80, 443)
	r, c := newLBReconciler(t, svc, lbPool("p", frpv1alpha1.ClientModePerService, "s1"), tokenSecret())
	reconcileUntilStable(t, r, svc, 3)

	got := getService(t, c, "default", "web")
	if got.Annotations[loadbalancer.AnnotationAllocated] != "p/s1" {
		t.Fatalf("allocated annotation: %q", got.Annotations[loadbalancer.AnnotationAllocated])
	}
	if len(got.Finalizers) != 1 || got.Finalizers[0] != loadbalancer.Finalizer {
		t.Fatalf("finalizer: %v", got.Finalizers)
	}
	client := &frpv1alpha1.Client{}
	if err := c.Get(context.TODO(), types.NamespacedName{Namespace: opNS, Name: "lb-default-web-s1"}, client); err != nil {
		t.Fatalf("client: %v", err)
	}
	if client.Spec.Server.Host != "s1.example" {
		t.Fatalf("client host: %s", client.Spec.Server.Host)
	}
	ups := listManagedUpstreams(t, c)
	if len(ups) != 2 {
		t.Fatalf("upstreams: %d", len(ups))
	}
	// Client not Ready yet → no ingress
	if len(got.Status.LoadBalancer.Ingress) != 0 {
		t.Fatalf("ingress must wait for Ready client: %+v", got.Status.LoadBalancer.Ingress)
	}
}

func markClientReady(t *testing.T, c ctrlclient.Client, name string) {
	t.Helper()
	client := &frpv1alpha1.Client{}
	if err := c.Get(context.TODO(), types.NamespacedName{Namespace: opNS, Name: name}, client); err != nil {
		t.Fatal(err)
	}
	client.Status.Conditions = []metav1.Condition{{Type: "Ready", Status: metav1.ConditionTrue, Reason: "PodRunning", LastTransitionTime: metav1.Now()}}
	if err := c.Status().Update(context.TODO(), client); err != nil {
		t.Fatal(err)
	}
}

func TestServiceReconcile_IngressGatedOnReadyAndRunning(t *testing.T) {
	svc := lbService("default", "web", 443)
	r, c := newLBReconciler(t, svc, lbPool("p", frpv1alpha1.ClientModePerService, "s1"), tokenSecret())
	reconcileUntilStable(t, r, svc, 3)
	markClientReady(t, c, "lb-default-web-s1")

	// Ready but proxy not running → still no ingress
	r.StatusFunc = func(models.Config) ([]handler.ProxyStatus, error) {
		return []handler.ProxyStatus{{Name: "lb-default-web-s1-443", Type: "tcp", Status: "start error", Err: "port already used"}}, nil
	}
	reconcileUntilStable(t, r, svc, 1)
	if got := getService(t, c, "default", "web"); len(got.Status.LoadBalancer.Ingress) != 0 {
		t.Fatalf("ingress with proxy in start error: %+v", got.Status.LoadBalancer.Ingress)
	}

	r.StatusFunc = func(models.Config) ([]handler.ProxyStatus, error) {
		return []handler.ProxyStatus{{Name: "lb-default-web-s1-443", Type: "tcp", Status: "running"}}, nil
	}
	reconcileUntilStable(t, r, svc, 1)
	got := getService(t, c, "default", "web")
	if len(got.Status.LoadBalancer.Ingress) != 1 || got.Status.LoadBalancer.Ingress[0].IP != "203.0.113.1" {
		t.Fatalf("ingress: %+v", got.Status.LoadBalancer.Ingress)
	}
}

func TestServiceReconcile_HostnamePublicAddress(t *testing.T) {
	svc := lbService("default", "web", 443)
	p := lbPool("p", frpv1alpha1.ClientModePerService, "s1")
	p.Spec.Servers[0].PublicAddress = "lb.example.com"
	r, c := newLBReconciler(t, svc, p, tokenSecret())
	reconcileUntilStable(t, r, svc, 3)
	markClientReady(t, c, "lb-default-web-s1")
	r.StatusFunc = func(models.Config) ([]handler.ProxyStatus, error) {
		return []handler.ProxyStatus{{Name: "lb-default-web-s1-443", Status: "running"}}, nil
	}
	reconcileUntilStable(t, r, svc, 1)
	got := getService(t, c, "default", "web")
	if len(got.Status.LoadBalancer.Ingress) != 1 || got.Status.LoadBalancer.Ingress[0].Hostname != "lb.example.com" || got.Status.LoadBalancer.Ingress[0].IP != "" {
		t.Fatalf("ingress: %+v", got.Status.LoadBalancer.Ingress)
	}
}

func TestServiceReconcile_SecondServiceSamePortPendingThenTakesOver(t *testing.T) {
	a := lbService("default", "a", 443)
	b := lbService("default", "b", 443)
	b.CreationTimestamp = metav1.NewTime(a.CreationTimestamp.Add(1))
	r, c := newLBReconciler(t, a, b, lbPool("p", frpv1alpha1.ClientModePerService, "s1"), tokenSecret())
	reconcileUntilStable(t, r, a, 3)
	reconcileUntilStable(t, r, b, 3)

	if got := getService(t, c, "default", "b"); got.Annotations[loadbalancer.AnnotationAllocated] != "" {
		t.Fatalf("b must be pending, got %q", got.Annotations[loadbalancer.AnnotationAllocated])
	}
	if n := len(listManagedUpstreams(t, c)); n != 1 {
		t.Fatalf("upstreams: %d, want 1", n)
	}

	// delete a → finalizer path cleans up
	if err := c.Delete(context.TODO(), getService(t, c, "default", "a")); err != nil {
		t.Fatal(err)
	}
	reconcileUntilStable(t, r, a, 2)
	if err := c.Get(context.TODO(), types.NamespacedName{Namespace: "default", Name: "a"}, &corev1.Service{}); err == nil {
		t.Fatal("a should be gone after finalizer removal")
	}
	if n := len(listManagedUpstreams(t, c)); n != 0 {
		t.Fatalf("a's upstreams not cleaned: %d", n)
	}

	reconcileUntilStable(t, r, b, 2)
	if got := getService(t, c, "default", "b"); got.Annotations[loadbalancer.AnnotationAllocated] != "p/s1" {
		t.Fatalf("b must bind after a is gone, got %q", got.Annotations[loadbalancer.AnnotationAllocated])
	}
}

func TestServiceReconcile_StickyAndReallocateWhenServerRemoved(t *testing.T) {
	svc := lbService("default", "web", 443)
	p := lbPool("p", frpv1alpha1.ClientModePerService, "s1", "s2")
	r, c := newLBReconciler(t, svc, p, tokenSecret())
	reconcileUntilStable(t, r, svc, 3)
	if got := getService(t, c, "default", "web"); got.Annotations[loadbalancer.AnnotationAllocated] != "p/s1" {
		t.Fatalf("initial: %q", got.Annotations[loadbalancer.AnnotationAllocated])
	}

	// A new pool sorting earlier appears → must stay on p/s1
	if err := c.Create(context.TODO(), lbPool("a", frpv1alpha1.ClientModePerService, "a1")); err != nil {
		t.Fatal(err)
	}
	reconcileUntilStable(t, r, svc, 2)
	if got := getService(t, c, "default", "web"); got.Annotations[loadbalancer.AnnotationAllocated] != "p/s1" {
		t.Fatalf("sticky violated: %q", got.Annotations[loadbalancer.AnnotationAllocated])
	}

	// remove s1 from pool p → reallocate (to a/a1, the first fit)
	cur := &frpv1alpha1.ServerPool{}
	if err := c.Get(context.TODO(), types.NamespacedName{Namespace: opNS, Name: "p"}, cur); err != nil {
		t.Fatal(err)
	}
	cur.Spec.Servers = cur.Spec.Servers[1:]
	if err := c.Update(context.TODO(), cur); err != nil {
		t.Fatal(err)
	}
	reconcileUntilStable(t, r, svc, 3)
	got := getService(t, c, "default", "web")
	if got.Annotations[loadbalancer.AnnotationAllocated] != "a/a1" {
		t.Fatalf("reallocation: %q", got.Annotations[loadbalancer.AnnotationAllocated])
	}
	for _, u := range listManagedUpstreams(t, c) {
		if u.Labels[loadbalancer.LabelServer] == "s1" {
			t.Fatalf("stale upstream on removed server: %s", u.Name)
		}
	}
}

func TestServiceReconcile_SharedClientLifecycle(t *testing.T) {
	a := lbService("default", "a", 80)
	b := lbService("default", "b", 443)
	r, c := newLBReconciler(t, a, b, lbPool("p", frpv1alpha1.ClientModeShared, "s1"), tokenSecret())
	reconcileUntilStable(t, r, a, 3)
	reconcileUntilStable(t, r, b, 3)

	list := &frpv1alpha1.ClientList{}
	if err := c.List(context.TODO(), list, ctrlclient.InNamespace(opNS)); err != nil {
		t.Fatal(err)
	}
	if len(list.Items) != 1 || list.Items[0].Name != "pool-p-s1" {
		t.Fatalf("shared mode must create exactly one client: %+v", list.Items)
	}

	if err := c.Delete(context.TODO(), getService(t, c, "default", "a")); err != nil {
		t.Fatal(err)
	}
	reconcileUntilStable(t, r, a, 2)
	if err := c.Get(context.TODO(), types.NamespacedName{Namespace: opNS, Name: "pool-p-s1"}, &frpv1alpha1.Client{}); err != nil {
		t.Fatal("shared client must survive while b still uses it")
	}
	if err := c.Delete(context.TODO(), getService(t, c, "default", "b")); err != nil {
		t.Fatal(err)
	}
	reconcileUntilStable(t, r, b, 2)
	if err := c.Get(context.TODO(), types.NamespacedName{Namespace: opNS, Name: "pool-p-s1"}, &frpv1alpha1.Client{}); err == nil {
		t.Fatal("shared client must be deleted with its last upstream")
	}
}

func TestServiceReconcile_ClassRemovedCleansUp(t *testing.T) {
	svc := lbService("default", "web", 443)
	r, c := newLBReconciler(t, svc, lbPool("p", frpv1alpha1.ClientModePerService, "s1"), tokenSecret())
	reconcileUntilStable(t, r, svc, 3)

	cur := getService(t, c, "default", "web")
	cur.Spec.LoadBalancerClass = nil
	cur.Spec.Type = corev1.ServiceTypeClusterIP
	if err := c.Update(context.TODO(), cur); err != nil {
		t.Fatal(err)
	}
	reconcileUntilStable(t, r, svc, 2)
	got := getService(t, c, "default", "web")
	if len(got.Finalizers) != 0 || got.Annotations[loadbalancer.AnnotationAllocated] != "" || len(got.Status.LoadBalancer.Ingress) != 0 {
		t.Fatalf("not cleaned: fin=%v ann=%q ing=%v", got.Finalizers, got.Annotations[loadbalancer.AnnotationAllocated], got.Status.LoadBalancer.Ingress)
	}
	if n := len(listManagedUpstreams(t, c)); n != 0 {
		t.Fatalf("upstreams left: %d", n)
	}
}

func TestServiceReconcile_PoolStatusProjection(t *testing.T) {
	svc := lbService("default", "web", 443)
	r, c := newLBReconciler(t, svc, lbPool("p", frpv1alpha1.ClientModePerService, "s1"), tokenSecret())
	reconcileUntilStable(t, r, svc, 3)
	p := &frpv1alpha1.ServerPool{}
	if err := c.Get(context.TODO(), types.NamespacedName{Namespace: opNS, Name: "p"}, p); err != nil {
		t.Fatal(err)
	}
	if len(p.Status.Servers) != 1 || len(p.Status.Servers[0].AllocatedPorts) != 1 ||
		p.Status.Servers[0].AllocatedPorts[0].Port != 443 || p.Status.Servers[0].AllocatedPorts[0].Service != "default/web" {
		t.Fatalf("pool status: %+v", p.Status)
	}
}
```

- [ ] **Step 2: Run to verify failure** — `go test ./controllers/ -run TestServiceReconcile` → undefined `ServiceReconciler`.

- [ ] **Step 3: Implement the reconciler**

```go
// controllers/service_controller.go
package controllers

import (
	"context"
	"fmt"
	"net"
	"reflect"
	"sort"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	frpv1alpha1 "github.com/zufardhiyaulhaq/frp-operator/api/v1alpha1"
	"github.com/zufardhiyaulhaq/frp-operator/pkg/client/handler"
	"github.com/zufardhiyaulhaq/frp-operator/pkg/client/models"
	"github.com/zufardhiyaulhaq/frp-operator/pkg/client/status"
	"github.com/zufardhiyaulhaq/frp-operator/pkg/loadbalancer"
)

const (
	EventReasonBound             = "Bound"
	EventReasonServerReallocated = "ServerReallocated"
	EventReasonPoolDeleted       = "PoolDeleted"
	EventReasonClientNotReady    = "ClientNotReady"
	EventReasonProxyStartError   = "ProxyStartError"

	lbRequeue = 30 * time.Second
)

// ServiceReconciler binds LoadBalancer Services with our loadBalancerClass to ServerPool servers.
type ServiceReconciler struct {
	ctrlclient.Client
	Scheme            *runtime.Scheme
	Recorder          record.EventRecorder
	OperatorNamespace string
	// StatusFunc reads frpc proxy status; nil means handler.Status. Tests override it.
	StatusFunc func(models.Config) ([]handler.ProxyStatus, error)
}

//+kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;update;patch
//+kubebuilder:rbac:groups="",resources=services/status,verbs=get;update;patch
//+kubebuilder:rbac:groups="",resources=events,verbs=create;patch
//+kubebuilder:rbac:groups=frp.zufardhiyaulhaq.com,resources=serverpools,verbs=get;list;watch
//+kubebuilder:rbac:groups=frp.zufardhiyaulhaq.com,resources=serverpools/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=frp.zufardhiyaulhaq.com,resources=clients;upstreams,verbs=get;list;watch;create;update;patch;delete

func (r *ServiceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx).WithValues("service", req.NamespacedName)

	svc := &corev1.Service{}
	if err := r.Get(ctx, req.NamespacedName, svc); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	claimed := loadbalancer.IsClaimed(svc)
	hasFinalizer := controllerutil.ContainsFinalizer(svc, loadbalancer.Finalizer)

	if !svc.DeletionTimestamp.IsZero() || !claimed {
		if !hasFinalizer {
			return ctrl.Result{}, nil
		}
		if err := r.cleanupService(ctx, svc, nil); err != nil {
			return ctrl.Result{}, err
		}
		if err := r.updatePoolStatuses(ctx); err != nil {
			log.Error(err, "failed to update pool status")
		}
		if claimed == false && svc.DeletionTimestamp.IsZero() {
			delete(svc.Annotations, loadbalancer.AnnotationAllocated)
			svc.Status.LoadBalancer = corev1.LoadBalancerStatus{}
			if err := r.Status().Update(ctx, svc); err != nil && !apierrors.IsNotFound(err) {
				return ctrl.Result{}, err
			}
		}
		controllerutil.RemoveFinalizer(svc, loadbalancer.Finalizer)
		if err := r.Update(ctx, svc); err != nil && !apierrors.IsNotFound(err) {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	if !hasFinalizer {
		controllerutil.AddFinalizer(svc, loadbalancer.Finalizer)
		if err := r.Update(ctx, svc); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	pools, err := r.listPools(ctx)
	if err != nil {
		return ctrl.Result{}, err
	}
	allocs, err := r.listAllocations(ctx)
	if err != nil {
		return ctrl.Result{}, err
	}
	request := requestFor(svc)

	bound, err := loadbalancer.ParseAllocated(svc.Annotations[loadbalancer.AnnotationAllocated])
	if err != nil {
		log.Info("ignoring malformed allocated-server annotation", "error", err.Error())
		bound = nil
	}

	refs, reason := r.resolveBinding(ctx, svc, request, bound, pools, allocs)
	if reason != nil {
		r.Recorder.Event(svc, corev1.EventTypeWarning, reason.Reason, reason.Message)
		return r.setPending(ctx, svc)
	}

	// Drop generated objects on servers we are no longer bound to (reallocation / annotation edit).
	if err := r.cleanupService(ctx, svc, refs); err != nil {
		return ctrl.Result{}, err
	}
	if err := r.applyBinding(ctx, svc, request, refs, pools); err != nil {
		return ctrl.Result{}, err
	}
	if lost := r.raceCheck(ctx, svc, refs, request); lost != nil {
		r.Recorder.Event(svc, corev1.EventTypeWarning, lost.Reason, lost.Message)
		if err := r.cleanupService(ctx, svc, nil); err != nil {
			return ctrl.Result{}, err
		}
		return r.setPending(ctx, svc)
	}

	want := loadbalancer.FormatAllocated(refs)
	if svc.Annotations[loadbalancer.AnnotationAllocated] != want {
		if svc.Annotations == nil {
			svc.Annotations = map[string]string{}
		}
		svc.Annotations[loadbalancer.AnnotationAllocated] = want
		if err := r.Update(ctx, svc); err != nil {
			return ctrl.Result{}, err
		}
		r.Recorder.Event(svc, corev1.EventTypeNormal, EventReasonBound, "bound to "+want)
	}

	ingress := r.ingressFor(ctx, svc, request, refs, pools)
	if !reflect.DeepEqual(svc.Status.LoadBalancer.Ingress, ingress) {
		svc.Status.LoadBalancer.Ingress = ingress
		if err := r.Status().Update(ctx, svc); err != nil {
			return ctrl.Result{}, err
		}
	}
	if err := r.updatePoolStatuses(ctx); err != nil {
		log.Error(err, "failed to update pool status")
	}
	return ctrl.Result{RequeueAfter: lbRequeue}, nil
}

func requestFor(svc *corev1.Service) loadbalancer.Request {
	return loadbalancer.Request{
		ServiceKey: loadbalancer.ServiceKey(svc),
		Ports:      svc.Spec.Ports,
		Pool:       strings.TrimSpace(svc.Annotations[loadbalancer.AnnotationServerPool]),
		Servers:    loadbalancer.ParseServerList(svc.Annotations[loadbalancer.AnnotationServer]),
	}
}

// resolveBinding keeps an existing valid binding (sticky) or runs selection.
func (r *ServiceReconciler) resolveBinding(ctx context.Context, svc *corev1.Service, request loadbalancer.Request,
	bound []loadbalancer.ServerRef, pools []frpv1alpha1.ServerPool, allocs []loadbalancer.Allocation) ([]loadbalancer.ServerRef, *loadbalancer.Reason) {

	if len(bound) > 0 && boundStillValid(bound, request, pools) {
		for _, ref := range bound {
			pool := loadbalancer.FindPool(pools, ref.Pool)
			server := loadbalancer.FindServer(*pool, ref.Server)
			if reason := loadbalancer.Fit(request, *pool, *server, allocs); reason != nil {
				return nil, reason
			}
		}
		return bound, nil
	}

	refs, reason := loadbalancer.Allocate(request, pools, allocs)
	if reason != nil {
		if len(bound) > 0 {
			r.Recorder.Event(svc, corev1.EventTypeWarning, EventReasonPoolDeleted,
				fmt.Sprintf("bound server(s) %s no longer exist; %s", loadbalancer.FormatAllocated(bound), reason.Message))
		}
		return nil, reason
	}
	if len(bound) > 0 {
		r.Recorder.Event(svc, corev1.EventTypeWarning, EventReasonServerReallocated,
			fmt.Sprintf("%s -> %s", loadbalancer.FormatAllocated(bound), loadbalancer.FormatAllocated(refs)))
	}
	return refs, nil
}

// boundStillValid: every bound server still exists, and if the user pins servers the pinned set
// must equal the bound set (so annotation edits are honoured).
func boundStillValid(bound []loadbalancer.ServerRef, request loadbalancer.Request, pools []frpv1alpha1.ServerPool) bool {
	for _, ref := range bound {
		pool := loadbalancer.FindPool(pools, ref.Pool)
		if pool == nil || loadbalancer.FindServer(*pool, ref.Server) == nil {
			return false
		}
		if request.Pool != "" && request.Pool != ref.Pool {
			return false
		}
	}
	if len(request.Servers) == 0 {
		return true
	}
	if len(request.Servers) != len(bound) {
		return false
	}
	want := map[string]bool{}
	for _, s := range request.Servers {
		want[s] = true
	}
	for _, ref := range bound {
		if !want[ref.Server] {
			return false
		}
	}
	return true
}

func (r *ServiceReconciler) listPools(ctx context.Context) ([]frpv1alpha1.ServerPool, error) {
	list := &frpv1alpha1.ServerPoolList{}
	if err := r.List(ctx, list, ctrlclient.InNamespace(r.OperatorNamespace)); err != nil {
		return nil, err
	}
	return list.Items, nil
}

func (r *ServiceReconciler) listManagedUpstreams(ctx context.Context, extra ...ctrlclient.ListOption) ([]frpv1alpha1.Upstream, error) {
	list := &frpv1alpha1.UpstreamList{}
	opts := append([]ctrlclient.ListOption{
		ctrlclient.InNamespace(r.OperatorNamespace),
		ctrlclient.MatchingLabels{loadbalancer.LabelManagedBy: loadbalancer.ManagedByValue},
	}, extra...)
	if err := r.List(ctx, list, opts...); err != nil {
		return nil, err
	}
	return list.Items, nil
}

func (r *ServiceReconciler) listAllocations(ctx context.Context) ([]loadbalancer.Allocation, error) {
	ups, err := r.listManagedUpstreams(ctx)
	if err != nil {
		return nil, err
	}
	var allocs []loadbalancer.Allocation
	for i := range ups {
		if a, ok := loadbalancer.AllocationFromUpstream(&ups[i]); ok {
			allocs = append(allocs, a)
		}
	}
	return allocs, nil
}

// applyBinding ensures Clients and Upstreams for every bound server and port.
func (r *ServiceReconciler) applyBinding(ctx context.Context, svc *corev1.Service, request loadbalancer.Request,
	refs []loadbalancer.ServerRef, pools []frpv1alpha1.ServerPool) error {

	wantUpstreams := map[string]bool{}
	for _, ref := range refs {
		pool := loadbalancer.FindPool(pools, ref.Pool)
		server := loadbalancer.FindServer(*pool, ref.Server)

		desiredClient := loadbalancer.BuildClient(pool, *server, svc, ref)
		if err := r.ensureClient(ctx, desiredClient); err != nil {
			return err
		}
		for _, port := range request.Ports {
			desired := loadbalancer.BuildUpstream(pool, svc, port, ref)
			wantUpstreams[desired.Name] = true
			if err := r.ensureUpstream(ctx, desired); err != nil {
				return err
			}
		}
	}
	// Remove upstreams of this Service for ports that no longer exist.
	existing, err := r.listManagedUpstreams(ctx, ctrlclient.MatchingLabels{loadbalancer.LabelServiceUID: string(svc.UID)})
	if err != nil {
		return err
	}
	for i := range existing {
		if !wantUpstreams[existing[i].Name] {
			if err := r.Delete(ctx, &existing[i]); err != nil && !apierrors.IsNotFound(err) {
				return err
			}
		}
	}
	return nil
}

func (r *ServiceReconciler) ensureClient(ctx context.Context, desired *frpv1alpha1.Client) error {
	current := &frpv1alpha1.Client{}
	err := r.Get(ctx, types.NamespacedName{Namespace: desired.Namespace, Name: desired.Name}, current)
	if apierrors.IsNotFound(err) {
		return r.Create(ctx, desired)
	}
	if err != nil {
		return err
	}
	if reflect.DeepEqual(current.Spec, desired.Spec) && reflect.DeepEqual(current.Labels, desired.Labels) {
		return nil
	}
	current.Spec = desired.Spec
	current.Labels = desired.Labels
	return r.Update(ctx, current)
}

func (r *ServiceReconciler) ensureUpstream(ctx context.Context, desired *frpv1alpha1.Upstream) error {
	current := &frpv1alpha1.Upstream{}
	err := r.Get(ctx, types.NamespacedName{Namespace: desired.Namespace, Name: desired.Name}, current)
	if apierrors.IsNotFound(err) {
		return r.Create(ctx, desired)
	}
	if err != nil {
		return err
	}
	if reflect.DeepEqual(current.Spec, desired.Spec) && reflect.DeepEqual(current.Labels, desired.Labels) &&
		reflect.DeepEqual(current.Annotations, desired.Annotations) {
		return nil
	}
	current.Spec = desired.Spec
	current.Labels = desired.Labels
	current.Annotations = desired.Annotations
	return r.Update(ctx, current)
}

// cleanupService deletes generated objects of svc that are not on one of keep. keep == nil
// deletes everything. Shared Clients are deleted when no Upstream references them any more.
func (r *ServiceReconciler) cleanupService(ctx context.Context, svc *corev1.Service, keep []loadbalancer.ServerRef) error {
	keepSet := map[loadbalancer.ServerRef]bool{}
	for _, ref := range keep {
		keepSet[ref] = true
	}
	ups, err := r.listManagedUpstreams(ctx, ctrlclient.MatchingLabels{loadbalancer.LabelServiceUID: string(svc.UID)})
	if err != nil {
		return err
	}
	touchedClients := map[string]bool{}
	for i := range ups {
		ref := loadbalancer.ServerRef{Pool: ups[i].Labels[loadbalancer.LabelPool], Server: ups[i].Labels[loadbalancer.LabelServer]}
		if keepSet[ref] {
			continue
		}
		touchedClients[ups[i].Spec.Client] = true
		if err := r.Delete(ctx, &ups[i]); err != nil && !apierrors.IsNotFound(err) {
			return err
		}
	}
	// Per-service clients of this service not on a kept server.
	clients := &frpv1alpha1.ClientList{}
	if err := r.List(ctx, clients, ctrlclient.InNamespace(r.OperatorNamespace),
		ctrlclient.MatchingLabels{loadbalancer.LabelManagedBy: loadbalancer.ManagedByValue, loadbalancer.LabelServiceUID: string(svc.UID)}); err != nil {
		return err
	}
	for i := range clients.Items {
		ref := loadbalancer.ServerRef{Pool: clients.Items[i].Labels[loadbalancer.LabelPool], Server: clients.Items[i].Labels[loadbalancer.LabelServer]}
		if keepSet[ref] {
			continue
		}
		if err := r.Delete(ctx, &clients.Items[i]); err != nil && !apierrors.IsNotFound(err) {
			return err
		}
		delete(touchedClients, clients.Items[i].Name)
	}
	// Shared clients: delete when orphaned.
	for name := range touchedClients {
		remaining, err := r.listManagedUpstreams(ctx)
		if err != nil {
			return err
		}
		inUse := false
		for _, u := range remaining {
			if u.Spec.Client == name {
				inUse = true
				break
			}
		}
		if inUse {
			continue
		}
		c := &frpv1alpha1.Client{}
		if err := r.Get(ctx, types.NamespacedName{Namespace: r.OperatorNamespace, Name: name}, c); err != nil {
			if apierrors.IsNotFound(err) {
				continue
			}
			return err
		}
		if c.Labels[loadbalancer.LabelManagedBy] != loadbalancer.ManagedByValue {
			continue
		}
		if err := r.Delete(ctx, c); err != nil && !apierrors.IsNotFound(err) {
			return err
		}
	}
	return nil
}

// raceCheck resolves two Services that both created Upstreams for the same server/port.
func (r *ServiceReconciler) raceCheck(ctx context.Context, svc *corev1.Service, refs []loadbalancer.ServerRef, request loadbalancer.Request) *loadbalancer.Reason {
	allocs, err := r.listAllocations(ctx)
	if err != nil {
		return nil
	}
	mine := loadbalancer.ServiceKey(svc)
	for _, ref := range refs {
		for _, port := range request.Ports {
			var own *loadbalancer.Allocation
			var others []loadbalancer.Allocation
			for i := range allocs {
				a := allocs[i]
				if a.Pool != ref.Pool || a.Server != ref.Server || a.Port != port.Port || string(a.Protocol) != string(protocolOrTCP(port)) {
					continue
				}
				if a.ServiceKey == mine {
					own = &allocs[i]
				} else {
					others = append(others, a)
				}
			}
			if own == nil {
				continue
			}
			for _, o := range others {
				if loadbalancer.Winner(*own, o).ServiceKey != mine {
					return &loadbalancer.Reason{Reason: loadbalancer.ReasonPortUnavailable,
						Message: fmt.Sprintf("port %d/%s on %s won by %s", port.Port, protocolOrTCP(port), ref, o.ServiceKey)}
				}
			}
		}
	}
	return nil
}

func protocolOrTCP(p corev1.ServicePort) corev1.Protocol {
	if p.Protocol == "" {
		return corev1.ProtocolTCP
	}
	return p.Protocol
}

// ingressFor returns one ingress entry per bound server whose Client is Ready and whose
// proxies for this Service are all running. Servers that are not ready emit an Event and are omitted.
func (r *ServiceReconciler) ingressFor(ctx context.Context, svc *corev1.Service, request loadbalancer.Request,
	refs []loadbalancer.ServerRef, pools []frpv1alpha1.ServerPool) []corev1.LoadBalancerIngress {

	var ingress []corev1.LoadBalancerIngress
	for _, ref := range refs {
		pool := loadbalancer.FindPool(pools, ref.Pool)
		server := loadbalancer.FindServer(*pool, ref.Server)
		clientName := loadbalancer.ClientName(svc, ref, pool.Spec.ClientMode)

		client := &frpv1alpha1.Client{}
		if err := r.Get(ctx, types.NamespacedName{Namespace: r.OperatorNamespace, Name: clientName}, client); err != nil {
			r.Recorder.Event(svc, corev1.EventTypeWarning, EventReasonClientNotReady, fmt.Sprintf("%s: client %s: %v", ref, clientName, err))
			continue
		}
		if !clientReady(client) {
			r.Recorder.Event(svc, corev1.EventTypeWarning, EventReasonClientNotReady, fmt.Sprintf("%s: client %s is not Ready: %s", ref, clientName, client.Status.Message))
			continue
		}
		proxies, err := r.proxyStatus(ctx, client)
		if err != nil {
			r.Recorder.Event(svc, corev1.EventTypeWarning, EventReasonClientNotReady, fmt.Sprintf("%s: cannot read frpc status: %v", ref, err))
			continue
		}
		byName := map[string]handler.ProxyStatus{}
		for _, p := range proxies {
			byName[p.Name] = p
		}
		ready := true
		for _, port := range request.Ports {
			name := loadbalancer.UpstreamName(svc, ref, port.Port)
			p, ok := byName[name]
			if !ok || p.Status != "running" {
				ready = false
				msg := fmt.Sprintf("port %d on %s: proxy %s status %q", port.Port, ref, name, p.Status)
				if p.Err != "" {
					msg += ": " + p.Err
				}
				r.Recorder.Event(svc, corev1.EventTypeWarning, EventReasonProxyStartError, msg)
			}
		}
		if !ready {
			continue
		}
		if net.ParseIP(server.PublicAddress) != nil {
			ingress = append(ingress, corev1.LoadBalancerIngress{IP: server.PublicAddress})
		} else {
			ingress = append(ingress, corev1.LoadBalancerIngress{Hostname: server.PublicAddress})
		}
	}
	return ingress
}

func clientReady(c *frpv1alpha1.Client) bool {
	for _, cond := range c.Status.Conditions {
		if cond.Type == status.ConditionTypeReady && cond.Status == metav1.ConditionTrue {
			return true
		}
	}
	return false
}

func (r *ServiceReconciler) proxyStatus(ctx context.Context, client *frpv1alpha1.Client) ([]handler.ProxyStatus, error) {
	cfg, err := models.NewConfig(r.Client, client, nil, nil)
	if err != nil {
		return nil, err
	}
	cfg.Common.AdminAddress = client.Name + "-frpc." + client.Namespace + ".svc"
	fn := r.StatusFunc
	if fn == nil {
		fn = handler.Status
	}
	return fn(cfg)
}

func (r *ServiceReconciler) setPending(ctx context.Context, svc *corev1.Service) (ctrl.Result, error) {
	if len(svc.Status.LoadBalancer.Ingress) != 0 {
		svc.Status.LoadBalancer.Ingress = nil
		if err := r.Status().Update(ctx, svc); err != nil {
			return ctrl.Result{}, err
		}
	}
	if err := r.updatePoolStatuses(ctx); err != nil {
		log.FromContext(ctx).Error(err, "failed to update pool status")
	}
	return ctrl.Result{RequeueAfter: lbRequeue}, nil
}

// updatePoolStatuses rewrites status.servers[].allocatedPorts and the Ready condition of every pool.
func (r *ServiceReconciler) updatePoolStatuses(ctx context.Context) error {
	pools, err := r.listPools(ctx)
	if err != nil {
		return err
	}
	allocs, err := r.listAllocations(ctx)
	if err != nil {
		return err
	}
	for i := range pools {
		pool := &pools[i]
		desired := frpv1alpha1.ServerPoolStatus{Conditions: pool.Status.Conditions}
		for _, s := range pool.Spec.Servers {
			st := frpv1alpha1.ServerPoolServerStatus{Name: s.Name}
			for _, a := range allocs {
				if a.Pool == pool.Name && a.Server == s.Name {
					st.AllocatedPorts = append(st.AllocatedPorts, frpv1alpha1.AllocatedPort{Port: a.Port, Protocol: string(a.Protocol), Service: a.ServiceKey})
				}
			}
			sort.Slice(st.AllocatedPorts, func(x, y int) bool {
				if st.AllocatedPorts[x].Port != st.AllocatedPorts[y].Port {
					return st.AllocatedPorts[x].Port < st.AllocatedPorts[y].Port
				}
				return st.AllocatedPorts[x].Protocol < st.AllocatedPorts[y].Protocol
			})
			desired.Servers = append(desired.Servers, st)
		}
		readyMsg, readyStatus := r.poolReady(ctx, pool)
		desired.Conditions = setPoolCondition(desired.Conditions, metav1.Condition{
			Type: status.ConditionTypeReady, Status: readyStatus, Reason: "Validated", Message: readyMsg,
		})
		if reflect.DeepEqual(pool.Status.Servers, desired.Servers) && sameConditionsIgnoringTime(pool.Status.Conditions, desired.Conditions) {
			continue
		}
		pool.Status = desired
		if err := r.Status().Update(ctx, pool); err != nil && !apierrors.IsNotFound(err) {
			return err
		}
	}
	return nil
}

// poolReady validates server names are unique, allowedPorts parse, and token Secrets exist.
func (r *ServiceReconciler) poolReady(ctx context.Context, pool *frpv1alpha1.ServerPool) (string, metav1.ConditionStatus) {
	seen := map[string]bool{}
	if _, err := loadbalancer.ParsePortRanges(pool.Spec.AllowedPorts); err != nil {
		return err.Error(), metav1.ConditionFalse
	}
	for _, s := range pool.Spec.Servers {
		if seen[s.Name] {
			return fmt.Sprintf("duplicate server name %q", s.Name), metav1.ConditionFalse
		}
		seen[s.Name] = true
		if _, err := loadbalancer.ParsePortRanges(s.AllowedPorts); err != nil {
			return fmt.Sprintf("server %s: %v", s.Name, err), metav1.ConditionFalse
		}
		if s.Authentication.Token != nil {
			sec := &corev1.Secret{}
			if err := r.Get(ctx, types.NamespacedName{Namespace: pool.Namespace, Name: s.Authentication.Token.Secret.Name}, sec); err != nil {
				return fmt.Sprintf("server %s: token secret %q: %v", s.Name, s.Authentication.Token.Secret.Name, err), metav1.ConditionFalse
			}
		}
	}
	return "pool is valid", metav1.ConditionTrue
}

func setPoolCondition(conds []metav1.Condition, c metav1.Condition) []metav1.Condition {
	c.LastTransitionTime = metav1.Now()
	for i := range conds {
		if conds[i].Type == c.Type {
			if conds[i].Status == c.Status {
				c.LastTransitionTime = conds[i].LastTransitionTime
			}
			conds[i] = c
			return conds
		}
	}
	return append(conds, c)
}

func sameConditionsIgnoringTime(a, b []metav1.Condition) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Type != b[i].Type || a[i].Status != b[i].Status || a[i].Reason != b[i].Reason || a[i].Message != b[i].Message {
			return false
		}
	}
	return true
}

func (r *ServiceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Recorder = mgr.GetEventRecorderFor("service-controller")
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Service{}).
		Complete(r)
}
```

Note for the implementer: `claimed == false` in the deletion branch is written that way on purpose to read as "class removed"; `gofmt` accepts it, but `go vet` may flag `== false` under some analyzers — if it does, change to `!claimed`.

- [ ] **Step 4: Register in `main.go`**

Add after the `UpstreamReconciler` block, and add the two flags next to the existing ones:

```go
	var enableLoadBalancer bool
	var operatorNamespace string
	flag.BoolVar(&enableLoadBalancer, "enable-loadbalancer-controller", true,
		"Bind Service type=LoadBalancer with loadBalancerClass frp.zufardhiyaulhaq.com/frp to ServerPool servers.")
	flag.StringVar(&operatorNamespace, "operator-namespace", os.Getenv("POD_NAMESPACE"),
		"Namespace that holds ServerPools and generated Clients/Upstreams. Defaults to POD_NAMESPACE.")
```

```go
	if enableLoadBalancer {
		if operatorNamespace == "" {
			setupLog.Error(nil, "--operator-namespace (or POD_NAMESPACE) is required when the LoadBalancer controller is enabled")
			os.Exit(1)
		}
		if err = (&controllers.ServiceReconciler{
			Client:            mgr.GetClient(),
			Scheme:            mgr.GetScheme(),
			OperatorNamespace: operatorNamespace,
		}).SetupWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create controller", "controller", "Service")
			os.Exit(1)
		}
	}
```

- [ ] **Step 5: Run tests and build**

Run: `PATH=$HOME/.asdf/installs/golang/1.23.12/go/bin:$PATH go build ./... && go vet ./... && go test ./controllers/ ./pkg/... && gofmt -l .`
Expected: all tests PASS, gofmt prints nothing. If `TestServiceReconcile_SecondServiceSamePortPendingThenTakesOver` fails because the fake client does not run finalizer semantics: the fake client in controller-runtime 0.18 *does* keep an object with finalizers after Delete and removes it once finalizers are cleared — no workaround needed. If a Status().Update on `Service` fails with "not found status subresource", ensure `WithStatusSubresource(&corev1.Service{}, …)` is present in the builder.

- [ ] **Step 6: Regenerate RBAC**

Run: `PATH=$HOME/.asdf/installs/golang/1.23.12/go/bin:$PATH make manifests` → `config/rbac/role.yaml` gains `services/status` and `serverpools*` rules.

- [ ] **Step 7: Confirm changed files** — `controllers/service_controller.go`, `controllers/service_controller_test.go`, `main.go`, `config/rbac/role.yaml`.

---

### Task 6: Watches for pools and generated objects

**Files:**
- Modify: `controllers/service_controller.go` (`SetupWithManager`), `controllers/service_controller_test.go`

**Interfaces:**
- Consumes: Task 5.
- Produces: `func (r *ServiceReconciler) claimedServices(ctx context.Context) []reconcile.Request` (exported for tests as `ClaimedServiceRequests`).

- [ ] **Step 1: Write the failing test**

Append to `controllers/service_controller_test.go`:

```go
func TestClaimedServiceRequests(t *testing.T) {
	claimed := lbService("default", "web", 443)
	other := lbService("default", "other", 80)
	other.Spec.LoadBalancerClass = nil
	r, _ := newLBReconciler(t, claimed, other)
	reqs := r.ClaimedServiceRequests(context.TODO())
	if len(reqs) != 1 || reqs[0].Name != "web" || reqs[0].Namespace != "default" {
		t.Fatalf("got %v", reqs)
	}
}

func TestPredicate_OnlyClaimedOrFinalized(t *testing.T) {
	claimed := lbService("default", "web", 443)
	plain := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "x", Namespace: "default"}}
	finalized := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "y", Namespace: "default", Finalizers: []string{loadbalancer.Finalizer}}}
	if !serviceOfInterest(claimed) || serviceOfInterest(plain) || !serviceOfInterest(finalized) {
		t.Fatal("predicate mismatch")
	}
}
```

- [ ] **Step 2: Run to verify failure** — undefined `ClaimedServiceRequests`, `serviceOfInterest`.

- [ ] **Step 3: Implement**

Replace `SetupWithManager` in `controllers/service_controller.go` and add helpers (new imports: `ctrlbuilder "sigs.k8s.io/controller-runtime/pkg/builder"` (aliased — `builder` is taken by `pkg/client/builder` elsewhere in this package), `sigs.k8s.io/controller-runtime/pkg/event`, `sigs.k8s.io/controller-runtime/pkg/handler` aliased `ctrlhandler`, `sigs.k8s.io/controller-runtime/pkg/predicate`, `sigs.k8s.io/controller-runtime/pkg/reconcile`):

```go
func serviceOfInterest(obj ctrlclient.Object) bool {
	svc, ok := obj.(*corev1.Service)
	if !ok {
		return false
	}
	return loadbalancer.IsClaimed(svc) || controllerutil.ContainsFinalizer(svc, loadbalancer.Finalizer)
}

// ClaimedServiceRequests enqueues every Service we own; used when a pool or a generated object changes.
func (r *ServiceReconciler) ClaimedServiceRequests(ctx context.Context) []reconcile.Request {
	list := &corev1.ServiceList{}
	if err := r.List(ctx, list); err != nil {
		return nil
	}
	var reqs []reconcile.Request
	for i := range list.Items {
		if serviceOfInterest(&list.Items[i]) {
			reqs = append(reqs, reconcile.Request{NamespacedName: types.NamespacedName{Namespace: list.Items[i].Namespace, Name: list.Items[i].Name}})
		}
	}
	return reqs
}

func (r *ServiceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Recorder = mgr.GetEventRecorderFor("service-controller")
	all := ctrlhandler.EnqueueRequestsFromMapFunc(func(ctx context.Context, _ ctrlclient.Object) []reconcile.Request {
		return r.ClaimedServiceRequests(ctx)
	})
	managed := predicate.NewPredicateFuncs(func(obj ctrlclient.Object) bool {
		return obj.GetNamespace() == r.OperatorNamespace &&
			obj.GetLabels()[loadbalancer.LabelManagedBy] == loadbalancer.ManagedByValue
	})
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Service{}, ctrlbuilder.WithPredicates(predicate.NewPredicateFuncs(serviceOfInterest))).
		Watches(&frpv1alpha1.ServerPool{}, all).
		Watches(&frpv1alpha1.Upstream{}, all, ctrlbuilder.WithPredicates(managed)).
		Watches(&frpv1alpha1.Client{}, all, ctrlbuilder.WithPredicates(managed, predicate.Funcs{
			// Only status/label changes of generated Clients matter (Ready flips); spec updates come from us.
			UpdateFunc: func(e event.UpdateEvent) bool {
				oldC, ok1 := e.ObjectOld.(*frpv1alpha1.Client)
				newC, ok2 := e.ObjectNew.(*frpv1alpha1.Client)
				return ok1 && ok2 && !reflect.DeepEqual(oldC.Status.Conditions, newC.Status.Conditions)
			},
		})).
		Complete(r)
}
```

- [ ] **Step 4: Run** — `go build ./... && go vet ./... && go test ./controllers/` → PASS.
- [ ] **Step 5: Confirm changed files** — the two controller files only.

---

### Task 7: Chart — RBAC, env, values, version; `make run` support

**Files:**
- Modify: `charts/frp-operator/templates/clusterrole.yaml`, `charts/frp-operator/templates/deployment.yaml`, `charts/frp-operator/values.yaml`, `charts/frp-operator/Chart.yaml`, `config/default/manager_auth_proxy_patch.yaml` (no change needed unless it lists args — check), `Makefile` (`run` target)
- Scratch: `$SCRATCH/check-chart-lb.sh`

- [ ] **Step 1: Write the chart assertion script (test first)**

```bash
# $SCRATCH/check-chart-lb.sh
#!/usr/bin/env bash
set -euo pipefail
cd "$(git rev-parse --show-toplevel)"
out=$(helm template frp charts/frp-operator)
grep -q 'serverpools' <<<"$out" || { echo "FAIL: serverpools RBAC missing"; exit 1; }
grep -q 'services/status' <<<"$out" || { echo "FAIL: services/status RBAC missing"; exit 1; }
grep -q 'name: POD_NAMESPACE' <<<"$out" || { echo "FAIL: POD_NAMESPACE env missing"; exit 1; }
grep -q -- '--enable-loadbalancer-controller=true' <<<"$out" || { echo "FAIL: LB flag missing"; exit 1; }
out2=$(helm template frp charts/frp-operator --set loadBalancer.enabled=false)
grep -q -- '--enable-loadbalancer-controller=false' <<<"$out2" || { echo "FAIL: LB flag not driven by values"; exit 1; }
grep -q 'version: 1.9.0' charts/frp-operator/Chart.yaml || { echo "FAIL: chart version"; exit 1; }
grep -q 'appVersion: 0.11.0' charts/frp-operator/Chart.yaml || { echo "FAIL: appVersion"; exit 1; }
grep -q 'tag: "v0.11.0"' charts/frp-operator/values.yaml || { echo "FAIL: operator tag"; exit 1; }
helm lint charts/frp-operator >/dev/null
echo OK
```

Run: `bash $SCRATCH/check-chart-lb.sh` → FAIL on the first assertion.

- [ ] **Step 2: ClusterRole** — in `charts/frp-operator/templates/clusterrole.yaml` add rules (keep existing ones):

```yaml
- apiGroups:
  - ""
  resources:
  - services/status
  verbs:
  - get
  - patch
  - update
- apiGroups:
  - frp.zufardhiyaulhaq.com
  resources:
  - serverpools
  verbs:
  - get
  - list
  - watch
- apiGroups:
  - frp.zufardhiyaulhaq.com
  resources:
  - serverpools/status
  verbs:
  - get
  - patch
  - update
```

- [ ] **Step 3: Deployment** — in `charts/frp-operator/templates/deployment.yaml`, in the `args:` list after `- --leader-elect` add `- --enable-loadbalancer-controller={{ .Values.loadBalancer.enabled }}`; after `imagePullPolicy: Always` add:

```yaml
        env:
        - name: POD_NAMESPACE
          valueFrom:
            fieldRef:
              fieldPath: metadata.namespace
```

- [ ] **Step 4: values.yaml / Chart.yaml**

`values.yaml`: `operator.tag: "v0.11.0"`, and append:

```yaml
loadBalancer:
  # -- Bind `Service type=LoadBalancer` with `loadBalancerClass: frp.zufardhiyaulhaq.com/frp` to `ServerPool` servers
  enabled: true
```

`Chart.yaml`: `version: 1.9.0`, `appVersion: 0.11.0`.

- [ ] **Step 5: `make run` support** — in `Makefile`, change the `run` target's command to `POD_NAMESPACE=$${POD_NAMESPACE:-default} go run ./main.go` so local runs default the operator namespace to `default` (the AGENTS.md dev flow applies examples there).

- [ ] **Step 6: Run** — `bash $SCRATCH/check-chart-lb.sh` → `OK`; `bash $SCRATCH/check-chart.sh` (from the metrics work, if still present) → `OK`.
- [ ] **Step 7: Confirm changed files** — the five chart/Makefile files.

---

### Task 8: Metrics and dashboard row

**Files:**
- Create: `pkg/metrics/loadbalancer.go`, `pkg/metrics/loadbalancer_test.go`
- Modify: `controllers/service_controller.go` (call sites), `dashboards/frp-operator.json`

**Interfaces:**
- Produces: `metrics.RecordLoadBalancer(namespace, service, pool, server, state string)` (`state` ∈ `bound`,`pending`; one series per Service, previous series for that Service deleted first), `metrics.DeleteLoadBalancer(namespace, service string)`, `metrics.SetPoolAllocated(pool, server string, n int)`, `metrics.DeletePool(pool string)`.

- [ ] **Step 1: Write the failing tests**

```go
// pkg/metrics/loadbalancer_test.go
package metrics

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestRecordLoadBalancer_OneSeriesPerService(t *testing.T) {
	t.Cleanup(func() { DeleteLoadBalancer("ns", "web") })
	RecordLoadBalancer("ns", "web", "", "", "pending")
	RecordLoadBalancer("ns", "web", "prod", "sg-01", "bound")
	if n := testutil.CollectAndCount(lbService); n != 1 {
		t.Fatalf("series = %d, want 1", n)
	}
	if v := testutil.ToFloat64(lbService.WithLabelValues("ns", "web", "prod", "sg-01", "bound")); v != 1 {
		t.Fatalf("bound = %v", v)
	}
	DeleteLoadBalancer("ns", "web")
	if n := testutil.CollectAndCount(lbService); n != 0 {
		t.Fatalf("after delete series = %d", n)
	}
}

func TestPoolAllocated(t *testing.T) {
	t.Cleanup(func() { DeletePool("prod") })
	SetPoolAllocated("prod", "sg-01", 2)
	SetPoolAllocated("prod", "sg-02", 0)
	if n := testutil.CollectAndCount(poolAllocated); n != 2 {
		t.Fatalf("series = %d", n)
	}
	if v := testutil.ToFloat64(poolAllocated.WithLabelValues("prod", "sg-01")); v != 2 {
		t.Fatalf("sg-01 = %v", v)
	}
	DeletePool("prod")
	if n := testutil.CollectAndCount(poolAllocated); n != 0 {
		t.Fatalf("after delete series = %d", n)
	}
}
```

- [ ] **Step 2: Run to verify failure** — `go test ./pkg/metrics/` → undefined.

- [ ] **Step 3: Implement**

```go
// pkg/metrics/loadbalancer.go
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	ctrlmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"
)

var (
	lbService = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "frp_loadbalancer_service",
		Help: "LoadBalancer Services handled by the operator; state is bound or pending. Always 1.",
	}, []string{"namespace", "service", "pool", "server", "state"})
	poolAllocated = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "frp_serverpool_allocated_ports",
		Help: "Number of ports allocated on a ServerPool server.",
	}, []string{"pool", "server"})
)

func init() {
	ctrlmetrics.Registry.MustRegister(lbService, poolAllocated)
}

// RecordLoadBalancer writes one frp_loadbalancer_service series per Service, replacing any
// series previously written for it (so state/pool/server flips do not leave stale series).
func RecordLoadBalancer(namespace, service, pool, server, state string) {
	mu.Lock()
	defer mu.Unlock()
	lbService.DeletePartialMatch(prometheus.Labels{"namespace": namespace, "service": service})
	lbService.WithLabelValues(namespace, service, pool, server, state).Set(1)
}

func DeleteLoadBalancer(namespace, service string) {
	mu.Lock()
	defer mu.Unlock()
	lbService.DeletePartialMatch(prometheus.Labels{"namespace": namespace, "service": service})
}

func SetPoolAllocated(pool, server string, n int) {
	mu.Lock()
	defer mu.Unlock()
	poolAllocated.WithLabelValues(pool, server).Set(float64(n))
}

func DeletePool(pool string) {
	mu.Lock()
	defer mu.Unlock()
	poolAllocated.DeletePartialMatch(prometheus.Labels{"pool": pool})
}
```

- [ ] **Step 4: Wire into the reconciler**

In `controllers/service_controller.go` (import `"github.com/zufardhiyaulhaq/frp-operator/pkg/metrics"`):
- In the deletion/unclaimed branch, before removing the finalizer: `metrics.DeleteLoadBalancer(svc.Namespace, svc.Name)`.
- In `setPending`, first line: `metrics.RecordLoadBalancer(svc.Namespace, svc.Name, "", "", "pending")`.
- After the ingress status update in `Reconcile`: `metrics.RecordLoadBalancer(svc.Namespace, svc.Name, refs[0].Pool, refs[0].Server, "bound")` (for multi-server bindings record the first; the pool/server label is informational).
- In `updatePoolStatuses`, inside the per-server loop after computing `st`: `metrics.SetPoolAllocated(pool.Name, s.Name, len(st.AllocatedPorts))`. Pools that disappear keep stale series until operator restart; acceptable and noted in the release notes.

- [ ] **Step 5: Dashboard row**

Edit `dashboards/frp-operator.json` with a short Python script (stdlib only): load JSON, compute `y = max(p.gridPos.y + p.gridPos.h)` over panels, append these panels with ids starting at `max(id)+1`, save with `indent=2` and a trailing newline:

1. `row` "LoadBalancer", `gridPos {h:1,w:24,x:0,y:y}`.
2. `stat` "Bound services": expr `count(frp_loadbalancer_service{state="bound"}) or vector(0)`, `gridPos {h:4,w:6,x:0,y:y+1}`, datasource `${datasource}`.
3. `stat` "Pending services": expr `count(frp_loadbalancer_service{state="pending"}) or vector(0)`, `gridPos {h:4,w:6,x:6,y:y+1}`, thresholds red when > 0.
4. `table` "Services": expr `frp_loadbalancer_service`, `format: table`, `instant: true`, transformations `[organize(exclude __name__,Time,job,instance,pod,service_,endpoint,container,prometheus,Value)]` — note the operator's own scrape adds a `service` label that collides with ours because `honorLabels: true` keeps ours; verify in Grafana that the column shown is the Service name. `gridPos {h:8,w:12,x:12,y:y+1}`.
5. `bargauge` "Allocated ports per server": expr `frp_serverpool_allocated_ports`, legend `{{pool}}/{{server}}`, `gridPos {h:4,w:12,x:0,y:y+5}`.

Copy the `datasource`, `fieldConfig` and `options` blocks from the existing "Clients" stat/table panels so styling matches. Validate: `python3 -c "import json;json.load(open('dashboards/frp-operator.json'))"`.

- [ ] **Step 6: Run** — `go build ./... && go vet ./... && go test ./pkg/metrics/ ./controllers/` → PASS.
- [ ] **Step 7: Confirm changed files** — metrics files, controller, dashboard JSON.

---

### Task 9: Examples, docs, release notes

**Files:**
- Create: `examples/loadbalancer/README.md`, `examples/loadbalancer/serverpool.yaml`, `examples/loadbalancer/service-auto.yaml`, `examples/loadbalancer/service-pinned.yaml`, `examples/loadbalancer/service-multi.yaml`, `docs/releases/v0.11.0.md`
- Modify: `charts/frp-operator/README.md.gotmpl`, `README.md` + `charts/frp-operator/README.md` (regenerated by `make readme`), `AGENTS.md`

- [ ] **Step 1: Examples**

`examples/loadbalancer/serverpool.yaml`:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: sg-01-token
  namespace: frp-operator
stringData:
  token: change-me
---
apiVersion: frp.zufardhiyaulhaq.com/v1alpha1
kind: ServerPool
metadata:
  name: prod
  namespace: frp-operator          # must be the operator's namespace
spec:
  allocationPolicy: Auto           # Auto (default) | Explicit
  clientMode: PerService           # PerService (default) | Shared
  allowedPorts: ["80", "443", "8000-9000"]
  servers:
  - name: sg-01
    host: 178.128.100.87
    port: 7000
    publicAddress: 178.128.100.87
    authentication:
      token:
        secret:
          name: sg-01-token
          key: token
  - name: sg-02
    host: 178.128.100.88
    port: 7000
    publicAddress: lb2.example.com
    allowedPorts: ["443"]
    authentication:
      token:
        secret:
          name: sg-01-token
          key: token
```

`service-auto.yaml`:

```yaml
apiVersion: v1
kind: Service
metadata:
  name: web-auto
  namespace: default
spec:
  type: LoadBalancer
  loadBalancerClass: frp.zufardhiyaulhaq.com/frp
  selector:
    app: web
  ports:
  - name: https
    port: 443
    targetPort: 8443
    protocol: TCP
```

`service-pinned.yaml`: same with name `web-pinned`, annotations `frp.zufardhiyaulhaq.com/server-pool: prod` and `frp.zufardhiyaulhaq.com/server: sg-01`, port 8080.

`service-multi.yaml`: same with name `web-multi`, annotation `frp.zufardhiyaulhaq.com/server: sg-01,sg-02`, port 443.

`README.md`: how to apply, what `kubectl get svc` shows (`<pending>` until the frpc client is Ready and the proxy is running), how to read `kubectl describe svc` Events, and `kubectl get serverpool -n frp-operator -o yaml` for allocations.

- [ ] **Step 2: Chart README template** — in `charts/frp-operator/README.md.gotmpl` add a feature bullet under `## Features` (`- Expose \`Service type=LoadBalancer\` through a pool of frps servers (\`ServerPool\` + \`loadBalancerClass: frp.zufardhiyaulhaq.com/frp\`)`) and a new section before `## Monitoring`:

```markdown
## LoadBalancer Services

Create a `ServerPool` in the operator namespace listing your frps servers, then create Services with `type: LoadBalancer` and `loadBalancerClass: frp.zufardhiyaulhaq.com/frp`. The operator picks a server, generates the `Client`/`Upstream` resources in the operator namespace and writes the server's `publicAddress` to `status.loadBalancer.ingress`.

| Annotation on the Service | Meaning |
|---|---|
| `frp.zufardhiyaulhaq.com/server-pool: <name>` | Use only this pool (required for pools with `allocationPolicy: Explicit`) |
| `frp.zufardhiyaulhaq.com/server: sg-01[,sg-02]` | Bind to exactly these servers; one ingress IP each, all-or-nothing |
| `frp.zufardhiyaulhaq.com/allocated-server` | Written by the operator; the current binding |

Rules:
- A Service gets **one** server (one IP) unless it pins several. All its ports land on that server with `remotePort` = Service port.
- Pools are tried in name order, servers in spec order; the first server with every requested port free (and allowed by `allowedPorts`) wins. Bindings are sticky: a bound Service never moves unless its server is removed from the pool.
- If no server fits, the Service stays `<pending>` with an Event (`PortUnavailable`, `PortNotAllowed`, `NoServerAvailable`, …). First come, first served: a running Service never loses its IP to another one. Delete the holder and the pending Service binds within seconds.
- The ingress address appears only after the generated frpc client is `Ready` and every proxy reports `running`.
- `clientMode: Shared` runs one frpc pod per server for all Services; `PerService` (default) runs one per Service per server.
- Only TCP and UDP ports are supported. Set `loadBalancer.enabled=false` to turn the controller off.

See [`examples/loadbalancer`](https://github.com/zufardhiyaulhaq/frp-operator/tree/main/examples/loadbalancer).
```

Add `frp_loadbalancer_service` and `frp_serverpool_allocated_ports` rows to the Monitoring metrics table.

Run: `make readme` (helm-docs) → `README.md` and `charts/frp-operator/README.md` regenerated; `git diff --stat` shows both.

- [ ] **Step 3: AGENTS.md** — add under CRDs: `- **ServerPool**: pool of frps servers used to back \`Service type=LoadBalancer\` (loadBalancerClass \`frp.zufardhiyaulhaq.com/frp\`)`; under Controllers: `**ServiceReconciler** binds claimed LoadBalancer Services to ServerPool servers and generates Client/Upstream CRs in the operator namespace (pure selection logic in \`pkg/loadbalancer/\`).`; in the directory tree add `pkg/loadbalancer/  # LB allocator, naming, builders`; note `POD_NAMESPACE`/`--operator-namespace` under Development Workflow (`make run` defaults it to `default`).

- [ ] **Step 4: Release notes** `docs/releases/v0.11.0.md`, same format as `docs/releases/v0.9.0.md`:

```markdown
Releasing v0.11.0 FRP Operator 🥳🥳

Helm chart 1.9.0.

Changes:
1. `Service type=LoadBalancer` support. Create a `ServerPool` in the operator namespace and set `loadBalancerClass: frp.zufardhiyaulhaq.com/frp` on a Service; the operator picks an frps server, generates the `Client`/`Upstream` resources and writes the server's public address to `status.loadBalancer.ingress`. Pin servers with `frp.zufardhiyaulhaq.com/server: sg-01[,sg-02]`, choose a pool with `frp.zufardhiyaulhaq.com/server-pool`. See the README "LoadBalancer Services" section.
2. New CRD `ServerPool` (`allocationPolicy`, `clientMode`, `allowedPorts`, `servers[]`, `clientTemplate`).
3. New metrics `frp_loadbalancer_service` and `frp_serverpool_allocated_ports`, plus a LoadBalancer row in the Grafana dashboard. Series for a deleted pool remain until the operator restarts.
4. Chart: `loadBalancer.enabled` (default `true`), `POD_NAMESPACE` env, RBAC for `services/status` and `serverpools`.

## How to Upgrade

Helm by default doesn't update the CRDs, you need to apply the CRDs manually:
```
kubectl apply -f https://raw.githubusercontent.com/zufardhiyaulhaq/frp-operator/v0.11.0/charts/frp-operator/crds/crds.yaml
```

The controller only touches Services that carry our `loadBalancerClass`; existing Services and hand-written `Client`/`Upstream` resources are unaffected.

**Full Changelog**: https://github.com/zufardhiyaulhaq/frp-operator/compare/v0.10.0...v0.11.0
```

- [ ] **Step 5: Verify** — `helm lint charts/frp-operator`; `bash $SCRATCH/check-chart-lb.sh` → OK; `go build ./... && go test ./...` → PASS; `kubectl --context orbstack apply --dry-run=client -f examples/loadbalancer/` (after `make install` on orbstack) → four objects validate.
- [ ] **Step 6: Confirm changed files** — examples, READMEs, AGENTS.md, release notes.

---

### Task 10: Manual verification on home-lab-kubernetes-01 (user-assisted)

Not a subagent task — the controller performs it with the user's cluster after the build is green.

- [ ] Build & push image `v0.11.0` (user's release process) or `make run` locally with `POD_NAMESPACE=<operator ns>` against the cluster.
- [ ] Apply a `ServerPool` with the user's existing frps (`host`, `token` Secret, `publicAddress`).
- [ ] Apply `examples/loadbalancer/service-pinned.yaml` adapted to a real port; confirm `kubectl get svc` shows the IP after the generated Client is Ready, and `curl` reaches the backend.
- [ ] Apply a second Service on the same port → `<pending>` with a `PortUnavailable` Event; delete the first → second binds; recreate the first → `<pending>`.
- [ ] Check the Grafana "LoadBalancer" row populates.

---

## Self-review

**Spec coverage:** API (Task 1), annotations/naming/allowedPorts (2), selection rules incl. ambiguity, Explicit pools, all-or-nothing (3), builders/labels/clientID (4), reconcile steps 1–8, sticky binding, reallocation, pool deletion, race check, ingress gating, pool status/Ready condition, class change = deletion, shared-client lifecycle (5), watches (6), chart/RBAC/env/flag (7), metrics + dashboard (8), examples/README/AGENTS/release notes (9), manual test (10). `UnsupportedProtocol` for SCTP: Task 3. Hash-suffixed names: Task 2. `--operator-namespace` for `make run`: Tasks 5 and 7.

**Placeholder scan:** none.

**Type consistency:** `loadbalancer.Request/Allocation/Reason/ServerRef`, `Allocate/Fit/Winner/FindPool/FindServer`, `BuildClient/BuildUpstream/AllocationFromUpstream`, `ClientName/ClientID/UpstreamName/ServiceHost/ServiceKey`, `ParsePortRanges/PortAllowed`, `ServiceReconciler{StatusFunc}`, `ClaimedServiceRequests`, `serviceOfInterest`, metrics `RecordLoadBalancer/DeleteLoadBalancer/SetPoolAllocated/DeletePool` — used with the same names and signatures across tasks. `frpv1alpha1.ClientModeShared/PerService`, `AllocationPolicyAuto/Explicit`, `PortRange` come from Task 1.
{% endraw %}
