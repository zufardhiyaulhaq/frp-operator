package frps

import (
	"context"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/zufardhiyaulhaq/frp-operator/api/v1alpha1"
	"github.com/zufardhiyaulhaq/frp-operator/pkg/frps/frpstest"
)

func newFake(t *testing.T, objs ...client.Object) client.Client {
	t.Helper()
	s := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(s); err != nil {
		t.Fatal(err)
	}
	if err := v1.AddToScheme(s); err != nil {
		t.Fatal(err)
	}
	return fake.NewClientBuilder().WithScheme(s).WithObjects(objs...).Build()
}

func secret(name string, data map[string][]byte) *corev1.Secret {
	return &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "frp"}, Data: data}
}

func tlsSecret(t *testing.T, name string, dnsNames ...string) *corev1.Secret {
	t.Helper()
	crt, key, err := frpstest.SelfSigned(dnsNames...)
	if err != nil {
		t.Fatal(err)
	}
	return secret(name, map[string][]byte{"tls.crt": crt, "tls.key": key, "ca.crt": crt})
}

func testServer(mutate func(*v1.ServerSpec)) *v1.Server {
	s := &v1.Server{
		ObjectMeta: metav1.ObjectMeta{Name: "public", Namespace: "frp"},
		Spec: v1.ServerSpec{
			Bind:           v1.ServerSpec_Bind{Port: 7000},
			Authentication: &v1.ServerSpec_Authentication{Token: &v1.ClientSpec_Server_Authentication_Token{Secret: v1.Secret{Name: "token", Key: "token"}}},
		},
	}
	if mutate != nil {
		mutate(&s.Spec)
	}
	return s
}

func resolve(t *testing.T, s *v1.Server, objs ...client.Object) (*Resolved, error) {
	t.Helper()
	return Resolve(context.Background(), newFake(t, objs...), s, 50)
}

func mustResolve(t *testing.T, s *v1.Server, objs ...client.Object) *Resolved {
	t.Helper()
	r, err := resolve(t, s, objs...)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	return r
}

func TestResolve_TokenFromSecret(t *testing.T) {
	r := mustResolve(t, testServer(nil), secret("token", map[string][]byte{"token": []byte("s3cret")}))
	if r.Token != "s3cret" || r.NewToken {
		t.Errorf("Token = %q, NewToken = %v", r.Token, r.NewToken)
	}
	if !strings.Contains(r.TOML, `auth.token = "s3cret"`) {
		t.Errorf("TOML =\n%s", r.TOML)
	}
}

func TestResolve_GeneratesTokenOnce(t *testing.T) {
	noAuth := testServer(func(s *v1.ServerSpec) { s.Authentication = nil })

	fresh := mustResolve(t, noAuth)
	if !fresh.NewToken || len(fresh.Token) != 64 {
		t.Fatalf("Token = %q, NewToken = %v; want a new 64-character token", fresh.Token, fresh.NewToken)
	}

	stored := mustResolve(t, noAuth, secret("public-connection", map[string][]byte{"token": []byte("kept")}))
	if stored.Token != "kept" || stored.NewToken {
		t.Errorf("Token = %q, NewToken = %v; want the stored token reused", stored.Token, stored.NewToken)
	}
}

func TestResolve_RejectsMissingOrEmptyToken(t *testing.T) {
	if _, err := resolve(t, testServer(nil), secret("token", map[string][]byte{"other": []byte("x")})); err == nil || !strings.Contains(err.Error(), `key "token" not found`) {
		t.Errorf("missing key: error = %v", err)
	}
	if _, err := resolve(t, testServer(nil), secret("token", map[string][]byte{"token": {}})); err == nil || !strings.Contains(err.Error(), "is empty") {
		t.Errorf("empty token: error = %v", err)
	}
	if _, err := resolve(t, testServer(nil)); err == nil || !strings.Contains(err.Error(), "secret frp/token") {
		t.Errorf("missing Secret: error = %v", err)
	}
}

func TestResolve_TLSCertificateAndClientCA(t *testing.T) {
	s := testServer(func(s *v1.ServerSpec) {
		s.Bind.TLS = &v1.ServerSpec_Bind_TLS{
			Certificate: &v1.SecretNameRef{Secret: v1.SecretName{Name: "frps-cert"}},
			ClientCA:    &v1.SecretNameRef{Secret: v1.SecretName{Name: "client-ca"}},
		}
	})
	cert := tlsSecret(t, "frps-cert", "frps.example.com")
	r := mustResolve(t, s, secret("token", map[string][]byte{"token": []byte("t")}), cert, tlsSecret(t, "client-ca", "ca"))

	if len(r.ServerNames) != 1 || r.ServerNames[0] != "frps.example.com" {
		t.Errorf("ServerNames = %v", r.ServerNames)
	}
	if string(r.ConnectionCA) != string(cert.Data["ca.crt"]) {
		t.Error("ConnectionCA is not the certificate Secret's ca.crt")
	}
	for _, k := range []string{"frps-cert/tls.crt", "frps-cert/tls.key", "client-ca/ca.crt"} {
		if _, ok := r.Mounted[k]; !ok {
			t.Errorf("Mounted is missing %s: %v", k, r.Mounted)
		}
	}
	for _, want := range []string{"transport.tls.force = true", `transport.tls.certFile = "/etc/frps/tls/tls.crt"`, `transport.tls.trustedCaFile = "/etc/frps/tls/ca.crt"`} {
		if !strings.Contains(r.TOML, want) {
			t.Errorf("TOML is missing %q:\n%s", want, r.TOML)
		}
	}
}

func TestResolve_TLSWithoutCertificateUsesFrpsDefault(t *testing.T) {
	s := testServer(func(s *v1.ServerSpec) { s.Bind.TLS = &v1.ServerSpec_Bind_TLS{} })
	r := mustResolve(t, s, secret("token", map[string][]byte{"token": []byte("t")}))
	if !strings.Contains(r.TOML, "transport.tls.force = true") || strings.Contains(r.TOML, "certFile") || r.ServerNames != nil {
		t.Errorf("TOML =\n%s\nServerNames = %v", r.TOML, r.ServerNames)
	}

	off := testServer(func(s *v1.ServerSpec) { s.Bind.TLS = &v1.ServerSpec_Bind_TLS{Force: ptr(false)} })
	if r := mustResolve(t, off, secret("token", map[string][]byte{"token": []byte("t")})); !strings.Contains(r.TOML, "transport.tls.force = false") {
		t.Errorf("TOML =\n%s", r.TOML)
	}
}

func TestResolve_RejectsMismatchedCertificate(t *testing.T) {
	cert := tlsSecret(t, "frps-cert", "frps.example.com")
	cert.Data["tls.key"] = tlsSecret(t, "other", "x").Data["tls.key"]
	s := testServer(func(s *v1.ServerSpec) {
		s.Bind.TLS = &v1.ServerSpec_Bind_TLS{Certificate: &v1.SecretNameRef{Secret: v1.SecretName{Name: "frps-cert"}}}
	})
	if _, err := resolve(t, s, secret("token", map[string][]byte{"token": []byte("t")}), cert); err == nil || !strings.Contains(err.Error(), "bind.tls.certificate frp/frps-cert") {
		t.Errorf("error = %v", err)
	}
}

func TestResolve_GeneratedDashboardLoginIsStable(t *testing.T) {
	s := testServer(func(s *v1.ServerSpec) { s.Dashboard = &v1.ServerSpec_Dashboard{Enabled: true} })
	token := secret("token", map[string][]byte{"token": []byte("t")})

	fresh := mustResolve(t, s, token)
	if !fresh.GeneratedDashboard || fresh.Dashboard == nil || fresh.Dashboard.Username != "admin" || len(fresh.Dashboard.Password) != 32 {
		t.Fatalf("Dashboard = %+v, GeneratedDashboard = %v", fresh.Dashboard, fresh.GeneratedDashboard)
	}
	stored := mustResolve(t, s, token, secret("public-dashboard", map[string][]byte{"username": []byte("admin"), "password": []byte("kept")}))
	if stored.Dashboard.Password != "kept" {
		t.Errorf("Password = %q, want the stored one", stored.Dashboard.Password)
	}
	if !strings.Contains(stored.TOML, `webServer.password = "kept"`) || strings.Contains(stored.TOML, "enablePrometheus") {
		t.Errorf("TOML =\n%s", stored.TOML)
	}
}

func TestResolve_DashboardCredentialsAndTLS(t *testing.T) {
	s := testServer(func(s *v1.ServerSpec) {
		s.Dashboard = &v1.ServerSpec_Dashboard{
			Enabled:     true,
			Credentials: &v1.SecretNameRef{Secret: v1.SecretName{Name: "dash"}},
			TLS:         &v1.ServerSpec_Dashboard_TLS{Certificate: v1.SecretNameRef{Secret: v1.SecretName{Name: "dash-cert"}}},
		}
		s.Metrics = &v1.ServerSpec_Metrics{Enabled: true}
	})
	r := mustResolve(t, s,
		secret("token", map[string][]byte{"token": []byte("t")}),
		secret("dash", map[string][]byte{"username": []byte("ops"), "password": []byte("pw")}),
		tlsSecret(t, "dash-cert", "dashboard.example.com"))
	if r.GeneratedDashboard || r.Dashboard.Username != "ops" {
		t.Errorf("Dashboard = %+v, GeneratedDashboard = %v", r.Dashboard, r.GeneratedDashboard)
	}
	for _, want := range []string{`webServer.user = "ops"`, `webServer.tls.certFile = "/etc/frps/dashboard-tls/tls.crt"`, "enablePrometheus = true"} {
		if !strings.Contains(r.TOML, want) {
			t.Errorf("TOML is missing %q:\n%s", want, r.TOML)
		}
	}
	if _, ok := r.Mounted["dash-cert/tls.crt"]; !ok {
		t.Errorf("Mounted = %v", r.Mounted)
	}
}

func TestResolve_NoWebServerWithoutDashboardOrMetrics(t *testing.T) {
	r := mustResolve(t, testServer(nil), secret("token", map[string][]byte{"token": []byte("t")}))
	if r.Dashboard != nil || strings.Contains(r.TOML, "webServer") {
		t.Errorf("Dashboard = %+v, TOML =\n%s", r.Dashboard, r.TOML)
	}
}

func TestResolve_InvalidSpec(t *testing.T) {
	s := testServer(func(s *v1.ServerSpec) { s.Service = &v1.ServerSpec_Service{Type: "NodePort"} })
	if _, err := resolve(t, s, secret("token", map[string][]byte{"token": []byte("t")})); err == nil {
		t.Fatal("Resolve accepted a NodePort Service")
	}
}
