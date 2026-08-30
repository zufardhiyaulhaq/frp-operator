package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	ctrlmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"
)

var (
	lbService = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "frp_loadbalancer_service",
		Help: "LoadBalancer Services handled by the operator; state is bound or pending. Always 1.",
	}, []string{"namespace", "service", "pool", "server", "state"})
	poolAllocated = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "frp_serverpool_allocated_ports",
		Help: "Number of ports allocated on a ServerPool server.",
	}, []string{"pool", "server"})
)

func init() {
	ctrlmetrics.Registry.MustRegister(lbService, poolAllocated)
}

// RecordLoadBalancer writes one frp_loadbalancer_service series per Service, replacing any
// series previously written for it (so state/pool/server flips do not leave stale series).
func RecordLoadBalancer(namespace, service, pool, server, state string) {
	mu.Lock()
	defer mu.Unlock()
	lbService.DeletePartialMatch(prometheus.Labels{"namespace": namespace, "service": service})
	lbService.WithLabelValues(namespace, service, pool, server, state).Set(1)
}

// DeleteLoadBalancer removes every frp_loadbalancer_service series for the Service.
func DeleteLoadBalancer(namespace, service string) {
	mu.Lock()
	defer mu.Unlock()
	lbService.DeletePartialMatch(prometheus.Labels{"namespace": namespace, "service": service})
}

// SetPoolAllocated records the number of ports allocated on a ServerPool server.
func SetPoolAllocated(pool, server string, n int) {
	mu.Lock()
	defer mu.Unlock()
	poolAllocated.WithLabelValues(pool, server).Set(float64(n))
}

// DeletePool removes every frp_serverpool_allocated_ports series for the pool.
func DeletePool(pool string) {
	mu.Lock()
	defer mu.Unlock()
	poolAllocated.DeletePartialMatch(prometheus.Labels{"pool": pool})
}
