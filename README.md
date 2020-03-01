# GitHubActionsRunnerController

GitHubActionsRunnerController is Kubernetes Custom Controller that runs self-hosted runner of GitHub Actions.

## Installation

```shell
$ kubectl apply -k manifests
```

## Usage

Applying an `examples` manifest runs self-hosted runner of GitHub Actions.

**`TOKEN` must have `administration` permission of a target repository to use `POST /repos/:owner/:repo/actions/runners/registration-token` endpoint.**

```shell
$ echo -n "<YOUR GITHUB TOKEN>" > examples/TOKEN
$ kubectl apply -k examples
```

![runners](https://github.com/kaidotdev/github-actions-runner-controller/wiki/images/runners.png)

The runner is based on an image that defined at `Runner` manifest.
Its image is rebuilt as an image for Runner using [GoogleContainerTools/kaniko](https://github.com/GoogleContainerTools/kaniko) by github-actions-runner-controller, and it is distributed via local docker registry.

```shell
$ cat examples/runner.yaml
apiVersion: github-actions-runner.kaidotdev.github.io/v1alpha1
kind: Runner
metadata:
  name: example
spec:
  image: ubuntu:18.04
  repository: kaidotdev/github-actions-runner-controller
  tokenSecretKeyRef:
    name: credentials
    key: TOKEN

# This shows the image is pulling from the local docker registry
$ kubectl get pod -l app=example -o jsonpath='{$.items[*].metadata.name}: {$.items[*].spec.containers[0].image}'
example-6dd7c8974c-4sgjv: 127.0.0.1:31994/f601e6d⏎

# This shows the image is based on ubuntu:18.04
$ kubectl exec -it example-6dd7c8974c-4sgjv cat /etc/os-release
NAME="Ubuntu"
VERSION="18.04.4 LTS (Bionic Beaver)"
ID=ubuntu
ID_LIKE=debian
PRETTY_NAME="Ubuntu 18.04.4 LTS"
VERSION_ID="18.04"
HOME_URL="https://www.ubuntu.com/"
SUPPORT_URL="https://help.ubuntu.com/"
BUG_REPORT_URL="https://bugs.launchpad.net/ubuntu/"
PRIVACY_POLICY_URL="https://www.ubuntu.com/legal/terms-and-policies/privacy-policy"
VERSION_CODENAME=bionic
UBUNTU_CODENAME=bionic
```

You can pass additional information to runner pod via `template`.

```yaml
apiVersion: github-actions-runner.kaidotdev.github.io/v1alpha1
kind: Runner
metadata:
  name: example
spec:
  image: ubuntu:18.04
  repository: kaidotdev/github-actions-runner-controller
  tokenSecretKeyRef:
    name: credentials
    key: TOKEN
  template:
    metadata:
      labels:
        version: v1
      annotations:
        # `--enable-runner-metrics` is required to scrape from prometheus
        prometheus.io/scrape: "true"
        prometheus.io/schema: "http"
        prometheus.io/port: "9090"
        prometheus.io/path: "/metrics"
        sidecar.istio.io/inject: "false"
    spec:
      env:
        - name: FOO
          valueFrom:
            fieldRef:
              fieldPath: metadata.name
        - name: BAR
          value: bar
```

Therefore, when combined with [DirectXMan12/k8s-prometheus-adapter](https://github.com/DirectXMan12/k8s-prometheus-adapter), it is possible to scale according to runner metrics using by HPA.

```yaml
    - seriesQuery: 'github_actions_runs{status="queued"}'
      resources:
        overrides:
          namespace:
            resource: namespace
          pod:
            resource: pod
      name:
        matches: "^(.*)$"
        as: "${1}_queued"
      metricsQuery: <<.Series>>{<<.LabelMatchers>>}
```

```yaml
apiVersion: autoscaling/v2beta2
kind: HorizontalPodAutoscaler
metadata:
  name: example-runner
spec:
  maxReplicas: 5
  minReplicas: 1
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: example
  metrics:
    - type: Pods
      pods:
        metric:
          name: github_actions_runs_queued
        target:
          type: AverageValue
          averageValue: 3
```

## How to develop

### `skaffold dev`

```sh
$ make dev
```

### Test

```sh
$ make test
```

### Lint

```sh
$ make lint
```

### Generate CRD from `*_types.go` by controller-gen

```sh
$ make gen
```
