# monocle-operator

A k8s operator for [Monocle](https://github.com/change-metrics/monocle).

## Description

The operator is currently in alpha version and should not be used in production.

The status is: Work In Progress.

### Day 1 - Basic Install

Automatic application provisioning and config management

tasks:

- [X] Elastic / API / Crawler deployment support
- [X] Validate config ConfigMap update
- [X] Handle secret data update
- [] Set Monocle Resource Status
- [] Produce and publish the operator container image
- [] Document operator deployment
- [] Git config support
- Far more task TBD

### Day 2 - Seamless Upgrades

Support minor version upgrade

tasks: TBD

### Day 3 - Full lifecycle

Application and storage lifecycle (backup and failure recovery)

tasks: TBD

### Day 4 - Deep insights

Metrics, alerts, log processing

tasks: TBD

### Day 5 - Auto-pilot

Metrics, alerts, log processing

tasks: TBD

## Hack on the operator

### Dependencies

The operator code is managed via the [Operator SDK](https://sdk.operatorframework.io/). Please follow [installation instructions](https://sdk.operatorframework.io/docs/building-operators/golang/installation/) to install the `operator-sdk` CLI tool.

`kubectl` and `go` are required tools.

Optionaly the project provides a `flake` file for nix users. See [flake.nix](./flake.nix)
and run `nix develop`.

### Start the operator in dev mode

Assuming is properly configured `~/.kube/config` file simply run:

```bash
# Install the CRD
$ make install
# Set the WATCH_NAMESPACE env var
$ export WATCH_NAMESPACE=$(kubectl config view -o jsonpath="{.contexts[?(@.name == '$(kubectl config current-context)')].context.namespace}")
# Start the manager in dev mode
$ go run ./main.go
```

In another terminal apply the Custom Resource:

```bash
$ kubectl apply -f config/samples/monocle_v1alpha1_monocle-alt.yaml
# The reconcile loop should stop shortly and you should see in
# the other terminal:
# 1.676629527659292e+09   INFO    monocle operand reconcile terminated
```

Setup a port forward to access the Monocle Web UI:

```bash
$ kubectl port-forward service/monocle-sample-api 8090:8080
$ firefox http://localhost:8090
```

Edit the Monocle config:

```bash
$ kubectl edit cm monocle-sample-api
```

The Monocle API and Crawler process detect that the config data (exposed via a ConfigMap mounted
as a volume) changed and automatically handle the change.

Edit the Monocle Secrets (mainly used by the crawler process):

```bash
$ kubectl edit secret monocle-sample-api
```

The Monocle controller detects the secret change and force a rollout for the api and crawler
deployments.

Elastic volume is persistant so you might want to wipe the Elastic data to
start a fresh deployment.

```bash
$ kubectl delete pvc monocle-sample-elastic-data-volume-monocle-sample-elastic-0
```


## License

Copyright 2023 Monocle developers.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

