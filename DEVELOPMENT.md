# Development

Developing and testing the Gateway API Inference Extension (GIE) is done by
building your Endpoint Picker (EPP) image and attaching that to a `Gateway` on a
development cluster, with some model serving backend to route traffic to.

We provide `Makefile` targets and development environment deployment manifests
under the `deploy/environments` directory, which include support for
multiple kinds of clusters:

* Kubernetes In Docker (KIND)
* Kubernetes (WIP: https://github.com/neuralmagic/gateway-api-inference-extension/issues/14)
* OpenShift (WIP: https://github.com/neuralmagic/gateway-api-inference-extension/issues/22)

We support multiple different model serving platforms for testing:

* VLLM
* VLLM-Simulator

In the following sections we will cover how to use the different development
environment options.

## Kubernetes In Docker (KIND)

A [KIND] cluster can be used for basic development and testing on a local
system. This environment will generally be limited to using a model serving
simulator and as such is very limited compared to clusters with full model
serving resources.

[KIND]:https://github.com/kubernetes-sigs/kind

### Setup

> **WARNING**: This current requires you to have manually built the vllm
> simulator separately on your local system. In a future iteration this will
> be handled automatically and will not be required.

Run the following:

```console
make environment.dev.kind
```

This will create a `kind` cluster (or re-use an existing one) using the system's
local container runtime and deploy the development stack into the `default`
namespace. Instrutions will be provided on how to access the `Gateway` and send
requests for testing.

> **NOTE**: If you require significant customization of this environment beyond
> what the standard deployment provides, you can use the `deploy/components`
> with `kustomize` to build your own highly customized environment. You can use
> the `deploy/environments/kind` deployment as a reference for your own.

#### Development Cycle

To test your changes to the GIE in this environment, make your changes locally
and then run the following:

```console
make environment.dev.kind.update
```

This will build images with your recent changes and load the new images to the
cluster. Then a rollout the `Deployments` will be performed so that your
recent changes are refleted.

## Kubernetes

A Kubernetes (or OpenShift) cluster can be used for development and testing.
There is a cluster-level infrastructure deployment that needs to be managed,
and then development environments can be created on a per-namespace basis to
enable sharing the cluster with multiple developers if desired.

### Setup - Infrastructure

> **WARNING**: In shared cluster situations you should probably not be
> running this unless you're the cluster admin and you're _certain_ it's you
> that should be running this.

The following will deploy all the infrastructure-level requirements (e.g. CRDs,
Operators, etc) to support the namespace-level development environments:

```console
make environment.dev.kubernetes.infrastructure
```

When the `deploy/environments/dev/kubernetes-infra` deployment's components are
updated, this will need to be re-deployed.

### Setup - Developer Environment

> **WARNING**: This setup is currently very manual in regards to container
> images for the VLLM simulator and the EPP. It is expected that you build and
> push images for both to your own private registry. In future iterations, we
> will be automating this further to make it simpler.

To deploy a development environment to the cluster you'll need to explicitly
provide a namespace. This can be `default` if this is your personal cluster,
but on a shared cluster you should pick something unique. For example:

```console
export NAMESPACE=annas-dev-environment
```

Create the namespace:

```console
kubectl create namespace ${NAMESPACE}
```

You'll need to provide a `Secret` with the login credentials for your private
repository (e.g. quay.io). It should look something like this:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: anna-pull-secret
data:
  .dockerconfigjson: <YOUR_TOKEN_HERE>
type: kubernetes.io/dockerconfigjson
```

Apply that to your namespace:

```console
kubectl -n ${NAMESPACE} apply -f secret.yaml
```

Export the name of the secret to the environment:

```console
export REGISTRY_SECRET=anna-pull-secret
```

Now you need to provide several other environment variables. You'll need to
indicate the location and tag of the `vllm-sim` image:

```console
export VLLM_SIM_IMAGE="<YOUR_REGISTRY>/<YOUR_IMAGE>"
export VLLM_SIM_TAG="<YOUR_TAG>"
```

The same thing will need to be done for the EPP:

```console
export EPP_IMAGE="<YOUR_REGISTRY>/<YOUR_IMAGE>"
export EPP_TAG="<YOUR_TAG>"
```

Once all this is set up, you can deploy the environment:

```console
make environment.dev.kubernetes
```

This will deploy the entire stack to whatever namespace you chose. You can test
by exposing the inference `Gateway` via port-forward:

```console
kubectl -n ${NAMESPACE} port-forward service/inference-gateway-istio 8080:80
```

And making requests with `curl`:

```console
curl -s -w '\n' http://localhost:8080/v1/completions -H 'Content-Type: application/json' -d '{"model":"food-review","prompt":"hi","max_tokens":10,"temperature":0}' | jq
```

#### Development Cycle

> **WARNING**: This is a very manual process at the moment. We expect to make
> this more automated in future iterations.

Make your changes locally and commit them. Then select an image tag based on
the `git` SHA:

```console
export EPP_TAG=$(git rev-parse HEAD)
```

Build the image:

```console
DEV_VERSION=$EPP_TAG make image-build
```

Tag the image for your private registry and push it:

```console
$CONTAINER_RUNTIME tag quay.io/vllm-d/gateway-api-inference-extension/epp:$TAG \
    <MY_REGISTRY>/<MY_IMAGE>:$EPP_TAG
$CONTAINER_RUNTIME push <MY_REGISTRY>/<MY_IMAGE>:$EPP_TAG
```

Then you can re-deploy the environment with the new changes (don't forget all
the required env vars):

```console
make environment.dev.kubernetes
```

And test the changes.
