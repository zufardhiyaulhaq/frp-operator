# E2E Tests in GitHub Actions (Tier 1) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** On every PR, prove that Tier 1 frp-operator features work end to end: Client status, operator metrics, and real traffic through an in-cluster frps.

**Architecture:** `make test-e2e` creates its own k3d cluster with a private kubeconfig, imports a locally built operator image, and installs the operator from the repo's Helm chart. A Go test suite behind the `e2e` build tag then deploys frps, echo backends, an egress proxy and a client pod. Each feature case gets its own namespace and Client, and runs traffic from the client pod to the frps pod IP. The same make target runs in a new GitHub Actions job on PRs and `main`.

**Tech Stack:** Go 1.23, client-go / controller-runtime client (already in `go.mod`), k3d v5.9.0, k3s `v1.33.7-k3s1`, Helm 3, frps `v0.71.0`, `alpine/socat`, `mendhak/http-https-echo`, `nicolaka/netshoot`, `tarampampam/3proxy`, `nginx`.

**Spec:** `docs/superpowers/specs/2026-10-10-e2e-github-workflow-design.md`

## Global Constraints

- Go `1.23` (CI pins it). Add **no new modules**. Everything comes from modules already in `go.mod`: `k8s.io/api`, `k8s.io/apimachinery`, `k8s.io/client-go`, `sigs.k8s.io/controller-runtime`, `sigs.k8s.io/yaml`. Run `go mod tidy` only to promote indirect deps.
- Pinned images: `fatedier/frps:v0.71.0` (must match `frpcImage` in `controllers/client_controller.go`), `alpine/socat:1.8.0.3`, `mendhak/http-https-echo:42`, `nicolaka/netshoot:v0.14`, `tarampampam/3proxy:2.3.0`, `nginx:1.27-alpine`, `rancher/k3s:v1.33.7-k3s1`. k3d CLI `v5.9.0`.
- **Context safety:**
  - The suite only talks to the kubeconfig in `E2E_KUBECONFIG`, which `make e2e-up` writes from k3d.
  - `requireE2EContext` refuses anything except a kubeconfig with exactly one context, `k3d-frp-operator-e2e`.
  - Never read, write or switch `~/.kube/config`: k3d runs with `--kubeconfig-update-default=false --kubeconfig-switch-context=false`.
- **frps proxy names are global per frps.** Every Upstream and Visitor is named `<case-namespace>-<short>` through `Case.Upstream` / `Case.Visitor`. Never create one directly.
- **Remote ports** come only from `test/e2e/ports.go`, and every case owns its own.
- **TLS Secrets** consumed by frpc must use the keys `tls.crt`, `tls.key` and `ca.crt`. The operator mounts the whole Secret at `/etc/frp/tls` and renders those fixed paths, so the CR's `key` field is ignored.
- **Timeouts:**

  | Wait | Timeout |
  |---|---|
  | Infra namespace ready | 4 min |
  | Client condition | 3 min |
  | Proxy running in metrics | 3 min |
  | Traffic check | retried for 30 s |
  | `go test` | `-timeout 15m` |
  | CI job | `timeout-minutes: 20` |

  `-count=1`, no automatic re-runs.
- **Deviations from the spec:**
  - Q27's "proxy running 60 s" becomes **3 min**. The operator only re-reads Upstreams on its 30 s requeue, and a reload also waits for the kubelet to sync the ConfigMap volume, which takes up to about 1 min.
  - The proxy password is **`p@ss/w0rd`**, because 3proxy's `users` syntax is `user:CL:password` and can't contain `:`.
- **Build tags:** suite files are `*_test.go` with `//go:build e2e`. Pure helpers (`guard.go`, `metrics.go`, `pods.go`, `ports.go`) are untagged so `make test` unit-tests them.
- **Commits:** end every commit message with the attribution line the host session specifies.

## Review Focus

1. **Docker Hub rate limit or a typo'd image tag** on a CI runner. Expected: setup fails within 4 min with a message naming the pod, container and `ImagePullBackOff` reason, not a bare timeout. Pinned by `TestPodProblem` (Task 3).
2. **Re-running `make test-e2e` on an existing cluster** (local dev, or after a failed run kept its namespaces). Expected: the run succeeds. Infra is applied idempotently, `newCase` deletes a leftover namespace first, and the LoadBalancer case deletes a leftover Service. Pinned by running the suite twice in Task 5 Step 7 and Task 14 Step 5.
3. **`E2E_KUBECONFIG` pointing at a kubeconfig that holds production contexts.** Expected: the suite refuses before touching any cluster. Pinned by `TestRequireE2EContext` (Task 3).
4. **Two parallel cases colliding on one frps** (same remote port). Expected: impossible by construction. Pinned by `TestPortsAreUnique` (Task 3).
5. **Slow InvalidConfig recovery.** The operator doesn't watch Secrets, so after the Secret is fixed, recovery waits for its error backoff. Expected: recovery within 3 min of the fix. Pinned by `TestInvalidConfigThenRecovery`'s 3 min wait (Task 13). If it flakes, that is an operator bug to park, not a timeout to raise.

---

## File Structure

| File | Responsibility |
|---|---|
| `charts/frp-operator/values.yaml`, `templates/deployment.yaml` | New `operator.imagePullPolicy` value |
| `api/v1alpha1/client_types.go`, `serverpool_types.go` (+ generated CRDs, chart `crds.yaml`) | `wss` needs a TLS terminator: field docs |
| `charts/frp-operator/README.md.gotmpl` (+ generated READMEs) | Feature list line for `wss`; values table |
| `Makefile` | `e2e-image`, `e2e-up`, `test-e2e`, `e2e-down` |
| `test/e2e/guard.go` | `ContextName`, `requireE2EContext` (untagged) |
| `test/e2e/metrics.go` | `metricValue` Prometheus text parser (untagged) |
| `test/e2e/pods.go` | `podReady`, `podProblem` (untagged) |
| `test/e2e/ports.go` | Remote port per case, `allPorts` (untagged) |
| `test/e2e/manifests/infra.yaml` | frps-main, frps-mtls, wss-terminator, backends, 3proxy, client pod (Go template) |
| `test/e2e/setup_test.go` | `TestMain`, `Env`, infra setup, global dump |
| `test/e2e/certs_test.go` | `Certs`, `newCerts` |
| `test/e2e/kube_test.go` | `apply`, `upsert`, `deleteAndWait`, `exec`, `logs`, dumps |
| `test/e2e/traffic_test.go` | `eventually`, `expectTCPEcho`, `expectUDPEcho`, `expectNoTCP`, `curl` |
| `test/e2e/case_test.go` | `Case`, `newCase`, Client/Upstream/Visitor builders, waits |
| `test/e2e/*_feature_test.go` | One file per feature area (Tasks 5–13) |
| `.github/workflows/pullrequest.yml`, `main.yml` | New `e2e` job (replaces old one in `main.yml`) |
| `AGENTS.md` | Document the targets |

---

### Before Task 1: Branch

- [ ] Branch off the latest `main` and commit the spec and this plan:

```bash
git fetch origin && git switch -c feat/e2e-github-workflow origin/main
git add docs/superpowers/specs/2026-10-10-e2e-github-workflow-design.md docs/superpowers/plans/2026-10-10-e2e-github-workflow.md
git commit -m "docs: e2e-in-GitHub-Actions design and implementation plan"
```

Every task below commits on this branch. The branch ends with a PR; merging is the owner's call, after `code-review`.

---

### Task 1: Chart value `operator.imagePullPolicy`

**Files:**
- Modify: `charts/frp-operator/values.yaml` (the `operator:` block)
- Modify: `charts/frp-operator/templates/deployment.yaml:39`
- Regenerate: `README.md`, `charts/frp-operator/README.md` (via `make readme`)

**Interfaces:**
- Produces: Helm value `operator.imagePullPolicy` (string, default `Always`). Task 4 sets it to `IfNotPresent`.

- [ ] **Step 1: Write the failing check**

```bash
helm template t charts/frp-operator | grep -c 'imagePullPolicy: Always'
helm template t charts/frp-operator --set operator.imagePullPolicy=IfNotPresent | grep -c 'imagePullPolicy: IfNotPresent'
```

- [ ] **Step 2: Run it to verify it fails**

Expected: the first prints `1`. The second prints `0` and exits 1, because the template hardcodes `Always`.

- [ ] **Step 3: Implement**

In `charts/frp-operator/values.yaml`, replace the `operator:` block with:

```yaml
operator:
  image: "ghcr.io/zufardhiyaulhaq/frp-operator"
  tag: "v0.12.1"
  # -- Image pull policy for the operator. `Always` keeps moving tags such as `main` fresh; use `IfNotPresent` for a locally imported image (the e2e suite does).
  imagePullPolicy: Always
  replica: 1
```

In `charts/frp-operator/templates/deployment.yaml`, replace line 39:

```yaml
        imagePullPolicy: {{ .Values.operator.imagePullPolicy | default "Always" }}
```

- [ ] **Step 4: Run the check to verify it passes, then regenerate docs**

```bash
helm template t charts/frp-operator | grep -c 'imagePullPolicy: Always'
helm template t charts/frp-operator --set operator.imagePullPolicy=IfNotPresent | grep -c 'imagePullPolicy: IfNotPresent'
helm lint charts/frp-operator
make readme
grep -c 'operator.imagePullPolicy' README.md charts/frp-operator/README.md
```

Expected: `1`, `1`, `1 chart(s) linted, 0 chart(s) failed`, and both READMEs `1`.

- [ ] **Step 5: Commit**

```bash
git add charts/frp-operator/values.yaml charts/frp-operator/templates/deployment.yaml README.md charts/frp-operator/README.md
git commit -m "feat(chart): add operator.imagePullPolicy value (default Always)"
```

---

### Task 2: Document that `wss` needs a TLS terminator

**Files:**
- Modify: `api/v1alpha1/client_types.go` (the `Protocol` field of `ClientSpec_Server`)
- Modify: `api/v1alpha1/serverpool_types.go` (the `TransportProtocol` comment)
- Modify: `charts/frp-operator/README.md.gotmpl` (feature list)
- Regenerate: `config/crd/bases/*`, `charts/frp-operator/crds/crds.yaml`, READMEs

**Interfaces:** none (docs only).

- [ ] **Step 1: Write the failing check**

```bash
grep -c "TLS terminator" config/crd/bases/frp.zufardhiyaulhaq.com_clients.yaml config/crd/bases/frp.zufardhiyaulhaq.com_serverpools.yaml charts/frp-operator/crds/crds.yaml README.md
```

- [ ] **Step 2: Run it to verify it fails**

Expected: every file prints `0`.

- [ ] **Step 3: Implement**

In `api/v1alpha1/client_types.go`, replace:

```go
	// +kubebuilder:validation:Enum=tcp;kcp;quic;websocket;wss
	// +optional
	Protocol       *string                          `json:"protocol,omitempty"`
```

with:

```go
	// +kubebuilder:validation:Enum=tcp;kcp;quic;websocket;wss
	// +optional
	// Protocol is how frpc connects to frps: tcp (default), kcp, quic, websocket or wss.
	// frps itself only accepts ws, so wss needs a TLS terminator (for example nginx or a cloud
	// load balancer) in front of frps that forwards plain websocket to frps's bindPort.
	Protocol       *string                          `json:"protocol,omitempty"`
```

In `api/v1alpha1/serverpool_types.go`, replace:

```go
	// TransportProtocol is the frpc→frps transport (Client.spec.server.protocol).
```

with:

```go
	// TransportProtocol is the frpc→frps transport (Client.spec.server.protocol). wss needs a
	// TLS terminator in front of frps, because frps itself only accepts ws.
```

In `charts/frp-operator/README.md.gotmpl`, directly after the line starting `- Transport tuning on \`Client\``, add:

```markdown
- `protocol: wss` needs a TLS terminator in front of frps (frps itself only accepts websocket); the e2e suite runs nginx for this
```

- [ ] **Step 4: Regenerate and run the check to verify it passes**

```bash
make manifests generate
cat config/crd/bases/frp.zufardhiyaulhaq.com_{clients,upstreams,visitors,serverpools}.yaml > charts/frp-operator/crds/crds.yaml
make readme
grep -c "TLS terminator" config/crd/bases/frp.zufardhiyaulhaq.com_clients.yaml config/crd/bases/frp.zufardhiyaulhaq.com_serverpools.yaml charts/frp-operator/crds/crds.yaml README.md
git diff --stat
```

Expected: every count ≥ 1. The diff only touches the two type files, the CRDs, `crds.yaml`, the `.gotmpl` and the two READMEs. Descriptions only, no schema change.

- [ ] **Step 5: Commit**

```bash
git add api/ config/crd charts/frp-operator/crds charts/frp-operator/README.md.gotmpl README.md charts/frp-operator/README.md
git commit -m "docs: protocol wss needs a TLS terminator in front of frps"
```

---

### Task 3: Pure e2e helpers (guard, metrics parser, pod diagnosis, ports)

**Files:**
- Create: `test/e2e/guard.go`, `test/e2e/guard_test.go`
- Create: `test/e2e/metrics.go`, `test/e2e/metrics_test.go`
- Create: `test/e2e/pods.go`, `test/e2e/pods_test.go`
- Create: `test/e2e/ports.go`, `test/e2e/ports_test.go`

**Interfaces:**
- Produces (package `e2e`, untagged):
  - `const ContextName = "k3d-frp-operator-e2e"`
  - `func requireE2EContext(cfg *clientcmdapi.Config) error`
  - `func metricValue(body, name string, labels map[string]string) (float64, bool)`
  - `func podReady(p *corev1.Pod) bool`
  - `func podProblem(p *corev1.Pod) string`
  - Port constants `portTCP` … `portLoadBalancer`, and `func allPorts() []int`

- [ ] **Step 1: Write the failing tests**

`test/e2e/guard_test.go`:

```go
package e2e

import (
	"strings"
	"testing"

	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

func kubeconfig(current string, contexts ...string) *clientcmdapi.Config {
	cfg := clientcmdapi.NewConfig()
	for _, name := range contexts {
		cfg.Contexts[name] = &clientcmdapi.Context{Cluster: name, AuthInfo: name}
	}
	cfg.CurrentContext = current
	return cfg
}

func TestRequireE2EContext(t *testing.T) {
	prod := "arn:aws:eks:us-east-2:111111111111:cluster/prod"
	tests := []struct {
		name    string
		cfg     *clientcmdapi.Config
		wantErr string
	}{
		{name: "the e2e cluster only", cfg: kubeconfig(ContextName, ContextName)},
		{name: "a production context is current", cfg: kubeconfig(prod, ContextName, prod), wantErr: "current kubeconfig context"},
		{name: "e2e is current but other contexts exist", cfg: kubeconfig(ContextName, ContextName, "orbstack"), wantErr: "2 contexts"},
		{name: "current context is not defined", cfg: kubeconfig(ContextName), wantErr: "not defined"},
		{name: "empty kubeconfig", cfg: kubeconfig(""), wantErr: "current kubeconfig context"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := requireE2EContext(tt.cfg)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("requireE2EContext() = %v, want nil", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("requireE2EContext() = %v, want an error containing %q", err, tt.wantErr)
			}
		})
	}
}
```

`test/e2e/metrics_test.go`:

```go
package e2e

import "testing"

func TestMetricValue(t *testing.T) {
	body := `# HELP frp_proxy_status One-hot proxy phase
# TYPE frp_proxy_status gauge
frp_client_config_synced_total 3
frp_proxy_status{client="frpc",namespace="e2e-tcp",proxy="e2e-tcp-echo",status="running",type="tcp"} 1
frp_proxy_status{client="frpc",namespace="e2e-tcp",proxy="e2e-tcp-echo",status="start error",type="tcp"} 0
frp_client_config_synced{client="frpc",namespace="e2e-tcp"} 0
`
	tests := []struct {
		name   string
		metric string
		labels map[string]string
		want   float64
		ok     bool
	}{
		{"running phase", "frp_proxy_status", map[string]string{"namespace": "e2e-tcp", "proxy": "e2e-tcp-echo", "status": "running"}, 1, true},
		{"label value with a space", "frp_proxy_status", map[string]string{"proxy": "e2e-tcp-echo", "status": "start error"}, 0, true},
		{"unknown proxy", "frp_proxy_status", map[string]string{"proxy": "nope"}, 0, false},
		{"label values match exactly, not by prefix", "frp_proxy_status", map[string]string{"proxy": "e2e-tcp"}, 0, false},
		{"a longer metric name is not matched", "frp_client_config_synced", nil, 0, true},
		{"metric without labels", "frp_client_config_synced_total", nil, 3, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := metricValue(body, tt.metric, tt.labels)
			if ok != tt.ok || got != tt.want {
				t.Fatalf("metricValue() = (%v, %v), want (%v, %v)", got, ok, tt.want, tt.ok)
			}
		})
	}
}
```

`test/e2e/pods_test.go`:

```go
package e2e

import (
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func pod(status corev1.PodStatus) *corev1.Pod {
	return &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: "backends", Name: "echo-abc"}, Status: status}
}

func TestPodReady(t *testing.T) {
	ready := pod(corev1.PodStatus{Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue}}})
	notReady := pod(corev1.PodStatus{Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionFalse}}})
	if !podReady(ready) || podReady(notReady) || podReady(pod(corev1.PodStatus{})) {
		t.Fatal("podReady must be true only for a PodReady=True condition")
	}
}

func TestPodProblem(t *testing.T) {
	tests := []struct {
		name   string
		status corev1.PodStatus
		want   []string
	}{
		{
			name: "image pull failure names the container and reason",
			status: corev1.PodStatus{ContainerStatuses: []corev1.ContainerStatus{{
				Name:  "tcp",
				State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "ImagePullBackOff", Message: "toomanyrequests: rate limit"}},
			}}},
			want: []string{"backends/echo-abc", "container tcp", "ImagePullBackOff", "toomanyrequests"},
		},
		{
			name: "crashed container",
			status: corev1.PodStatus{ContainerStatuses: []corev1.ContainerStatus{{
				Name:  "frps",
				State: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{Reason: "Error", ExitCode: 1}},
			}}},
			want: []string{"container frps terminated", "exit 1"},
		},
		{
			name:   "not scheduled",
			status: corev1.PodStatus{Conditions: []corev1.PodCondition{{Type: corev1.PodScheduled, Status: corev1.ConditionFalse, Message: "0/1 nodes are available"}}},
			want:   []string{"not scheduled", "0/1 nodes"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := podProblem(pod(tt.status))
			for _, w := range tt.want {
				if !strings.Contains(got, w) {
					t.Errorf("podProblem() = %q, want it to contain %q", got, w)
				}
			}
		})
	}
}
```

`test/e2e/ports_test.go`:

```go
package e2e

import "testing"

func TestPortsAreUnique(t *testing.T) {
	seen := map[int]bool{}
	for _, p := range allPorts() {
		if seen[p] {
			t.Fatalf("remote port %d is used by two cases; cases share one frps and run in parallel", p)
		}
		seen[p] = true
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./test/e2e/`
Expected: build failure, `undefined: requireE2EContext` (and the other helpers).

- [ ] **Step 3: Implement**

`test/e2e/guard.go`:

```go
// Package e2e is the end-to-end suite for frp-operator. The suite runs behind the `e2e` build
// tag (`make test-e2e`); the untagged files hold pure helpers that `make test` unit-tests.
package e2e

import (
	"fmt"

	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

// ContextName is the only kubeconfig context the suite talks to: the k3d cluster `make e2e-up`
// creates.
const ContextName = "k3d-frp-operator-e2e"

// requireE2EContext refuses any kubeconfig except the one `make e2e-up` writes: exactly one
// context, named ContextName, and current. The suite creates namespaces and NetworkPolicies, so
// it must never reach another cluster by accident.
func requireE2EContext(cfg *clientcmdapi.Config) error {
	if cfg.CurrentContext != ContextName {
		return fmt.Errorf("refusing to run: current kubeconfig context is %q, want %q", cfg.CurrentContext, ContextName)
	}
	if _, ok := cfg.Contexts[ContextName]; !ok {
		return fmt.Errorf("refusing to run: context %q is not defined in the kubeconfig", ContextName)
	}
	if len(cfg.Contexts) != 1 {
		return fmt.Errorf("refusing to run: kubeconfig has %d contexts, want only %q (use the file written by make e2e-up, not ~/.kube/config)", len(cfg.Contexts), ContextName)
	}
	return nil
}
```

`test/e2e/metrics.go`:

```go
package e2e

import (
	"regexp"
	"strconv"
	"strings"
)

var labelPair = regexp.MustCompile(`([a-zA-Z_][a-zA-Z0-9_]*)="((?:[^"\\]|\\.)*)"`)

// metricValue returns the value of the first sample of metric `name` in a Prometheus text
// exposition whose labels include every entry of `labels`.
func metricValue(body, name string, labels map[string]string) (float64, bool) {
	for _, line := range strings.Split(body, "\n") {
		var labelSet, rest string
		switch {
		case strings.HasPrefix(line, name+"{"):
			end := strings.LastIndex(line, "}")
			if end < 0 {
				continue
			}
			labelSet, rest = line[len(name)+1:end], line[end+1:]
		case strings.HasPrefix(line, name+" "):
			rest = line[len(name):]
		default:
			continue
		}
		if !hasLabels(labelSet, labels) {
			continue
		}
		fields := strings.Fields(rest)
		if len(fields) == 0 {
			continue
		}
		value, err := strconv.ParseFloat(fields[0], 64)
		if err != nil {
			continue
		}
		return value, true
	}
	return 0, false
}

func hasLabels(labelSet string, want map[string]string) bool {
	got := map[string]string{}
	for _, m := range labelPair.FindAllStringSubmatch(labelSet, -1) {
		got[m[1]] = m[2]
	}
	for k, v := range want {
		if got[k] != v {
			return false
		}
	}
	return true
}
```

`test/e2e/pods.go`:

```go
package e2e

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
)

func podReady(p *corev1.Pod) bool {
	for _, c := range p.Status.Conditions {
		if c.Type == corev1.PodReady {
			return c.Status == corev1.ConditionTrue
		}
	}
	return false
}

// podProblem says why a pod is not ready, so a setup timeout names the cause (for example
// ImagePullBackOff from a Docker Hub rate limit) instead of only "timed out".
func podProblem(p *corev1.Pod) string {
	where := p.Namespace + "/" + p.Name
	statuses := append(append([]corev1.ContainerStatus{}, p.Status.InitContainerStatuses...), p.Status.ContainerStatuses...)
	for _, cs := range statuses {
		if w := cs.State.Waiting; w != nil {
			return fmt.Sprintf("%s: container %s waiting: %s %s", where, cs.Name, w.Reason, w.Message)
		}
		if term := cs.State.Terminated; term != nil {
			return fmt.Sprintf("%s: container %s terminated: %s (exit %d)", where, cs.Name, term.Reason, term.ExitCode)
		}
		if !cs.Ready {
			return fmt.Sprintf("%s: container %s not ready", where, cs.Name)
		}
	}
	for _, c := range p.Status.Conditions {
		if c.Type == corev1.PodScheduled && c.Status != corev1.ConditionTrue {
			return fmt.Sprintf("%s: not scheduled: %s", where, c.Message)
		}
	}
	return fmt.Sprintf("%s: phase %s", where, p.Status.Phase)
}
```

`test/e2e/ports.go`:

```go
package e2e

// Remote ports on frps. Cases share one frps and run in parallel, so every case owns its ports.
// HTTP, HTTPS and TCPMUX cases use frps's vhost ports and are routed by domain instead.
const (
	portTCP          = 20001
	portUDP          = 20002
	portEnabled      = 20003
	portDisabled     = 20004
	portKCP          = 20011
	portQUIC         = 20012
	portWebsocket    = 20013
	portWSS          = 20014
	portTLS          = 20015
	portMTLS         = 20016 // on frps-mtls
	portProxyHTTP    = 20021
	portProxySOCKS   = 20022
	portReloadFirst  = 20031
	portReloadSecond = 20032
	portInvalid      = 20041
	portLoadBalancer = 5353 // TCP and UDP; for LoadBalancer Services the Service port is the remote port
)

func allPorts() []int {
	return []int{
		portTCP, portUDP, portEnabled, portDisabled,
		portKCP, portQUIC, portWebsocket, portWSS, portTLS, portMTLS,
		portProxyHTTP, portProxySOCKS, portReloadFirst, portReloadSecond, portInvalid,
		portLoadBalancer,
	}
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./test/e2e/ -v && go vet ./test/e2e/ && make test`
Expected: `TestRequireE2EContext`, `TestMetricValue`, `TestPodReady`, `TestPodProblem` and `TestPortsAreUnique` PASS. `make test` lists `test/e2e` as `ok`.

- [ ] **Step 5: Commit**

```bash
git add test/e2e/guard.go test/e2e/guard_test.go test/e2e/metrics.go test/e2e/metrics_test.go test/e2e/pods.go test/e2e/pods_test.go test/e2e/ports.go test/e2e/ports_test.go
git commit -m "test(e2e): add context guard, metrics parser, pod diagnosis and port table"
```

---

### Task 4: `make e2e-up` / `e2e-down` (own k3d cluster, chart install)

**Files:**
- Modify: `Makefile` (add an `##@ E2E` section after the `test-frpc-config` target)

**Interfaces:**
- Consumes: `operator.imagePullPolicy` (Task 1).
- Produces:
  - Make targets `e2e-image`, `e2e-up`, `e2e-down` (Task 5 adds `test-e2e`).
  - Variables `E2E_KUBECONFIG` (default `./output/e2e-kubeconfig`), `E2E_ARTIFACTS_DIR` (`./output/e2e-artifacts`) and `E2E_SKIP_IMAGE_BUILD` (set it to skip `docker build`).
  - Release `frp-operator` in namespace `frp-operator`.

- [ ] **Step 1: Write the failing check**

```bash
make -n e2e-up
```

- [ ] **Step 2: Run it to verify it fails**

Expected: `make: *** No rule to make target 'e2e-up'.`

- [ ] **Step 3: Implement**

Append to `Makefile`, directly after the `test-frpc-config` target:

```make
##@ E2E

E2E_CLUSTER ?= frp-operator-e2e
E2E_K3S_IMAGE ?= rancher/k3s:v1.33.7-k3s1
E2E_IMG ?= frp-operator:e2e
E2E_KUBECONFIG ?= $(OUT_DIR)/e2e-kubeconfig
E2E_ARTIFACTS_DIR ?= $(OUT_DIR)/e2e-artifacts
E2E_KUBECTL = kubectl --kubeconfig $(E2E_KUBECONFIG)

.PHONY: e2e-image
e2e-image: ## Build the operator image for the e2e cluster (skipped when E2E_SKIP_IMAGE_BUILD is set, as in CI).
ifndef E2E_SKIP_IMAGE_BUILD
	docker build -t $(E2E_IMG) .
endif

.PHONY: e2e-up
e2e-up: e2e-image ## Create the k3d e2e cluster and install the operator from the Helm chart. Never touches ~/.kube/config.
	@k3d cluster get $(E2E_CLUSTER) >/dev/null 2>&1 || k3d cluster create $(E2E_CLUSTER) \
		--image $(E2E_K3S_IMAGE) --no-lb --wait \
		--kubeconfig-update-default=false --kubeconfig-switch-context=false \
		--k3s-arg "--disable=traefik@server:0" --k3s-arg "--disable=servicelb@server:0"
	k3d kubeconfig get $(E2E_CLUSTER) > $(E2E_KUBECONFIG)
	k3d image import $(E2E_IMG) --cluster $(E2E_CLUSTER)
	$(E2E_KUBECTL) apply --server-side -f charts/frp-operator/crds/crds.yaml
	helm upgrade --install frp-operator ./charts/frp-operator --kubeconfig $(E2E_KUBECONFIG) \
		--namespace frp-operator --create-namespace \
		--set operator.image=frp-operator --set operator.tag=e2e --set operator.imagePullPolicy=IfNotPresent \
		--wait --timeout 3m
	$(E2E_KUBECTL) -n frp-operator rollout restart deployment/frp-operator-controller-manager
	$(E2E_KUBECTL) -n frp-operator rollout status deployment/frp-operator-controller-manager --timeout 2m

.PHONY: e2e-down
e2e-down: ## Delete the k3d e2e cluster and its kubeconfig.
	k3d cluster delete $(E2E_CLUSTER)
	rm -f $(E2E_KUBECONFIG)
```

Why each flag matters:
- `--disable=servicelb`: k3s's built-in load balancer must not claim `type: LoadBalancer` Services. The frp-operator has to.
- `--kubeconfig-update-default=false`: keeps `~/.kube/config` untouched.
- `rollout restart`: picks up a re-imported `:e2e` image on reruns.

- [ ] **Step 4: Run it to verify it passes (needs Docker, k3d v5.9.0, helm)**

```bash
make e2e-up
kubectl --kubeconfig output/e2e-kubeconfig config get-contexts -o name
kubectl --kubeconfig output/e2e-kubeconfig -n frp-operator get deploy frp-operator-controller-manager -o jsonpath='{.spec.template.spec.containers[0].image} {.spec.template.spec.containers[0].imagePullPolicy}{"\n"}'
kubectl config get-contexts -o name | grep -c frp-operator-e2e || true
make e2e-up
```

Expected:
- the context list prints only `k3d-frp-operator-e2e`
- the deployment line is `frp-operator:e2e IfNotPresent`
- `~/.kube/config` contains no `frp-operator-e2e` context (count `0`)
- the second `make e2e-up` succeeds (idempotent)

- [ ] **Step 5: Commit**

```bash
git add Makefile
git commit -m "build(e2e): make e2e-up/e2e-down create a private k3d cluster and install the chart"
```

---

### Task 5: Suite foundation + first case (TCP)

**Files:**
- Create: `test/e2e/manifests/infra.yaml`
- Create: `test/e2e/setup_test.go`, `test/e2e/certs_test.go`, `test/e2e/kube_test.go`, `test/e2e/traffic_test.go`, `test/e2e/case_test.go`
- Create: `test/e2e/tcp_feature_test.go`
- Modify: `Makefile` (add `test-e2e`)
- Modify (if `go mod tidy` changes them): `go.mod`, `go.sum`

**Interfaces:**
- Consumes: `requireE2EContext`, `metricValue`, `podReady`, `podProblem`, the port constants (Task 3), and `make e2e-up` (Task 4).
- Produces (all `//go:build e2e`, package `e2e`):
  - `var env *Env`
  - `type Env struct { Kube client.Client; Clientset kubernetes.Interface; Config *rest.Config; Certs *Certs; FrpsIP, FrpsMTLSIP, Artifacts string }`
  - Constants `operatorNamespace`, `frpsNamespace`, `backendsNamespace`, `clientNamespace`, `egressNamespace`, `frpsToken`, `proxyUser`, `proxyPassword`, `frpsMainHost`, `frpsMTLSHost`, `wssHost`, `tcpEchoHost` (port 9000), `udpEchoHost` (port 9001), `httpEchoHost` (port 80/443), `egressProxyHost` (port 3128/1080), `metricsURL`
  - `type Certs struct { CA, ServerCert, ServerKey, ClientCert, ClientKey, OtherCA []byte }`
  - `func (e *Env) upsert(ctx context.Context, obj client.Object) error`
  - `func (e *Env) deleteAndWait(ctx context.Context, obj client.Object) error`
  - `func (e *Env) exec(ctx context.Context, namespace, pod, container string, cmd ...string) (string, error)`
  - `func (e *Env) sh(ctx context.Context, script string) (string, error)`
  - `func (e *Env) logs(ctx context.Context, namespace, pod, container string) (string, error)`
  - `func (e *Env) dumpCase(ctx context.Context, namespace string)`
  - `func eventually(t *testing.T, timeout time.Duration, what string, check func(ctx context.Context) error)`
  - `func expectTCPEcho(t *testing.T, host string, port int)`
  - `func expectUDPEcho(t *testing.T, host string, port int)`
  - `func expectNoTCP(t *testing.T, host string, port int)`
  - `func curl(ctx context.Context, args string) (string, error)`
  - `func must(t *testing.T, err error)`
  - `func ptr[T any](v T) *T`
  - `type Case struct { t *testing.T; NS string }`
  - `func newCase(t *testing.T, name string) *Case` (calls `t.Parallel()`)
  - `func (c *Case) Client(name string, mutate func(*frpv1alpha1.ClientSpec_Server)) *frpv1alpha1.Client`
  - `func (c *Case) Upstream(clientName, short string, spec frpv1alpha1.UpstreamSpec) string` (returns the proxy name `<ns>-<short>`)
  - `func (c *Case) Visitor(clientName, short string, spec frpv1alpha1.VisitorSpec) string`
  - `func (c *Case) Secret(name string, data map[string][]byte)`
  - `func tcpEcho(remotePort int) frpv1alpha1.UpstreamSpec`
  - `func udpEcho(remotePort int) frpv1alpha1.UpstreamSpec`
  - `func (c *Case) waitCondition(clientName, condType string, status metav1.ConditionStatus, reason string) *metav1.Condition`
  - `func (c *Case) waitClientSynced(clientName string)`
  - `func (c *Case) waitMetric(name string, labels map[string]string, want float64)`
  - `func (c *Case) waitProxyRunning(clientName, proxy string)`
  - `func (c *Case) waitFrpcLog(clientName, substr string)`
  - `func (c *Case) waitEvent(objectName, reason string)`
  - `func (c *Case) pod(clientName string) *corev1.Pod`

- [ ] **Step 1: Write the failing test (the first case) and the `test-e2e` target**

`test/e2e/tcp_feature_test.go`:

```go
//go:build e2e

package e2e

import "testing"

// TestTCP exposes the TCP echo backend through frps-main and sends a line through it.
func TestTCP(t *testing.T) {
	c := newCase(t, "tcp")
	c.Client("frpc", nil)
	proxy := c.Upstream("frpc", "echo", tcpEcho(portTCP))

	c.waitClientSynced("frpc")
	c.waitProxyRunning("frpc", proxy)
	expectTCPEcho(t, env.FrpsIP, portTCP)
}
```

Add to `Makefile` after `e2e-up`:

```make
.PHONY: test-e2e
test-e2e: e2e-up ## Run the end-to-end suite on the k3d e2e cluster (needs Docker, k3d, helm).
	E2E_KUBECONFIG=$(abspath $(E2E_KUBECONFIG)) E2E_ARTIFACTS_DIR=$(abspath $(E2E_ARTIFACTS_DIR)) \
		go test -tags e2e -count=1 -parallel 4 -timeout 15m -v ./test/e2e/...
```

- [ ] **Step 2: Run it to verify it fails**

Run: `go vet -tags e2e ./test/e2e/`
Expected: build failure, `undefined: newCase` (and the other helpers).

- [ ] **Step 3: Write the infra manifest**

`test/e2e/manifests/infra.yaml` (a Go template; `{{ .Token }}`, `{{ .ProxyUser }}`, `{{ .ProxyPassword }}` and `{{ .ClusterDNS }}` are filled in by `setupInfra`):

```yaml
apiVersion: v1
kind: Namespace
metadata: {name: frps}
---
apiVersion: v1
kind: Namespace
metadata: {name: backends}
---
apiVersion: v1
kind: Namespace
metadata: {name: egress}
---
apiVersion: v1
kind: Namespace
metadata: {name: e2e-client}
---
# frps-main: token auth, every listener the cases use, and a server certificate. TLS is not
# forced, so clients with and without trustedCaFile both work.
apiVersion: v1
kind: ConfigMap
metadata: {name: frps-main, namespace: frps}
data:
  frps.toml: |
    bindPort = 7000
    kcpBindPort = 7000
    quicBindPort = 7001
    vhostHTTPPort = 8080
    vhostHTTPSPort = 8443
    tcpmuxHTTPConnectPort = 5002
    auth.method = "token"
    auth.token = "{{ .Token }}"
    transport.tls.certFile = "/etc/frps/tls/tls.crt"
    transport.tls.keyFile = "/etc/frps/tls/tls.key"
---
# frps-mtls: trustedCaFile makes frps force TLS and require a client certificate.
apiVersion: v1
kind: ConfigMap
metadata: {name: frps-mtls, namespace: frps}
data:
  frps.toml: |
    bindPort = 7000
    auth.method = "token"
    auth.token = "{{ .Token }}"
    transport.tls.certFile = "/etc/frps/tls/tls.crt"
    transport.tls.keyFile = "/etc/frps/tls/tls.key"
    transport.tls.trustedCaFile = "/etc/frps/tls/ca.crt"
---
# wss-terminator: frps only accepts ws, so wss needs TLS terminated in front of it.
apiVersion: v1
kind: ConfigMap
metadata: {name: wss-terminator, namespace: frps}
data:
  nginx.conf: |
    worker_processes 1;
    events {}
    stream {
      server {
        listen 7443 ssl;
        ssl_certificate     /etc/nginx/tls/tls.crt;
        ssl_certificate_key /etc/nginx/tls/tls.key;
        proxy_pass frps-main.frps.svc.cluster.local:7000;
      }
    }
---
apiVersion: v1
kind: Service
metadata: {name: frps-main, namespace: frps}
spec:
  selector: {app: frps-main}
  ports:
  - {name: bind, port: 7000, protocol: TCP}
  - {name: kcp, port: 7000, protocol: UDP}
  - {name: quic, port: 7001, protocol: UDP}
---
apiVersion: v1
kind: Service
metadata: {name: frps-mtls, namespace: frps}
spec:
  selector: {app: frps-mtls}
  ports:
  - {name: bind, port: 7000, protocol: TCP}
---
apiVersion: v1
kind: Service
metadata: {name: wss-terminator, namespace: frps}
spec:
  selector: {app: wss-terminator}
  ports:
  - {name: wss, port: 7443, protocol: TCP}
---
apiVersion: apps/v1
kind: Deployment
metadata: {name: frps-main, namespace: frps}
spec:
  replicas: 1
  selector: {matchLabels: {app: frps-main}}
  template:
    metadata: {labels: {app: frps-main}}
    spec:
      containers:
      - name: frps
        image: fatedier/frps:v0.71.0
        args: ["-c", "/etc/frps/frps.toml"]
        readinessProbe: {tcpSocket: {port: 7000}, periodSeconds: 2}
        volumeMounts:
        - {name: config, mountPath: /etc/frps/frps.toml, subPath: frps.toml}
        - {name: tls, mountPath: /etc/frps/tls, readOnly: true}
      volumes:
      - {name: config, configMap: {name: frps-main}}
      - {name: tls, secret: {secretName: frps-tls}}
---
apiVersion: apps/v1
kind: Deployment
metadata: {name: frps-mtls, namespace: frps}
spec:
  replicas: 1
  selector: {matchLabels: {app: frps-mtls}}
  template:
    metadata: {labels: {app: frps-mtls}}
    spec:
      containers:
      - name: frps
        image: fatedier/frps:v0.71.0
        args: ["-c", "/etc/frps/frps.toml"]
        readinessProbe: {tcpSocket: {port: 7000}, periodSeconds: 2}
        volumeMounts:
        - {name: config, mountPath: /etc/frps/frps.toml, subPath: frps.toml}
        - {name: tls, mountPath: /etc/frps/tls, readOnly: true}
      volumes:
      - {name: config, configMap: {name: frps-mtls}}
      - {name: tls, secret: {secretName: frps-tls}}
---
apiVersion: apps/v1
kind: Deployment
metadata: {name: wss-terminator, namespace: frps}
spec:
  replicas: 1
  selector: {matchLabels: {app: wss-terminator}}
  template:
    metadata: {labels: {app: wss-terminator}}
    spec:
      containers:
      - name: nginx
        image: nginx:1.27-alpine
        readinessProbe: {tcpSocket: {port: 7443}, periodSeconds: 2}
        volumeMounts:
        - {name: config, mountPath: /etc/nginx/nginx.conf, subPath: nginx.conf}
        - {name: tls, mountPath: /etc/nginx/tls, readOnly: true}
      volumes:
      - {name: config, configMap: {name: wss-terminator}}
      - {name: tls, secret: {secretName: frps-tls}}
---
# Backends: one pod echoes TCP (9000) and UDP (9001), so one Service selector can carry both.
apiVersion: apps/v1
kind: Deployment
metadata: {name: echo, namespace: backends}
spec:
  replicas: 1
  selector: {matchLabels: {app: echo}}
  template:
    metadata: {labels: {app: echo}}
    spec:
      containers:
      - name: tcp
        image: alpine/socat:1.8.0.3
        args: ["TCP-LISTEN:9000,fork,reuseaddr", "EXEC:cat"]
        readinessProbe: {tcpSocket: {port: 9000}, periodSeconds: 2}
      - name: udp
        image: alpine/socat:1.8.0.3
        args: ["UDP-RECVFROM:9001,fork", "EXEC:cat"]
---
apiVersion: v1
kind: Service
metadata: {name: tcp-echo, namespace: backends}
spec:
  selector: {app: echo}
  ports:
  - {name: tcp, port: 9000, protocol: TCP}
---
apiVersion: v1
kind: Service
metadata: {name: udp-echo, namespace: backends}
spec:
  selector: {app: echo}
  ports:
  - {name: udp, port: 9001, protocol: UDP}
---
apiVersion: apps/v1
kind: Deployment
metadata: {name: http-echo, namespace: backends}
spec:
  replicas: 1
  selector: {matchLabels: {app: http-echo}}
  template:
    metadata: {labels: {app: http-echo}}
    spec:
      containers:
      - name: echo
        image: mendhak/http-https-echo:42
        readinessProbe: {httpGet: {path: /, port: 8080}, periodSeconds: 2}
---
apiVersion: v1
kind: Service
metadata: {name: http-echo, namespace: backends}
spec:
  selector: {app: http-echo}
  ports:
  - {name: http, port: 80, targetPort: 8080, protocol: TCP}
  - {name: https, port: 443, targetPort: 8443, protocol: TCP}
---
# Egress proxy (http :3128, socks5 :1080) with username/password. It resolves names through
# cluster DNS, because frpc asks it to reach frps-main by its Service name.
apiVersion: apps/v1
kind: Deployment
metadata: {name: egress-proxy, namespace: egress}
spec:
  replicas: 1
  selector: {matchLabels: {app: egress-proxy}}
  template:
    metadata: {labels: {app: egress-proxy}}
    spec:
      containers:
      - name: proxy
        image: tarampampam/3proxy:2.3.0
        env:
        - {name: PROXY_LOGIN, value: "{{ .ProxyUser }}"}
        - {name: PROXY_PASSWORD, value: "{{ .ProxyPassword }}"}
        - {name: PRIMARY_RESOLVER, value: "{{ .ClusterDNS }}"}
        - {name: LOG_OUTPUT, value: /dev/stdout}
        readinessProbe: {tcpSocket: {port: 3128}, periodSeconds: 2}
---
apiVersion: v1
kind: Service
metadata: {name: egress-proxy, namespace: egress}
spec:
  selector: {app: egress-proxy}
  ports:
  - {name: http, port: 3128, protocol: TCP}
  - {name: socks5, port: 1080, protocol: TCP}
---
# The "Internet side": every traffic check runs from this pod.
apiVersion: v1
kind: Pod
metadata: {name: client, namespace: e2e-client}
spec:
  containers:
  - name: client
    image: nicolaka/netshoot:v0.14
    command: ["sleep", "infinity"]
```

- [ ] **Step 4: Write the suite foundation**

`test/e2e/setup_test.go`:

```go
//go:build e2e

package e2e

import (
	"bytes"
	"context"
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"text/template"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"

	frpv1alpha1 "github.com/zufardhiyaulhaq/frp-operator/api/v1alpha1"
)

const (
	operatorNamespace = "frp-operator"
	frpsNamespace     = "frps"
	backendsNamespace = "backends"
	clientNamespace   = "e2e-client"
	egressNamespace   = "egress"

	frpsToken     = "e2e-token"
	proxyUser     = "e2e"
	proxyPassword = "p@ss/w0rd" // @ and / must be URL-escaped by the operator; 3proxy forbids ':'

	frpsMainHost    = "frps-main.frps.svc.cluster.local"
	frpsMTLSHost    = "frps-mtls.frps.svc.cluster.local"
	wssHost         = "wss-terminator.frps.svc.cluster.local"
	tcpEchoHost     = "tcp-echo.backends.svc.cluster.local"   // port 9000
	udpEchoHost     = "udp-echo.backends.svc.cluster.local"   // port 9001
	httpEchoHost    = "http-echo.backends.svc.cluster.local"  // port 80 (http), 443 (https)
	egressProxyHost = "egress-proxy.egress.svc.cluster.local" // port 3128 (http), 1080 (socks5)
	metricsURL      = "http://frp-operator-controller-manager-metrics-service.frp-operator.svc.cluster.local:8080/metrics"
)

//go:embed manifests/infra.yaml
var infraManifest string

// Env is the shared state of one suite run.
type Env struct {
	Kube       client.Client
	Clientset  kubernetes.Interface
	Config     *rest.Config
	Certs      *Certs
	FrpsIP     string // frps-main pod IP: cases reach remotePorts here; LoadBalancer publicAddress
	FrpsMTLSIP string // frps-mtls pod IP
	Artifacts  string // E2E_ARTIFACTS_DIR; "" disables dumps
}

var env *Env

func TestMain(m *testing.M) {
	code, err := run(m)
	if err != nil {
		fmt.Fprintln(os.Stderr, "e2e setup failed:", err)
		os.Exit(1)
	}
	os.Exit(code)
}

func run(m *testing.M) (int, error) {
	path := os.Getenv("E2E_KUBECONFIG")
	if path == "" {
		return 0, fmt.Errorf("E2E_KUBECONFIG is not set: run `make test-e2e`")
	}
	raw, err := clientcmd.LoadFromFile(path)
	if err != nil {
		return 0, err
	}
	if err := requireE2EContext(raw); err != nil {
		return 0, err
	}
	cfg, err := clientcmd.NewDefaultClientConfig(*raw, &clientcmd.ConfigOverrides{}).ClientConfig()
	if err != nil {
		return 0, err
	}
	e, err := newEnv(cfg, os.Getenv("E2E_ARTIFACTS_DIR"))
	if err != nil {
		return 0, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	if err := e.setupInfra(ctx); err != nil {
		e.dumpGlobal(context.Background())
		return 0, err
	}
	env = e
	code := m.Run()
	if code != 0 {
		e.dumpGlobal(context.Background())
	}
	return code, nil
}

func newEnv(cfg *rest.Config, artifacts string) (*Env, error) {
	scheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		return nil, err
	}
	if err := frpv1alpha1.AddToScheme(scheme); err != nil {
		return nil, err
	}
	kube, err := client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		return nil, err
	}
	clientset, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, err
	}
	return &Env{Kube: kube, Clientset: clientset, Config: cfg, Artifacts: artifacts}, nil
}

// setupInfra deploys frps, the backends, the egress proxy and the client pod, and waits for them.
func (e *Env) setupInfra(ctx context.Context) error {
	certs, err := newCerts([]string{frpsMainHost, frpsMTLSHost, wssHost})
	if err != nil {
		return err
	}
	e.Certs = certs

	dns := &corev1.Service{}
	if err := e.Kube.Get(ctx, client.ObjectKey{Namespace: "kube-system", Name: "kube-dns"}, dns); err != nil {
		return fmt.Errorf("find cluster DNS: %w", err)
	}
	var manifest bytes.Buffer
	if err := template.Must(template.New("infra").Parse(infraManifest)).Execute(&manifest, map[string]string{
		"Token": frpsToken, "ProxyUser": proxyUser, "ProxyPassword": proxyPassword, "ClusterDNS": dns.Spec.ClusterIP,
	}); err != nil {
		return err
	}
	if err := e.apply(ctx, manifest.Bytes()); err != nil {
		return err
	}
	if err := e.upsert(ctx, &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "frps-tls", Namespace: frpsNamespace},
		Data:       map[string][]byte{"tls.crt": certs.ServerCert, "tls.key": certs.ServerKey, "ca.crt": certs.CA},
	}); err != nil {
		return err
	}
	for _, ns := range []string{frpsNamespace, backendsNamespace, egressNamespace, clientNamespace} {
		if err := e.waitPodsReady(ctx, ns, 4*time.Minute); err != nil {
			return err
		}
	}
	if e.FrpsIP, err = e.podIP(ctx, frpsNamespace, "app=frps-main"); err != nil {
		return err
	}
	if e.FrpsMTLSIP, err = e.podIP(ctx, frpsNamespace, "app=frps-mtls"); err != nil {
		return err
	}
	return nil
}

// dumpGlobal saves the shared components' logs and all events for the CI artifact.
func (e *Env) dumpGlobal(ctx context.Context) {
	if e.Artifacts == "" {
		return
	}
	dir := filepath.Join(e.Artifacts, "_global")
	for _, ns := range []string{operatorNamespace, frpsNamespace, egressNamespace, backendsNamespace} {
		e.dumpPodLogs(ctx, ns, filepath.Join(dir, ns))
	}
	e.dumpEvents(ctx, "", filepath.Join(dir, "events.txt"))
}
```

`test/e2e/certs_test.go`:

```go
//go:build e2e

package e2e

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"time"
)

// Certs are PEM blocks generated for one suite run; nothing is stored in the repo.
type Certs struct {
	CA         []byte // signs the frps server cert and the frpc client cert
	ServerCert []byte // frps-main, frps-mtls and wss-terminator (one cert, all three DNS names)
	ServerKey  []byte
	ClientCert []byte // frpc client cert for mTLS
	ClientKey  []byte
	OtherCA    []byte // an unrelated CA, for "a wrong CA is rejected"
}

func newCerts(serverNames []string) (*Certs, error) {
	ca, caKey, caPEM, err := newCA("frp-operator-e2e CA")
	if err != nil {
		return nil, err
	}
	serverCert, serverKey, err := newLeaf(ca, caKey, "frps", serverNames, x509.ExtKeyUsageServerAuth)
	if err != nil {
		return nil, err
	}
	clientCert, clientKey, err := newLeaf(ca, caKey, "frpc", nil, x509.ExtKeyUsageClientAuth)
	if err != nil {
		return nil, err
	}
	_, _, otherPEM, err := newCA("unrelated CA")
	if err != nil {
		return nil, err
	}
	return &Certs{CA: caPEM, ServerCert: serverCert, ServerKey: serverKey, ClientCert: clientCert, ClientKey: clientKey, OtherCA: otherPEM}, nil
}

func newCA(name string) (*x509.Certificate, *ecdsa.PrivateKey, []byte, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, nil, err
	}
	tmpl := &x509.Certificate{
		SerialNumber:          serial(),
		Subject:               pkix.Name{CommonName: name},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return nil, nil, nil, err
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, nil, nil, err
	}
	return cert, key, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), nil
}

func newLeaf(ca *x509.Certificate, caKey *ecdsa.PrivateKey, name string, dnsNames []string, usage x509.ExtKeyUsage) ([]byte, []byte, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, err
	}
	tmpl := &x509.Certificate{
		SerialNumber: serial(),
		Subject:      pkix.Name{CommonName: name},
		DNSNames:     dnsNames,
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{usage},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, ca, &key.PublicKey, caKey)
	if err != nil {
		return nil, nil, err
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return nil, nil, err
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
		pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}), nil
}

func serial() *big.Int {
	n, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 62))
	if err != nil {
		panic(err)
	}
	return n
}
```

`test/e2e/kube_test.go`:

```go
//go:build e2e

package e2e

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apimachinery/pkg/util/yaml"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/remotecommand"
	"sigs.k8s.io/controller-runtime/pkg/client"
	sigsyaml "sigs.k8s.io/yaml"

	frpv1alpha1 "github.com/zufardhiyaulhaq/frp-operator/api/v1alpha1"
)

// apply server-side-applies every object of a multi-document YAML, so reruns are idempotent.
func (e *Env) apply(ctx context.Context, manifest []byte) error {
	dec := yaml.NewYAMLOrJSONDecoder(bytes.NewReader(manifest), 4096)
	for {
		obj := &unstructured.Unstructured{}
		if err := dec.Decode(&obj.Object); err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
		if len(obj.Object) == 0 {
			continue
		}
		if err := e.Kube.Patch(ctx, obj, client.Apply, client.FieldOwner("frp-operator-e2e"), client.ForceOwnership); err != nil {
			return fmt.Errorf("apply %s %s/%s: %w", obj.GetKind(), obj.GetNamespace(), obj.GetName(), err)
		}
	}
}

// upsert creates obj, or replaces it if it already exists.
func (e *Env) upsert(ctx context.Context, obj client.Object) error {
	err := e.Kube.Create(ctx, obj)
	if !apierrors.IsAlreadyExists(err) {
		return err
	}
	existing := obj.DeepCopyObject().(client.Object)
	if err := e.Kube.Get(ctx, client.ObjectKeyFromObject(obj), existing); err != nil {
		return err
	}
	obj.SetResourceVersion(existing.GetResourceVersion())
	return e.Kube.Update(ctx, obj)
}

// deleteAndWait deletes obj (if present) and waits until it is gone, finalizers included.
func (e *Env) deleteAndWait(ctx context.Context, obj client.Object) error {
	if err := e.Kube.Delete(ctx, obj); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return err
	}
	return wait.PollUntilContextTimeout(ctx, 2*time.Second, 3*time.Minute, true, func(ctx context.Context) (bool, error) {
		probe := obj.DeepCopyObject().(client.Object)
		return apierrors.IsNotFound(e.Kube.Get(ctx, client.ObjectKeyFromObject(obj), probe)), nil
	})
}

func (e *Env) waitPodsReady(ctx context.Context, namespace string, timeout time.Duration) error {
	problem := "no pods yet"
	err := wait.PollUntilContextTimeout(ctx, 2*time.Second, timeout, true, func(ctx context.Context) (bool, error) {
		pods := &corev1.PodList{}
		if err := e.Kube.List(ctx, pods, client.InNamespace(namespace)); err != nil {
			problem = err.Error()
			return false, nil
		}
		if len(pods.Items) == 0 {
			problem = "no pods yet"
			return false, nil
		}
		for i := range pods.Items {
			if !podReady(&pods.Items[i]) {
				problem = podProblem(&pods.Items[i])
				return false, nil
			}
		}
		return true, nil
	})
	if err != nil {
		return fmt.Errorf("namespace %s not ready after %s: %s", namespace, timeout, problem)
	}
	return nil
}

func (e *Env) podIP(ctx context.Context, namespace, selector string) (string, error) {
	pods, err := e.Clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{LabelSelector: selector})
	if err != nil {
		return "", err
	}
	for i := range pods.Items {
		if podReady(&pods.Items[i]) && pods.Items[i].Status.PodIP != "" {
			return pods.Items[i].Status.PodIP, nil
		}
	}
	return "", fmt.Errorf("no ready pod %q in %s", selector, namespace)
}

// exec runs cmd in a pod container and returns stdout; a non-zero exit is an error with stderr.
func (e *Env) exec(ctx context.Context, namespace, pod, container string, cmd ...string) (string, error) {
	req := e.Clientset.CoreV1().RESTClient().Post().Resource("pods").Name(pod).Namespace(namespace).
		SubResource("exec").
		VersionedParams(&corev1.PodExecOptions{Container: container, Command: cmd, Stdout: true, Stderr: true}, clientgoscheme.ParameterCodec)
	executor, err := remotecommand.NewSPDYExecutor(e.Config, "POST", req.URL())
	if err != nil {
		return "", err
	}
	var stdout, stderr bytes.Buffer
	if err := executor.StreamWithContext(ctx, remotecommand.StreamOptions{Stdout: &stdout, Stderr: &stderr}); err != nil {
		return stdout.String(), fmt.Errorf("%v: %s", err, strings.TrimSpace(stderr.String()))
	}
	return stdout.String(), nil
}

// sh runs a shell script on the test client pod (netshoot in e2e-client).
func (e *Env) sh(ctx context.Context, script string) (string, error) {
	return e.exec(ctx, clientNamespace, "client", "client", "sh", "-c", script)
}

func (e *Env) logs(ctx context.Context, namespace, pod, container string) (string, error) {
	out, err := e.Clientset.CoreV1().Pods(namespace).GetLogs(pod, &corev1.PodLogOptions{Container: container}).DoRaw(ctx)
	return string(out), err
}

func (e *Env) dumpPodLogs(ctx context.Context, namespace, dir string) {
	pods, err := e.Clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return
	}
	_ = os.MkdirAll(dir, 0o755)
	for _, p := range pods.Items {
		for _, c := range p.Spec.Containers {
			out, err := e.logs(ctx, namespace, p.Name, c.Name)
			if err != nil {
				out = err.Error()
			}
			_ = os.WriteFile(filepath.Join(dir, p.Name+"_"+c.Name+".log"), []byte(out), 0o644)
		}
	}
}

func (e *Env) dumpEvents(ctx context.Context, namespace, file string) {
	events, err := e.Clientset.CoreV1().Events(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return
	}
	var b strings.Builder
	for _, ev := range events.Items {
		fmt.Fprintf(&b, "%s %s %s/%s %s %s: %s\n", ev.LastTimestamp.Format(time.RFC3339), ev.Namespace,
			ev.InvolvedObject.Kind, ev.InvolvedObject.Name, ev.Type, ev.Reason, ev.Message)
	}
	_ = os.MkdirAll(filepath.Dir(file), 0o755)
	_ = os.WriteFile(file, []byte(b.String()), 0o644)
}

// dumpCase saves a failed case: pod logs, events, Client status YAML and rendered frpc configs.
func (e *Env) dumpCase(ctx context.Context, namespace string) {
	if e.Artifacts == "" {
		return
	}
	dir := filepath.Join(e.Artifacts, namespace)
	e.dumpPodLogs(ctx, namespace, dir)
	e.dumpEvents(ctx, namespace, filepath.Join(dir, "events.txt"))
	clients := &frpv1alpha1.ClientList{}
	if err := e.Kube.List(ctx, clients, client.InNamespace(namespace)); err == nil {
		for _, cl := range clients.Items {
			if out, err := sigsyaml.Marshal(cl); err == nil {
				_ = os.WriteFile(filepath.Join(dir, "client_"+cl.Name+".yaml"), out, 0o644)
			}
		}
	}
	configMaps := &corev1.ConfigMapList{}
	if err := e.Kube.List(ctx, configMaps, client.InNamespace(namespace)); err == nil {
		for _, cm := range configMaps.Items {
			if data, ok := cm.Data["config.toml"]; ok {
				_ = os.WriteFile(filepath.Join(dir, cm.Name+".toml"), []byte(data), 0o644)
			}
		}
	}
}
```

`test/e2e/traffic_test.go`:

```go
//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"
)

// eventually retries check until it returns nil or timeout passes; then the last error fails t.
func eventually(t *testing.T, timeout time.Duration, what string, check func(ctx context.Context) error) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		err := check(ctx)
		cancel()
		if err == nil {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("%s: still failing after %s: %v", what, timeout, err)
		}
		time.Sleep(2 * time.Second)
	}
}

func expectTCPEcho(t *testing.T, host string, port int) { t.Helper(); expectEcho(t, "TCP", host, port) }
func expectUDPEcho(t *testing.T, host string, port int) { t.Helper(); expectEcho(t, "UDP", host, port) }

// expectEcho sends a unique line from the test client to host:port and expects it back.
func expectEcho(t *testing.T, proto, host string, port int) {
	t.Helper()
	token := fmt.Sprintf("ping-%d", time.Now().UnixNano())
	eventually(t, 30*time.Second, fmt.Sprintf("%s echo via %s:%d", proto, host, port), func(ctx context.Context) error {
		out, err := env.sh(ctx, fmt.Sprintf("echo %s | socat -t 3 - %s:%s:%d", token, proto, host, port))
		if err != nil {
			return err
		}
		if !strings.Contains(out, token) {
			return fmt.Errorf("got %q, want %q echoed back", out, token)
		}
		return nil
	})
}

// expectNoTCP checks three times over ~10s that nothing answers on host:port.
func expectNoTCP(t *testing.T, host string, port int) {
	t.Helper()
	for i := 0; i < 3; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		out, err := env.sh(ctx, fmt.Sprintf("echo probe | socat -t 2 - TCP:%s:%d,connect-timeout=2", host, port))
		cancel()
		if err == nil {
			t.Fatalf("TCP %s:%d answered (%q), want nothing listening", host, port, out)
		}
		time.Sleep(3 * time.Second)
	}
}

// curl runs curl on the test client and returns stdout.
func curl(ctx context.Context, args string) (string, error) {
	return env.sh(ctx, "curl -sS --max-time 5 "+args)
}
```

`test/e2e/case_test.go`:

```go
//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	frpv1alpha1 "github.com/zufardhiyaulhaq/frp-operator/api/v1alpha1"
)

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func ptr[T any](v T) *T { return &v }

// Case is one feature case: its own namespace, its own Clients, its own remote ports.
type Case struct {
	t  *testing.T
	NS string
}

// newCase runs t in parallel and gives it a fresh namespace e2e-<name> holding the frps token
// Secret `frps-token`. A namespace left over from an earlier run is deleted first. The namespace
// is deleted when the test passes, and kept and dumped when it fails.
func newCase(t *testing.T, name string) *Case {
	t.Helper()
	t.Parallel()
	ctx := context.Background()
	c := &Case{t: t, NS: "e2e-" + name}
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: c.NS}}
	must(t, env.deleteAndWait(ctx, ns))
	must(t, env.Kube.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: c.NS}}))
	c.Secret("frps-token", map[string][]byte{"token": []byte(frpsToken)})
	t.Cleanup(func() {
		if t.Failed() {
			env.dumpCase(context.Background(), c.NS)
			return
		}
		_ = env.Kube.Delete(context.Background(), &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: c.NS}})
	})
	return c
}

func (c *Case) Secret(name string, data map[string][]byte) {
	c.t.Helper()
	must(c.t, env.Kube.Create(context.Background(), &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: c.NS}, Data: data}))
}

// Client creates a Client for frps-main with token auth; mutate adjusts its server spec.
func (c *Case) Client(name string, mutate func(*frpv1alpha1.ClientSpec_Server)) *frpv1alpha1.Client {
	c.t.Helper()
	cl := &frpv1alpha1.Client{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: c.NS},
		Spec: frpv1alpha1.ClientSpec{Server: frpv1alpha1.ClientSpec_Server{
			Host: frpsMainHost,
			Port: 7000,
			Authentication: frpv1alpha1.ClientSpec_Server_Authentication{
				Token: &frpv1alpha1.ClientSpec_Server_Authentication_Token{Secret: frpv1alpha1.Secret{Name: "frps-token", Key: "token"}},
			},
		}},
	}
	if mutate != nil {
		mutate(&cl.Spec.Server)
	}
	must(c.t, env.Kube.Create(context.Background(), cl))
	return cl
}

// Upstream creates an Upstream named <ns>-<short> and returns that name, which is also the frps
// proxy name. frps proxy names are global, so the namespace prefix keeps parallel cases apart.
func (c *Case) Upstream(clientName, short string, spec frpv1alpha1.UpstreamSpec) string {
	c.t.Helper()
	name := c.NS + "-" + short
	spec.Client = clientName
	must(c.t, env.Kube.Create(context.Background(), &frpv1alpha1.Upstream{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: c.NS}, Spec: spec}))
	return name
}

// Visitor creates a Visitor named <ns>-<short> and returns that name.
func (c *Case) Visitor(clientName, short string, spec frpv1alpha1.VisitorSpec) string {
	c.t.Helper()
	name := c.NS + "-" + short
	spec.Client = clientName
	must(c.t, env.Kube.Create(context.Background(), &frpv1alpha1.Visitor{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: c.NS}, Spec: spec}))
	return name
}

// tcpEcho exposes the TCP echo backend on remotePort.
func tcpEcho(remotePort int) frpv1alpha1.UpstreamSpec {
	return frpv1alpha1.UpstreamSpec{TCP: &frpv1alpha1.UpstreamSpec_TCP{
		Host: tcpEchoHost, Port: 9000, Server: frpv1alpha1.UpstreamSpec_TCP_Server{Port: remotePort},
	}}
}

// udpEcho exposes the UDP echo backend on remotePort.
func udpEcho(remotePort int) frpv1alpha1.UpstreamSpec {
	return frpv1alpha1.UpstreamSpec{UDP: &frpv1alpha1.UpstreamSpec_UDP{
		Host: udpEchoHost, Port: 9001, Server: frpv1alpha1.UpstreamSpec_UDP_Server{Port: remotePort},
	}}
}

// waitCondition waits for a Client condition (reason "" matches any) and returns it.
func (c *Case) waitCondition(clientName, condType string, status metav1.ConditionStatus, reason string) *metav1.Condition {
	c.t.Helper()
	var got *metav1.Condition
	eventually(c.t, 3*time.Minute, fmt.Sprintf("Client %s/%s %s=%s %s", c.NS, clientName, condType, status, reason), func(ctx context.Context) error {
		cl := &frpv1alpha1.Client{}
		if err := env.Kube.Get(ctx, client.ObjectKey{Namespace: c.NS, Name: clientName}, cl); err != nil {
			return err
		}
		cond := meta.FindStatusCondition(cl.Status.Conditions, condType)
		if cond == nil || cond.Status != status || (reason != "" && cond.Reason != reason) {
			return fmt.Errorf("condition is %+v", cond)
		}
		got = cond
		return nil
	})
	return got
}

// waitClientSynced waits for Ready=True and ConfigSynced=True.
func (c *Case) waitClientSynced(clientName string) {
	c.t.Helper()
	c.waitCondition(clientName, "Ready", metav1.ConditionTrue, "")
	c.waitCondition(clientName, "ConfigSynced", metav1.ConditionTrue, "")
}

// waitMetric waits until the operator's /metrics reports want for the sample. Metrics refresh on
// every reconcile (30s requeue), and a reload also waits for the ConfigMap volume sync, hence 3m.
func (c *Case) waitMetric(name string, labels map[string]string, want float64) {
	c.t.Helper()
	eventually(c.t, 3*time.Minute, fmt.Sprintf("%s%v = %v", name, labels, want), func(ctx context.Context) error {
		body, err := env.sh(ctx, "curl -sS --max-time 5 "+metricsURL)
		if err != nil {
			return err
		}
		got, ok := metricValue(body, name, labels)
		if !ok || got != want {
			return fmt.Errorf("got %v (present=%v)", got, ok)
		}
		return nil
	})
}

// waitProxyRunning waits until frpc reports proxy as running, through the operator's metrics.
func (c *Case) waitProxyRunning(clientName, proxy string) {
	c.t.Helper()
	c.waitMetric("frp_proxy_status", map[string]string{"namespace": c.NS, "client": clientName, "proxy": proxy, "status": "running"}, 1)
}

// waitFrpcLog waits until the frpc pod of clientName logs a line containing substr.
func (c *Case) waitFrpcLog(clientName, substr string) {
	c.t.Helper()
	eventually(c.t, 3*time.Minute, fmt.Sprintf("frpc %s logs %q", clientName, substr), func(ctx context.Context) error {
		out, err := env.logs(ctx, c.NS, clientName+"-frpc", "frpc")
		if err != nil {
			return err
		}
		if !strings.Contains(out, substr) {
			return fmt.Errorf("not logged yet")
		}
		return nil
	})
}

// waitEvent waits for a Warning event with reason on objectName.
func (c *Case) waitEvent(objectName, reason string) {
	c.t.Helper()
	eventually(c.t, 90*time.Second, fmt.Sprintf("Warning %s event on %s", reason, objectName), func(ctx context.Context) error {
		events, err := env.Clientset.CoreV1().Events(c.NS).List(ctx, metav1.ListOptions{
			FieldSelector: "involvedObject.name=" + objectName + ",reason=" + reason + ",type=Warning",
		})
		if err != nil {
			return err
		}
		if len(events.Items) == 0 {
			return fmt.Errorf("no event yet")
		}
		return nil
	})
}

// pod returns the frpc pod of clientName.
func (c *Case) pod(clientName string) *corev1.Pod {
	c.t.Helper()
	p := &corev1.Pod{}
	must(c.t, env.Kube.Get(context.Background(), client.ObjectKey{Namespace: c.NS, Name: clientName + "-frpc"}, p))
	return p
}
```

- [ ] **Step 5: Tidy modules and compile**

```bash
go mod tidy
go vet -tags e2e ./test/e2e/
go vet ./...
git diff --stat go.mod go.sum
```

Expected: vet clean with and without the tag. If `go.mod` changed, the only change is indirect deps becoming direct (`k8s.io/client-go`, `sigs.k8s.io/yaml`), with **no new module**.

- [ ] **Step 6: Run the suite to verify TCP passes**

Run: `make test-e2e`
Expected:
- setup finishes and every infra namespace is ready
- `--- PASS: TestTCP`
- the unit tests from Task 3 also PASS
- `ok  github.com/zufardhiyaulhaq/frp-operator/test/e2e`

If setup fails, the error names the pod and the reason.

- [ ] **Step 7: Run it again on the same cluster (idempotency, Review Focus #2)**

Run: `make test-e2e`
Expected: PASS again. `newCase` deletes the leftover `e2e-tcp` namespace if the first run kept it.

- [ ] **Step 8: Commit**

```bash
git add Makefile test/e2e go.mod go.sum
git commit -m "test(e2e): suite foundation (in-cluster frps, backends, client) and TCP case"
```

---

### Task 6: UDP and `enabled: false`

**Files:**
- Create: `test/e2e/udp_feature_test.go`

**Interfaces:**
- Consumes: `newCase`, `Case.Client`, `Case.Upstream`, `tcpEcho`, `udpEcho`, `waitClientSynced`, `waitProxyRunning`, `expectTCPEcho`, `expectUDPEcho`, `expectNoTCP`, `ptr`, `portUDP`, `portEnabled`, `portDisabled` (Tasks 3 and 5).
- Produces: nothing new.

- [ ] **Step 1: Write the tests**

```go
//go:build e2e

package e2e

import "testing"

// TestUDP sends a datagram through frps-main to the UDP echo backend.
func TestUDP(t *testing.T) {
	c := newCase(t, "udp")
	c.Client("frpc", nil)
	proxy := c.Upstream("frpc", "echo", udpEcho(portUDP))

	c.waitClientSynced("frpc")
	c.waitProxyRunning("frpc", proxy)
	expectUDPEcho(t, env.FrpsIP, portUDP)
}

// TestDisabledUpstream checks that enabled:false leaves the proxy off while its sibling works.
func TestDisabledUpstream(t *testing.T) {
	c := newCase(t, "disabled")
	c.Client("frpc", nil)
	on := c.Upstream("frpc", "on", tcpEcho(portEnabled))
	off := tcpEcho(portDisabled)
	off.Enabled = ptr(false)
	c.Upstream("frpc", "off", off)

	c.waitClientSynced("frpc")
	c.waitProxyRunning("frpc", on)
	expectTCPEcho(t, env.FrpsIP, portEnabled)
	expectNoTCP(t, env.FrpsIP, portDisabled)
}
```

- [ ] **Step 2: Run to verify they fail first**

Before writing the file above, temporarily break the expectation to see the failure path. Run `make test-e2e` once with `expectUDPEcho(t, env.FrpsIP, portUDP+1000)`.
Expected: `TestUDP` fails with `UDP echo via <ip>:21002: still failing after 30s`, and `output/e2e-artifacts/e2e-udp/` contains the frpc log and `frpc-frpc-config.toml`. This also proves the failure dump works. Then restore `portUDP`.

- [ ] **Step 3: Run to verify they pass**

Run: `make test-e2e`
Expected: `--- PASS: TestUDP`, `--- PASS: TestDisabledUpstream`, plus `TestTCP`.

- [ ] **Step 4: Commit**

```bash
git add test/e2e/udp_feature_test.go
git commit -m "test(e2e): UDP and enabled:false cases"
```

---

### Task 7: HTTP vhost, HTTPS vhost, TCPMUX

**Files:**
- Create: `test/e2e/http_feature_test.go`

**Interfaces:**
- Consumes: `newCase`, `Case.Secret`, `Case.Client`, `Case.Upstream`, `waitClientSynced`, `waitProxyRunning`, `eventually`, `curl`, `httpEchoHost`, `env.FrpsIP` (Task 5).
- Produces: nothing new.

Facts this task relies on:
- `mendhak/http-https-echo` answers with JSON pretty-printed with two spaces (`"path": "/api/ping"`, `"host": "..."`) and lowercases header names.
- frps routes HTTP by Host on 8080, HTTPS by SNI on 8443, and TCPMUX by the HTTP CONNECT host on 5002.

- [ ] **Step 1: Write the tests**

```go
//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	frpv1alpha1 "github.com/zufardhiyaulhaq/frp-operator/api/v1alpha1"
)

func expectHTTPStatus(t *testing.T, what, args, want string) {
	t.Helper()
	eventually(t, 30*time.Second, what, func(ctx context.Context) error {
		code, err := curl(ctx, "-o /dev/null -w '%{http_code}' "+args)
		if err != nil {
			return err
		}
		if code != want {
			return fmt.Errorf("status %s, want %s", code, want)
		}
		return nil
	})
}

func expectBody(t *testing.T, what, args string, wants ...string) {
	t.Helper()
	eventually(t, 30*time.Second, what, func(ctx context.Context) error {
		out, err := curl(ctx, args)
		if err != nil {
			return err
		}
		for _, w := range wants {
			if !strings.Contains(strings.ToLower(out), strings.ToLower(w)) {
				return fmt.Errorf("response lacks %q:\n%s", w, out)
			}
		}
		return nil
	})
}

// TestHTTPVhost covers customDomains, locations, request/response headers, hostHeaderRewrite
// and basic auth on frps's vhost HTTP port.
func TestHTTPVhost(t *testing.T) {
	c := newCase(t, "http")
	c.Secret("basic-auth", map[string][]byte{"user": []byte("alice"), "password": []byte("s3cret")})
	domain := c.NS + ".e2e.local"
	c.Client("frpc", nil)
	proxy := c.Upstream("frpc", "web", frpv1alpha1.UpstreamSpec{HTTP: &frpv1alpha1.UpstreamSpec_HTTP{
		Host:              httpEchoHost,
		Port:              80,
		CustomDomains:     []string{domain},
		Locations:         []string{"/api"},
		HostHeaderRewrite: "internal.e2e",
		RequestHeaders:    &frpv1alpha1.HTTPHeaders{Set: map[string]string{"x-e2e": "from-frp"}},
		ResponseHeaders:   &frpv1alpha1.HTTPHeaders{Set: map[string]string{"x-e2e-response": "yes"}},
		HTTPUser:          &frpv1alpha1.SecretRef{Secret: frpv1alpha1.Secret{Name: "basic-auth", Key: "user"}},
		HTTPPassword:      &frpv1alpha1.SecretRef{Secret: frpv1alpha1.Secret{Name: "basic-auth", Key: "password"}},
	}})
	c.waitClientSynced("frpc")
	c.waitProxyRunning("frpc", proxy)

	base := fmt.Sprintf("-H 'Host: %s' http://%s:8080", domain, env.FrpsIP)
	expectHTTPStatus(t, "basic auth is enforced", base+"/api/ping", "401")
	expectBody(t, "request reaches the backend rewritten", "-i -u alice:s3cret "+base+"/api/ping",
		"HTTP/1.1 200", `"path": "/api/ping"`, `"host": "internal.e2e"`, `"x-e2e": "from-frp"`, "x-e2e-response: yes")
	expectHTTPStatus(t, "paths outside locations are not routed", "-u alice:s3cret "+base+"/other", "404")
}

// TestHTTPSVhost routes TLS by SNI on frps's vhost HTTPS port to the backend's own TLS.
func TestHTTPSVhost(t *testing.T) {
	c := newCase(t, "https")
	domain := c.NS + ".e2e.local"
	c.Client("frpc", nil)
	proxy := c.Upstream("frpc", "secure", frpv1alpha1.UpstreamSpec{HTTPS: &frpv1alpha1.UpstreamSpec_HTTPS{
		Host: httpEchoHost, Port: 443, CustomDomains: []string{domain},
	}})
	c.waitClientSynced("frpc")
	c.waitProxyRunning("frpc", proxy)

	expectBody(t, "TLS passes through to the backend",
		fmt.Sprintf("-k --resolve %s:8443:%s https://%s:8443/hello", domain, env.FrpsIP, domain), `"path": "/hello"`)
}

// TestTCPMUX tunnels through frps's HTTP CONNECT multiplexer, routed by the CONNECT host.
func TestTCPMUX(t *testing.T) {
	c := newCase(t, "tcpmux")
	domain := c.NS + ".e2e.local"
	c.Client("frpc", nil)
	proxy := c.Upstream("frpc", "mux", frpv1alpha1.UpstreamSpec{TCPMUX: &frpv1alpha1.UpstreamSpec_TCPMUX{
		Host: httpEchoHost, Port: 80, Multiplexer: "httpconnect", CustomDomains: []string{domain},
	}})
	c.waitClientSynced("frpc")
	c.waitProxyRunning("frpc", proxy)

	expectBody(t, "CONNECT tunnel reaches the backend",
		fmt.Sprintf("--proxytunnel -x http://%s:5002 http://%s/hello", env.FrpsIP, domain), `"path": "/hello"`)
}
```

- [ ] **Step 2: Run to verify the HTTP checks really check**

Temporarily change `"x-e2e": "from-frp"` in the `expectBody` list to `"x-e2e": "wrong"`, then run `make test-e2e`.
Expected: `TestHTTPVhost` fails with `response lacks "\"x-e2e\": \"wrong\""`, and the dumped response shows `"x-e2e": "from-frp"`. Then restore it.

- [ ] **Step 3: Run to verify they pass**

Run: `make test-e2e`
Expected: `--- PASS: TestHTTPVhost`, `--- PASS: TestHTTPSVhost`, `--- PASS: TestTCPMUX`.

- [ ] **Step 4: Commit**

```bash
git add test/e2e/http_feature_test.go
git commit -m "test(e2e): HTTP vhost, HTTPS vhost and TCPMUX cases"
```

---

### Task 8: STCP + visitor

**Files:**
- Create: `test/e2e/stcp_feature_test.go`

**Interfaces:**
- Consumes: `newCase`, `Case.Secret`, `Case.Client`, `Case.Upstream`, `Case.Visitor`, `waitClientSynced`, `waitProxyRunning`, `expectTCPEcho`, `tcpEchoHost` (Task 5).
- Produces: nothing new.

Fact: the operator exposes each visitor's bind port on the Client Service `<client>-frpc` (TCP only). The visitor must bind `0.0.0.0` to be reachable through that Service.

- [ ] **Step 1: Write the test**

```go
//go:build e2e

package e2e

import (
	"fmt"
	"testing"

	frpv1alpha1 "github.com/zufardhiyaulhaq/frp-operator/api/v1alpha1"
)

// TestSTCPVisitor publishes the TCP echo backend as STCP on one Client and reaches it through a
// visitor on a second Client, via the visitor port on that Client's Service.
func TestSTCPVisitor(t *testing.T) {
	c := newCase(t, "stcp")
	c.Secret("stcp-key", map[string][]byte{"key": []byte("e2e-stcp-secret")})
	key := frpv1alpha1.Secret{Name: "stcp-key", Key: "key"}

	c.Client("server", nil)
	proxy := c.Upstream("server", "private", frpv1alpha1.UpstreamSpec{STCP: &frpv1alpha1.UpstreamSpec_STCP{
		Host: tcpEchoHost, Port: 9000, SecretKey: frpv1alpha1.UpstreamSpec_STCP_SecretKey{Secret: key},
	}})
	c.Client("visitor", nil)
	c.Visitor("visitor", "private-visitor", frpv1alpha1.VisitorSpec{STCP: &frpv1alpha1.VisitorSpec_STCP{
		Host: "0.0.0.0", Port: 9100, ServerName: proxy,
		ServerSecretKey: frpv1alpha1.VisitorSpec_STCP_ServerSecretKey{Secret: key},
	}})

	c.waitClientSynced("server")
	c.waitProxyRunning("server", proxy)
	c.waitClientSynced("visitor")
	expectTCPEcho(t, fmt.Sprintf("visitor-frpc.%s.svc.cluster.local", c.NS), 9100)
}
```

- [ ] **Step 2: Run to verify a wrong secret fails**

Temporarily give the visitor a different Secret: add `c.Secret("wrong-key", map[string][]byte{"key": []byte("nope")})` and point `ServerSecretKey` at it. Run `make test-e2e`.
Expected: `TestSTCPVisitor` fails at `TCP echo via visitor-frpc...:9100`. Then restore.

- [ ] **Step 3: Run to verify it passes**

Run: `make test-e2e`
Expected: `--- PASS: TestSTCPVisitor`.

- [ ] **Step 4: Commit**

```bash
git add test/e2e/stcp_feature_test.go
git commit -m "test(e2e): STCP upstream reached through a visitor"
```

---

### Task 9: Server protocols kcp, quic, websocket, wss

**Files:**
- Create: `test/e2e/protocol_feature_test.go`

**Interfaces:**
- Consumes: `newCase`, `Case.Client`, `Case.Upstream`, `tcpEcho`, `waitClientSynced`, `waitProxyRunning`, `expectTCPEcho`, `ptr`, `frpsMainHost`, `wssHost`, `portKCP`, `portQUIC`, `portWebsocket`, `portWSS` (Tasks 3 and 5).
- Produces: nothing new.

Facts:
- frps-main listens for kcp on 7000/udp and quic on 7001/udp. Both are exposed on the `frps-main` Service.
- wss goes to `wss-terminator:7443`, where nginx terminates TLS and forwards plain websocket to `frps-main:7000`.
- frpc enables TLS for wss and skips verification when no CA is set.

- [ ] **Step 1: Check the terminator image supports `stream` + SSL**

```bash
kubectl --kubeconfig output/e2e-kubeconfig -n frps exec deploy/wss-terminator -- nginx -V 2>&1 | grep -o 'with-stream_ssl_module'
```

Expected: `with-stream_ssl_module`. Setup in Task 5 already requires the pod to be Ready on 7443.

- [ ] **Step 2: Write the test**

```go
//go:build e2e

package e2e

import (
	"testing"

	frpv1alpha1 "github.com/zufardhiyaulhaq/frp-operator/api/v1alpha1"
)

// TestServerProtocols connects frpc to frps over each non-default transport and sends traffic.
func TestServerProtocols(t *testing.T) {
	cases := []struct {
		name, host string
		port       int
		remotePort int
	}{
		{name: "kcp", host: frpsMainHost, port: 7000, remotePort: portKCP},
		{name: "quic", host: frpsMainHost, port: 7001, remotePort: portQUIC},
		{name: "websocket", host: frpsMainHost, port: 7000, remotePort: portWebsocket},
		{name: "wss", host: wssHost, port: 7443, remotePort: portWSS},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := newCase(t, "proto-"+tc.name)
			c.Client("frpc", func(s *frpv1alpha1.ClientSpec_Server) {
				s.Host, s.Port, s.Protocol = tc.host, tc.port, ptr(tc.name)
			})
			proxy := c.Upstream("frpc", "echo", tcpEcho(tc.remotePort))

			c.waitClientSynced("frpc")
			c.waitProxyRunning("frpc", proxy)
			expectTCPEcho(t, env.FrpsIP, tc.remotePort)
		})
	}
}
```

- [ ] **Step 3: Run to verify a wrong pairing fails**

Temporarily set the `quic` row's port to `7000` (kcp's port). Run `make test-e2e`.
Expected: `TestServerProtocols/quic` fails waiting for `frp_proxy_status ... running`, and its frpc log shows a quic dial error. Then restore `7001`.

- [ ] **Step 4: Run to verify it passes**

Run: `make test-e2e`
Expected: `--- PASS: TestServerProtocols` with all 4 subtests PASS.

- [ ] **Step 5: Commit**

```bash
git add test/e2e/protocol_feature_test.go
git commit -m "test(e2e): kcp, quic, websocket and wss (via TLS terminator) transports"
```

---

### Task 10: TLS server verification and mTLS

**Files:**
- Create: `test/e2e/tls_feature_test.go`

**Interfaces:**
- Consumes: `newCase`, `Case.Secret`, `Case.Client`, `Case.Upstream`, `tcpEcho`, `waitClientSynced`, `waitProxyRunning`, `waitFrpcLog`, `expectTCPEcho`, `env.Certs`, `env.FrpsIP`, `env.FrpsMTLSIP`, `frpsMTLSHost`, `portTLS`, `portMTLS` (Tasks 3 and 5).
- Produces: nothing new.

Facts:
- The operator mounts the whole Secret named by `certFile` (or by `trustedCaFile` if there's no cert) at `/etc/frp/tls`, with fixed file names `tls.crt`, `tls.key`, `ca.crt`.
- A wrong CA fails with Go's `x509: certificate signed by unknown authority`.
- A missing client cert against forced mTLS fails with `remote error: tls: ...`.

- [ ] **Step 1: Write the tests**

```go
//go:build e2e

package e2e

import (
	"testing"

	frpv1alpha1 "github.com/zufardhiyaulhaq/frp-operator/api/v1alpha1"
)

func trustCA(secret string) *frpv1alpha1.ClientSpec_Server_TLS {
	return &frpv1alpha1.ClientSpec_Server_TLS{
		Enable:        true,
		TrustedCAFile: &frpv1alpha1.ConfigMapOrSecretRef{Secret: &frpv1alpha1.Secret{Name: secret, Key: "ca.crt"}},
	}
}

// TestTLSServerVerification: a Client that trusts the right CA connects; one that trusts an
// unrelated CA is rejected, which proves verification is really on.
func TestTLSServerVerification(t *testing.T) {
	c := newCase(t, "tls")
	c.Secret("frps-ca", map[string][]byte{"ca.crt": env.Certs.CA})
	c.Secret("wrong-ca", map[string][]byte{"ca.crt": env.Certs.OtherCA})

	c.Client("trusted", func(s *frpv1alpha1.ClientSpec_Server) { s.TLS = trustCA("frps-ca") })
	proxy := c.Upstream("trusted", "echo", tcpEcho(portTLS))
	c.Client("untrusted", func(s *frpv1alpha1.ClientSpec_Server) { s.TLS = trustCA("wrong-ca") })

	c.waitClientSynced("trusted")
	c.waitProxyRunning("trusted", proxy)
	expectTCPEcho(t, env.FrpsIP, portTLS)
	c.waitFrpcLog("untrusted", "x509")
}

// TestMutualTLS: frps-mtls forces client certificates. A Client with one connects; a Client
// without one is rejected.
func TestMutualTLS(t *testing.T) {
	c := newCase(t, "mtls")
	c.Secret("client-tls", map[string][]byte{"tls.crt": env.Certs.ClientCert, "tls.key": env.Certs.ClientKey, "ca.crt": env.Certs.CA})
	c.Secret("frps-ca", map[string][]byte{"ca.crt": env.Certs.CA})
	ref := func(key string) *frpv1alpha1.SecretRef {
		return &frpv1alpha1.SecretRef{Secret: frpv1alpha1.Secret{Name: "client-tls", Key: key}}
	}

	c.Client("with-cert", func(s *frpv1alpha1.ClientSpec_Server) {
		s.Host = frpsMTLSHost
		s.TLS = trustCA("client-tls")
		s.TLS.CertFile, s.TLS.KeyFile = ref("tls.crt"), ref("tls.key")
	})
	proxy := c.Upstream("with-cert", "echo", tcpEcho(portMTLS))
	c.Client("without-cert", func(s *frpv1alpha1.ClientSpec_Server) {
		s.Host = frpsMTLSHost
		s.TLS = trustCA("frps-ca")
	})

	c.waitClientSynced("with-cert")
	c.waitProxyRunning("with-cert", proxy)
	expectTCPEcho(t, env.FrpsMTLSIP, portMTLS)
	c.waitFrpcLog("without-cert", "remote error: tls:")
}
```

- [ ] **Step 2: Run to verify the negative checks are real**

Temporarily point `untrusted` at `trustCA("frps-ca")`. Run `make test-e2e`.
Expected: `TestTLSServerVerification` fails at `frpc untrusted logs "x509"`, because nothing is rejected. Then restore `"wrong-ca"`.

- [ ] **Step 3: Run to verify they pass**

Run: `make test-e2e`
Expected: `--- PASS: TestTLSServerVerification`, `--- PASS: TestMutualTLS`.

- [ ] **Step 4: Commit**

```bash
git add test/e2e/tls_feature_test.go
git commit -m "test(e2e): TLS server verification (wrong CA rejected) and mTLS"
```

---

### Task 11: Egress proxy (http + socks5) proven by NetworkPolicy

**Files:**
- Create: `test/e2e/egress_feature_test.go`

**Interfaces:**
- Consumes: `newCase`, `Case.Secret`, `Case.Client`, `Case.Upstream`, `tcpEcho`, `waitClientSynced`, `waitProxyRunning`, `expectTCPEcho`, `eventually`, `must`, `podReady`, `env.exec`, `env.logs`, `egressProxyHost`, `frpsMainHost`, `proxyUser`, `proxyPassword`, `egressNamespace`, `backendsNamespace`, `portProxyHTTP`, `portProxySOCKS` (Tasks 3 and 5).
- Produces: nothing new.

Facts:
- k3s enforces NetworkPolicy by default.
- 3proxy logs one JSON line per connection, containing `"auth":{"user":"<user>"}` and `"server":{"ip":"…", "port":<port>}`.

- [ ] **Step 1: Write the test**

```go
//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	frpv1alpha1 "github.com/zufardhiyaulhaq/frp-operator/api/v1alpha1"
)

// onlyEgressViaProxy lets pods in the case namespace reach DNS, the egress proxy and the
// backends — and nothing else, so frps is only reachable through the proxy.
func (c *Case) onlyEgressViaProxy() {
	c.t.Helper()
	udp, tcp, dns := corev1.ProtocolUDP, corev1.ProtocolTCP, intstr.FromInt32(53)
	to := func(namespace string) []networkingv1.NetworkPolicyPeer {
		return []networkingv1.NetworkPolicyPeer{{NamespaceSelector: &metav1.LabelSelector{
			MatchLabels: map[string]string{"kubernetes.io/metadata.name": namespace},
		}}}
	}
	must(c.t, env.Kube.Create(context.Background(), &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "only-via-egress-proxy", Namespace: c.NS},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{},
			PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeEgress},
			Egress: []networkingv1.NetworkPolicyEgressRule{
				{To: to("kube-system"), Ports: []networkingv1.NetworkPolicyPort{{Protocol: &udp, Port: &dns}, {Protocol: &tcp, Port: &dns}}},
				{To: to(egressNamespace)},
				{To: to(backendsNamespace)},
			},
		},
	}))
}

// probeEgressBlocked proves the policy is enforced from inside the case namespace: the egress
// proxy is reachable and frps is not.
func (c *Case) probeEgressBlocked() {
	c.t.Helper()
	ctx := context.Background()
	must(c.t, env.Kube.Create(ctx, &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "probe", Namespace: c.NS},
		Spec: corev1.PodSpec{Containers: []corev1.Container{{
			Name: "probe", Image: "nicolaka/netshoot:v0.14", Command: []string{"sleep", "infinity"},
		}}},
	}))
	eventually(c.t, 2*time.Minute, "probe pod ready", func(ctx context.Context) error {
		p := &corev1.Pod{}
		if err := env.Kube.Get(ctx, client.ObjectKey{Namespace: c.NS, Name: "probe"}, p); err != nil {
			return err
		}
		if !podReady(p) {
			return fmt.Errorf("%s", podProblem(p))
		}
		return nil
	})
	eventually(c.t, 30*time.Second, "egress proxy reachable from the case namespace", func(ctx context.Context) error {
		_, err := env.exec(ctx, c.NS, "probe", "probe", "nc", "-z", "-w", "3", egressProxyHost, "3128")
		return err
	})
	eventually(c.t, 30*time.Second, "frps blocked from the case namespace", func(ctx context.Context) error {
		for i := 0; i < 3; i++ {
			if _, err := env.exec(ctx, c.NS, "probe", "probe", "nc", "-z", "-w", "3", frpsMainHost, "7000"); err == nil {
				return fmt.Errorf("frps is reachable directly: the NetworkPolicy is not enforced")
			}
		}
		return nil
	})
}

// TestEgressProxy runs frpc behind an http and a socks5 egress proxy with Secret-sourced
// credentials. frps is unreachable directly, so working traffic proves the proxy carried it.
func TestEgressProxy(t *testing.T) {
	cases := []struct {
		name       string
		port       int
		remotePort int
	}{
		{name: "http", port: 3128, remotePort: portProxyHTTP},
		{name: "socks5", port: 1080, remotePort: portProxySOCKS},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := newCase(t, "proxy-"+tc.name)
			c.onlyEgressViaProxy()
			c.probeEgressBlocked()
			c.Secret("egress-proxy", map[string][]byte{"username": []byte(proxyUser), "password": []byte(proxyPassword)})
			c.Client("frpc", func(s *frpv1alpha1.ClientSpec_Server) {
				s.Transport = &frpv1alpha1.ClientSpec_Server_Transport{
					ProxyURL: fmt.Sprintf("%s://%s:%d", tc.name, egressProxyHost, tc.port),
					ProxyCredentials: &frpv1alpha1.ClientSpec_Server_Transport_ProxyCredentials{
						Username: frpv1alpha1.SecretRef{Secret: frpv1alpha1.Secret{Name: "egress-proxy", Key: "username"}},
						Password: frpv1alpha1.SecretRef{Secret: frpv1alpha1.Secret{Name: "egress-proxy", Key: "password"}},
					},
				}
			})
			proxy := c.Upstream("frpc", "echo", tcpEcho(tc.remotePort))

			c.waitClientSynced("frpc")
			c.waitProxyRunning("frpc", proxy)
			expectTCPEcho(t, env.FrpsIP, tc.remotePort)

			// The proxy logged an authenticated connection to frps's bind port.
			eventually(t, 30*time.Second, "egress proxy logged the frps connection", func(ctx context.Context) error {
				pods, err := env.Clientset.CoreV1().Pods(egressNamespace).List(ctx, metav1.ListOptions{LabelSelector: "app=egress-proxy"})
				if err != nil || len(pods.Items) == 0 {
					return fmt.Errorf("egress proxy pod: %v", err)
				}
				out, err := env.logs(ctx, egressNamespace, pods.Items[0].Name, "proxy")
				if err != nil {
					return err
				}
				for _, line := range strings.Split(out, "\n") {
					if strings.Contains(line, `"user":"`+proxyUser+`"`) && strings.Contains(line, `"port":7000`) {
						return nil
					}
				}
				return fmt.Errorf("no log line for user %q to port 7000", proxyUser)
			})
		})
	}
}
```

- [ ] **Step 2: Run to verify the policy check catches a missing policy**

Temporarily comment out `c.onlyEgressViaProxy()`. Run `make test-e2e`.
Expected: `TestEgressProxy/http` fails with `frps is reachable directly: the NetworkPolicy is not enforced`. Then restore it.

- [ ] **Step 3: Run to verify it passes**

Run: `make test-e2e`
Expected: `--- PASS: TestEgressProxy` (`http` and `socks5`).

- [ ] **Step 4: Commit**

```bash
git add test/e2e/egress_feature_test.go
git commit -m "test(e2e): egress proxy (http, socks5) proven by NetworkPolicy and proxy log"
```

---

### Task 12: LoadBalancer Service via ServerPool

**Files:**
- Create: `test/e2e/loadbalancer_feature_test.go`

**Interfaces:**
- Consumes: `env.upsert`, `env.deleteAndWait`, `env.dumpCase`, `eventually`, `must`, `ptr`, `expectTCPEcho`, `expectUDPEcho`, `operatorNamespace`, `backendsNamespace`, `frpsMainHost`, `frpsToken`, `env.FrpsIP`, `portLoadBalancer` (Tasks 3 and 5).
- Produces: nothing new.

Facts:
- The ServerPool and its token Secret must live in the operator namespace.
- The generated Upstream dials `<svc>.<ns>.svc.cluster.local:<servicePort>`, and the remote port equals the Service port.
- The Service's `status.loadBalancer.ingress` is set only once the generated Client is Ready and every proxy runs, with `ipMode: Proxy`.
- k3s servicelb is disabled (Task 4), so only frp-operator claims the Service.

- [ ] **Step 1: Write the test**

```go
//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	frpv1alpha1 "github.com/zufardhiyaulhaq/frp-operator/api/v1alpha1"
)

// TestLoadBalancerService binds a type=LoadBalancer Service (TCP and UDP on one port) to a
// ServerPool server and checks the ingress address and the traffic.
func TestLoadBalancerService(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	must(t, env.upsert(ctx, &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "e2e-frps-token", Namespace: operatorNamespace},
		Data:       map[string][]byte{"token": []byte(frpsToken)},
	}))
	pool := &frpv1alpha1.ServerPool{
		ObjectMeta: metav1.ObjectMeta{Name: "e2e", Namespace: operatorNamespace},
		Spec: frpv1alpha1.ServerPoolSpec{Servers: []frpv1alpha1.ServerPoolServer{{
			Name: "sg-01", Host: frpsMainHost, Port: 7000, PublicAddress: env.FrpsIP,
			Authentication: frpv1alpha1.ClientSpec_Server_Authentication{
				Token: &frpv1alpha1.ClientSpec_Server_Authentication_Token{Secret: frpv1alpha1.Secret{Name: "e2e-frps-token", Key: "token"}},
			},
		}}},
	}
	must(t, env.upsert(ctx, pool))

	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "lb-echo", Namespace: backendsNamespace},
		Spec: corev1.ServiceSpec{
			Type:              corev1.ServiceTypeLoadBalancer,
			LoadBalancerClass: ptr("frp.zufardhiyaulhaq.com/frp"),
			Selector:          map[string]string{"app": "echo"},
			Ports: []corev1.ServicePort{
				{Name: "echo-tcp", Port: portLoadBalancer, Protocol: corev1.ProtocolTCP, TargetPort: intstr.FromInt32(9000)},
				{Name: "echo-udp", Port: portLoadBalancer, Protocol: corev1.ProtocolUDP, TargetPort: intstr.FromInt32(9001)},
			},
		},
	}
	must(t, env.deleteAndWait(ctx, svc.DeepCopy())) // a leftover from an earlier failed run
	must(t, env.Kube.Create(ctx, svc))
	t.Cleanup(func() {
		if t.Failed() {
			env.dumpCase(context.Background(), operatorNamespace)
			return
		}
		_ = env.deleteAndWait(context.Background(), svc)
		_ = env.Kube.Delete(context.Background(), pool)
	})

	eventually(t, 3*time.Minute, "LoadBalancer ingress is the frps address", func(ctx context.Context) error {
		got := &corev1.Service{}
		if err := env.Kube.Get(ctx, client.ObjectKeyFromObject(svc), got); err != nil {
			return err
		}
		ingress := got.Status.LoadBalancer.Ingress
		if len(ingress) != 1 || ingress[0].IP != env.FrpsIP {
			return fmt.Errorf("ingress = %+v, want one entry with IP %s", ingress, env.FrpsIP)
		}
		if ingress[0].IPMode == nil || *ingress[0].IPMode != corev1.LoadBalancerIPModeProxy {
			return fmt.Errorf("ipMode = %v, want Proxy", ingress[0].IPMode)
		}
		return nil
	})
	expectTCPEcho(t, env.FrpsIP, portLoadBalancer)
	expectUDPEcho(t, env.FrpsIP, portLoadBalancer)
}
```

- [ ] **Step 2: Run to verify it fails without the operator's class**

Temporarily set `LoadBalancerClass: ptr("example.com/other")`. Run `make test-e2e`.
Expected: `TestLoadBalancerService` fails at `LoadBalancer ingress is the frps address` with `ingress = []`. Then restore it.

- [ ] **Step 3: Run to verify it passes**

Run: `make test-e2e`
Expected: `--- PASS: TestLoadBalancerService`.

- [ ] **Step 4: Commit**

```bash
git add test/e2e/loadbalancer_feature_test.go
git commit -m "test(e2e): LoadBalancer Service via ServerPool, TCP and UDP on one port"
```

---

### Task 13: Lifecycle — reload without restart, InvalidConfig → recovery

**Files:**
- Create: `test/e2e/lifecycle_feature_test.go`

**Interfaces:**
- Consumes: `newCase`, `Case.Secret`, `Case.Client`, `Case.Upstream`, `tcpEcho`, `waitClientSynced`, `waitProxyRunning`, `waitCondition`, `waitEvent`, `waitMetric`, `Case.pod`, `expectTCPEcho`, `must`, `frpsToken`, `portReloadFirst`, `portReloadSecond`, `portInvalid` (Tasks 3 and 5).
- Produces: nothing new.

Facts:
- The operator picks up new Upstreams on its 30 s requeue.
- A reload waits for the kubelet to sync the ConfigMap volume, then runs `frpc verify` and the admin reload. The pod is not replaced.
- An invalid spec sets `ConfigSynced=False`, reason `InvalidConfig`, with a message like `invalid configuration: key "token" not found in secret <ns>/<name>`. It also emits a Warning event `InvalidConfig` and sets `frp_client_config_synced` to 0.
- The operator doesn't watch Secrets, so recovery comes from the error backoff (Review Focus #5).

- [ ] **Step 1: Write the tests**

```go
//go:build e2e

package e2e

import (
	"context"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	frpv1alpha1 "github.com/zufardhiyaulhaq/frp-operator/api/v1alpha1"
)

func restarts(p *corev1.Pod) int32 {
	var n int32
	for _, cs := range p.Status.ContainerStatuses {
		n += cs.RestartCount
	}
	return n
}

// TestReloadWithoutRestart adds an Upstream to a running Client: the new proxy must come up
// through a config reload, with the same frpc pod and no container restart.
func TestReloadWithoutRestart(t *testing.T) {
	c := newCase(t, "reload")
	c.Client("frpc", nil)
	first := c.Upstream("frpc", "first", tcpEcho(portReloadFirst))
	c.waitClientSynced("frpc")
	c.waitProxyRunning("frpc", first)
	before := c.pod("frpc")

	second := c.Upstream("frpc", "second", tcpEcho(portReloadSecond))
	c.waitProxyRunning("frpc", second)
	expectTCPEcho(t, env.FrpsIP, portReloadSecond)
	expectTCPEcho(t, env.FrpsIP, portReloadFirst)

	after := c.pod("frpc")
	if after.UID != before.UID {
		t.Fatalf("frpc pod was replaced (%s -> %s), want an in-place reload", before.UID, after.UID)
	}
	if n := restarts(after); n != 0 {
		t.Fatalf("frpc container restarted %d times, want 0", n)
	}
}

// TestInvalidConfigThenRecovery: a token Secret without the referenced key must surface as
// InvalidConfig in status, events and metrics; fixing the Secret must bring the Client back.
func TestInvalidConfigThenRecovery(t *testing.T) {
	c := newCase(t, "invalid")
	ctx := context.Background()
	c.Secret("broken-token", map[string][]byte{"other": []byte("x")})
	c.Client("frpc", func(s *frpv1alpha1.ClientSpec_Server) {
		s.Authentication.Token.Secret = frpv1alpha1.Secret{Name: "broken-token", Key: "token"}
	})
	proxy := c.Upstream("frpc", "echo", tcpEcho(portInvalid))

	cond := c.waitCondition("frpc", "ConfigSynced", metav1.ConditionFalse, "InvalidConfig")
	if !strings.Contains(cond.Message, `key "token" not found`) {
		t.Errorf("ConfigSynced message = %q, want it to name the missing key", cond.Message)
	}
	c.waitEvent("frpc", "InvalidConfig")
	c.waitMetric("frp_client_config_synced", map[string]string{"namespace": c.NS, "client": "frpc"}, 0)

	secret := &corev1.Secret{}
	must(t, env.Kube.Get(ctx, client.ObjectKey{Namespace: c.NS, Name: "broken-token"}, secret))
	secret.Data["token"] = []byte(frpsToken)
	must(t, env.Kube.Update(ctx, secret))

	c.waitClientSynced("frpc")
	c.waitMetric("frp_client_config_synced", map[string]string{"namespace": c.NS, "client": "frpc"}, 1)
	c.waitProxyRunning("frpc", proxy)
	expectTCPEcho(t, env.FrpsIP, portInvalid)
}
```

- [ ] **Step 2: Run to verify the reload check catches a restart**

Temporarily add `must(t, env.Kube.Delete(context.Background(), c.pod("frpc")))` right after `before := c.pod("frpc")`. Run `make test-e2e`.
Expected: `TestReloadWithoutRestart` fails with `frpc pod was replaced`. Then remove the line.

- [ ] **Step 3: Run to verify they pass**

Run: `make test-e2e`
Expected: `--- PASS: TestReloadWithoutRestart`, `--- PASS: TestInvalidConfigThenRecovery`.

- [ ] **Step 4: Commit**

```bash
git add test/e2e/lifecycle_feature_test.go
git commit -m "test(e2e): reload without pod restart, InvalidConfig and recovery"
```

---

### Task 14: CI job, docs, and the 10-minute budget

**Files:**
- Modify: `.github/workflows/pullrequest.yml` (add job `e2e`)
- Modify: `.github/workflows/main.yml` (replace the existing `e2e` job, from `  e2e:` to the end of the file)
- Modify: `AGENTS.md`
- Modify (workflow repo): `~/Documents/personal/github/workflow/skills/frp-operator-verify/SKILL.md`, `~/Documents/personal/github/workflow/.claude/agent-memory/frp-operator-developer/MEMORY.md`

**Interfaces:**
- Consumes: `make test-e2e` with `E2E_SKIP_IMAGE_BUILD=1`, and `output/e2e-artifacts` (Tasks 4 and 5).
- Produces: GitHub Actions job `e2e` on PRs and `main`.

- [ ] **Step 1: Write the failing check**

```bash
python3 -c "import yaml; [print(f, 'e2e' in yaml.safe_load(open(f))['jobs'] and 'make test-e2e' in open(f).read()) for f in ['.github/workflows/pullrequest.yml','.github/workflows/main.yml']]"
```

Expected now: `pullrequest.yml False`, `main.yml False`. `main.yml` has an `e2e` job, but it runs no `make test-e2e`.

- [ ] **Step 2: Add the job**

In `.github/workflows/pullrequest.yml`, append this job under `jobs:`. In `.github/workflows/main.yml`, **replace** the whole existing `e2e:` job (the `nolar/setup-k3d-k3s` + `sleep 180` one, which runs to the end of the file) with the same block:

```yaml
  e2e:
    runs-on: ubuntu-latest
    timeout-minutes: 20
    steps:
      - name: Set up Go
        uses: actions/setup-go@v2
        with:
          go-version: 1.23
      - name: Check out code
        uses: actions/checkout@v2
      - name: Install k3d
        run: curl -fsSL https://raw.githubusercontent.com/k3d-io/k3d/main/install.sh | TAG=v5.9.0 bash
      - name: Install helm
        uses: azure/setup-helm@v4
        with:
          version: v3.16.4
      - name: Set up Docker Buildx
        uses: docker/setup-buildx-action@f95db51fddba0c2d1ec667646a06c2ce06100226 # v3.0.0
      - name: Build operator image
        uses: docker/build-push-action@0565240e2d4ab88bba5387d719585280857ece09 # v5.0.0
        with:
          context: .
          load: true
          tags: frp-operator:e2e
          platforms: linux/amd64
          cache-from: type=gha,scope=e2e
          cache-to: type=gha,mode=max,scope=e2e
      - name: Run e2e suite
        run: make test-e2e E2E_SKIP_IMAGE_BUILD=1
      - name: Upload e2e artifacts
        if: failure()
        uses: actions/upload-artifact@v4
        with:
          name: e2e-artifacts
          path: output/e2e-artifacts
          if-no-files-found: ignore
```

- [ ] **Step 3: Run the check to verify it passes**

Run the Step 1 command again.
Expected: `pullrequest.yml True`, `main.yml True`. `grep -c nolar .github/workflows/main.yml` prints `0`.

- [ ] **Step 4: Document the targets**

In `AGENTS.md`, under `# Test` in the command block, add:

```bash
make test-e2e           # End-to-end on a private k3d cluster (frps in-cluster, real traffic); needs Docker, k3d, helm
make e2e-down           # Delete the e2e k3d cluster
```

In the Directory Structure block, add:

```
test/e2e/           # e2e suite (build tag e2e); untagged helpers are unit-tested by make test
```

In `~/Documents/personal/github/workflow/skills/frp-operator-verify/SKILL.md`, add after the "Build" gate item:

```markdown
5. **End to end (Docker + k3d + helm):** `make test-e2e` — creates the private k3d cluster
   `frp-operator-e2e` (its own kubeconfig in `output/`), installs the chart with the local image,
   deploys frps/backends/proxy, and runs every Tier 1 case with real traffic. `make e2e-down`
   afterwards. Failed cases keep their namespace and dump to `output/e2e-artifacts/`.
```

In `~/Documents/personal/github/workflow/.claude/agent-memory/frp-operator-developer/MEMORY.md`, under `## Verification commands`, add:

```markdown
- `make test-e2e` — private k3d cluster `frp-operator-e2e` (kubeconfig `output/e2e-kubeconfig`, guard refuses any other context), chart install with `frp-operator:e2e`, in-cluster frps-main / frps-mtls / nginx wss terminator, Tier 1 cases with real traffic. CI job `e2e` runs it on every PR. New features get a case in `test/e2e/` (names via `Case.Upstream`, ports in `ports.go`).
```

Under `## frp facts`, add:

```markdown
- frps cannot accept `wss` itself — it needs a TLS terminator in front (frp's own e2e uses one).
- The operator mounts the whole TLS Secret at `/etc/frp/tls` with fixed names `tls.crt`/`tls.key`/`ca.crt`; the CR's `key` field is ignored.
```

- [ ] **Step 5: Measure the budget and idempotency locally**

```bash
make e2e-down
time make test-e2e
make test-e2e
```

Expected: the first run passes all cases, from cluster creation, in under 10 minutes. Record the time in the PR description. The second run, on the same cluster, passes too (Review Focus #2).

If the first run is over 10 minutes, raise `-parallel 4` to `-parallel 6` in the `test-e2e` target and re-measure. Do not raise timeouts.

- [ ] **Step 6: Final verification gate**

```bash
gofmt -l api pkg controllers test
go vet ./... && go vet -tags e2e ./test/e2e/
make test
make test-frpc-config
```

Expected: `gofmt` prints nothing, vet is clean, and every package reports `ok`.

- [ ] **Step 7: Commit (frp-operator repo only)**

```bash
git add .github/workflows/pullrequest.yml .github/workflows/main.yml AGENTS.md
git commit -m "ci: run the e2e suite on every PR and main (replaces the sleep-based e2e job)"
```

Leave the workflow-repo skill and memory edits uncommitted. That repo commits only when the owner asks; mention them in the report.

---

## Follow-ups (Tier 2, not in this plan)

- XTCP + STCP fallback (decide the node/NAT topology first), cross-user `allowUsers` with `user`/`serverUser`.
- OIDC: a second frps with `auth.method = "oidc"` and a mock IdP (for example `navikt/mock-oauth2-server`).
- Plugins (socks5, http_proxy, static_file, https2http, …), loadBalancer groups, proxyProtocol.
- **Operator:** `ClientReconciler` doesn't watch Secrets or Upstreams. Changes wait for the 30 s requeue, and InvalidConfig recovery waits for error backoff, which can grow to minutes. Consider watching referenced Secrets, or returning `RequeueAfter` instead of an error.
