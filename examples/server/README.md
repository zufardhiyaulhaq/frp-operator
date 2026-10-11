# Server (frps)

> **Alpha.** `Server` is new in v0.13.0; its API and behaviour may still change.

This example runs **frps** in a public cluster with a `Server` resource, and connects a private
cluster to it with an ordinary `Client` and `Upstream`. The operator runs one frps pod per
`Server`, behind a `LoadBalancer` Service that exposes the bind port, the vhost ports and every
port in `allowPorts`.

## Files

| File | Cluster | Description |
|------|---------|-------------|
| `server.yaml` | public | A cert-manager `Certificate` for `frps.example.com` and the `Server` `public-01` |
| `connection-secret.yaml` | private | The copied `public-01-connection` Secret (`token`, `ca.crt`) |
| `client.yaml` | private | A `Client` pointed at the Server's address, verifying it through `tls.serverName` |
| `upstream.yaml` | private | A TCP `Upstream` on the Server's port 8000 |

## 1. Public cluster

```console
kubectl apply -f server.yaml
kubectl get server public-01            # wait for READY True
kubectl get server public-01 -o jsonpath='{.status}'
```

The status has everything the private side needs:

| Field | Use |
|-------|-----|
| `status.address` | The Client's `host` (load-balancer IP or hostname) |
| `status.tls.serverNames` | The Client's `tls.serverName` |
| `status.exposedPorts` | The ports Upstreams can claim; a ServerPool's `allowedPorts` |
| `status.connectionSecret` | The Secret to copy (`token`, and `ca.crt` when the certificate has one) |

Without `authentication.token`, the operator generates the token. Deleting `public-01-connection`
rotates it, and every connected cluster then needs the new copy.

## 2. Copy the connection Secret

```console
kubectl --context public get secret public-01-connection -o json \
  | jq '{apiVersion, kind, type, data, metadata: {name: .metadata.name}}' \
  | kubectl --context private apply -f -
```

Any secret-sync tool works as well.

## 3. Private cluster

Put the values from step 1 into `client.yaml`, then:

```console
kubectl apply -f client.yaml -f upstream.yaml
```

A ServerPool server uses the same fields (`host`, `port`, `authentication`, `tls`), with
`allowedPorts` set to `status.exposedPorts`.

## Notes

- frps cannot reload its configuration: any change to the Server or a Secret it uses restarts the
  frps pod, and connected Clients reconnect on their own. Set `restartPolicy: Manual` to choose
  when (the Server then reports `ConfigSynced=False`, reason `RestartPending`, until you delete
  the pod).
- There is one frps pod per Server. For availability, run several Servers and spread Services over
  them with a private-side ServerPool.
- A cloud that rejects mixed TCP/UDP load balancers shows `Ready=False`, reason
  `LoadBalancerFailed`.
- Don't enable PROXY protocol on the load balancer: frps cannot read it.
- `wss` clients need a TLS terminator in front of frps and `bind.tls.force: false`.
- The dashboard is only ever exposed through its own Service (`dashboard.service`), never an
  Ingress. Exposing it outside the cluster without `dashboard.tls` raises a `DashboardInsecure`
  warning.
