# Phase 4: Advanced Features Design

**Date:** 2026-02-16
**Compatibility:** Strict backwards compatibility (v1alpha1 additive changes only)

## Use Case

Load balancing, plugins, and additional proxy types for complex deployments.

## Current State

- No load balancing support
- No FRP plugin support
- No TCPMUX proxy type
- Limited client transport configuration

## Proposed Changes

### 4A: Load Balancing

Distribute traffic across multiple backend services using FRP's group/groupKey mechanism.

```go
type LoadBalancer struct {
    // Group is the load balancer group name
    Group string `json:"group"`
    // +optional
    // GroupKey is the shared secret for the group (prevents unauthorized joining)
    GroupKey *SecretRef `json:"groupKey,omitempty"`
}

type UpstreamSpec_TCP struct {
    // ... existing fields ...

    // +optional
    // LoadBalancer enables load balancing across multiple upstreams with the same group
    LoadBalancer *LoadBalancer `json:"loadBalancer,omitempty"`
}

// Also add to UDP and HTTP types
```

### 4B: Plugin Support

Use FRP plugins instead of direct service forwarding for special protocols.

```go
type UpstreamSpec_TCP struct {
    // Host/Port are optional when Plugin is used
    // +optional
    Host string `json:"host,omitempty"`
    // +optional
    Port int `json:"port,omitempty"`

    Server UpstreamSpec_TCP_Server `json:"server"`

    // +optional
    // Plugin configures an FRP plugin instead of direct forwarding
    Plugin *UpstreamPlugin `json:"plugin,omitempty"`

    // ... rest of existing fields ...
}

type UpstreamPlugin struct {
    // +kubebuilder:validation:Enum=socks5;http_proxy;static_file;https2http;https2https;http2http;http2https;unix_domain_socket;tls2raw
    Type string `json:"type"`

    // --- socks5, http_proxy ---
    // +optional
    Username *SecretRef `json:"username,omitempty"`
    // +optional
    Password *SecretRef `json:"password,omitempty"`

    // --- static_file ---
    // +optional
    // LocalPath is the directory to serve files from
    LocalPath string `json:"localPath,omitempty"`
    // +optional
    // StripPrefix removes the prefix from the URL path
    StripPrefix string `json:"stripPrefix,omitempty"`
    // +optional
    // HTTPUser enables basic auth for static file server
    HTTPUser *SecretRef `json:"httpUser,omitempty"`
    // +optional
    // HTTPPassword for basic auth
    HTTPPassword *SecretRef `json:"httpPassword,omitempty"`

    // --- https2http, https2https, http2https ---
    // +optional
    // LocalAddr is the target address for protocol conversion
    LocalAddr string `json:"localAddr,omitempty"`
    // +optional
    // CrtPath is the certificate file path in the container
    CrtPath string `json:"crtPath,omitempty"`
    // +optional
    // KeyPath is the key file path in the container
    KeyPath string `json:"keyPath,omitempty"`
    // +optional
    // HostHeaderRewrite modifies the Host header
    HostHeaderRewrite string `json:"hostHeaderRewrite,omitempty"`

    // --- unix_domain_socket ---
    // +optional
    // UnixPath is the path to the Unix socket
    UnixPath string `json:"unixPath,omitempty"`
}
```

### 4C: TCPMUX Proxy

Multiplex multiple services over HTTP CONNECT protocol.

```go
type UpstreamSpec struct {
    // ... existing fields ...

    // +optional
    // TCPMUX exposes a service using TCP multiplexing over HTTP CONNECT
    TCPMUX *UpstreamSpec_TCPMUX `json:"tcpmux,omitempty"`
}

type UpstreamSpec_TCPMUX struct {
    Host string `json:"host"`
    Port int    `json:"port"`
    // +kubebuilder:validation:Enum=httpconnect
    Multiplexer string `json:"multiplexer"`
    // CustomDomains for routing
    CustomDomains []string `json:"customDomains"`
    // +optional
    Transport *UpstreamSpec_TCP_Transport `json:"transport,omitempty"`
}
```

### 4D: Client Transport Tuning

Fine-tune connection behavior for performance optimization.

```go
type ClientSpec_Server struct {
    // ... existing fields ...

    // +optional
    // Transport configures connection behavior
    Transport *ClientSpec_Server_Transport `json:"transport,omitempty"`
}

type ClientSpec_Server_Transport struct {
    // +optional
    // +kubebuilder:default=1
    // PoolCount is the number of pre-established connections to the server
    PoolCount int `json:"poolCount,omitempty"`

    // +optional
    // +kubebuilder:default=true
    // TCPMux enables TCP stream multiplexing to reduce connection overhead
    TCPMux bool `json:"tcpMux,omitempty"`

    // +optional
    // +kubebuilder:default="10s"
    // DialServerTimeout is the connection timeout to the FRP server
    DialServerTimeout string `json:"dialServerTimeout,omitempty"`

    // +optional
    // +kubebuilder:default="-1s"
    // DialServerKeepalive is the keepalive interval (-1s to disable)
    DialServerKeepalive string `json:"dialServerKeepalive,omitempty"`

    // +optional
    // ConnectServerLocalIP binds the outbound connection to a specific local IP
    ConnectServerLocalIP string `json:"connectServerLocalIP,omitempty"`
}
```

## Example Usage

### Load Balancing Across Multiple Backends

```yaml
# Backend 1
apiVersion: frp.zufardhiyaulhaq.com/v1alpha1
kind: Upstream
metadata:
  name: api-node-1
spec:
  client: my-client
  tcp:
    host: api-1.default.svc
    port: 8080
    server:
      port: 9000
    loadBalancer:
      group: "api-cluster"
      groupKey:
        secret:
          name: lb-secret
          key: groupKey
---
# Backend 2
apiVersion: frp.zufardhiyaulhaq.com/v1alpha1
kind: Upstream
metadata:
  name: api-node-2
spec:
  client: my-client
  tcp:
    host: api-2.default.svc
    port: 8080
    server:
      port: 9000  # Same remote port - FRP will load balance
    loadBalancer:
      group: "api-cluster"
      groupKey:
        secret:
          name: lb-secret
          key: groupKey
```

### SOCKS5 Proxy Plugin

```yaml
apiVersion: frp.zufardhiyaulhaq.com/v1alpha1
kind: Upstream
metadata:
  name: socks5-proxy
spec:
  client: my-client
  tcp:
    server:
      port: 1080
    plugin:
      type: socks5
      username:
        secret:
          name: socks-auth
          key: username
      password:
        secret:
          name: socks-auth
          key: password
```

### HTTP Proxy Plugin

```yaml
apiVersion: frp.zufardhiyaulhaq.com/v1alpha1
kind: Upstream
metadata:
  name: http-proxy
spec:
  client: my-client
  tcp:
    server:
      port: 8118
    plugin:
      type: http_proxy
      username:
        secret:
          name: proxy-auth
          key: username
      password:
        secret:
          name: proxy-auth
          key: password
```

### Static File Server Plugin

```yaml
apiVersion: frp.zufardhiyaulhaq.com/v1alpha1
kind: Upstream
metadata:
  name: file-server
spec:
  client: my-client
  http:
    subdomain: "files"
    plugin:
      type: static_file
      localPath: "/data/public"
      stripPrefix: "/download"
      httpUser:
        secret:
          name: file-auth
          key: username
      httpPassword:
        secret:
          name: file-auth
          key: password
```

### Unix Socket Forwarding (Docker Daemon)

```yaml
apiVersion: frp.zufardhiyaulhaq.com/v1alpha1
kind: Upstream
metadata:
  name: docker-api
spec:
  client: my-client
  tcp:
    server:
      port: 2375
    plugin:
      type: unix_domain_socket
      unixPath: "/var/run/docker.sock"
```

### TCPMUX Proxy

```yaml
apiVersion: frp.zufardhiyaulhaq.com/v1alpha1
kind: Upstream
metadata:
  name: mux-service
spec:
  client: my-client
  tcpmux:
    host: internal-service.default.svc
    port: 8080
    multiplexer: httpconnect
    customDomains:
      - "mux.example.com"
```

### Client Transport Tuning

```yaml
apiVersion: frp.zufardhiyaulhaq.com/v1alpha1
kind: Client
metadata:
  name: optimized-client
spec:
  server:
    host: frp.example.com
    port: 7000
    authentication:
      token:
        secret:
          name: frp-token
          key: token
    transport:
      poolCount: 5
      tcpMux: true
      dialServerTimeout: "15s"
      dialServerKeepalive: "30s"
```

## Files to Modify

### API Types
- `api/v1alpha1/client_types.go` - ClientSpec_Server_Transport
- `api/v1alpha1/upstream_types.go` - LoadBalancer, UpstreamPlugin, TCPMUX
- `api/v1alpha1/types.go` - SecretRef (if not already added)

### Business Logic
- `pkg/client/utils/template.go` - TOML templates for all new features
- `pkg/client/models/config.go` - Config transformation for plugins, LB, TCPMUX
- `pkg/client/builder/pod_builder.go` - Volume mounts for plugin files (static_file, unix socket)

### Tests
- Unit tests for all new configuration combinations
- Integration tests for plugin functionality

### Examples
- `examples/load-balancing/` - Load balancer examples
- `examples/plugins/` - Plugin examples (socks5, http_proxy, static_file)
- `examples/tcpmux/` - TCPMUX examples

## TOML Template Additions

### Load Balancer

```toml
{{ if $upstream.TCP.LoadBalancer }}
loadBalancer.group = "{{ $upstream.TCP.LoadBalancer.Group }}"
{{ if $upstream.TCP.LoadBalancer.GroupKey }}
loadBalancer.groupKey = "{{ $upstream.TCP.LoadBalancer.GroupKey }}"
{{ end }}
{{ end }}
```

### Plugin Configuration

```toml
{{ if $upstream.TCP.Plugin }}
plugin = "{{ $upstream.TCP.Plugin.Type }}"

{{ if eq $upstream.TCP.Plugin.Type "socks5" }}
{{ if $upstream.TCP.Plugin.Username }}
plugin.username = "{{ $upstream.TCP.Plugin.Username }}"
{{ end }}
{{ if $upstream.TCP.Plugin.Password }}
plugin.password = "{{ $upstream.TCP.Plugin.Password }}"
{{ end }}
{{ end }}

{{ if eq $upstream.TCP.Plugin.Type "http_proxy" }}
{{ if $upstream.TCP.Plugin.Username }}
plugin.httpUser = "{{ $upstream.TCP.Plugin.Username }}"
{{ end }}
{{ if $upstream.TCP.Plugin.Password }}
plugin.httpPassword = "{{ $upstream.TCP.Plugin.Password }}"
{{ end }}
{{ end }}

{{ if eq $upstream.TCP.Plugin.Type "static_file" }}
plugin.localPath = "{{ $upstream.TCP.Plugin.LocalPath }}"
{{ if $upstream.TCP.Plugin.StripPrefix }}
plugin.stripPrefix = "{{ $upstream.TCP.Plugin.StripPrefix }}"
{{ end }}
{{ if $upstream.TCP.Plugin.HTTPUser }}
plugin.httpUser = "{{ $upstream.TCP.Plugin.HTTPUser }}"
{{ end }}
{{ if $upstream.TCP.Plugin.HTTPPassword }}
plugin.httpPassword = "{{ $upstream.TCP.Plugin.HTTPPassword }}"
{{ end }}
{{ end }}

{{ if eq $upstream.TCP.Plugin.Type "unix_domain_socket" }}
plugin.unixPath = "{{ $upstream.TCP.Plugin.UnixPath }}"
{{ end }}
{{ end }}
```

### TCPMUX

```toml
{{ if eq $upstream.Type 6 }}
name = "{{ $upstream.Name }}"
type = "tcpmux"
multiplexer = "{{ $upstream.TCPMUX.Multiplexer }}"
localIP = "{{ $upstream.TCPMUX.Host }}"
localPort = {{ $upstream.TCPMUX.Port }}
customDomains = [{{ range $i, $d := $upstream.TCPMUX.CustomDomains }}{{ if $i }}, {{ end }}"{{ $d }}"{{ end }}]

{{ if $upstream.TCPMUX.Transport }}
transport.useEncryption = {{ $upstream.TCPMUX.Transport.UseEncryption }}
transport.useCompression = {{ $upstream.TCPMUX.Transport.UseCompression }}
{{ end }}
{{ end }}
```

### Client Transport

```toml
{{ if .Common.Transport }}
transport.poolCount = {{ .Common.Transport.PoolCount }}
transport.tcpMux = {{ .Common.Transport.TCPMux }}
{{ if .Common.Transport.DialServerTimeout }}
transport.dialServerTimeout = "{{ .Common.Transport.DialServerTimeout }}"
{{ end }}
{{ if .Common.Transport.DialServerKeepalive }}
transport.dialServerKeepalive = "{{ .Common.Transport.DialServerKeepalive }}"
{{ end }}
{{ if .Common.Transport.ConnectServerLocalIP }}
transport.connectServerLocalIP = "{{ .Common.Transport.ConnectServerLocalIP }}"
{{ end }}
{{ end }}
```

## Validation Rules

1. **Load Balancer:** All upstreams in a group must have the same remote port
2. **Plugin:** When plugin is specified, host/port are optional (mutually exclusive with plugin)
3. **TCPMUX:** CustomDomains is required
4. **Transport Timeouts:** Must be valid Go duration strings (e.g., "10s", "1m")

## Implementation Notes

### Plugin Volume Mounts

For `static_file` and `unix_domain_socket` plugins, the pod builder needs to:
1. Mount the specified paths into the container
2. Ensure appropriate permissions

For `static_file`:
```go
// Add volume mount for the local path
pod.Spec.Volumes = append(pod.Spec.Volumes, corev1.Volume{
    Name: "static-files",
    VolumeSource: corev1.VolumeSource{
        PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
            ClaimName: "files-pvc",
        },
    },
})
```

For `unix_domain_socket`:
```go
// Add hostPath volume for the socket
pod.Spec.Volumes = append(pod.Spec.Volumes, corev1.Volume{
    Name: "docker-socket",
    VolumeSource: corev1.VolumeSource{
        HostPath: &corev1.HostPathVolumeSource{
            Path: "/var/run/docker.sock",
            Type: &socketType,
        },
    },
})
```

### Load Balancer Group Key Handling

The group key should be fetched from the secret at config build time, similar to other secret handling.

## Testing Strategy

1. Unit tests for all new TOML template generations
2. Unit tests for plugin configuration validation
3. Integration tests for load balancing across multiple upstreams
4. Integration tests for TCPMUX functionality
5. Example manifests for all plugin types
