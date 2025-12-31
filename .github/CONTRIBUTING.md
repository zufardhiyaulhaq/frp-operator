# Contributing
By participating to this project, you agree to abide our [code of conduct](https://github.com/zufardhiyaulhaq/frp-operator/blob/main/.github/CODE_OF_CONDUCT.md).

## Development
For small things like fixing typos in documentation, you can [make edits through GitHub](https://help.github.com/articles/editing-files-in-another-user-s-repository/), which will handle forking and making a pull request (PR) for you. For anything bigger or more complex, you'll probably want to set up a development environment on your machine, a quick procedure for which is as folows:

### Setup your machine
Prerequisites:
- make
- [Go 1.23](https://golang.org/doc/install)
- [operator-sdk v1.36.0](https://sdk.operatorframework.io/)

Fork and clone **[frp-operator](https://github.com/zufardhiyaulhaq/frp-operator)** repository.

- deploy CRDs
```
kubectl apply -f config/crd/bases/
```

- Run frp-operator locally
```
make install run
```

- deploy some examples
```
kubectl apply -f examples/simple/deployment/
kubectl apply -f examples/simple/client/
```

### Submit a pull request
As you are ready with your code contribution, push your branch to your `frp-operator` fork and open a pull request against the **main** branch.

Please also update the [CHANGELOG.md](https://github.com/zufardhiyaulhaq/frp-operator/blob/main/CHANGELOG.md) to note what you've added or fixed.
