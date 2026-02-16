# HTTP/HTTPS Upstream

This example demonstrates HTTP and HTTPS upstreams with features like subdomain routing, custom domains, path-based routing, header manipulation, and health checks.

## Architecture

### HTTP Upstream
```
Browser -> FRP Server (port 80) -> FRP Client (K8s) -> Nginx Service (K8s)
              |
        [Layer 7 Routing]
        - Subdomain: nginx.tunnel.example.com
        - Custom domains: nginx.example.com
        - Paths: /api, /web
        - Header manipulation
```

### HTTPS Upstream
```
Browser -> FRP Server (port 443) -> FRP Client (K8s) -> Nginx Service (K8s)
              |
        [SNI Routing Only]
        - TLS passthrough
        - Routes by hostname (SNI)
        - Cannot inspect/modify HTTP content
```

## FRP Server Configuration

Configure your FRP server (frps) with HTTP/HTTPS virtual hosting:

```toml
# frps.toml
bindAddr = "0.0.0.0"
bindPort = 7000

# Authentication
auth.method = "token"
auth.token = "your-secret-token"

# HTTP Virtual Host - enables subdomain and custom domain routing
vhostHTTPPort = 80

# HTTPS Virtual Host - enables SNI-based routing
vhostHTTPSPort = 443

# Subdomain configuration
# Proxies with subdomain = "nginx" will be accessible at nginx.tunnel.example.com
subDomainHost = "tunnel.example.com"

# HTTP timeout
vhostHTTPTimeout = 60

# Optional: Custom 404 page
# custom404Page = "/path/to/404.html"

# Optional: Dashboard
webServer.addr = "0.0.0.0"
webServer.port = 7500
webServer.user = "admin"
webServer.password = "admin"
```

Run the FRP server:
```bash
./frps -c frps.toml
```

## DNS Configuration

For subdomain and custom domain routing to work, configure DNS:

```
# For subdomain routing (*.tunnel.example.com)
*.tunnel.example.com    A     <frps-server-ip>

# For custom domains
nginx.example.com       A     <frps-server-ip>
secure.example.com      A     <frps-server-ip>
ssl.example.com         A     <frps-server-ip>
```

## Kubernetes Setup

### 1. Create the nginx deployment and service

```bash
kubectl apply -f examples/http/deployment/
```

### 2. Create the FRP client secret

```bash
# Encode your token
echo -n "your-secret-token" | base64

# Update examples/http/client/secret.yaml with the encoded token
kubectl apply -f examples/http/client/secret.yaml
```

### 3. Update FRP client configuration

Edit `examples/http/client/client.yaml`:
```yaml
spec:
  server:
    host: <your-frps-server-ip>
    port: 7000
```

### 4. Apply all configurations

```bash
kubectl apply -f examples/http/client/
```

### 5. Verify the setup

```bash
kubectl get pods
kubectl logs -f <client-pod-name>
```

You should see:
```
[I] [proxy/http.go:xxx] [nginx-http] start proxy success
[I] [proxy/https.go:xxx] [nginx-https] start proxy success
```

## Accessing the Services

### HTTP Upstream

```bash
# Via subdomain
curl http://nginx.tunnel.example.com/api

# Via custom domain
curl http://nginx.example.com/web

# With host header (for testing)
curl -H "Host: nginx.example.com" http://<frps-server-ip>/api
```

### HTTPS Upstream

```bash
# Via custom domain (TLS passthrough)
curl https://secure.example.com

# Note: The backend must serve valid TLS certificates
```

## Features Demonstrated

### HTTP Upstream Features

| Feature | Description | Example |
|---------|-------------|---------|
| `subdomain` | Route by subdomain | `nginx.tunnel.example.com` |
| `customDomains` | Route by custom domains | `nginx.example.com` |
| `locations` | Path-based routing | `/api`, `/web` |
| `hostHeaderRewrite` | Rewrite Host header to backend | `nginx.internal` |
| `requestHeaders` | Add/modify request headers | `X-Forwarded-For`, `X-Real-IP` |
| `responseHeaders` | Add/modify response headers | `X-Served-By` |
| `healthCheck` | HTTP health checks | `GET /health` |
| `transport` | Encryption & compression | Between frpc and frps |

### HTTPS Upstream Features

| Feature | Description |
|---------|-------------|
| `customDomains` | SNI-based routing (hostname only) |
| `proxyProtocol` | Preserve client IP (v1 or v2) |
| `transport` | Encryption & compression |

**Note**: HTTPS is TLS passthrough. FRP cannot inspect encrypted traffic, so path-based routing, header manipulation, and HTTP-level health checks are NOT available for HTTPS.

## HTTP vs HTTPS Comparison

| Feature | HTTP | HTTPS |
|---------|------|-------|
| Subdomain routing | Yes | No (use customDomains) |
| Custom domains | Yes | Yes (SNI-based) |
| Path routing (`locations`) | Yes | No |
| Header manipulation | Yes | No |
| HTTP basic auth | Yes | No |
| HTTP health checks | Yes | No |
| TLS termination | At FRP server | Passthrough to backend |

## Files

| File | Description |
|------|-------------|
| `deployment/service.yaml` | Nginx deployment and service |
| `client/secret.yaml` | FRP server authentication token |
| `client/client.yaml` | FRP client configuration |
| `client/upstream-http.yaml` | HTTP upstream with full features |
| `client/upstream-https.yaml` | HTTPS upstream (SNI routing) |
