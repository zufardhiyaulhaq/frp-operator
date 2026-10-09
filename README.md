# frp-operator

Expose your service in Kubernetes to the Internet with open source FRP!

![Version: 1.10.0](https://img.shields.io/badge/Version-1.10.0-informational?style=flat-square) ![Type: application](https://img.shields.io/badge/Type-application-informational?style=flat-square) ![AppVersion: 0.12.0](https://img.shields.io/badge/AppVersion-0.12.0-informational?style=flat-square) [![made with Go](https://img.shields.io/badge/made%20with-Go-brightgreen)](http://golang.org) [![Github main branch build](https://img.shields.io/github/workflow/status/zufardhiyaulhaq/frp-operator/Main)](https://github.com/zufardhiyaulhaq/frp-operator/actions/workflows/main.yml) [![GitHub issues](https://img.shields.io/github/issues/zufardhiyaulhaq/frp-operator)](https://github.com/zufardhiyaulhaq/frp-operator/issues) [![GitHub pull requests](https://img.shields.io/github/issues-pr/zufardhiyaulhaq/frp-operator)](https://github.com/zufardhiyaulhaq/frp-operator/pulls)[![Artifact Hub](https://img.shields.io/endpoint?url=https://artifacthub.io/badge/repository/frp-operator)](https://artifacthub.io/packages/search?repo=frp-operator)

## Features

**Custom Resources**
- `Client` — declarative FRP client instance connecting to an external FRP server
- `Upstream` — a Kubernetes Service/port exposed through FRP
- `Visitor` — inbound tunnel that consumes another client's STCP/XTCP `Upstream` (for P2P scenarios)
- `ServerPool` — a pool of frps servers that backs `Service type=LoadBalancer` (see [LoadBalancer Controller](#loadbalancer-controller))

**Upstream protocols**
- `TCP` and `UDP` — straightforward port forwarding with optional health checks and bandwidth limits
- `STCP` — secret-key TCP, private to clients that share the key (with `allowUsers`)
- `XTCP` — encrypted peer-to-peer with NAT traversal, including the `enableAssistedAddrs` toggle for STUN-only or full address discovery, and an STCP fallback visitor
- `HTTP` / `HTTPS` — virtual-host routing, custom headers, locations, basic auth, and per-route health checks
- `TCPMUX` — multiplexed TCP for sharing a single server port across upstreams

**Secure access to the FRP Server**
- Token authentication (sourced from Kubernetes `Secret`)
- OIDC authentication
- TLS to the FRP server, including mutual TLS verification
- STCP/XTCP secret keys (sourced from Kubernetes `Secret`) and `allowUsers`, with `user` on `Client` and `serverUser` on `Visitor` for cross-user access

**Advanced traffic features**
- FRP plugins on `Upstream` (e.g. static_file, unix_domain_socket, http_proxy, socks5, https2http, etc.)
- Transport tuning on `Client` (protocol: tcp/kcp/quic/websocket/wss, pool count, multiplexing, dial timeout/keepalive, egress proxy via `proxyURL` with Secret-sourced `proxyCredentials`, also used for the OIDC token request) and per `Upstream` (encryption, compression, bandwidth limits)
- Load balancing across TCP, HTTP, and HTTPS upstreams via FRP groups (HTTPS groups need frps >= v0.66.0)
- Group-level health checks

**Operational features**
- Pod templates on `Client` to set resources, node selectors, tolerations, labels, annotations, affinity, security context, environment variables (e.g. `GOMEMLIMIT`), and more
- `enabled: false` on `Upstream` and `Visitor` to pause a tunnel without deleting it (its port is released)
- Wire protocol v2 (`transport.wireProtocol`) on `Client` for an AEAD-encrypted control channel (frps >= v0.69.0)
- Each `Client` reports a `clientID` of `<namespace>/<name>` in the frps dashboard (frps >= v0.67.0; set `spec.clientID: ""` to omit) — frps allows only one online client per clientID
- Reliable, restart-free config reload — operator `exec`s into the pod and verifies `/frp/config.toml` matches the expected state, and runs `frpc verify` on the rendered config before triggering the reload
- Validation for duplicate `Upstream` server ports (per protocol, so 53/TCP and 53/UDP can coexist), duplicate `Visitor` ports, and missing Secrets/keys, surfaced via descriptive errors
- Expose `Service type=LoadBalancer` through a pool of frps servers (`ServerPool` + `loadBalancerClass: frp.zufardhiyaulhaq.com/frp`)
- Helm chart with native CRDs and RBAC
- Prometheus metrics for every `Client` and proxy (`frp_client_ready`, `frp_client_config_synced`, `frp_proxy_status`, ...) plus controller-runtime metrics, served on `:8080` (or `:8443` with Kubernetes token authentication when `metrics.secure=true`)
- Optional Prometheus Operator `ServiceMonitor` and a Grafana dashboard (`dashboards/frp-operator.json`)

## Document
1. [RFC: Fast Reverse Proxy Operator](https://docs.google.com/document/d/18_X4KKLNMAFcfYP-Nh0wwU31RP903IrLuc1Uemxcpoo)

## Installing

To install the chart with the release name `my-release`:

```console
helm repo add frp-operator https://zufardhiyaulhaq.com/frp-operator/charts/releases/
helm install my-frp-operator frp-operator/frp-operator --values values.yaml
```

## Prerequisite
To expose your private Kubernetes service into public network. You need public machine running FRP Server that act as a proxy. Currently the operator doesn't have capability to spine a new machine on cloud providers, but this can be setup in a minute.

1. Create machine on cloud provider
2. Download `frps` [binary](https://github.com/fatedier/frp)
3. Create server configuration
```
vi frps.ini

[common]
bind_address = 0.0.0.0
bind_port = 7000
token = yourtoken
```
4. Run FRP server
```
frps -c ./frps.ini
```

You can reuse our build-in ansible playbook to setup the FRP server on your machine, please check https://github.com/zufardhiyaulhaq/frp-operator/tree/main/ansible/server

## Usage
1. Apply some example
```console
kubectl apply -f examples/deployment/
kubectl apply -f examples/client/
```
2. Check frpc object
```console
kubectl get client
NAME        AGE
client-01   17m

kubectl get upstream
NAME    AGE
nginx   17m
```

3. access the URL
```console
http://178.128.100.87:8080/
```

## LoadBalancer Controller

Besides the hand-written `Client`/`Upstream` flow, the operator can act as a load-balancer controller: a `Service` of `type: LoadBalancer` with `loadBalancerClass: frp.zufardhiyaulhaq.com/frp` is bound to one frps server from a `ServerPool`, the operator generates the `Client`/`Upstream` resources in its own namespace, and the server's public address lands in `status.loadBalancer.ingress` — exactly like a cloud load balancer, but through FRP. Charts and tools that already create LoadBalancer Services work unchanged.

### ServerPool

A `ServerPool` lives in the operator namespace (next to the frps token Secrets) and lists the frps servers a Service may be bound to:

```yaml
apiVersion: frp.zufardhiyaulhaq.com/v1alpha1
kind: ServerPool
metadata:
  name: prod
  namespace: frp-operator           # the operator's namespace
spec:
  allocationPolicy: Auto            # Auto (default): eligible without annotation | Explicit: only when a Service names it
  clientMode: PerService            # PerService (default): one frpc pod per Service per server | Shared: one frpc pod per server
  allowedPorts: ["80", "443", "8000-9000"]   # optional; empty = any port
  servers:
  - name: sg-01                     # stable key used in annotations and generated names (lowercase, max 40 chars)
    host: 178.128.100.87            # address frpc dials
    port: 7000
    publicAddress: 178.128.100.87   # IP or hostname written to the Service status
    allowedPorts: ["443"]           # optional per-server override
    authentication:
      token:
        secret:
          name: sg-01-token
          key: token
    # transportProtocol, tls, transport, adminServer: same shapes as Client.spec.server
  clientTemplate:                   # optional; applied to every generated Client
    podTemplate:
      resources:
        limits:
          memory: 64Mi
```

`status.servers[].allocatedPorts` shows which Service holds which port on each server, and the `Ready` condition reports validation problems (missing token Secret, duplicate or invalid server names, bad `allowedPorts`).

### Service

```yaml
apiVersion: v1
kind: Service
metadata:
  name: web
  namespace: default
  annotations:
    frp.zufardhiyaulhaq.com/server-pool: prod   # optional
    frp.zufardhiyaulhaq.com/server: sg-01       # optional; "sg-01,sg-02" for one IP per server
spec:
  type: LoadBalancer
  loadBalancerClass: frp.zufardhiyaulhaq.com/frp
  selector:
    app: web
  ports:
  - name: https
    port: 443          # opened as remotePort 443 on the frps server
    targetPort: 8443
    protocol: TCP
```

`kubectl get svc web` shows `<pending>` until the generated frpc client is `Ready` and its proxies are `running`, then the server's `publicAddress`. `kubectl describe svc web` shows the Events explaining a pending state; the generated objects are `lb-default-web-sg-01` (`Client`) and `lb-default-web-sg-01-443` (`Upstream`) in the operator namespace.

### Annotations

| Annotation on the Service | Meaning |
|---|---|
| `frp.zufardhiyaulhaq.com/server-pool: <name>` | Use only this pool (required for pools with `allocationPolicy: Explicit`) |
| `frp.zufardhiyaulhaq.com/server: sg-01[,sg-02]` | Bind to exactly these servers; one ingress IP each, all-or-nothing |
| `frp.zufardhiyaulhaq.com/allocated-server` | Written by the operator; the current binding |

### Rules

- A Service gets **one** server (one IP) unless it pins several. All its ports land on that server with `remotePort` = Service port.
- Pools are tried in name order, servers in spec order; the first server with every requested port free (and allowed by `allowedPorts`) wins. Bindings are sticky: a bound Service never moves unless its server is removed from the pool.
- If no server fits, the Service stays `<pending>` with an Event (`PortUnavailable`, `PortNotAllowed`, `NoServerAvailable`, …). First come, first served: a running Service never loses its IP to another one. Delete the holder and the pending Service binds within seconds.
- The ingress address appears only after the generated frpc client is `Ready` and every proxy reports `running`.
- `clientMode: Shared` runs one frpc pod per server for all Services; `PerService` (default) runs one per Service per server.
- Only TCP and UDP ports are supported. Set `loadBalancer.enabled=false` to turn the controller off.
- Editing the `frp.zufardhiyaulhaq.com/server-pool` or `frp.zufardhiyaulhaq.com/server` annotation on a bound Service is honoured and re-selects (a live tunnel may move); a pinned multi-server Service is all-or-nothing: if one pinned server is removed from the pool, all of its tunnels are torn down until the binding can be satisfied again.

### Uninstalling / disabling

Every claimed Service carries the finalizer `frp.zufardhiyaulhaq.com/loadbalancer`, which the operator removes once it has cleaned up that Service's generated `Client`/`Upstream` objects. Before running `helm uninstall` or setting `loadBalancer.enabled=false`, either delete the claimed Services or remove their `loadBalancerClass` so the operator can clean up first — otherwise those Services are left stuck deleting (or stuck with a stale finalizer) once the controller stops running.

If the operator is already gone and a Service is stuck on the finalizer, strip it manually:
```console
kubectl patch svc <name> -p '{"metadata":{"finalizers":null}}' --type=merge
```
then delete the generated `Client`/`Upstream` objects yourself (label `frp.zufardhiyaulhaq.com/managed-by=loadbalancer`) in the operator namespace.

### Security note

The operator matches `Upstream` objects to `Client` objects by name, across namespaces — an `Upstream` naming a generated Client (`pool-<pool>-<server>` in `clientMode: Shared`, `lb-<ns>-<svc>-<server>` in `PerService`) attaches to that Client's live tunnel regardless of which namespace the `Upstream` lives in. This means any user who can create `Upstream` objects can attach to a generated Client if they can guess or discover its name. Restrict `create` on `upstreams.frp.zufardhiyaulhaq.com` to trusted namespaces via RBAC.

See [`examples/loadbalancer`](https://github.com/zufardhiyaulhaq/frp-operator/tree/main/examples/loadbalancer).

## Monitoring

The operator exposes Prometheus metrics on the `<release>-controller-manager-metrics-service` Service (port `http`/8080 by default):

| Metric | Labels | Description |
|---|---|---|
| `frp_client_info` | `namespace`, `client`, `server_address`, `server_port`, `client_id`, `frpc_image` | Always 1 |
| `frp_client_ready` | `namespace`, `client` | 1 when the `Ready` condition is True |
| `frp_client_config_synced` | `namespace`, `client` | 1 when the `ConfigSynced` condition is True |
| `frp_client_upstreams` / `frp_client_visitors` | `namespace`, `client` | Attached resource counts |
| `frp_client_admin_up` | `namespace`, `client` | 1 when the frpc admin API answered `/api/status` |
| `frp_client_last_reconcile_timestamp_seconds` | `namespace`, `client` | Unix time of the last reconcile |
| `frp_proxy_status` | `namespace`, `client`, `proxy`, `type`, `status` | One-hot frpc proxy phase (`running`, `start error`, `check failed`, `closed`, `wait start`, `new`) |
| `frp_proxy_info` | `namespace`, `client`, `proxy`, `type`, `local_addr`, `remote_addr`, `plugin` | Always 1 |
| `frp_loadbalancer_service` | `namespace`, `service`, `pool`, `server`, `state` | Always 1; state is bound or pending |
| `frp_serverpool_allocated_ports` | `pool`, `server` | Ports allocated on the server |

Enable scraping with `metrics.serviceMonitor.enabled=true` (Prometheus Operator; the VictoriaMetrics operator also consumes `ServiceMonitor` objects). With `metrics.secure=true` the scraper's ServiceAccount must be allowed `GET /metrics` (bind it to the `<release>-metrics-reader` ClusterRole).

A Grafana dashboard lives at [`dashboards/frp-operator.json`](https://github.com/zufardhiyaulhaq/frp-operator/blob/main/dashboards/frp-operator.json) — import it into Grafana (Dashboards → New → Import) and pick your Prometheus/VictoriaMetrics datasource.

## Values

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| loadBalancer.enabled | bool | `true` | Bind `Service type=LoadBalancer` with `loadBalancerClass: frp.zufardhiyaulhaq.com/frp` to `ServerPool` servers |
| metrics.secure | bool | `false` |  |
| metrics.serviceMonitor.additionalLabels | object | `{}` |  |
| metrics.serviceMonitor.enabled | bool | `false` |  |
| metrics.serviceMonitor.interval | string | `"30s"` |  |
| operator.image | string | `"ghcr.io/zufardhiyaulhaq/frp-operator"` |  |
| operator.replica | int | `1` |  |
| operator.tag | string | `"v0.12.0"` |  |
| resources.limits.cpu | string | `"200m"` |  |
| resources.limits.memory | string | `"100Mi"` |  |
| resources.requests.cpu | string | `"100m"` |  |
| resources.requests.memory | string | `"20Mi"` |  |

see example files [here](https://github.com/zufardhiyaulhaq/frp-operator/blob/main/charts/frp-operator/values.yaml)

----------------------------------------------
Autogenerated from chart metadata using [helm-docs v1.14.2](https://github.com/norwoodj/helm-docs/releases/v1.14.2)
