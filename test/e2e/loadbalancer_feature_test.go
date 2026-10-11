//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	frpv1alpha1 "github.com/zufardhiyaulhaq/frp-operator/api/v1alpha1"
)

// TestLoadBalancerService binds a type=LoadBalancer Service (TCP and UDP on one port) to a
// ServerPool server and checks the ingress address and the traffic.
func TestLoadBalancerService(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	must(t, env.upsert(ctx, &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "e2e-frps-token", Namespace: operatorNamespace},
		Data:       map[string][]byte{"token": []byte(frpsToken)},
	}))
	pool := &frpv1alpha1.ServerPool{
		ObjectMeta: metav1.ObjectMeta{Name: "e2e", Namespace: operatorNamespace},
		Spec: frpv1alpha1.ServerPoolSpec{Servers: []frpv1alpha1.ServerPoolServer{{
			Name: "sg-01", Host: frpsMainHost, Port: 7000, PublicAddress: env.FrpsIP,
			Authentication: frpv1alpha1.ClientSpec_Server_Authentication{
				Token: &frpv1alpha1.ClientSpec_Server_Authentication_Token{Secret: frpv1alpha1.Secret{Name: "e2e-frps-token", Key: "token"}},
			},
		}}},
	}
	must(t, env.upsert(ctx, pool))

	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "lb-echo", Namespace: backendsNamespace},
		Spec: corev1.ServiceSpec{
			Type:              corev1.ServiceTypeLoadBalancer,
			LoadBalancerClass: ptr("frp.zufardhiyaulhaq.com/frp"),
			Selector:          map[string]string{"app": "echo"},
			Ports: []corev1.ServicePort{
				{Name: "echo-tcp", Port: portLoadBalancer, Protocol: corev1.ProtocolTCP, TargetPort: intstr.FromInt32(9000)},
				{Name: "echo-udp", Port: portLoadBalancer, Protocol: corev1.ProtocolUDP, TargetPort: intstr.FromInt32(9001)},
			},
		},
	}
	must(t, env.deleteAndWait(ctx, svc.DeepCopy())) // a leftover from an earlier failed run
	must(t, env.Kube.Create(ctx, svc))
	t.Cleanup(func() {
		if t.Failed() {
			env.dumpCase(context.Background(), operatorNamespace)
			return
		}
		_ = env.deleteAndWait(context.Background(), svc)
		_ = env.Kube.Delete(context.Background(), pool)
	})

	eventually(t, 3*time.Minute, "LoadBalancer ingress is the frps address", func(ctx context.Context) error {
		got := &corev1.Service{}
		if err := env.Kube.Get(ctx, client.ObjectKeyFromObject(svc), got); err != nil {
			return err
		}
		ingress := got.Status.LoadBalancer.Ingress
		if len(ingress) != 1 || ingress[0].IP != env.FrpsIP {
			return fmt.Errorf("ingress = %+v, want one entry with IP %s", ingress, env.FrpsIP)
		}
		if ingress[0].IPMode == nil || *ingress[0].IPMode != corev1.LoadBalancerIPModeProxy {
			return fmt.Errorf("ipMode = %v, want Proxy", ingress[0].IPMode)
		}
		return nil
	})
	expectTCPEcho(t, env.FrpsIP, portLoadBalancer)
	expectUDPEcho(t, env.FrpsIP, portLoadBalancer)
}
