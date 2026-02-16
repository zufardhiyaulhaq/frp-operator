package builder

import (
	"strings"
	"testing"

	"github.com/zufardhiyaulhaq/frp-operator/pkg/client/models"
)

func TestConfigurationBuilder_Build(t *testing.T) {
	tests := []struct {
		name           string
		config         models.Config
		wantErr        bool
		wantContains   []string
		wantNotContain []string
	}{
		{
			name: "basic common config without authentication",
			config: models.Config{
				Common: models.Common{
					ServerAddress: "frp.example.com",
					ServerPort:    7000,
					AdminAddress:  "0.0.0.0",
					AdminPort:     7400,
					AdminUsername: "admin",
					AdminPassword: "secret",
				},
			},
			wantErr: false,
			wantContains: []string{
				`serverAddr = "frp.example.com"`,
				`serverPort = 7000`,
				`webServer.addr = "0.0.0.0"`,
				`webServer.port = 7400`,
				`webServer.user = "admin"`,
				`webServer.password = "secret"`,
			},
			wantNotContain: []string{
				`auth.method`,
				`auth.token`,
				`natHoleStunServer`,
			},
		},
		{
			name: "common config with token authentication",
			config: models.Config{
				Common: models.Common{
					ServerAddress: "frp.example.com",
					ServerPort:    7000,
					ServerAuthentication: models.ServerAuthentication{
						Type:  1,
						Token: "my-secret-token",
					},
					AdminAddress:  "0.0.0.0",
					AdminPort:     7400,
					AdminUsername: "admin",
					AdminPassword: "secret",
				},
			},
			wantErr: false,
			wantContains: []string{
				`serverAddr = "frp.example.com"`,
				`auth.method = "token"`,
				`auth.token = "my-secret-token"`,
			},
		},
		{
			name: "common config with STUN server",
			config: models.Config{
				Common: models.Common{
					ServerAddress: "frp.example.com",
					ServerPort:    7000,
					AdminAddress:  "0.0.0.0",
					AdminPort:     7400,
					AdminUsername: "admin",
					AdminPassword: "secret",
					STUNServer:    stringPtr("stun.example.com:3478"),
				},
			},
			wantErr: false,
			wantContains: []string{
				`natHoleStunServer = "stun.example.com:3478"`,
			},
		},
		{
			name: "TCP upstream - basic",
			config: models.Config{
				Common: basicCommon(),
				Upstreams: []models.Upstream{
					{
						Name: "my-tcp-service",
						Type: 1, // TCP
						TCP: models.Upstream_TCP{
							Host:       "127.0.0.1",
							Port:       8080,
							ServerPort: 18080,
						},
					},
				},
			},
			wantErr: false,
			wantContains: []string{
				`[[proxies]]`,
				`name = "my-tcp-service"`,
				`type = "tcp"`,
				`localIP = "127.0.0.1"`,
				`localPort = 8080`,
				`remotePort = 18080`,
			},
			wantNotContain: []string{
				`transport.proxyProtocolVersion`,
				`healthCheck.type`,
				`transport.useEncryption`,
			},
		},
		{
			name: "TCP upstream with proxy protocol",
			config: models.Config{
				Common: basicCommon(),
				Upstreams: []models.Upstream{
					{
						Name: "tcp-with-proxy-protocol",
						Type: 1,
						TCP: models.Upstream_TCP{
							Host:          "127.0.0.1",
							Port:          8080,
							ServerPort:    18080,
							ProxyProtocol: stringPtr("v2"),
						},
					},
				},
			},
			wantErr: false,
			wantContains: []string{
				`name = "tcp-with-proxy-protocol"`,
				`type = "tcp"`,
				`transport.proxyProtocolVersion = "v2"`,
			},
		},
		{
			name: "TCP upstream with health check",
			config: models.Config{
				Common: basicCommon(),
				Upstreams: []models.Upstream{
					{
						Name: "tcp-with-healthcheck",
						Type: 1,
						TCP: models.Upstream_TCP{
							Host:       "127.0.0.1",
							Port:       8080,
							ServerPort: 18080,
							HealthCheck: &models.Upstream_TCP_HealthCheck{
								TimeoutSeconds:  5,
								MaxFailed:       3,
								IntervalSeconds: 10,
							},
						},
					},
				},
			},
			wantErr: false,
			wantContains: []string{
				`name = "tcp-with-healthcheck"`,
				`healthCheck.type = "tcp"`,
				`healthCheck.timeoutSeconds = 5`,
				`healthCheck.maxFailed = 3`,
				`healthCheck.intervalSeconds = 10`,
			},
		},
		{
			name: "TCP upstream with transport options",
			config: models.Config{
				Common: basicCommon(),
				Upstreams: []models.Upstream{
					{
						Name: "tcp-with-transport",
						Type: 1,
						TCP: models.Upstream_TCP{
							Host:       "127.0.0.1",
							Port:       8080,
							ServerPort: 18080,
							Transport: &models.Upstream_TCP_Transport{
								UseEncryption:  true,
								UseCompression: true,
							},
						},
					},
				},
			},
			wantErr: false,
			wantContains: []string{
				`name = "tcp-with-transport"`,
				`transport.useEncryption = true`,
				`transport.useCompression = true`,
			},
		},
		{
			name: "TCP upstream with bandwidth limit",
			config: models.Config{
				Common: basicCommon(),
				Upstreams: []models.Upstream{
					{
						Name: "tcp-with-bandwidth",
						Type: 1,
						TCP: models.Upstream_TCP{
							Host:       "127.0.0.1",
							Port:       8080,
							ServerPort: 18080,
							Transport: &models.Upstream_TCP_Transport{
								UseEncryption:  false,
								UseCompression: false,
								BandwdithLimit: &models.Upstream_TCP_Transport_BandwidthLimit{
									Enabled: true,
									Limit:   100,
									Type:    "MB",
								},
							},
						},
					},
				},
			},
			wantErr: false,
			wantContains: []string{
				`name = "tcp-with-bandwidth"`,
				`transport.bandwidthLimit = "100MB"`,
				`transport.bandwidthLimitMode = "client"`,
			},
		},
		{
			name: "TCP upstream with bandwidth limit disabled",
			config: models.Config{
				Common: basicCommon(),
				Upstreams: []models.Upstream{
					{
						Name: "tcp-bandwidth-disabled",
						Type: 1,
						TCP: models.Upstream_TCP{
							Host:       "127.0.0.1",
							Port:       8080,
							ServerPort: 18080,
							Transport: &models.Upstream_TCP_Transport{
								UseEncryption:  false,
								UseCompression: false,
								BandwdithLimit: &models.Upstream_TCP_Transport_BandwidthLimit{
									Enabled: false,
									Limit:   100,
									Type:    "MB",
								},
							},
						},
					},
				},
			},
			wantErr: false,
			wantNotContain: []string{
				`transport.bandwidthLimit`,
				`transport.bandwidthLimitMode`,
			},
		},
		{
			name: "TCP upstream with proxy URL",
			config: models.Config{
				Common: basicCommon(),
				Upstreams: []models.Upstream{
					{
						Name: "tcp-with-proxy-url",
						Type: 1,
						TCP: models.Upstream_TCP{
							Host:       "127.0.0.1",
							Port:       8080,
							ServerPort: 18080,
							Transport: &models.Upstream_TCP_Transport{
								UseEncryption:  false,
								UseCompression: false,
								ProxyURL:       stringPtr("http://proxy.example.com:8080"),
							},
						},
					},
				},
			},
			wantErr: false,
			wantContains: []string{
				`transport.proxyURL = "http://proxy.example.com:8080"`,
			},
		},
		{
			name: "TCP upstream with all options",
			config: models.Config{
				Common: basicCommon(),
				Upstreams: []models.Upstream{
					{
						Name: "tcp-full",
						Type: 1,
						TCP: models.Upstream_TCP{
							Host:          "127.0.0.1",
							Port:          8080,
							ServerPort:    18080,
							ProxyProtocol: stringPtr("v1"),
							HealthCheck: &models.Upstream_TCP_HealthCheck{
								TimeoutSeconds:  3,
								MaxFailed:       5,
								IntervalSeconds: 15,
							},
							Transport: &models.Upstream_TCP_Transport{
								UseEncryption:  true,
								UseCompression: true,
								BandwdithLimit: &models.Upstream_TCP_Transport_BandwidthLimit{
									Enabled: true,
									Limit:   50,
									Type:    "KB",
								},
								ProxyURL: stringPtr("socks5://proxy:1080"),
							},
						},
					},
				},
			},
			wantErr: false,
			wantContains: []string{
				`name = "tcp-full"`,
				`type = "tcp"`,
				`localIP = "127.0.0.1"`,
				`localPort = 8080`,
				`remotePort = 18080`,
				`transport.proxyProtocolVersion = "v1"`,
				`healthCheck.type = "tcp"`,
				`healthCheck.timeoutSeconds = 3`,
				`healthCheck.maxFailed = 5`,
				`healthCheck.intervalSeconds = 15`,
				`transport.useEncryption = true`,
				`transport.useCompression = true`,
				`transport.bandwidthLimit = "50KB"`,
				`transport.bandwidthLimitMode = "client"`,
				`transport.proxyURL = "socks5://proxy:1080"`,
			},
		},
		{
			name: "UDP upstream",
			config: models.Config{
				Common: basicCommon(),
				Upstreams: []models.Upstream{
					{
						Name: "my-udp-service",
						Type: 2, // UDP
						UDP: models.Upstream_UDP{
							Host:       "127.0.0.1",
							Port:       53,
							ServerPort: 5353,
						},
					},
				},
			},
			wantErr: false,
			wantContains: []string{
				`[[proxies]]`,
				`name = "my-udp-service"`,
				`type = "udp"`,
				`localIP = "127.0.0.1"`,
				`localPort = 53`,
				`remotePort = 5353`,
			},
		},
		{
			name: "STCP upstream - basic",
			config: models.Config{
				Common: basicCommon(),
				Upstreams: []models.Upstream{
					{
						Name: "my-stcp-service",
						Type: 3, // STCP
						STCP: models.Upstream_STCP{
							Host:      "127.0.0.1",
							Port:      22,
							SecretKey: "stcp-secret-key",
						},
					},
				},
			},
			wantErr: false,
			wantContains: []string{
				`[[proxies]]`,
				`name = "my-stcp-service"`,
				`type = "stcp"`,
				`localIP = "127.0.0.1"`,
				`localPort = 22`,
				`secretKey = "stcp-secret-key"`,
			},
		},
		{
			name: "STCP upstream with proxy protocol",
			config: models.Config{
				Common: basicCommon(),
				Upstreams: []models.Upstream{
					{
						Name: "stcp-with-proxy-protocol",
						Type: 3,
						STCP: models.Upstream_STCP{
							Host:          "127.0.0.1",
							Port:          22,
							SecretKey:     "secret",
							ProxyProtocol: stringPtr("v2"),
						},
					},
				},
			},
			wantErr: false,
			wantContains: []string{
				`name = "stcp-with-proxy-protocol"`,
				`type = "stcp"`,
				`transport.proxyProtocolVersion = "v2"`,
			},
		},
		{
			name: "STCP upstream with health check",
			config: models.Config{
				Common: basicCommon(),
				Upstreams: []models.Upstream{
					{
						Name: "stcp-with-healthcheck",
						Type: 3,
						STCP: models.Upstream_STCP{
							Host:      "127.0.0.1",
							Port:      22,
							SecretKey: "secret",
							HealthCheck: &models.Upstream_TCP_HealthCheck{
								TimeoutSeconds:  5,
								MaxFailed:       3,
								IntervalSeconds: 10,
							},
						},
					},
				},
			},
			wantErr: false,
			wantContains: []string{
				`name = "stcp-with-healthcheck"`,
				`healthCheck.type = "tcp"`,
				`healthCheck.timeoutSeconds = 5`,
				`healthCheck.maxFailed = 3`,
				`healthCheck.intervalSeconds = 10`,
			},
		},
		{
			name: "STCP upstream with transport options",
			config: models.Config{
				Common: basicCommon(),
				Upstreams: []models.Upstream{
					{
						Name: "stcp-with-transport",
						Type: 3,
						STCP: models.Upstream_STCP{
							Host:      "127.0.0.1",
							Port:      22,
							SecretKey: "secret",
							Transport: &models.Upstream_TCP_Transport{
								UseEncryption:  true,
								UseCompression: true,
								BandwdithLimit: &models.Upstream_TCP_Transport_BandwidthLimit{
									Enabled: true,
									Limit:   200,
									Type:    "KB",
								},
								ProxyURL: stringPtr("http://proxy:3128"),
							},
						},
					},
				},
			},
			wantErr: false,
			wantContains: []string{
				`name = "stcp-with-transport"`,
				`transport.useEncryption = true`,
				`transport.useCompression = true`,
				`transport.bandwidthLimit = "200KB"`,
				`transport.bandwidthLimitMode = "client"`,
				`transport.proxyURL = "http://proxy:3128"`,
			},
		},
		{
			name: "XTCP upstream - basic",
			config: models.Config{
				Common: basicCommon(),
				Upstreams: []models.Upstream{
					{
						Name: "my-xtcp-service",
						Type: 4, // XTCP
						XTCP: models.Upstream_STCP{
							Host:      "127.0.0.1",
							Port:      3389,
							SecretKey: "xtcp-secret-key",
						},
					},
				},
			},
			wantErr: false,
			wantContains: []string{
				`[[proxies]]`,
				`name = "my-xtcp-service"`,
				`type = "xtcp"`,
				`localIP = "127.0.0.1"`,
				`localPort = 3389`,
				`secretKey = "xtcp-secret-key"`,
			},
		},
		{
			name: "XTCP upstream with proxy protocol",
			config: models.Config{
				Common: basicCommon(),
				Upstreams: []models.Upstream{
					{
						Name: "xtcp-with-proxy-protocol",
						Type: 4,
						XTCP: models.Upstream_STCP{
							Host:          "127.0.0.1",
							Port:          3389,
							SecretKey:     "secret",
							ProxyProtocol: stringPtr("v1"),
						},
					},
				},
			},
			wantErr: false,
			wantContains: []string{
				`name = "xtcp-with-proxy-protocol"`,
				`type = "xtcp"`,
				`transport.proxyProtocolVersion = "v1"`,
			},
		},
		{
			name: "XTCP upstream with health check",
			config: models.Config{
				Common: basicCommon(),
				Upstreams: []models.Upstream{
					{
						Name: "xtcp-with-healthcheck",
						Type: 4,
						XTCP: models.Upstream_STCP{
							Host:      "127.0.0.1",
							Port:      3389,
							SecretKey: "secret",
							HealthCheck: &models.Upstream_TCP_HealthCheck{
								TimeoutSeconds:  10,
								MaxFailed:       5,
								IntervalSeconds: 30,
							},
						},
					},
				},
			},
			wantErr: false,
			wantContains: []string{
				`name = "xtcp-with-healthcheck"`,
				`healthCheck.type = "tcp"`,
				`healthCheck.timeoutSeconds = 10`,
				`healthCheck.maxFailed = 5`,
				`healthCheck.intervalSeconds = 30`,
			},
		},
		{
			name: "XTCP upstream with transport options",
			config: models.Config{
				Common: basicCommon(),
				Upstreams: []models.Upstream{
					{
						Name: "xtcp-with-transport",
						Type: 4,
						XTCP: models.Upstream_STCP{
							Host:      "127.0.0.1",
							Port:      3389,
							SecretKey: "secret",
							Transport: &models.Upstream_TCP_Transport{
								UseEncryption:  true,
								UseCompression: false,
								BandwdithLimit: &models.Upstream_TCP_Transport_BandwidthLimit{
									Enabled: true,
									Limit:   500,
									Type:    "MB",
								},
								ProxyURL: stringPtr("socks5://proxy:1080"),
							},
						},
					},
				},
			},
			wantErr: false,
			wantContains: []string{
				`name = "xtcp-with-transport"`,
				`transport.useEncryption = true`,
				`transport.useCompression = false`,
				`transport.bandwidthLimit = "500MB"`,
				`transport.bandwidthLimitMode = "client"`,
				`transport.proxyURL = "socks5://proxy:1080"`,
			},
		},
		{
			name: "STCP visitor",
			config: models.Config{
				Common: basicCommon(),
				Visitors: []models.Visitor{
					{
						Name: "my-stcp-visitor",
						Type: 1, // STCPVisitor
						STCP: models.Visitor_STCP{
							Host:       "127.0.0.1",
							Port:       2222,
							ServerName: "remote-ssh-service",
							SecretKey:  "visitor-secret",
						},
					},
				},
			},
			wantErr: false,
			wantContains: []string{
				`[[visitors]]`,
				`name = "my-stcp-visitor"`,
				`type = "stcp"`,
				`serverName = "remote-ssh-service"`,
				`secretKey = "visitor-secret"`,
				`bindAddr = "127.0.0.1"`,
				`bindPort = 2222`,
			},
		},
		{
			name: "XTCP visitor without fallback",
			config: models.Config{
				Common: basicCommon(),
				Visitors: []models.Visitor{
					{
						Name: "my-xtcp-visitor",
						Type: 2, // XTCPVisitor
						XTCP: models.Visitor_XTCP{
							Host:                 "0.0.0.0",
							Port:                 3390,
							ServerName:           "remote-rdp-service",
							SecretKey:            "xtcp-visitor-secret",
							PersistantConnection: true,
						},
					},
				},
			},
			wantErr: false,
			wantContains: []string{
				`[[visitors]]`,
				`name = "my-xtcp-visitor"`,
				`type = "xtcp"`,
				`serverName = "remote-rdp-service"`,
				`secretKey = "xtcp-visitor-secret"`,
				`bindAddr = "0.0.0.0"`,
				`bindPort = 3390`,
				`keepTunnelOpen = true`,
				`natHoleStun.disableAssistedAddrs = true`,
			},
			wantNotContain: []string{
				`fallbackTo`,
				`fallbackTimeoutMs`,
			},
		},
		{
			name: "XTCP visitor with enableAssistedAddrs",
			config: models.Config{
				Common: basicCommon(),
				Visitors: []models.Visitor{
					{
						Name: "xtcp-visitor-assisted",
						Type: 2,
						XTCP: models.Visitor_XTCP{
							Host:                 "0.0.0.0",
							Port:                 3391,
							ServerName:           "remote-service",
							SecretKey:            "secret",
							PersistantConnection: true,
							EnableAssistedAddrs:  true,
						},
					},
				},
			},
			wantErr: false,
			wantContains: []string{
				`name = "xtcp-visitor-assisted"`,
				`type = "xtcp"`,
				`keepTunnelOpen = true`,
			},
			wantNotContain: []string{
				`natHoleStun.disableAssistedAddrs`,
			},
		},
		{
			name: "XTCP visitor with fallback",
			config: models.Config{
				Common: basicCommon(),
				Visitors: []models.Visitor{
					{
						Name: "my-xtcp-visitor-with-fallback",
						Type: 2,
						XTCP: models.Visitor_XTCP{
							Host:                 "0.0.0.0",
							Port:                 3390,
							ServerName:           "remote-rdp-service",
							SecretKey:            "xtcp-visitor-secret",
							PersistantConnection: false,
							Fallback: &models.Visitor_XTCP_Fallback{
								ServerName: "fallback-stcp-service",
								Timeout:    5000,
							},
						},
					},
				},
			},
			wantErr: false,
			wantContains: []string{
				`[[visitors]]`,
				`name = "my-xtcp-visitor-with-fallback"`,
				`type = "xtcp"`,
				`serverName = "remote-rdp-service"`,
				`keepTunnelOpen = false`,
				`natHoleStun.disableAssistedAddrs = true`,
				`fallbackTo = "my-xtcp-visitor-with-fallback-fallback"`,
				`fallbackTimeoutMs = 5000`,
				`name = "my-xtcp-visitor-with-fallback-fallback"`,
				`type = "stcp"`,
				`serverName = "fallback-stcp-service"`,
				`bindPort = -1`,
			},
		},
		{
			name: "multiple upstreams",
			config: models.Config{
				Common: basicCommon(),
				Upstreams: []models.Upstream{
					{
						Name: "tcp-service",
						Type: 1,
						TCP: models.Upstream_TCP{
							Host:       "127.0.0.1",
							Port:       80,
							ServerPort: 8080,
						},
					},
					{
						Name: "udp-service",
						Type: 2,
						UDP: models.Upstream_UDP{
							Host:       "127.0.0.1",
							Port:       53,
							ServerPort: 5353,
						},
					},
					{
						Name: "stcp-service",
						Type: 3,
						STCP: models.Upstream_STCP{
							Host:      "127.0.0.1",
							Port:      22,
							SecretKey: "ssh-secret",
						},
					},
				},
			},
			wantErr: false,
			wantContains: []string{
				`name = "tcp-service"`,
				`type = "tcp"`,
				`name = "udp-service"`,
				`type = "udp"`,
				`name = "stcp-service"`,
				`type = "stcp"`,
			},
		},
		{
			name: "multiple visitors",
			config: models.Config{
				Common: basicCommon(),
				Visitors: []models.Visitor{
					{
						Name: "stcp-visitor",
						Type: 1,
						STCP: models.Visitor_STCP{
							Host:       "127.0.0.1",
							Port:       2222,
							ServerName: "ssh-server",
							SecretKey:  "stcp-secret",
						},
					},
					{
						Name: "xtcp-visitor",
						Type: 2,
						XTCP: models.Visitor_XTCP{
							Host:                 "127.0.0.1",
							Port:                 3390,
							ServerName:           "rdp-server",
							SecretKey:            "xtcp-secret",
							PersistantConnection: true,
						},
					},
				},
			},
			wantErr: false,
			wantContains: []string{
				`name = "stcp-visitor"`,
				`type = "stcp"`,
				`serverName = "ssh-server"`,
				`name = "xtcp-visitor"`,
				`type = "xtcp"`,
				`serverName = "rdp-server"`,
				`keepTunnelOpen = true`,
			},
		},
		{
			name: "empty upstreams and visitors",
			config: models.Config{
				Common:    basicCommon(),
				Upstreams: []models.Upstream{},
				Visitors:  []models.Visitor{},
			},
			wantErr: false,
			wantContains: []string{
				`serverAddr = "frp.example.com"`,
				`serverPort = 7000`,
			},
			wantNotContain: []string{
				`[[proxies]]`,
				`[[visitors]]`,
			},
		},
		{
			name: "config with upstreams and visitors combined",
			config: models.Config{
				Common: models.Common{
					ServerAddress: "frp.example.com",
					ServerPort:    7000,
					ServerAuthentication: models.ServerAuthentication{
						Type:  1,
						Token: "auth-token",
					},
					AdminAddress:  "0.0.0.0",
					AdminPort:     7400,
					AdminUsername: "admin",
					AdminPassword: "password",
					STUNServer:    stringPtr("stun.example.com:3478"),
				},
				Upstreams: []models.Upstream{
					{
						Name: "web-service",
						Type: 1,
						TCP: models.Upstream_TCP{
							Host:       "web.local",
							Port:       80,
							ServerPort: 8080,
						},
					},
				},
				Visitors: []models.Visitor{
					{
						Name: "remote-access",
						Type: 1,
						STCP: models.Visitor_STCP{
							Host:       "127.0.0.1",
							Port:       8888,
							ServerName: "remote-server",
							SecretKey:  "access-key",
						},
					},
				},
			},
			wantErr: false,
			wantContains: []string{
				`serverAddr = "frp.example.com"`,
				`auth.method = "token"`,
				`auth.token = "auth-token"`,
				`natHoleStunServer = "stun.example.com:3478"`,
				`[[proxies]]`,
				`name = "web-service"`,
				`type = "tcp"`,
				`localIP = "web.local"`,
				`[[visitors]]`,
				`name = "remote-access"`,
				`type = "stcp"`,
				`serverName = "remote-server"`,
			},
		},
		{
			name: "HTTP upstream - basic with subdomain",
			config: models.Config{
				Common: basicCommon(),
				Upstreams: []models.Upstream{
					{
						Name: "my-http-service",
						Type: 5, // HTTP
						HTTP: models.Upstream_HTTP{
							Host:      "web-service.default.svc",
							Port:      8080,
							Subdomain: "webapp",
						},
					},
				},
			},
			wantErr: false,
			wantContains: []string{
				`[[proxies]]`,
				`name = "my-http-service"`,
				`type = "http"`,
				`localIP = "web-service.default.svc"`,
				`localPort = 8080`,
				`subdomain = "webapp"`,
			},
			wantNotContain: []string{
				`customDomains`,
				`locations`,
				`hostHeaderRewrite`,
			},
		},
		{
			name: "HTTP upstream - with custom domains",
			config: models.Config{
				Common: basicCommon(),
				Upstreams: []models.Upstream{
					{
						Name: "http-custom-domains",
						Type: 5,
						HTTP: models.Upstream_HTTP{
							Host:          "api-service.default.svc",
							Port:          8080,
							CustomDomains: []string{"api.example.com", "api2.example.com"},
						},
					},
				},
			},
			wantErr: false,
			wantContains: []string{
				`name = "http-custom-domains"`,
				`type = "http"`,
				`customDomains = ["api.example.com", "api2.example.com"]`,
			},
		},
		{
			name: "HTTP upstream - with locations and headers",
			config: models.Config{
				Common: basicCommon(),
				Upstreams: []models.Upstream{
					{
						Name: "http-full",
						Type: 5,
						HTTP: models.Upstream_HTTP{
							Host:              "api-service.default.svc",
							Port:              8080,
							Subdomain:         "api",
							Locations:         []string{"/v1", "/v2"},
							HostHeaderRewrite: "internal-api.local",
							RequestHeaders:    map[string]string{"X-Forwarded-By": "frp-operator"},
							ResponseHeaders:   map[string]string{"X-Frame-Options": "DENY"},
						},
					},
				},
			},
			wantErr: false,
			wantContains: []string{
				`name = "http-full"`,
				`subdomain = "api"`,
				`locations = ["/v1", "/v2"]`,
				`hostHeaderRewrite = "internal-api.local"`,
				`requestHeaders.set.X-Forwarded-By = "frp-operator"`,
				`responseHeaders.set.X-Frame-Options = "DENY"`,
			},
		},
		{
			name: "HTTP upstream - with auth and health check",
			config: models.Config{
				Common: basicCommon(),
				Upstreams: []models.Upstream{
					{
						Name: "http-auth",
						Type: 5,
						HTTP: models.Upstream_HTTP{
							Host:         "protected.default.svc",
							Port:         8080,
							Subdomain:    "admin",
							HTTPUser:     "admin",
							HTTPPassword: "secret123",
							HealthCheck: &models.Upstream_HTTP_HealthCheck{
								Type:            "http",
								Path:            "/health",
								TimeoutSeconds:  5,
								IntervalSeconds: 10,
								MaxFailed:       3,
							},
						},
					},
				},
			},
			wantErr: false,
			wantContains: []string{
				`name = "http-auth"`,
				`httpUser = "admin"`,
				`httpPassword = "secret123"`,
				`healthCheck.type = "http"`,
				`healthCheck.path = "/health"`,
				`healthCheck.timeoutSeconds = 5`,
			},
		},
		{
			name: "HTTPS upstream - basic",
			config: models.Config{
				Common: basicCommon(),
				Upstreams: []models.Upstream{
					{
						Name: "my-https-service",
						Type: 6, // HTTPS
						HTTPS: models.Upstream_HTTPS{
							Host:          "secure-app.default.svc",
							Port:          443,
							CustomDomains: []string{"secure.example.com"},
						},
					},
				},
			},
			wantErr: false,
			wantContains: []string{
				`[[proxies]]`,
				`name = "my-https-service"`,
				`type = "https"`,
				`localIP = "secure-app.default.svc"`,
				`localPort = 443`,
				`customDomains = ["secure.example.com"]`,
			},
		},
		{
			name: "HTTPS upstream - with proxy protocol",
			config: models.Config{
				Common: basicCommon(),
				Upstreams: []models.Upstream{
					{
						Name: "https-proxy-protocol",
						Type: 6,
						HTTPS: models.Upstream_HTTPS{
							Host:          "app.default.svc",
							Port:          443,
							CustomDomains: []string{"app.example.com"},
							ProxyProtocol: stringPtr("v2"),
						},
					},
				},
			},
			wantErr: false,
			wantContains: []string{
				`name = "https-proxy-protocol"`,
				`type = "https"`,
				`transport.proxyProtocolVersion = "v2"`,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder := NewConfigurationBuilder().SetConfig(tt.config)
			result, err := builder.Build()

			if tt.wantErr {
				if err == nil {
					t.Errorf("ConfigurationBuilder.Build() expected error but got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("ConfigurationBuilder.Build() unexpected error = %v", err)
				return
			}

			for _, want := range tt.wantContains {
				if !strings.Contains(result, want) {
					t.Errorf("ConfigurationBuilder.Build() result missing expected content: %q\nGot:\n%s", want, result)
				}
			}

			for _, notWant := range tt.wantNotContain {
				if strings.Contains(result, notWant) {
					t.Errorf("ConfigurationBuilder.Build() result should not contain: %q\nGot:\n%s", notWant, result)
				}
			}
		})
	}
}

func TestConfigurationBuilder_Build_RemovesEmptyLines(t *testing.T) {
	builder := NewConfigurationBuilder().SetConfig(models.Config{
		Common: basicCommon(),
	})

	result, err := builder.Build()
	if err != nil {
		t.Fatalf("ConfigurationBuilder.Build() unexpected error = %v", err)
	}

	lines := strings.Split(result, "\n")
	for i, line := range lines {
		if len(strings.TrimSpace(line)) == 0 {
			t.Errorf("ConfigurationBuilder.Build() should not have empty lines, found at line %d", i+1)
		}
	}
}

func TestNewConfigurationBuilder(t *testing.T) {
	builder := NewConfigurationBuilder()
	if builder == nil {
		t.Error("NewConfigurationBuilder() returned nil")
	}
}

func TestConfigurationBuilder_SetConfig(t *testing.T) {
	config := models.Config{
		Common: models.Common{
			ServerAddress: "test.example.com",
			ServerPort:    9000,
		},
	}

	builder := NewConfigurationBuilder().SetConfig(config)

	if builder.Config.Common.ServerAddress != "test.example.com" {
		t.Errorf("SetConfig() did not set ServerAddress correctly")
	}
	if builder.Config.Common.ServerPort != 9000 {
		t.Errorf("SetConfig() did not set ServerPort correctly")
	}
}

func TestConfigurationBuilder_SetConfig_Chainable(t *testing.T) {
	config := models.Config{
		Common: basicCommon(),
	}

	result := NewConfigurationBuilder().SetConfig(config)

	if result == nil {
		t.Error("SetConfig() should return the builder for chaining")
	}
}

// Helper functions

func basicCommon() models.Common {
	return models.Common{
		ServerAddress: "frp.example.com",
		ServerPort:    7000,
		AdminAddress:  "0.0.0.0",
		AdminPort:     7400,
		AdminUsername: "admin",
		AdminPassword: "password",
	}
}

func stringPtr(s string) *string {
	return &s
}
