# Kube Commander

[![Build Status](https://travis-ci.org/AnatolyRugalev/kube-commander.svg?branch=master)](https://travis-ci.org/AnatolyRugalev/kube-commander)
[![kube-commander on snap](https://snapcraft.io/kube-commander/badge.svg)](https://snapcraft.io/kube-commander)

![Kube Commander](docs/demo.gif)

## Requirements

1. [kubectl](https://kubernetes.io/docs/tasks/tools/install-kubectl/) with access to Kubernetes cluster
2. And... that's it!

## kube-commander vs. kubernetes-dashboard comparison

|                                           | kube-commander           | kubernetes-dashboard     |
|-------------------------------------------|--------------------------|--------------------------|
| Easy to use                               | :heavy_check_mark:       | :heavy_check_mark:       |
| Doesn't require installation into cluster | :heavy_check_mark:       | :heavy_multiplication_x: |
| Doesn't require web access to cluster     | :heavy_check_mark:       | :heavy_multiplication_x: |
| Can be used over SSH                      | :heavy_check_mark:       | :heavy_multiplication_x: |
| Responsiveness                            | :zap:                    | :turtle:                 |
| Mouse support                             | :heavy_check_mark:       | :heavy_check_mark:       |
| Keyboard-only interactions                | :heavy_check_mark:       | :heavy_multiplication_x: |
| Auto-refresh working properly             | :heavy_check_mark:       | :heavy_multiplication_x: |
| Written on Go                             | :heavy_check_mark:       | :heavy_multiplication_x: |
| Suitable for hackers                      | :heavy_check_mark:       | :heavy_multiplication_x: |
| Charts and metrics support                | :hammer: [#6](https://github.com/AnatolyRugalev/kube-commander/issues/6)|:heavy_check_mark:|

## Installation

1. [Install from snap](#snap)
2. [Archlinux User Repository](#aur)
3. [Install binary](#binary)
4. [Install from sources](#sources)

### Snap

```bash
sudo snap install kube-commander
```

*NOTE: please be aware of [this bug](https://github.com/AnatolyRugalev/kube-commander/issues/35) in snap implementation*

### AUR

If you use Archlinux you can install kube-commander from AUR with your favorite AUR helper:

```bash
yay -S kube-commander
```

### Binary

You can install kube-commander from binary release for your OS. Linux, macOS and Windows are supported. You can find 
a package for your OS on [Releases page](https://github.com/AnatolyRugalev/kube-commander/releases).

1. Untar archive on your machine
2. Put kube-commander executable in your `$PATH`. E.g. `/usr/local/bin`

*NOTE: if you use Windows it is highly recommended to use [Git Bash](https://gitforwindows.org/) terminal to launch
kube-commander or to use Linux subsystem*

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

Feel free to file an issue if you have a feature request in mind or experience a bug.

## Special thanks

* [termui](https://github.com/gizak/termui)
* [Goreleaser](https://goreleaser.com)
