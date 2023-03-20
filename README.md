# backport

a tool to aid with the backporting of PRs

## to build with TAP

```
❯ tanzu apps workload create backport \
  --namespace dev \
  --git-branch main \
  --git-repo https://github.com/garethjevans/backport \
  --label apps.tanzu.vmware.com/has-tests=true \
  --label app.kubernetes.io/part-of=backport \
  --type web \
  --param-yaml testing_pipeline_matching_labels='{"apps.tanzu.vmware.com/pipeline":"golang-pipeline"}' \
  --param dockerfile=./Dockerfile \
   --yes
```

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
