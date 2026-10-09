# Egress Proxy

Use this when the cluster can only reach the Internet (and therefore frps) through an outbound
HTTP, HTTPS, SOCKS5 or NTLM proxy. frpc tunnels its connection to frps through the proxy; the
proxy credentials come from a Kubernetes `Secret`.

## Architecture

```
Internet -> FRP Server (frps) <- egress proxy <- FRP Client (K8s) -> Nginx Service (K8s)
```

Only frpc's connection to frps goes through the proxy. Traffic from frpc to your in-cluster
services (`Upstream`s) is unchanged, so Upstreams need nothing proxy-specific.

## Files

| File | Contents |
|---|---|
| `secret.yaml` | frps token, proxy username/password, OIDC client credentials |
| `client.yaml` | `Client` with token auth through the proxy |
| `client-oidc.yaml` | `Client` with OIDC auth through the proxy (the token request is proxied too) |
| `upstream.yaml` | One TCP `Upstream` per client |

## Configuration

```yaml
spec:
  server:
    transport:
      proxyURL: "http://egress-proxy.corp.internal:3128"   # scheme://host:port, no credentials
      proxyCredentials:                                    # optional
        username:
          secret:
            name: egress-proxy-credentials
            key: username
        password:
          secret:
            name: egress-proxy-credentials
            key: password
```

- **Schemes:** `http://`, `https://`, `socks5://` or `ntlm://`.
- **Credentials:** the operator reads them from the Secrets and URL-escapes them into the URL
  (`p@ss:w/rd` becomes `p%40ss%3Aw%2Frd`). Use either `proxyCredentials` or credentials in
  `proxyURL`, not both.
- **Protocol:** `spec.server.protocol` must be `tcp` (default), `websocket` or `wss`. `kcp` and
  `quic` are UDP and cannot go through a proxy; the operator rejects that combination instead of
  letting frpc silently bypass the proxy.
- **OIDC:** frpc's OIDC client ignores `transport.proxyURL` and the `http_proxy` env var, so the
  operator also renders the proxy as `auth.oidc.proxyURL`. NTLM is not supported by the OIDC
  client; with an `ntlm://` proxy the token endpoint must be reachable directly.
- **ServerPool:** each `servers[].transport` accepts the same `proxyURL` and `proxyCredentials`;
  the Secrets must live in the operator's namespace, next to the `ServerPool`.

## Apply

```bash
kubectl apply -f examples/egress-proxy/
```

## Verify

The rendered frpc config is in the `<client>-frpc-config` ConfigMap:

```bash
kubectl get configmap egress-oidc-client-frpc-config -o jsonpath='{.data.config\.toml}'
```

For `client-oidc.yaml` it looks like this (secret values shown are the example placeholders):

```toml
serverAddr = "frps.example.com"
serverPort = 7000
clientID = "default/egress-oidc-client"
auth.method = "oidc"
auth.oidc.clientID = "frpc"
auth.oidc.clientSecret = "your-oidc-client-secret"
auth.oidc.tokenEndpointURL = "https://idp.example.com/oauth2/token"
auth.oidc.audience = "frps"
auth.oidc.proxyURL = "http://svc-frp:p%40ss%3Aw%2Frd@egress-proxy.corp.internal:3128"
webServer.addr = "0.0.0.0"
webServer.port = 7400
webServer.user = "frpc-user"
webServer.password = "frpc-password"
transport.tcpMux = true
transport.proxyURL = "http://svc-frp:p%40ss%3Aw%2Frd@egress-proxy.corp.internal:3128"
[[proxies]]
name = "web-oidc"
type = "tcp"
localIP = "nginx.default.svc.cluster.local"
localPort = 80
remotePort = 8081
```

Check that frpc logged in through the proxy:

```bash
kubectl logs egress-oidc-client-frpc
```

Look for `login to server success`. With an HTTP proxy, rejected credentials show up as
`DialTcpByHttpProxy error, StatusCode [407]`; an unreachable proxy as a dial error naming the
proxy address.

> The rendered config, including the proxy password, is stored in a ConfigMap, the same as the
> frps token. Restrict read access to ConfigMaps in this namespace accordingly.
