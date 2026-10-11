package e2e

import "testing"

func TestMetricValue(t *testing.T) {
	body := `# HELP frp_proxy_status One-hot proxy phase
# TYPE frp_proxy_status gauge
frp_client_config_synced_total 3
frp_proxy_status{client="frpc",namespace="e2e-tcp",proxy="e2e-tcp-echo",status="running",type="tcp"} 1
frp_proxy_status{client="frpc",namespace="e2e-tcp",proxy="e2e-tcp-echo",status="start error",type="tcp"} 0
frp_client_config_synced{client="frpc",namespace="e2e-tcp"} 0
`
	tests := []struct {
		name   string
		metric string
		labels map[string]string
		want   float64
		ok     bool
	}{
		{"running phase", "frp_proxy_status", map[string]string{"namespace": "e2e-tcp", "proxy": "e2e-tcp-echo", "status": "running"}, 1, true},
		{"label value with a space", "frp_proxy_status", map[string]string{"proxy": "e2e-tcp-echo", "status": "start error"}, 0, true},
		{"unknown proxy", "frp_proxy_status", map[string]string{"proxy": "nope"}, 0, false},
		{"label values match exactly, not by prefix", "frp_proxy_status", map[string]string{"proxy": "e2e-tcp"}, 0, false},
		{"a longer metric name is not matched", "frp_client_config_synced", nil, 0, true},
		{"metric without labels", "frp_client_config_synced_total", nil, 3, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := metricValue(body, tt.metric, tt.labels)
			if ok != tt.ok || got != tt.want {
				t.Fatalf("metricValue() = (%v, %v), want (%v, %v)", got, ok, tt.want, tt.ok)
			}
		})
	}
}
