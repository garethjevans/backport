# backport

a tool to aid with the backporting of PRs

## to build with TAP

### Workload for Configuration

```
apiVersion: carto.run/v1alpha1
kind: Workload
metadata:
  labels:
    app.kubernetes.io/part-of: backport
    apps.tanzu.vmware.com/has-tests: "true"
    apps.tanzu.vmware.com/workload-type: web
  name: backport
  namespace: dev
spec:
  env:
  - name: LOG_LEVEL
    value: info
  params:
  - name: annotations
    value:
      autoscaling.knative.dev/min-scale: "1"
  - name: testing_pipeline_matching_labels
    value:
      apps.tanzu.vmware.com/pipeline: golang-pipeline
  - name: dockerfile
    value: ./Dockerfile
  source:
    git:
      ref:
        branch: main
      url: https://github.com/garethjevans/backport
```

### Golang Pipeline

```
apiVersion: tekton.dev/v1beta1
kind: Pipeline
metadata:
  labels:
    apps.tanzu.vmware.com/pipeline: golang-pipeline
  name: developer-defined-golang-pipeline
  namespace: dev
spec:
  params:
  - name: source-url
    type: string
  - name: source-revision
    type: string
  tasks:
  - name: test
    params:
    - name: source-url
      value: $(params.source-url)
    - name: source-revision
      value: $(params.source-revision)
    taskSpec:
      params:
      - name: source-url
        type: string
      - name: source-revision
        type: string
      steps:
      - image: golang
        name: test
        script: |
          cd `mktemp -d`
          wget -qO- $(params.source-url) | tar xvz -m
          go test ./...
```
