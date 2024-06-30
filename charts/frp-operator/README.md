# frp-operator

Expose your service in Kubernetes to the Internet with open source FRP!

![Version: 1.2.0](https://img.shields.io/badge/Version-1.2.0-informational?style=flat-square) ![Type: application](https://img.shields.io/badge/Type-application-informational?style=flat-square) ![AppVersion: 0.3.1](https://img.shields.io/badge/AppVersion-0.3.1-informational?style=flat-square) [![made with Go](https://img.shields.io/badge/made%20with-Go-brightgreen)](http://golang.org) [![Github main branch build](https://img.shields.io/github/workflow/status/zufardhiyaulhaq/frp-operator/Main)](https://github.com/zufardhiyaulhaq/frp-operator/actions/workflows/main.yml) [![GitHub issues](https://img.shields.io/github/issues/zufardhiyaulhaq/frp-operator)](https://github.com/zufardhiyaulhaq/frp-operator/issues) [![GitHub pull requests](https://img.shields.io/github/issues-pr/zufardhiyaulhaq/frp-operator)](https://github.com/zufardhiyaulhaq/frp-operator/pulls)[![Artifact Hub](https://img.shields.io/endpoint?url=https://artifacthub.io/badge/repository/frp-operator)](https://artifacthub.io/packages/search?repo=frp-operator)

## Document
1. [RFC: Fast Reverse Proxy Operator](https://docs.google.com/document/d/18_X4KKLNMAFcfYP-Nh0wwU31RP903IrLuc1Uemxcpoo)

## Installing

To install the chart with the release name `my-release`:

```console
helm repo add frp-operator https://zufardhiyaulhaq.com/frp-operator/charts/releases/
helm install my-frp-operator frp-operator/frp-operator --values values.yaml
```

## Prerequisite
To expose your private Kubernetes service into public network. You need public machine running FRP Server that act as a proxy. Currently the operator doesn't have capability to spine a new machine on cloud providers, but this can be setup in a minute.

1. Create machine on cloud provider
2. Download `frps` [binary](https://github.com/fatedier/frp)
3. Create server configuration
```
vi frps.ini

[common]
bind_address = 0.0.0.0
bind_port = 7000
token = yourtoken
```
4. Run FRP server
```
frps -c ./frps.ini
```

You can reuse our build-in ansible playbook to setup the FRP server on your machine, please check https://github.com/zufardhiyaulhaq/frp-operator/tree/main/ansible/server

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
| operator.tag | string | `"v0.3.1"` |  |
| resources.limits.cpu | string | `"200m"` |  |
| resources.limits.memory | string | `"100Mi"` |  |
| resources.requests.cpu | string | `"100m"` |  |
| resources.requests.memory | string | `"20Mi"` |  |

see example files [here](https://github.com/zufardhiyaulhaq/frp-operator/blob/main/charts/frp-operator/values.yaml)

