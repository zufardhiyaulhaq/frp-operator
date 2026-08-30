# LoadBalancer Service

This example exposes a Kubernetes `Service` of `type: LoadBalancer` through a pool of `frps`
servers, without writing `Client`/`Upstream` resources by hand. Set
`loadBalancerClass: frp.zufardhiyaulhaq.com/frp` on the Service and the operator picks a server
from a `ServerPool`, generates the `Client`/`Upstream` resources for you, and writes the server's
public address to `status.loadBalancer.ingress`.

## Files

| File | Description |
|------|-------------|
| `serverpool.yaml` | A `ServerPool` named `prod` with two `frps` servers (`sg-01`, `sg-02`) and the token `Secret` it references |
| `service-auto.yaml` | `web-auto` — no annotations; the operator auto-selects a pool and a server |
| `service-pinned.yaml` | `web-pinned` — pinned to pool `prod`, server `sg-01` |
| `service-multi.yaml` | `web-multi` — pinned to two servers (`sg-01,sg-02`); gets one ingress IP per server |

## Prerequisites

1. The operator running with the LoadBalancer controller enabled (`loadBalancer.enabled: true` in
   the Helm values, the default). This example assumes the operator runs in a namespace named
   `frp-operator` — edit `namespace: frp-operator` in the manifests below to match your actual
   release/`POD_NAMESPACE` namespace if it differs.
2. One or more `frps` servers reachable from the cluster (see the repo's top-level
   [Prerequisite](../../README.md#prerequisite) section for how to stand one up).
3. A workload behind a Service selector `app: web` listening on `targetPort: 8443` (any Deployment
   will do — the LoadBalancer feature only cares about the Service in front of it).

## Apply

```bash
# Update host/port/publicAddress/token in serverpool.yaml for your frps server(s) first.
kubectl apply -f examples/loadbalancer/serverpool.yaml
kubectl apply -f examples/loadbalancer/service-auto.yaml
kubectl apply -f examples/loadbalancer/service-pinned.yaml
kubectl apply -f examples/loadbalancer/service-multi.yaml
```

## Check the result

```console
$ kubectl get svc web-auto web-pinned web-multi
NAME         TYPE           CLUSTER-IP   EXTERNAL-IP   PORT(S)          AGE
web-auto     LoadBalancer   10.0.12.34   <pending>     443:31234/TCP    5s
web-pinned   LoadBalancer   10.0.12.35   <pending>     8080:31235/TCP   5s
web-multi    LoadBalancer   10.0.12.36   <pending>     443:31236/TCP    5s
```

`EXTERNAL-IP` stays `<pending>` until:
1. the operator has bound the Service to a server (an Upstream/Client pair exists for it), **and**
2. the generated `Client`'s `Ready` condition is `True`, **and**
3. every proxy for that Service reports `running` on the frpc admin API.

Once all three are satisfied, `EXTERNAL-IP` becomes the server's `publicAddress` (an IP or
hostname). `web-multi` gets one ingress entry per pinned server, so it may show two addresses.

```console
$ kubectl get svc web-auto
NAME       TYPE           CLUSTER-IP   EXTERNAL-IP      PORT(S)         AGE
web-auto   LoadBalancer   10.0.12.34   178.128.100.87   443:31234/TCP   2m
```

## Reading Events

If a Service stays `<pending>`, `kubectl describe svc` explains why:

```bash
kubectl describe svc web-auto
```

```
Events:
  Type     Reason              Age   From               Message
  ----     ------              ----  ----               -------
  Warning  NoServerAvailable   30s   service-controller  no server in pools [prod] can provide ports [443/TCP]
```

Reasons you may see: `PortUnavailable` (another Service already holds that port on the specific
server the Service is pinned or bound to), `PortNotAllowed` (port outside `allowedPorts`),
`NoServerAvailable` (no server in any candidate pool can provide the requested ports),
`ServerNotFound` / `AmbiguousServer` (the `server` annotation names
servers that don't exist together in exactly one pool), `PoolNotFound` (the `server-pool`
annotation names a pool that doesn't exist), `UnsupportedProtocol` (a port isn't TCP or UDP),
`ClientNotReady` (the generated `Client` hasn't come up yet), `ProxyStartError` (frpc rejected the
proxy), or `PoolDeleted` (the bound pool/server disappeared; the generated objects were removed).
`Normal` event `Bound` records a (re)selection; `Warning` event `ServerReallocated` is emitted when
a bound Service had to move to a different server.

## Inspecting allocations

`ServerPool.status` is a live projection of which ports are taken on each server:

```bash
kubectl get serverpool -n frp-operator prod -o yaml
```

```yaml
status:
  servers:
  - name: sg-01
    allocatedPorts:
    - port: 8080
      protocol: TCP
      service: default/web-pinned
  - name: sg-02
    allocatedPorts:
    - port: 443
      protocol: TCP
      service: default/web-multi
```

The generated `Client`/`Upstream` objects (in the operator's namespace, named `lb-<namespace>-
<service>-<server>...`) are the source of truth; `ServerPool.status` and the
`frp_serverpool_allocated_ports` metric are both derived from them.

## Notes

- Editing the `frp.zufardhiyaulhaq.com/server-pool` or `frp.zufardhiyaulhaq.com/server` annotation
  on an already-bound Service is honoured: the operator re-selects, which can move a live tunnel
  to a different server/IP.
- A pinned multi-server Service (`web-multi` here) is all-or-nothing — if `sg-01` or `sg-02` is
  removed from the pool, every tunnel for that Service is torn down until the full pinned set can
  be satisfied again.
- Bindings are otherwise sticky: a bound Service keeps its server even if a "better" one becomes
  available later, and first-come-first-served applies when two Services race for the same port.

See the chart README's [LoadBalancer Services](../../README.md#loadbalancer-services) section for
the full annotation reference and selection rules.
