# Change Log
## [1.1.16]

* Bump golang.org/x/net to v0.36.0
* Bump k8s.io/kubernetes to v1.31.6
 
## [1.1.15]

* Bump k8s.io/apimachinery to v0.31.3
* Bump k8s.io/component-base to v0.31.3
* Bump k8s.io/klog/v2 to v2.130.1
* Bump k8s.io/kubernetes to v1.31.3
* Bump k8s.io/utils to v0.0.0-20240921022957-49e7df575cb6
* Bump golang.org/x/net to v0.33.0

## [1.1.14]

* Bump distroless-iptables to v0.6.3
* Bump Go build image to 1.23.2-bookworm

## [1.1.13]

* Use Microsoft image for Go toolchain
* Bump k8s.io/kubernetes to v1.27.16

## [0.1.12]

* Bump Go version to 1.22

## [0.1.11]

* Use base image registry.k8s.io/build-image/distroless-iptables:v0.4.8
* Bump golang.org/x/net from 0.17.0 to 0.23.0
* Bump k8s.io/kubernetes to v1.27.13

## [0.1.10]

* Use base image registry.k8s.io/build-image/distroless-iptables:v0.4.5

## [0.1.9]

* Use base image registry.k8s.io/build-image/distroless-iptables:v0.4.2
* Update k8s.io/kubernetes to v1.27.8
* Specify go 1.20 in go.mod

## [0.1.8]

* Use base image registry.k8s.io/build-image/distroless-iptables:v0.3.2

## [0.1.7]

* Use go 1.20 builder image
* Use base image registry.k8s.io/build-image/distroless-iptables:v0.2.4

## [0.1.6]

* Bump golang.org/x/text from 0.3.7 to 0.3.8
* Bump golang.org/x/net from 0.0.0-20220225172249-27dd8689420f to 0.7.0
* Update k8s.io/kubernetes to v1.23.17

## [0.1.5]

* Dynamically update base image to specific architecture

## [0.1.4]

* Switch base image to distroless-iptables

## [0.1.3]

* Remove /vendor from VCS
* Initialize logging flags

## [0.1.2]

* Update usage examples, README, and CHANGELOG formatting
* Add CI for multi-arch builds

## [0.1.1]

* Update Kubernetes to v1.23.0 from v1.13.0-alpha
* Migrate to semantic versioning
* Update publish action to support semantic versioning
* Remove version prefix in CHANGELOG

## [0.1.0]

* Update README for v2 goals
* Setup go modules
* Add editor files and future test output to gitignore
* Azure repo setup, remove obsolete files
* Update to the current thockin/go-build-template
* VERSION -> Version (changed in thockin/go-build-template)
* Config resync interval as a flag
* Merge data from multiple config files
* v2 for image name
* Update to the latest OSS guidelines
* Setup github actions
* Add ci-pipeline for this repo
* Merge CI actions, use make to build and test
* Enable Code QL analysis
* Add quality visibility to the repo
* Add go reference to the repo
* masq daemon crashes on bad config
* Update README with new usage instructions
