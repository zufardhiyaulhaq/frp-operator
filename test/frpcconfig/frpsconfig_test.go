//go:build frpcverify

package frpcconfig

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/zufardhiyaulhaq/frp-operator/api/v1alpha1"
	"github.com/zufardhiyaulhaq/frp-operator/pkg/frps"
	"github.com/zufardhiyaulhaq/frp-operator/pkg/frps/frpstest"
)

// TestGeneratedServerConfigsPassFrpsVerify renders frps.toml for every Server feature and every
// Server under examples/ through frps.Resolve, then validates each with the real frps binary.
func TestGeneratedServerConfigsPassFrpsVerify(t *testing.T) {
	image := os.Getenv("FRPS_IMAGE")
	if image == "" {
		t.Fatal("FRPS_IMAGE is not set; run via `make test-frpc-config`")
	}
	if image != frps.Image {
		t.Fatalf("FRPS_IMAGE %q differs from frps.Image %q", image, frps.Image)
	}

	all := renderServerFeatureCases(t)
	all = append(all, renderServerExamples(t, loadExamples(t))...)

	dir := t.TempDir()
	if keep := os.Getenv("FRPC_CONFIG_DIR"); keep != "" {
		abs, err := filepath.Abs(filepath.Join(keep, "frps"))
		if err == nil {
			err = os.MkdirAll(abs, 0o755)
		}
		if err != nil {
			t.Fatalf("FRPC_CONFIG_DIR %q: %v", keep, err)
		}
		dir = abs
	}
	for _, r := range all {
		for _, want := range r.expect {
			if !strings.Contains(r.config, want) {
				t.Errorf("%s: rendered frps.toml is missing %q\n%s", r.name, want, r.config)
			}
		}
		if err := os.WriteFile(filepath.Join(dir, r.name+".toml"), []byte(r.config), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	results := binaryVerify(t, image, "frps", dir)
	for _, r := range all {
		out, ok := results[r.name+".toml"]
		if !ok {
			t.Errorf("%s: frps verify produced no result", r.name)
			continue
		}
		if !strings.Contains(out, "syntax is ok") {
			t.Errorf("%s: frps verify failed: %s\n%s", r.name, strings.TrimSpace(out), r.config)
		}
	}
	t.Logf("verified %d generated server configs with %s", len(all), image)
}

func ref(name string) *v1.SecretNameRef { return &v1.SecretNameRef{Secret: v1.SecretName{Name: name}} }

// issuedCertificate is what cert-manager (or any issuer) puts in a TLS Secret.
func issuedCertificate(t *testing.T, name, namespace string) *corev1.Secret {
	t.Helper()
	crt, key, err := frpstest.SelfSigned("frps.example.com")
	if err != nil {
		t.Fatal(err)
	}
	return &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Data: map[string][]byte{"tls.crt": crt, "tls.key": key, "ca.crt": crt}}
}

func resolveServer(t *testing.T, objs []client.Object, s *v1.Server) (string, error) {
	t.Helper()
	k8s := fake.NewClientBuilder().WithScheme(scheme()).WithObjects(objs...).Build()
	res, err := frps.Resolve(context.Background(), k8s, s, 50)
	if err != nil {
		return "", err
	}
	return res.TOML, nil
}

type serverCase struct {
	name    string
	server  *v1.Server
	objects []client.Object
	expect  []string
}

func renderServerFeatureCases(t *testing.T) []rendered {
	base := func(mutate func(*v1.ServerSpec)) *v1.Server {
		s := &v1.Server{ObjectMeta: metav1.ObjectMeta{Name: "s", Namespace: ns}, Spec: v1.ServerSpec{
			Bind:           v1.ServerSpec_Bind{Port: 7000},
			Authentication: &v1.ServerSpec_Authentication{Token: &v1.ClientSpec_Server_Authentication_Token{Secret: sec("token")}},
		}}
		if mutate != nil {
			mutate(&s.Spec)
		}
		return s
	}
	cases := []serverCase{
		{name: "server_minimal", server: base(nil),
			expect: []string{"bindPort = 7000", `auth.token = "TOKEN-VAL"`, "{ start = 2, end = 1 }"}},
		{name: "server_generated_token", server: base(func(s *v1.ServerSpec) { s.Authentication = nil }),
			expect: []string{`auth.token = "`}},
		{name: "server_all_ports", server: base(func(s *v1.ServerSpec) {
			s.Bind.KCPPort, s.Bind.QUICPort = ptr(int32(7000)), ptr(int32(7001))
			s.AllowPorts = &v1.ServerSpec_AllowPorts{TCP: []v1.PortRange{"8000-8010"}, UDP: []v1.PortRange{"5353"}}
			s.Vhost = &v1.ServerSpec_Vhost{HTTPPort: ptr(int32(80)), HTTPSPort: ptr(int32(443)), TCPMuxPort: ptr(int32(5002)), SubdomainHost: "example.com"}
		}), expect: []string{"kcpBindPort = 7000", "quicBindPort = 7001", "vhostHTTPPort = 80", "vhostHTTPSPort = 443",
			"tcpmuxHTTPConnectPort = 5002", `subDomainHost = "example.com"`, "{ start = 8000, end = 8010 }", "{ single = 5353 }"}},
		{name: "server_tls_mtls", objects: []client.Object{issuedCertificate(t, "frps-cert", ns), issuedCertificate(t, "client-ca", ns)},
			server: base(func(s *v1.ServerSpec) {
				s.Bind.TLS = &v1.ServerSpec_Bind_TLS{Certificate: ref("frps-cert"), ClientCA: ref("client-ca")}
			}), expect: []string{"transport.tls.force = true", `transport.tls.certFile = "/etc/frps/tls/tls.crt"`, `transport.tls.trustedCaFile = "/etc/frps/tls/ca.crt"`}},
		{name: "server_tls_random_certificate_not_forced", server: base(func(s *v1.ServerSpec) {
			s.Bind.TLS = &v1.ServerSpec_Bind_TLS{Force: ptr(false)}
		}), expect: []string{"transport.tls.force = false"}},
		{name: "server_dashboard_metrics_tls", objects: []client.Object{issuedCertificate(t, "dash-cert", ns)},
			server: base(func(s *v1.ServerSpec) {
				s.Dashboard = &v1.ServerSpec_Dashboard{Enabled: true, Credentials: ref("creds"),
					TLS: &v1.ServerSpec_Dashboard_TLS{Certificate: *ref("dash-cert")}}
				s.Metrics = &v1.ServerSpec_Metrics{Enabled: true}
			}), expect: []string{"webServer.port = 7500", `webServer.user = "DASH-USER-VAL"`, "enablePrometheus = true",
				`webServer.tls.certFile = "/etc/frps/dashboard-tls/tls.crt"`}},
		{name: "server_metrics_only", server: base(func(s *v1.ServerSpec) { s.Metrics = &v1.ServerSpec_Metrics{Enabled: true} }),
			expect: []string{"webServer.port = 7500", "enablePrometheus = true"}},
	}

	var out []rendered
	for _, sc := range cases {
		config, err := resolveServer(t, append(sc.objects, credsSecret()), sc.server)
		if err != nil {
			t.Errorf("%s: %v", sc.name, err)
			continue
		}
		out = append(out, rendered{name: sc.name, config: config, expect: sc.expect})
	}
	return out
}

// renderServerExamples renders every Server under examples/. TLS Secrets the example only asks
// cert-manager for (a Certificate) are issued here.
func renderServerExamples(t *testing.T, sets map[string]*exampleSet) []rendered {
	var out []rendered
	for key, set := range sets {
		for _, s := range set.servers {
			objs := append([]client.Object{}, set.objects...)
			have := map[string]bool{}
			for _, o := range objs {
				if _, ok := o.(*corev1.Secret); ok {
					have[o.GetNamespace()+"/"+o.GetName()] = true
				}
			}
			var wanted []string
			if tl := s.Spec.Bind.TLS; tl != nil {
				if tl.Certificate != nil {
					wanted = append(wanted, tl.Certificate.Secret.Name)
				}
				if tl.ClientCA != nil {
					wanted = append(wanted, tl.ClientCA.Secret.Name)
				}
			}
			if d := s.Spec.Dashboard; d != nil && d.TLS != nil {
				wanted = append(wanted, d.TLS.Certificate.Secret.Name)
			}
			for _, name := range wanted {
				if !have[s.Namespace+"/"+name] {
					objs = append(objs, issuedCertificate(t, name, s.Namespace))
					have[s.Namespace+"/"+name] = true
				}
			}
			config, err := resolveServer(t, objs, s)
			if err != nil {
				t.Errorf("examples/%s: Server %s: %v", key, s.Name, err)
				continue
			}
			out = append(out, rendered{name: "example_" + strings.ReplaceAll(key, string(filepath.Separator), "_") + "_server_" + s.Name, config: config})
		}
	}
	return out
}
