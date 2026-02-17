# Operations Examples

This directory contains examples for production deployment configurations of the FRP operator.

## Examples

### client-with-podtemplate.yaml

Demonstrates advanced pod customization options:

- **Resource limits**: CPU and memory constraints
- **Node scheduling**: nodeSelector for targeting specific nodes
- **Tolerations**: Schedule on nodes with taints
- **Custom labels/annotations**: For monitoring integration (e.g., Prometheus)
- **Service account**: Custom service account for RBAC
- **Priority class**: Pod scheduling priority
- **Admin server with pprof**: Enables pprof endpoints for debugging

## Prerequisites

Create required secrets before applying:

```bash
# FRP authentication token
kubectl create secret generic frp-token --from-literal=token=your-frp-token

# Admin server credentials
kubectl create secret generic admin-creds \
  --from-literal=username=admin \
  --from-literal=password=admin-password
```

## Applying

```bash
kubectl apply -f client-with-podtemplate.yaml
```

## Verifying Status

Check the client status:

```bash
kubectl get clients
```

Output shows phase, upstream count, and visitor count:

```
NAME                PHASE     UPSTREAMS   VISITORS   AGE
production-client   Running   2           1          5m
```

## frps Configuration

```toml
# frps.toml
bindAddr = "0.0.0.0"
bindPort = 7000

auth.method = "token"
auth.token = "your-frp-token"

# Dashboard (optional)
webServer.addr = "0.0.0.0"
webServer.port = 7500
webServer.user = "admin"
webServer.password = "admin"
```
