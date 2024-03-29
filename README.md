# ip-masq-agent-v2

[![CI](https://github.com/Azure/ip-masq-agent-v2/actions/workflows/main.yaml/badge.svg)](https://github.com/Azure/ip-masq-agent-v2/actions/workflows/main.yaml)
[![CodeQL](https://github.com/Azure/ip-masq-agent-v2/actions/workflows/codeql-analysis.yaml/badge.svg)](https://github.com/Azure/ip-masq-agent-v2/actions/workflows/codeql-analysis.yaml)
[![Go Report Card](https://goreportcard.com/badge/github.com/azure/ip-masq-agent-v2)](https://goreportcard.com/report/github.com/azure/ip-masq-agent-v2)
[![GoDoc](https://godoc.org/github.com/azure/ip-masq-agent-v2?status.svg)](https://pkg.go.dev/github.com/azure/ip-masq-agent-v2)

Based on the original [ip-masq-agent](https://github.com/kubernetes-sigs/ip-masq-agent), v2 aims to solve more specific networking cases, allow for more configuration options, and improve observability. This includes:
* Merging configuration from multiple sources
* Detecting conflicting configurations
* Fixing security vulnerabilities
* Quickly crashing on errors

## Overview

The ip-masq-agent configures `iptables` rules to `MASQUERADE` traffic outside link-local (optional, enabled by default) and additional arbitrary IP ranges.

It creates an `iptables` chain called `IP-MASQ-AGENT`, which contains match rules for link local (`169.254.0.0/16`) and each of the user-specified IP ranges. It also creates a rule in `POSTROUTING` that jumps to this chain for any traffic not bound for a `LOCAL` destination.

IPs that match the rules (except for the final rule) in `IP-MASQ-AGENT` are *not* subject to `MASQUERADE` via the `IP-MASQ-AGENT` chain (they `RETURN` early from the chain). The final rule in the `IP-MASQ-AGENT` chain will `MASQUERADE` any non-`LOCAL` traffic.

`RETURN` in `IP-MASQ-AGENT` resumes rule processing at the next rule the calling chain, `POSTROUTING`. Take care to avoid creating additional rules in `POSTROUTING` that cause packets bound for your configured ranges to undergo `MASQUERADE`.

## Launching the agent as a DaemonSet
This repo includes an example yaml file that can be used to launch the ip-masq-agent as a DaemonSet in a Kubernetes cluster.

```
kubectl create -f examples/ip-masq-agent.yaml
```

## Configuring the agent

Important: You should not attempt to run this agent in a cluster where the Kubelet is also configuring a non-masquerade CIDR. You can pass `--non-masquerade-cidr=0.0.0.0/0` to the Kubelet to nullify its rule, which will prevent the Kubelet from interfering with this agent.

By default, the agent is configured to treat the three private IP ranges specified by [RFC 1918](https://tools.ietf.org/html/rfc1918) as non-masquerade CIDRs. These ranges are `10.0.0.0/8`, `172.16.0.0/12`, and `192.168.0.0/16`. To change this behavior, see the flags section below. The agent will also treat link-local (`169.254.0.0/16`) as a non-masquerade CIDR by default.

By default, the agent is configured to reload its configuration from the `/etc/config/` directory in its container every 60 seconds.

The agent configuration file should be written in yaml or json syntax, and may contain three optional keys:
- `nonMasqueradeCIDRs []string`: A list strings in CIDR notation that specify the non-masquerade ranges.
- `masqLinkLocal bool`: Whether to masquerade traffic to `169.254.0.0/16`. False by default.
- `masqLinkLocalIPv6 bool`: Whether to masquerade traffic to `fe80::/10`. False by default.

The agent will look for config files in its container at `/etc/config/`. This file can be provided via a `ConfigMap`, plumbed into the container by specifying it as a [projected volume](https://kubernetes.io/docs/concepts/storage/projected-volumes/), and mounting it. 

As a result, the agent can be reconfigured in a live cluster by creating or editing these `ConfigMaps`. Please ensure that your file name in the `ConfigMaps` matches the config file prefix: `ip-masq-*`. 

This repo includes an example use-case. You can modify it to your needs and apply it to your cluster:

```
kubectl create configmap examples/config-custom.yaml
```

Note that we created the `ConfigMap` in the same namespace as the DaemonSet Pods, and named the `ConfigMap` to match the spec in `examples/ip-masq-agent.yaml`. This is necessary for the `ConfigMap` to appear in the Pods' filesystems.

Tolerance of multiple `ConfigMaps` allows custom keys to be defined while avoiding conflicts of any `ConfigMaps` that will need to be reconciled dynamically. This repo also provides an example of what might be configured by a cloud provider:

```
kubectl create configmap examples/config-reconciled.yaml
```

The cloud provider may wish to reconcile a `ConfigMap` so that it can be in-sync with the subnet of the cluster.


### Agent Flags

The agent accepts multiple flags, which may be specified in the yaml file.

`masq-chain`
: The name of the `iptables` chain to use. Default set to `IP-MASQ-AGENT`.

`nomasq-all-reserved-ranges`
: Whether or not to masquerade all RFC reserved ranges when the configmap is empty. The default is `false`. When `false`, the agent will masquerade to every destination except the ranges reserved by RFC 1918 (namely `10.0.0.0/8`, `172.16.0.0/12`, and `192.168.0.0/16`). When `true`, the agent will masquerade to every destination that is not marked reserved by an RFC. The full list of ranges is (`10.0.0.0/8`, `172.16.0.0/12`, `192.168.0.0/16`, `100.64.0.0/10`, `192.0.0.0/24`, `192.0.2.0/24`, `192.88.99.0/24`, `198.18.0.0/15`, `198.51.100.0/24`, `203.0.113.0/24`, and `240.0.0.0/4`). Note however, that this list of ranges is overridden by specifying the nonMasqueradeCIDRs key in the agent configmap.

`enable-ipv6`
: Whether to configure ip6tables rules. Default is `false`. 

`resync-interval`
: How often to refresh the config (in seconds). Default set to `60`.

[klog](https://github.com/kubernetes/klog) also offers a range of flags that the agent inherits from (e.g. `--v` for log verbosity level).

## Rationale
(from the [incubator proposal](https://gist.github.com/mtaufen/253309166e7d5aa9e9b560600a438447))

This agent solves the problem of configuring the CIDR ranges for non-masquerade in a cluster (via iptables rules). Today, this is accomplished by passing a `--non-masquerade-cidr` flag to the Kubelet, which only allows one CIDR to be configured as non-masquerade. [RFC 1918](https://tools.ietf.org/html/rfc1918), however, defines three ranges (`10/8`, `172.16/12`, `192.168/16`) for the private IP address space.

Some users will want to communicate between these ranges without masquerade - for instance, if an organization's existing network uses the `10/8` range, they may wish to run their cluster and `Pod`s in `192.168/16` to avoid IP conflicts. They will also want these `Pod`s to be able to communicate efficiently (no masquerade) with each-other *and* with their existing network resources in `10/8`. This requires that every node in their cluster skips masquerade for both ranges.

We are trying to eliminate networking code from the Kubelet, so rather than extend the Kubelet to accept multiple CIDRs, ip-masq-agent allows you to run a DaemonSet that configures a list of CIDRs as non-masquerade.

## Contributing

This project welcomes contributions and suggestions.

### Developing

Clone the repo to `$GOPATH/src/github.com/Azure/ip-masq-agent-v2`.

The build tooling is based on [thockin/go-build-template](https://github.com/thockin/go-build-template).

Run `make` or `make build` to compile the ip-masq-agent.  This will use a Docker image
to build the agent, with the current directory volume-mounted into place.  This
will store incremental state for the fastest possible build.  Run `make
all-build` to build for all architectures.

Run `make test` to run the unit tests.

Run `make container` to build the container image.  It will calculate the image
tag based on the most recent git tag, and whether the repo is "dirty" since
that tag (see `make version`).  Run `make all-container` to build containers
for all architectures.

Run `make push` to push the container image to `REGISTRY`.  Run `make all-push`
to push the container images for all architectures.

Run `make clean` to clean up.

### Contribution requirements

Most contributions require you to agree to a
Contributor License Agreement (CLA) declaring that you have the right to, and actually do, grant us
the rights to use your contribution. For details, visit https://cla.opensource.microsoft.com.

When you submit a pull request, a CLA bot will automatically determine whether you need to provide
a CLA and decorate the PR appropriately (e.g., status check, comment). Simply follow the instructions
provided by the bot. You will only need to do this once across all repos using our CLA.

### Code of Conduct

This project has adopted the [Microsoft Open Source Code of Conduct](https://opensource.microsoft.com/codeofconduct/).
For more information see the [Code of Conduct FAQ](https://opensource.microsoft.com/codeofconduct/faq/) or
contact [opencode@microsoft.com](mailto:opencode@microsoft.com) with any additional questions or comments.

### Trademarks

This project may contain trademarks or logos for projects, products, or services. Authorized use of Microsoft trademarks or logos is subject to and must follow Microsoft’s Trademark & Brand Guidelines. Use of Microsoft trademarks or logos in modified versions of this project must not cause confusion or imply Microsoft sponsorship. Any use of third-party trademarks or logos are subject to those third-party’s policies.