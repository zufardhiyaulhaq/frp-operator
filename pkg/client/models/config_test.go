package models

import (
	"testing"

	frpv1alpha1 "github.com/zufardhiyaulhaq/frp-operator/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
