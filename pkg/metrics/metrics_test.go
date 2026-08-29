package metrics

import (
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrlmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"

	frpv1alpha1 "github.com/zufardhiyaulhaq/frp-operator/api/v1alpha1"
	"github.com/zufardhiyaulhaq/frp-operator/pkg/client/handler"
)

func newClient(ns, name string, ready, synced bool) *frpv1alpha1.Client {
	cond := func(t string, ok bool) metav1.Condition {
		s := metav1.ConditionFalse
		if ok {
			s = metav1.ConditionTrue
		}
		return metav1.Condition{Type: t, Status: s}
	}
	c := &frpv1alpha1.Client{}
	c.Namespace = ns
	c.Name = name
	c.Spec.Server.Host = "frps.example.com"
	c.Spec.Server.Port = 7000
	c.Status.UpstreamCount = 2
	c.Status.VisitorCount = 1
	c.Status.Conditions = []metav1.Condition{cond("Ready", ready), cond("ConfigSynced", synced)}
	return c
}

func TestRecordClient(t *testing.T) {
	t.Cleanup(func() { DeleteClient("ns1", "c1") })
	RecordClient(newClient("ns1", "c1", true, false), "ns1/c1", "fatedier/frpc:v0.71.0")

	want := `
# HELP frp_client_info Static information about a Client. Always 1.
# TYPE frp_client_info gauge
frp_client_info{client="c1",client_id="ns1/c1",frpc_image="fatedier/frpc:v0.71.0",namespace="ns1",server_address="frps.example.com",server_port="7000"} 1
# HELP frp_client_ready 1 when the Client's Ready condition is True.
# TYPE frp_client_ready gauge
frp_client_ready{client="c1",namespace="ns1"} 1
# HELP frp_client_config_synced 1 when the Client's ConfigSynced condition is True.
# TYPE frp_client_config_synced gauge
frp_client_config_synced{client="c1",namespace="ns1"} 0
# HELP frp_client_upstreams Number of Upstream resources attached to the Client.
# TYPE frp_client_upstreams gauge
frp_client_upstreams{client="c1",namespace="ns1"} 2
# HELP frp_client_visitors Number of Visitor resources attached to the Client.
# TYPE frp_client_visitors gauge
frp_client_visitors{client="c1",namespace="ns1"} 1
`
	if err := testutil.GatherAndCompare(ctrlmetrics.Registry, strings.NewReader(want),
		"frp_client_info", "frp_client_ready", "frp_client_config_synced", "frp_client_upstreams", "frp_client_visitors"); err != nil {
		t.Fatal(err)
	}
}

func TestRecordClient_ReplacesInfoWhenLabelsChange(t *testing.T) {
	t.Cleanup(func() { DeleteClient("ns1", "c1") })
	RecordClient(newClient("ns1", "c1", true, true), "ns1/c1", "fatedier/frpc:v0.71.0")
	RecordClient(newClient("ns1", "c1", true, true), "", "fatedier/frpc:v0.72.0")

	if n := testutil.CollectAndCount(clientInfo, "frp_client_info"); n != 1 {
		t.Fatalf("frp_client_info series = %d, want 1 (old label set must be removed)", n)
	}
}

func TestRecordProxies_OneHot(t *testing.T) {
	t.Cleanup(func() { DeleteClient("ns1", "c1") })
	RecordProxies("ns1", "c1", []handler.ProxyStatus{
		{Name: "web", Type: "http", Status: "running", LocalAddr: "10.0.0.5:80", RemoteAddr: "web.example.com"},
		{Name: "ssh", Type: "tcp", Status: "start error", LocalAddr: "10.0.0.6:22", RemoteAddr: ":6000"},
	}, nil)

	want := `
# HELP frp_client_admin_up 1 when the frpc admin API answered GET /api/status.
# TYPE frp_client_admin_up gauge
frp_client_admin_up{client="c1",namespace="ns1"} 1
# HELP frp_proxy_info Static information about a proxy reported by frpc. Always 1.
# TYPE frp_proxy_info gauge
frp_proxy_info{client="c1",local_addr="10.0.0.6:22",namespace="ns1",plugin="",proxy="ssh",remote_addr=":6000",type="tcp"} 1
frp_proxy_info{client="c1",local_addr="10.0.0.5:80",namespace="ns1",plugin="",proxy="web",remote_addr="web.example.com",type="http"} 1
`
	if err := testutil.GatherAndCompare(ctrlmetrics.Registry, strings.NewReader(want), "frp_client_admin_up", "frp_proxy_info"); err != nil {
		t.Fatal(err)
	}

	// one-hot: 6 known phases per proxy, exactly one is 1
	if n := testutil.CollectAndCount(proxyStatus, "frp_proxy_status"); n != 12 {
		t.Fatalf("frp_proxy_status series = %d, want 12", n)
	}
	if v := testutil.ToFloat64(proxyStatus.WithLabelValues("ns1", "c1", "web", "http", "running")); v != 1 {
		t.Errorf("web running = %v, want 1", v)
	}
	if v := testutil.ToFloat64(proxyStatus.WithLabelValues("ns1", "c1", "web", "http", "closed")); v != 0 {
		t.Errorf("web closed = %v, want 0", v)
	}
	if v := testutil.ToFloat64(proxyStatus.WithLabelValues("ns1", "c1", "ssh", "tcp", "start error")); v != 1 {
		t.Errorf("ssh start error = %v, want 1", v)
	}
	if v := testutil.ToFloat64(proxyStatus.WithLabelValues("ns1", "c1", "ssh", "tcp", "running")); v != 0 {
		t.Errorf("ssh running = %v, want 0", v)
	}
}

func TestRecordProxies_UnknownPhaseIsEmitted(t *testing.T) {
	t.Cleanup(func() { DeleteClient("ns1", "c1") })
	RecordProxies("ns1", "c1", []handler.ProxyStatus{{Name: "x", Type: "tcp", Status: "weird"}}, nil)
	if v := testutil.ToFloat64(proxyStatus.WithLabelValues("ns1", "c1", "x", "tcp", "weird")); v != 1 {
		t.Errorf("weird = %v, want 1", v)
	}
	if n := testutil.CollectAndCount(proxyStatus, "frp_proxy_status"); n != 7 {
		t.Fatalf("series = %d, want 7 (6 known + 1 unknown)", n)
	}
}

func TestRecordProxies_PrunesVanishedProxy(t *testing.T) {
	t.Cleanup(func() { DeleteClient("ns1", "c1") })
	RecordProxies("ns1", "c1", []handler.ProxyStatus{
		{Name: "a", Type: "tcp", Status: "running"},
		{Name: "b", Type: "tcp", Status: "running"},
	}, nil)
	RecordProxies("ns1", "c1", []handler.ProxyStatus{
		{Name: "a", Type: "tcp", Status: "closed"},
	}, nil)

	if n := testutil.CollectAndCount(proxyInfo, "frp_proxy_info"); n != 1 {
		t.Errorf("frp_proxy_info series = %d, want 1", n)
	}
	if n := testutil.CollectAndCount(proxyStatus, "frp_proxy_status"); n != 6 {
		t.Errorf("frp_proxy_status series = %d, want 6", n)
	}
	if v := testutil.ToFloat64(proxyStatus.WithLabelValues("ns1", "c1", "a", "tcp", "running")); v != 0 {
		t.Errorf("a running = %v, want 0 after transition", v)
	}
}

func TestRecordProxies_ErrorSetsAdminDownAndPrunes(t *testing.T) {
	t.Cleanup(func() { DeleteClient("ns1", "c1") })
	RecordProxies("ns1", "c1", []handler.ProxyStatus{{Name: "a", Type: "tcp", Status: "running"}}, nil)
	RecordProxies("ns1", "c1", nil, errors.New("connection refused"))

	if v := testutil.ToFloat64(adminUp.WithLabelValues("ns1", "c1")); v != 0 {
		t.Errorf("admin_up = %v, want 0", v)
	}
	if n := testutil.CollectAndCount(proxyStatus, "frp_proxy_status"); n != 0 {
		t.Errorf("frp_proxy_status series = %d, want 0", n)
	}
	if n := testutil.CollectAndCount(proxyInfo, "frp_proxy_info"); n != 0 {
		t.Errorf("frp_proxy_info series = %d, want 0", n)
	}
}

func TestDeleteClient_RemovesOnlyThatClient(t *testing.T) {
	t.Cleanup(func() { DeleteClient("ns1", "c1"); DeleteClient("ns2", "c2") })
	RecordClient(newClient("ns1", "c1", true, true), "ns1/c1", "img")
	RecordClient(newClient("ns2", "c2", true, true), "ns2/c2", "img")
	RecordProxies("ns1", "c1", []handler.ProxyStatus{{Name: "a", Type: "tcp", Status: "running"}}, nil)
	RecordProxies("ns2", "c2", []handler.ProxyStatus{{Name: "b", Type: "tcp", Status: "running"}}, nil)
	SetLastReconcile("ns1", "c1", time.Unix(100, 0))
	SetLastReconcile("ns2", "c2", time.Unix(200, 0))

	DeleteClient("ns1", "c1")

	for name, n := range map[string]int{
		"frp_client_info":                             testutil.CollectAndCount(clientInfo, "frp_client_info"),
		"frp_client_ready":                            testutil.CollectAndCount(clientReady, "frp_client_ready"),
		"frp_client_config_synced":                    testutil.CollectAndCount(clientConfigSynced, "frp_client_config_synced"),
		"frp_client_upstreams":                        testutil.CollectAndCount(clientUpstreams, "frp_client_upstreams"),
		"frp_client_visitors":                         testutil.CollectAndCount(clientVisitors, "frp_client_visitors"),
		"frp_client_admin_up":                         testutil.CollectAndCount(adminUp, "frp_client_admin_up"),
		"frp_client_last_reconcile_timestamp_seconds": testutil.CollectAndCount(lastReconcile, "frp_client_last_reconcile_timestamp_seconds"),
		"frp_proxy_info":                              testutil.CollectAndCount(proxyInfo, "frp_proxy_info"),
	} {
		if n != 1 {
			t.Errorf("%s series = %d, want 1 (only ns2/c2 should remain)", name, n)
		}
	}
	if n := testutil.CollectAndCount(proxyStatus, "frp_proxy_status"); n != 6 {
		t.Errorf("frp_proxy_status series = %d, want 6", n)
	}
	if v := testutil.ToFloat64(lastReconcile.WithLabelValues("ns2", "c2")); v != 200 {
		t.Errorf("last reconcile ns2/c2 = %v, want 200", v)
	}
}

func TestConcurrency(t *testing.T) {
	t.Cleanup(func() { DeleteClient("ns1", "c1"); DeleteClient("ns2", "c2") })

	var wg sync.WaitGroup
	// ~20 goroutines racing on two clients
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			ns := "ns1"
			client := "c1"
			if idx%2 == 0 {
				ns = "ns2"
				client = "c2"
			}
			c := newClient(ns, client, true, true)
			RecordClient(c, ns+"/"+client, "img")
			RecordProxies(ns, client, []handler.ProxyStatus{
				{Name: "proxy-" + string(rune('a'+idx)), Type: "tcp", Status: "running"},
			}, nil)
			SetLastReconcile(ns, client, time.Unix(int64(100+idx), 0))
		}(i)
	}
	wg.Wait()

	// After concurrent operations, delete both clients
	DeleteClient("ns1", "c1")
	DeleteClient("ns2", "c2")

	// Verify all metrics families have zero series for these clients
	metricsToCheck := map[string]*prometheus.GaugeVec{
		"frp_client_info":                             clientInfo,
		"frp_client_ready":                            clientReady,
		"frp_client_config_synced":                    clientConfigSynced,
		"frp_client_upstreams":                        clientUpstreams,
		"frp_client_visitors":                         clientVisitors,
		"frp_client_admin_up":                         adminUp,
		"frp_client_last_reconcile_timestamp_seconds": lastReconcile,
		"frp_proxy_info":                              proxyInfo,
		"frp_proxy_status":                            proxyStatus,
	}
	for name, metric := range metricsToCheck {
		count := testutil.CollectAndCount(metric, name)
		if count != 0 {
			t.Errorf("%s: expected 0 series after DeleteClient for both clients, got %d", name, count)
		}
	}
}
