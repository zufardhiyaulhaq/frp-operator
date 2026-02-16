# Point to Point Intranet Service

Instead of exposing your service to outside world. We can utilize FRP to only allow specific client to access our service. In this use cases, we want to expose our nginx deployment in Kubernetes A to only services on Kubernetes B.

```
Clients (Kubernetes B) -> Visitor (Kubernetes B) <-- NAT Traversal/Proxy (FRP Server) --> Upstream (Kubernetes A) --> Nginx (Kubernetes A)
```

## FRP Server Configuration

For STCP/XTCP (P2P) to work, you need a basic FRP server. The server acts as a rendezvous point for NAT traversal.

```toml
# frps.toml
bindAddr = "0.0.0.0"
bindPort = 7000

# Authentication
auth.method = "token"
auth.token = "your-secret-token"

# STUN server for NAT traversal (optional, uses Google's STUN by default)
# natHoleStunServer = "stun.example.com:3478"

# Optional: Dashboard
webServer.addr = "0.0.0.0"
webServer.port = 7500
webServer.user = "admin"
webServer.password = "admin"

# Logging
log.to = "./frps.log"
log.level = "info"
```

Run the FRP server:
```bash
./frps -c frps.toml
```

**Note**: For XTCP (NAT traversal), the FRP server helps clients discover each other's public IP and port. Once connected, traffic flows directly between clients (P2P) without going through the server. If NAT traversal fails, STCP (relay through server) is used as fallback.

## Prerequisites

You will need 2 secrets
1. Secret to authenticate to FRP server
2. Secret to authenticate between Visitor & Upstream

### Kubernetes A
In Kubernetes A, we will create nginx deployment, and will create 2 Upstream (xftp & sftp). XFTP is using NAT traversal. traffic flow from Kubernetes B to Kubernetes A directly, without going to FRP server. the SFTP is created as fallback if NAT traversal is not working.

create the deployment
```
kubectl apply -f examples/p2p/kubernetes-a/deployment
```

and create the Upstream
```
kubectl apply -f examples/p2p/kubernetes-a/client
```

the FTP client & nginx deployment will be created
```
(⎈|orbstack:default) [31/12/25 | 11:55:50]
➜  frp-operator git:(main) k get pod
NAME                                READY   STATUS    RESTARTS   AGE
client-01-frpc                      1/1     Running   0          20m
nginx-deployment-688845894b-kn27w   1/1     Running   0          103m
(⎈|orbstack:default) [31/12/25 | 11:56:25]
➜  frp-operator git:(main) k get svc
NAME                                              TYPE        CLUSTER-IP        EXTERNAL-IP   PORT(S)    AGE
client-01-frpc                                    ClusterIP   192.168.194.158   <none>        7400/TCP   95m
nginx-service                                     ClusterIP   192.168.194.246   <none>        80/TCP     103m
```

checking the logs
```
025-12-31 16:35:34.085 [I] [client/control.go:172] [a0ec1886673a5764] [nginx-stcp] start proxy success
2025-12-31 16:35:34.109 [I] [client/control.go:172] [a0ec1886673a5764] [nginx-xtcp] start proxy success
2025-12-31 16:36:38.746 [I] [proxy/xtcp.go:80] [a0ec1886673a5764] [nginx-xtcp] nathole prepare success, nat type: HardNAT, behavior: BehaviorPortChanged, addresses: [104.28.159.130:36604 104.28.159.130:35515], assistedAddresses: [192.168.194.13:48886]
2025-12-31 16:36:39.890 [I] [proxy/xtcp.go:101] [a0ec1886673a5764] [nginx-xtcp] get natHoleRespMsg, sid [17671989981c6569161d9c40cb], protocol [quic], candidate address [118.99.104.60:62279 118.99.104.60:32166], assisted address [192.168.194.12:58791], detectBehavior: {Role:sender Mode:0 TTL:0 SendDelayMs:0 ReadTimeoutMs:5000 CandidatePorts:[] SendRandomPorts:0 ListenRandomPorts:0}
```

### Kubernetes B
In Kubernetes B, we will create Visitor object. It's act as the gateway to call the nginx service on Kubernetes A.
```
kubectl apply -f examples/p2p/kubernetes-b/client
```

After you create the client & visitor, FRP client pod & service will spawn. If you notice, apart from 7400 for admin port, it will also show port 5000 which is port you defined on visitor-xtcp.yaml
```
(⎈ |orbstack:default) [31/12/25 | 11:41:54] - [main]
zufar.dhiyaullhaq@Zufar-Dhiyaulhaq frp-operator % k get svc
NAME             TYPE        CLUSTER-IP        EXTERNAL-IP   PORT(S)             AGE
client-01-frpc   ClusterIP   192.168.194.252   <none>        7400/TCP,5000/TCP   38m
(⎈ |orbstack:default) [31/12/25 | 11:51:13] - [main]
zufar.dhiyaullhaq@Zufar-Dhiyaulhaq frp-operator % k get pod
NAME             READY   STATUS    RESTARTS   AGE
client-01-frpc   1/1     Running   0          9m27s
```

you can spawn client and curl the service with port that you defined on visitor-xtcp.yaml
```
kubectl run nginx --image=nginx

root@nginx:/# curl http://client-01-frpc.default.svc:5000 -v
* Host client-01-frpc.default.svc:5000 was resolved.
* IPv6: (none)
* IPv4: 192.168.194.252
*   Trying 192.168.194.252:5000...
* Connected to client-01-frpc.default.svc (192.168.194.252) port 5000
* using HTTP/1.x
> GET / HTTP/1.1
> Host: client-01-frpc.default.svc:5000
> User-Agent: curl/8.14.1
> Accept: */*
> 
* Request completely sent off
< HTTP/1.1 200 OK
< Server: nginx/1.29.4
< Date: Wed, 31 Dec 2025 16:45:35 GMT
< Content-Type: text/html
< Content-Length: 615
< Last-Modified: Tue, 09 Dec 2025 18:28:10 GMT
< Connection: keep-alive
< ETag: "69386a3a-267"
< Accept-Ranges: bytes
< 
<!DOCTYPE html>
<html>
<head>
<title>Welcome to nginx!</title>
<style>
html { color-scheme: light dark; }
body { width: 35em; margin: 0 auto;
font-family: Tahoma, Verdana, Arial, sans-serif; }
</style>
</head>
<body>
<h1>Welcome to nginx!</h1>
<p>If you see this page, the nginx web server is successfully installed and
working. Further configuration is required.</p>

<p>For online documentation and support please refer to
<a href="http://nginx.org/">nginx.org</a>.<br/>
Commercial support is available at
<a href="http://nginx.com/">nginx.com</a>.</p>

<p><em>Thank you for using nginx.</em></p>
</body>
</html>
```

If you see the logs of the pods
```
k logs client-01-frpc -f
2025-12-31 16:46:20.103 [I] [visitor/xtcp.go:292] [e277620720a87637] [nginx-xtcp] nathole prepare success, nat type: HardNAT, behavior: BehaviorPortChanged, addresses: [118.99.104.60:4620 118.99.104.60:50394], assistedAddresses: [192.168.194.13:43252]
2025-12-31 16:46:21.565 [I] [visitor/xtcp.go:318] [e277620720a87637] [nginx-xtcp] get natHoleRespMsg, sid [1767199582afcaddf68899163b], protocol [quic], candidate address [104.28.163.38:39153 104.28.163.38:38467], assisted address [192.168.194.13:46704], detectBehavior: {Role:sender Mode:0 TTL:0 SendDelayMs:0 ReadTimeoutMs:5000 CandidatePorts:[] SendRandomPorts:0 ListenRandomPorts:0}
```
