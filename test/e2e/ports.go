package e2e

// Remote ports on frps. Cases share one frps and run in parallel, so every case owns its ports.
// HTTP, HTTPS and TCPMUX cases use frps's vhost ports and are routed by domain instead.
const (
	portTCP          = 20001
	portUDP          = 20002
	portEnabled      = 20003
	portDisabled     = 20004
	portKCP          = 20011
	portQUIC         = 20012
	portWebsocket    = 20013
	portWSS          = 20014
	portTLS          = 20015
	portMTLS         = 20016 // on frps-mtls
	portProxyHTTP    = 20021
	portProxySOCKS   = 20022
	portReloadFirst  = 20031
	portReloadSecond = 20032
	portInvalid      = 20041
	portLoadBalancer = 5353 // TCP and UDP; for LoadBalancer Services the Service port is the remote port
)

func allPorts() []int {
	return []int{
		portTCP, portUDP, portEnabled, portDisabled,
		portKCP, portQUIC, portWebsocket, portWSS, portTLS, portMTLS,
		portProxyHTTP, portProxySOCKS, portReloadFirst, portReloadSecond, portInvalid,
		portLoadBalancer,
	}
}
