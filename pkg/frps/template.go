package frps

// FRPS_TEMPLATE renders frps.toml from a Config. Every string goes through quote; optional keys
// are omitted when unset. allowPorts is always rendered (see Ports.AllowRanges).
const FRPS_TEMPLATE = `bindAddr = "0.0.0.0"
bindPort = {{ .Ports.Bind }}
{{- if .Ports.KCP }}
kcpBindPort = {{ .Ports.KCP }}
{{- end }}
{{- if .Ports.QUIC }}
quicBindPort = {{ .Ports.QUIC }}
{{- end }}
auth.method = "token"
auth.token = {{ quote .Token }}
{{- with .TLS }}
transport.tls.force = {{ .Force }}
{{- if .CertFile }}
transport.tls.certFile = {{ quote .CertFile }}
transport.tls.keyFile = {{ quote .KeyFile }}
{{- end }}
{{- if .TrustedCAFile }}
transport.tls.trustedCaFile = {{ quote .TrustedCAFile }}
{{- end }}
{{- end }}
{{- if .Ports.HTTP }}
vhostHTTPPort = {{ .Ports.HTTP }}
{{- end }}
{{- if .Ports.HTTPS }}
vhostHTTPSPort = {{ .Ports.HTTPS }}
{{- end }}
{{- if .Ports.TCPMux }}
tcpmuxHTTPConnectPort = {{ .Ports.TCPMux }}
{{- end }}
{{- if .SubdomainHost }}
subDomainHost = {{ quote .SubdomainHost }}
{{- end }}
allowPorts = [
{{- range .Ports.AllowRanges }}
{{- if eq .Start .End }}
  { single = {{ .Start }} },
{{- else }}
  { start = {{ .Start }}, end = {{ .End }} },
{{- end }}
{{- end }}
]
{{- with .WebServer }}
webServer.addr = "0.0.0.0"
webServer.port = {{ .Port }}
webServer.user = {{ quote .User }}
webServer.password = {{ quote .Password }}
{{- if .CertFile }}
webServer.tls.certFile = {{ quote .CertFile }}
webServer.tls.keyFile = {{ quote .KeyFile }}
{{- end }}
{{- if .Prometheus }}
enablePrometheus = true
{{- end }}
{{- end }}
`
