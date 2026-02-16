# Simple TCP Upstream

This example demonstrates a basic TCP upstream that exposes a Kubernetes service to the internet via FRP.

## Architecture

```
Internet -> FRP Server (port 8080) -> FRP Client (K8s) -> Nginx Service (K8s)
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
```

Run the FRP server:
```bash
./frps -c frps.toml
```

## Kubernetes Setup

### 1. Create the nginx deployment and service

```bash
kubectl apply -f examples/simple/deployment/
```

### 2. Create the FRP client secret

Update the secret with your FRP server token:
```bash
# Encode your token
echo -n "your-secret-token" | base64

# Update examples/simple/client/secret.yaml with the encoded token
kubectl apply -f examples/simple/client/secret.yaml
```

### 3. Create the FRP client and upstream

Update `examples/simple/client/client.yaml` with your FRP server address:
```yaml
spec:
  server:
    host: <your-frps-server-ip>
    port: 7000
```

Apply the configuration:
```bash
kubectl apply -f examples/simple/client/
```

### 4. Verify the setup

Check the FRP client pod:
```bash
kubectl get pods
kubectl logs -f <client-pod-name>
```

You should see:
```
[I] [proxy/tcp.go:xxx] [nginx] start proxy success
```

### 5. Access the service

From the internet, access your nginx service:
```bash
curl http://<frps-server-ip>:8080
```

## Features Demonstrated

- **TCP Upstream**: Exposes port 80 of nginx as port 8080 on FRP server
- **Proxy Protocol v2**: Preserves client IP information (requires nginx to be configured for proxy protocol)

## Files

| File | Description |
|------|-------------|
| `deployment/service.yaml` | Nginx deployment and service |
| `client/secret.yaml` | FRP server authentication token |
| `client/client.yaml` | FRP client configuration |
| `client/upstream.yaml` | TCP upstream definition |
