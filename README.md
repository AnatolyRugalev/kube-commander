# Kube Commander

[![Build Status](https://travis-ci.org/AnatolyRugalev/kube-commander.svg?branch=master)](https://travis-ci.org/AnatolyRugalev/kube-commander)

![Kube Commander](docs/demo.gif)

## TUI

Kube Commander UI is based on [termui](https://github.com/gizak/termui).

## Installation

1. [Install from snap](#snap)
2. [Install from sources](#sources)

### Snap

```bash
sudo snap install kube-commander
```

### Sources

If you have go environment configured you can install kube-commander easily with this command:

```bash
go get -u github.com/AnatolyRugalev/kube-commander/cmd/kube-commander
```

*NOTE: Make sure your `$PATH` has `$GOPATH/bin` in it.*

## Usage

### Launching
 
Before starting kube-commander make sure you have proper kubectl configuration:

```bash
kubectl cluster-info
```

Then you can start kube-commander:

```bash'
kube-commander
```

To start kube-commander with non-default kubectl context, namespace or config itself you can use this flags
and env vars:

| Flag      | Env var     | Description             |
|-----------|-------------|-------------------------|
|kubeconfig |KUBECONFIG   |Path to kubeconfig       |
|context    |KUBECONTEXT  |Context name             |
|namespace  |KUBENAMESPACE|Initial namespace to show|

Example:

```bash
kube-commander --context=my-cluster-2 --namespace=my-namespace --kubeconfig=~/.kube/my-config
```
Or:
```bash
export KUBECONFIG=$HOME/.kube/my-config
export KUBECONTEXT=my-cluster-2
export KUBENAMESPACE=my-namespace
kube-commander
```

### Hotkeys

TBD

## Contribution

Feel free to file an issue if you have feature request in mind or experience a bug.

