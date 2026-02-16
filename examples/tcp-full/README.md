# TCP Upstream with Full Features

This example demonstrates a TCP upstream with all available features: health checks, transport encryption/compression, and bandwidth limiting.

## Architecture

```
Internet -> FRP Server (port 8080) -> FRP Client (K8s) -> Nginx Service (K8s)
                                          |
                                    [Encrypted]
                                    [Compressed]
                                    [Bandwidth Limited]
```

## FRP Server Configuration

Configure your FRP server (frps) with the following settings:

```toml
# frps.toml
bindAddr = "0.0.0.0"
bindPort = 7000

# Authentication
auth.method = "token"
auth.token = "your-secret-token"

# Optional: Enable dashboard
webServer.addr = "0.0.0.0"
webServer.port = 7500
webServer.user = "admin"
webServer.password = "admin"

# Optional: Logging
log.to = "./frps.log"
log.level = "info"
log.maxDays = 3
```

Run the FRP server:
```bash
./frps -c frps.toml
```

## Kubernetes Setup

### 1. Create the nginx deployment and service

```bash
kubectl apply -f examples/tcp-full/deployment/
```

### 2. Create the FRP client secret

```bash
# Encode your token
echo -n "your-secret-token" | base64

# Update examples/tcp-full/client/secret.yaml with the encoded token
kubectl apply -f examples/tcp-full/client/secret.yaml
```

### 3. Create the FRP client and upstream

Update `examples/tcp-full/client/client.yaml` with your FRP server address:
```yaml
spec:
  server:
    host: <your-frps-server-ip>
    port: 7000
```

Apply the configuration:
```bash
kubectl apply -f examples/tcp-full/client/
```

### 4. Verify the setup

```bash
kubectl get pods
kubectl logs -f <client-pod-name>
```

## Features Demonstrated

### Health Check
```yaml
healthCheck:
  timeoutSeconds: 5    # Health check timeout
  maxFailed: 3         # Max failures before marking unhealthy
  intervalSeconds: 10  # Check interval
```

The FRP client will perform TCP health checks against the local service. If the service becomes unhealthy, FRP will stop routing traffic to it.

### Transport Encryption & Compression
```yaml
transport:
  useEncryption: true   # Encrypt traffic between frpc and frps
  useCompression: true  # Compress traffic to reduce bandwidth
```

- **Encryption**: Adds security for traffic between client and server
- **Compression**: Reduces bandwidth usage (useful for text-heavy traffic)

### Bandwidth Limiting
```yaml
transport:
  bandwidthLimit:
    enabled: true
    limit: 1024
    type: MB    # KB or MB
```

Limits the bandwidth to 1024 MB/s for this proxy. Useful for:
- Preventing a single service from consuming all bandwidth
- Cost control on metered connections
- QoS management

### Proxy Protocol v2
```yaml
proxyProtocol: v2
```

Preserves the original client IP. The backend service must support proxy protocol to use this feature.

## Files

| File | Description |
|------|-------------|
| `deployment/service.yaml` | Nginx deployment and service |
| `client/secret.yaml` | FRP server authentication token |
| `client/client.yaml` | FRP client configuration |
| `client/upstream.yaml` | TCP upstream with full features |
