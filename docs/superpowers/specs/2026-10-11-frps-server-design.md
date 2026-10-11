{% raw %}
# frps Server CRD — design

Agreed with the owner in a brainstorming session on 2026-10-11 (Q1–Q10, design sections 1–4).
This is the source of truth for the implementation plan.

## Goal

Let frp-operator run **frps** in a Kubernetes cluster that has a public IP (typically through a
`LoadBalancer` Service). Private clusters running frp-operator then connect their Clients and
ServerPools to it. Today the operator only manages the client side and points at frps servers
that someone else runs.

## Decisions

| # | Decision |
|---|---|
| Q1 | A new **`Server`** custom resource, reconciled by the operator (not a static chart mode, not docs only) |
| Q2 | Public ports are **declared** on the Server (`allowPorts`) and expanded into the Service; nothing is exposed dynamically |
| Q3 | The Server publishes a **connection Secret** (`token`, `ca.crt`); you copy it to private clusters with any tool. No cross-cluster credentials |
| Q4 | **No new client-side field.** The connection Secret's keys match what Client and ServerPool already use (`authentication.token.secret`, `tls.trustedCaFile`); the address comes from `status.address` |
| Q5 | v1 scope: bind ports, allowPorts, vhost, token auth, tunnel TLS / mTLS, Service options, dashboard + metrics, `podTemplate`. Out: server-side OIDC, frps server plugins, multiple replicas |
| — | frps runs as **one bare Pod** (owner question). frps instances share no state, so more than one replica behind one load balancer breaks routing; availability comes from several Servers plus a private-side ServerPool |
| Q6 | `allowPorts` is **split by protocol** (`tcp`, `udp`), so the Service carries no unneeded UDP and doesn't double the load-balancer listeners |
| Q7 | API fields are **grouped by who connects**: `bind` (frpc → frps), `allowPorts` and `vhost` (Internet → proxies) |
| Q8 | Tunnel TLS lives under **`bind.tls`**. vhost HTTPS is SNI passthrough; frps holds no website certificates |
| Q9 | The dashboard is exposed **only through a Service**, never an Ingress, and never on the proxy load balancer |
| Q10 | The dashboard can have its **own TLS certificate**. An exposed dashboard without one gets a Warning event |

## Verified frp v0.71.0 facts this design relies on

- frps can't reload its configuration (frpc can), so any config change means restarting frps.
- frps `allowPorts` doesn't distinguish protocols. A TCP and a UDP proxy can claim the same number.
- vhost HTTPS routes by TLS SNI and passes the encrypted stream through. TLS ends at the backend.
- The dashboard UI, its admin API (which includes `DELETE /api/proxies`) and `/metrics` share one
  web server behind one basic-auth login (`server/api_router.go:31-36`).
- `frps verify` exists, like `frpc verify`.

## API

```yaml
apiVersion: frp.zufardhiyaulhaq.com/v1alpha1
kind: Server
metadata: {name: public-01, namespace: frp-system}
spec:
  bind:                                    # frpc → frps (the tunnel)
    port: 7000                             # required; tcp and websocket clients
    kcpPort: 7000                          # optional, UDP (kcp clients)
    quicPort: 7001                         # optional, UDP (quic clients)
    tls:                                   # optional; secures frpc ↔ frps only
      certificate: {secret: {name: frps-cert}}       # tls.crt, tls.key (+ ca.crt)
      clientCA: {secret: {name: frps-client-ca}}    # ca.crt → client certificates required (mTLS)
  allowPorts:                              # Internet → TCP/UDP Upstreams, one port per proxy
    tcp: ["8000-8010"]
    udp: ["5353"]
  vhost:                                   # Internet → HTTP/HTTPS/TCPMUX, shared by domain (optional)
    httpPort: 80
    httpsPort: 443
    tcpmuxPort: 5002
    subdomainHost: example.com
  authentication:
    token: {secret: {name: my-token, key: token}}   # optional; generated if omitted
  service:                                 # the public proxy Service
    type: LoadBalancer                     # default
    annotations: {}
    loadBalancerClass: ""
    externalTrafficPolicy: Cluster
  dashboard:
    enabled: false                         # UI + admin API on 7500
    credentials: {secret: {name: my-dashboard}}     # username/password; generated if omitted
    tls: {certificate: {secret: {name: frps-dashboard-cert}}}   # optional; HTTPS dashboard
    service:                               # dedicated Service, separate from the proxy Service
      type: ClusterIP                      # default; or NodePort / LoadBalancer
      annotations: {}
      loadBalancerClass: ""
      loadBalancerSourceRanges: []
  metrics:
    enabled: false                         # enablePrometheus; also starts the web server
    serviceMonitor: false
  podTemplate: {}                          # same shape as Client
status:
  address: 203.0.113.10                    # load-balancer ingress IP or hostname
  exposedPorts: {tcp: [...], udp: [...]}   # copy into a private ServerPool's allowedPorts
  connectionSecret: public-01-connection
  conditions: [Ready, ConfigSynced]
```

What each port group is for:

| Group | Who connects | Notes |
|---|---|---|
| `bind.port` / `kcpPort` / `quicPort` | frpc from private clusters | Chosen by the Client's `spec.server.protocol` (`tcp`/`websocket` → `port`, `kcp` → `kcpPort`, `quic` → `quicPort`). `wss` needs a TLS terminator in front of `port` |
| `allowPorts.tcp` / `.udp` | Internet users of TCP/UDP Upstreams | One port per proxy; frps opens it when a client claims it |
| `vhost.*` | Internet users of HTTP/HTTPS/TCPMUX Upstreams | One port shared by any number of domains, routed by Host / SNI / CONNECT host |

**Validation.** A failure sets `ConfigSynced=False/InvalidConfig` with a Warning event, and the previous pod keeps serving. The rules:
- every port is in 1–65535
- `bind`, `vhost` and `allowPorts` don't overlap within the same protocol
- `kcpPort` ≠ `quicPort`
- the expanded public Service ports stay at or below the operator flag `--server-max-ports` (default 50)
- every referenced Secret and key exists

## Reconciliation

A new `ServerReconciler` owns these objects for each Server:

| Object | Name | Notes |
|---|---|---|
| Secret | `<server>-frps-config` | The rendered `frps.toml`. A Secret, because it holds the token |
| Secret | `<server>-connection` | `token` (generated once if `authentication` is omitted, otherwise copied from your Secret) + `ca.crt` (when `bind.tls.certificate` has one). This is what private clusters copy |
| Secret | `<server>-dashboard` | Generated `username`/`password`, only if `dashboard.credentials` is omitted and the web server runs |
| Pod | `<server>-frps` | Image `fatedier/frps:v0.71.0` (the same pin as frpc). Readiness = TCP on `bind.port`. `podTemplate` merged in |
| Service | `<server>-frps` | `spec.service.type` (default `LoadBalancer`). Bind ports, vhost ports and expanded `allowPorts`, each with its own protocol |
| Service | `<server>-frps-web` | Dashboard, admin API and `/metrics` on 7500. Created only when `dashboard.enabled` or `metrics.enabled`. `dashboard.service.type` (default `ClusterIP`) |
| ServiceMonitor | `<server>-frps` | Only with `metrics.serviceMonitor`. Basic auth from the dashboard credentials; HTTPS when dashboard TLS is set |

**Applying changes.** The pod carries an annotation holding a hash of the rendered config plus the content of every referenced Secret. When the desired hash differs, the operator deletes the pod and creates a new one, the same flow Client uses for an image change. The reconciler **watches the Secrets a Server references**, so a rotated token or certificate takes effect at once. Connected frpc clients reconnect on their own.

**Status:**

| Condition | Meaning |
|---|---|
| `Ready=True` | The pod is ready **and** `status.address` is set |
| `Ready=False/AddressPending` | The load balancer is still provisioning |
| `Ready=False/LoadBalancerFailed` | The cloud rejected the Service (for example mixed TCP/UDP); the cloud controller's `SyncLoadBalancerFailed` message is copied |
| `Ready=False/PodFailed` | The frps pod is crashing |
| `ConfigSynced=True` | The running pod has the current config hash |
| `ConfigSynced=False/InvalidConfig` | The spec is invalid; the previous pod keeps serving |

A Warning event `DashboardInsecure` is emitted when `dashboard.service.type` is `NodePort` or `LoadBalancer` and `dashboard.tls` is unset.

**Deletion.** Owner references remove every object. The load balancer's IP goes with it.

## Connecting a private cluster

1. **Public cluster:** apply the Server and wait for `status.address` and `Ready=True`.
2. **Copy `<server>-connection`** to the private cluster. A documented `kubectl get secret … | jq | kubectl apply` works, or any secret-sync tool.
3. **Private cluster:** write a normal Client (or a ServerPool server):
   - `host` = `status.address`, `port` = `bind.port`
   - `authentication.token.secret` → the copied Secret's `token`
   - `tls.trustedCaFile` → its `ca.crt` (when `bind.tls` is set)

   A ServerPool's `allowedPorts` should match `status.exposedPorts`; the docs say so.
4. **DNS for vhost:** point `customDomains`, or `*.<subdomainHost>`, at `status.address`.

## Testing

- **Unit:**
  - rendering `frps.toml` (every field, optional keys omitted when unset, strings quoted)
  - port expansion and validation (overlap, protocol split, max ports)
  - the config hash
  - reading the address from the Service
- **Real binary:** every Server feature rendered and checked with `frps verify` from the pinned image, extending `make test-frpc-config`.
- **e2e:**
  - A Server with `service.type: ClusterIP` (the e2e cluster has no cloud load balancer), and a Client connected to `<server>-frps.<ns>.svc` that carries TCP echo traffic.
  - A spec change rolls the pod, and the Client reconnects.
  - A missing token key surfaces as `InvalidConfig`.
  - The dashboard Service answers with basic auth.

## Delivery

- **CRD:** a new `Server` CRD in `charts/frp-operator/crds/crds.yaml`.
- **Chart:** RBAC for `servers`, and for `servicemonitors` when enabled.
- **Operator flag:** `--server-max-ports`.
- **Docs:** `examples/server/` with `server.yaml`, `client.yaml`, `upstream.yaml` and a README; a README section.
- **Release:** v0.13.0 (minor: new CRD).
- **Jekyll:** the spec and plan are wrapped in raw tags, because GitHub Pages runs every Markdown file through Liquid.

## Out of scope

- server-side OIDC
- frps server plugins
- multiple frps replicas behind one load balancer
- any Ingress
- a client-side connection field
- cross-cluster automation (reading the Server from a private cluster)
- dynamic port exposure
{% endraw %}
