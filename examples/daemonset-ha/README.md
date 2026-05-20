# DaemonSet HA Upstream

This example demonstrates `Client.spec.workload.kind: DaemonSet`, which runs
one `frpc` per matching node. Combined with `Upstream.spec.tcp.loadBalancer.group`,
`frps` round-robins incoming traffic across every registered DaemonSet pod.

## Why DaemonSet

The default `Client.spec.workload.kind: Pod` creates a single bare Pod. That
works for development and single-tunnel setups but has two drawbacks for
production:

1. Only one frpc client means a single point of failure on the tunnel side.
2. A bare Pod is not rescheduled by kubelet when its node is lost
   (`NodeLost` state is not `NotFound`, so the operator's recreate branch
   never fires either).

Switching to `workload.kind: DaemonSet` runs one frpc per matching worker
node. Pair this with `loadBalancer.group` + `groupKey` on every Upstream and
frps will spread connections across the live group members. Losing a node
drops one tunnel pod; the others keep serving.

## Architecture

```
Internet
   │
   ▼
FRP Server (port 18080)
   │  loadBalancer.group=webhello-tcp
   ├──────────────► frpc on worker-1 (DaemonSet pod)
   ├──────────────► frpc on worker-2 (DaemonSet pod)
   └──────────────► frpc on worker-3 (DaemonSet pod)
                              │
                              ▼
                  webhello Service (ClusterIP)
```

## How config rollouts work for DaemonSet

The bare-Pod path uses the frpc admin API to hot-reload `config.toml` without
a pod restart. The DaemonSet path covers N pods reachable only through the
operator-managed headless admin Service, so the operator instead stamps a
fingerprint of the rendered ConfigMap onto the pod template
(`frp.zufardhiyaulhaq.com/config-hash`). A ConfigMap change bumps the
annotation which triggers a normal DaemonSet rolling update.

## Apply

```bash
# 1. Sample workload to expose
kubectl apply -f examples/daemonset-ha/deployment/

# 2. Edit examples/daemonset-ha/client/client.yaml — set spec.server.host
#    to your frps EIP / hostname.

# 3. Edit examples/daemonset-ha/client/secret.yaml — set token to match
#    frps `auth.token`. The same value is reused as loadBalancer.groupKey
#    on the Upstream.

kubectl apply -f examples/daemonset-ha/client/
```

## Verify

```bash
# DaemonSet rolled out (one frpc per worker node)
kubectl get ds edge-frpc

# Each pod registered the proxy and joined the load-balancer group
kubectl logs -l app.kubernetes.io/name=edge --tail=20 | grep -E "proxy added|start proxy"

# Client status reports running with pod count
kubectl get client edge -o jsonpath='{.status.phase}: {.status.message}{"\n"}'

# Trigger a config rollout and watch the DaemonSet roll
kubectl patch upstream webhello --type=json \
  -p='[{"op":"replace","path":"/spec/tcp/server/port","value":18081}]'
kubectl rollout status ds/edge-frpc
```

## Files

| File | Description |
|------|-------------|
| `deployment/service.yaml` | Sample backend (HTTP echo) + Service |
| `client/secret.yaml` | FRP server token (also used as groupKey) |
| `client/client.yaml` | Client with `workload.kind: DaemonSet` |
| `client/upstream.yaml` | TCP Upstream with `loadBalancer.group` |
