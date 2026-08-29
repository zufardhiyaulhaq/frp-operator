{% raw %}
# Remove kube-rbac-proxy Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the deprecated `kube-rbac-proxy` sidecar with controller-runtime's built-in secure metrics server, so the operator serves authenticated HTTPS metrics itself on `:8443`.

**Architecture:** The manager binary opens `:8443` using `metricsserver.Options{SecureServing: true, FilterProvider: filters.WithAuthenticationAndAuthorization}`. Kubernetes TokenReview + SubjectAccessReview enforce access. The Helm chart drops the sidecar container, the standalone `proxy-role` ClusterRole, and the proxy ClusterRoleBinding; auth rules are merged into the existing `manager-role`. The Kustomize manifests (`config/`) are updated to match.

**Tech Stack:** Go 1.23 · controller-runtime v0.18.4 · Kubebuilder v3 · Helm v3 · Kustomize

**Spec:** `docs/superpowers/specs/2026-05-18-remove-kube-rbac-proxy-design.md`

**Reference commit:** [istio-ratelimit-operator@b33b0cb1](https://github.com/zufardhiyaulhaq/istio-ratelimit-operator/commit/b33b0cb1d86bfe0a93a9ae56289d6178a2cf20d3)

---

## File Structure

**Modified:**
- `main.go` — secure metrics wiring, new flags
- `charts/frp-operator/templates/deployment.yaml` — sidecar removed, manager opens `:8443`
- `charts/frp-operator/templates/clusterrole.yaml` — auth rules merged, `proxy-role` deleted
- `charts/frp-operator/templates/clusterrolebinding.yaml` — `proxy-rolebinding` deleted
- `charts/frp-operator/templates/service.yaml` — `targetPort: https` → `targetPort: 8443`
- `config/default/kustomization.yaml` — `patchesStrategicMerge` → `patches`
- `config/rbac/kustomization.yaml` — auth_proxy refs → metrics refs

**Created:**
- `config/default/manager_metrics_patch.yaml` — JSON-patch adding secure metrics args

**Deleted:**
- `config/default/manager_auth_proxy_patch.yaml`
- `config/rbac/auth_proxy_role.yaml`
- `config/rbac/auth_proxy_role_binding.yaml`

**Renamed:**
- `config/rbac/auth_proxy_service.yaml` → `config/rbac/metrics_service.yaml` (and edit targetPort)
- `config/rbac/auth_proxy_client_clusterrole.yaml` → `config/rbac/metrics_reader_clusterrole.yaml` (no content change)

**Testing approach:** This change is wiring and YAML. There is no useful unit test for `main()` flag wiring or for Helm templates' literal text — we instead use `go build`, `go vet`, `helm template`, and `grep`-style assertions on rendered output as the verification layer. Each task ends with a verification step + commit.

---

## Task 1: Update `main.go` to serve secure metrics

**Files:**
- Modify: `main.go`

- [ ] **Step 1: Replace the imports block**

Current imports (lines 19–38):

```go
import (
	"flag"
	"os"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	frpv1alpha1 "github.com/zufardhiyaulhaq/frp-operator/api/v1alpha1"
	"github.com/zufardhiyaulhaq/frp-operator/controllers"
	//+kubebuilder:scaffold:imports
)
```

Replace with:

```go
import (
	"crypto/tls"
	"flag"
	"os"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/metrics/filters"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	frpv1alpha1 "github.com/zufardhiyaulhaq/frp-operator/api/v1alpha1"
	"github.com/zufardhiyaulhaq/frp-operator/controllers"
	//+kubebuilder:scaffold:imports
)
```

- [ ] **Step 2: Replace flag declarations and add metricsServerOptions**

Current `main()` body (lines 52–77):

```go
func main() {
	var metricsAddr string
	var enableLeaderElection bool
	var probeAddr string
	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	opts := zap.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                 scheme,
		HealthProbeBindAddress: probeAddr,
		Metrics: metricsserver.Options{
			BindAddress: metricsAddr,
		},
		LeaderElection:   enableLeaderElection,
		LeaderElectionID: "742639e4.zufardhiyaulhaq.com",
	})
```

Replace with:

```go
func main() {
	var metricsAddr string
	var enableLeaderElection bool
	var probeAddr string
	var secureMetrics bool
	var enableHTTP2 bool
	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8443", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.BoolVar(&secureMetrics, "metrics-secure", true,
		"If set, the metrics endpoint is served securely via HTTPS. Use --metrics-secure=false to use HTTP instead.")
	flag.BoolVar(&enableHTTP2, "enable-http2", false,
		"If set, HTTP/2 will be enabled for the metrics and webhook servers.")
	opts := zap.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	var tlsOpts []func(*tls.Config)
	if !enableHTTP2 {
		tlsOpts = append(tlsOpts, func(c *tls.Config) {
			setupLog.Info("disabling http/2")
			c.NextProtos = []string{"http/1.1"}
		})
	}

	metricsServerOptions := metricsserver.Options{
		BindAddress:   metricsAddr,
		SecureServing: secureMetrics,
		TLSOpts:       tlsOpts,
	}

	if secureMetrics {
		metricsServerOptions.FilterProvider = filters.WithAuthenticationAndAuthorization
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                 scheme,
		HealthProbeBindAddress: probeAddr,
		Metrics:                metricsServerOptions,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "742639e4.zufardhiyaulhaq.com",
	})
```

- [ ] **Step 3: Verify it compiles**

Run: `make build`
Expected: builds `bin/manager` with no errors.

- [ ] **Step 4: Run vet and existing tests**

Run: `make vet && make test`
Expected: PASS. (`make test` already produces `output/coverage.html`; no new tests required for this wiring change.)

- [ ] **Step 5: Smoke-test the new flags**

Run: `./bin/manager --help 2>&1 | grep -E "metrics-secure|enable-http2|metrics-bind-address"`
Expected output contains three lines:
- `-enable-http2` (default `false`)
- `-metrics-bind-address` default `:8443`
- `-metrics-secure` (default `true`)

- [ ] **Step 6: Commit**

```bash
git add main.go
git commit -m "feat: serve secure metrics via controller-runtime built-in filter

Replace plaintext :8080 metrics with secure :8443 endpoint using
controller-runtime's metricsserver.Options{SecureServing,
FilterProvider} backed by Kubernetes TokenReview/SubjectAccessReview.
Adds --metrics-secure (default true) and --enable-http2 (default
false) flags. HTTP/2 is disabled by default to mitigate CVE-2023-44487
and CVE-2023-39325."
```

---

## Task 2: Remove kube-rbac-proxy sidecar from Helm Deployment

**Files:**
- Modify: `charts/frp-operator/templates/deployment.yaml`

- [ ] **Step 1: Rewrite the `containers:` block**

Current `containers:` block (lines 24–59):

```yaml
      containers:
      - args:
        - --secure-listen-address=0.0.0.0:8443
        - --upstream=http://127.0.0.1:8080/
        - --logtostderr=true
        - --v=10
        image: gcr.io/kubebuilder/kube-rbac-proxy:v0.13.1
        name: kube-rbac-proxy
        ports:
        - containerPort: 8443
          name: https
          protocol: TCP
      - args:
        - --health-probe-bind-address=:8081
        - --metrics-bind-address=127.0.0.1:8080
        - --leader-elect
        command:
        - /manager
        image: "{{ .Values.operator.image }}:{{ .Values.operator.tag }}"
        imagePullPolicy: Always
        livenessProbe:
          httpGet:
            path: /healthz
            port: 8081
          initialDelaySeconds: 15
          periodSeconds: 20
        name: manager
        readinessProbe:
          httpGet:
            path: /readyz
            port: 8081
          initialDelaySeconds: 5
          periodSeconds: 10
        resources: {{ .Values.resources | toYaml  | nindent 10 }}
        securityContext:
          allowPrivilegeEscalation: false
```

Replace with:

```yaml
      containers:
      - args:
        - --health-probe-bind-address=:8081
        - --metrics-bind-address=:8443
        - --metrics-secure=true
        - --leader-elect
        command:
        - /manager
        image: "{{ .Values.operator.image }}:{{ .Values.operator.tag }}"
        imagePullPolicy: Always
        livenessProbe:
          httpGet:
            path: /healthz
            port: 8081
          initialDelaySeconds: 15
          periodSeconds: 20
        name: manager
        ports:
        - containerPort: 8443
          name: https
          protocol: TCP
        readinessProbe:
          httpGet:
            path: /readyz
            port: 8081
          initialDelaySeconds: 5
          periodSeconds: 10
        resources: {{ .Values.resources | toYaml  | nindent 10 }}
        securityContext:
          allowPrivilegeEscalation: false
```

- [ ] **Step 2: Verify the chart still renders**

Run: `helm template test-release charts/frp-operator/ | grep -A 40 'kind: Deployment'`
Expected: a single Deployment with one container named `manager`. NO `kube-rbac-proxy` container. The manager `args` include `--metrics-bind-address=:8443` and `--metrics-secure=true`.

- [ ] **Step 3: Assert the sidecar is gone**

Run: `helm template test-release charts/frp-operator/ | grep -c 'kube-rbac-proxy' || echo 0`
Expected: `0`

- [ ] **Step 4: Commit**

```bash
git add charts/frp-operator/templates/deployment.yaml
git commit -m "feat(chart): drop kube-rbac-proxy sidecar from Deployment

Manager now serves metrics directly on :8443 with --metrics-secure=true.
The named port 'https' moves to the manager container."
```

---

## Task 3: Merge auth rules into manager-role; drop proxy-role

**Files:**
- Modify: `charts/frp-operator/templates/clusterrole.yaml`

- [ ] **Step 1: Add tokenreviews and subjectaccessreviews rules to manager-role**

Find the `rules:` line under `name: {{ .Release.Name }}-manager-role` (currently line 10). Insert these two rules at the top of the rules list (immediately after `rules:`):

```yaml
- apiGroups:
  - authentication.k8s.io
  resources:
  - tokenreviews
  verbs:
  - create
- apiGroups:
  - authorization.k8s.io
  resources:
  - subjectaccessreviews
  verbs:
  - create
```

So lines 10–14 transition from:

```yaml
rules:
- apiGroups:
  - ""
  resources:
  - configmaps
```

To:

```yaml
rules:
- apiGroups:
  - authentication.k8s.io
  resources:
  - tokenreviews
  verbs:
  - create
- apiGroups:
  - authorization.k8s.io
  resources:
  - subjectaccessreviews
  verbs:
  - create
- apiGroups:
  - ""
  resources:
  - configmaps
```

- [ ] **Step 2: Delete the standalone `proxy-role` ClusterRole**

At the bottom of the file (currently lines 161–177), delete this entire document:

```yaml
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: {{ .Release.Name }}-proxy-role
rules:
- apiGroups:
  - authentication.k8s.io
  resources:
  - tokenreviews
  verbs:
  - create
- apiGroups:
  - authorization.k8s.io
  resources:
  - subjectaccessreviews
  verbs:
  - create
```

Leave the `{{ .Release.Name }}-metrics-reader` ClusterRole (lines 151–159) unchanged. After this edit the file should end with the `metrics-reader` document.

- [ ] **Step 3: Verify the chart renders and rules are in the right place**

Run: `helm template test-release charts/frp-operator/ | grep -B1 -A6 'tokenreviews'`
Expected: appears once, inside the `test-release-manager-role` ClusterRole.

Run: `helm template test-release charts/frp-operator/ | grep -c 'proxy-role'`
Expected: `0`

Run: `helm template test-release charts/frp-operator/ | grep -c 'metrics-reader'`
Expected: `1` (the standalone reader ClusterRole, unchanged).

- [ ] **Step 4: Commit**

```bash
git add charts/frp-operator/templates/clusterrole.yaml
git commit -m "feat(chart): merge metrics authn/authz rules into manager-role

The manager now performs TokenReview and SubjectAccessReview itself
to gate /metrics, so the dedicated proxy-role ClusterRole is removed."
```

---

## Task 4: Drop proxy-rolebinding from ClusterRoleBindings

**Files:**
- Modify: `charts/frp-operator/templates/clusterrolebinding.yaml`

- [ ] **Step 1: Delete the proxy-rolebinding document**

Current file ends with two ClusterRoleBinding documents. Delete the second one (currently lines 19–36):

```yaml
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: {{ .Release.Name }}-proxy-rolebinding
  labels:
    app.kubernetes.io/name: {{ .Release.Name }}
    helm.sh/chart: {{ template "frp-operator.chart" . }}
    app.kubernetes.io/instance: {{ .Release.Name }}
    app.kubernetes.io/managed-by: {{ .Release.Service }}
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: {{ .Release.Name }}-proxy-role
subjects:
- kind: ServiceAccount
  name: {{ .Release.Name }}-controller-manager
  namespace: {{ .Release.Namespace }}
```

After this edit, the file contains only the `manager-rolebinding` document and ends at the closing of its `subjects:` list (line 18 of the current file).

- [ ] **Step 2: Verify**

Run: `helm template test-release charts/frp-operator/ | grep -c 'proxy-rolebinding'`
Expected: `0`

Run: `helm template test-release charts/frp-operator/ | grep -c 'manager-rolebinding'`
Expected: `1`

- [ ] **Step 3: Commit**

```bash
git add charts/frp-operator/templates/clusterrolebinding.yaml
git commit -m "feat(chart): remove proxy-rolebinding ClusterRoleBinding

No longer needed now that proxy-role is deleted and the manager
ServiceAccount carries the auth rules directly."
```

---

## Task 5: Fix Service `targetPort`

**Files:**
- Modify: `charts/frp-operator/templates/service.yaml`

- [ ] **Step 1: Change `targetPort`**

Find this block (currently lines 12–16):

```yaml
  ports:
  - name: https
    port: 8443
    protocol: TCP
    targetPort: https
```

Replace `targetPort: https` with `targetPort: 8443`. Result:

```yaml
  ports:
  - name: https
    port: 8443
    protocol: TCP
    targetPort: 8443
```

- [ ] **Step 2: Verify**

Run: `helm template test-release charts/frp-operator/ | grep -A 5 'kind: Service' | grep -E 'targetPort|port:'`
Expected: includes `port: 8443` and `targetPort: 8443`.

- [ ] **Step 3: Commit**

```bash
git add charts/frp-operator/templates/service.yaml
git commit -m "feat(chart): point metrics Service targetPort at 8443

The named port 'https' now lives on the manager container, but the
numeric form keeps the Service spec resilient to port renames."
```

---

## Task 6: Update Kustomize manifests to match

**Files:**
- Delete: `config/default/manager_auth_proxy_patch.yaml`
- Delete: `config/rbac/auth_proxy_role.yaml`
- Delete: `config/rbac/auth_proxy_role_binding.yaml`
- Create: `config/default/manager_metrics_patch.yaml`
- Rename: `config/rbac/auth_proxy_service.yaml` → `config/rbac/metrics_service.yaml`
- Rename: `config/rbac/auth_proxy_client_clusterrole.yaml` → `config/rbac/metrics_reader_clusterrole.yaml`
- Modify: `config/default/kustomization.yaml`
- Modify: `config/rbac/kustomization.yaml`

- [ ] **Step 1: Delete the auth-proxy files**

Run:

```bash
git rm config/default/manager_auth_proxy_patch.yaml \
       config/rbac/auth_proxy_role.yaml \
       config/rbac/auth_proxy_role_binding.yaml
```

Expected: three files staged for deletion.

- [ ] **Step 2: Rename the two surviving files (preserves git history)**

Run:

```bash
git mv config/rbac/auth_proxy_service.yaml config/rbac/metrics_service.yaml
git mv config/rbac/auth_proxy_client_clusterrole.yaml config/rbac/metrics_reader_clusterrole.yaml
```

Expected: two renames staged.

- [ ] **Step 3: Fix `targetPort` in the renamed metrics_service.yaml**

In `config/rbac/metrics_service.yaml`, find the `targetPort: https` line and change it to `targetPort: 8443`. (This mirrors the chart change in Task 5.)

- [ ] **Step 4: Create `config/default/manager_metrics_patch.yaml`**

Write this exact content to `config/default/manager_metrics_patch.yaml`:

```yaml
- op: add
  path: /spec/template/spec/containers/0/args/0
  value: --metrics-bind-address=:8443
- op: add
  path: /spec/template/spec/containers/0/args/0
  value: --metrics-secure=true
```

- [ ] **Step 5: Update `config/default/kustomization.yaml`**

Find the existing `patchesStrategicMerge:` block referencing `manager_auth_proxy_patch.yaml` and replace it with a `patches:` block. The exact replacement:

Old (find and remove the strategic merge that points at `manager_auth_proxy_patch.yaml`):

```yaml
patchesStrategicMerge:
# Protect the /metrics endpoint by putting it behind auth.
# If you want your controller-manager to expose the /metrics
# endpoint w/o any authn/z, please comment the following line.
- manager_auth_proxy_patch.yaml
```

New:

```yaml
patches:
- path: manager_metrics_patch.yaml
  target:
    kind: Deployment
```

Leave all other lines (resources, namePrefix, namespace, bases, manager_config_patch, etc.) untouched.

- [ ] **Step 6: Update `config/rbac/kustomization.yaml`**

Replace the file contents with:

```yaml
resources:
# All RBAC will be applied under this service account in
# the deployment namespace. You may comment out this resource
# if your manager will use a service account that exists at
# runtime. Be sure to update RoleBinding and ClusterRoleBinding
# subjects if changing service account names.
- service_account.yaml
- role.yaml
- role_binding.yaml
- leader_election_role.yaml
- leader_election_role_binding.yaml
- metrics_service.yaml
- metrics_reader_clusterrole.yaml
```

(This removes the four `auth_proxy_*.yaml` lines and the surrounding "Comment the following 4 lines…" instructional comment, and adds the two renamed files.)

- [ ] **Step 7: Verify no references to auth_proxy remain**

Run: `grep -rn 'auth_proxy\|kube-rbac-proxy' config/ charts/ main.go || echo 'CLEAN'`
Expected: `CLEAN` (a single literal `CLEAN` line printed by `echo`).

- [ ] **Step 8: Verify Kustomize still builds**

Run: `kubectl kustomize config/default/ > /tmp/frp-operator-rendered.yaml && grep -c 'kube-rbac-proxy' /tmp/frp-operator-rendered.yaml || true`
Expected: kustomize emits the YAML without error; the grep prints `0`.

If `kubectl` is not available locally, substitute `kustomize build config/default/ > /tmp/frp-operator-rendered.yaml`. If neither is installed, skip this verification — Task 7 will catch any issues.

- [ ] **Step 9: Commit**

```bash
git add config/
git commit -m "feat(kustomize): align config/ manifests with chart changes

Drop the kube-rbac-proxy patch and RBAC, rename auth_proxy_service /
auth_proxy_client_clusterrole to their metrics_* equivalents, and
switch the default kustomization to a JSON-patch that flips the
manager args to secure metrics on :8443."
```

---

## Task 7: Full verification

**Files:** none modified.

- [ ] **Step 1: Clean build and test**

Run: `make fmt && make vet && make build && make test`
Expected: all four targets succeed. `bin/manager` exists. `output/coverage.html` regenerated.

- [ ] **Step 2: Render full chart and inspect**

Run: `helm template test-release charts/frp-operator/ > /tmp/frp-operator-chart.yaml`
Expected: succeeds with no errors.

- [ ] **Step 3: Assert sidecar and proxy resources are gone from the chart**

Run:

```bash
echo "kube-rbac-proxy count: $(grep -c 'kube-rbac-proxy' /tmp/frp-operator-chart.yaml || true)"
echo "proxy-role count: $(grep -c 'proxy-role' /tmp/frp-operator-chart.yaml || true)"
echo "proxy-rolebinding count: $(grep -c 'proxy-rolebinding' /tmp/frp-operator-chart.yaml || true)"
```

Expected: all three counts are `0`.

- [ ] **Step 4: Assert the new manager args are present**

Run:

```bash
grep -E -- '--metrics-bind-address=:8443|--metrics-secure=true' /tmp/frp-operator-chart.yaml
```

Expected: both flags appear in the rendered manager Deployment.

- [ ] **Step 5: Assert tokenreviews live inside the manager-role**

Run: `grep -B2 -A4 'tokenreviews' /tmp/frp-operator-chart.yaml`
Expected: one match, and the nearest preceding `ClusterRole` name is `test-release-manager-role` (or whatever `{{ .Release.Name }}-manager-role` rendered to).

- [ ] **Step 6: End-to-end run against orbstack**

Per AGENTS.md, switch kube-context to orbstack, then:

```bash
make install run &
MANAGER_PID=$!
sleep 5
kubectl --context orbstack apply -f examples/simple/
sleep 10
kubectl --context orbstack get pods,svc,configmap -A | grep frp
kill $MANAGER_PID
```

Expected: the operator reconciles the example `Client`. A Pod, Service, and ConfigMap appear with the expected names. No errors in the operator log relating to metrics startup.

- [ ] **Step 7: Verify secure metrics endpoint with a deployed chart**

Build & load image, then install the chart:

```bash
make docker-build
helm install frp-operator charts/frp-operator/ --kube-context orbstack --create-namespace --namespace frp-operator-system
kubectl --context orbstack -n frp-operator-system wait --for=condition=Available deploy --all --timeout=60s
kubectl --context orbstack -n frp-operator-system port-forward svc/frp-operator-controller-manager-metrics-service 8443:8443 &
PF_PID=$!
sleep 3

# Unauthenticated → expect 401 or 403
curl -sk -o /dev/null -w "no-auth: %{http_code}\n" https://localhost:8443/metrics

# Authenticated with metrics-reader-bound SA token → expect 200
kubectl --context orbstack -n frp-operator-system create serviceaccount metrics-reader-test
kubectl --context orbstack create clusterrolebinding metrics-reader-test \
    --clusterrole=frp-operator-metrics-reader \
    --serviceaccount=frp-operator-system:metrics-reader-test
TOKEN=$(kubectl --context orbstack -n frp-operator-system create token metrics-reader-test)
curl -sk -o /dev/null -w "with-auth: %{http_code}\n" \
    -H "Authorization: Bearer $TOKEN" https://localhost:8443/metrics

kill $PF_PID
kubectl --context orbstack delete clusterrolebinding metrics-reader-test
kubectl --context orbstack -n frp-operator-system delete serviceaccount metrics-reader-test
helm uninstall frp-operator --kube-context orbstack --namespace frp-operator-system
```

Expected output:
- `no-auth: 401` (or `403`)
- `with-auth: 200`

- [ ] **Step 8: No commit for this task**

Task 7 is verification-only. If any step fails, return to the relevant earlier task, fix, recommit there, and re-run Task 7 from Step 1.

---

## Out of scope (do NOT touch)

- `Chart.yaml` version bump (handled by `make helm.create.releases` in the release flow per AGENTS.md).
- `README.md` regeneration (handled by `make readme` in the release flow).
- Controller-runtime upgrade (v0.18.4 already supports the secure metrics filter).
- CRD or `api/v1alpha1` changes.
- Controller reconciliation logic in `controllers/` or `pkg/client/`.
- Adding a `values.yaml` toggle to re-enable the proxy.
{% endraw %}
