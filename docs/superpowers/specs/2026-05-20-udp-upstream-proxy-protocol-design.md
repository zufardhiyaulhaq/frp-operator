{% raw %}
# Proxy Protocol + Transport for UDP Upstream

**Date:** 2026-05-20
**Status:** Approved

## Background

The frp-operator's TCP `Upstream` already exposes a `proxyProtocol` field and a `transport` block. UDP `Upstream` exposes neither — its spec is only `host`, `port`, and `server.port`.

FRP itself supports the full transport block for UDP proxies. Confirmed from the FRP source (`fatedier/frp`, `pkg/config/v1/proxy.go`) and the official reference (https://gofrp.org/en/docs/reference/proxy/):

- `UDPProxyConfig` embeds `ProxyBaseConfig`.
- `ProxyBaseConfig` contains `Transport ProxyTransport`.
- `ProxyTransport` = `useEncryption`, `useCompression`, `bandwidthLimit`, `bandwidthLimitMode`, `proxyProtocolVersion`.

Real-IP / Proxy Protocol for UDP is documented at https://gofrp.org/en/docs/features/common/realip/ ("UDP proxies also support Proxy Protocol functionality").

## Goal

Add two optional fields to the UDP `Upstream` — `proxyProtocol` and `transport` — so UDP reaches transport parity with TCP. The change is additive and non-breaking.

## Design decisions

1. **Dedicated UDP structs.** New `UpstreamSpec_UDP_Transport` and `UpstreamSpec_UDP_Transport_BandwidthLimit` are created. The UDP path does not reference `UpstreamSpec_TCP_Transport`. Same for the model layer. This keeps UDP and TCP structurally independent.
2. **No `proxyURL` field.** TCP's transport struct carries a `proxyURL` field, but `proxyURL` is a client-level FRP setting (`ClientTransportConfig.proxyURL`), not a per-proxy field of `ProxyTransport`. The new UDP transport struct deliberately omits it. Relocating TCP's `proxyURL` to the `Client` CRD is tracked as a separate follow-up spec and is out of scope here.
3. **Correct Go spelling.** TCP's struct uses a misspelled identifier `BandwdithLimit`. The new UDP structs use the correct spelling `BandwidthLimit`. The JSON/YAML wire key is `bandwidthLimit` either way, so this is purely cleaner Go and does not affect users.
4. **`bandwidthLimitMode`.** Rendered as the literal `"client"`, matching the existing TCP template behavior. Exposing a configurable mode is not in scope.

## Changes

### 1. CRD types — `api/v1alpha1/upstream_types.go`

Add two new structs:

```go
type UpstreamSpec_UDP_Transport struct {
    // +kubebuilder:default=true
    UseEncryption bool `json:"useEncryption"`
    // +kubebuilder:default=false
    UseCompression bool `json:"useCompression"`
    // +optional
    BandwidthLimit *UpstreamSpec_UDP_Transport_BandwidthLimit `json:"bandwidthLimit"`
}

type UpstreamSpec_UDP_Transport_BandwidthLimit struct {
    // +kubebuilder:default=false
    Enabled bool `json:"enabled"`
    Limit   int  `json:"limit"`
    // +kubebuilder:validation:Enum=KB;MB
    Type string `json:"type"`
}
```

Extend `UpstreamSpec_UDP` (currently `Host`, `Port`, `Server`) with:

```go
// +kubebuilder:validation:Enum=v1;v2
// +optional
ProxyProtocol *string `json:"proxyProtocol,omitempty"`
// +optional
Transport *UpstreamSpec_UDP_Transport `json:"transport,omitempty"`
```

### 2. Model — `pkg/client/models/config.go`

Add two new model structs:

```go
type Upstream_UDP_Transport struct {
    UseCompression bool
    UseEncryption  bool
    BandwidthLimit *Upstream_UDP_Transport_BandwidthLimit
}

type Upstream_UDP_Transport_BandwidthLimit struct {
    Enabled bool
    Limit   int
    Type    string
}
```

Extend `Upstream_UDP` (currently `Host`, `Port`, `ServerPort`) with:

```go
ProxyProtocol *string
Transport     *Upstream_UDP_Transport
```

### 3. Transform — `pkg/client/models/config.go`

In the `if upstreamObject.Spec.UDP != nil` block (currently ~lines 674–679), after the existing `Host`/`Port`/`ServerPort` assignments, add `ProxyProtocol` and `Transport` copying. This mirrors the TCP transform at ~lines 555–557 and ~567+:

- If `upstreamObject.Spec.UDP.ProxyProtocol != nil`, set `upstream.UDP.ProxyProtocol`.
- If `upstreamObject.Spec.UDP.Transport != nil`, build an `Upstream_UDP_Transport` copying `UseCompression` and `UseEncryption`; if `Transport.BandwidthLimit != nil`, build the nested `Upstream_UDP_Transport_BandwidthLimit` copying `Enabled`, `Limit`, `Type`.

### 4. Template — `pkg/client/utils/template.go`

The UDP block is currently:

```
{{ if eq $upstream.Type 2 }}
name = "{{ $upstream.Name }}"
type = "udp"
localIP = "{{ $upstream.UDP.Host }}"
localPort = {{ $upstream.UDP.Port }}
remotePort = {{ $upstream.UDP.ServerPort }}
{{ end }}
```

Extend it (before the closing `{{ end }}`) with proxy-protocol and transport rendering, producing the same TOML shape the TCP block produces:

```
{{ if $upstream.UDP.ProxyProtocol }}
transport.proxyProtocolVersion = "{{ $upstream.UDP.ProxyProtocol }}"
{{ end }}

{{ if $upstream.UDP.Transport }}
transport.useEncryption = {{ $upstream.UDP.Transport.UseEncryption }}
transport.useCompression = {{ $upstream.UDP.Transport.UseCompression }}
{{ if $upstream.UDP.Transport.BandwidthLimit }}
{{ if $upstream.UDP.Transport.BandwidthLimit.Enabled }}
transport.bandwidthLimit = "{{ $upstream.UDP.Transport.BandwidthLimit.Limit }}{{ $upstream.UDP.Transport.BandwidthLimit.Type }}"
transport.bandwidthLimitMode = "client"
{{ end }}
{{ end }}
{{ end }}
```

### 5. CRD regeneration

Run `make manifests && make generate`. This updates:
- `api/v1alpha1/zz_generated.deepcopy.go` (DeepCopy for the two new structs).
- `charts/frp-operator/crds/frp.zufardhiyaulhaq.com_upstreams.yaml` and `charts/frp-operator/crds/crds.yaml`.
- `config/crd/bases/frp.zufardhiyaulhaq.com_upstreams.yaml`.

### 6. Example — `examples/advanced/udp-transport.yaml`

New example file demonstrating a UDP `Upstream` with `proxyProtocol` and `transport`, following the format of `examples/tcp-full/client/upstream.yaml`:

```yaml
# UDP Upstream with Proxy Protocol and transport tuning
apiVersion: frp.zufardhiyaulhaq.com/v1alpha1
kind: Upstream
metadata:
  name: dns
  namespace: default
spec:
  client: client-01
  udp:
    host: dns-service.default.svc.cluster.local
    port: 53
    server:
      port: 6002
    proxyProtocol: v2
    transport:
      useEncryption: true
      useCompression: true
      bandwidthLimit:
        enabled: true
        limit: 1024
        type: MB
```

## Testing

Add cases to:

- `pkg/client/builder/configuration_builder_test.go` — assert the rendered TOML for: (a) UDP with `proxyProtocol: v2` contains `transport.proxyProtocolVersion = "v2"`; (b) UDP with a full `transport` block contains `transport.useEncryption`, `transport.useCompression`, and `transport.bandwidthLimit` / `bandwidthLimitMode`; (c) bare UDP (no new fields) renders exactly as before — regression guard.
- `pkg/client/models/config_test.go` — assert `NewConfig` maps `Spec.UDP.ProxyProtocol` and `Spec.UDP.Transport` (including the nested bandwidth limit) onto the `Upstream_UDP` model.

## Out of scope

- TCP `proxyURL` relocation to the `Client` CRD — separate follow-up spec.
- Any change to TCP, STCP, XTCP, HTTP, HTTPS, or TCPMUX upstreams.
- A configurable `bandwidthLimitMode` (stays the literal `"client"`).
{% endraw %}
