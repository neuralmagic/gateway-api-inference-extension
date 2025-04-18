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

WIP

## OpenShift

WIP
