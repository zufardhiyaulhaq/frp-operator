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
local_ip = {{ $upstream.TCP.Host }}
local_port = {{ $upstream.TCP.Port }}
remote_port = {{ $upstream.TCP.ServerPort }}
{{ if $upstream.TCP.ProxyProtocol }}
proxy_protocol_version = {{ $upstream.TCP.ProxyProtocol }}
{{ end }}
use_encryption = true
{{ end }}
{{ end }}
`
