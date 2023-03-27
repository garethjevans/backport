SOURCE_IMAGE = os.getenv("SOURCE_IMAGE", default='your-registry.io/project/backport-source')
LOCAL_PATH = os.getenv("LOCAL_PATH", default='.')
NAMESPACE = os.getenv("NAMESPACE", default='default')

allow_k8s_contexts('gke_ship-interfaces-dev_us-central1-a_tap-multi-1-1-0')

k8s_custom_deploy(
    'backport',
    apply_cmd="tanzu apps workload apply -f config/workload.yaml --update-strategy replace --debug --live-update" +
               " --local-path " + LOCAL_PATH +
               " --source-image " + SOURCE_IMAGE +
               " --namespace " + NAMESPACE +
               " --yes",
    delete_cmd="tanzu apps workload delete -f config/workload.yaml --namespace " + NAMESPACE + " --yes",
    container_selector='workload',
    deps=['go.mod', 'go.sum', '*.go', 'pkg', 'Dockerfile'],
)

k8s_resource('backport', port_forwards=["8080:8080"],
            extra_pod_selectors=[{'carto.run/workload-name': 'backport', 'app.kubernetes.io/component': 'run'}])
