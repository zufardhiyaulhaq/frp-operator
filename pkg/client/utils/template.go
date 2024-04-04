package utils

const CLIENT_TEMPLATE = `
# frpc.toml
[common]
serverAddr = {{ .Common.ServerAddress }}
serverPort = {{ .Common.ServerPort }}

{{ if eq .Common.ServerAuthentication.Type 1 }}
auth.method = "token"
auth.token = {{ .Common.ServerAuthentication.Token }}
{{ end }}

webServer.addr = {{ .Common.AdminAddress }}
webServer.port = {{ .Common.AdminPort }}
webServer.user = {{ .Common.AdminUsername }}
webServer.password = {{ .Common.AdminPassword }}

{{ range $upstream := .Upstreams }}

[{{ $upstream.Name }}]

{{ if eq $upstream.Type 1 }}
name = {{ $upstream.Name }}
type = tcp
localIP = {{ $upstream.TCP.Host }}
localPort = {{ $upstream.TCP.Port }}
remotePort = {{ $upstream.TCP.ServerPort }}

{{ if $upstream.TCP.ProxyProtocol }}
transport.proxyProtocolVersion = {{ $upstream.TCP.ProxyProtocol }}
{{ end }}


{{ if $upstream.TCP.HealthCheck }}
healthCheck.type = "tcp"
healthCheck.timeoutSeconds = $upstream.TCP.HealthCheck.TimeoutSeconds
healthCheck.maxFailed = $upstream.TCP.HealthCheck.MaxFailed
healthCheck.intervalSeconds = $upstream.TCP.HealthCheck.IntervalSeconds
{{ end }}

transport.useEncryption = true
{{ end }}

{{ if eq $upstream.Type 2 }}
name = {{ $upstream.Name }}
type = udp
localIP = {{ $upstream.TCP.Host }}
localPort = {{ $upstream.TCP.Port }}
remotePort = {{ $upstream.TCP.ServerPort }}
{{ end }}

{{ end }}
`
