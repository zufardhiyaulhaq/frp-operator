# Operator Metrics and Grafana Dashboard (v0.10.0)

**Date:** 2026-08-29
**Status:** Approved
**Target release:** frp-operator v0.10.0 / chart 1.8.0

## Background

The operator serves only controller-runtime's generic metrics (`controller_runtime_*`,
`workqueue_*`, Go runtime) on `:8443` over HTTPS with Kubernetes token authentication. Nothing
scrapes it by default and nothing in it is FRP-specific. frpc itself exposes no Prometheus
endpoint — its admin server (`:7400`) is JSON only (`/api/status`, `/api/config`,
`/api/reload`, `/healthz`). So today a Grafana user can see an frpc pod's CPU and memory
(cAdvisor, kube-state-metrics) but not whether any tunnel is up.

Rejected: a per-pod sidecar exporter (extra container per Client, pod-spec churn, another
binary to maintain). The operator already talks to every frpc admin API each reconcile, so it
is the natural exporter.

## Goal

1. Export FRP-specific gauges from the operator's existing `/metrics` endpoint.
2. Make the endpoint scrapable out of the box: plain HTTP by default, chart-generated
   `ServiceMonitor`.
3. Ship a Grafana dashboard in the repository, optionally as a sidecar-loadable ConfigMap.

## Design decisions

1. **Metrics are gauges derived from state the reconciler already has** (Client spec/status)
   plus one new read of frpc's `GET /api/status` per reconcile. No new goroutines, timers, or
   caches — the reconcile loop (30 s requeue) is the sampling interval.
2. **`frp_proxy_status` is one-hot over frpc's status strings.** For each proxy exactly one
   series `{status="<phase>"}` has value 1 and the others 0; phases are frpc's
   `new`, `wait start`, `start error`, `running`, `check failed`, `closed`
   (`client/proxy/proxy_wrapper.go`). This keeps `sum by (status)` and
   `frp_proxy_status{status="running"} == 0` trivial in PromQL.
3. **Status-fetch failure never fails the reconcile.** It sets `frp_client_admin_up` to 0 and
   removes that Client's `frp_proxy_*` series. The `ConfigSync` path is unchanged.
4. **Series are deleted when the Client disappears.** The reconciler's `Get` returning NotFound
   (`controllers/client_controller.go:115-118`) is the only signal (no finalizer exists); it now
   calls `metrics.DeleteClient(namespace, name)` before returning.
5. **Metrics are plain HTTP by default.** New chart value `metrics.secure` (default `false`)
   renders `--metrics-secure=false --metrics-bind-address=:8080` and a Service port `http/8080`.
   `true` restores the current `:8443` HTTPS + token-auth setup. The endpoint carries only
   names and counts, which the user judged acceptable to expose in-cluster.
6. **The scrape object is an opt-in generic `ServiceMonitor`.** `metrics.serviceMonitor.enabled`
   (Prometheus Operator `monitoring.coreos.com/v1`), default `false`, selecting the existing metrics
   Service by its labels. The VictoriaMetrics operator consumes `ServiceMonitor` objects too, so no
   `VMServiceScrape` variant ships *(amended during execution at the user's request)*. When
   `metrics.secure` is `true` it renders `scheme: https`,
   `bearerTokenFile: /var/run/secrets/kubernetes.io/serviceaccount/token`, and
   `tlsConfig.insecureSkipVerify: true`; RBAC for the scraper's ServiceAccount is the user's
   responsibility (documented).
7. **The dashboard JSON in the repo is the single source of truth.**
   `dashboards/frp-operator.json` at the repository root is committed and hand-maintained; the
   same file is imported into the user's Grafana. *(Amended during execution: the user asked for
   the dashboard at the repo root and no helper scripts; the chart ConfigMap option was dropped
   because Helm's `.Files.Get` cannot read outside the chart directory.)*
8. **Only proxies are reported from `/api/status`, not visitors.** frpc's status API does not
   include visitors; `frp_client_visitors` (count from Client status) is the only visitor metric.

## Metrics

Namespace `frp`. All labels below are in addition to `namespace` and `client` (the Client CR's
namespace and name).

| Metric | Type | Extra labels | Value / source |
|---|---|---|---|
| `frp_client_info` | gauge | `server_address`, `server_port`, `client_id`, `frpc_image` | always 1; `client_id` is the resolved value (empty string when omitted) |
| `frp_client_ready` | gauge | — | 1 if condition `Ready` is `True`, else 0 |
| `frp_client_config_synced` | gauge | — | 1 if condition `ConfigSync` is `True`, else 0 |
| `frp_client_upstreams` | gauge | — | `status.upstreamCount` |
| `frp_client_visitors` | gauge | — | `status.visitorCount` |
| `frp_client_admin_up` | gauge | — | 1 if `GET /api/status` returned 200 and parsed |
| `frp_client_last_reconcile_timestamp_seconds` | gauge | — | Unix time of the last completed reconcile |
| `frp_proxy_status` | gauge | `proxy`, `type`, `status` | one-hot per decision 2 |
| `frp_proxy_info` | gauge | `proxy`, `type`, `local_addr`, `remote_addr`, `plugin` | always 1 |

`frp_client_*` (except `admin_up`) are written from `updateStatus` so they always match what
`kubectl get client` shows. `frp_client_admin_up` and `frp_proxy_*` are written right after the
status fetch. Proxies that vanish from `/api/status` (disabled, deleted) have their series
removed on the next reconcile: the package keeps a per-Client set of the label sets it last wrote
and deletes those not re-written.

## Components

### `pkg/client/handler/status.go` (new)

```go
type ProxyStatus struct {
    Name, Type, Status, Err, LocalAddr, Plugin, RemoteAddr string
}
// Status calls GET /api/status on the frpc admin API and flattens
// frpc's map[type][]ProxyStatusResp into a slice sorted by name.
func Status(cfg models.Config) ([]ProxyStatus, error)
```

Mirrors `Reload`: same `AdminAddress:AdminPort`, basic auth from `Common.AdminUsername/Password`,
5 s timeout, non-200 → error with body. JSON field names follow frpc v0.71.0
`client/http/model/types.go` (`name`, `type`, `status`, `err`, `local_addr`, `plugin`,
`remote_addr`).

### `pkg/metrics/metrics.go` (new)

- Declares the gauge vectors above with `prometheus.NewGaugeVec` and registers them on
  `sigs.k8s.io/controller-runtime/pkg/metrics.Registry` in `init()`.
- `RecordClient(c *frpv1alpha1.Client, image string)` — writes the `frp_client_*` gauges from
  spec and status.
- `RecordProxies(namespace, client string, proxies []handler.ProxyStatus, err error)` — writes
  `frp_client_admin_up`, `frp_proxy_status` (one-hot), `frp_proxy_info`; prunes stale proxy series.
- `DeleteClient(namespace, client string)` — `DeletePartialMatch` on every vector for the two
  labels, and clears the tracking set.
- Tracking state: `map[key]map[labelset]struct{}` guarded by a mutex (reconciles for different
  Clients can run concurrently).

### `controllers/client_controller.go`

- NotFound on the Client `Get` → `metrics.DeleteClient(req.Namespace, req.Name)`, return.
- After the pod is confirmed Running and before the ConfigMap-sync/reload block: build `config`
  with the service admin address (already done at line ~374 for the reload; hoist it up), call
  `handler.Status`, then `metrics.RecordProxies`.
- `updateClientStatus` (existing helper that persists status, line ~438) → `metrics.RecordClient` and sets
  `frp_client_last_reconcile_timestamp_seconds`.

### Chart

`values.yaml` additions:

```yaml
metrics:
  # Serve /metrics over HTTPS with Kubernetes token authentication. When false (default)
  # the endpoint is plain HTTP on port 8080.
  secure: false
  serviceMonitor:
    enabled: false
    interval: 30s
    additionalLabels: {}
```

- `deployment.yaml`: args and container port derived from `metrics.secure`
  (`:8080` name `http` / `:8443` name `https`).
- `service.yaml`: port name/number follow the same switch.
- New `servicemonitor.yaml` (wrapped in its `enabled` guard). It uses `endpoints[0].port: http|https`
  and the auth block from decision 6 when secure.
- `Chart.yaml` `version: 1.8.0`, `appVersion: 0.10.0`; `values.yaml` `operator.tag: v0.10.0`;
  `make readme`.

### Dashboard `dashboards/frp-operator.json`

Grafana schema v39+ JSON (works on Grafana 10–13). Title "FRP Operator", uid `frp-operator`,
tags `frp`, `frp-operator`. Variables: `datasource` (type `datasource`, query `prometheus`),
`namespace` (`label_values(frp_client_info, namespace)`, multi, All), `client`
(`label_values(frp_client_info{namespace=~"$namespace"}, client)`, multi, All).

Rows and panels:

1. **Clients** — stats: clients total, ready, config synced, admin API reachable, proxies
   running / not running; table: client, namespace, server, clientID, image, upstreams,
   visitors, ready, synced (joined from `frp_client_info` and the boolean gauges).
2. **Proxies** — table: client, proxy, type, status, local→remote (`frp_proxy_info` joined
   with `frp_proxy_status == 1`); timeseries: running proxies per client; table of proxies
   whose `running` series is 0, with status.
3. **Operator** — reconcile rate and error rate (`controller_runtime_reconcile_total`
   filtered `controller="client"`), p50/p95 reconcile duration, workqueue depth,
   `controller_runtime_reconcile_errors_total` increase.
4. **Resources** — frpc pods CPU/memory/restarts (`container_*` and `kube_pod_container_*`
   filtered by pods created by kind `Client` via `kube_pod_info{created_by_kind="Client"}`),
   operator pod CPU/memory.

All queries use `$datasource`, `namespace=~"$namespace"`, `client=~"$client"`.

### User's Grafana

Push the JSON with `POST /api/dashboards/db` into a new folder "FRP Operator" (created via
`POST /api/folders`) using the API key in `~/.secret/grafana-zufardhiyaulhaq-com`, `overwrite:
true` (done once during execution with a throwaway script; nothing is kept in the repo).

### Docs

- `README.md.gotmpl`: "Prometheus metrics and Grafana dashboard" feature bullet; short
  "Monitoring" section listing the metrics, how to enable scraping, and the dashboard path.
- `AGENTS.md`: add `pkg/metrics/`, `handler/status.go` and `dashboards/` to the architecture list.
- `docs/releases/v0.10.0.md` in the existing format.

## Error handling

- `handler.Status` errors (timeout, 401, malformed JSON) → logged at info level, `admin_up`=0,
  proxy series pruned, reconcile continues.
- Unknown `status` strings from a newer frpc are still emitted as their own label value; the
  one-hot set is the union of the six known phases and whatever was observed.
- Label cardinality is bounded by Clients × proxies; no user-controlled free text other than
  names/addresses already in the CRs.

## Testing

TDD; every production change preceded by a failing test.

- `pkg/client/handler/status_test.go`: `httptest.Server` returning a captured v0.71.0
  `/api/status` body → flattened, sorted slice; 401 and invalid JSON → error; basic-auth header
  asserted.
- `pkg/metrics/metrics_test.go`: uses `prometheus/testutil` on the registry — RecordClient
  produces expected series; RecordProxies one-hot semantics; a proxy that disappears is pruned;
  admin error sets `admin_up`=0 and prunes; DeleteClient removes every series for that Client
  and none for another.
- Controller: unit test that a NotFound Client triggers `DeleteClient` (via a small interface or
  by gathering the registry after reconcile with the fake client).
- Chart: `helm template` cases for `metrics.secure` true/false and each scrape `enabled` flag;
  `helm lint`.
- Dashboard: `python3 -c json.load` sanity check in CI-less form (part of the task's verification
  step); manual check in the user's Grafana after the API push.
- `go build ./... && go vet ./... && go test ./...`. golangci-lint is **not** run (known OOM
  with the pinned version).

## Out of scope

- Per-pod sidecar exporter.
- Alerting rules (`PrometheusRule`/`VMRule`).
- Visitor status (not available from frpc's API).
- Byte/connection counters (frpc does not expose them).
