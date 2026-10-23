package frps

import v1 "github.com/zufardhiyaulhaq/frp-operator/api/v1alpha1"

// Image is the frps image the operator runs; keep it on the same frp release as frpcImage.
const Image = "fatedier/frps:v0.71.1"

const (
	// ConfigDir holds frps.toml in the frps pod.
	ConfigDir = "/etc/frps/config"
	// TLSDir holds tls.crt, tls.key and ca.crt for the tunnel.
	TLSDir = "/etc/frps/tls"
	// DashboardTLSDir holds tls.crt and tls.key for the web server.
	DashboardTLSDir = "/etc/frps/dashboard-tls"

	// SelectorLabel selects a Server's frps pod.
	SelectorLabel = "frp.zufardhiyaulhaq.com/server"
	// ServiceRoleLabel tells a Server's public Service ("proxy") from its web Service ("web").
	ServiceRoleLabel = "frp.zufardhiyaulhaq.com/service"
	// ConfigHashAnnotation on the frps pod is the hash of everything it was started with.
	ConfigHashAnnotation = "frp.zufardhiyaulhaq.com/config-hash"
)

func ConfigSecretName(s *v1.Server) string     { return s.Name + "-frps-config" }
func ConnectionSecretName(s *v1.Server) string { return s.Name + "-connection" }
func DashboardSecretName(s *v1.Server) string  { return s.Name + "-dashboard" }
func PodName(s *v1.Server) string              { return s.Name + "-frps" }
func ServiceName(s *v1.Server) string          { return s.Name + "-frps" }
func WebServiceName(s *v1.Server) string       { return s.Name + "-frps-web" }
