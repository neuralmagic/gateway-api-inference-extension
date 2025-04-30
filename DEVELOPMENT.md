# Development

Developing and testing the Gateway API Inference Extension (GIE) is done by
building your Endpoint Picker (EPP) image and attaching that to a `Gateway` on a
development cluster, with some model serving backend to route traffic to.

We provide `Makefile` targets and development environment deployment manifests
under the `deploy/environments` directory, which include support for
multiple kinds of clusters:

* [Kubernetes In Docker (KIND)](#kubernetes-in-docker-(kind))
* [Kubernetes](#kubernetes)
* [OpenShift](#kubernetes)

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

```bash
make environment.dev.kind
```

This will create a `kind` cluster (or re-use an existing one) using the system's
local container runtime and deploy the development stack into the `default`
namespace.

There are several ways to access the gateway:

**Port forward**:

```sh
$ kubectl --context kind-gie-dev port-forward service/inference-gateway 8080:80
```

**NodePort `inference-gateway-istio`**
> **Warning**: This method doesn't work on `podman` correctly, as `podman` support
> with `kind` is not fully implemented yet.

```sh
# Determine the k8s node address
$ kubectl --context kind-gie-dev get node -o yaml | grep address
# The service is accessible over port 80 of the worker IP address.
```

**LoadBalancer**

```sh
# Install and run cloud-provider-kind:
$ go install sigs.k8s.io/cloud-provider-kind@latest && cloud-provider-kind &
$ kubectl --context kind-gie-dev get service inference-gateway
# Wait for the LoadBalancer External-IP to become available. The service is accessible over port 80.
```

You can now make requests macthing the IP:port of one of the access mode above:

```sh
$ curl -s -w '\n' http://<IP:port>/v1/completions -H 'Content-Type: application/json' -d '{"model":"food-review","prompt":"hi","max_tokens":10,"temperature":0}' | jq
```

By default the created inference gateway, can be accessed on port 30080. This can
be overriden to any free port in the range of 30000 to 32767, by running the above
command as follows:

```bash
GATEWAY_HOST_PORT=<selected-port> make environment.dev.kind
```

**Where:** &lt;selected-port&gt; is the port on your local machine you want to use to
access the inference gatyeway.

> **NOTE**: If you require significant customization of this environment beyond
> what the standard deployment provides, you can use the `deploy/components`
> with `kustomize` to build your own highly customized environment. You can use
> the `deploy/environments/kind` deployment as a reference for your own.

#### Development Cycle

To test your changes to the GIE in this environment, make your changes locally
and then run the following:

```bash
make environment.dev.kind.update
```

This will build images with your recent changes and load the new images to the
cluster. Then a rollout the `Deployments` will be performed so that your
recent changes are refleted.

## Kubernetes

A Kubernetes (or OpenShift) cluster can be used for development and testing.
There is a cluster-level infrastructure deployment that needs to be managed,
and then development environments can be created on a per-namespace basis to
enable sharing the cluster with multiple developers (or feel free to just use
the `default` namespace if the cluster is private/personal).

### Setup - Infrastructure

> **WARNING**: In shared cluster situations you should probably not be
> running this unless you're the cluster admin and you're _certain_ it's you
> that should be running this, as this can be disruptive to other developers
> in the cluster.

The following will deploy all the infrastructure-level requirements (e.g. CRDs,
Operators, etc) to support the namespace-level development environments:

```bash
make environment.dev.kubernetes.infrastructure
```

Whenever the `deploy/environments/dev/kubernetes-infra` deployment's components
are updated, this will need to be re-deployed.

### Setup - Developer Environment

> **WARNING**: This setup is currently very manual in regards to container
> images for the VLLM simulator and the EPP. It is expected that you build and
> push images for both to your own private registry. In future iterations, we
> will be providing automation around this to make it simpler.

To deploy a development environment to the cluster you'll need to explicitly
provide a namespace. This can be `default` if this is your personal cluster,
but on a shared cluster you should pick something unique. For example:

```bash
export NAMESPACE=annas-dev-environment
```

> **NOTE**: You could also use a tool like `uuidgen` to come up with a unique
> name (e.g. `anna-0d03d66c-8880-4000-88b7-22f1d430f7d0`).

Create the namespace:

```bash
kubectl create namespace ${NAMESPACE}
```

Set the default namespace for kubectl commands

```bash
kubectl config set-context --current --namespace="${NAMESPACE}"
```

> NOTE: If you are using OpenShift (oc CLI), use the following instead: `oc project "${NAMESPACE}"`

You'll need to provide a `Secret` with the login credentials for your private
repository (e.g. quay.io). It should look something like this:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: anna-pull-secret
data:
  .dockerconfigjson: <YOUR_ENCODED_CONFIG_HERE>
type: kubernetes.io/dockerconfigjson
```

Apply that to your namespace:

```bash
kubectl apply -f secret.yaml
```

Export the name of the `Secret` to the environment:

```bash
export REGISTRY_SECRET=anna-pull-secret
```

Set the `VLLM_MODE` environment variable based on which version of vLLM you want to deploy:

* `vllm-sim`: Lightweight simulator for simple environments (default).
* `vllm`: Full vLLM model server, using GPU/CPU for inferencing
* `vllm-p2p`: Full vLLM with LMCache P2P support for enable KV-Cache aware routing

```bash
export VLLM_MODE=vllm-sim  # or vllm / vllm-p2p
```

- Set Hugging Face token variable:

```bash
export HF_TOKEN="<HF_TOKEN>"
```

**Warning**: For vllm mode, the default image uses llama3-8b. Make sure you have permission to access these files in their respective repositories.

**Note:** The model can be replaced. See [Environment Configuration](#environment-configuration) for model settings.

> NOTE: you can set above images to the prebuilt images on quay.io.
> Just note that the EPP image is not using the `/epp` suffix.

Once all this is set up, you can deploy the environment:

```bash
make environment.dev.kubernetes
```

This will deploy the entire stack to whatever namespace you chose. You can test
by exposing the inference `Gateway` via port-forward:

```bash
kubectl port-forward service/inference-gateway 8080:80
```

And making requests with `curl`:

**vllm-sim:**

```bash
curl -s -w '\n' http://localhost:8080/v1/completions -H 'Content-Type: application/json' \
  -d '{"model":"food-review","prompt":"hi","max_tokens":10,"temperature":0}' | jq
```

**vllm or vllm-p2p:**

```bash
curl -s -w '\n' http://localhost:8080/v1/completions -H 'Content-Type: application/json' \
  -d '{"model":"meta-llama/Llama-3.1-8B-Instruct","prompt":"hi","max_tokens":10,"temperature":0}' | jq
```

#### Environment Configurateion

**1. Setting the EPP image and tag:**

You can optionally set a custom EPP image (otherwise, the default will be used):

```bash
export EPP_IMAGE="<YOUR_REGISTRY>/<YOUR_IMAGE>"
export EPP_TAG="<YOUR_TAG>"
```

**2. Setting the vLLM image and tag:**

Each vLLM mode has default image values, but you can override them:

For `vllm-sim` mode:**

```bash
export VLLM_SIM_IMAGE="<YOUR_REGISTRY>/<YOUR_IMAGE>"
export VLLM_SIM_TAG="<YOUR_TAG>"
```

For `vllm` and `vllm-p2p` modes:**

```bash
export VLLM_IMAGE="<YOUR_REGISTRY>/<YOUR_IMAGE>"
export VLLM_TAG="<YOUR_TAG>"
```

**3. Setting the model name and label:**

You can replace the model name that will be used in the system.

```bash
export MODEL_NAME="${MODEL_NAME:-mistralai/Mistral-7B-Instruct-v0.2}"
export MODEL_LABEL="${MODEL_LABEL:-mistral7b}"
```

It is also recommended to update the inference pool name accordingly so that it aligns with the models:

```bash
export POOL_NAME="${POOL_NAME:-vllm-Mistral-7B-Instruct}"
```

**4. Additional environment settings:**

More Setting of environment variables can be found in the `scripts/kubernetes-dev-env.sh`.



#### Development Cycle

> **WARNING**: This is a very manual process at the moment. We expect to make
> this more automated in future iterations.

Make your changes locally and commit them. Then select an image tag based on
the `git` SHA:

```bash
export EPP_TAG=$(git rev-parse HEAD)
```

Build the image:

```bash
DEV_VERSION=$EPP_TAG make image-build
```

Tag the image for your private registry and push it:

```bash
$CONTAINER_RUNTIME tag quay.io/vllm-d/gateway-api-inference-extension/epp:$TAG \
    <MY_REGISTRY>/<MY_IMAGE>:$EPP_TAG
$CONTAINER_RUNTIME push <MY_REGISTRY>/<MY_IMAGE>:$EPP_TAG
```

> **NOTE**: `$CONTAINER_RUNTIME` can be configured or replaced with whatever your
> environment's standard container runtime is (e.g. `podman`, `docker`).

Then you can re-deploy the environment with the new changes (don't forget all
the required env vars):

```bash
make environment.dev.kubernetes
```

And test the changes.

### Cleanup Environment

To clean up the development environment and remove all deployed resources in your namespace, run:

```sh
make clean.environment.dev.kubernetes
```

**Warning**: This cleanup doesn't remove any custom pull `Secrets` you may have added. If you are
done with this environment, you may want to remove those manually as well.

If you also want to remove the namespace entirely, run:

```sh
kubectl delete namespace ${NAMESPACE}
```
