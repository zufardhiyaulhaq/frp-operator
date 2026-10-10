//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	frpv1alpha1 "github.com/zufardhiyaulhaq/frp-operator/api/v1alpha1"
)

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func ptr[T any](v T) *T { return &v }

// Case is one feature case: its own namespace, its own Clients, its own remote ports.
type Case struct {
	t  *testing.T
	NS string
}

// newCase runs t in parallel and gives it a fresh namespace e2e-<name> holding the frps token
// Secret `frps-token`. A namespace left over from an earlier run is deleted first. The namespace
// is deleted when the test passes, and kept and dumped when it fails.
func newCase(t *testing.T, name string) *Case {
	t.Helper()
	t.Parallel()
	ctx := context.Background()
	c := &Case{t: t, NS: "e2e-" + name}
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: c.NS}}
	must(t, env.deleteAndWait(ctx, ns))
	must(t, env.Kube.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: c.NS}}))
	c.Secret("frps-token", map[string][]byte{"token": []byte(frpsToken)})
	t.Cleanup(func() {
		if t.Failed() {
			env.dumpCase(context.Background(), c.NS)
			return
		}
		_ = env.Kube.Delete(context.Background(), &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: c.NS}})
	})
	return c
}

func (c *Case) Secret(name string, data map[string][]byte) {
	c.t.Helper()
	must(c.t, env.Kube.Create(context.Background(), &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: c.NS}, Data: data}))
}

// Client creates a Client for frps-main with token auth; mutate adjusts its server spec.
func (c *Case) Client(name string, mutate func(*frpv1alpha1.ClientSpec_Server)) *frpv1alpha1.Client {
	c.t.Helper()
	cl := &frpv1alpha1.Client{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: c.NS},
		Spec: frpv1alpha1.ClientSpec{Server: frpv1alpha1.ClientSpec_Server{
			Host: frpsMainHost,
			Port: 7000,
			Authentication: frpv1alpha1.ClientSpec_Server_Authentication{
				Token: &frpv1alpha1.ClientSpec_Server_Authentication_Token{Secret: frpv1alpha1.Secret{Name: "frps-token", Key: "token"}},
			},
		}},
	}
	if mutate != nil {
		mutate(&cl.Spec.Server)
	}
	must(c.t, env.Kube.Create(context.Background(), cl))
	return cl
}

// Upstream creates an Upstream named <ns>-<short> and returns that name, which is also the frps
// proxy name. frps proxy names are global, so the namespace prefix keeps parallel cases apart.
func (c *Case) Upstream(clientName, short string, spec frpv1alpha1.UpstreamSpec) string {
	c.t.Helper()
	name := c.NS + "-" + short
	spec.Client = clientName
	must(c.t, env.Kube.Create(context.Background(), &frpv1alpha1.Upstream{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: c.NS}, Spec: spec}))
	return name
}

// Visitor creates a Visitor named <ns>-<short> and returns that name.
func (c *Case) Visitor(clientName, short string, spec frpv1alpha1.VisitorSpec) string {
	c.t.Helper()
	name := c.NS + "-" + short
	spec.Client = clientName
	must(c.t, env.Kube.Create(context.Background(), &frpv1alpha1.Visitor{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: c.NS}, Spec: spec}))
	return name
}

// tcpEcho exposes the TCP echo backend on remotePort.
func tcpEcho(remotePort int) frpv1alpha1.UpstreamSpec {
	return frpv1alpha1.UpstreamSpec{TCP: &frpv1alpha1.UpstreamSpec_TCP{
		Host: tcpEchoHost, Port: 9000, Server: frpv1alpha1.UpstreamSpec_TCP_Server{Port: remotePort},
	}}
}

// udpEcho exposes the UDP echo backend on remotePort.
func udpEcho(remotePort int) frpv1alpha1.UpstreamSpec {
	return frpv1alpha1.UpstreamSpec{UDP: &frpv1alpha1.UpstreamSpec_UDP{
		Host: udpEchoHost, Port: 9001, Server: frpv1alpha1.UpstreamSpec_UDP_Server{Port: remotePort},
	}}
}

// waitCondition waits for a Client condition (reason "" matches any) and returns it.
func (c *Case) waitCondition(clientName, condType string, status metav1.ConditionStatus, reason string) *metav1.Condition {
	c.t.Helper()
	var got *metav1.Condition
	eventually(c.t, 3*time.Minute, fmt.Sprintf("Client %s/%s %s=%s %s", c.NS, clientName, condType, status, reason), func(ctx context.Context) error {
		cl := &frpv1alpha1.Client{}
		if err := env.Kube.Get(ctx, client.ObjectKey{Namespace: c.NS, Name: clientName}, cl); err != nil {
			return err
		}
		cond := meta.FindStatusCondition(cl.Status.Conditions, condType)
		if cond == nil || cond.Status != status || (reason != "" && cond.Reason != reason) {
			return fmt.Errorf("condition is %+v", cond)
		}
		got = cond
		return nil
	})
	return got
}

// waitClientSynced waits for Ready=True and ConfigSynced=True.
func (c *Case) waitClientSynced(clientName string) {
	c.t.Helper()
	c.waitCondition(clientName, "Ready", metav1.ConditionTrue, "")
	c.waitCondition(clientName, "ConfigSynced", metav1.ConditionTrue, "")
}

// waitMetric waits until the operator's /metrics reports want for the sample. Metrics refresh on
// every reconcile (30s requeue), and a reload also waits for the ConfigMap volume sync, hence 3m.
func (c *Case) waitMetric(name string, labels map[string]string, want float64) {
	c.t.Helper()
	eventually(c.t, 3*time.Minute, fmt.Sprintf("%s%v = %v", name, labels, want), func(ctx context.Context) error {
		body, err := env.sh(ctx, "curl -sS --max-time 5 "+metricsURL)
		if err != nil {
			return err
		}
		got, ok := metricValue(body, name, labels)
		if !ok || got != want {
			return fmt.Errorf("got %v (present=%v)", got, ok)
		}
		return nil
	})
}

// waitProxyRunning waits until frpc reports proxy as running, through the operator's metrics.
func (c *Case) waitProxyRunning(clientName, proxy string) {
	c.t.Helper()
	c.waitMetric("frp_proxy_status", map[string]string{"namespace": c.NS, "client": clientName, "proxy": proxy, "status": "running"}, 1)
}

// waitFrpcLog waits until the frpc pod of clientName logs a line containing substr.
func (c *Case) waitFrpcLog(clientName, substr string) {
	c.t.Helper()
	eventually(c.t, 3*time.Minute, fmt.Sprintf("frpc %s logs %q", clientName, substr), func(ctx context.Context) error {
		out, err := env.logs(ctx, c.NS, clientName+"-frpc", "frpc")
		if err != nil {
			return err
		}
		if !strings.Contains(out, substr) {
			return fmt.Errorf("not logged yet")
		}
		return nil
	})
}

// waitEvent waits for a Warning event with reason on objectName.
func (c *Case) waitEvent(objectName, reason string) {
	c.t.Helper()
	eventually(c.t, 90*time.Second, fmt.Sprintf("Warning %s event on %s", reason, objectName), func(ctx context.Context) error {
		events, err := env.Clientset.CoreV1().Events(c.NS).List(ctx, metav1.ListOptions{
			FieldSelector: "involvedObject.name=" + objectName + ",reason=" + reason + ",type=Warning",
		})
		if err != nil {
			return err
		}
		if len(events.Items) == 0 {
			return fmt.Errorf("no event yet")
		}
		return nil
	})
}

// pod returns the frpc pod of clientName.
func (c *Case) pod(clientName string) *corev1.Pod {
	c.t.Helper()
	p := &corev1.Pod{}
	must(c.t, env.Kube.Get(context.Background(), client.ObjectKey{Namespace: c.NS, Name: clientName + "-frpc"}, p))
	return p
}
