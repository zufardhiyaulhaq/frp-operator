package utils

const CLIENT_TEMPLATE = `
# frpc.toml
serverAddr = {{ quote .Common.ServerAddress }}
serverPort = {{ .Common.ServerPort }}
{{ if .Common.ClientID }}
clientID = {{ quote .Common.ClientID }}
{{ end }}
{{ if .Common.User }}
user = {{ quote .Common.User }}
{{ end }}

{{ if and .Common.ServerProtocol (ne .Common.ServerProtocol "tcp") }}
transport.protocol = {{ quote .Common.ServerProtocol }}
{{ end }}

{{ if eq .Common.ServerAuthentication.Type 1 }}
auth.method = "token"
auth.token = {{ quote .Common.ServerAuthentication.Token }}
{{ end }}

{{ if eq .Common.ServerAuthentication.Type 2 }}
auth.method = "oidc"
auth.oidc.clientID = {{ quote .Common.ServerAuthentication.OIDCClientID }}
auth.oidc.clientSecret = {{ quote .Common.ServerAuthentication.OIDCClientSecret }}
auth.oidc.tokenEndpointURL = {{ quote .Common.ServerAuthentication.OIDCTokenURL }}
{{ if .Common.ServerAuthentication.OIDCAudience }}
auth.oidc.audience = {{ quote .Common.ServerAuthentication.OIDCAudience }}
{{ end }}
{{ if .Common.ServerAuthentication.OIDCScope }}
auth.oidc.scope = {{ quote .Common.ServerAuthentication.OIDCScope }}
{{ end }}
{{ end }}

webServer.addr = {{ quote .Common.AdminAddress }}
webServer.port = {{ .Common.AdminPort }}
webServer.user = {{ quote .Common.AdminUsername }}
webServer.password = {{ quote .Common.AdminPassword }}
{{ if .Common.PprofEnable }}
webServer.pprofEnable = true
{{ end }}

{{ if .Common.STUNServer }}
natHoleStunServer = {{ quote .Common.STUNServer }}
{{ end }}

{{ if .Common.TLS }}
transport.tls.enable = {{ .Common.TLS.Enable }}
{{ if .Common.TLS.CertFile }}
transport.tls.certFile = {{ quote .Common.TLS.CertFile }}
{{ end }}
{{ if .Common.TLS.KeyFile }}
transport.tls.keyFile = {{ quote .Common.TLS.KeyFile }}
{{ end }}
{{ if .Common.TLS.TrustedCAFile }}
transport.tls.trustedCaFile = {{ quote .Common.TLS.TrustedCAFile }}
{{ end }}
{{ end }}

{{ if .Common.Transport }}
transport.poolCount = {{ .Common.Transport.PoolCount }}
transport.tcpMux = {{ .Common.Transport.TCPMux }}
{{ if .Common.Transport.DialServerTimeout }}
transport.dialServerTimeout = {{ .Common.Transport.DialServerTimeout }}
{{ end }}
{{ if .Common.Transport.DialServerKeepalive }}
transport.dialServerKeepalive = {{ .Common.Transport.DialServerKeepalive }}
{{ end }}
{{ if .Common.Transport.ConnectServerLocalIP }}
transport.connectServerLocalIP = {{ quote .Common.Transport.ConnectServerLocalIP }}
{{ end }}
{{ if .Common.Transport.WireProtocol }}
transport.wireProtocol = {{ quote .Common.Transport.WireProtocol }}
{{ end }}
{{ if .Common.Transport.ProxyURL }}
transport.proxyURL = {{ quote .Common.Transport.ProxyURL }}
{{ end }}
{{ end }}

{{ range $upstream := .Upstreams }}

[[proxies]]

{{ if eq $upstream.Type 1 }}
name = {{ quote $upstream.Name }}
type = "tcp"
{{ if $upstream.Disabled }}
enabled = false
{{ end }}
{{ if $upstream.TCP.Host }}
localIP = {{ quote $upstream.TCP.Host }}
{{ end }}
{{ if $upstream.TCP.Port }}
localPort = {{ $upstream.TCP.Port }}
{{ end }}
remotePort = {{ $upstream.TCP.ServerPort }}

{{ if $upstream.TCP.Plugin }}
plugin.type = {{ quote $upstream.TCP.Plugin.Type }}

{{ if eq $upstream.TCP.Plugin.Type "socks5" }}
{{ if $upstream.TCP.Plugin.Username }}
plugin.username = {{ quote $upstream.TCP.Plugin.Username }}
{{ end }}
{{ if $upstream.TCP.Plugin.Password }}
plugin.password = {{ quote $upstream.TCP.Plugin.Password }}
{{ end }}
{{ end }}

{{ if eq $upstream.TCP.Plugin.Type "http_proxy" }}
{{ if $upstream.TCP.Plugin.Username }}
plugin.httpUser = {{ quote $upstream.TCP.Plugin.Username }}
{{ end }}
{{ if $upstream.TCP.Plugin.Password }}
plugin.httpPassword = {{ quote $upstream.TCP.Plugin.Password }}
{{ end }}
{{ end }}

{{ if eq $upstream.TCP.Plugin.Type "static_file" }}
plugin.localPath = {{ quote $upstream.TCP.Plugin.LocalPath }}
{{ if $upstream.TCP.Plugin.StripPrefix }}
plugin.stripPrefix = {{ quote $upstream.TCP.Plugin.StripPrefix }}
{{ end }}
{{ if $upstream.TCP.Plugin.HTTPUser }}
plugin.httpUser = {{ quote $upstream.TCP.Plugin.HTTPUser }}
{{ end }}
{{ if $upstream.TCP.Plugin.HTTPPassword }}
plugin.httpPassword = {{ quote $upstream.TCP.Plugin.HTTPPassword }}
{{ end }}
{{ end }}

{{ if eq $upstream.TCP.Plugin.Type "unix_domain_socket" }}
plugin.unixPath = {{ quote $upstream.TCP.Plugin.UnixPath }}
{{ end }}

{{ if eq $upstream.TCP.Plugin.Type "https2http" }}
plugin.localAddr = {{ quote $upstream.TCP.Plugin.LocalAddr }}
{{ end }}

{{ if eq $upstream.TCP.Plugin.Type "https2https" }}
plugin.localAddr = {{ quote $upstream.TCP.Plugin.LocalAddr }}
{{ end }}

{{ if eq $upstream.TCP.Plugin.Type "http2https" }}
plugin.localAddr = {{ quote $upstream.TCP.Plugin.LocalAddr }}
{{ end }}

{{ if eq $upstream.TCP.Plugin.Type "http2http" }}
plugin.localAddr = {{ quote $upstream.TCP.Plugin.LocalAddr }}
{{ end }}
{{ end }}

{{ if $upstream.TCP.ProxyProtocol }}
transport.proxyProtocolVersion = {{ quote $upstream.TCP.ProxyProtocol }}
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
{{ end }}

{{ if $upstream.TCP.LoadBalancer }}
loadBalancer.group = {{ quote $upstream.TCP.LoadBalancer.Group }}
{{ if $upstream.TCP.LoadBalancer.GroupKey }}
loadBalancer.groupKey = {{ quote $upstream.TCP.LoadBalancer.GroupKey }}
{{ end }}
{{ end }}
{{ end }}

{{ if eq $upstream.Type 2 }}
name = {{ quote $upstream.Name }}
type = "udp"
{{ if $upstream.Disabled }}
enabled = false
{{ end }}
localIP = {{ quote $upstream.UDP.Host }}
localPort = {{ $upstream.UDP.Port }}
remotePort = {{ $upstream.UDP.ServerPort }}

{{ if $upstream.UDP.ProxyProtocol }}
transport.proxyProtocolVersion = {{ quote $upstream.UDP.ProxyProtocol }}
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
{{ end }}

{{ if eq $upstream.Type 3 }}
name = {{ quote $upstream.Name }}
type = "stcp"
{{ if $upstream.Disabled }}
enabled = false
{{ end }}
localIP = {{ quote $upstream.STCP.Host }}
localPort = {{ $upstream.STCP.Port }}
secretKey = {{ quote $upstream.STCP.SecretKey }}

{{ if $upstream.STCP.ProxyProtocol }}
transport.proxyProtocolVersion = {{ quote $upstream.STCP.ProxyProtocol }}
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
{{ end }}

{{ if $upstream.STCP.AllowUsers }}
allowUsers = [{{ range $i, $u := $upstream.STCP.AllowUsers }}{{ if $i }}, {{ end }}{{ quote $u }}{{ end }}]
{{ end }}
{{ end }}

{{ if eq $upstream.Type 4 }}
name = {{ quote $upstream.Name }}
type = "xtcp"
{{ if $upstream.Disabled }}
enabled = false
{{ end }}
localIP = {{ quote $upstream.XTCP.Host }}
localPort = {{ $upstream.XTCP.Port }}
secretKey = {{ quote $upstream.XTCP.SecretKey }}

{{ if $upstream.XTCP.ProxyProtocol }}
transport.proxyProtocolVersion = {{ quote $upstream.XTCP.ProxyProtocol }}
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
{{ end }}

{{ if $upstream.XTCP.AllowUsers }}
allowUsers = [{{ range $i, $u := $upstream.XTCP.AllowUsers }}{{ if $i }}, {{ end }}{{ quote $u }}{{ end }}]
{{ end }}
{{ end }}

{{ if eq $upstream.Type 5 }}
name = {{ quote $upstream.Name }}
type = "http"
{{ if $upstream.Disabled }}
enabled = false
{{ end }}
localIP = {{ quote $upstream.HTTP.Host }}
localPort = {{ $upstream.HTTP.Port }}

{{ if $upstream.HTTP.Subdomain }}
subdomain = {{ quote $upstream.HTTP.Subdomain }}
{{ end }}

{{ if $upstream.HTTP.CustomDomains }}
customDomains = [{{ range $i, $d := $upstream.HTTP.CustomDomains }}{{ if $i }}, {{ end }}{{ quote $d }}{{ end }}]
{{ end }}

{{ if $upstream.HTTP.Locations }}
locations = [{{ range $i, $l := $upstream.HTTP.Locations }}{{ if $i }}, {{ end }}{{ quote $l }}{{ end }}]
{{ end }}

{{ if $upstream.HTTP.HostHeaderRewrite }}
hostHeaderRewrite = {{ quote $upstream.HTTP.HostHeaderRewrite }}
{{ end }}

{{ if $upstream.HTTP.RequestHeaders }}
{{ range $k, $v := $upstream.HTTP.RequestHeaders }}
requestHeaders.set.{{ quote $k }} = {{ quote $v }}
{{ end }}
{{ end }}

{{ if $upstream.HTTP.ResponseHeaders }}
{{ range $k, $v := $upstream.HTTP.ResponseHeaders }}
responseHeaders.set.{{ quote $k }} = {{ quote $v }}
{{ end }}
{{ end }}

{{ if $upstream.HTTP.HTTPUser }}
httpUser = {{ quote $upstream.HTTP.HTTPUser }}
{{ end }}
{{ if $upstream.HTTP.HTTPPassword }}
httpPassword = {{ quote $upstream.HTTP.HTTPPassword }}
{{ end }}

{{ if $upstream.HTTP.HealthCheck }}
healthCheck.type = {{ quote $upstream.HTTP.HealthCheck.Type }}
healthCheck.path = {{ quote $upstream.HTTP.HealthCheck.Path }}
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
{{ end }}

{{ if $upstream.HTTP.LoadBalancer }}
loadBalancer.group = {{ quote $upstream.HTTP.LoadBalancer.Group }}
{{ if $upstream.HTTP.LoadBalancer.GroupKey }}
loadBalancer.groupKey = {{ quote $upstream.HTTP.LoadBalancer.GroupKey }}
{{ end }}
{{ end }}
{{ end }}

{{ if eq $upstream.Type 6 }}
name = {{ quote $upstream.Name }}
type = "https"
{{ if $upstream.Disabled }}
enabled = false
{{ end }}
localIP = {{ quote $upstream.HTTPS.Host }}
localPort = {{ $upstream.HTTPS.Port }}

{{ if $upstream.HTTPS.CustomDomains }}
customDomains = [{{ range $i, $d := $upstream.HTTPS.CustomDomains }}{{ if $i }}, {{ end }}{{ quote $d }}{{ end }}]
{{ end }}

{{ if $upstream.HTTPS.ProxyProtocol }}
transport.proxyProtocolVersion = {{ quote $upstream.HTTPS.ProxyProtocol }}
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
{{ end }}

{{ if $upstream.HTTPS.LoadBalancer }}
loadBalancer.group = {{ quote $upstream.HTTPS.LoadBalancer.Group }}
{{ if $upstream.HTTPS.LoadBalancer.GroupKey }}
loadBalancer.groupKey = {{ quote $upstream.HTTPS.LoadBalancer.GroupKey }}
{{ end }}
{{ end }}
{{ end }}

{{ if eq $upstream.Type 7 }}
name = {{ quote $upstream.Name }}
type = "tcpmux"
{{ if $upstream.Disabled }}
enabled = false
{{ end }}
multiplexer = {{ quote $upstream.TCPMUX.Multiplexer }}
localIP = {{ quote $upstream.TCPMUX.Host }}
localPort = {{ $upstream.TCPMUX.Port }}

{{ if $upstream.TCPMUX.CustomDomains }}
customDomains = [{{ range $i, $d := $upstream.TCPMUX.CustomDomains }}{{ if $i }}, {{ end }}{{ quote $d }}{{ end }}]
{{ end }}

{{ if $upstream.TCPMUX.Transport }}
transport.useEncryption = {{ $upstream.TCPMUX.Transport.UseEncryption }}
transport.useCompression = {{ $upstream.TCPMUX.Transport.UseCompression }}
{{ if $upstream.TCPMUX.Transport.BandwdithLimit }}
{{ if $upstream.TCPMUX.Transport.BandwdithLimit.Enabled }}
transport.bandwidthLimit = "{{ $upstream.TCPMUX.Transport.BandwdithLimit.Limit }}{{ $upstream.TCPMUX.Transport.BandwdithLimit.Type }}"
transport.bandwidthLimitMode = "client"
{{ end }}
{{ end }}
{{ end }}
{{ end }}

{{ end }}

{{ range $visitor := .Visitors }}

[[visitors]]
{{ if eq $visitor.Type 1 }}
name = {{ quote $visitor.Name }}
type = "stcp"
{{ if $visitor.Disabled }}
enabled = false
{{ end }}
{{ if $visitor.STCP.ServerUser }}
serverUser = {{ quote $visitor.STCP.ServerUser }}
{{ end }}
serverName = {{ quote $visitor.STCP.ServerName }}
secretKey = {{ quote $visitor.STCP.SecretKey }}
bindAddr = {{ quote $visitor.STCP.Host }}
bindPort = {{ $visitor.STCP.Port }}
{{ end }}

{{ if eq $visitor.Type 2 }}
name = {{ quote $visitor.Name }}
type = "xtcp"
{{ if $visitor.Disabled }}
enabled = false
{{ end }}
{{ if $visitor.XTCP.ServerUser }}
serverUser = {{ quote $visitor.XTCP.ServerUser }}
{{ end }}
serverName = {{ quote $visitor.XTCP.ServerName }}
secretKey = {{ quote $visitor.XTCP.SecretKey }}
bindAddr = {{ quote $visitor.XTCP.Host }}
bindPort = {{ $visitor.XTCP.Port }}
keepTunnelOpen = {{ $visitor.XTCP.PersistantConnection }}
{{ if not $visitor.XTCP.EnableAssistedAddrs }}
natTraversal.disableAssistedAddrs = true
{{ end }}
{{ if $visitor.XTCP.Fallback }}
fallbackTo = {{ quote (printf "%s-fallback" $visitor.Name) }}
fallbackTimeoutMs = {{ $visitor.XTCP.Fallback.Timeout }}

[[visitors]]
name = {{ quote (printf "%s-fallback" $visitor.Name) }}
type = "stcp"
{{ if $visitor.Disabled }}
enabled = false
{{ end }}
{{ if $visitor.XTCP.ServerUser }}
serverUser = {{ quote $visitor.XTCP.ServerUser }}
{{ end }}
serverName = {{ quote $visitor.XTCP.Fallback.ServerName }}
secretKey = {{ quote $visitor.XTCP.Fallback.SecretKey }}
bindPort = -1
{{ end }}
{{ end }}

{{ end }}
`
