package models

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"time"

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
	DialServerTimeout    int64 // seconds; 0 leaves frpc's default
	DialServerKeepalive  int64 // seconds; 0 leaves frpc's default, -1 disables
	ConnectServerLocalIP string
	WireProtocol         string
	ProxyURL             string
}

type Common struct {
	ServerAddress        string
	ServerPort           int
	ServerProtocol       string
	ClientID             string
	User                 string
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

// Visitor types. Values start at 1 and must match the `eq $visitor.Type N` branches in
// CLIENT_TEMPLATE.
const (
	STCPVisitor VisitorType = iota + 1
	XTCPVisitor
)

type Visitor struct {
	Name     string
	Disabled bool
	Type     VisitorType
	STCP     Visitor_STCP
	XTCP     Visitor_XTCP
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
	ServerUser string
	ServerName string
	SecretKey  string
}

type Visitor_XTCP struct {
	Host                 string
	Port                 int
	ServerUser           string
	ServerName           string
	SecretKey            string
	PersistantConnection bool
	EnableAssistedAddrs  bool
	Fallback             *Visitor_XTCP_Fallback
}

type Visitor_XTCP_Fallback struct {
	ServerName string
	SecretKey  string
	Timeout    int
}

type UpstreamType int64

// Upstream types. Values start at 1 and must match the `eq $upstream.Type N` branches in
// CLIENT_TEMPLATE.
const (
	TCP UpstreamType = iota + 1
	UDP
	STCP
	XTCP
	HTTP
	HTTPS
	TCPMUX
)

type Upstream_TCPMUX struct {
	Host          string
	Port          int
	Multiplexer   string
	CustomDomains []string
	Transport     *ProxyTransport
}

type Upstream struct {
	Name     string
	Disabled bool
	Type     UpstreamType
	TCP      Upstream_TCP
	UDP      Upstream_UDP
	STCP     Upstream_STCP
	XTCP     Upstream_STCP
	HTTP     Upstream_HTTP
	HTTPS    Upstream_HTTPS
	TCPMUX   Upstream_TCPMUX
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
	Transport     *ProxyTransport
	AllowUsers    []string
}

type LoadBalancerConfig struct {
	Group    string
	GroupKey string
}

// secretValue reads one key of a Secret. A missing Secret or key is an error: frpc must never
// be configured with an empty credential that the CR explicitly references.
func secretValue(k8sclient client.Client, namespace string, ref frpv1alpha1.Secret) (string, error) {
	secret := &corev1.Secret{}
	if err := k8sclient.Get(context.TODO(), types.NamespacedName{Name: ref.Name, Namespace: namespace}, secret); err != nil {
		return "", err
	}
	value, ok := secret.Data[ref.Key]
	if !ok {
		return "", errors.NewBadRequest(fmt.Sprintf("key %q not found in secret %s/%s", ref.Key, namespace, ref.Name))
	}
	return string(value), nil
}

// optionalSecretValue is secretValue for an optional SecretRef: nil yields "".
func optionalSecretValue(k8sclient client.Client, namespace string, ref *frpv1alpha1.SecretRef) (string, error) {
	if ref == nil {
		return "", nil
	}
	return secretValue(k8sclient, namespace, ref.Secret)
}

// resolveLoadBalancer converts a CRD LoadBalancer into the model, reading groupKey from its
// Secret when set.
func resolveLoadBalancer(k8sclient client.Client, namespace string, lb *frpv1alpha1.LoadBalancer) (*LoadBalancerConfig, error) {
	if lb == nil {
		return nil, nil
	}
	groupKey, err := optionalSecretValue(k8sclient, namespace, lb.GroupKey)
	if err != nil {
		return nil, err
	}
	return &LoadBalancerConfig{Group: lb.Group, GroupKey: groupKey}, nil
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
	Transport     *ProxyTransport
	LoadBalancer  *LoadBalancerConfig
	Plugin        *PluginConfig
}

type Upstream_TCP_HealthCheck struct {
	TimeoutSeconds  int
	MaxFailed       int
	IntervalSeconds int
}

// ProxyTransport is the per-proxy transport block shared by every proxy type.
type ProxyTransport struct {
	UseCompression bool
	UseEncryption  bool
	BandwidthLimit *BandwidthLimit
}

type BandwidthLimit struct {
	Enabled bool
	Limit   int
	Type    string
}

type Upstream_UDP struct {
	Host          string
	Port          int
	ServerPort    int
	ProxyProtocol *string
	Transport     *ProxyTransport
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
	Transport         *ProxyTransport
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
	Transport     *ProxyTransport
	LoadBalancer  *LoadBalancerConfig
}

// durationSeconds converts a CRD duration ("15s", "1m", "-1s") or whole-seconds value ("15")
// into the whole seconds frpc expects. An empty value returns 0 (frpc's default).
func durationSeconds(field, value string) (int64, error) {
	if value == "" {
		return 0, nil
	}
	if seconds, err := strconv.ParseInt(value, 10, 64); err == nil {
		return seconds, nil
	}
	d, err := time.ParseDuration(value)
	if err != nil || d%time.Second != 0 {
		return 0, errors.NewBadRequest(fmt.Sprintf("%s %q must be a duration in whole seconds (e.g. \"15s\", \"1m\", \"-1s\")", field, value))
	}
	return int64(d / time.Second), nil
}

// newProxyTransport maps the CRD transport shared by TCP, STCP, XTCP, HTTP, HTTPS and TCPMUX
// upstreams into the model.
func newProxyTransport(t *frpv1alpha1.UpstreamSpec_TCP_Transport) *ProxyTransport {
	if t == nil {
		return nil
	}
	return &ProxyTransport{UseCompression: t.UseCompression, UseEncryption: t.UseEncryption, BandwidthLimit: newBandwidthLimit(t.BandwidthLimit)}
}

// newUDPProxyTransport maps the UDP upstream transport, which has the same shape.
func newUDPProxyTransport(t *frpv1alpha1.UpstreamSpec_UDP_Transport) *ProxyTransport {
	if t == nil {
		return nil
	}
	limit := (*frpv1alpha1.UpstreamSpec_TCP_Transport_BandwidthLimit)(t.BandwidthLimit)
	return &ProxyTransport{UseCompression: t.UseCompression, UseEncryption: t.UseEncryption, BandwidthLimit: newBandwidthLimit(limit)}
}

func newBandwidthLimit(l *frpv1alpha1.UpstreamSpec_TCP_Transport_BandwidthLimit) *BandwidthLimit {
	if l == nil {
		return nil
	}
	return &BandwidthLimit{Enabled: l.Enabled, Limit: l.Limit, Type: l.Type}
}

func newTCPHealthCheck(h *frpv1alpha1.UpstreamSpec_TCP_HealthCheck) *Upstream_TCP_HealthCheck {
	if h == nil {
		return nil
	}
	return &Upstream_TCP_HealthCheck{TimeoutSeconds: h.TimeoutSeconds, MaxFailed: h.MaxFailed, IntervalSeconds: h.IntervalSeconds}
}

// isEnabled interprets the optional CRD enabled flag: nil or true means enabled.
func isEnabled(enabled *bool) bool {
	return enabled == nil || *enabled
}

// validateUpstreamServerPorts checks that no two TCP/UDP upstreams use the same server port
// unless they are in the same load balancer group (which is intentional for load balancing)
func validateUpstreamServerPorts(upstreamObjects []frpv1alpha1.Upstream) error {
	// Track (port, protocol) -> {upstreamName, lbGroup} for conflict detection. frps binds TCP
	// and UDP remote ports independently, so 53/TCP and 53/UDP do not conflict.
	type portKey struct {
		port     int
		protocol string
	}
	type portInfo struct {
		upstreamName string
		lbGroup      string
	}
	serverPorts := make(map[portKey]portInfo) // (port, protocol) -> first upstream info

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

		key := portKey{port: port, protocol: protocol}
		if existing, exists := serverPorts[key]; exists {
			// Allow same port if both are in the same load balancer group
			if lbGroup != "" && existing.lbGroup == lbGroup {
				continue // Same LB group, allowed
			}
			return errors.NewBadRequest(
				fmt.Sprintf("duplicate server port %d: upstream %q (%s) conflicts with upstream %q",
					port, upstream.Name, protocol, existing.upstreamName))
		}
		serverPorts[key] = portInfo{upstreamName: upstream.Name, lbGroup: lbGroup}
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

// validateUpstreamNames rejects two upstreams that would render the same frpc proxy name.
// frpc v0.70.0+ rejects configs with duplicate proxy names, and the reconciler matches
// spec.client across namespaces, so two Upstreams named the same in different namespaces
// would otherwise collide. Disabled upstreams still count: frpc validates names before
// applying `enabled`.
func validateUpstreamNames(upstreamObjects []frpv1alpha1.Upstream) error {
	seen := make(map[string]frpv1alpha1.Upstream) // proxy name -> first upstream with that name

	for _, upstream := range upstreamObjects {
		if existing, exists := seen[upstream.Name]; exists {
			return errors.NewBadRequest(
				fmt.Sprintf("duplicate upstream name %q: %s/%s conflicts with %s/%s",
					upstream.Name, existing.Namespace, existing.Name, upstream.Namespace, upstream.Name))
		}
		seen[upstream.Name] = upstream
	}

	return nil
}

// validateVisitorNames rejects two visitors that would render the same frpc visitor name,
// including the synthetic "<name>-fallback" visitor emitted for XTCP fallbacks. Disabled
// visitors still count: frpc validates names before applying `enabled`.
func validateVisitorNames(visitorObjects []frpv1alpha1.Visitor) error {
	seen := make(map[string]frpv1alpha1.Visitor) // visitor name -> first visitor with that name

	checkAndReserve := func(name string, owner frpv1alpha1.Visitor) error {
		if existing, exists := seen[name]; exists {
			return errors.NewBadRequest(
				fmt.Sprintf("duplicate visitor name %q: %s/%s conflicts with %s/%s",
					name, existing.Namespace, existing.Name, owner.Namespace, owner.Name))
		}
		seen[name] = owner
		return nil
	}

	for _, visitor := range visitorObjects {
		if err := checkAndReserve(visitor.Name, visitor); err != nil {
			return err
		}
	}

	for _, visitor := range visitorObjects {
		if visitor.Spec.XTCP != nil && visitor.Spec.XTCP.Fallback != nil {
			if err := checkAndReserve(visitor.Name+"-fallback", visitor); err != nil {
				return err
			}
		}
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

	// Validate that no duplicate names exist for upstreams (frpc rejects duplicate proxy names)
	if err := validateUpstreamNames(upstreamObjects); err != nil {
		return Config{}, err
	}

	// Validate that no duplicate names exist for visitors, including synthetic XTCP fallback
	// visitor names (frpc rejects duplicate visitor names)
	if err := validateVisitorNames(visitorObjects); err != nil {
		return Config{}, err
	}

	config := Config{
		Common: Common{
			ServerAddress:  clientObject.Spec.Server.Host,
			ServerPort:     clientObject.Spec.Server.Port,
			ServerProtocol: "tcp",
			ClientID:       clientObject.Namespace + "/" + clientObject.Name,
			AdminAddress:   DEFAULT_ADMIN_ADDRESS,
			AdminPort:      DEFAULT_ADMIN_PORT,
			AdminUsername:  DEFAULT_ADMIN_USERNAME,
			AdminPassword:  DEFAULT_ADMIN_PASSWORD,
			STUNServer:     clientObject.Spec.Server.STUNServer,
		},
	}

	if clientObject.Spec.ClientID != nil {
		config.Common.ClientID = *clientObject.Spec.ClientID
	}
	config.Common.User = clientObject.Spec.User

	if clientObject.Spec.Server.Protocol != nil {
		config.Common.ServerProtocol = *clientObject.Spec.Server.Protocol
	}

	if clientObject.Spec.Server.AdminServer != nil {
		config.Common.AdminPort = clientObject.Spec.Server.AdminServer.Port
		config.Common.PprofEnable = clientObject.Spec.Server.AdminServer.PprofEnable

		if clientObject.Spec.Server.AdminServer.Username != nil {
			username, err := secretValue(k8sclient, clientObject.Namespace, clientObject.Spec.Server.AdminServer.Username.Secret)
			if err != nil {
				return config, err
			}
			config.Common.AdminUsername = username
		}

		if clientObject.Spec.Server.AdminServer.Password != nil {
			password, err := secretValue(k8sclient, clientObject.Namespace, clientObject.Spec.Server.AdminServer.Password.Secret)
			if err != nil {
				return config, err
			}
			config.Common.AdminPassword = password
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
		config.Common.ServerAuthentication.Type = TokenAuth

		token, err := secretValue(k8sclient, clientObject.Namespace, clientObject.Spec.Server.Authentication.Token.Secret)
		if err != nil {
			return config, err
		}
		config.Common.ServerAuthentication.Token = token
	}

	// Handle OIDC authentication
	if clientObject.Spec.Server.Authentication.OIDC != nil {
		config.Common.ServerAuthentication.Type = OIDCAuth

		clientID, err := secretValue(k8sclient, clientObject.Namespace, clientObject.Spec.Server.Authentication.OIDC.ClientID.Secret)
		if err != nil {
			return config, err
		}
		config.Common.ServerAuthentication.OIDCClientID = clientID

		clientSecret, err := secretValue(k8sclient, clientObject.Namespace, clientObject.Spec.Server.Authentication.OIDC.ClientSecret.Secret)
		if err != nil {
			return config, err
		}
		config.Common.ServerAuthentication.OIDCClientSecret = clientSecret

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
		dialServerTimeout, err := durationSeconds("dialServerTimeout", clientObject.Spec.Server.Transport.DialServerTimeout)
		if err != nil {
			return config, err
		}
		dialServerKeepalive, err := durationSeconds("dialServerKeepalive", clientObject.Spec.Server.Transport.DialServerKeepalive)
		if err != nil {
			return config, err
		}

		config.Common.Transport = &TransportConfig{
			PoolCount:            clientObject.Spec.Server.Transport.PoolCount,
			DialServerTimeout:    dialServerTimeout,
			DialServerKeepalive:  dialServerKeepalive,
			ConnectServerLocalIP: clientObject.Spec.Server.Transport.ConnectServerLocalIP,
			WireProtocol:         clientObject.Spec.Server.Transport.WireProtocol,
			ProxyURL:             clientObject.Spec.Server.Transport.ProxyURL,
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
			Name:     upstreamObject.Name,
			Disabled: !isEnabled(upstreamObject.Spec.Enabled),
		}

		protocolCount := 0
		for _, set := range []bool{
			upstreamObject.Spec.TCP != nil, upstreamObject.Spec.UDP != nil, upstreamObject.Spec.STCP != nil, upstreamObject.Spec.XTCP != nil,
			upstreamObject.Spec.HTTP != nil, upstreamObject.Spec.HTTPS != nil, upstreamObject.Spec.TCPMUX != nil,
		} {
			if set {
				protocolCount++
			}
		}
		if protocolCount == 0 {
			return config, errors.NewBadRequest("TCP, UDP, STCP, XTCP, HTTP, HTTPS, or TCPMUX upstream is required")
		}
		if protocolCount > 1 {
			return config, errors.NewBadRequest("Multiple protocol on the same Upstream object")
		}

		if upstreamObject.Spec.TCP != nil {
			if upstreamObject.Spec.TCP.Plugin == nil && upstreamObject.Spec.TCP.Port == 0 {
				return config, errors.NewBadRequest(fmt.Sprintf("TCP upstream %q needs either port or plugin", upstreamObject.Name))
			}
			upstream.Type = TCP
			upstream.TCP.Host = upstreamObject.Spec.TCP.Host
			upstream.TCP.Port = upstreamObject.Spec.TCP.Port
			upstream.TCP.ServerPort = upstreamObject.Spec.TCP.Server.Port

			if upstreamObject.Spec.TCP.ProxyProtocol != nil {
				upstream.TCP.ProxyProtocol = upstreamObject.Spec.TCP.ProxyProtocol
			}

			upstream.TCP.HealthCheck = newTCPHealthCheck(upstreamObject.Spec.TCP.HealthCheck)

			upstream.TCP.Transport = newProxyTransport(upstreamObject.Spec.TCP.Transport)

			// Handle LoadBalancer
			loadBalancer, err := resolveLoadBalancer(k8sclient, clientObject.Namespace, upstreamObject.Spec.TCP.LoadBalancer)
			if err != nil {
				return config, err
			}
			upstream.TCP.LoadBalancer = loadBalancer

			// Handle Plugin
			if upstreamObject.Spec.TCP.Plugin != nil {
				upstream.TCP.Plugin = &PluginConfig{
					Type:        upstreamObject.Spec.TCP.Plugin.Type,
					LocalPath:   upstreamObject.Spec.TCP.Plugin.LocalPath,
					StripPrefix: upstreamObject.Spec.TCP.Plugin.StripPrefix,
					LocalAddr:   upstreamObject.Spec.TCP.Plugin.LocalAddr,
					UnixPath:    upstreamObject.Spec.TCP.Plugin.UnixPath,
				}

				for _, credential := range []struct {
					ref  *frpv1alpha1.SecretRef
					dest *string
				}{
					{upstreamObject.Spec.TCP.Plugin.Username, &upstream.TCP.Plugin.Username},
					{upstreamObject.Spec.TCP.Plugin.Password, &upstream.TCP.Plugin.Password},
					{upstreamObject.Spec.TCP.Plugin.HTTPUser, &upstream.TCP.Plugin.HTTPUser},
					{upstreamObject.Spec.TCP.Plugin.HTTPPassword, &upstream.TCP.Plugin.HTTPPassword},
				} {
					value, err := optionalSecretValue(k8sclient, clientObject.Namespace, credential.ref)
					if err != nil {
						return config, err
					}
					*credential.dest = value
				}

				// http_proxy takes its credentials as httpUser/httpPassword in frpc; accept the
				// CRD's httpUser/httpPassword fields for it as well as username/password.
				if upstream.TCP.Plugin.Type == "http_proxy" {
					if upstream.TCP.Plugin.Username == "" {
						upstream.TCP.Plugin.Username = upstream.TCP.Plugin.HTTPUser
					}
					if upstream.TCP.Plugin.Password == "" {
						upstream.TCP.Plugin.Password = upstream.TCP.Plugin.HTTPPassword
					}
				}
			}
		}

		if upstreamObject.Spec.UDP != nil {
			upstream.Type = UDP
			upstream.UDP.Host = upstreamObject.Spec.UDP.Host
			upstream.UDP.Port = upstreamObject.Spec.UDP.Port
			upstream.UDP.ServerPort = upstreamObject.Spec.UDP.Server.Port

			if upstreamObject.Spec.UDP.ProxyProtocol != nil {
				upstream.UDP.ProxyProtocol = upstreamObject.Spec.UDP.ProxyProtocol
			}

			upstream.UDP.Transport = newUDPProxyTransport(upstreamObject.Spec.UDP.Transport)
		}

		if upstreamObject.Spec.STCP != nil {
			upstream.Type = STCP
			upstream.STCP.Host = upstreamObject.Spec.STCP.Host
			upstream.STCP.Port = upstreamObject.Spec.STCP.Port

			secretKey, err := secretValue(k8sclient, clientObject.Namespace, upstreamObject.Spec.STCP.SecretKey.Secret)
			if err != nil {
				return config, err
			}
			upstream.STCP.SecretKey = secretKey

			if upstreamObject.Spec.STCP.ProxyProtocol != nil {
				upstream.STCP.ProxyProtocol = upstreamObject.Spec.STCP.ProxyProtocol
			}

			upstream.STCP.HealthCheck = newTCPHealthCheck(upstreamObject.Spec.STCP.HealthCheck)

			upstream.STCP.Transport = newProxyTransport(upstreamObject.Spec.STCP.Transport)

			if len(upstreamObject.Spec.STCP.AllowUsers) > 0 {
				upstream.STCP.AllowUsers = upstreamObject.Spec.STCP.AllowUsers
			}
		}

		if upstreamObject.Spec.XTCP != nil {
			upstream.Type = XTCP
			upstream.XTCP.Host = upstreamObject.Spec.XTCP.Host
			upstream.XTCP.Port = upstreamObject.Spec.XTCP.Port

			secretKey, err := secretValue(k8sclient, clientObject.Namespace, upstreamObject.Spec.XTCP.SecretKey.Secret)
			if err != nil {
				return config, err
			}
			upstream.XTCP.SecretKey = secretKey

			if upstreamObject.Spec.XTCP.ProxyProtocol != nil {
				upstream.XTCP.ProxyProtocol = upstreamObject.Spec.XTCP.ProxyProtocol
			}

			upstream.XTCP.HealthCheck = newTCPHealthCheck(upstreamObject.Spec.XTCP.HealthCheck)

			upstream.XTCP.Transport = newProxyTransport(upstreamObject.Spec.XTCP.Transport)

			if len(upstreamObject.Spec.XTCP.AllowUsers) > 0 {
				upstream.XTCP.AllowUsers = upstreamObject.Spec.XTCP.AllowUsers
			}
		}

		if upstreamObject.Spec.HTTP != nil {
			upstream.Type = HTTP
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

			if upstreamObject.Spec.HTTP.HTTPUser != nil {
				value, err := secretValue(k8sclient, clientObject.Namespace, upstreamObject.Spec.HTTP.HTTPUser.Secret)
				if err != nil {
					return config, err
				}
				upstream.HTTP.HTTPUser = value
			}

			if upstreamObject.Spec.HTTP.HTTPPassword != nil {
				value, err := secretValue(k8sclient, clientObject.Namespace, upstreamObject.Spec.HTTP.HTTPPassword.Secret)
				if err != nil {
					return config, err
				}
				upstream.HTTP.HTTPPassword = value
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

			upstream.HTTP.Transport = newProxyTransport(upstreamObject.Spec.HTTP.Transport)

			loadBalancer, err := resolveLoadBalancer(k8sclient, clientObject.Namespace, upstreamObject.Spec.HTTP.LoadBalancer)
			if err != nil {
				return config, err
			}
			upstream.HTTP.LoadBalancer = loadBalancer
		}

		if upstreamObject.Spec.HTTPS != nil {
			upstream.Type = HTTPS
			upstream.HTTPS.Host = upstreamObject.Spec.HTTPS.Host
			upstream.HTTPS.Port = upstreamObject.Spec.HTTPS.Port
			upstream.HTTPS.CustomDomains = upstreamObject.Spec.HTTPS.CustomDomains

			if upstreamObject.Spec.HTTPS.ProxyProtocol != nil {
				upstream.HTTPS.ProxyProtocol = upstreamObject.Spec.HTTPS.ProxyProtocol
			}

			upstream.HTTPS.Transport = newProxyTransport(upstreamObject.Spec.HTTPS.Transport)

			loadBalancer, err := resolveLoadBalancer(k8sclient, clientObject.Namespace, upstreamObject.Spec.HTTPS.LoadBalancer)
			if err != nil {
				return config, err
			}
			upstream.HTTPS.LoadBalancer = loadBalancer
		}

		if upstreamObject.Spec.TCPMUX != nil {
			upstream.Type = TCPMUX
			upstream.TCPMUX.Host = upstreamObject.Spec.TCPMUX.Host
			upstream.TCPMUX.Port = upstreamObject.Spec.TCPMUX.Port
			upstream.TCPMUX.Multiplexer = upstreamObject.Spec.TCPMUX.Multiplexer
			upstream.TCPMUX.CustomDomains = upstreamObject.Spec.TCPMUX.CustomDomains

			upstream.TCPMUX.Transport = newProxyTransport(upstreamObject.Spec.TCPMUX.Transport)
		}

		upstreams = append(upstreams, upstream)
	}

	visitors := []Visitor{}
	for _, visitorObject := range visitorObjects {
		visitor := Visitor{
			Name:     visitorObject.Name,
			Disabled: !isEnabled(visitorObject.Spec.Enabled),
		}

		if visitorObject.Spec.STCP == nil && visitorObject.Spec.XTCP == nil {
			return config, errors.NewBadRequest("STCP, XTCP visitor is required")
		}

		if visitorObject.Spec.STCP != nil && visitorObject.Spec.XTCP != nil {
			return config, errors.NewBadRequest("Multiple protocol on the same Visitor object")
		}

		if visitorObject.Spec.STCP != nil {
			visitor.Type = STCPVisitor
			visitor.STCP.Host = visitorObject.Spec.STCP.Host
			visitor.STCP.Port = visitorObject.Spec.STCP.Port
			visitor.STCP.ServerUser = visitorObject.Spec.STCP.ServerUser
			visitor.STCP.ServerName = visitorObject.Spec.STCP.ServerName

			secretKey, err := secretValue(k8sclient, clientObject.Namespace, visitorObject.Spec.STCP.ServerSecretKey.Secret)
			if err != nil {
				return config, err
			}
			visitor.STCP.SecretKey = secretKey
		}

		if visitorObject.Spec.XTCP != nil {
			visitor.Type = XTCPVisitor
			visitor.XTCP.Host = visitorObject.Spec.XTCP.Host
			visitor.XTCP.Port = visitorObject.Spec.XTCP.Port
			visitor.XTCP.ServerUser = visitorObject.Spec.XTCP.ServerUser
			visitor.XTCP.ServerName = visitorObject.Spec.XTCP.ServerName
			visitor.XTCP.PersistantConnection = visitorObject.Spec.XTCP.PersistantConnection
			visitor.XTCP.EnableAssistedAddrs = visitorObject.Spec.XTCP.EnableAssistedAddrs

			secretKey, err := secretValue(k8sclient, clientObject.Namespace, visitorObject.Spec.XTCP.ServerSecretKey.Secret)
			if err != nil {
				return config, err
			}
			visitor.XTCP.SecretKey = secretKey

			if visitorObject.Spec.XTCP.Fallback != nil {
				visitor.XTCP.Fallback = &Visitor_XTCP_Fallback{
					ServerName: visitorObject.Spec.XTCP.Fallback.ServerName,
					SecretKey:  secretKey,
					Timeout:    visitorObject.Spec.XTCP.Fallback.Timeout,
				}

				if visitorObject.Spec.XTCP.Fallback.ServerSecretKey != nil {
					fallbackSecretKey, err := secretValue(k8sclient, clientObject.Namespace, visitorObject.Spec.XTCP.Fallback.ServerSecretKey.Secret)
					if err != nil {
						return config, err
					}
					visitor.XTCP.Fallback.SecretKey = fallbackSecretKey
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
