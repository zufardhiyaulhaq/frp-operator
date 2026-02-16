package models

import (
	"testing"

	frpv1alpha1 "github.com/zufardhiyaulhaq/frp-operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestValidateUpstreamServerPorts(t *testing.T) {
	tests := []struct {
		name      string
		upstreams []frpv1alpha1.Upstream
		wantErr   bool
		errMsg    string
	}{
		{
			name:      "empty upstreams",
			upstreams: []frpv1alpha1.Upstream{},
			wantErr:   false,
		},
		{
			name: "single TCP upstream",
			upstreams: []frpv1alpha1.Upstream{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "upstream1"},
					Spec: frpv1alpha1.UpstreamSpec{
						TCP: &frpv1alpha1.UpstreamSpec_TCP{
							Host:   "localhost",
							Port:   80,
							Server: frpv1alpha1.UpstreamSpec_TCP_Server{Port: 8080},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "single UDP upstream",
			upstreams: []frpv1alpha1.Upstream{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "upstream1"},
					Spec: frpv1alpha1.UpstreamSpec{
						UDP: &frpv1alpha1.UpstreamSpec_UDP{
							Host:   "localhost",
							Port:   53,
							Server: frpv1alpha1.UpstreamSpec_UDP_Server{Port: 5353},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "two TCP upstreams with different ports",
			upstreams: []frpv1alpha1.Upstream{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "upstream1"},
					Spec: frpv1alpha1.UpstreamSpec{
						TCP: &frpv1alpha1.UpstreamSpec_TCP{
							Host:   "localhost",
							Port:   80,
							Server: frpv1alpha1.UpstreamSpec_TCP_Server{Port: 8080},
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "upstream2"},
					Spec: frpv1alpha1.UpstreamSpec{
						TCP: &frpv1alpha1.UpstreamSpec_TCP{
							Host:   "localhost",
							Port:   81,
							Server: frpv1alpha1.UpstreamSpec_TCP_Server{Port: 8081},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "two TCP upstreams with same server port - error",
			upstreams: []frpv1alpha1.Upstream{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "upstream1"},
					Spec: frpv1alpha1.UpstreamSpec{
						TCP: &frpv1alpha1.UpstreamSpec_TCP{
							Host:   "localhost",
							Port:   80,
							Server: frpv1alpha1.UpstreamSpec_TCP_Server{Port: 8080},
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "upstream2"},
					Spec: frpv1alpha1.UpstreamSpec{
						TCP: &frpv1alpha1.UpstreamSpec_TCP{
							Host:   "localhost",
							Port:   81,
							Server: frpv1alpha1.UpstreamSpec_TCP_Server{Port: 8080},
						},
					},
				},
			},
			wantErr: true,
			errMsg:  "duplicate server port 8080",
		},
		{
			name: "TCP and UDP upstreams with same server port - error",
			upstreams: []frpv1alpha1.Upstream{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "tcp-upstream"},
					Spec: frpv1alpha1.UpstreamSpec{
						TCP: &frpv1alpha1.UpstreamSpec_TCP{
							Host:   "localhost",
							Port:   80,
							Server: frpv1alpha1.UpstreamSpec_TCP_Server{Port: 8080},
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "udp-upstream"},
					Spec: frpv1alpha1.UpstreamSpec{
						UDP: &frpv1alpha1.UpstreamSpec_UDP{
							Host:   "localhost",
							Port:   53,
							Server: frpv1alpha1.UpstreamSpec_UDP_Server{Port: 8080},
						},
					},
				},
			},
			wantErr: true,
			errMsg:  "duplicate server port 8080",
		},
		{
			name: "two UDP upstreams with same server port - error",
			upstreams: []frpv1alpha1.Upstream{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "udp1"},
					Spec: frpv1alpha1.UpstreamSpec{
						UDP: &frpv1alpha1.UpstreamSpec_UDP{
							Host:   "localhost",
							Port:   53,
							Server: frpv1alpha1.UpstreamSpec_UDP_Server{Port: 5353},
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "udp2"},
					Spec: frpv1alpha1.UpstreamSpec{
						UDP: &frpv1alpha1.UpstreamSpec_UDP{
							Host:   "localhost",
							Port:   54,
							Server: frpv1alpha1.UpstreamSpec_UDP_Server{Port: 5353},
						},
					},
				},
			},
			wantErr: true,
			errMsg:  "duplicate server port 5353",
		},
		{
			name: "STCP upstream is ignored (no server port)",
			upstreams: []frpv1alpha1.Upstream{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "stcp-upstream"},
					Spec: frpv1alpha1.UpstreamSpec{
						STCP: &frpv1alpha1.UpstreamSpec_STCP{
							Host: "localhost",
							Port: 80,
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "XTCP upstream is ignored (no server port)",
			upstreams: []frpv1alpha1.Upstream{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "xtcp-upstream"},
					Spec: frpv1alpha1.UpstreamSpec{
						XTCP: &frpv1alpha1.UpstreamSpec_XTCP{
							Host: "localhost",
							Port: 80,
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "mixed TCP/UDP with STCP/XTCP - only validate TCP/UDP ports",
			upstreams: []frpv1alpha1.Upstream{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "tcp-upstream"},
					Spec: frpv1alpha1.UpstreamSpec{
						TCP: &frpv1alpha1.UpstreamSpec_TCP{
							Host:   "localhost",
							Port:   80,
							Server: frpv1alpha1.UpstreamSpec_TCP_Server{Port: 8080},
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "stcp-upstream"},
					Spec: frpv1alpha1.UpstreamSpec{
						STCP: &frpv1alpha1.UpstreamSpec_STCP{
							Host: "localhost",
							Port: 80,
						},
					},
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateUpstreamServerPorts(tt.upstreams)
			if tt.wantErr {
				if err == nil {
					t.Errorf("validateUpstreamServerPorts() expected error but got nil")
					return
				}
				if tt.errMsg != "" && !contains(err.Error(), tt.errMsg) {
					t.Errorf("validateUpstreamServerPorts() error = %v, want error containing %q", err, tt.errMsg)
				}
			} else {
				if err != nil {
					t.Errorf("validateUpstreamServerPorts() unexpected error = %v", err)
				}
			}
		})
	}
}

func TestValidateVisitorPorts(t *testing.T) {
	tests := []struct {
		name     string
		visitors []frpv1alpha1.Visitor
		wantErr  bool
		errMsg   string
	}{
		{
			name:     "empty visitors",
			visitors: []frpv1alpha1.Visitor{},
			wantErr:  false,
		},
		{
			name: "single STCP visitor",
			visitors: []frpv1alpha1.Visitor{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "visitor1"},
					Spec: frpv1alpha1.VisitorSpec{
						STCP: &frpv1alpha1.VisitorSpec_STCP{
							Host:       "127.0.0.1",
							Port:       8080,
							ServerName: "server1",
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "single XTCP visitor",
			visitors: []frpv1alpha1.Visitor{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "visitor1"},
					Spec: frpv1alpha1.VisitorSpec{
						XTCP: &frpv1alpha1.VisitorSpec_XTCP{
							Host:       "127.0.0.1",
							Port:       8080,
							ServerName: "server1",
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "two STCP visitors with different ports",
			visitors: []frpv1alpha1.Visitor{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "visitor1"},
					Spec: frpv1alpha1.VisitorSpec{
						STCP: &frpv1alpha1.VisitorSpec_STCP{
							Host:       "127.0.0.1",
							Port:       8080,
							ServerName: "server1",
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "visitor2"},
					Spec: frpv1alpha1.VisitorSpec{
						STCP: &frpv1alpha1.VisitorSpec_STCP{
							Host:       "127.0.0.1",
							Port:       8081,
							ServerName: "server2",
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "two STCP visitors with same port - error",
			visitors: []frpv1alpha1.Visitor{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "visitor1"},
					Spec: frpv1alpha1.VisitorSpec{
						STCP: &frpv1alpha1.VisitorSpec_STCP{
							Host:       "127.0.0.1",
							Port:       8080,
							ServerName: "server1",
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "visitor2"},
					Spec: frpv1alpha1.VisitorSpec{
						STCP: &frpv1alpha1.VisitorSpec_STCP{
							Host:       "127.0.0.1",
							Port:       8080,
							ServerName: "server2",
						},
					},
				},
			},
			wantErr: true,
			errMsg:  "duplicate visitor port 8080",
		},
		{
			name: "two XTCP visitors with same port - error",
			visitors: []frpv1alpha1.Visitor{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "visitor1"},
					Spec: frpv1alpha1.VisitorSpec{
						XTCP: &frpv1alpha1.VisitorSpec_XTCP{
							Host:       "127.0.0.1",
							Port:       9090,
							ServerName: "server1",
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "visitor2"},
					Spec: frpv1alpha1.VisitorSpec{
						XTCP: &frpv1alpha1.VisitorSpec_XTCP{
							Host:       "127.0.0.1",
							Port:       9090,
							ServerName: "server2",
						},
					},
				},
			},
			wantErr: true,
			errMsg:  "duplicate visitor port 9090",
		},
		{
			name: "STCP and XTCP visitors with same port - error",
			visitors: []frpv1alpha1.Visitor{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "stcp-visitor"},
					Spec: frpv1alpha1.VisitorSpec{
						STCP: &frpv1alpha1.VisitorSpec_STCP{
							Host:       "127.0.0.1",
							Port:       8080,
							ServerName: "server1",
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "xtcp-visitor"},
					Spec: frpv1alpha1.VisitorSpec{
						XTCP: &frpv1alpha1.VisitorSpec_XTCP{
							Host:       "127.0.0.1",
							Port:       8080,
							ServerName: "server2",
						},
					},
				},
			},
			wantErr: true,
			errMsg:  "duplicate visitor port 8080",
		},
		{
			name: "three visitors - two with same port - error",
			visitors: []frpv1alpha1.Visitor{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "visitor1"},
					Spec: frpv1alpha1.VisitorSpec{
						STCP: &frpv1alpha1.VisitorSpec_STCP{
							Host:       "127.0.0.1",
							Port:       8080,
							ServerName: "server1",
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "visitor2"},
					Spec: frpv1alpha1.VisitorSpec{
						XTCP: &frpv1alpha1.VisitorSpec_XTCP{
							Host:       "127.0.0.1",
							Port:       9090,
							ServerName: "server2",
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "visitor3"},
					Spec: frpv1alpha1.VisitorSpec{
						STCP: &frpv1alpha1.VisitorSpec_STCP{
							Host:       "127.0.0.1",
							Port:       8080,
							ServerName: "server3",
						},
					},
				},
			},
			wantErr: true,
			errMsg:  "duplicate visitor port 8080",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateVisitorPorts(tt.visitors)
			if tt.wantErr {
				if err == nil {
					t.Errorf("validateVisitorPorts() expected error but got nil")
					return
				}
				if tt.errMsg != "" && !contains(err.Error(), tt.errMsg) {
					t.Errorf("validateVisitorPorts() error = %v, want error containing %q", err, tt.errMsg)
				}
			} else {
				if err != nil {
					t.Errorf("validateVisitorPorts() unexpected error = %v", err)
				}
			}
		})
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && searchSubstring(s, substr)))
}

func searchSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// Helper to create a fake client with secrets
func createFakeClient(secrets ...*corev1.Secret) *fake.ClientBuilder {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	objects := make([]runtime.Object, len(secrets))
	for i, s := range secrets {
		objects[i] = s
	}

	return fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(objects...)
}

// Helper to create a secret
func createSecret(namespace, name string, data map[string][]byte) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Data: data,
	}
}

// Helper to create a basic client object
func createBasicClient(namespace, name, host string, port int) *frpv1alpha1.Client {
	return &frpv1alpha1.Client{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: frpv1alpha1.ClientSpec{
			Server: frpv1alpha1.ClientSpec_Server{
				Host: host,
				Port: port,
				Authentication: frpv1alpha1.ClientSpec_Server_Authentication{
					Token: nil,
				},
			},
		},
	}
}

func stringPtr(s string) *string {
	return &s
}

func TestNewConfig_BasicClient(t *testing.T) {
	fakeClient := createFakeClient().Build()

	clientObj := createBasicClient("default", "test-client", "frp.example.com", 7000)

	config, err := NewConfig(fakeClient, clientObj, []frpv1alpha1.Upstream{}, []frpv1alpha1.Visitor{})
	if err != nil {
		t.Fatalf("NewConfig() unexpected error = %v", err)
	}

	if config.Common.ServerAddress != "frp.example.com" {
		t.Errorf("NewConfig() ServerAddress = %v, want %v", config.Common.ServerAddress, "frp.example.com")
	}
	if config.Common.ServerPort != 7000 {
		t.Errorf("NewConfig() ServerPort = %v, want %v", config.Common.ServerPort, 7000)
	}
	if config.Common.ServerProtocol != "TCP" {
		t.Errorf("NewConfig() ServerProtocol = %v, want %v", config.Common.ServerProtocol, "TCP")
	}
	if config.Common.AdminAddress != DEFAULT_ADMIN_ADDRESS {
		t.Errorf("NewConfig() AdminAddress = %v, want %v", config.Common.AdminAddress, DEFAULT_ADMIN_ADDRESS)
	}
	if config.Common.AdminPort != DEFAULT_ADMIN_PORT {
		t.Errorf("NewConfig() AdminPort = %v, want %v", config.Common.AdminPort, DEFAULT_ADMIN_PORT)
	}
	if config.Common.AdminUsername != DEFAULT_ADMIN_USERNAME {
		t.Errorf("NewConfig() AdminUsername = %v, want %v", config.Common.AdminUsername, DEFAULT_ADMIN_USERNAME)
	}
	if config.Common.AdminPassword != DEFAULT_ADMIN_PASSWORD {
		t.Errorf("NewConfig() AdminPassword = %v, want %v", config.Common.AdminPassword, DEFAULT_ADMIN_PASSWORD)
	}
}

func TestNewConfig_WithProtocol(t *testing.T) {
	fakeClient := createFakeClient().Build()

	clientObj := createBasicClient("default", "test-client", "frp.example.com", 7000)
	clientObj.Spec.Server.Protocol = stringPtr("kcp")

	config, err := NewConfig(fakeClient, clientObj, []frpv1alpha1.Upstream{}, []frpv1alpha1.Visitor{})
	if err != nil {
		t.Fatalf("NewConfig() unexpected error = %v", err)
	}

	if config.Common.ServerProtocol != "kcp" {
		t.Errorf("NewConfig() ServerProtocol = %v, want %v", config.Common.ServerProtocol, "kcp")
	}
}

func TestNewConfig_WithSTUNServer(t *testing.T) {
	fakeClient := createFakeClient().Build()

	clientObj := createBasicClient("default", "test-client", "frp.example.com", 7000)
	clientObj.Spec.Server.STUNServer = stringPtr("stun.example.com:3478")

	config, err := NewConfig(fakeClient, clientObj, []frpv1alpha1.Upstream{}, []frpv1alpha1.Visitor{})
	if err != nil {
		t.Fatalf("NewConfig() unexpected error = %v", err)
	}

	if config.Common.STUNServer == nil {
		t.Fatal("NewConfig() STUNServer is nil, expected value")
	}
	if *config.Common.STUNServer != "stun.example.com:3478" {
		t.Errorf("NewConfig() STUNServer = %v, want %v", *config.Common.STUNServer, "stun.example.com:3478")
	}
}

func TestNewConfig_WithTokenAuthentication(t *testing.T) {
	tokenSecret := createSecret("default", "frp-token", map[string][]byte{
		"token": []byte("my-secret-token"),
	})
	fakeClient := createFakeClient(tokenSecret).Build()

	clientObj := createBasicClient("default", "test-client", "frp.example.com", 7000)
	clientObj.Spec.Server.Authentication.Token = &frpv1alpha1.ClientSpec_Server_Authentication_Token{
		Secret: frpv1alpha1.Secret{
			Name: "frp-token",
			Key:  "token",
		},
	}

	config, err := NewConfig(fakeClient, clientObj, []frpv1alpha1.Upstream{}, []frpv1alpha1.Visitor{})
	if err != nil {
		t.Fatalf("NewConfig() unexpected error = %v", err)
	}

	if config.Common.ServerAuthentication.Type != 1 {
		t.Errorf("NewConfig() ServerAuthentication.Type = %v, want %v", config.Common.ServerAuthentication.Type, 1)
	}
	if config.Common.ServerAuthentication.Token != "my-secret-token" {
		t.Errorf("NewConfig() ServerAuthentication.Token = %v, want %v", config.Common.ServerAuthentication.Token, "my-secret-token")
	}
}

func TestNewConfig_TokenSecretNotFound(t *testing.T) {
	fakeClient := createFakeClient().Build()

	clientObj := createBasicClient("default", "test-client", "frp.example.com", 7000)
	clientObj.Spec.Server.Authentication.Token = &frpv1alpha1.ClientSpec_Server_Authentication_Token{
		Secret: frpv1alpha1.Secret{
			Name: "non-existent-secret",
			Key:  "token",
		},
	}

	_, err := NewConfig(fakeClient, clientObj, []frpv1alpha1.Upstream{}, []frpv1alpha1.Visitor{})
	if err == nil {
		t.Error("NewConfig() expected error for missing secret, got nil")
	}
}

func TestNewConfig_WithAdminServer(t *testing.T) {
	usernameSecret := createSecret("default", "admin-creds", map[string][]byte{
		"username": []byte("custom-admin"),
		"password": []byte("custom-password"),
	})
	fakeClient := createFakeClient(usernameSecret).Build()

	clientObj := createBasicClient("default", "test-client", "frp.example.com", 7000)
	clientObj.Spec.Server.AdminServer = &frpv1alpha1.ClientSpec_Server_AdminServer{
		Port: 7500,
		Username: &frpv1alpha1.ClientSpec_Server_AdminServer_Username{
			Secret: frpv1alpha1.Secret{
				Name: "admin-creds",
				Key:  "username",
			},
		},
		Password: &frpv1alpha1.ClientSpec_Server_AdminServer_Password{
			Secret: frpv1alpha1.Secret{
				Name: "admin-creds",
				Key:  "password",
			},
		},
	}

	config, err := NewConfig(fakeClient, clientObj, []frpv1alpha1.Upstream{}, []frpv1alpha1.Visitor{})
	if err != nil {
		t.Fatalf("NewConfig() unexpected error = %v", err)
	}

	if config.Common.AdminPort != 7500 {
		t.Errorf("NewConfig() AdminPort = %v, want %v", config.Common.AdminPort, 7500)
	}
	if config.Common.AdminUsername != "custom-admin" {
		t.Errorf("NewConfig() AdminUsername = %v, want %v", config.Common.AdminUsername, "custom-admin")
	}
	if config.Common.AdminPassword != "custom-password" {
		t.Errorf("NewConfig() AdminPassword = %v, want %v", config.Common.AdminPassword, "custom-password")
	}
}

func TestNewConfig_TCPUpstream(t *testing.T) {
	fakeClient := createFakeClient().Build()
	clientObj := createBasicClient("default", "test-client", "frp.example.com", 7000)

	upstreams := []frpv1alpha1.Upstream{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "tcp-upstream"},
			Spec: frpv1alpha1.UpstreamSpec{
				TCP: &frpv1alpha1.UpstreamSpec_TCP{
					Host:   "127.0.0.1",
					Port:   80,
					Server: frpv1alpha1.UpstreamSpec_TCP_Server{Port: 8080},
				},
			},
		},
	}

	config, err := NewConfig(fakeClient, clientObj, upstreams, []frpv1alpha1.Visitor{})
	if err != nil {
		t.Fatalf("NewConfig() unexpected error = %v", err)
	}

	if len(config.Upstreams) != 1 {
		t.Fatalf("NewConfig() Upstreams length = %v, want 1", len(config.Upstreams))
	}

	upstream := config.Upstreams[0]
	if upstream.Name != "tcp-upstream" {
		t.Errorf("NewConfig() upstream.Name = %v, want %v", upstream.Name, "tcp-upstream")
	}
	if upstream.Type != 1 {
		t.Errorf("NewConfig() upstream.Type = %v, want 1 (TCP)", upstream.Type)
	}
	if upstream.TCP.Host != "127.0.0.1" {
		t.Errorf("NewConfig() upstream.TCP.Host = %v, want %v", upstream.TCP.Host, "127.0.0.1")
	}
	if upstream.TCP.Port != 80 {
		t.Errorf("NewConfig() upstream.TCP.Port = %v, want %v", upstream.TCP.Port, 80)
	}
	if upstream.TCP.ServerPort != 8080 {
		t.Errorf("NewConfig() upstream.TCP.ServerPort = %v, want %v", upstream.TCP.ServerPort, 8080)
	}
}

func TestNewConfig_TCPUpstreamWithAllOptions(t *testing.T) {
	fakeClient := createFakeClient().Build()
	clientObj := createBasicClient("default", "test-client", "frp.example.com", 7000)

	upstreams := []frpv1alpha1.Upstream{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "tcp-full"},
			Spec: frpv1alpha1.UpstreamSpec{
				TCP: &frpv1alpha1.UpstreamSpec_TCP{
					Host:          "127.0.0.1",
					Port:          80,
					Server:        frpv1alpha1.UpstreamSpec_TCP_Server{Port: 8080},
					ProxyProtocol: stringPtr("v2"),
					HealthCheck: &frpv1alpha1.UpstreamSpec_TCP_HealthCheck{
						TimeoutSeconds:  5,
						MaxFailed:       3,
						IntervalSeconds: 10,
					},
					Transport: &frpv1alpha1.UpstreamSpec_TCP_Transport{
						UseEncryption:  true,
						UseCompression: true,
						BandwdithLimit: &frpv1alpha1.UpstreamSpec_TCP_Transport_BandwdithLimit{
							Enabled: true,
							Limit:   100,
							Type:    "MB",
						},
						ProxyURL: stringPtr("http://proxy:8080"),
					},
				},
			},
		},
	}

	config, err := NewConfig(fakeClient, clientObj, upstreams, []frpv1alpha1.Visitor{})
	if err != nil {
		t.Fatalf("NewConfig() unexpected error = %v", err)
	}

	upstream := config.Upstreams[0]
	if upstream.TCP.ProxyProtocol == nil || *upstream.TCP.ProxyProtocol != "v2" {
		t.Errorf("NewConfig() upstream.TCP.ProxyProtocol = %v, want v2", upstream.TCP.ProxyProtocol)
	}
	if upstream.TCP.HealthCheck == nil {
		t.Fatal("NewConfig() upstream.TCP.HealthCheck is nil")
	}
	if upstream.TCP.HealthCheck.TimeoutSeconds != 5 {
		t.Errorf("NewConfig() HealthCheck.TimeoutSeconds = %v, want 5", upstream.TCP.HealthCheck.TimeoutSeconds)
	}
	if upstream.TCP.HealthCheck.MaxFailed != 3 {
		t.Errorf("NewConfig() HealthCheck.MaxFailed = %v, want 3", upstream.TCP.HealthCheck.MaxFailed)
	}
	if upstream.TCP.Transport == nil {
		t.Fatal("NewConfig() upstream.TCP.Transport is nil")
	}
	if !upstream.TCP.Transport.UseEncryption {
		t.Error("NewConfig() Transport.UseEncryption should be true")
	}
	if !upstream.TCP.Transport.UseCompression {
		t.Error("NewConfig() Transport.UseCompression should be true")
	}
	if upstream.TCP.Transport.BandwdithLimit == nil || !upstream.TCP.Transport.BandwdithLimit.Enabled {
		t.Error("NewConfig() Transport.BandwdithLimit should be enabled")
	}
	if upstream.TCP.Transport.BandwdithLimit.Limit != 100 {
		t.Errorf("NewConfig() BandwdithLimit.Limit = %v, want 100", upstream.TCP.Transport.BandwdithLimit.Limit)
	}
	if upstream.TCP.Transport.ProxyURL == nil || *upstream.TCP.Transport.ProxyURL != "http://proxy:8080" {
		t.Errorf("NewConfig() Transport.ProxyURL = %v, want http://proxy:8080", upstream.TCP.Transport.ProxyURL)
	}
}

func TestNewConfig_UDPUpstream(t *testing.T) {
	fakeClient := createFakeClient().Build()
	clientObj := createBasicClient("default", "test-client", "frp.example.com", 7000)

	upstreams := []frpv1alpha1.Upstream{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "udp-upstream"},
			Spec: frpv1alpha1.UpstreamSpec{
				UDP: &frpv1alpha1.UpstreamSpec_UDP{
					Host:   "127.0.0.1",
					Port:   53,
					Server: frpv1alpha1.UpstreamSpec_UDP_Server{Port: 5353},
				},
			},
		},
	}

	config, err := NewConfig(fakeClient, clientObj, upstreams, []frpv1alpha1.Visitor{})
	if err != nil {
		t.Fatalf("NewConfig() unexpected error = %v", err)
	}

	if len(config.Upstreams) != 1 {
		t.Fatalf("NewConfig() Upstreams length = %v, want 1", len(config.Upstreams))
	}

	upstream := config.Upstreams[0]
	if upstream.Type != 2 {
		t.Errorf("NewConfig() upstream.Type = %v, want 2 (UDP)", upstream.Type)
	}
	if upstream.UDP.Host != "127.0.0.1" {
		t.Errorf("NewConfig() upstream.UDP.Host = %v, want %v", upstream.UDP.Host, "127.0.0.1")
	}
	if upstream.UDP.Port != 53 {
		t.Errorf("NewConfig() upstream.UDP.Port = %v, want %v", upstream.UDP.Port, 53)
	}
	if upstream.UDP.ServerPort != 5353 {
		t.Errorf("NewConfig() upstream.UDP.ServerPort = %v, want %v", upstream.UDP.ServerPort, 5353)
	}
}

func TestNewConfig_STCPUpstream(t *testing.T) {
	secretKeySecret := createSecret("default", "stcp-secret", map[string][]byte{
		"key": []byte("stcp-secret-key"),
	})
	fakeClient := createFakeClient(secretKeySecret).Build()
	clientObj := createBasicClient("default", "test-client", "frp.example.com", 7000)

	upstreams := []frpv1alpha1.Upstream{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "stcp-upstream"},
			Spec: frpv1alpha1.UpstreamSpec{
				STCP: &frpv1alpha1.UpstreamSpec_STCP{
					Host: "127.0.0.1",
					Port: 22,
					SecretKey: frpv1alpha1.UpstreamSpec_STCP_SecretKey{
						Secret: frpv1alpha1.Secret{
							Name: "stcp-secret",
							Key:  "key",
						},
					},
				},
			},
		},
	}

	config, err := NewConfig(fakeClient, clientObj, upstreams, []frpv1alpha1.Visitor{})
	if err != nil {
		t.Fatalf("NewConfig() unexpected error = %v", err)
	}

	if len(config.Upstreams) != 1 {
		t.Fatalf("NewConfig() Upstreams length = %v, want 1", len(config.Upstreams))
	}

	upstream := config.Upstreams[0]
	if upstream.Type != 3 {
		t.Errorf("NewConfig() upstream.Type = %v, want 3 (STCP)", upstream.Type)
	}
	if upstream.STCP.Host != "127.0.0.1" {
		t.Errorf("NewConfig() upstream.STCP.Host = %v, want %v", upstream.STCP.Host, "127.0.0.1")
	}
	if upstream.STCP.Port != 22 {
		t.Errorf("NewConfig() upstream.STCP.Port = %v, want %v", upstream.STCP.Port, 22)
	}
	if upstream.STCP.SecretKey != "stcp-secret-key" {
		t.Errorf("NewConfig() upstream.STCP.SecretKey = %v, want %v", upstream.STCP.SecretKey, "stcp-secret-key")
	}
}

func TestNewConfig_STCPUpstreamWithOptions(t *testing.T) {
	secretKeySecret := createSecret("default", "stcp-secret", map[string][]byte{
		"key": []byte("stcp-secret-key"),
	})
	fakeClient := createFakeClient(secretKeySecret).Build()
	clientObj := createBasicClient("default", "test-client", "frp.example.com", 7000)

	upstreams := []frpv1alpha1.Upstream{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "stcp-full"},
			Spec: frpv1alpha1.UpstreamSpec{
				STCP: &frpv1alpha1.UpstreamSpec_STCP{
					Host: "127.0.0.1",
					Port: 22,
					SecretKey: frpv1alpha1.UpstreamSpec_STCP_SecretKey{
						Secret: frpv1alpha1.Secret{
							Name: "stcp-secret",
							Key:  "key",
						},
					},
					ProxyProtocol: stringPtr("v1"),
					HealthCheck: &frpv1alpha1.UpstreamSpec_TCP_HealthCheck{
						TimeoutSeconds:  3,
						MaxFailed:       5,
						IntervalSeconds: 15,
					},
					Transport: &frpv1alpha1.UpstreamSpec_TCP_Transport{
						UseEncryption:  true,
						UseCompression: false,
					},
				},
			},
		},
	}

	config, err := NewConfig(fakeClient, clientObj, upstreams, []frpv1alpha1.Visitor{})
	if err != nil {
		t.Fatalf("NewConfig() unexpected error = %v", err)
	}

	upstream := config.Upstreams[0]
	if upstream.STCP.ProxyProtocol == nil || *upstream.STCP.ProxyProtocol != "v1" {
		t.Errorf("NewConfig() upstream.STCP.ProxyProtocol = %v, want v1", upstream.STCP.ProxyProtocol)
	}
	if upstream.STCP.HealthCheck == nil {
		t.Fatal("NewConfig() upstream.STCP.HealthCheck is nil")
	}
	if upstream.STCP.HealthCheck.TimeoutSeconds != 3 {
		t.Errorf("NewConfig() HealthCheck.TimeoutSeconds = %v, want 3", upstream.STCP.HealthCheck.TimeoutSeconds)
	}
	if upstream.STCP.Transport == nil {
		t.Fatal("NewConfig() upstream.STCP.Transport is nil")
	}
	if !upstream.STCP.Transport.UseEncryption {
		t.Error("NewConfig() Transport.UseEncryption should be true")
	}
}

func TestNewConfig_XTCPUpstream(t *testing.T) {
	secretKeySecret := createSecret("default", "xtcp-secret", map[string][]byte{
		"key": []byte("xtcp-secret-key"),
	})
	fakeClient := createFakeClient(secretKeySecret).Build()
	clientObj := createBasicClient("default", "test-client", "frp.example.com", 7000)

	upstreams := []frpv1alpha1.Upstream{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "xtcp-upstream"},
			Spec: frpv1alpha1.UpstreamSpec{
				XTCP: &frpv1alpha1.UpstreamSpec_XTCP{
					Host: "127.0.0.1",
					Port: 3389,
					SecretKey: frpv1alpha1.UpstreamSpec_XTCP_SecretKey{
						Secret: frpv1alpha1.Secret{
							Name: "xtcp-secret",
							Key:  "key",
						},
					},
				},
			},
		},
	}

	config, err := NewConfig(fakeClient, clientObj, upstreams, []frpv1alpha1.Visitor{})
	if err != nil {
		t.Fatalf("NewConfig() unexpected error = %v", err)
	}

	if len(config.Upstreams) != 1 {
		t.Fatalf("NewConfig() Upstreams length = %v, want 1", len(config.Upstreams))
	}

	upstream := config.Upstreams[0]
	if upstream.Type != 4 {
		t.Errorf("NewConfig() upstream.Type = %v, want 4 (XTCP)", upstream.Type)
	}
	if upstream.XTCP.Host != "127.0.0.1" {
		t.Errorf("NewConfig() upstream.XTCP.Host = %v, want %v", upstream.XTCP.Host, "127.0.0.1")
	}
	if upstream.XTCP.Port != 3389 {
		t.Errorf("NewConfig() upstream.XTCP.Port = %v, want %v", upstream.XTCP.Port, 3389)
	}
	if upstream.XTCP.SecretKey != "xtcp-secret-key" {
		t.Errorf("NewConfig() upstream.XTCP.SecretKey = %v, want %v", upstream.XTCP.SecretKey, "xtcp-secret-key")
	}
}

func TestNewConfig_XTCPUpstreamWithOptions(t *testing.T) {
	secretKeySecret := createSecret("default", "xtcp-secret", map[string][]byte{
		"key": []byte("xtcp-secret-key"),
	})
	fakeClient := createFakeClient(secretKeySecret).Build()
	clientObj := createBasicClient("default", "test-client", "frp.example.com", 7000)

	upstreams := []frpv1alpha1.Upstream{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "xtcp-full"},
			Spec: frpv1alpha1.UpstreamSpec{
				XTCP: &frpv1alpha1.UpstreamSpec_XTCP{
					Host: "127.0.0.1",
					Port: 3389,
					SecretKey: frpv1alpha1.UpstreamSpec_XTCP_SecretKey{
						Secret: frpv1alpha1.Secret{
							Name: "xtcp-secret",
							Key:  "key",
						},
					},
					ProxyProtocol: stringPtr("v2"),
					HealthCheck: &frpv1alpha1.UpstreamSpec_TCP_HealthCheck{
						TimeoutSeconds:  10,
						MaxFailed:       5,
						IntervalSeconds: 30,
					},
					Transport: &frpv1alpha1.UpstreamSpec_TCP_Transport{
						UseEncryption:  true,
						UseCompression: true,
						BandwdithLimit: &frpv1alpha1.UpstreamSpec_TCP_Transport_BandwdithLimit{
							Enabled: true,
							Limit:   500,
							Type:    "KB",
						},
					},
				},
			},
		},
	}

	config, err := NewConfig(fakeClient, clientObj, upstreams, []frpv1alpha1.Visitor{})
	if err != nil {
		t.Fatalf("NewConfig() unexpected error = %v", err)
	}

	upstream := config.Upstreams[0]
	if upstream.XTCP.ProxyProtocol == nil || *upstream.XTCP.ProxyProtocol != "v2" {
		t.Errorf("NewConfig() upstream.XTCP.ProxyProtocol = %v, want v2", upstream.XTCP.ProxyProtocol)
	}
	if upstream.XTCP.HealthCheck == nil {
		t.Fatal("NewConfig() upstream.XTCP.HealthCheck is nil")
	}
	if upstream.XTCP.Transport == nil {
		t.Fatal("NewConfig() upstream.XTCP.Transport is nil")
	}
	if upstream.XTCP.Transport.BandwdithLimit == nil {
		t.Fatal("NewConfig() upstream.XTCP.Transport.BandwdithLimit is nil")
	}
	if upstream.XTCP.Transport.BandwdithLimit.Limit != 500 {
		t.Errorf("NewConfig() BandwdithLimit.Limit = %v, want 500", upstream.XTCP.Transport.BandwdithLimit.Limit)
	}
}

func TestNewConfig_STCPVisitor(t *testing.T) {
	secretKeySecret := createSecret("default", "visitor-secret", map[string][]byte{
		"key": []byte("visitor-secret-key"),
	})
	fakeClient := createFakeClient(secretKeySecret).Build()
	clientObj := createBasicClient("default", "test-client", "frp.example.com", 7000)

	visitors := []frpv1alpha1.Visitor{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "stcp-visitor"},
			Spec: frpv1alpha1.VisitorSpec{
				STCP: &frpv1alpha1.VisitorSpec_STCP{
					Host:       "127.0.0.1",
					Port:       2222,
					ServerName: "ssh-server",
					ServerSecretKey: frpv1alpha1.VisitorSpec_STCP_ServerSecretKey{
						Secret: frpv1alpha1.Secret{
							Name: "visitor-secret",
							Key:  "key",
						},
					},
				},
			},
		},
	}

	config, err := NewConfig(fakeClient, clientObj, []frpv1alpha1.Upstream{}, visitors)
	if err != nil {
		t.Fatalf("NewConfig() unexpected error = %v", err)
	}

	if len(config.Visitors) != 1 {
		t.Fatalf("NewConfig() Visitors length = %v, want 1", len(config.Visitors))
	}

	visitor := config.Visitors[0]
	if visitor.Name != "stcp-visitor" {
		t.Errorf("NewConfig() visitor.Name = %v, want %v", visitor.Name, "stcp-visitor")
	}
	if visitor.Type != 1 {
		t.Errorf("NewConfig() visitor.Type = %v, want 1 (STCP)", visitor.Type)
	}
	if visitor.STCP.Host != "127.0.0.1" {
		t.Errorf("NewConfig() visitor.STCP.Host = %v, want %v", visitor.STCP.Host, "127.0.0.1")
	}
	if visitor.STCP.Port != 2222 {
		t.Errorf("NewConfig() visitor.STCP.Port = %v, want %v", visitor.STCP.Port, 2222)
	}
	if visitor.STCP.ServerName != "ssh-server" {
		t.Errorf("NewConfig() visitor.STCP.ServerName = %v, want %v", visitor.STCP.ServerName, "ssh-server")
	}
	if visitor.STCP.SecretKey != "visitor-secret-key" {
		t.Errorf("NewConfig() visitor.STCP.SecretKey = %v, want %v", visitor.STCP.SecretKey, "visitor-secret-key")
	}
}

func TestNewConfig_XTCPVisitor(t *testing.T) {
	secretKeySecret := createSecret("default", "visitor-secret", map[string][]byte{
		"key": []byte("xtcp-visitor-secret-key"),
	})
	fakeClient := createFakeClient(secretKeySecret).Build()
	clientObj := createBasicClient("default", "test-client", "frp.example.com", 7000)

	visitors := []frpv1alpha1.Visitor{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "xtcp-visitor"},
			Spec: frpv1alpha1.VisitorSpec{
				XTCP: &frpv1alpha1.VisitorSpec_XTCP{
					Host:                 "0.0.0.0",
					Port:                 3390,
					ServerName:           "rdp-server",
					PersistantConnection: true,
					ServerSecretKey: frpv1alpha1.VisitorSpec_XTCP_ServerSecretKey{
						Secret: frpv1alpha1.Secret{
							Name: "visitor-secret",
							Key:  "key",
						},
					},
				},
			},
		},
	}

	config, err := NewConfig(fakeClient, clientObj, []frpv1alpha1.Upstream{}, visitors)
	if err != nil {
		t.Fatalf("NewConfig() unexpected error = %v", err)
	}

	if len(config.Visitors) != 1 {
		t.Fatalf("NewConfig() Visitors length = %v, want 1", len(config.Visitors))
	}

	visitor := config.Visitors[0]
	if visitor.Type != 2 {
		t.Errorf("NewConfig() visitor.Type = %v, want 2 (XTCP)", visitor.Type)
	}
	if visitor.XTCP.Host != "0.0.0.0" {
		t.Errorf("NewConfig() visitor.XTCP.Host = %v, want %v", visitor.XTCP.Host, "0.0.0.0")
	}
	if visitor.XTCP.Port != 3390 {
		t.Errorf("NewConfig() visitor.XTCP.Port = %v, want %v", visitor.XTCP.Port, 3390)
	}
	if visitor.XTCP.ServerName != "rdp-server" {
		t.Errorf("NewConfig() visitor.XTCP.ServerName = %v, want %v", visitor.XTCP.ServerName, "rdp-server")
	}
	if !visitor.XTCP.PersistantConnection {
		t.Error("NewConfig() visitor.XTCP.PersistantConnection should be true")
	}
	if visitor.XTCP.SecretKey != "xtcp-visitor-secret-key" {
		t.Errorf("NewConfig() visitor.XTCP.SecretKey = %v, want %v", visitor.XTCP.SecretKey, "xtcp-visitor-secret-key")
	}
	if visitor.XTCP.EnableAssistedAddrs {
		t.Error("NewConfig() visitor.XTCP.EnableAssistedAddrs should be false by default")
	}
}

func TestNewConfig_XTCPVisitorWithEnableAssistedAddrs(t *testing.T) {
	secretKeySecret := createSecret("default", "visitor-secret", map[string][]byte{
		"key": []byte("xtcp-visitor-secret-key"),
	})
	fakeClient := createFakeClient(secretKeySecret).Build()
	clientObj := createBasicClient("default", "test-client", "frp.example.com", 7000)

	visitors := []frpv1alpha1.Visitor{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "xtcp-visitor-assisted"},
			Spec: frpv1alpha1.VisitorSpec{
				XTCP: &frpv1alpha1.VisitorSpec_XTCP{
					Host:                 "0.0.0.0",
					Port:                 3390,
					ServerName:           "rdp-server",
					PersistantConnection: true,
					EnableAssistedAddrs:  true,
					ServerSecretKey: frpv1alpha1.VisitorSpec_XTCP_ServerSecretKey{
						Secret: frpv1alpha1.Secret{
							Name: "visitor-secret",
							Key:  "key",
						},
					},
				},
			},
		},
	}

	config, err := NewConfig(fakeClient, clientObj, []frpv1alpha1.Upstream{}, visitors)
	if err != nil {
		t.Fatalf("NewConfig() unexpected error = %v", err)
	}

	visitor := config.Visitors[0]
	if !visitor.XTCP.EnableAssistedAddrs {
		t.Error("NewConfig() visitor.XTCP.EnableAssistedAddrs should be true")
	}
}

func TestNewConfig_XTCPVisitorWithFallback(t *testing.T) {
	secretKeySecret := createSecret("default", "visitor-secret", map[string][]byte{
		"key": []byte("xtcp-visitor-secret-key"),
	})
	fakeClient := createFakeClient(secretKeySecret).Build()
	clientObj := createBasicClient("default", "test-client", "frp.example.com", 7000)

	visitors := []frpv1alpha1.Visitor{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "xtcp-visitor-fallback"},
			Spec: frpv1alpha1.VisitorSpec{
				XTCP: &frpv1alpha1.VisitorSpec_XTCP{
					Host:                 "0.0.0.0",
					Port:                 3390,
					ServerName:           "rdp-server",
					PersistantConnection: false,
					ServerSecretKey: frpv1alpha1.VisitorSpec_XTCP_ServerSecretKey{
						Secret: frpv1alpha1.Secret{
							Name: "visitor-secret",
							Key:  "key",
						},
					},
					Fallback: &frpv1alpha1.VisitorSpec_Fallback{
						ServerName: "stcp-fallback-server",
						Timeout:    5000,
					},
				},
			},
		},
	}

	config, err := NewConfig(fakeClient, clientObj, []frpv1alpha1.Upstream{}, visitors)
	if err != nil {
		t.Fatalf("NewConfig() unexpected error = %v", err)
	}

	visitor := config.Visitors[0]
	if visitor.XTCP.Fallback == nil {
		t.Fatal("NewConfig() visitor.XTCP.Fallback is nil")
	}
	if visitor.XTCP.Fallback.ServerName != "stcp-fallback-server" {
		t.Errorf("NewConfig() Fallback.ServerName = %v, want %v", visitor.XTCP.Fallback.ServerName, "stcp-fallback-server")
	}
	if visitor.XTCP.Fallback.Timeout != 5000 {
		t.Errorf("NewConfig() Fallback.Timeout = %v, want 5000", visitor.XTCP.Fallback.Timeout)
	}
}

func TestNewConfig_UpstreamNoProtocol(t *testing.T) {
	fakeClient := createFakeClient().Build()
	clientObj := createBasicClient("default", "test-client", "frp.example.com", 7000)

	upstreams := []frpv1alpha1.Upstream{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "no-protocol"},
			Spec:       frpv1alpha1.UpstreamSpec{},
		},
	}

	_, err := NewConfig(fakeClient, clientObj, upstreams, []frpv1alpha1.Visitor{})
	if err == nil {
		t.Error("NewConfig() expected error for upstream without protocol")
	}
	if !contains(err.Error(), "TCP, UDP, STCP, XTCP, HTTP, or HTTPS upstream is required") {
		t.Errorf("NewConfig() error = %v, want error containing 'TCP, UDP, STCP, XTCP, HTTP, or HTTPS upstream is required'", err)
	}
}

func TestNewConfig_VisitorNoProtocol(t *testing.T) {
	fakeClient := createFakeClient().Build()
	clientObj := createBasicClient("default", "test-client", "frp.example.com", 7000)

	visitors := []frpv1alpha1.Visitor{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "no-protocol"},
			Spec:       frpv1alpha1.VisitorSpec{},
		},
	}

	_, err := NewConfig(fakeClient, clientObj, []frpv1alpha1.Upstream{}, visitors)
	if err == nil {
		t.Error("NewConfig() expected error for visitor without protocol")
	}
	if !contains(err.Error(), "STCP, XTCP visitor is required") {
		t.Errorf("NewConfig() error = %v, want error containing 'STCP, XTCP visitor is required'", err)
	}
}

func TestNewConfig_STCPUpstreamSecretNotFound(t *testing.T) {
	fakeClient := createFakeClient().Build()
	clientObj := createBasicClient("default", "test-client", "frp.example.com", 7000)

	upstreams := []frpv1alpha1.Upstream{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "stcp-upstream"},
			Spec: frpv1alpha1.UpstreamSpec{
				STCP: &frpv1alpha1.UpstreamSpec_STCP{
					Host: "127.0.0.1",
					Port: 22,
					SecretKey: frpv1alpha1.UpstreamSpec_STCP_SecretKey{
						Secret: frpv1alpha1.Secret{
							Name: "non-existent-secret",
							Key:  "key",
						},
					},
				},
			},
		},
	}

	_, err := NewConfig(fakeClient, clientObj, upstreams, []frpv1alpha1.Visitor{})
	if err == nil {
		t.Error("NewConfig() expected error for missing secret")
	}
}

func TestNewConfig_XTCPUpstreamSecretNotFound(t *testing.T) {
	fakeClient := createFakeClient().Build()
	clientObj := createBasicClient("default", "test-client", "frp.example.com", 7000)

	upstreams := []frpv1alpha1.Upstream{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "xtcp-upstream"},
			Spec: frpv1alpha1.UpstreamSpec{
				XTCP: &frpv1alpha1.UpstreamSpec_XTCP{
					Host: "127.0.0.1",
					Port: 3389,
					SecretKey: frpv1alpha1.UpstreamSpec_XTCP_SecretKey{
						Secret: frpv1alpha1.Secret{
							Name: "non-existent-secret",
							Key:  "key",
						},
					},
				},
			},
		},
	}

	_, err := NewConfig(fakeClient, clientObj, upstreams, []frpv1alpha1.Visitor{})
	if err == nil {
		t.Error("NewConfig() expected error for missing secret")
	}
}

func TestNewConfig_STCPVisitorSecretNotFound(t *testing.T) {
	fakeClient := createFakeClient().Build()
	clientObj := createBasicClient("default", "test-client", "frp.example.com", 7000)

	visitors := []frpv1alpha1.Visitor{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "stcp-visitor"},
			Spec: frpv1alpha1.VisitorSpec{
				STCP: &frpv1alpha1.VisitorSpec_STCP{
					Host:       "127.0.0.1",
					Port:       2222,
					ServerName: "ssh-server",
					ServerSecretKey: frpv1alpha1.VisitorSpec_STCP_ServerSecretKey{
						Secret: frpv1alpha1.Secret{
							Name: "non-existent-secret",
							Key:  "key",
						},
					},
				},
			},
		},
	}

	_, err := NewConfig(fakeClient, clientObj, []frpv1alpha1.Upstream{}, visitors)
	if err == nil {
		t.Error("NewConfig() expected error for missing secret")
	}
}

func TestNewConfig_XTCPVisitorSecretNotFound(t *testing.T) {
	fakeClient := createFakeClient().Build()
	clientObj := createBasicClient("default", "test-client", "frp.example.com", 7000)

	visitors := []frpv1alpha1.Visitor{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "xtcp-visitor"},
			Spec: frpv1alpha1.VisitorSpec{
				XTCP: &frpv1alpha1.VisitorSpec_XTCP{
					Host:       "0.0.0.0",
					Port:       3390,
					ServerName: "rdp-server",
					ServerSecretKey: frpv1alpha1.VisitorSpec_XTCP_ServerSecretKey{
						Secret: frpv1alpha1.Secret{
							Name: "non-existent-secret",
							Key:  "key",
						},
					},
				},
			},
		},
	}

	_, err := NewConfig(fakeClient, clientObj, []frpv1alpha1.Upstream{}, visitors)
	if err == nil {
		t.Error("NewConfig() expected error for missing secret")
	}
}

func TestNewConfig_MultipleUpstreams_Sorted(t *testing.T) {
	fakeClient := createFakeClient().Build()
	clientObj := createBasicClient("default", "test-client", "frp.example.com", 7000)

	upstreams := []frpv1alpha1.Upstream{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "z-upstream"},
			Spec: frpv1alpha1.UpstreamSpec{
				TCP: &frpv1alpha1.UpstreamSpec_TCP{
					Host:   "127.0.0.1",
					Port:   80,
					Server: frpv1alpha1.UpstreamSpec_TCP_Server{Port: 8080},
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "a-upstream"},
			Spec: frpv1alpha1.UpstreamSpec{
				TCP: &frpv1alpha1.UpstreamSpec_TCP{
					Host:   "127.0.0.1",
					Port:   81,
					Server: frpv1alpha1.UpstreamSpec_TCP_Server{Port: 8081},
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "m-upstream"},
			Spec: frpv1alpha1.UpstreamSpec{
				TCP: &frpv1alpha1.UpstreamSpec_TCP{
					Host:   "127.0.0.1",
					Port:   82,
					Server: frpv1alpha1.UpstreamSpec_TCP_Server{Port: 8082},
				},
			},
		},
	}

	config, err := NewConfig(fakeClient, clientObj, upstreams, []frpv1alpha1.Visitor{})
	if err != nil {
		t.Fatalf("NewConfig() unexpected error = %v", err)
	}

	if len(config.Upstreams) != 3 {
		t.Fatalf("NewConfig() Upstreams length = %v, want 3", len(config.Upstreams))
	}

	expectedOrder := []string{"a-upstream", "m-upstream", "z-upstream"}
	for i, expected := range expectedOrder {
		if config.Upstreams[i].Name != expected {
			t.Errorf("NewConfig() Upstreams[%d].Name = %v, want %v", i, config.Upstreams[i].Name, expected)
		}
	}
}

func TestNewConfig_MultipleVisitors_Sorted(t *testing.T) {
	secretKeySecret := createSecret("default", "visitor-secret", map[string][]byte{
		"key": []byte("secret-key"),
	})
	fakeClient := createFakeClient(secretKeySecret).Build()
	clientObj := createBasicClient("default", "test-client", "frp.example.com", 7000)

	visitors := []frpv1alpha1.Visitor{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "z-visitor"},
			Spec: frpv1alpha1.VisitorSpec{
				STCP: &frpv1alpha1.VisitorSpec_STCP{
					Host:       "127.0.0.1",
					Port:       2222,
					ServerName: "server1",
					ServerSecretKey: frpv1alpha1.VisitorSpec_STCP_ServerSecretKey{
						Secret: frpv1alpha1.Secret{Name: "visitor-secret", Key: "key"},
					},
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "a-visitor"},
			Spec: frpv1alpha1.VisitorSpec{
				STCP: &frpv1alpha1.VisitorSpec_STCP{
					Host:       "127.0.0.1",
					Port:       2223,
					ServerName: "server2",
					ServerSecretKey: frpv1alpha1.VisitorSpec_STCP_ServerSecretKey{
						Secret: frpv1alpha1.Secret{Name: "visitor-secret", Key: "key"},
					},
				},
			},
		},
	}

	config, err := NewConfig(fakeClient, clientObj, []frpv1alpha1.Upstream{}, visitors)
	if err != nil {
		t.Fatalf("NewConfig() unexpected error = %v", err)
	}

	if len(config.Visitors) != 2 {
		t.Fatalf("NewConfig() Visitors length = %v, want 2", len(config.Visitors))
	}

	expectedOrder := []string{"a-visitor", "z-visitor"}
	for i, expected := range expectedOrder {
		if config.Visitors[i].Name != expected {
			t.Errorf("NewConfig() Visitors[%d].Name = %v, want %v", i, config.Visitors[i].Name, expected)
		}
	}
}

func TestNewConfig_DuplicateUpstreamServerPorts(t *testing.T) {
	fakeClient := createFakeClient().Build()
	clientObj := createBasicClient("default", "test-client", "frp.example.com", 7000)

	upstreams := []frpv1alpha1.Upstream{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "upstream1"},
			Spec: frpv1alpha1.UpstreamSpec{
				TCP: &frpv1alpha1.UpstreamSpec_TCP{
					Host:   "127.0.0.1",
					Port:   80,
					Server: frpv1alpha1.UpstreamSpec_TCP_Server{Port: 8080},
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "upstream2"},
			Spec: frpv1alpha1.UpstreamSpec{
				TCP: &frpv1alpha1.UpstreamSpec_TCP{
					Host:   "127.0.0.1",
					Port:   81,
					Server: frpv1alpha1.UpstreamSpec_TCP_Server{Port: 8080},
				},
			},
		},
	}

	_, err := NewConfig(fakeClient, clientObj, upstreams, []frpv1alpha1.Visitor{})
	if err == nil {
		t.Error("NewConfig() expected error for duplicate server ports")
	}
	if !contains(err.Error(), "duplicate server port") {
		t.Errorf("NewConfig() error = %v, want error containing 'duplicate server port'", err)
	}
}

func TestNewConfig_DuplicateVisitorPorts(t *testing.T) {
	secretKeySecret := createSecret("default", "visitor-secret", map[string][]byte{
		"key": []byte("secret-key"),
	})
	fakeClient := createFakeClient(secretKeySecret).Build()
	clientObj := createBasicClient("default", "test-client", "frp.example.com", 7000)

	visitors := []frpv1alpha1.Visitor{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "visitor1"},
			Spec: frpv1alpha1.VisitorSpec{
				STCP: &frpv1alpha1.VisitorSpec_STCP{
					Host:       "127.0.0.1",
					Port:       8080,
					ServerName: "server1",
					ServerSecretKey: frpv1alpha1.VisitorSpec_STCP_ServerSecretKey{
						Secret: frpv1alpha1.Secret{Name: "visitor-secret", Key: "key"},
					},
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "visitor2"},
			Spec: frpv1alpha1.VisitorSpec{
				STCP: &frpv1alpha1.VisitorSpec_STCP{
					Host:       "127.0.0.1",
					Port:       8080,
					ServerName: "server2",
					ServerSecretKey: frpv1alpha1.VisitorSpec_STCP_ServerSecretKey{
						Secret: frpv1alpha1.Secret{Name: "visitor-secret", Key: "key"},
					},
				},
			},
		},
	}

	_, err := NewConfig(fakeClient, clientObj, []frpv1alpha1.Upstream{}, visitors)
	if err == nil {
		t.Error("NewConfig() expected error for duplicate visitor ports")
	}
	if !contains(err.Error(), "duplicate visitor port") {
		t.Errorf("NewConfig() error = %v, want error containing 'duplicate visitor port'", err)
	}
}
