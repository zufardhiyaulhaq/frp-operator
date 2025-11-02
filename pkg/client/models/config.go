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
}

type ServerAuthentication struct {
	Type  ServerAuthenticationType
	Token string
}

type UpstreamType int64

const (
	TCP UpstreamType = iota
	UDP UpstreamType = iota
)

type Upstream struct {
	Name string
	Type UpstreamType
	TCP  Upstream_TCP
	UDP  Upstream_UDP
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

func NewConfig(k8sclient client.Client, clientObject *frpv1alpha1.Client, upstreamObjects []frpv1alpha1.Upstream) (Config, error) {
	config := Config{
		Common: Common{
			ServerAddress:  clientObject.Spec.Server.Host,
			ServerPort:     clientObject.Spec.Server.Port,
			ServerProtocol: "TCP",
			AdminAddress:   DEFAULT_ADMIN_ADDRESS,
			AdminPort:      DEFAULT_ADMIN_PORT,
			AdminUsername:  DEFAULT_ADMIN_USERNAME,
			AdminPassword:  DEFAULT_ADMIN_PASSWORD,
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

		if upstreamObject.Spec.TCP == nil && upstreamObject.Spec.UDP == nil {
			return config, errors.NewBadRequest("TCP or UDP upstream is required")
		}

		if upstreamObject.Spec.TCP != nil && upstreamObject.Spec.UDP != nil {
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

		upstreams = append(upstreams, upstream)
	}
	config.Upstreams = upstreams
	sort.Sort(config.Upstreams)

	return config, nil
}
