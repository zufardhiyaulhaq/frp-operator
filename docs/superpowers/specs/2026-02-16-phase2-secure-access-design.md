# Phase 2: Secure Access Control Design

**Date:** 2026-02-16
**Compatibility:** Strict backwards compatibility (v1alpha1 additive changes only)

## Use Case

Fine-grained access control for P2P tunnels, OIDC authentication for enterprise environments, and TLS security for encrypted connections.

## Current State

- **Authentication:** Only token-based authentication is supported
- **Access Control:** No `allowUsers` support for STCP/XTCP upstreams
- **TLS:** No client certificate configuration

## Proposed Changes

### 2A: allowUsers for STCP/XTCP

Add access control to specify which FRP users can connect to tunnels.

```go
type UpstreamSpec_STCP struct {
    // ... existing fields ...

    // +optional
    // AllowUsers specifies which FRP users can connect to this tunnel.
    // Use "*" to allow any user. Empty means only the same user.
    AllowUsers []string `json:"allowUsers,omitempty"`
}

type UpstreamSpec_XTCP struct {
    // ... existing fields ...

    // +optional
    AllowUsers []string `json:"allowUsers,omitempty"`
}
```

### 2B: OIDC Authentication

Add OIDC as alternative to token authentication for enterprise SSO integration.

```go
type ClientSpec_Server_Authentication struct {
    // +optional
    Token *ClientSpec_Server_Authentication_Token `json:"token,omitempty"`
    // +optional
    OIDC *ClientSpec_Server_Authentication_OIDC `json:"oidc,omitempty"`
}

type ClientSpec_Server_Authentication_OIDC struct {
    ClientID     SecretRef `json:"clientId"`
    ClientSecret SecretRef `json:"clientSecret"`
    // +optional
    Audience string `json:"audience,omitempty"`
    TokenEndpointURL string `json:"tokenEndpointUrl"`
    // +optional
    Scope string `json:"scope,omitempty"`
}

type SecretRef struct {
    Secret Secret `json:"secret"`
}
```

**Note:** Making `Token` optional requires a validation webhook to ensure either `token` or `oidc` is specified.

### 2C: TLS Configuration

Add client-side TLS certificate and server CA verification.

```go
type ClientSpec_Server struct {
    // ... existing fields ...

    // +optional
    TLS *ClientSpec_Server_TLS `json:"tls,omitempty"`
}

type ClientSpec_Server_TLS struct {
    // +kubebuilder:default=true
    Enable bool `json:"enable"`
    // +optional
    CertFile *SecretRef `json:"certFile,omitempty"`
    // +optional
    KeyFile *SecretRef `json:"keyFile,omitempty"`
    // +optional
    TrustedCAFile *ConfigMapOrSecretRef `json:"trustedCaFile,omitempty"`
}

type ConfigMapOrSecretRef struct {
    // +optional
    Secret *Secret `json:"secret,omitempty"`
    // +optional
    ConfigMap *ConfigMapRef `json:"configMap,omitempty"`
}

type ConfigMapRef struct {
    Name string `json:"name"`
    Key  string `json:"key"`
}
```

## Example Usage

### STCP with allowUsers

```yaml
apiVersion: frp.zufardhiyaulhaq.com/v1alpha1
kind: Upstream
metadata:
  name: private-api
spec:
  client: my-client
  stcp:
    host: api-service.default.svc
    port: 8080
    secretKey:
      secret:
        name: stcp-secret
        key: secretKey
    allowUsers:
      - "alice"
      - "bob"
```

### STCP Open to All Users

```yaml
apiVersion: frp.zufardhiyaulhaq.com/v1alpha1
kind: Upstream
metadata:
  name: shared-service
spec:
  client: my-client
  stcp:
    host: shared-service.default.svc
    port: 8080
    secretKey:
      secret:
        name: stcp-secret
        key: secretKey
    allowUsers:
      - "*"
```

### OIDC Authentication

```yaml
apiVersion: frp.zufardhiyaulhaq.com/v1alpha1
kind: Client
metadata:
  name: enterprise-client
spec:
  server:
    host: frp.example.com
    port: 7000
    authentication:
      oidc:
        clientId:
          secret:
            name: oidc-creds
            key: clientId
        clientSecret:
          secret:
            name: oidc-creds
            key: clientSecret
        audience: "frp-server"
        tokenEndpointUrl: "https://auth.example.com/oauth/token"
        scope: "openid profile"
```

### TLS with Client Certificates

```yaml
apiVersion: frp.zufardhiyaulhaq.com/v1alpha1
kind: Client
metadata:
  name: secure-client
spec:
  server:
    host: frp.example.com
    port: 7000
    authentication:
      token:
        secret:
          name: frp-token
          key: token
    tls:
      enable: true
      certFile:
        secret:
          name: tls-certs
          key: tls.crt
      keyFile:
        secret:
          name: tls-certs
          key: tls.key
      trustedCaFile:
        configMap:
          name: ca-bundle
          key: ca.crt
```

### TLS with CA Only (Server Verification)

```yaml
apiVersion: frp.zufardhiyaulhaq.com/v1alpha1
kind: Client
metadata:
  name: verified-client
spec:
  server:
    host: frp.example.com
    port: 7000
    authentication:
      token:
        secret:
          name: frp-token
          key: token
    tls:
      enable: true
      trustedCaFile:
        secret:
          name: ca-secret
          key: ca.crt
```

## Files to Modify

### API Types
- `api/v1alpha1/client_types.go` - Add OIDC, TLS types
- `api/v1alpha1/upstream_types.go` - Add allowUsers to STCP/XTCP
- `api/v1alpha1/types.go` - Add SecretRef, ConfigMapRef, ConfigMapOrSecretRef

### Business Logic
- `pkg/client/utils/template.go` - Add TOML for allowUsers, OIDC, TLS
- `pkg/client/models/config.go` - Add config transformation for new fields

### Tests
- Unit tests for all new configuration fields
- Integration tests for OIDC and TLS scenarios

### Examples
- `examples/secure-access/` - allowUsers, OIDC, TLS examples

## TOML Template Additions

### allowUsers for STCP

```toml
{{ if eq $upstream.Type 3 }}
name = "{{ $upstream.Name }}"
type = "stcp"
localIP = "{{ $upstream.STCP.Host }}"
localPort = {{ $upstream.STCP.Port }}
secretKey = "{{ $upstream.STCP.SecretKey }}"

{{ if $upstream.STCP.AllowUsers }}
allowUsers = [{{ range $i, $u := $upstream.STCP.AllowUsers }}{{ if $i }}, {{ end }}"{{ $u }}"{{ end }}]
{{ end }}
{{ end }}
```

### OIDC Authentication

```toml
{{ if eq .Common.ServerAuthentication.Type 2 }}
auth.method = "oidc"
auth.oidc.clientID = "{{ .Common.ServerAuthentication.OIDC.ClientID }}"
auth.oidc.clientSecret = "{{ .Common.ServerAuthentication.OIDC.ClientSecret }}"
auth.oidc.tokenEndpointURL = "{{ .Common.ServerAuthentication.OIDC.TokenEndpointURL }}"
{{ if .Common.ServerAuthentication.OIDC.Audience }}
auth.oidc.audience = "{{ .Common.ServerAuthentication.OIDC.Audience }}"
{{ end }}
{{ if .Common.ServerAuthentication.OIDC.Scope }}
auth.oidc.scope = "{{ .Common.ServerAuthentication.OIDC.Scope }}"
{{ end }}
{{ end }}
```

### TLS Configuration

```toml
{{ if .Common.TLS }}
transport.tls.enable = {{ .Common.TLS.Enable }}
{{ if .Common.TLS.CertFile }}
transport.tls.certFile = "/etc/frp/tls/tls.crt"
{{ end }}
{{ if .Common.TLS.KeyFile }}
transport.tls.keyFile = "/etc/frp/tls/tls.key"
{{ end }}
{{ if .Common.TLS.TrustedCAFile }}
transport.tls.trustedCaFile = "/etc/frp/tls/ca.crt"
{{ end }}
{{ end }}
```

**Note:** TLS files need to be mounted into the pod via additional volume mounts from secrets/configmaps.

## Validation Rules

1. Either `token` or `oidc` must be specified in authentication (not both, not neither)
2. If `oidc` is used, `clientId`, `clientSecret`, and `tokenEndpointUrl` are required
3. For TLS with client certs, both `certFile` and `keyFile` must be provided together
4. `allowUsers` items must be non-empty strings

## Implementation Notes

### TLS File Mounting

When TLS is configured, the pod builder needs to:
1. Create a volume from the secret/configmap containing TLS files
2. Mount the volume at `/etc/frp/tls/`
3. Set appropriate file permissions

### OIDC Secret Handling

The OIDC client credentials should be fetched at config build time, similar to existing token handling.

## Testing Strategy

1. Unit tests for configuration builder with OIDC and TLS configurations
2. Unit tests for allowUsers array serialization
3. Integration test with OIDC provider (mock or real)
4. Pod builder tests for TLS volume mounting
