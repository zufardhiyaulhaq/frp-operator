package frps

import (
	"reflect"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	v1 "github.com/zufardhiyaulhaq/frp-operator/api/v1alpha1"
)

func tlsServer() *v1.Server {
	return &v1.Server{ObjectMeta: metav1.ObjectMeta{Name: "public", Namespace: "frp"}, Spec: v1.ServerSpec{
		Bind: v1.ServerSpec_Bind{Port: 7000, TLS: &v1.ServerSpec_Bind_TLS{
			Certificate: &v1.SecretNameRef{Secret: v1.SecretName{Name: "frps-cert"}},
			ClientCA:    &v1.SecretNameRef{Secret: v1.SecretName{Name: "client-ca"}},
		}},
		Dashboard: &v1.ServerSpec_Dashboard{Enabled: true, TLS: &v1.ServerSpec_Dashboard_TLS{Certificate: v1.SecretNameRef{Secret: v1.SecretName{Name: "dash-cert"}}}},
	}}
}

func mustPorts(t *testing.T, s *v1.Server) *Ports {
	t.Helper()
	p, err := Validate(&s.Spec, 50)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// projected maps each file of a projected volume to "<secret>/<key>".
func projected(pod *corev1.Pod, volume string) map[string]string {
	for _, v := range pod.Spec.Volumes {
		if v.Name == volume && v.Projected != nil {
			got := map[string]string{}
			for _, src := range v.Projected.Sources {
				for _, it := range src.Secret.Items {
					got[it.Path] = src.Secret.Name + "/" + it.Key
				}
			}
			return got
		}
	}
	return nil
}

func TestBuildPod_MountsConfigAndCertificates(t *testing.T) {
	s := tlsServer()
	pod := BuildPod(s, mustPorts(t, s))

	if pod.Name != "public-frps" || pod.Namespace != "frp" || pod.Labels[SelectorLabel] != "public" {
		t.Fatalf("pod meta = %+v", pod.ObjectMeta)
	}
	c := pod.Spec.Containers[0]
	if c.Image != Image || !reflect.DeepEqual(c.Command, []string{"frps", "-c", "/etc/frps/config/frps.toml"}) {
		t.Errorf("container = %s %v", c.Image, c.Command)
	}
	if c.ReadinessProbe == nil || c.ReadinessProbe.TCPSocket == nil || c.ReadinessProbe.TCPSocket.Port.IntVal != 7000 {
		t.Errorf("readinessProbe = %+v", c.ReadinessProbe)
	}
	sc := c.SecurityContext
	if sc == nil || sc.RunAsNonRoot == nil || !*sc.RunAsNonRoot || sc.RunAsUser == nil || *sc.RunAsUser != 65532 ||
		sc.AllowPrivilegeEscalation == nil || *sc.AllowPrivilegeEscalation || sc.Capabilities == nil ||
		!reflect.DeepEqual(sc.Capabilities.Drop, []corev1.Capability{"ALL"}) {
		t.Errorf("securityContext = %+v", sc)
	}
	if got, want := projected(pod, "tls"), map[string]string{"tls.crt": "frps-cert/tls.crt", "tls.key": "frps-cert/tls.key", "ca.crt": "client-ca/ca.crt"}; !reflect.DeepEqual(got, want) {
		t.Errorf("tls files = %v, want %v", got, want)
	}
	if got, want := projected(pod, "dashboard-tls"), map[string]string{"tls.crt": "dash-cert/tls.crt", "tls.key": "dash-cert/tls.key"}; !reflect.DeepEqual(got, want) {
		t.Errorf("dashboard-tls files = %v, want %v", got, want)
	}
	mounts := map[string]string{}
	for _, m := range c.VolumeMounts {
		mounts[m.Name] = m.MountPath
	}
	if want := map[string]string{"config": ConfigDir, "tls": TLSDir, "dashboard-tls": DashboardTLSDir}; !reflect.DeepEqual(mounts, want) {
		t.Errorf("mounts = %v, want %v", mounts, want)
	}
	if pod.Spec.Volumes[0].Secret == nil || pod.Spec.Volumes[0].Secret.SecretName != "public-frps-config" {
		t.Errorf("config volume = %+v", pod.Spec.Volumes[0])
	}
	ports := map[string]int32{}
	for _, p := range c.Ports {
		ports[p.Name] = p.ContainerPort
	}
	if want := map[string]int32{"bind": 7000, "web": 7500}; !reflect.DeepEqual(ports, want) {
		t.Errorf("container ports = %v, want %v", ports, want)
	}
}

func TestBuildPod_LowPortsUseTheUnprivilegedPortSysctl(t *testing.T) {
	s := &v1.Server{ObjectMeta: metav1.ObjectMeta{Name: "public", Namespace: "frp"}, Spec: v1.ServerSpec{Bind: v1.ServerSpec_Bind{Port: 7000}}}
	if pod := BuildPod(s, mustPorts(t, s)); pod.Spec.SecurityContext != nil {
		t.Errorf("securityContext = %+v, want none without low ports", pod.Spec.SecurityContext)
	}
	s.Spec.Vhost = &v1.ServerSpec_Vhost{HTTPPort: ptr(int32(80))}
	pod := BuildPod(s, mustPorts(t, s))
	want := []corev1.Sysctl{{Name: "net.ipv4.ip_unprivileged_port_start", Value: "0"}}
	if pod.Spec.SecurityContext == nil || !reflect.DeepEqual(pod.Spec.SecurityContext.Sysctls, want) {
		t.Errorf("securityContext = %+v", pod.Spec.SecurityContext)
	}
}

func TestBuildPod_PodTemplateCannotChangeTheSelector(t *testing.T) {
	s := tlsServer()
	s.Spec.PodTemplate = &v1.ClientSpec_PodTemplate{Labels: map[string]string{SelectorLabel: "other", "team": "net"}}
	pod := BuildPod(s, mustPorts(t, s))
	if pod.Labels[SelectorLabel] != "public" || pod.Labels["team"] != "net" {
		t.Errorf("labels = %v", pod.Labels)
	}
}

func TestConfigHash(t *testing.T) {
	spec := corev1.PodSpec{Containers: []corev1.Container{{Image: Image}}}
	mounted := map[string][]byte{"x/tls.crt": []byte("1"), "y/ca.crt": []byte("2")}
	base := ConfigHash("a", mounted, spec)
	if again := ConfigHash("a", map[string][]byte{"y/ca.crt": []byte("2"), "x/tls.crt": []byte("1")}, spec); again != base {
		t.Error("the same inputs gave a different hash")
	}
	otherSpec := spec.DeepCopy()
	otherSpec.Containers[0].Image = "fatedier/frps:v9"
	for name, h := range map[string]string{
		"toml":         ConfigHash("b", mounted, spec),
		"mounted file": ConfigHash("a", map[string][]byte{"x/tls.crt": []byte("rotated"), "y/ca.crt": []byte("2")}, spec),
		"pod spec":     ConfigHash("a", mounted, *otherSpec),
	} {
		if h == base {
			t.Errorf("a %s change kept the hash", name)
		}
	}
}

func TestBuildService(t *testing.T) {
	s := tlsServer()
	svc := BuildService(s, mustPorts(t, s))
	if svc.Name != "public-frps" || svc.Spec.Type != corev1.ServiceTypeLoadBalancer ||
		svc.Spec.ExternalTrafficPolicy != corev1.ServiceExternalTrafficPolicyCluster || svc.Spec.LoadBalancerClass != nil ||
		svc.Labels[ServiceRoleLabel] != "proxy" || svc.Spec.Selector[SelectorLabel] != "public" || len(svc.Spec.Ports) != 1 {
		t.Errorf("service = %+v", svc)
	}

	s.Spec.Service = &v1.ServerSpec_Service{Type: "LoadBalancer", LoadBalancerClass: "example.com/lb", ExternalTrafficPolicy: "Local",
		Annotations: map[string]string{"metallb.universe.tf/loadBalancerIPs": "203.0.113.10"}}
	svc = BuildService(s, mustPorts(t, s))
	if svc.Spec.LoadBalancerClass == nil || *svc.Spec.LoadBalancerClass != "example.com/lb" || svc.Spec.ExternalTrafficPolicy != "Local" ||
		svc.Annotations["metallb.universe.tf/loadBalancerIPs"] != "203.0.113.10" {
		t.Errorf("service = %+v", svc)
	}

	s.Spec.Service = &v1.ServerSpec_Service{Type: "ClusterIP"}
	if svc = BuildService(s, mustPorts(t, s)); svc.Spec.Type != corev1.ServiceTypeClusterIP || svc.Spec.ExternalTrafficPolicy != "" {
		t.Errorf("ClusterIP service = %+v", svc.Spec)
	}
}

func TestBuildWebService(t *testing.T) {
	s := tlsServer()
	svc := BuildWebService(s)
	if svc.Name != "public-frps-web" || svc.Spec.Type != corev1.ServiceTypeClusterIP || svc.Labels[ServiceRoleLabel] != "web" ||
		len(svc.Spec.Ports) != 1 || svc.Spec.Ports[0].Port != 7500 || svc.Spec.Ports[0].Name != "web" {
		t.Errorf("web service = %+v", svc)
	}
	s.Spec.Dashboard.Service = &v1.ServerSpec_Dashboard_Service{Type: "LoadBalancer", LoadBalancerSourceRanges: []string{"10.0.0.0/8"}}
	if svc = BuildWebService(s); svc.Spec.Type != corev1.ServiceTypeLoadBalancer || !reflect.DeepEqual(svc.Spec.LoadBalancerSourceRanges, []string{"10.0.0.0/8"}) {
		t.Errorf("web service = %+v", svc.Spec)
	}
}

func TestBuildServiceMonitor(t *testing.T) {
	s := tlsServer()
	sm := BuildServiceMonitor(s)
	if sm.GetAPIVersion() != "monitoring.coreos.com/v1" || sm.GetKind() != "ServiceMonitor" || sm.GetName() != "public-frps" {
		t.Fatalf("meta = %v", sm.Object)
	}
	endpoints, _, _ := unstructured.NestedSlice(sm.Object, "spec", "endpoints")
	ep := endpoints[0].(map[string]interface{})
	user, _, _ := unstructured.NestedString(ep, "basicAuth", "username", "name")
	if ep["scheme"] != "https" || ep["port"] != "web" || user != "public-dashboard" {
		t.Errorf("endpoint = %v", ep)
	}
	role, _, _ := unstructured.NestedString(sm.Object, "spec", "selector", "matchLabels", ServiceRoleLabel)
	if role != "web" {
		t.Errorf("selector role = %q", role)
	}
}

func TestKeepNodePorts(t *testing.T) {
	current := []corev1.ServicePort{{Name: "bind", Protocol: corev1.ProtocolTCP, Port: 7000, NodePort: 31000}, {Name: "kcp", Protocol: corev1.ProtocolUDP, Port: 7000, NodePort: 31001}}
	desired := []corev1.ServicePort{{Name: "bind", Protocol: corev1.ProtocolTCP, Port: 7000}, {Name: "tcp-8000", Protocol: corev1.ProtocolTCP, Port: 8000}}
	got := KeepNodePorts(current, desired, corev1.ServiceTypeLoadBalancer)
	if got[0].NodePort != 31000 || got[1].NodePort != 0 {
		t.Errorf("LoadBalancer ports = %+v", got)
	}
	desired = []corev1.ServicePort{{Name: "bind", Protocol: corev1.ProtocolTCP, Port: 7000}}
	if got := KeepNodePorts(current, desired, corev1.ServiceTypeClusterIP); got[0].NodePort != 0 {
		t.Errorf("ClusterIP ports = %+v", got)
	}
}

func TestAddress(t *testing.T) {
	svc := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "public-frps", Namespace: "frp"}}
	svc.Spec.Type = corev1.ServiceTypeClusterIP
	if got := Address(svc); got != "public-frps.frp.svc" {
		t.Errorf("ClusterIP address = %q", got)
	}
	svc.Spec.Type = corev1.ServiceTypeLoadBalancer
	if got := Address(svc); got != "" {
		t.Errorf("pending address = %q", got)
	}
	svc.Status.LoadBalancer.Ingress = []corev1.LoadBalancerIngress{{Hostname: "abc.elb.amazonaws.com"}}
	if got := Address(svc); got != "abc.elb.amazonaws.com" {
		t.Errorf("hostname address = %q", got)
	}
	svc.Status.LoadBalancer.Ingress = []corev1.LoadBalancerIngress{{IP: "203.0.113.10"}}
	if got := Address(svc); got != "203.0.113.10" {
		t.Errorf("IP address = %q", got)
	}
}

func TestDashboardInsecure(t *testing.T) {
	s := tlsServer()
	s.Spec.Dashboard.Service = &v1.ServerSpec_Dashboard_Service{Type: "LoadBalancer"}
	if DashboardInsecure(s) {
		t.Error("a dashboard with TLS is reported insecure")
	}
	s.Spec.Dashboard.TLS = nil
	if !DashboardInsecure(s) {
		t.Error("an exposed dashboard without TLS is not reported")
	}
	s.Spec.Dashboard.Service.Type = "ClusterIP"
	if DashboardInsecure(s) {
		t.Error("a ClusterIP dashboard is reported insecure")
	}
}

func TestSecretNames(t *testing.T) {
	s := tlsServer()
	s.Spec.Authentication = &v1.ServerSpec_Authentication{Token: &v1.ClientSpec_Server_Authentication_Token{Secret: v1.Secret{Name: "token", Key: "token"}}}
	want := []string{"public-frps-config", "public-connection", "public-dashboard", "token", "frps-cert", "client-ca", "dash-cert"}
	if got := SecretNames(s); !reflect.DeepEqual(got, want) {
		t.Errorf("SecretNames = %v, want %v", got, want)
	}
}
