# frp-operator

Expose your service in Kubernetes to the Internet with open source FRP!

![Version: 1.0.0](https://img.shields.io/badge/Version-1.0.0-informational?style=flat-square) ![Type: application](https://img.shields.io/badge/Type-application-informational?style=flat-square) ![AppVersion: 0.1.0](https://img.shields.io/badge/AppVersion-0.1.0-informational?style=flat-square) [![made with Go](https://img.shields.io/badge/made%20with-Go-brightgreen)](http://golang.org) [![Github master branch build](https://img.shields.io/github/workflow/status/zufardhiyaulhaq/frp-operator/Master)](https://github.com/zufardhiyaulhaq/frp-operator/actions/workflows/master.yml) [![GitHub issues](https://img.shields.io/github/issues/zufardhiyaulhaq/frp-operator)](https://github.com/zufardhiyaulhaq/frp-operator/issues) [![GitHub pull requests](https://img.shields.io/github/issues-pr/zufardhiyaulhaq/frp-operator)](https://github.com/zufardhiyaulhaq/frp-operator/pulls)

## Installing

To install the chart with the release name `my-release`:

```console
helm repo add zufardhiyaulhaq https://charts.zufardhiyaulhaq.com/
helm install my-release zufardhiyaulhaq/frp-operator --values values.yaml
```

## Usage
1. Apply some example
```console
kubectl apply -f examples/deployment/
kubectl apply -f examples/client/
```
2. Check frpc object
```console
kubectl get client
NAME        AGE
client-01   17m

kubectl get upstream
NAME    AGE
nginx   17m
```

3. access the URL
```console
http://178.128.100.87:8080/
```

## Values

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| operator.image | string | `"zufardhiyaulhaq/frp-operator"` |  |
| operator.replica | int | `1` |  |
| operator.tag | string | `"v0.1.0"` |  |
| resources.limits.cpu | string | `"200m"` |  |
| resources.limits.memory | string | `"100Mi"` |  |
| resources.requests.cpu | string | `"100m"` |  |
| resources.requests.memory | string | `"20Mi"` |  |

see example files [here](https://github.com/zufardhiyaulhaq/frp-operator/blob/master/charts/frp-operator/values.yaml)

