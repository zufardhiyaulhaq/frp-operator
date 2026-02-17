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
	Token ServerAuthenticationType = iota
)

type Config struct {
	Common    Common
	Upstreams Upstreams
	Visitors  Visitors
}

type Common struct {
	ServerAddress        string
	ServerPort           int
	ServerProtocol       string
	ServerAuthentication ServerAuthentication
	AdminAddress         string
	AdminPort            int
	AdminUsername        string
	AdminPassword        string
	STUNServer           *string
	PprofEnable          bool
	TLS                  *TLSConfig
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
	Name string
	Type VisitorType
	STCP Visitor_STCP
	XTCP Visitor_XTCP
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
	TCP   UpstreamType = iota
	UDP   UpstreamType = iota
	STCP  UpstreamType = iota
	XTCP  UpstreamType = iota
	HTTP  UpstreamType = iota
	HTTPS UpstreamType = iota
)

type Upstream struct {
	Name  string
	Type  UpstreamType
	TCP   Upstream_TCP
	UDP   Upstream_UDP
	STCP  Upstream_STCP
	XTCP  Upstream_STCP
	HTTP  Upstream_HTTP
	HTTPS Upstream_HTTPS
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

type Upstream_TCP struct {
	Host          string
	Port          int
	ServerPort    int
	ProxyProtocol *string
	HealthCheck   *Upstream_TCP_HealthCheck
	Transport     *Upstream_TCP_Transport
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
	Host       string
	Port       int
	ServerPort int
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
}

// validateUpstreamServerPorts checks that no two TCP/UDP upstreams use the same server port
func validateUpstreamServerPorts(upstreamObjects []frpv1alpha1.Upstream) error {
	serverPorts := make(map[int]string) // port -> upstream name

	for _, upstream := range upstreamObjects {
		var port int
		var protocol string

		if upstream.Spec.TCP != nil {
			port = upstream.Spec.TCP.Server.Port
			protocol = "TCP"
		} else if upstream.Spec.UDP != nil {
			port = upstream.Spec.UDP.Server.Port
			protocol = "UDP"
		} else {
			continue // STCP/XTCP don't have server ports
		}

		if existingName, exists := serverPorts[port]; exists {
			return errors.NewBadRequest(
				fmt.Sprintf("duplicate server port %d: upstream %q (%s) conflicts with upstream %q",
					port, upstream.Name, protocol, existingName))
		}
		serverPorts[port] = upstream.Name
	}

	return nil
}

// validateVisitorPorts checks that no two STCP/XTCP visitors use the same port
func validateVisitorPorts(visitorObjects []frpv1alpha1.Visitor) error {
	visitorPorts := make(map[int]string) // port -> visitor name

	for _, visitor := range visitorObjects {
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

	upstreams := []Upstream{}
	for _, upstreamObject := range upstreamObjects {
		upstream := Upstream{
			Name: upstreamObject.Name,
		}

		if upstreamObject.Spec.TCP == nil && upstreamObject.Spec.UDP == nil && upstreamObject.Spec.STCP == nil && upstreamObject.Spec.XTCP == nil && upstreamObject.Spec.HTTP == nil && upstreamObject.Spec.HTTPS == nil {
			return config, errors.NewBadRequest("TCP, UDP, STCP, XTCP, HTTP, or HTTPS upstream is required")
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
		}

		if upstreamObject.Spec.UDP != nil {
			upstream.Type = 2
			upstream.UDP.Host = upstreamObject.Spec.UDP.Host
			upstream.UDP.Port = upstreamObject.Spec.UDP.Port
			upstream.UDP.ServerPort = upstreamObject.Spec.UDP.Server.Port
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
		}

		upstreams = append(upstreams, upstream)
	}

	visitors := []Visitor{}
	for _, visitorObject := range visitorObjects {
		visitor := Visitor{
			Name: visitorObject.Name,
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
