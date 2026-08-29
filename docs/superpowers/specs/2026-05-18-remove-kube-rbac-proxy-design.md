{% raw %}
# Remove kube-rbac-proxy from frp-operator

**Date:** 2026-05-18
**Status:** Approved
**Reference:** [istio-ratelimit-operator@b33b0cb1](https://github.com/zufardhiyaulhaq/istio-ratelimit-operator/commit/b33b0cb1d86bfe0a93a9ae56289d6178a2cf20d3)

## Background

`kube-rbac-proxy` is the sidecar that kubebuilder-scaffolded operators have historically used to put authn/authz in front of the manager's `/metrics` endpoint. It is now deprecated, and controller-runtime ships an equivalent built-in: `metricsserver.Options{SecureServing, FilterProvider, TLSOpts}` combined with `sigs.k8s.io/controller-runtime/pkg/metrics/filters.WithAuthenticationAndAuthorization`. The manager performs TLS termination, TokenReview, and SubjectAccessReview itself — no sidecar required.

This migration mirrors the same change already shipped in `istio-ratelimit-operator` (commit `b33b0cb1`).

## Goal

Remove the `kube-rbac-proxy` sidecar from the frp-operator Helm chart and serve metrics securely from the manager binary directly.

## Architecture

```
BEFORE: Prometheus → :8443 (kube-rbac-proxy sidecar) → :8080 (manager, plaintext, in-pod)
AFTER:  Prometheus → :8443 (manager, TLS + authn/authz filter)
```

The manager opens `:8443` with controller-runtime's secure metrics server. Each scrape request is authenticated via Kubernetes TokenReview and authorized via SubjectAccessReview against the `nonResourceURL` `/metrics`. The `metrics-reader` ClusterRole continues to grant that permission; clients (e.g. Prometheus) authenticate using a projected ServiceAccount token bound to it.

HTTP/2 is disabled by default to mitigate CVE-2023-44487 (Rapid Reset) and CVE-2023-39325 (GOAWAY flood) on the metrics and webhook servers. An `--enable-http2` flag is provided as an opt-in escape hatch.

## Changes

### 1. `main.go`

- Import `crypto/tls` and `sigs.k8s.io/controller-runtime/pkg/metrics/filters`.
- Change `--metrics-bind-address` default from `:8080` to `:8443`.
- Add flag `--metrics-secure` (bool, default `true`): if set, the metrics endpoint is served over HTTPS with the authn/authz filter applied. `--metrics-secure=false` falls back to plaintext HTTP without the filter.
- Add flag `--enable-http2` (bool, default `false`): if set, enables HTTP/2 on the metrics and webhook servers.
- Build a `tlsOpts []func(*tls.Config)` slice: when `!enableHTTP2`, append a function that sets `c.NextProtos = []string{"http/1.1"}` and logs "disabling http/2".
- Build `metricsServerOptions := metricsserver.Options{BindAddress: metricsAddr, SecureServing: secureMetrics, TLSOpts: tlsOpts}`. If `secureMetrics`, set `metricsServerOptions.FilterProvider = filters.WithAuthenticationAndAuthorization`.
- Pass `metricsServerOptions` as the `Metrics` field of `ctrl.Options`.

### 2. `charts/frp-operator/templates/deployment.yaml`

- Remove the entire `kube-rbac-proxy` container (the first entry in `containers:`).
- Update the `manager` container args:
  - Replace `--metrics-bind-address=127.0.0.1:8080` with `--metrics-bind-address=:8443`.
  - Add `--metrics-secure=true`.
- Add a `ports` block to the manager container exposing `containerPort: 8443, name: https, protocol: TCP`.

### 3. `charts/frp-operator/templates/clusterrole.yaml`

- Add two rules to the existing `{{ .Release.Name }}-manager-role` ClusterRole:
  - `apiGroups: ["authentication.k8s.io"]`, `resources: ["tokenreviews"]`, `verbs: ["create"]`
  - `apiGroups: ["authorization.k8s.io"]`, `resources: ["subjectaccessreviews"]`, `verbs: ["create"]`
- Delete the standalone `{{ .Release.Name }}-proxy-role` ClusterRole document at the bottom of the file (lines 161–177 in current file).
- Leave the `{{ .Release.Name }}-metrics-reader` ClusterRole unchanged — Prometheus still uses it.

### 4. `charts/frp-operator/templates/clusterrolebinding.yaml`

- Delete the `{{ .Release.Name }}-proxy-rolebinding` ClusterRoleBinding document (the second YAML document in the file).
- Leave `{{ .Release.Name }}-manager-rolebinding` unchanged.

### 5. `charts/frp-operator/templates/service.yaml`

- Change `targetPort: https` to `targetPort: 8443`. The named port `https` now lives on the manager container, but the numeric form keeps the chart resilient to port-name changes (matches the reference commit).

### 6. `config/` (Kustomize manifests)

Even though AGENTS.md notes the Kustomize manifests are "mostly not used" (the chart is canonical), keep them in lockstep with the chart.

- Delete `config/default/manager_auth_proxy_patch.yaml`.
- Create `config/default/manager_metrics_patch.yaml` containing a JSON patch:
  ```yaml
  - op: add
    path: /spec/template/spec/containers/0/args/0
    value: --metrics-bind-address=:8443
  - op: add
    path: /spec/template/spec/containers/0/args/0
    value: --metrics-secure=true
  ```
- Update `config/default/kustomization.yaml`: replace the `patchesStrategicMerge: [manager_auth_proxy_patch.yaml]` block with:
  ```yaml
  patches:
  - path: manager_metrics_patch.yaml
    target:
      kind: Deployment
  ```
- Delete `config/rbac/auth_proxy_role.yaml`.
- Delete `config/rbac/auth_proxy_role_binding.yaml`.
- Rename `config/rbac/auth_proxy_service.yaml` → `config/rbac/metrics_service.yaml`, and update its `targetPort: https` → `targetPort: 8443`.
- Rename `config/rbac/auth_proxy_client_clusterrole.yaml` → `config/rbac/metrics_reader_clusterrole.yaml` (no content change).
- Update `config/rbac/kustomization.yaml`: remove `auth_proxy_service.yaml`, `auth_proxy_role.yaml`, `auth_proxy_role_binding.yaml`, `auth_proxy_client_clusterrole.yaml`; add `metrics_service.yaml`, `metrics_reader_clusterrole.yaml`. Remove the "Comment the following 4 lines…" instructional comment.

## Testing & Verification

Per AGENTS.md, manual end-to-end validation against orbstack:

1. **Build:** `make manifests && make generate && make build` — should compile cleanly with the new imports.
2. **Vet / lint:** `make fmt && make vet && make lint`.
3. **Chart render:** `helm template charts/frp-operator/` — verify:
   - No container named `kube-rbac-proxy` in the rendered Deployment.
   - Manager container has `--metrics-bind-address=:8443`, `--metrics-secure=true`, and exposes `containerPort: 8443`.
   - `manager-role` ClusterRole contains `tokenreviews` and `subjectaccessreviews` rules.
   - No `proxy-role` ClusterRole or `proxy-rolebinding` ClusterRoleBinding.
   - Metrics Service `targetPort: 8443`.
4. **Run locally:** `make install run`, apply `examples/simple/`, verify the operator still creates ConfigMap / Service / Pod for `Client` resources and that FRP client logs look healthy.
5. **Metrics auth:**
   - `kubectl port-forward svc/<release>-controller-manager-metrics-service 8443:8443`
   - Unauthenticated: `curl -k https://localhost:8443/metrics` → expect 401/403.
   - Authenticated with metrics-reader-bound SA token: `curl -k -H "Authorization: Bearer $(kubectl create token <sa>)" https://localhost:8443/metrics` → expect 200 with Prometheus metrics.

## Release notes

Add to the next chart release notes:

> **BREAKING:** The `kube-rbac-proxy` sidecar has been removed. Metrics are now served directly by the manager on `:8443` using controller-runtime's built-in secure metrics server with Kubernetes TokenReview / SubjectAccessReview. Existing Prometheus scrape configs continue to work if they target the `*-controller-manager-metrics-service` Service and authenticate with a ServiceAccount token bound to the `*-metrics-reader` ClusterRole. The `gcr.io/kubebuilder/kube-rbac-proxy` image is no longer pulled.

## Out of scope

- No controller-runtime version bump (`v0.18.4` already supports `metricsserver.Options.FilterProvider` and `pkg/metrics/filters`).
- No CRD or `api/v1alpha1` changes.
- No controller reconciliation logic changes.
- No `values.yaml` opt-in toggle to keep the proxy — this is a clean breaking change per the brainstorm decision.
- Chart version bump in `charts/frp-operator/Chart.yaml` is handled by the normal release flow (`make helm.create.releases`), not this change.
- `make readme` regeneration is part of the release flow.
{% endraw %}
