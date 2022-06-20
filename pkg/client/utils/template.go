package utils

const CLIENT_TEMPLATE = `
[common]
server_addr = {{ .Common.ServerAddress }}
server_port = {{ .Common.ServerPort }}

{{ if eq .Common.ServerAuthentication.Type 1 }}
token = {{ .Common.ServerAuthentication.Token }}
{{ end }}

admin_addr = {{ .Common.AdminAddress }}
admin_port = {{ .Common.AdminPort }}
admin_user = {{ .Common.AdminUsername }}
admin_pwd = {{ .Common.AdminPassword }}

{{ range $upstream := .Upstreams }}
[{{ $upstream.Name }}]
{{ if eq $upstream.Type 1 }}
type = tcp
local_ip = {{ $upstream.TCP.Host }}
local_port = {{ $upstream.TCP.Port }}
remote_port = {{ $upstream.TCP.ServerPort }}
{{ if eq $upstream.TCP.ProxyProtocol . }}
proxy_protocol_version = {{ $upstream.TCP.ProxyProtocol }}
{{ end }}
use_encryption = true
{{ end }}
{{ end }}
`
