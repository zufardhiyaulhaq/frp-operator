{% raw %}
# LoadBalancer Services backed by FRP (v0.11.0)

**Date:** 2026-08-30
**Status:** Approved
**Target release:** frp-operator v0.11.0 / chart 1.9.0

## Background

Today a user exposes a Kubernetes Service through frp by hand-writing a `Client` and one
`Upstream` per port. Charts and tools that create `Service type=LoadBalancer` cannot use
the operator at all. This design adds a LoadBalancer controller: a Service with
`spec.loadBalancerClass: frp.zufardhiyaulhaq.com/frp` is allocated a server from a pool of
frps servers, the operator generates the `Client`/`Upstream` resources, and the server's
public address is written to `status.loadBalancer.ingress`.

Gateway API is out of scope: frp cannot express routes, filters or listeners beyond a
port, and most of the API would be unimplementable.

## Goal

- A `Service type=LoadBalancer` with our `loadBalancerClass` gets one public IP/hostname
  from a `ServerPool`, on the ports it asked for, with no other manifests.
- Allocation is deterministic, sticky, and never moves a working Service on its own.
- Existing `Client`/`Upstream`/`Visitor` behaviour is unchanged; the new controller only
  produces those CRs, it does not change how they are reconciled.

## Design decisions

1. **Trigger is `spec.type: LoadBalancer` + `spec.loadBalancerClass: frp.zufardhiyaulhaq.com/frp`.**
   Services without the class are ignored, so the operator coexists with MetalLB / cloud LBs.
2. **`ServerPool` is a namespaced CRD in the operator namespace.** Its token Secrets and all
   generated `Client`/`Upstream` objects live there too; Upstream `host` is
   `<svc>.<ns>.svc.cluster.local`, so nothing is copied across namespaces.
3. **Allocation state is derived from generated Upstreams.** "Is 443 free on `prod/sg-01`?"
   is a label query. No second source of truth; `ServerPool.status` is a projection.
4. **remotePort = Service port, always.** If the requested ports are not all free on one
   server the Service stays Pending. No fallback ports, no ranges.
5. **One server per Service by default; more only when pinned.** A Service gets exactly one
   ingress entry unless `frp.zufardhiyaulhaq.com/server` lists several servers.
6. **Selection is deterministic** (pools sorted by name, servers in spec order, first fit),
   not random or least-used, so restarts and Pending→Ready bounces land on the same server.
7. **Binding is sticky.** After the first allocation the operator records the binding on the
   Service and never re-selects, except when a bound server is removed from the pool spec.
8. **First come, first served.** No priorities, no eviction: a running Service never loses
   its IP to another Service.
9. **Client mode is a pool setting.** `PerService` (default): one generated `Client` per
   Service per server. `Shared`: one generated `Client` per server, shared by all Services.
10. **Pools can opt out of auto-selection** with `allocationPolicy: Explicit`.
11. **Ingress is gated on readiness.** An ingress entry is written only once the generated
    Client is `Ready` and every proxy for the Service's ports reports `running`.

## API

### ServerPool

```yaml
apiVersion: frp.zufardhiyaulhaq.com/v1alpha1
kind: ServerPool
metadata:
  name: prod
  namespace: frp-operator          # operator namespace
spec:
  allocationPolicy: Auto           # Auto (default) | Explicit
  clientMode: PerService           # PerService (default) | Shared
  allowedPorts: ["80", "443", "8000-9000"]   # pool default; empty = any port
  servers:
  - name: sg-01                    # stable key; unique within the pool
    host: 178.128.100.87           # address frpc dials
    port: 7000
    publicAddress: 178.128.100.87  # written to Service status; IP or hostname
    allowedPorts: ["443"]          # optional per-server override (replaces pool default)
    authentication:                # same shape as Client.spec.server.authentication
      token:
        secret: { name: sg-01-token, key: token }
    # optional, same shapes as Client.spec.server:
    # transportProtocol: tcp | kcp | quic | websocket | wss   (Client.spec.server.protocol)
    # tls: {...}
    # transport: {...}
    # adminServer: {...}
  - name: sg-02
    host: 178.128.100.88
    port: 7000
    publicAddress: lb2.example.com
    authentication:
      token:
        secret: { name: sg-02-token, key: token }
  clientTemplate:                  # optional; applied to every generated Client
    podTemplate: {}                # Client.spec.podTemplate
status:
  servers:
  - name: sg-01
    allocatedPorts:
    - { port: 443, protocol: TCP, service: default/service-a }
  conditions:
  - type: Ready                    # False: missing Secret, duplicate server name, bad allowedPorts
    status: "True"
```

Field notes:

- `servers[].name` is the key used in annotations, labels and generated names. Renaming a
  server is a removal plus an addition (bound Services are reallocated).
- `allowedPorts` entries match `^\d+(-\d+)?$` (CRD validation). A server whose allowed set
  does not contain every requested port is skipped by auto-select; a pinned server that does
  not allow a port makes the Service Pending.
- `transportProtocol` is the frpc→frps transport (`Client.spec.server.protocol`). It is not
  related to Service port protocols.
- `publicAddress` may be an IP or a hostname; the operator writes `ingress[].ip` or
  `ingress[].hostname` accordingly. Changing it updates Service status without moving the tunnel.

### Service

```yaml
apiVersion: v1
kind: Service
metadata:
  name: service-a
  annotations:
    frp.zufardhiyaulhaq.com/server-pool: prod      # optional
    frp.zufardhiyaulhaq.com/server: sg-01          # optional; "sg-01,sg-02,sg-03" for several
spec:
  type: LoadBalancer
  loadBalancerClass: frp.zufardhiyaulhaq.com/frp
  selector: { app: web }
  ports:
  - { name: https, port: 443, targetPort: 8443, protocol: TCP }
```

Service port `protocol: TCP` → TCP upstream, `UDP` → UDP upstream, `SCTP` → Pending with an
Event. Each Upstream uses `host: <svc>.<ns>.svc.cluster.local`, `port: <port>`,
`remotePort: <port>`.

### Annotations

User-set:

| Annotation | Meaning |
|---|---|
| `frp.zufardhiyaulhaq.com/server-pool: <name>` | Consider only this pool. Required for `Explicit` pools. Absent → all `Auto` pools sorted by name. |
| `frp.zufardhiyaulhaq.com/server: <name>[,<name>...]` | Bind to exactly these servers, one ingress entry each, all-or-nothing. Absent → auto-pick one server. |

Operator-written:

| Annotation | Meaning |
|---|---|
| `frp.zufardhiyaulhaq.com/allocated-server: <pool>/<server>[,<pool>/<server>...]` | The binding record. While present the Service is never re-selected (see reallocation below). |

## Selection

Inputs: the Service's ports, its two annotations, all `ServerPool`s in the operator
namespace, and existing allocations (generated Upstreams labelled with pool/server/port).

1. Candidate pools: the `server-pool` annotation's pool if set (any policy); otherwise all
   pools with `allocationPolicy: Auto`, sorted by name.
2. If `server` is set:
   - With `server-pool`: every named server must exist in that pool, else `ServerNotFound`.
   - Without `server-pool`: find `Auto` pools containing *all* named servers. Exactly one →
     use it. Zero → `ServerNotFound`. More than one → `AmbiguousServer` (fail loudly rather
     than pick by name).
   - All named servers must allow and have free every Service port, else `PortNotAllowed` /
     `PortUnavailable` naming the blocking server and port. All-or-nothing: no partial binding.
3. If `server` is not set: iterate candidate pools in name order, servers in spec order; the
   first server that allows and has free every Service port wins. None → `NoServerAvailable`.
4. A Service that names an `Explicit` pool with no fit stays Pending; there is no fallback
   to `Auto` pools.

Each failure reason is a Kubernetes Event on the Service (`Warning`, reason = the name
above) and clears `status.loadBalancer.ingress`. The Service is requeued after 30 s and
also whenever a generated Upstream or a `ServerPool` changes.

## Binding and stickiness

- On success the operator patches `frp.zufardhiyaulhaq.com/allocated-server` onto the
  Service. Later reconciles read it and skip selection entirely. Port additions are
  allocated on the bound server(s) only; if unavailable there the Service goes Pending — it
  does not move.
- The binding is dropped and selection re-run only when a bound server no longer exists in
  its pool's `spec.servers` (or the pool is gone). This emits `Warning ServerReallocated
  <old> -> <new>` and is the only case an IP changes without the user editing the Service.
- Editing the `server` annotation is honoured: added servers are allocated, removed servers
  are released (their generated objects deleted, ingress entry dropped).
- Deleting and recreating a Service loses the binding annotation. Users who need a fixed IP
  pin with `server-pool` + `server`.
- Pool deletion: bound Services go Pending, generated objects are deleted, Event
  `PoolDeleted`. The binding annotation is kept, so recreating the pool rebinds to the same
  server if it is still free.

## Reconciliation

New `ServiceReconciler` in `controllers/service_controller.go`, registered only when
`--enable-loadbalancer-controller` is true (default). It watches:

- `core/v1 Service`, predicate: `spec.type == LoadBalancer && spec.loadBalancerClass == frp.zufardhiyaulhaq.com/frp`
  (plus Services carrying our finalizer, so class/type changes are seen as deletions).
- `ServerPool`: enqueue every claimed Service.
- Generated `Upstream` and `Client` (label `frp.zufardhiyaulhaq.com/managed-by: loadbalancer`):
  enqueue the owning Service from the `service-uid` label; enqueue all Pending Services on
  Upstream deletion.

Per Service:

1. Not ours (class/type changed) or being deleted, with our finalizer present: delete all
   generated objects with label `frp.zufardhiyaulhaq.com/service-uid=<uid>`; in Shared mode
   delete the shared Client if it has no remaining Upstreams; remove finalizer; clear status.
2. Ensure finalizer `frp.zufardhiyaulhaq.com/loadbalancer`.
3. Resolve binding from the annotation; validate bound servers still exist; otherwise run
   selection. On failure: Event, clear ingress, requeue 30 s.
4. For each bound server ensure a `Client` in the operator namespace:
   - `PerService`: name `lb-<svc-ns>-<svc-name>-<server>`, `clientID` `lb/<svc-ns>/<svc-name>/<server>`.
   - `Shared`: name `pool-<pool>-<server>`, `clientID` `pool/<pool>/<server>`; created on first use.
   - Spec: `ServerPool.spec.servers[i]` fields mapped onto `Client.spec.server`
     (`transportProtocol` → `protocol`), `clientTemplate.podTemplate` → `spec.podTemplate`.
     Updated in place when the pool entry changes (hot reload / image roll handled by the
     existing ClientReconciler).
5. For each bound server × Service port ensure an `Upstream`
   `lb-<svc-ns>-<svc-name>-<server>-<port>` (TCP or UDP), labelled with pool, server,
   port, protocol, `service-uid`, `managed-by`. Delete Upstreams for ports no longer on the
   Service.
6. Race check: after creating, re-list Upstreams for the same server/port. If another
   Service's Upstream exists, the Service with the older `creationTimestamp` (then the
   lexically smaller `namespace/name`) keeps it; the other deletes its own and goes Pending.
7. Write Service status: `status.loadBalancer.ingress[]` one entry per bound server, only
   when that server's generated Client is `Ready` and the proxy status for each port is
   `running` (the ServiceReconciler calls the existing `handler.Status` against the generated
   Client's admin service, the same call ClientReconciler uses for metrics).
   Otherwise ingress is empty and an Event carries the Client's condition message
   (`ClientNotReady`, `ProxyStartError: 443 on prod/sg-01: <frpc error>`).
8. Update `ServerPool.status.servers[].allocatedPorts` and `Ready` condition.

Generated object names longer than 63 characters are truncated and suffixed with an
8-character hash of the full name so they remain valid label values.

The operator namespace comes from `POD_NAMESPACE` (downward API in the chart) and can be
overridden with `--operator-namespace` for `make run`.

`ClientReconciler` is not modified. Hand-written `Upstream`s on hand-written `Client`s
pointing at the same frps are invisible to allocation; a collision surfaces as a
`start error` on the generated proxy, which keeps the Service Pending with a
`ProxyStartError` Event.

## Package layout

- `api/v1alpha1/serverpool_types.go` — CRD types (`ServerPool`, `ServerPoolSpec`,
  `ServerPoolServer`, `ServerPoolStatus`).
- `pkg/loadbalancer/allocator.go` — pure selection logic: `Allocate(req Request, pools []ServerPool, allocations []Allocation) (Binding, *Reason)`;
  `Reason` carries the Event reason and message. No Kubernetes client.
- `pkg/loadbalancer/naming.go` — generated names, labels, annotation parsing/formatting.
- `pkg/loadbalancer/builder.go` — `ServerPoolServer` + Service port → `Client` / `Upstream` objects.
- `controllers/service_controller.go` — watches, finalizer, apply/delete, status, Events.
- `pkg/metrics` — `frp_serverpool_allocated_ports{pool,server}` and
  `frp_loadbalancer_service{namespace,service,pool,server,state}` (`state`: `bound` | `pending`).

## Chart, RBAC, docs

- CRD `serverpools.frp.zufardhiyaulhaq.com` generated into `config/crd/bases` and synced into
  `charts/frp-operator/crds/crds.yaml`.
- ClusterRole additions: `services` get/list/watch/update/patch; `services/status`
  update/patch; `events` create/patch; `serverpools`, `serverpools/status` full.
- Deployment: `POD_NAMESPACE` downward-API env; `--enable-loadbalancer-controller`
  driven by `loadBalancer.enabled` (default `true`).
- Dashboard: one row for pool utilisation and bound/pending Services.
- `examples/loadbalancer/`: pool with two servers, pinned Service, auto Service,
  multi-server Service.
- README (`README.md.gotmpl`) section "LoadBalancer Services": annotations table, pool
  manifest, semantics (one server per Service, Pending rules, stickiness, first-come
  first-served). `AGENTS.md`: new CRD, controller, package. `docs/releases/v0.11.0.md`;
  chart `1.9.0`, appVersion `0.11.0`.

## Testing

- `pkg/loadbalancer` table tests, no Kubernetes: every selection rule (Auto/Explicit,
  name-order determinism, `server` with and without `server-pool`, ambiguity, all-or-nothing
  multi-server, `allowedPorts` pool default vs override, SCTP rejection, race tie-break);
  naming truncation/hash; annotation round-trip.
- `controllers/service_controller_test.go` (fake client, same style as
  `client_controller_test.go`): ignore Services without the class; generated Client and
  Upstream shape in PerService and Shared modes; finalizer cleanup incl. shared Client
  last-user deletion; class change treated as deletion; server removed → reallocation
  Event; ingress gated on Client Ready; pool deleted → Pending, binding kept.
- `helm lint`, `helm template` assertions for RBAC, env and flag. `go build/vet/test`,
  gofmt. No `make lint` (golangci-lint OOMs on this machine).
- Manual on home-lab-kubernetes-01 against the existing frps: pool with one server, pinned
  Service, `kubectl get svc` shows the IP, curl succeeds; create a second Service on the same
  port → Pending; delete the first → second binds; recreate the first → Pending.

## Out of scope

- Gateway API.
- HTTP/HTTPS vhost proxies from Services (TCP/UDP only).
- Priorities, eviction, per-server opt-out flag (use a separate `Explicit` pool).
- `externalTrafficPolicy` / client source IP (proxy protocol could follow).
- IPv6 / dual-stack ingress entries.
- Random or least-used server selection.
{% endraw %}
