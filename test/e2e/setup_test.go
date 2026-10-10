//go:build e2e

package e2e

import (
	"bytes"
	"context"
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"text/template"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"

	frpv1alpha1 "github.com/zufardhiyaulhaq/frp-operator/api/v1alpha1"
)

const (
	operatorNamespace = "frp-operator"
	frpsNamespace     = "frps"
	backendsNamespace = "backends"
	clientNamespace   = "e2e-client"
	egressNamespace   = "egress"

	frpsToken     = "e2e-token"
	proxyUser     = "e2e"
	proxyPassword = "p@ss/w0rd" // @ and / must be URL-escaped by the operator; 3proxy forbids ':'

	frpsMainHost    = "frps-main.frps.svc.cluster.local"
	frpsMTLSHost    = "frps-mtls.frps.svc.cluster.local"
	wssHost         = "wss-terminator.frps.svc.cluster.local"
	tcpEchoHost     = "tcp-echo.backends.svc.cluster.local"   // port 9000
	udpEchoHost     = "udp-echo.backends.svc.cluster.local"   // port 9001
	httpEchoHost    = "http-echo.backends.svc.cluster.local"  // port 80 (http), 443 (https)
	egressProxyHost = "egress-proxy.egress.svc.cluster.local" // port 3128 (http), 1080 (socks5)
	metricsURL      = "http://frp-operator-controller-manager-metrics-service.frp-operator.svc.cluster.local:8080/metrics"
)

//go:embed manifests/infra.yaml
var infraManifest string

// Env is the shared state of one suite run.
type Env struct {
	Kube       client.Client
	Clientset  kubernetes.Interface
	Config     *rest.Config
	Certs      *Certs
	FrpsIP     string // frps-main pod IP: cases reach remotePorts here; LoadBalancer publicAddress
	FrpsMTLSIP string // frps-mtls pod IP
	Artifacts  string // E2E_ARTIFACTS_DIR; "" disables dumps
}

var env *Env

func TestMain(m *testing.M) {
	code, err := run(m)
	if err != nil {
		fmt.Fprintln(os.Stderr, "e2e setup failed:", err)
		os.Exit(1)
	}
	os.Exit(code)
}

func run(m *testing.M) (int, error) {
	path := os.Getenv("E2E_KUBECONFIG")
	if path == "" {
		return 0, fmt.Errorf("E2E_KUBECONFIG is not set: run `make test-e2e`")
	}
	raw, err := clientcmd.LoadFromFile(path)
	if err != nil {
		return 0, err
	}
	if err := requireE2EContext(raw); err != nil {
		return 0, err
	}
	cfg, err := clientcmd.NewDefaultClientConfig(*raw, &clientcmd.ConfigOverrides{}).ClientConfig()
	if err != nil {
		return 0, err
	}
	e, err := newEnv(cfg, os.Getenv("E2E_ARTIFACTS_DIR"))
	if err != nil {
		return 0, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	if err := e.setupInfra(ctx); err != nil {
		e.dumpGlobal(context.Background())
		return 0, err
	}
	env = e
	code := m.Run()
	if code != 0 {
		e.dumpGlobal(context.Background())
	}
	return code, nil
}

func newEnv(cfg *rest.Config, artifacts string) (*Env, error) {
	scheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		return nil, err
	}
	if err := frpv1alpha1.AddToScheme(scheme); err != nil {
		return nil, err
	}
	kube, err := client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		return nil, err
	}
	clientset, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, err
	}
	return &Env{Kube: kube, Clientset: clientset, Config: cfg, Artifacts: artifacts}, nil
}

// setupInfra deploys frps, the backends, the egress proxy and the client pod, and waits for them.
func (e *Env) setupInfra(ctx context.Context) error {
	certs, err := newCerts([]string{frpsMainHost, frpsMTLSHost, wssHost})
	if err != nil {
		return err
	}
	e.Certs = certs

	dns := &corev1.Service{}
	if err := e.Kube.Get(ctx, client.ObjectKey{Namespace: "kube-system", Name: "kube-dns"}, dns); err != nil {
		return fmt.Errorf("find cluster DNS: %w", err)
	}
	var manifest bytes.Buffer
	if err := template.Must(template.New("infra").Parse(infraManifest)).Execute(&manifest, map[string]string{
		"Token": frpsToken, "ProxyUser": proxyUser, "ProxyPassword": proxyPassword, "ClusterDNS": dns.Spec.ClusterIP,
	}); err != nil {
		return err
	}
	if err := e.apply(ctx, manifest.Bytes()); err != nil {
		return err
	}
	if err := e.upsert(ctx, &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "frps-tls", Namespace: frpsNamespace},
		Data:       map[string][]byte{"tls.crt": certs.ServerCert, "tls.key": certs.ServerKey, "ca.crt": certs.CA},
	}); err != nil {
		return err
	}
	for _, ns := range []string{frpsNamespace, backendsNamespace, egressNamespace, clientNamespace} {
		if err := e.waitPodsReady(ctx, ns, 4*time.Minute); err != nil {
			return err
		}
	}
	if e.FrpsIP, err = e.podIP(ctx, frpsNamespace, "app=frps-main"); err != nil {
		return err
	}
	if e.FrpsMTLSIP, err = e.podIP(ctx, frpsNamespace, "app=frps-mtls"); err != nil {
		return err
	}
	return nil
}

// dumpGlobal saves the shared components' logs and all events for the CI artifact.
func (e *Env) dumpGlobal(ctx context.Context) {
	if e.Artifacts == "" {
		return
	}
	dir := filepath.Join(e.Artifacts, "_global")
	for _, ns := range []string{operatorNamespace, frpsNamespace, egressNamespace, backendsNamespace} {
		e.dumpPodLogs(ctx, ns, filepath.Join(dir, ns))
	}
	e.dumpEvents(ctx, "", filepath.Join(dir, "events.txt"))
}
