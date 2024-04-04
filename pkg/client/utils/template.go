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
name = {{ $upstream.TCP.Name }}
type = {{ $upstream.TCP.Type }}
subdomain = {{ $upstream.TCP.SubDomain }}
localIP = {{ $upstream.TCP.Host }}
localPort = {{ $upstream.TCP.Port }}
remotePort = {{ $upstream.TCP.ServerPort }}
{{ if $upstream.TCP.ProxyProtocol }}
transport.proxyProtocolVersion = {{ $upstream.TCP.ProxyProtocol }}
{{ end }}
transport.useEncryption = true
{{ end }}
{{ end }}
`
