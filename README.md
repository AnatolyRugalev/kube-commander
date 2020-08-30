# Kube Commander  (kubecom)

[![Build Status](https://img.shields.io/travis/anatolyrugalev/kube-commander?style=for-the-badge)](https://travis-ci.org/AnatolyRugalev/kube-commander)
[![Docker Image](https://img.shields.io/docker/v/anatolyrugalev/kubecom?sort=semver&style=for-the-badge)](https://hub.docker.com/r/anatolyrugalev/kubecom)
[![Aur](https://img.shields.io/aur/version/kube-commander?style=for-the-badge)](https://aur.archlinux.org/packages/kube-commander/)

kube-commander is an easy to use tool for observing Kubernetes cluster from your terminal.

> Soon `kube-commander` will change its name to `kubecom`. Please don't mind some naming inconsistency - I'm
> trying to make the migration as seamless as possible.

![Kube Commander](https://user-images.githubusercontent.com/1397674/83310793-ecdada00-a215-11ea-9f26-37f5fb673147.gif)

## kube-commander vs. kubernetes-dashboard comparison

|                                           | kube-commander           | kubernetes-dashboard     | 
|-------------------------------------------|--------------------------|--------------------------|
| Easy to use                               | :heavy_check_mark:       | :heavy_check_mark:       |
| Realtime data update                      | :heavy_check_mark:       | :heavy_multiplication_x: |
| Doesn't require deployment                | :heavy_check_mark:       | :heavy_multiplication_x: |
| Doesn't require http access to cluster    | :heavy_check_mark:       | :heavy_multiplication_x: |
| Can be used over SSH                      | :heavy_check_mark:       | :heavy_multiplication_x: |
| Responsiveness                            | :zap:                    | :turtle:                 |
| Suitable for hackers                      | :heavy_check_mark:       | :heavy_multiplication_x: |
| Requires cluster-specific configuration   | :heavy_check_mark:       | :heavy_check_mark:       |

## Requirements

1. GNU/Linux, MacOS or Windows system
2. [kubectl](https://kubernetes.io/docs/tasks/tools/install-kubectl/) installed and configured to access your cluster
3. And... that's it!

## Installation

1. [Archlinux User Repository](#aur)
2. [Install binary](#binary)
3. [Install from sources](#sources)
4. [Run with Docker](#run-with-docker)

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

*NOTE: if you use Windows make sure you have installed [Git Bash](https://gitforwindows.org/) to support editing,
viewing logs and etc.*

### Sources

If you have Go environment configured you can install kube-commander easily with this command:

```bash
go get -u github.com/AnatolyRugalev/kube-commander/cmd/kubecom
```

*NOTE: Make sure your `$PATH` has `$GOPATH/bin` in it.*

### Run with Docker

```bash
alias kubecom="docker run --rm -v ~/.kube:/root/.kube -ti anatolyrugalev/kubecom:latest"
kubecom
```

## Usage

### Run
 
Before starting kube-commander make sure you have proper kubectl configuration:

```bash
kubectl cluster-info
```

Then you can start kube-commander:

```bash
kubecom
```

To start kube-commander with non-default kubectl context, namespace or config itself you can use this flags
and env vars:

| Flag      | Env var     | Description                                                                                   |
|-----------|-------------|-----------------------------------------------------------------------------------------------|
|kubeconfig |KUBECONFIG   |Path to kubeconfig                                                                             |
|context    |KUBECONTEXT  |Context name                                                                                   |
|namespace  |KUBENAMESPACE|Initial namespace to show                                                                      |
|editor     |EDITOR       |Name of the editor binary. Default: "vi". But you probably already have one defined by your OS |
|pager      |PAGER        |Pager command for 'describe' command. Default: "less"                                          |
|log-pager  |LOGPAGER     |Pager command for log output. Default: none                                                    |
|kubectl    |KUBECTL      |Name of kubectl binary. Default: "kubectl"                                                     |
|tail       |KUBETAIL     |Number of log lines to show with kubectl logs. Default: 1000                                   |
|klog       |KUBELOG      |Kubernetes log file for debugging. Default: none                                               |

Example:
```bash
kubecom --context=my-cluster-2 --namespace=my-namespace --kubeconfig=~/.kube/my-config
```

This is pretty useful example:
```bash
kubecom --log-pager="jq -c"
```

In this case kubecom will use jq as log pager so json logs will be colorized. You can pipe commands here as well:

```bash
kubecom --log-pager="jq -c | some_other_command"
```

### Supported resource types

For now kube-commander shows limited number of resources, but technically, it can show anything kubectl can. On 
kube-commander start you can see that some items in the menu are gray. This happens because kube-commander needs some
time to discover your cluster capabilities. This behavior could be configurable in future releases. Also we could
allow to add your custom resource types into the menu via this configuration.

### Hotkeys

The first thing you need to press is "?". This will show help dialog in case you missed it on start screen.

The initial version of kube-commander had a refresh key which updated list of resources. Now you don't have to do that:
kube-commander watches changes dynamically, so you can relax and take a sip of your coffee while waiting for a deployment.

The most of hotkeys you can find on help dialog. Here they are:

| Key | Action  |
|:---:|:--------|
|?| Show help dialog |
| ↑↓→← | Navigation. When table doesn't fit to the screen, use ← and → to scroll horizontally |
| Enter | Select menu item |
| Esc, Backspace | Go back |
| Q, Ctrl+C | Quit |
| Ctrl+N, F2 | Switch namespace |
| F3 | (experimental) Show all known resources in resource menu |
| D | Describe selected resource with `kubectl describe` |
| E | Edit selected resource with `kubectl edit` |
| Delete | Delete selected resource (then press "y" to confirm) |
| C | Copy resource name to the clipboard |
| / | Enter filtering mode. Type string and then press Enter to confirm |
| Ctrl+P | Switch to pods |
| Ctrl+D | Switch to deployments |
| Ctrl+I | Switch to ingresses |
| L | Show pod logs |
| Shift+L | Show previous pod logs |
| F | Forward pod port |
| S | Enter to container `/bin/sh` shell | 

## Contribution

We play by gentleman rules. If you want to contribute a code - please file an issue describing your intentions first.
This way we can avoid wasting time doing easy work the hard way. I'm always open to give my point of view on your ideas.

## Special thanks

* [tcell](https://github.com/gdamore/tcell) - TUI library
* [Goreleaser](https://goreleaser.com) - helps to ship go software
* [k9s](https://github.com/derailed/k9s) - another kubernetes TUI utility
