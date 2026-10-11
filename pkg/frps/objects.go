package frps

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"maps"
	"slices"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/intstr"

	v1 "github.com/zufardhiyaulhaq/frp-operator/api/v1alpha1"
	"github.com/zufardhiyaulhaq/frp-operator/pkg/podtemplate"
)

// nonRootUser runs frps; the upstream image would run it as root.
const nonRootUser int64 = 65532

// Labels are the labels of every object the operator creates for s.
func Labels(s *v1.Server) map[string]string {
	return map[string]string{
		"app.kubernetes.io/name":       "frps",
		"app.kubernetes.io/instance":   s.Name,
		"app.kubernetes.io/managed-by": "frp-operator",
		SelectorLabel:                  s.Name,
	}
}

func objectMeta(s *v1.Server, name string) metav1.ObjectMeta {
	return metav1.ObjectMeta{Name: name, Namespace: s.Namespace, Labels: Labels(s)}
}

func BuildConfigSecret(s *v1.Server, toml string) *corev1.Secret {
	return &corev1.Secret{ObjectMeta: objectMeta(s, ConfigSecretName(s)), Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{"frps.toml": []byte(toml)}}
}

// BuildConnectionSecret is what private clusters copy: the token, and ca.crt when there is one.
func BuildConnectionSecret(s *v1.Server, token string, ca []byte) *corev1.Secret {
	data := map[string][]byte{"token": []byte(token)}
	if len(ca) > 0 {
		data["ca.crt"] = ca
	}
	return &corev1.Secret{ObjectMeta: objectMeta(s, ConnectionSecretName(s)), Type: corev1.SecretTypeOpaque, Data: data}
}

func BuildDashboardSecret(s *v1.Server, c Credentials) *corev1.Secret {
	return &corev1.Secret{ObjectMeta: objectMeta(s, DashboardSecretName(s)), Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{"username": []byte(c.Username), "password": []byte(c.Password)}}
}

// DashboardCredentialsSecret is the Secret holding the web server's username and password.
func DashboardCredentialsSecret(s *v1.Server) string {
	if d := s.Spec.Dashboard; d != nil && d.Credentials != nil {
		return d.Credentials.Secret.Name
	}
	return DashboardSecretName(s)
}

func secretFiles(name string, keys ...string) corev1.VolumeProjection {
	items := make([]corev1.KeyToPath, 0, len(keys))
	for _, k := range keys {
		items = append(items, corev1.KeyToPath{Key: k, Path: k})
	}
	return corev1.VolumeProjection{Secret: &corev1.SecretProjection{LocalObjectReference: corev1.LocalObjectReference{Name: name}, Items: items}}
}

func addProjected(pod *corev1.Pod, name, dir string, sources ...corev1.VolumeProjection) {
	pod.Spec.Volumes = append(pod.Spec.Volumes, corev1.Volume{Name: name,
		VolumeSource: corev1.VolumeSource{Projected: &corev1.ProjectedVolumeSource{Sources: sources}}})
	pod.Spec.Containers[0].VolumeMounts = append(pod.Spec.Containers[0].VolumeMounts,
		corev1.VolumeMount{Name: name, MountPath: dir, ReadOnly: true})
}

// BuildPod builds the frps pod. The caller adds ConfigHashAnnotation (see ConfigHash).
func BuildPod(s *v1.Server, ports *Ports) *corev1.Pod {
	var containerPorts []corev1.ContainerPort
	lowPort := false
	for _, sp := range ports.ServicePorts() {
		containerPorts = append(containerPorts, corev1.ContainerPort{Name: sp.Name, ContainerPort: sp.Port, Protocol: sp.Protocol})
		lowPort = lowPort || sp.Port < 1024
	}
	if ports.WebServer {
		containerPorts = append(containerPorts, corev1.ContainerPort{Name: "web", ContainerPort: WebServerPort, Protocol: corev1.ProtocolTCP})
	}

	yes, no, user := true, false, nonRootUser
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: PodName(s), Namespace: s.Namespace, Labels: Labels(s),
			Annotations: map[string]string{"sidecar.istio.io/inject": "false", "linkerd.io/inject": "disabled"},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{
				Name:         "frps",
				Image:        Image,
				Command:      []string{"frps", "-c", ConfigDir + "/frps.toml"},
				Ports:        containerPorts,
				VolumeMounts: []corev1.VolumeMount{{Name: "config", MountPath: ConfigDir, ReadOnly: true}},
				ReadinessProbe: &corev1.Probe{
					ProbeHandler:  corev1.ProbeHandler{TCPSocket: &corev1.TCPSocketAction{Port: intstr.FromInt32(ports.Bind)}},
					PeriodSeconds: 5,
				},
				SecurityContext: &corev1.SecurityContext{
					RunAsNonRoot:             &yes,
					RunAsUser:                &user,
					AllowPrivilegeEscalation: &no,
					ReadOnlyRootFilesystem:   &yes,
					Capabilities:             &corev1.Capabilities{Drop: []corev1.Capability{"ALL"}},
				},
			}},
			Volumes: []corev1.Volume{{Name: "config", VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{SecretName: ConfigSecretName(s)},
			}}},
		},
	}
	// A non-root process gets no added capabilities in Kubernetes (no ambient capabilities), so
	// ports below 1024 are opened with this namespaced, safe sysctl instead of NET_BIND_SERVICE.
	if lowPort {
		pod.Spec.SecurityContext = &corev1.PodSecurityContext{Sysctls: []corev1.Sysctl{{Name: "net.ipv4.ip_unprivileged_port_start", Value: "0"}}}
	}
	if t := s.Spec.Bind.TLS; t != nil {
		var sources []corev1.VolumeProjection
		if t.Certificate != nil {
			sources = append(sources, secretFiles(t.Certificate.Secret.Name, "tls.crt", "tls.key"))
		}
		if t.ClientCA != nil {
			sources = append(sources, secretFiles(t.ClientCA.Secret.Name, "ca.crt"))
		}
		if len(sources) > 0 {
			addProjected(pod, "tls", TLSDir, sources...)
		}
	}
	if d := s.Spec.Dashboard; ports.WebServer && d != nil && d.TLS != nil {
		addProjected(pod, "dashboard-tls", DashboardTLSDir, secretFiles(d.TLS.Certificate.Secret.Name, "tls.crt", "tls.key"))
	}

	podtemplate.Apply(pod, s.Spec.PodTemplate)
	pod.Labels[SelectorLabel] = s.Name // the Services select on it; a template must not change it
	return pod
}

// ConfigHash identifies everything a running frps pod was started with: frps.toml, the content of
// every mounted Secret file and the pod spec (image, resources, ...). frps cannot reload, so a
// different hash means a restart.
func ConfigHash(toml string, mounted map[string][]byte, spec corev1.PodSpec) string {
	h := sha256.New()
	fmt.Fprintf(h, "%s\x00", toml)
	for _, k := range slices.Sorted(maps.Keys(mounted)) {
		fmt.Fprintf(h, "%s\x00%s\x00", k, mounted[k])
	}
	specJSON, _ := json.Marshal(spec) // a PodSpec always marshals
	h.Write(specJSON)
	return hex.EncodeToString(h.Sum(nil))[:16]
}

func withRole(labels map[string]string, role string) map[string]string {
	labels[ServiceRoleLabel] = role
	return labels
}

// BuildService is the public Service: bind, vhost and allowPorts, each with its own protocol.
func BuildService(s *v1.Server, ports *Ports) *corev1.Service {
	cfg := s.Spec.Service
	if cfg == nil {
		cfg = &v1.ServerSpec_Service{}
	}
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: ServiceName(s), Namespace: s.Namespace, Labels: withRole(Labels(s), "proxy"), Annotations: maps.Clone(cfg.Annotations)},
		Spec: corev1.ServiceSpec{
			Type:     corev1.ServiceTypeLoadBalancer,
			Selector: map[string]string{SelectorLabel: s.Name},
			Ports:    ports.ServicePorts(),
		},
	}
	if cfg.Type != "" {
		svc.Spec.Type = corev1.ServiceType(cfg.Type)
	}
	if svc.Spec.Type == corev1.ServiceTypeLoadBalancer {
		if cfg.LoadBalancerClass != "" {
			class := cfg.LoadBalancerClass
			svc.Spec.LoadBalancerClass = &class
		}
		svc.Spec.ExternalTrafficPolicy = corev1.ServiceExternalTrafficPolicyCluster
		if cfg.ExternalTrafficPolicy != "" {
			svc.Spec.ExternalTrafficPolicy = corev1.ServiceExternalTrafficPolicy(cfg.ExternalTrafficPolicy)
		}
	}
	return svc
}

// BuildWebService exposes the dashboard, admin API and /metrics (port 7500).
func BuildWebService(s *v1.Server) *corev1.Service {
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: WebServiceName(s), Namespace: s.Namespace, Labels: withRole(Labels(s), "web")},
		Spec: corev1.ServiceSpec{
			Type:     corev1.ServiceTypeClusterIP,
			Selector: map[string]string{SelectorLabel: s.Name},
			Ports:    []corev1.ServicePort{{Name: "web", Port: WebServerPort, TargetPort: intstr.FromInt32(WebServerPort), Protocol: corev1.ProtocolTCP}},
		},
	}
	if d := s.Spec.Dashboard; d != nil && d.Service != nil {
		cfg := d.Service
		svc.Annotations = maps.Clone(cfg.Annotations)
		if cfg.Type != "" {
			svc.Spec.Type = corev1.ServiceType(cfg.Type)
		}
		if svc.Spec.Type == corev1.ServiceTypeLoadBalancer {
			if cfg.LoadBalancerClass != "" {
				class := cfg.LoadBalancerClass
				svc.Spec.LoadBalancerClass = &class
			}
			svc.Spec.LoadBalancerSourceRanges = cfg.LoadBalancerSourceRanges
		}
	}
	return svc
}

// BuildServiceMonitor scrapes /metrics on the web Service with the dashboard credentials.
func BuildServiceMonitor(s *v1.Server) *unstructured.Unstructured {
	secret := DashboardCredentialsSecret(s)
	endpoint := map[string]interface{}{
		"port":   "web",
		"path":   "/metrics",
		"scheme": "http",
		"basicAuth": map[string]interface{}{
			"username": map[string]interface{}{"name": secret, "key": "username"},
			"password": map[string]interface{}{"name": secret, "key": "password"},
		},
	}
	if d := s.Spec.Dashboard; d != nil && d.TLS != nil {
		endpoint["scheme"] = "https"
		// The dashboard certificate is rarely issued for the Service's in-cluster DNS name.
		endpoint["tlsConfig"] = map[string]interface{}{"insecureSkipVerify": true}
	}
	sm := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "monitoring.coreos.com/v1",
		"kind":       "ServiceMonitor",
		"spec": map[string]interface{}{
			"selector":  map[string]interface{}{"matchLabels": map[string]interface{}{SelectorLabel: s.Name, ServiceRoleLabel: "web"}},
			"endpoints": []interface{}{endpoint},
		},
	}}
	sm.SetName(ServiceName(s))
	sm.SetNamespace(s.Namespace)
	sm.SetLabels(Labels(s))
	return sm
}

// KeepNodePorts copies the node ports the API server allocated onto the desired ports with the
// same name and protocol, so an update does not reallocate them. ClusterIP has no node ports.
func KeepNodePorts(current, desired []corev1.ServicePort, t corev1.ServiceType) []corev1.ServicePort {
	if t == corev1.ServiceTypeClusterIP {
		return desired
	}
	for i := range desired {
		for _, c := range current {
			if c.Name == desired[i].Name && c.Protocol == desired[i].Protocol {
				desired[i].NodePort = c.NodePort
			}
		}
	}
	return desired
}

// Address is where frpc connects: the load balancer's IP or hostname ("" while pending), or the
// Service's DNS name for ClusterIP.
func Address(svc *corev1.Service) string {
	if svc.Spec.Type == corev1.ServiceTypeClusterIP {
		return svc.Name + "." + svc.Namespace + ".svc"
	}
	for _, in := range svc.Status.LoadBalancer.Ingress {
		if in.IP != "" {
			return in.IP
		}
		if in.Hostname != "" {
			return in.Hostname
		}
	}
	return ""
}

func RestartPolicy(s *v1.Server) string {
	if s.Spec.RestartPolicy == "" {
		return v1.RestartPolicyAutomatic
	}
	return s.Spec.RestartPolicy
}

// DashboardInsecure reports a web server reachable outside the cluster without TLS.
func DashboardInsecure(s *v1.Server) bool {
	d := s.Spec.Dashboard
	if !WebServerEnabled(&s.Spec) || (d != nil && d.TLS != nil) {
		return false
	}
	return d != nil && d.Service != nil && (d.Service.Type == string(corev1.ServiceTypeNodePort) || d.Service.Type == string(corev1.ServiceTypeLoadBalancer))
}

// SecretNames are the Secrets a Server owns or reads; a change to any of them reconciles it.
func SecretNames(s *v1.Server) []string {
	names := []string{ConfigSecretName(s), ConnectionSecretName(s), DashboardSecretName(s)}
	if a := s.Spec.Authentication; a != nil && a.Token != nil {
		names = append(names, a.Token.Secret.Name)
	}
	if t := s.Spec.Bind.TLS; t != nil {
		if t.Certificate != nil {
			names = append(names, t.Certificate.Secret.Name)
		}
		if t.ClientCA != nil {
			names = append(names, t.ClientCA.Secret.Name)
		}
	}
	if d := s.Spec.Dashboard; d != nil {
		if d.Credentials != nil {
			names = append(names, d.Credentials.Secret.Name)
		}
		if d.TLS != nil {
			names = append(names, d.TLS.Certificate.Secret.Name)
		}
	}
	return names
}
