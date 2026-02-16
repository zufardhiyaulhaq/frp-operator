package utils

const CLIENT_TEMPLATE = `
# frpc.toml
serverAddr = "{{ .Common.ServerAddress }}"
serverPort = {{ .Common.ServerPort }}

{{ if eq .Common.ServerAuthentication.Type 1 }}
auth.method = "token"
auth.token = "{{ .Common.ServerAuthentication.Token }}"
{{ end }}

webServer.addr = "{{ .Common.AdminAddress }}"
webServer.port = {{ .Common.AdminPort }}
webServer.user = "{{ .Common.AdminUsername }}"
webServer.password = "{{ .Common.AdminPassword }}"

{{ if .Common.STUNServer }}
natHoleStunServer = "{{ .Common.STUNServer }}"
{{ end }}

{{ range $upstream := .Upstreams }}

[[proxies]]

{{ if eq $upstream.Type 1 }}
name = "{{ $upstream.Name }}"
type = "tcp"
localIP = "{{ $upstream.TCP.Host }}"
localPort = {{ $upstream.TCP.Port }}
remotePort = {{ $upstream.TCP.ServerPort }}

{{ if $upstream.TCP.ProxyProtocol }}
transport.proxyProtocolVersion = "{{ $upstream.TCP.ProxyProtocol }}"
{{ end }}

{{ if $upstream.TCP.HealthCheck }}
healthCheck.type = "tcp"
healthCheck.timeoutSeconds = {{ $upstream.TCP.HealthCheck.TimeoutSeconds }}
healthCheck.maxFailed = {{ $upstream.TCP.HealthCheck.MaxFailed }}
healthCheck.intervalSeconds = {{ $upstream.TCP.HealthCheck.IntervalSeconds }}
{{ end }}

{{ if $upstream.TCP.Transport }}
transport.useEncryption = {{ $upstream.TCP.Transport.UseEncryption }}
transport.useCompression = {{ $upstream.TCP.Transport.UseCompression }}
{{ if $upstream.TCP.Transport.BandwdithLimit }}
{{ if $upstream.TCP.Transport.BandwdithLimit.Enabled }}
transport.bandwidthLimit = "{{ $upstream.TCP.Transport.BandwdithLimit.Limit }}{{ $upstream.TCP.Transport.BandwdithLimit.Type }}"
transport.bandwidthLimitMode = "client"
{{ end }}
{{ end }}
{{ if $upstream.TCP.Transport.ProxyURL }}
transport.proxyURL = "{{ $upstream.TCP.Transport.ProxyURL }}"
{{ end }}
{{ end }}
{{ end }}

{{ if eq $upstream.Type 2 }}
name = "{{ $upstream.Name }}"
type = "udp"
localIP = "{{ $upstream.UDP.Host }}"
localPort = {{ $upstream.UDP.Port }}
remotePort = {{ $upstream.UDP.ServerPort }}
{{ end }}

{{ if eq $upstream.Type 3 }}
name = "{{ $upstream.Name }}"
type = "stcp"
localIP = "{{ $upstream.STCP.Host }}"
localPort = {{ $upstream.STCP.Port }}
secretKey = "{{ $upstream.STCP.SecretKey }}"

{{ if $upstream.STCP.ProxyProtocol }}
transport.proxyProtocolVersion = "{{ $upstream.STCP.ProxyProtocol }}"
{{ end }}

{{ if $upstream.STCP.HealthCheck }}
healthCheck.type = "tcp"
healthCheck.timeoutSeconds = {{ $upstream.STCP.HealthCheck.TimeoutSeconds }}
healthCheck.maxFailed = {{ $upstream.STCP.HealthCheck.MaxFailed }}
healthCheck.intervalSeconds = {{ $upstream.STCP.HealthCheck.IntervalSeconds }}
{{ end }}

{{ if $upstream.STCP.Transport }}
transport.useEncryption = {{ $upstream.STCP.Transport.UseEncryption }}
transport.useCompression = {{ $upstream.STCP.Transport.UseCompression }}
{{ if $upstream.STCP.Transport.BandwdithLimit }}
{{ if $upstream.STCP.Transport.BandwdithLimit.Enabled }}
transport.bandwidthLimit = "{{ $upstream.STCP.Transport.BandwdithLimit.Limit }}{{ $upstream.STCP.Transport.BandwdithLimit.Type }}"
transport.bandwidthLimitMode = "client"
{{ end }}
{{ end }}
{{ if $upstream.STCP.Transport.ProxyURL }}
transport.proxyURL = "{{ $upstream.STCP.Transport.ProxyURL }}"
{{ end }}
{{ end }}
{{ end }}

{{ if eq $upstream.Type 4 }}
name = "{{ $upstream.Name }}"
type = "xtcp"
localIP = "{{ $upstream.XTCP.Host }}"
localPort = {{ $upstream.XTCP.Port }}
secretKey = "{{ $upstream.XTCP.SecretKey }}"

{{ if $upstream.XTCP.ProxyProtocol }}
transport.proxyProtocolVersion = "{{ $upstream.XTCP.ProxyProtocol }}"
{{ end }}

{{ if $upstream.XTCP.HealthCheck }}
healthCheck.type = "tcp"
healthCheck.timeoutSeconds = {{ $upstream.XTCP.HealthCheck.TimeoutSeconds }}
healthCheck.maxFailed = {{ $upstream.XTCP.HealthCheck.MaxFailed }}
healthCheck.intervalSeconds = {{ $upstream.XTCP.HealthCheck.IntervalSeconds }}
{{ end }}

{{ if $upstream.XTCP.Transport }}
transport.useEncryption = {{ $upstream.XTCP.Transport.UseEncryption }}
transport.useCompression = {{ $upstream.XTCP.Transport.UseCompression }}
{{ if $upstream.XTCP.Transport.BandwdithLimit }}
{{ if $upstream.XTCP.Transport.BandwdithLimit.Enabled }}
transport.bandwidthLimit = "{{ $upstream.XTCP.Transport.BandwdithLimit.Limit }}{{ $upstream.XTCP.Transport.BandwdithLimit.Type }}"
transport.bandwidthLimitMode = "client"
{{ end }}
{{ end }}
{{ if $upstream.XTCP.Transport.ProxyURL }}
transport.proxyURL = "{{ $upstream.XTCP.Transport.ProxyURL }}"
{{ end }}
{{ end }}
{{ end }}

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
{{ range $k, $v := $upstream.HTTP.RequestHeaders }}
requestHeaders.set.{{ $k }} = "{{ $v }}"
{{ end }}
{{ end }}

{{ if $upstream.HTTP.ResponseHeaders }}
{{ range $k, $v := $upstream.HTTP.ResponseHeaders }}
responseHeaders.set.{{ $k }} = "{{ $v }}"
{{ end }}
{{ end }}

{{ if $upstream.HTTP.HTTPUser }}
httpUser = "{{ $upstream.HTTP.HTTPUser }}"
{{ end }}
{{ if $upstream.HTTP.HTTPPassword }}
httpPassword = "{{ $upstream.HTTP.HTTPPassword }}"
{{ end }}

{{ if $upstream.HTTP.HealthCheck }}
healthCheck.type = "{{ $upstream.HTTP.HealthCheck.Type }}"
healthCheck.path = "{{ $upstream.HTTP.HealthCheck.Path }}"
healthCheck.timeoutSeconds = {{ $upstream.HTTP.HealthCheck.TimeoutSeconds }}
healthCheck.maxFailed = {{ $upstream.HTTP.HealthCheck.MaxFailed }}
healthCheck.intervalSeconds = {{ $upstream.HTTP.HealthCheck.IntervalSeconds }}
{{ end }}

{{ if $upstream.HTTP.Transport }}
transport.useEncryption = {{ $upstream.HTTP.Transport.UseEncryption }}
transport.useCompression = {{ $upstream.HTTP.Transport.UseCompression }}
{{ if $upstream.HTTP.Transport.BandwdithLimit }}
{{ if $upstream.HTTP.Transport.BandwdithLimit.Enabled }}
transport.bandwidthLimit = "{{ $upstream.HTTP.Transport.BandwdithLimit.Limit }}{{ $upstream.HTTP.Transport.BandwdithLimit.Type }}"
transport.bandwidthLimitMode = "client"
{{ end }}
{{ end }}
{{ if $upstream.HTTP.Transport.ProxyURL }}
transport.proxyURL = "{{ $upstream.HTTP.Transport.ProxyURL }}"
{{ end }}
{{ end }}
{{ end }}

{{ if eq $upstream.Type 6 }}
name = "{{ $upstream.Name }}"
type = "https"
localIP = "{{ $upstream.HTTPS.Host }}"
localPort = {{ $upstream.HTTPS.Port }}

{{ if $upstream.HTTPS.CustomDomains }}
customDomains = [{{ range $i, $d := $upstream.HTTPS.CustomDomains }}{{ if $i }}, {{ end }}"{{ $d }}"{{ end }}]
{{ end }}

{{ if $upstream.HTTPS.ProxyProtocol }}
transport.proxyProtocolVersion = "{{ $upstream.HTTPS.ProxyProtocol }}"
{{ end }}

{{ if $upstream.HTTPS.Transport }}
transport.useEncryption = {{ $upstream.HTTPS.Transport.UseEncryption }}
transport.useCompression = {{ $upstream.HTTPS.Transport.UseCompression }}
{{ if $upstream.HTTPS.Transport.BandwdithLimit }}
{{ if $upstream.HTTPS.Transport.BandwdithLimit.Enabled }}
transport.bandwidthLimit = "{{ $upstream.HTTPS.Transport.BandwdithLimit.Limit }}{{ $upstream.HTTPS.Transport.BandwdithLimit.Type }}"
transport.bandwidthLimitMode = "client"
{{ end }}
{{ end }}
{{ if $upstream.HTTPS.Transport.ProxyURL }}
transport.proxyURL = "{{ $upstream.HTTPS.Transport.ProxyURL }}"
{{ end }}
{{ end }}
{{ end }}

{{ end }}

{{ range $visitor := .Visitors }}

[[visitors]]
{{ if eq $visitor.Type 1 }}
name = "{{ $visitor.Name }}"
type = "stcp"
serverName = "{{ $visitor.STCP.ServerName }}"
secretKey = "{{ $visitor.STCP.SecretKey }}"
bindAddr = "{{ $visitor.STCP.Host }}"
bindPort = {{ $visitor.STCP.Port }}
{{ end }}

{{ if eq $visitor.Type 2 }}
name = "{{ $visitor.Name }}"
type = "xtcp"
serverName = "{{ $visitor.XTCP.ServerName }}"
secretKey = "{{ $visitor.XTCP.SecretKey }}"
bindAddr = "{{ $visitor.XTCP.Host }}"
bindPort = {{ $visitor.XTCP.Port }}
keepTunnelOpen = {{ $visitor.XTCP.PersistantConnection }}
{{ if not $visitor.XTCP.EnableAssistedAddrs }}
natHoleStun.disableAssistedAddrs = true
{{ end }}
{{ if $visitor.XTCP.Fallback }}
fallbackTo = "{{ $visitor.Name }}-fallback"
fallbackTimeoutMs = {{ $visitor.XTCP.Fallback.Timeout }}

[[visitors]]
name = "{{ $visitor.Name }}-fallback"
type = "stcp"
serverName = "{{ $visitor.XTCP.Fallback.ServerName }}"
secretKey = "{{ $visitor.XTCP.SecretKey }}"
bindPort = -1
{{ end }}
{{ end }}

{{ end }}
`
