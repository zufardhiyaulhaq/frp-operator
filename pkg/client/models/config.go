package models

import (
	"context"
	"fmt"
	"sort"

	corev1 "k8s.io/api/core/v1"

	frpv1alpha1 "github.com/zufardhiyaulhaq/frp-operator/api/v1alpha1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type ServerAuthenticationType int64

const DEFAULT_ADMIN_ADDRESS = "0.0.0.0"
const DEFAULT_ADMIN_PORT = 7400
const DEFAULT_ADMIN_USERNAME = "frpc-user"
const DEFAULT_ADMIN_PASSWORD = "frpc-password"

const (
	NoAuth    ServerAuthenticationType = iota // 0 - no authentication
	TokenAuth ServerAuthenticationType = iota // 1 - token authentication
	OIDCAuth  ServerAuthenticationType = iota // 2 - OIDC authentication
)

type Config struct {
	Common    Common
	Upstreams Upstreams
	Visitors  Visitors
}

type TransportConfig struct {
	PoolCount            int
	TCPMux               bool
	DialServerTimeout    string
	DialServerKeepalive  string
	ConnectServerLocalIP string
	WireProtocol         string
}

type Common struct {
	ServerAddress        string
	ServerPort           int
	ServerProtocol       string
	ClientID             string
	ServerAuthentication ServerAuthentication
	AdminAddress         string
	AdminPort            int
	AdminUsername        string
	AdminPassword        string
	STUNServer           *string
	PprofEnable          bool
	TLS                  *TLSConfig
	Transport            *TransportConfig
}

type TLSConfig struct {
	Enable        bool
	CertFile      string
	KeyFile       string
	TrustedCAFile string
}

type ServerAuthentication struct {
	Type             ServerAuthenticationType
	Token            string
	OIDCClientID     string
	OIDCClientSecret string
	OIDCTokenURL     string
	OIDCAudience     string
	OIDCScope        string
}

type VisitorType int64

const (
	STCPVisitor VisitorType = iota
	XTCPVisitor VisitorType = iota
)

type Visitor struct {
	Name    string
	Enabled bool
	Type    VisitorType
	STCP    Visitor_STCP
	XTCP    Visitor_XTCP
}

type Visitors []Visitor

func (p Visitors) Len() int {
	return len(p)
}
func (p Visitors) Less(i, j int) bool {
	return p[i].Name < p[j].Name
}
func (p Visitors) Swap(i, j int) {
	p[i], p[j] = p[j], p[i]
}

type Visitor_STCP struct {
	Host       string
	Port       int
	ServerName string
	SecretKey  string
}

type Visitor_XTCP struct {
	Host                 string
	Port                 int
	ServerName           string
	SecretKey            string
	PersistantConnection bool
	EnableAssistedAddrs  bool
	Fallback             *Visitor_XTCP_Fallback
}

type Visitor_XTCP_Fallback struct {
	ServerName string
	Timeout    int
}

type UpstreamType int64

const (
	TCP    UpstreamType = iota
	UDP    UpstreamType = iota
	STCP   UpstreamType = iota
	XTCP   UpstreamType = iota
	HTTP   UpstreamType = iota
	HTTPS  UpstreamType = iota
	TCPMUX UpstreamType = iota
)

type Upstream_TCPMUX struct {
	Host          string
	Port          int
	Multiplexer   string
	CustomDomains []string
	Transport     *Upstream_TCP_Transport
}

type Upstream struct {
	Name    string
	Enabled bool
	Type    UpstreamType
	TCP     Upstream_TCP
	UDP     Upstream_UDP
	STCP    Upstream_STCP
	XTCP    Upstream_STCP
	HTTP    Upstream_HTTP
	HTTPS   Upstream_HTTPS
	TCPMUX  Upstream_TCPMUX
}

type Upstreams []Upstream

func (p Upstreams) Len() int {
	return len(p)
}
func (p Upstreams) Less(i, j int) bool {
	return p[i].Name < p[j].Name
}
func (p Upstreams) Swap(i, j int) {
	p[i], p[j] = p[j], p[i]
}

type Upstream_STCP struct {
	Host          string
	Port          int
	SecretKey     string
	ProxyProtocol *string
	HealthCheck   *Upstream_TCP_HealthCheck
	Transport     *Upstream_TCP_Transport
	AllowUsers    []string
}

type Upstream_XTCP struct {
	Host          string
	Port          int
	SecretKey     string
	ProxyProtocol *string
	HealthCheck   *Upstream_TCP_HealthCheck
	Transport     *Upstream_TCP_Transport
	AllowUsers    []string
}

type LoadBalancerConfig struct {
	Group    string
	GroupKey string
}

// resolveLoadBalancer converts a CRD LoadBalancer into the model, reading groupKey from its
// Secret when set. A missing Secret or key leaves GroupKey empty (matches prior TCP behavior).
func resolveLoadBalancer(k8sclient client.Client, namespace string, lb *frpv1alpha1.LoadBalancer) *LoadBalancerConfig {
	if lb == nil {
		return nil
	}
	result := &LoadBalancerConfig{Group: lb.Group}
	if lb.GroupKey != nil {
		secret := &corev1.Secret{}
		err := k8sclient.Get(context.TODO(), types.NamespacedName{
			Name:      lb.GroupKey.Secret.Name,
			Namespace: namespace,
		}, secret)
		if err == nil {
			if val, ok := secret.Data[lb.GroupKey.Secret.Key]; ok {
				result.GroupKey = string(val)
			}
		}
	}
	return result
}

type PluginConfig struct {
	Type         string
	Username     string
	Password     string
	LocalPath    string
	StripPrefix  string
	HTTPUser     string
	HTTPPassword string
	LocalAddr    string
	UnixPath     string
}

type Upstream_TCP struct {
	Host          string
	Port          int
	ServerPort    int
	ProxyProtocol *string
	HealthCheck   *Upstream_TCP_HealthCheck
	Transport     *Upstream_TCP_Transport
	LoadBalancer  *LoadBalancerConfig
	Plugin        *PluginConfig
}

type Upstream_TCP_HealthCheck struct {
	TimeoutSeconds  int
	MaxFailed       int
	IntervalSeconds int
}

type Upstream_TCP_Transport struct {
	UseCompression bool
	UseEncryption  bool
	BandwdithLimit *Upstream_TCP_Transport_BandwidthLimit
	ProxyURL       *string
}

type Upstream_TCP_Transport_BandwidthLimit struct {
	Enabled bool
	Limit   int
	Type    string
}

type Upstream_UDP struct {
	Host          string
	Port          int
	ServerPort    int
	ProxyProtocol *string
	Transport     *Upstream_UDP_Transport
}

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

type Upstream_HTTP struct {
	Host              string
	Port              int
	Subdomain         string
	CustomDomains     []string
	Locations         []string
	HostHeaderRewrite string
	RequestHeaders    map[string]string
	ResponseHeaders   map[string]string
	HTTPUser          string
	HTTPPassword      string
	HealthCheck       *Upstream_HTTP_HealthCheck
	Transport         *Upstream_TCP_Transport
	LoadBalancer      *LoadBalancerConfig
}

type Upstream_HTTP_HealthCheck struct {
	Type            string
	Path            string
	TimeoutSeconds  int
	IntervalSeconds int
	MaxFailed       int
}

type Upstream_HTTPS struct {
	Host          string
	Port          int
	CustomDomains []string
	ProxyProtocol *string
	Transport     *Upstream_TCP_Transport
	LoadBalancer  *LoadBalancerConfig
}

// isEnabled interprets the optional CRD enabled flag: nil or true means enabled.
func isEnabled(enabled *bool) bool {
	return enabled == nil || *enabled
}

// validateUpstreamServerPorts checks that no two TCP/UDP upstreams use the same server port
// unless they are in the same load balancer group (which is intentional for load balancing)
func validateUpstreamServerPorts(upstreamObjects []frpv1alpha1.Upstream) error {
	// Track port -> {upstreamName, lbGroup} for conflict detection
	type portInfo struct {
		upstreamName string
		lbGroup      string
	}
	serverPorts := make(map[int]portInfo) // port -> first upstream info

	for _, upstream := range upstreamObjects {
		if !isEnabled(upstream.Spec.Enabled) {
			continue // disabled upstreams do not reserve their port
		}

		var port int
		var protocol string
		var lbGroup string

		if upstream.Spec.TCP != nil {
			port = upstream.Spec.TCP.Server.Port
			protocol = "TCP"
			if upstream.Spec.TCP.LoadBalancer != nil {
				lbGroup = upstream.Spec.TCP.LoadBalancer.Group
			}
		} else if upstream.Spec.UDP != nil {
			port = upstream.Spec.UDP.Server.Port
			protocol = "UDP"
		} else {
			continue // STCP/XTCP/HTTP/HTTPS/TCPMUX don't have server ports
		}

		if existing, exists := serverPorts[port]; exists {
			// Allow same port if both are in the same load balancer group
			if lbGroup != "" && existing.lbGroup == lbGroup {
				continue // Same LB group, allowed
			}
			return errors.NewBadRequest(
				fmt.Sprintf("duplicate server port %d: upstream %q (%s) conflicts with upstream %q",
					port, upstream.Name, protocol, existing.upstreamName))
		}
		serverPorts[port] = portInfo{upstreamName: upstream.Name, lbGroup: lbGroup}
	}

	return nil
}

// validateVisitorPorts checks that no two STCP/XTCP visitors use the same port
func validateVisitorPorts(visitorObjects []frpv1alpha1.Visitor) error {
	visitorPorts := make(map[int]string) // port -> visitor name

	for _, visitor := range visitorObjects {
		if !isEnabled(visitor.Spec.Enabled) {
			continue // disabled visitors do not reserve their port
		}

		var port int
		var protocol string

		if visitor.Spec.STCP != nil {
			port = visitor.Spec.STCP.Port
			protocol = "STCP"
		} else if visitor.Spec.XTCP != nil {
			port = visitor.Spec.XTCP.Port
			protocol = "XTCP"
		} else {
			continue
		}

		if existingName, exists := visitorPorts[port]; exists {
			return errors.NewBadRequest(
				fmt.Sprintf("duplicate visitor port %d: visitor %q (%s) conflicts with visitor %q",
					port, visitor.Name, protocol, existingName))
		}
		visitorPorts[port] = visitor.Name
	}

	return nil
}

// VisitorServicePorts returns the bind ports of enabled STCP/XTCP visitors, in input order.
// Disabled visitors are excluded so they do not occupy a port on the client Service.
func VisitorServicePorts(visitorObjects []frpv1alpha1.Visitor) []int {
	ports := []int{}
	for _, visitor := range visitorObjects {
		if !isEnabled(visitor.Spec.Enabled) {
			continue
		}
		if visitor.Spec.STCP != nil {
			ports = append(ports, visitor.Spec.STCP.Port)
		}
		if visitor.Spec.XTCP != nil {
			ports = append(ports, visitor.Spec.XTCP.Port)
		}
	}
	return ports
}

func NewConfig(k8sclient client.Client,
	clientObject *frpv1alpha1.Client,
	upstreamObjects []frpv1alpha1.Upstream,
	visitorObjects []frpv1alpha1.Visitor,
) (Config, error) {
	// Validate that no duplicate server ports exist for TCP/UDP upstreams
	if err := validateUpstreamServerPorts(upstreamObjects); err != nil {
		return Config{}, err
	}

	// Validate that no duplicate ports exist for STCP/XTCP visitors
	if err := validateVisitorPorts(visitorObjects); err != nil {
		return Config{}, err
	}

	config := Config{
		Common: Common{
			ServerAddress:  clientObject.Spec.Server.Host,
			ServerPort:     clientObject.Spec.Server.Port,
			ServerProtocol: "TCP",
			ClientID:       clientObject.Namespace + "/" + clientObject.Name,
			AdminAddress:   DEFAULT_ADMIN_ADDRESS,
			AdminPort:      DEFAULT_ADMIN_PORT,
			AdminUsername:  DEFAULT_ADMIN_USERNAME,
			AdminPassword:  DEFAULT_ADMIN_PASSWORD,
			STUNServer:     clientObject.Spec.Server.STUNServer,
		},
	}

	if clientObject.Spec.Server.Protocol != nil {
		config.Common.ServerProtocol = *clientObject.Spec.Server.Protocol
	}

	if clientObject.Spec.Server.AdminServer != nil {
		config.Common.AdminPort = clientObject.Spec.Server.AdminServer.Port
		config.Common.PprofEnable = clientObject.Spec.Server.AdminServer.PprofEnable

		// fetch admin username from secret
		if clientObject.Spec.Server.AdminServer.Username != nil {
			secret := &corev1.Secret{}

			err := k8sclient.Get(context.TODO(), types.NamespacedName{Name: clientObject.Spec.Server.AdminServer.Username.Secret.Name, Namespace: clientObject.Namespace}, secret)
			if err == nil {
				usernameByte, ok := secret.Data[clientObject.Spec.Server.AdminServer.Username.Secret.Key]
				if ok {
					config.Common.AdminUsername = string(usernameByte)
				}
			}
		}

		// fetch admin password from secret
		if clientObject.Spec.Server.AdminServer.Password != nil {
			secret := &corev1.Secret{}

			err := k8sclient.Get(context.TODO(), types.NamespacedName{Name: clientObject.Spec.Server.AdminServer.Password.Secret.Name, Namespace: clientObject.Namespace}, secret)
			if err == nil {
				usernameByte, ok := secret.Data[clientObject.Spec.Server.AdminServer.Password.Secret.Key]
				if ok {
					config.Common.AdminPassword = string(usernameByte)
				}
			}
		}
	}

	// Validate authentication - exactly one method must be specified
	if clientObject.Spec.Server.Authentication.Token == nil && clientObject.Spec.Server.Authentication.OIDC == nil {
		return config, errors.NewBadRequest("either token or oidc authentication is required")
	}
	if clientObject.Spec.Server.Authentication.Token != nil && clientObject.Spec.Server.Authentication.OIDC != nil {
		return config, errors.NewBadRequest("only one authentication method (token or oidc) can be specified")
	}

	if clientObject.Spec.Server.Authentication.Token != nil {
		config.Common.ServerAuthentication.Type = 1

		secret := &corev1.Secret{}
		err := k8sclient.Get(context.TODO(), types.NamespacedName{Name: clientObject.Spec.Server.Authentication.Token.Secret.Name, Namespace: clientObject.Namespace}, secret)
		if err != nil && errors.IsNotFound(err) {
			return config, err
		} else if err != nil {
			return config, err
		}

		tokenByte, ok := secret.Data[clientObject.Spec.Server.Authentication.Token.Secret.Key]
		if !ok {
			return config, err
		}

		config.Common.ServerAuthentication.Token = string(tokenByte)
	}

	// Handle OIDC authentication
	if clientObject.Spec.Server.Authentication.OIDC != nil {
		config.Common.ServerAuthentication.Type = 2

		// Fetch client ID from secret
		secret := &corev1.Secret{}
		err := k8sclient.Get(context.TODO(), types.NamespacedName{
			Name:      clientObject.Spec.Server.Authentication.OIDC.ClientID.Secret.Name,
			Namespace: clientObject.Namespace,
		}, secret)
		if err != nil {
			return config, err
		}
		clientIDByte, ok := secret.Data[clientObject.Spec.Server.Authentication.OIDC.ClientID.Secret.Key]
		if !ok {
			return config, errors.NewBadRequest(fmt.Sprintf("clientId key %s not found in secret %s",
				clientObject.Spec.Server.Authentication.OIDC.ClientID.Secret.Key,
				clientObject.Spec.Server.Authentication.OIDC.ClientID.Secret.Name))
		}
		config.Common.ServerAuthentication.OIDCClientID = string(clientIDByte)

		// Fetch client secret from secret
		err = k8sclient.Get(context.TODO(), types.NamespacedName{
			Name:      clientObject.Spec.Server.Authentication.OIDC.ClientSecret.Secret.Name,
			Namespace: clientObject.Namespace,
		}, secret)
		if err != nil {
			return config, err
		}
		clientSecretByte, ok := secret.Data[clientObject.Spec.Server.Authentication.OIDC.ClientSecret.Secret.Key]
		if !ok {
			return config, errors.NewBadRequest(fmt.Sprintf("clientSecret key %s not found in secret %s",
				clientObject.Spec.Server.Authentication.OIDC.ClientSecret.Secret.Key,
				clientObject.Spec.Server.Authentication.OIDC.ClientSecret.Secret.Name))
		}
		config.Common.ServerAuthentication.OIDCClientSecret = string(clientSecretByte)

		config.Common.ServerAuthentication.OIDCTokenURL = clientObject.Spec.Server.Authentication.OIDC.TokenEndpointURL
		config.Common.ServerAuthentication.OIDCAudience = clientObject.Spec.Server.Authentication.OIDC.Audience
		config.Common.ServerAuthentication.OIDCScope = clientObject.Spec.Server.Authentication.OIDC.Scope
	}

	// Handle TLS configuration
	if clientObject.Spec.Server.TLS != nil {
		config.Common.TLS = &TLSConfig{
			Enable: clientObject.Spec.Server.TLS.Enable,
		}

		// Set cert file path if configured
		if clientObject.Spec.Server.TLS.CertFile != nil {
			config.Common.TLS.CertFile = "/etc/frp/tls/tls.crt"
		}

		// Set key file path if configured
		if clientObject.Spec.Server.TLS.KeyFile != nil {
			config.Common.TLS.KeyFile = "/etc/frp/tls/tls.key"
		}

		// Set CA file path if configured
		if clientObject.Spec.Server.TLS.TrustedCAFile != nil {
			config.Common.TLS.TrustedCAFile = "/etc/frp/tls/ca.crt"
		}
	}

	// Handle Transport configuration
	if clientObject.Spec.Server.Transport != nil {
		config.Common.Transport = &TransportConfig{
			PoolCount:            clientObject.Spec.Server.Transport.PoolCount,
			DialServerTimeout:    clientObject.Spec.Server.Transport.DialServerTimeout,
			DialServerKeepalive:  clientObject.Spec.Server.Transport.DialServerKeepalive,
			ConnectServerLocalIP: clientObject.Spec.Server.Transport.ConnectServerLocalIP,
			WireProtocol:         clientObject.Spec.Server.Transport.WireProtocol,
		}

		if clientObject.Spec.Server.Transport.TCPMux != nil {
			config.Common.Transport.TCPMux = *clientObject.Spec.Server.Transport.TCPMux
		} else {
			config.Common.Transport.TCPMux = true // default
		}
	}

	upstreams := []Upstream{}
	for _, upstreamObject := range upstreamObjects {
		upstream := Upstream{
			Name:    upstreamObject.Name,
			Enabled: isEnabled(upstreamObject.Spec.Enabled),
		}

		if upstreamObject.Spec.TCP == nil && upstreamObject.Spec.UDP == nil && upstreamObject.Spec.STCP == nil && upstreamObject.Spec.XTCP == nil && upstreamObject.Spec.HTTP == nil && upstreamObject.Spec.HTTPS == nil && upstreamObject.Spec.TCPMUX == nil {
			return config, errors.NewBadRequest("TCP, UDP, STCP, XTCP, HTTP, HTTPS, or TCPMUX upstream is required")
		}

		protocolCount := 0
		if upstreamObject.Spec.TCP != nil {
			protocolCount++
		}
		if upstreamObject.Spec.UDP != nil {
			protocolCount++
		}
		if upstreamObject.Spec.STCP != nil {
			protocolCount++
		}
		if upstreamObject.Spec.XTCP != nil {
			protocolCount++
		}
		if upstreamObject.Spec.HTTP != nil {
			protocolCount++
		}
		if upstreamObject.Spec.HTTPS != nil {
			protocolCount++
		}
		if upstreamObject.Spec.TCPMUX != nil {
			protocolCount++
		}
		if protocolCount > 1 {
			return config, errors.NewBadRequest("Multiple protocol on the same Upstream object")
		}

		if upstreamObject.Spec.TCP != nil {
			upstream.Type = 1
			upstream.TCP.Host = upstreamObject.Spec.TCP.Host
			upstream.TCP.Port = upstreamObject.Spec.TCP.Port
			upstream.TCP.ServerPort = upstreamObject.Spec.TCP.Server.Port

			if upstreamObject.Spec.TCP.ProxyProtocol != nil {
				upstream.TCP.ProxyProtocol = upstreamObject.Spec.TCP.ProxyProtocol
			}

			if upstreamObject.Spec.TCP.HealthCheck != nil {
				upstream.TCP.HealthCheck = &Upstream_TCP_HealthCheck{
					TimeoutSeconds:  upstreamObject.Spec.TCP.HealthCheck.TimeoutSeconds,
					MaxFailed:       upstreamObject.Spec.TCP.HealthCheck.MaxFailed,
					IntervalSeconds: upstreamObject.Spec.TCP.HealthCheck.IntervalSeconds,
				}
			}

			if upstreamObject.Spec.TCP.Transport != nil {
				upstream.TCP.Transport = &Upstream_TCP_Transport{
					UseCompression: upstreamObject.Spec.TCP.Transport.UseCompression,
					UseEncryption:  upstreamObject.Spec.TCP.Transport.UseEncryption,
				}

				if upstreamObject.Spec.TCP.Transport.ProxyURL != nil {
					upstream.TCP.Transport.ProxyURL = upstreamObject.Spec.TCP.Transport.ProxyURL
				}

				if upstreamObject.Spec.TCP.Transport.BandwdithLimit != nil {
					upstream.TCP.Transport.BandwdithLimit = &Upstream_TCP_Transport_BandwidthLimit{
						Enabled: upstreamObject.Spec.TCP.Transport.BandwdithLimit.Enabled,
						Limit:   upstreamObject.Spec.TCP.Transport.BandwdithLimit.Limit,
						Type:    upstreamObject.Spec.TCP.Transport.BandwdithLimit.Type,
					}
				}
			}

			// Handle LoadBalancer
			upstream.TCP.LoadBalancer = resolveLoadBalancer(k8sclient, clientObject.Namespace, upstreamObject.Spec.TCP.LoadBalancer)

			// Handle Plugin
			if upstreamObject.Spec.TCP.Plugin != nil {
				upstream.TCP.Plugin = &PluginConfig{
					Type:        upstreamObject.Spec.TCP.Plugin.Type,
					LocalPath:   upstreamObject.Spec.TCP.Plugin.LocalPath,
					StripPrefix: upstreamObject.Spec.TCP.Plugin.StripPrefix,
					LocalAddr:   upstreamObject.Spec.TCP.Plugin.LocalAddr,
					UnixPath:    upstreamObject.Spec.TCP.Plugin.UnixPath,
				}

				// Fetch username from secret
				if upstreamObject.Spec.TCP.Plugin.Username != nil {
					secret := &corev1.Secret{}
					err := k8sclient.Get(context.TODO(), types.NamespacedName{
						Name:      upstreamObject.Spec.TCP.Plugin.Username.Secret.Name,
						Namespace: clientObject.Namespace,
					}, secret)
					if err == nil {
						if val, ok := secret.Data[upstreamObject.Spec.TCP.Plugin.Username.Secret.Key]; ok {
							upstream.TCP.Plugin.Username = string(val)
						}
					}
				}

				// Fetch password from secret
				if upstreamObject.Spec.TCP.Plugin.Password != nil {
					secret := &corev1.Secret{}
					err := k8sclient.Get(context.TODO(), types.NamespacedName{
						Name:      upstreamObject.Spec.TCP.Plugin.Password.Secret.Name,
						Namespace: clientObject.Namespace,
					}, secret)
					if err == nil {
						if val, ok := secret.Data[upstreamObject.Spec.TCP.Plugin.Password.Secret.Key]; ok {
							upstream.TCP.Plugin.Password = string(val)
						}
					}
				}

				// Fetch HTTPUser from secret
				if upstreamObject.Spec.TCP.Plugin.HTTPUser != nil {
					secret := &corev1.Secret{}
					err := k8sclient.Get(context.TODO(), types.NamespacedName{
						Name:      upstreamObject.Spec.TCP.Plugin.HTTPUser.Secret.Name,
						Namespace: clientObject.Namespace,
					}, secret)
					if err == nil {
						if val, ok := secret.Data[upstreamObject.Spec.TCP.Plugin.HTTPUser.Secret.Key]; ok {
							upstream.TCP.Plugin.HTTPUser = string(val)
						}
					}
				}

				// Fetch HTTPPassword from secret
				if upstreamObject.Spec.TCP.Plugin.HTTPPassword != nil {
					secret := &corev1.Secret{}
					err := k8sclient.Get(context.TODO(), types.NamespacedName{
						Name:      upstreamObject.Spec.TCP.Plugin.HTTPPassword.Secret.Name,
						Namespace: clientObject.Namespace,
					}, secret)
					if err == nil {
						if val, ok := secret.Data[upstreamObject.Spec.TCP.Plugin.HTTPPassword.Secret.Key]; ok {
							upstream.TCP.Plugin.HTTPPassword = string(val)
						}
					}
				}
			}
		}

		if upstreamObject.Spec.UDP != nil {
			upstream.Type = 2
			upstream.UDP.Host = upstreamObject.Spec.UDP.Host
			upstream.UDP.Port = upstreamObject.Spec.UDP.Port
			upstream.UDP.ServerPort = upstreamObject.Spec.UDP.Server.Port

			if upstreamObject.Spec.UDP.ProxyProtocol != nil {
				upstream.UDP.ProxyProtocol = upstreamObject.Spec.UDP.ProxyProtocol
			}

			if upstreamObject.Spec.UDP.Transport != nil {
				upstream.UDP.Transport = &Upstream_UDP_Transport{
					UseCompression: upstreamObject.Spec.UDP.Transport.UseCompression,
					UseEncryption:  upstreamObject.Spec.UDP.Transport.UseEncryption,
				}

				if upstreamObject.Spec.UDP.Transport.BandwidthLimit != nil {
					upstream.UDP.Transport.BandwidthLimit = &Upstream_UDP_Transport_BandwidthLimit{
						Enabled: upstreamObject.Spec.UDP.Transport.BandwidthLimit.Enabled,
						Limit:   upstreamObject.Spec.UDP.Transport.BandwidthLimit.Limit,
						Type:    upstreamObject.Spec.UDP.Transport.BandwidthLimit.Type,
					}
				}
			}
		}

		if upstreamObject.Spec.STCP != nil {
			upstream.Type = 3
			upstream.STCP.Host = upstreamObject.Spec.STCP.Host
			upstream.STCP.Port = upstreamObject.Spec.STCP.Port

			// fetch secret key from secret
			secret := &corev1.Secret{}
			err := k8sclient.Get(context.TODO(), types.NamespacedName{Name: upstreamObject.Spec.STCP.SecretKey.Secret.Name, Namespace: clientObject.Namespace}, secret)
			if err != nil && errors.IsNotFound(err) {
				return config, err
			} else if err != nil {
				return config, err
			}
			secretKeyByte, ok := secret.Data[upstreamObject.Spec.STCP.SecretKey.Secret.Key]
			if !ok {
				return config, err
			}
			upstream.STCP.SecretKey = string(secretKeyByte)

			if upstreamObject.Spec.STCP.ProxyProtocol != nil {
				upstream.STCP.ProxyProtocol = upstreamObject.Spec.STCP.ProxyProtocol
			}

			if upstreamObject.Spec.STCP.HealthCheck != nil {
				upstream.STCP.HealthCheck = &Upstream_TCP_HealthCheck{
					TimeoutSeconds:  upstreamObject.Spec.STCP.HealthCheck.TimeoutSeconds,
					MaxFailed:       upstreamObject.Spec.STCP.HealthCheck.MaxFailed,
					IntervalSeconds: upstreamObject.Spec.STCP.HealthCheck.IntervalSeconds,
				}
			}

			if upstreamObject.Spec.STCP.Transport != nil {
				upstream.STCP.Transport = &Upstream_TCP_Transport{
					UseCompression: upstreamObject.Spec.STCP.Transport.UseCompression,
					UseEncryption:  upstreamObject.Spec.STCP.Transport.UseEncryption,
				}

				if upstreamObject.Spec.STCP.Transport.ProxyURL != nil {
					upstream.STCP.Transport.ProxyURL = upstreamObject.Spec.STCP.Transport.ProxyURL
				}

				if upstreamObject.Spec.STCP.Transport.BandwdithLimit != nil {
					upstream.STCP.Transport.BandwdithLimit = &Upstream_TCP_Transport_BandwidthLimit{
						Enabled: upstreamObject.Spec.STCP.Transport.BandwdithLimit.Enabled,
						Limit:   upstreamObject.Spec.STCP.Transport.BandwdithLimit.Limit,
						Type:    upstreamObject.Spec.STCP.Transport.BandwdithLimit.Type,
					}
				}
			}

			if len(upstreamObject.Spec.STCP.AllowUsers) > 0 {
				upstream.STCP.AllowUsers = upstreamObject.Spec.STCP.AllowUsers
			}
		}

		if upstreamObject.Spec.XTCP != nil {
			upstream.Type = 4
			upstream.XTCP.Host = upstreamObject.Spec.XTCP.Host
			upstream.XTCP.Port = upstreamObject.Spec.XTCP.Port

			// fetch secret key from secret
			secret := &corev1.Secret{}
			err := k8sclient.Get(context.TODO(), types.NamespacedName{Name: upstreamObject.Spec.XTCP.SecretKey.Secret.Name, Namespace: clientObject.Namespace}, secret)
			if err != nil && errors.IsNotFound(err) {
				return config, err
			} else if err != nil {
				return config, err
			}
			secretKeyByte, ok := secret.Data[upstreamObject.Spec.XTCP.SecretKey.Secret.Key]
			if !ok {
				return config, err
			}
			upstream.XTCP.SecretKey = string(secretKeyByte)

			if upstreamObject.Spec.XTCP.ProxyProtocol != nil {
				upstream.XTCP.ProxyProtocol = upstreamObject.Spec.XTCP.ProxyProtocol
			}

			if upstreamObject.Spec.XTCP.HealthCheck != nil {
				upstream.XTCP.HealthCheck = &Upstream_TCP_HealthCheck{
					TimeoutSeconds:  upstreamObject.Spec.XTCP.HealthCheck.TimeoutSeconds,
					MaxFailed:       upstreamObject.Spec.XTCP.HealthCheck.MaxFailed,
					IntervalSeconds: upstreamObject.Spec.XTCP.HealthCheck.IntervalSeconds,
				}
			}

			if upstreamObject.Spec.XTCP.Transport != nil {
				upstream.XTCP.Transport = &Upstream_TCP_Transport{
					UseCompression: upstreamObject.Spec.XTCP.Transport.UseCompression,
					UseEncryption:  upstreamObject.Spec.XTCP.Transport.UseEncryption,
				}

				if upstreamObject.Spec.XTCP.Transport.ProxyURL != nil {
					upstream.XTCP.Transport.ProxyURL = upstreamObject.Spec.XTCP.Transport.ProxyURL
				}

				if upstreamObject.Spec.XTCP.Transport.BandwdithLimit != nil {
					upstream.XTCP.Transport.BandwdithLimit = &Upstream_TCP_Transport_BandwidthLimit{
						Enabled: upstreamObject.Spec.XTCP.Transport.BandwdithLimit.Enabled,
						Limit:   upstreamObject.Spec.XTCP.Transport.BandwdithLimit.Limit,
						Type:    upstreamObject.Spec.XTCP.Transport.BandwdithLimit.Type,
					}
				}
			}

			if len(upstreamObject.Spec.XTCP.AllowUsers) > 0 {
				upstream.XTCP.AllowUsers = upstreamObject.Spec.XTCP.AllowUsers
			}
		}

		if upstreamObject.Spec.HTTP != nil {
			upstream.Type = 5
			upstream.HTTP.Host = upstreamObject.Spec.HTTP.Host
			upstream.HTTP.Port = upstreamObject.Spec.HTTP.Port

			if upstreamObject.Spec.HTTP.Subdomain != "" {
				upstream.HTTP.Subdomain = upstreamObject.Spec.HTTP.Subdomain
			}

			if len(upstreamObject.Spec.HTTP.CustomDomains) > 0 {
				upstream.HTTP.CustomDomains = upstreamObject.Spec.HTTP.CustomDomains
			}

			if len(upstreamObject.Spec.HTTP.Locations) > 0 {
				upstream.HTTP.Locations = upstreamObject.Spec.HTTP.Locations
			}

			if upstreamObject.Spec.HTTP.HostHeaderRewrite != "" {
				upstream.HTTP.HostHeaderRewrite = upstreamObject.Spec.HTTP.HostHeaderRewrite
			}

			if upstreamObject.Spec.HTTP.RequestHeaders != nil {
				upstream.HTTP.RequestHeaders = upstreamObject.Spec.HTTP.RequestHeaders.Set
			}

			if upstreamObject.Spec.HTTP.ResponseHeaders != nil {
				upstream.HTTP.ResponseHeaders = upstreamObject.Spec.HTTP.ResponseHeaders.Set
			}

			// Fetch HTTP user from secret
			if upstreamObject.Spec.HTTP.HTTPUser != nil {
				secret := &corev1.Secret{}
				err := k8sclient.Get(context.TODO(), types.NamespacedName{
					Name:      upstreamObject.Spec.HTTP.HTTPUser.Secret.Name,
					Namespace: clientObject.Namespace,
				}, secret)
				if err != nil {
					return config, err
				}
				val, ok := secret.Data[upstreamObject.Spec.HTTP.HTTPUser.Secret.Key]
				if !ok {
					return config, errors.NewBadRequest(fmt.Sprintf("key %s not found in secret %s",
						upstreamObject.Spec.HTTP.HTTPUser.Secret.Key,
						upstreamObject.Spec.HTTP.HTTPUser.Secret.Name))
				}
				upstream.HTTP.HTTPUser = string(val)
			}

			// Fetch HTTP password from secret
			if upstreamObject.Spec.HTTP.HTTPPassword != nil {
				secret := &corev1.Secret{}
				err := k8sclient.Get(context.TODO(), types.NamespacedName{
					Name:      upstreamObject.Spec.HTTP.HTTPPassword.Secret.Name,
					Namespace: clientObject.Namespace,
				}, secret)
				if err != nil {
					return config, err
				}
				val, ok := secret.Data[upstreamObject.Spec.HTTP.HTTPPassword.Secret.Key]
				if !ok {
					return config, errors.NewBadRequest(fmt.Sprintf("key %s not found in secret %s",
						upstreamObject.Spec.HTTP.HTTPPassword.Secret.Key,
						upstreamObject.Spec.HTTP.HTTPPassword.Secret.Name))
				}
				upstream.HTTP.HTTPPassword = string(val)
			}

			if upstreamObject.Spec.HTTP.HealthCheck != nil {
				upstream.HTTP.HealthCheck = &Upstream_HTTP_HealthCheck{
					Type:            upstreamObject.Spec.HTTP.HealthCheck.Type,
					Path:            upstreamObject.Spec.HTTP.HealthCheck.Path,
					TimeoutSeconds:  upstreamObject.Spec.HTTP.HealthCheck.TimeoutSeconds,
					IntervalSeconds: upstreamObject.Spec.HTTP.HealthCheck.IntervalSeconds,
					MaxFailed:       upstreamObject.Spec.HTTP.HealthCheck.MaxFailed,
				}
			}

			if upstreamObject.Spec.HTTP.Transport != nil {
				upstream.HTTP.Transport = &Upstream_TCP_Transport{
					UseCompression: upstreamObject.Spec.HTTP.Transport.UseCompression,
					UseEncryption:  upstreamObject.Spec.HTTP.Transport.UseEncryption,
				}

				if upstreamObject.Spec.HTTP.Transport.ProxyURL != nil {
					upstream.HTTP.Transport.ProxyURL = upstreamObject.Spec.HTTP.Transport.ProxyURL
				}

				if upstreamObject.Spec.HTTP.Transport.BandwdithLimit != nil {
					upstream.HTTP.Transport.BandwdithLimit = &Upstream_TCP_Transport_BandwidthLimit{
						Enabled: upstreamObject.Spec.HTTP.Transport.BandwdithLimit.Enabled,
						Limit:   upstreamObject.Spec.HTTP.Transport.BandwdithLimit.Limit,
						Type:    upstreamObject.Spec.HTTP.Transport.BandwdithLimit.Type,
					}
				}
			}

			upstream.HTTP.LoadBalancer = resolveLoadBalancer(k8sclient, clientObject.Namespace, upstreamObject.Spec.HTTP.LoadBalancer)
		}

		if upstreamObject.Spec.HTTPS != nil {
			upstream.Type = 6
			upstream.HTTPS.Host = upstreamObject.Spec.HTTPS.Host
			upstream.HTTPS.Port = upstreamObject.Spec.HTTPS.Port
			upstream.HTTPS.CustomDomains = upstreamObject.Spec.HTTPS.CustomDomains

			if upstreamObject.Spec.HTTPS.ProxyProtocol != nil {
				upstream.HTTPS.ProxyProtocol = upstreamObject.Spec.HTTPS.ProxyProtocol
			}

			if upstreamObject.Spec.HTTPS.Transport != nil {
				upstream.HTTPS.Transport = &Upstream_TCP_Transport{
					UseCompression: upstreamObject.Spec.HTTPS.Transport.UseCompression,
					UseEncryption:  upstreamObject.Spec.HTTPS.Transport.UseEncryption,
				}

				if upstreamObject.Spec.HTTPS.Transport.BandwdithLimit != nil {
					upstream.HTTPS.Transport.BandwdithLimit = &Upstream_TCP_Transport_BandwidthLimit{
						Enabled: upstreamObject.Spec.HTTPS.Transport.BandwdithLimit.Enabled,
						Limit:   upstreamObject.Spec.HTTPS.Transport.BandwdithLimit.Limit,
						Type:    upstreamObject.Spec.HTTPS.Transport.BandwdithLimit.Type,
					}
				}
			}

			upstream.HTTPS.LoadBalancer = resolveLoadBalancer(k8sclient, clientObject.Namespace, upstreamObject.Spec.HTTPS.LoadBalancer)
		}

		if upstreamObject.Spec.TCPMUX != nil {
			upstream.Type = 7
			upstream.TCPMUX.Host = upstreamObject.Spec.TCPMUX.Host
			upstream.TCPMUX.Port = upstreamObject.Spec.TCPMUX.Port
			upstream.TCPMUX.Multiplexer = upstreamObject.Spec.TCPMUX.Multiplexer
			upstream.TCPMUX.CustomDomains = upstreamObject.Spec.TCPMUX.CustomDomains

			if upstreamObject.Spec.TCPMUX.Transport != nil {
				upstream.TCPMUX.Transport = &Upstream_TCP_Transport{
					UseCompression: upstreamObject.Spec.TCPMUX.Transport.UseCompression,
					UseEncryption:  upstreamObject.Spec.TCPMUX.Transport.UseEncryption,
				}
			}
		}

		upstreams = append(upstreams, upstream)
	}

	visitors := []Visitor{}
	for _, visitorObject := range visitorObjects {
		visitor := Visitor{
			Name:    visitorObject.Name,
			Enabled: isEnabled(visitorObject.Spec.Enabled),
		}

		if visitorObject.Spec.STCP == nil && visitorObject.Spec.XTCP == nil {
			return config, errors.NewBadRequest("STCP, XTCP visitor is required")
		}

		if visitorObject.Spec.STCP != nil && visitorObject.Spec.XTCP != nil {
			return config, errors.NewBadRequest("Multiple protocol on the same Visitor object")
		}

		if visitorObject.Spec.STCP != nil {
			visitor.Type = 1
			visitor.STCP.Host = visitorObject.Spec.STCP.Host
			visitor.STCP.Port = visitorObject.Spec.STCP.Port
			visitor.STCP.ServerName = visitorObject.Spec.STCP.ServerName

			// fetch secret key from secret
			secret := &corev1.Secret{}
			err := k8sclient.Get(context.TODO(), types.NamespacedName{Name: visitorObject.Spec.STCP.ServerSecretKey.Secret.Name, Namespace: clientObject.Namespace}, secret)
			if err != nil && errors.IsNotFound(err) {
				return config, err
			} else if err != nil {
				return config, err
			}
			secretKeyByte, ok := secret.Data[visitorObject.Spec.STCP.ServerSecretKey.Secret.Key]
			if !ok {
				return config, err
			}
			visitor.STCP.SecretKey = string(secretKeyByte)
		}

		if visitorObject.Spec.XTCP != nil {
			visitor.Type = 2
			visitor.XTCP.Host = visitorObject.Spec.XTCP.Host
			visitor.XTCP.Port = visitorObject.Spec.XTCP.Port
			visitor.XTCP.ServerName = visitorObject.Spec.XTCP.ServerName
			visitor.XTCP.PersistantConnection = visitorObject.Spec.XTCP.PersistantConnection
			visitor.XTCP.EnableAssistedAddrs = visitorObject.Spec.XTCP.EnableAssistedAddrs

			// fetch secret key from secret
			secret := &corev1.Secret{}
			err := k8sclient.Get(context.TODO(), types.NamespacedName{Name: visitorObject.Spec.XTCP.ServerSecretKey.Secret.Name, Namespace: clientObject.Namespace}, secret)
			if err != nil && errors.IsNotFound(err) {
				return config, err
			} else if err != nil {
				return config, err
			}
			secretKeyByte, ok := secret.Data[visitorObject.Spec.XTCP.ServerSecretKey.Secret.Key]
			if !ok {
				return config, err
			}
			visitor.XTCP.SecretKey = string(secretKeyByte)

			if visitorObject.Spec.XTCP.Fallback != nil {
				visitor.XTCP.Fallback = &Visitor_XTCP_Fallback{
					ServerName: visitorObject.Spec.XTCP.Fallback.ServerName,
					Timeout:    visitorObject.Spec.XTCP.Fallback.Timeout,
				}
			}
		}

		visitors = append(visitors, visitor)
	}

	config.Upstreams = upstreams
	config.Visitors = visitors
	sort.Sort(config.Upstreams)
	sort.Sort(config.Visitors)

	return config, nil
}
