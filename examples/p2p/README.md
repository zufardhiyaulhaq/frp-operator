# Point to Point Intranet Service

Instead of exposing your service to outside world. We can utilize FRP to only allow specific client to access our service. In this use cases, we want to expose our nginx deployment in Kubernetes A to only services on Kubernetes B.

```
Clients (Kubernetes B) -> Visitor (Kubernetes B) <-- NAT Traversal --> Upstream (Kubernetes A) --> Nginx (Kubernetes A)
```

