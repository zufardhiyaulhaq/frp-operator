// Package metrics exposes FRP-specific Prometheus gauges on the controller-runtime
// metrics registry. The reconciler is the only writer; values are refreshed every
// reconcile and removed when a Client disappears.
package metrics

import (
	"strconv"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrlmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"

	frpv1alpha1 "github.com/zufardhiyaulhaq/frp-operator/api/v1alpha1"
	"github.com/zufardhiyaulhaq/frp-operator/pkg/client/handler"
	"github.com/zufardhiyaulhaq/frp-operator/pkg/client/status"
)

// knownPhases are frpc's proxy phases (client/proxy/proxy_wrapper.go in frp v0.71.0).
var knownPhases = []string{"new", "wait start", "start error", "running", "check failed", "closed"}

var (
	clientInfo = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "frp_client_info",
		Help: "Static information about a Client. Always 1.",
	}, []string{"namespace", "client", "server_address", "server_port", "client_id", "frpc_image"})
	clientReady = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "frp_client_ready",
		Help: "1 when the Client's Ready condition is True.",
	}, []string{"namespace", "client"})
	clientConfigSynced = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "frp_client_config_synced",
		Help: "1 when the Client's ConfigSynced condition is True.",
	}, []string{"namespace", "client"})
	clientUpstreams = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "frp_client_upstreams",
		Help: "Number of Upstream resources attached to the Client.",
	}, []string{"namespace", "client"})
	clientVisitors = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "frp_client_visitors",
		Help: "Number of Visitor resources attached to the Client.",
	}, []string{"namespace", "client"})
	adminUp = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "frp_client_admin_up",
		Help: "1 when the frpc admin API answered GET /api/status.",
	}, []string{"namespace", "client"})
	lastReconcile = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "frp_client_last_reconcile_timestamp_seconds",
		Help: "Unix timestamp of the last completed reconcile of the Client.",
	}, []string{"namespace", "client"})
	proxyStatus = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "frp_proxy_status",
		Help: "One-hot proxy phase as reported by frpc: exactly one series per proxy has value 1.",
	}, []string{"namespace", "client", "proxy", "type", "status"})
	proxyInfo = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "frp_proxy_info",
		Help: "Static information about a proxy reported by frpc. Always 1.",
	}, []string{"namespace", "client", "proxy", "type", "local_addr", "remote_addr", "plugin"})
)

func init() {
	ctrlmetrics.Registry.MustRegister(clientInfo, clientReady, clientConfigSynced, clientUpstreams,
		clientVisitors, adminUp, lastReconcile, proxyStatus, proxyInfo)
}

type clientKey struct{ namespace, client string }

// written remembers the label sets last written for each Client so stale series can be pruned.
type written struct {
	info   prometheus.Labels
	status []prometheus.Labels
	pinfo  []prometheus.Labels
}

var (
	mu    sync.Mutex
	state = map[clientKey]*written{}
)

func get(k clientKey) *written {
	w, ok := state[k]
	if !ok {
		w = &written{}
		state[k] = w
	}
	return w
}

func conditionTrue(c *frpv1alpha1.Client, conditionType string) float64 {
	for _, cond := range c.Status.Conditions {
		if cond.Type == conditionType && cond.Status == metav1.ConditionTrue {
			return 1
		}
	}
	return 0
}

// RecordClient writes the frp_client_* gauges from the Client's spec and status.
// clientID is the resolved frpc clientID ("" when omitted); image is the frpc image in use.
func RecordClient(c *frpv1alpha1.Client, clientID, image string) {
	mu.Lock()
	defer mu.Unlock()
	k := clientKey{c.Namespace, c.Name}
	w := get(k)

	labels := prometheus.Labels{
		"namespace":      c.Namespace,
		"client":         c.Name,
		"server_address": c.Spec.Server.Host,
		"server_port":    strconv.Itoa(c.Spec.Server.Port),
		"client_id":      clientID,
		"frpc_image":     image,
	}
	if w.info != nil {
		clientInfo.Delete(w.info)
	}
	clientInfo.With(labels).Set(1)
	w.info = labels

	clientReady.WithLabelValues(c.Namespace, c.Name).Set(conditionTrue(c, status.ConditionTypeReady))
	clientConfigSynced.WithLabelValues(c.Namespace, c.Name).Set(conditionTrue(c, status.ConditionTypeConfigSync))
	clientUpstreams.WithLabelValues(c.Namespace, c.Name).Set(float64(c.Status.UpstreamCount))
	clientVisitors.WithLabelValues(c.Namespace, c.Name).Set(float64(c.Status.VisitorCount))
}

// SetLastReconcile records when the Client was last reconciled.
func SetLastReconcile(namespace, client string, t time.Time) {
	mu.Lock()
	defer mu.Unlock()
	lastReconcile.WithLabelValues(namespace, client).Set(float64(t.Unix()))
}

// RecordProxies writes frp_client_admin_up and the frp_proxy_* gauges. When statusErr is
// non-nil the admin API is reported down and all proxy series for the Client are removed.
func RecordProxies(namespace, client string, proxies []handler.ProxyStatus, statusErr error) {
	mu.Lock()
	defer mu.Unlock()
	k := clientKey{namespace, client}
	w := get(k)

	for _, l := range w.status {
		proxyStatus.Delete(l)
	}
	for _, l := range w.pinfo {
		proxyInfo.Delete(l)
	}
	w.status, w.pinfo = nil, nil

	if statusErr != nil {
		adminUp.WithLabelValues(namespace, client).Set(0)
		return
	}
	adminUp.WithLabelValues(namespace, client).Set(1)

	for _, p := range proxies {
		phases := knownPhases
		known := false
		for _, ph := range knownPhases {
			if ph == p.Status {
				known = true
				break
			}
		}
		if !known {
			phases = append(append([]string{}, knownPhases...), p.Status)
		}
		for _, ph := range phases {
			l := prometheus.Labels{"namespace": namespace, "client": client, "proxy": p.Name, "type": p.Type, "status": ph}
			v := 0.0
			if ph == p.Status {
				v = 1
			}
			proxyStatus.With(l).Set(v)
			w.status = append(w.status, l)
		}
		il := prometheus.Labels{"namespace": namespace, "client": client, "proxy": p.Name, "type": p.Type,
			"local_addr": p.LocalAddr, "remote_addr": p.RemoteAddr, "plugin": p.Plugin}
		proxyInfo.With(il).Set(1)
		w.pinfo = append(w.pinfo, il)
	}
}

// DeleteClient removes every series belonging to the Client.
func DeleteClient(namespace, client string) {
	mu.Lock()
	defer mu.Unlock()
	delete(state, clientKey{namespace, client})

	match := prometheus.Labels{"namespace": namespace, "client": client}
	for _, v := range []*prometheus.GaugeVec{clientInfo, clientReady, clientConfigSynced, clientUpstreams,
		clientVisitors, adminUp, lastReconcile, proxyStatus, proxyInfo} {
		v.DeletePartialMatch(match)
	}
}
