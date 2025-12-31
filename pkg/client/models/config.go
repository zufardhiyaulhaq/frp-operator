package models

import (
	"context"
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
}

type ServerAuthentication struct {
	Type  ServerAuthenticationType
	Token string
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
	Fallback             *Visitor_XTCP_Fallback
}

type Visitor_XTCP_Fallback struct {
	ServerName string
	Timeout    int
}

type UpstreamType int64

const (
	TCP  UpstreamType = iota
	UDP  UpstreamType = iota
	STCP UpstreamType = iota
	XTCP UpstreamType = iota
)

type Upstream struct {
	Name string
	Type UpstreamType
	TCP  Upstream_TCP
	UDP  Upstream_UDP
	STCP Upstream_STCP
	XTCP Upstream_STCP
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
	ProxyProtocol *string
	HealthCheck   *Upstream_TCP_HealthCheck
	Transport     *Upstream_TCP_Transport
}

type Upstream_XTCP struct {
	Host          string
	Port          int
	ProxyProtocol *string
	HealthCheck   *Upstream_TCP_HealthCheck
	Transport     *Upstream_TCP_Transport
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

func NewConfig(k8sclient client.Client,
	clientObject *frpv1alpha1.Client,
	upstreamObjects []frpv1alpha1.Upstream,
	visitorObjects []frpv1alpha1.Visitor,
) (Config, error) {
	// TODO: Add validator if more than
	// >=2 TCP/UDP upstreamObjects use the same serverPort
	// >=2 STCP/XTCP visitorObjects use the same Port
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

	upstreams := []Upstream{}
	for _, upstreamObject := range upstreamObjects {
		upstream := Upstream{
			Name: upstreamObject.Name,
		}

		if upstreamObject.Spec.TCP == nil && upstreamObject.Spec.UDP == nil && upstreamObject.Spec.STCP == nil && upstreamObject.Spec.XTCP == nil {
			return config, errors.NewBadRequest("TCP, UDP, STCP, XTCP upstream is required")
		}

		if upstreamObject.Spec.TCP != nil && upstreamObject.Spec.UDP != nil && upstreamObject.Spec.STCP != nil && upstreamObject.Spec.XTCP != nil {
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
		}

		if upstreamObject.Spec.XTCP != nil {
			upstream.Type = 4
			upstream.XTCP.Host = upstreamObject.Spec.XTCP.Host
			upstream.XTCP.Port = upstreamObject.Spec.XTCP.Port

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
