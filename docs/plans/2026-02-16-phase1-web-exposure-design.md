# Phase 1: Web Service Exposure Design

**Date:** 2026-02-16
**Compatibility:** Strict backwards compatibility (v1alpha1 additive changes only)

## Use Case

Expose internal web services to the internet with custom domains, URL routing, and TLS.

## Current State

The frp-operator currently supports TCP, UDP, STCP, and XTCP upstreams. Web service exposure via HTTP/HTTPS proxies is not yet supported, requiring users to use raw TCP proxies without FRP's HTTP-specific features like subdomain routing, URL path routing, or header manipulation.

## Proposed Changes

### New Upstream Types

Add `http` and `https` fields to `UpstreamSpec`:

```go
type UpstreamSpec struct {
    // ... existing fields ...

    // +optional
    HTTP *UpstreamSpec_HTTP `json:"http"`
    // +optional
    HTTPS *UpstreamSpec_HTTPS `json:"https"`
}
```

### HTTP Upstream

```go
type UpstreamSpec_HTTP struct {
    Host string `json:"host"`
    Port int    `json:"port"`

    // Domain routing (one of subdomain or customDomains required)
    // +optional
    Subdomain string `json:"subdomain,omitempty"`
    // +optional
    CustomDomains []string `json:"customDomains,omitempty"`

    // URL path routing
    // +optional
    Locations []string `json:"locations,omitempty"`

    // Header manipulation
    // +optional
    HostHeaderRewrite string `json:"hostHeaderRewrite,omitempty"`
    // +optional
    RequestHeaders *HTTPHeaders `json:"requestHeaders,omitempty"`
    // +optional
    ResponseHeaders *HTTPHeaders `json:"responseHeaders,omitempty"`

    // Basic authentication
    // +optional
    HTTPUser *SecretRef `json:"httpUser,omitempty"`
    // +optional
    HTTPPassword *SecretRef `json:"httpPassword,omitempty"`

    // Health check (HTTP-specific)
    // +optional
    HealthCheck *UpstreamSpec_HTTP_HealthCheck `json:"healthCheck,omitempty"`

    // Reuse existing transport options
    // +optional
    Transport *UpstreamSpec_TCP_Transport `json:"transport,omitempty"`
}

type HTTPHeaders struct {
    Set map[string]string `json:"set,omitempty"`
}

type UpstreamSpec_HTTP_HealthCheck struct {
    // +kubebuilder:validation:Enum=http
    Type string `json:"type"`
    Path string `json:"path"`
    TimeoutSeconds  int `json:"timeoutSeconds"`
    IntervalSeconds int `json:"intervalSeconds"`
    MaxFailed       int `json:"maxFailed"`
}

type SecretRef struct {
    Secret Secret `json:"secret"`
}
```

### HTTPS Upstream

```go
type UpstreamSpec_HTTPS struct {
    Host string `json:"host"`
    Port int    `json:"port"`

    // CustomDomains required for HTTPS (no subdomain support per FRP design)
    CustomDomains []string `json:"customDomains"`

    // +kubebuilder:validation:Enum=v1;v2
    // +optional
    ProxyProtocol *string `json:"proxyProtocol,omitempty"`

    // +optional
    Transport *UpstreamSpec_TCP_Transport `json:"transport,omitempty"`
}
```

## Example Usage

### HTTP Upstream with Subdomain

```yaml
apiVersion: frp.zufardhiyaulhaq.com/v1alpha1
kind: Upstream
metadata:
  name: my-webapp
spec:
  client: my-client
  http:
    host: my-service.default.svc
    port: 8080
    subdomain: "webapp"
    transport:
      useEncryption: true
```

### HTTP Upstream with Custom Domain and URL Routing

```yaml
apiVersion: frp.zufardhiyaulhaq.com/v1alpha1
kind: Upstream
metadata:
  name: api-gateway
spec:
  client: my-client
  http:
    host: api-service.default.svc
    port: 8080
    customDomains:
      - "api.example.com"
    locations:
      - "/v1"
      - "/v2"
    hostHeaderRewrite: "internal-api.local"
    requestHeaders:
      set:
        X-Forwarded-By: "frp-operator"
    responseHeaders:
      set:
        X-Frame-Options: "DENY"
```

### HTTP Upstream with Basic Auth and Health Check

```yaml
apiVersion: frp.zufardhiyaulhaq.com/v1alpha1
kind: Upstream
metadata:
  name: protected-app
spec:
  client: my-client
  http:
    host: internal-app.default.svc
    port: 8080
    subdomain: "admin"
    httpUser:
      secret:
        name: http-auth
        key: username
    httpPassword:
      secret:
        name: http-auth
        key: password
    healthCheck:
      type: http
      path: /health
      timeoutSeconds: 3
      intervalSeconds: 10
      maxFailed: 3
```

### HTTPS Upstream

```yaml
apiVersion: frp.zufardhiyaulhaq.com/v1alpha1
kind: Upstream
metadata:
  name: secure-app
spec:
  client: my-client
  https:
    host: app-service.default.svc
    port: 443
    customDomains:
      - "secure.example.com"
    proxyProtocol: "v2"
```

## Files to Modify

### API Types
- `api/v1alpha1/upstream_types.go` - Add HTTP, HTTPS types
- `api/v1alpha1/types.go` - Add SecretRef if not already present

### Business Logic
- `pkg/client/utils/template.go` - Add TOML templates for HTTP/HTTPS proxies
- `pkg/client/models/config.go` - Add HTTP/HTTPS config transformation
- `pkg/client/builder/configuration_builder.go` - Build config for HTTP/HTTPS

### Tests
- `pkg/client/builder/configuration_builder_test.go` - Unit tests for HTTP/HTTPS config
- `pkg/client/models/config_test.go` - Unit tests for HTTP/HTTPS model transformation

### Examples
- `examples/http/` - HTTP upstream examples
- `examples/https/` - HTTPS upstream examples

## TOML Template Addition

```toml
{{ if eq $upstream.Type 5 }}
name = "{{ $upstream.Name }}"
type = "http"
localIP = "{{ $upstream.HTTP.Host }}"
localPort = {{ $upstream.HTTP.Port }}

{{ if $upstream.HTTP.Subdomain }}
subdomain = "{{ $upstream.HTTP.Subdomain }}"
{{ end }}

{{ if $upstream.HTTP.CustomDomains }}
customDomains = [{{ range $i, $d := $upstream.HTTP.CustomDomains }}{{ if $i }}, {{ end }}"{{ $d }}"{{ end }}]
{{ end }}

{{ if $upstream.HTTP.Locations }}
locations = [{{ range $i, $l := $upstream.HTTP.Locations }}{{ if $i }}, {{ end }}"{{ $l }}"{{ end }}]
{{ end }}

{{ if $upstream.HTTP.HostHeaderRewrite }}
hostHeaderRewrite = "{{ $upstream.HTTP.HostHeaderRewrite }}"
{{ end }}

{{ if $upstream.HTTP.RequestHeaders }}
{{ range $k, $v := $upstream.HTTP.RequestHeaders.Set }}
requestHeaders.set.{{ $k }} = "{{ $v }}"
{{ end }}
{{ end }}

{{ if $upstream.HTTP.HTTPUser }}
httpUser = "{{ $upstream.HTTP.HTTPUser }}"
{{ end }}
{{ if $upstream.HTTP.HTTPPassword }}
httpPassword = "{{ $upstream.HTTP.HTTPPassword }}"
{{ end }}

{{ if $upstream.HTTP.HealthCheck }}
healthCheck.type = "http"
healthCheck.path = "{{ $upstream.HTTP.HealthCheck.Path }}"
healthCheck.timeoutSeconds = {{ $upstream.HTTP.HealthCheck.TimeoutSeconds }}
healthCheck.maxFailed = {{ $upstream.HTTP.HealthCheck.MaxFailed }}
healthCheck.intervalSeconds = {{ $upstream.HTTP.HealthCheck.IntervalSeconds }}
{{ end }}

{{ if $upstream.HTTP.Transport }}
transport.useEncryption = {{ $upstream.HTTP.Transport.UseEncryption }}
transport.useCompression = {{ $upstream.HTTP.Transport.UseCompression }}
{{ end }}
{{ end }}
```

## Validation Rules

1. For HTTP: Either `subdomain` OR `customDomains` must be specified (not both empty)
2. For HTTPS: `customDomains` is required
3. `httpUser` and `httpPassword` must both be specified or both omitted
4. Health check `path` must start with `/`

## Testing Strategy

1. Unit tests for configuration builder with all HTTP/HTTPS field combinations
2. Unit tests for model transformation including secret fetching
3. Integration test deploying HTTP upstream and verifying TOML output
4. Example manifests that can be applied against a real FRP server
