# monocle-operator

A k8s/OpenShift operator for [Monocle](https://github.com/change-metrics/monocle).

## Status

The operator is currently in alpha version and should not be used in production.

The status is: Work In Progress.

We use [Microshift](https://github.com/openshift/microshift) as development sandbox and for CI.

Thus the operator might have some adherences to OpenShift regarding:

- The Security Context Contraints

## Installation

### Vanilla installation

By default the Monocle operator watches all namespaces, but can be scoped to a specified namespace via the
`WATCH_NAMESPACE` environment variable.

To install the CRDs and the operator:

```
kubectl create -f https://raw.githubusercontent.com/change-metrics/monocle-operator/master/install/crds.yml
kubectl create -f https://raw.githubusercontent.com/change-metrics/monocle-operator/master/install/operator.yml
```

finally, reclaim a Monocle instance by running the following command:

```
kubectl create -f https://raw.githubusercontent.com/change-metrics/monocle-operator/master/config/samples/monocle_v1alpha1_monocle-alt.yaml
```

The role `monocle-operator-monocle-editor-role` is installed by the `operator.yml`. This role can be assigned via
a `RoleBinding` to users.

### Via OLM

`monocle-operator` is packaged into an OLM package available on [operatorshub.io](https://operatorhub.io/operator/monocle-operator).

## Status

### Phase 1 - Basic Install

Automatic application provisioning and config management

tasks:

- [X] Elastic / API / Crawler deployment support
- [X] Validate config ConfigMap update
- [X] Handle secret data update
- [X] Set Monocle Resource Status
- [X] Refresh elastic database in Monocle API Resource when ConfigMap is modified
- [X] Produce and publish the operator container image
- [X] Document operator deployment


### Phase 2 - Seamless Upgrades

Support minor version upgrade

- [] Enable upgrade test in CI

### Phase 3 - Full lifecycle

Application and storage lifecycle (backup and failure recovery)

tasks: TBD

### Phase 4 - Deep insights

Metrics, alerts, log processing

tasks: TBD

### Phase 5 - Auto-pilot

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
$ cp config/samples/monocle_v1alpha1_monocle.yaml monocle-sample.yaml
# Update the monoclePublicFQDN to a FQDN to contact the deployment
# You might need to update the /etc/hosts
$ kubectl apply -f config/samples/monocle_v1alpha1_monocle.yaml
# The reconcile loop should stop shortly and you should see in
# the other terminal:
# 1.676629527659292e+09   INFO    monocle operand reconcile terminated
```

Access the Monocle Web UI:

```bash
$ firefox https://<FQDN>
```

Edit the Monocle config:

```bash
$ kubectl edit cm monocle-sample-api
```

The Monocle API and Crawler process detect that the config data (exposed via a ConfigMap mounted
as a volume) changed and automatically handle the change.

The "janitor update-idents" command is run automatically after the Moncole config update.
The Monocle operator starts a Job "update-idents-job" to handle that task.

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

### Run the operator in standalone dev mode

This mode does not require the CRD to be installed on the cluster thus you don't
need the cluster-admin right to run the `monocle-operator`. This mode is for development
purpose only.

The following command takes as paramaters a `Namespace` and a `YAML file that describe the Monocle Custom Resource`.

```Shell
go run ./main.go --cr config/sample/monocle_v1alpha1_monocle.yaml --ns monocle
```

Running with those paramaters triggers the `standalone` mode which does not start the controller-runtime's Manager
but instead uses the controller-runtime's Client. This mode prevents the `Reconcile` function to look at any
`Monocle` Custom Resource set in any `Namespace` but only relies on the Custom Resource passed as the YAML file.

This command performs one or more `Reconcile` calls until the Monocle resources (deployment, statefulset, ...) are
detected as ready, then exits.

Most of the code involved to reconcile the state is covered by this mode and it make it useful to facilitate
the operator development.

## Generate assets

A CI post job build and publish the operator and OLM bundle container images
in the [quay.io organization](https://quay.io/organization/change-metrics).

These commands can be use build assets bases on an alternative `IMAGE_TAG_BASE` environment variable.

### Build and publish the operator image

```
make container-build
make container-push
```

### Build resources for the vanilla installation

```
make gen-operator-install
```

### Build the OLM bundle image

```
make bundle
make bundle-container-build
make bundle-container-push
```

### Deploy the OLM bundle via operator-sdk run bundle

```
# Create a new namespace
kubectl create ns bundle-catalog-ns
# It seems that a batch job fails if not added to privileged
oc adm policy add-scc-to-user privileged system:serviceaccount:bundle-catalog-ns:default
operator-sdk --verbose run bundle quay.io/change-metrics/monocle-operator-bundle:v0.0.2 \
    --namespace bundle-catalog-ns --security-context-config restricted
```

The command creates an index based on the opm image, defines a catalogsource resources,
defines a subscription to the monocle-operator, then accepts the installplan.

```
kubectl -n bundle-catalog-ns get calalogsource
kubectl -n bundle-catalog-ns get sub
kubectl -n bundle-catalog-ns get csv
kubectl -n bundle-catalog-ns get all
# The operator should be deployed in that namespace
# Another namespace can be created to request a Monocle instance to the operator
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

