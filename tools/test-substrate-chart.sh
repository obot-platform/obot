#!/usr/bin/env bash
set -euo pipefail

# Requires Helm, Python 3, and PyYAML.
cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.."
work=$(mktemp -d)
trap 'rm -rf -- "$work"' EXIT

render() {
    local chart=$1
    shift
    helm template obot "$chart" --namespace obot --set dev.useEmbeddedDb=true --include-crds "$@"
}

render chart > "$work/disabled.yaml"
if render chart --set substrate.enabled=true --set config.OBOT_SERVER_SUBSTRATE_SNAPSHOT_LOCATION=s3://snapshots --set substrate.image.repository= > "$work/missing.yaml" 2> "$work/missing.err"; then
    echo 'Expected rendering without an image repository to fail' >&2
    exit 1
fi
grep -q 'substrate.image.repository is required' "$work/missing.err"

flags=(--set substrate.enabled=true --set config.OBOT_SERVER_SUBSTRATE_SNAPSHOT_LOCATION=s3://snapshots --set substrate.image.repository=registry.example.com/substrate
    --set substrate.storage.existingSecret=snapshots --set substrate.postgres.storageSize=20Gi)
if render chart --set substrate.enabled=true > "$work/missing-bucket.yaml" 2> "$work/missing-bucket.err"; then
    echo 'Expected Claude integration without a snapshot bucket to fail' >&2
    exit 1
fi
grep -q 'config.OBOT_SERVER_SUBSTRATE_SNAPSHOT_LOCATION' "$work/missing-bucket.err"
render chart "${flags[@]}" > "$work/enabled.yaml"
render chart "${flags[@]}" --set-json 'substrate.worker.nodeSelector={"kubernetes.io/os":"linux","workload":"agents"}' --set-json 'substrate.worker.tolerations=[{"key":"workload","operator":"Equal","value":"agents","effect":"NoSchedule"}]' > "$work/scheduling.yaml"
if render chart "${flags[@]}" --set substrate.atelet.nodeSelector.workload=agents > "$work/old-selector.yaml" 2> "$work/old-selector.err"; then
    echo 'Expected the retired atelet node selector to fail' >&2
    exit 1
fi
grep -q 'move substrate.atelet.nodeSelector to substrate.worker.nodeSelector' "$work/old-selector.err"
render chart --set substrate.enabled=true --set config.OBOT_SERVER_SUBSTRATE_SNAPSHOT_LOCATION=s3://snapshots > "$work/environments.yaml"
render chart "${flags[@]}" --set config.OBOT_SERVER_SUBSTRATE_ADDRESS=ignored:443 --set service.port=8081 --set config.OBOT_SERVER_SUBSTRATE_SNAPSHOT_LOCATION=s3://custom --set config.OBOT_SERVER_SUBSTRATE_NAMESPACE=custom-workers --set config.OBOT_SERVER_SUBSTRATE_WORKER_IMAGE=custom/worker:test --set config.OBOT_SERVER_SUBSTRATE_RUNNER_IMAGE=custom/runner:test --set config.OBOT_SERVER_SUBSTRATE_OBOT_URL=http://obot.internal > "$work/overrides.yaml"
dsn='postgresql://substrate:test-password@database.example.com/substrate?sslmode=require&connect_timeout=10'
render chart "${flags[@]}" --set-string "substrate.postgres.dsn=$dsn" --set substrate.postgres.role=substrate > "$work/external.yaml"
if render chart "${flags[@]}" --set-string "substrate.postgres.dsn=$dsn" > "$work/missing-role.yaml" 2> "$work/missing-role.err"; then
    echo 'Expected external PostgreSQL without a role to fail' >&2
    exit 1
fi
grep -q 'substrate.postgres.role is required' "$work/missing-role.err"
if render chart "${flags[@]}" --set substrate.atelet.imageCredentialProviderConfig=/node/config.yaml > "$work/missing-provider-bin.yaml" 2> "$work/missing-provider-bin.err"; then
    echo 'Expected an incomplete image credential provider configuration to fail' >&2
    exit 1
fi
grep -q 'set both substrate.atelet' "$work/missing-provider-bin.err"
render chart "${flags[@]}" --set substrate.atelet.imageCredentialProviderConfig=/node/config.yaml --set substrate.atelet.imageCredentialProviderBinDir=/node/bin > "$work/image-provider.yaml"


helm package chart --destination "$work"
archives=("$work"/*.tgz)
render "${archives[0]}" "${flags[@]}" > "$work/packaged.yaml"

python3 - "$work" "$dsn" <<'PY'
import base64
import hashlib
import json
from pathlib import Path
import sys
import yaml

work = Path(sys.argv[1])
dsn = sys.argv[2]
namespaces = {'ate-system', 'podcertificate-controller-system'}


class UniqueLoader(yaml.SafeLoader):
    pass


def unique_mapping(loader, node):
    result = {}
    for key_node, value_node in node.value:
        key = loader.construct_object(key_node)
        assert key not in result, f'Duplicate YAML key: {key}'
        result[key] = loader.construct_object(value_node)
    return result


UniqueLoader.add_constructor(yaml.resolver.BaseResolver.DEFAULT_MAPPING_TAG, unique_mapping)


def read(name):
    text = (work / f'{name}.yaml').read_text()
    for placeholder in ('ko://', '${SUBSTRATE_VERSION', '${ENVOY_DATAPLANE_IMAGE}'):
        assert placeholder not in text, f'Unresolved placeholder: {placeholder}'
    return [doc for doc in yaml.load_all(text, Loader=UniqueLoader) if doc]


def resource(docs, kind, name):
    matches = [doc for doc in docs if doc['kind'] == kind and doc['metadata']['name'] == name]
    assert len(matches) == 1, f'Missing or duplicate {kind}/{name}'
    return matches[0]


def has_secret(workload, name):
    sources = workload['spec']['template']['spec']['containers'][0]['envFrom']
    return any(entry.get('secretRef', {}).get('name') == name for entry in sources)


disabled = read('disabled')
assert not any(doc['metadata'].get('namespace') in namespaces for doc in disabled)
assert not any(doc['kind'] == 'CustomResourceDefinition' for doc in disabled)
assert not any(doc.get('data', {}).get('OBOT_SERVER_SUBSTRATE_ADDRESS') for doc in disabled)
assert not any(doc['metadata'].get('name', '').endswith('-egress-credentials') for doc in disabled)
disabled_pod = resource(disabled, 'Deployment', 'obot-obot')['spec']['template']['spec']
assert not any(volume['name'] == 'substrate-api' for volume in disabled_pod.get('volumes', []))

# Legacy cleanup must work even with Substrate disabled.
cleanup_role = resource(disabled, 'ClusterRole', 'obot-obot-agent-cleanup')
assert cleanup_role['rules'] == [{'apiGroups': ['scheduling.k8s.io'], 'resources': ['priorityclasses'], 'verbs': ['list', 'delete']}]
mcp_role = resource(disabled, 'Role', 'obot-mcp-role')
for group, kind in [('', 'pods'), ('', 'secrets'), ('', 'services'), ('', 'resourcequotas'), ('', 'persistentvolumeclaims'), ('apps', 'deployments'), ('apps', 'replicasets'), ('batch', 'jobs')]:
    verbs = {verb for rule in mcp_role['rules'] if group in rule['apiGroups'] and kind in rule['resources'] for verb in rule['verbs']}
    assert {'list', 'delete'} <= verbs, f'Missing legacy cleanup permissions for {group}/{kind}'

docs = read('enabled')
environments = read('environments')
configmaps = [doc for doc in environments if doc['kind'] == 'ConfigMap']
config = next(doc['data'] for doc in configmaps if 'OBOT_SERVER_SUBSTRATE_WORKER_IMAGE' in doc.get('data', {}))
assert config['OBOT_SERVER_SUBSTRATE_ADDRESS'] == 'api.ate-system.svc:443'
assert config['OBOT_SERVER_SUBSTRATE_WORKER_IMAGE'] == 'ghcr.io/obot-platform/substrate/ateom-gvisor:v0.2.0'
# Application defaults and fixed credential paths are not passed as overrides.
for key in ('RUNNER_IMAGE', 'CA_FILE', 'TOKEN_FILE', 'CREDENTIAL_ADDRESS', 'CREDENTIAL_BUNDLE', 'CREDENTIAL_CLIENT_CA'):
    assert f'OBOT_SERVER_SUBSTRATE_{key}' not in config
assert config['OBOT_SERVER_SUBSTRATE_NAMESPACE'] == 'obot-agents'
assert 'OBOT_SERVER_CLAUDE_RUNNER_IMAGE' not in config
assert 'workerpool.yaml' not in resource(environments, 'ConfigMap', 'substrate-resources')['data']
roles = [doc for doc in environments if doc['kind'] == 'Role' and doc['metadata'].get('namespace') == 'obot-agents']
assert any('workerpools' in rule['resources'] and 'list' in rule['verbs'] and 'delete' in rule['verbs'] for role in roles for rule in role['rules'])
egress = resource(environments, 'Deployment', 'atenet-egress')['spec']['template']['spec']
args = next(c['args'] for c in egress['containers'] if c['name'] == 'ext-proc')
assert '--credential-provider-name=obot' in args
assert '--credential-provider-address=obot-obot-egress-credentials.obot.svc:8444' in args
service = resource(environments, 'Service', 'obot-obot-egress-credentials')
assert service['spec']['type'] == 'ClusterIP'
assert service['spec']['ports'][0]['port'] == 8444
assert service['spec']['ports'][0]['targetPort'] == 8444
pod = resource(environments, 'Deployment', 'obot-obot')['spec']['template']['spec']
projection = next(v['projected']['sources'] for v in pod['volumes'] if v['name'] == 'substrate-api')
assert any(s.get('podCertificate', {}).get('credentialBundlePath') == 'credential.pem' for s in projection)
assert any(s.get('clusterTrustBundle', {}).get('path') == 'pod-identity-ca.pem' for s in projection)
egress_data = resource(environments, 'ConfigMap', 'atenet-egress')['data']
# Hash the full rendered configuration so template-only changes also roll Envoy.
checksum = hashlib.sha256(json.dumps(egress_data, sort_keys=True, separators=(',', ':'), ensure_ascii=False).replace('&', r'\u0026').replace('<', r'\u003c').replace('>', r'\u003e').encode()).hexdigest()
assert resource(environments, 'Deployment', 'atenet-egress')['spec']['template']['metadata']['annotations']['obot.ai/egress-config'] == checksum
bootstrap = yaml.safe_load(resource(environments, 'ConfigMap', 'atenet-egress')['data']['envoy.yaml'])
# Only loopback exposes administrative operations; the pod-facing listener
# proxies exactly GET /ready and has no route for shutdown/configuration APIs.
admin = bootstrap['admin']['address']['socket_address']
assert admin == {'address': '127.0.0.1', 'port_value': 15000}
ready = next(l for l in bootstrap['static_resources']['listeners'] if l['name'] == 'readiness')
assert ready['address']['socket_address']['port_value'] == 15001
ready_hcm = ready['filter_chains'][0]['filters'][0]['typed_config']
routes = ready_hcm['route_config']['virtual_hosts'][0]['routes']
assert len(routes) == 1
assert routes[0]['match'] == {'path': '/ready', 'headers': [{'name': ':method', 'string_match': {'exact': 'GET'}}]}
assert routes[0]['route'] == {'cluster': 'local_admin', 'timeout': '1s'}
admin_cluster = next(c for c in bootstrap['static_resources']['clusters'] if c['name'] == 'local_admin')
assert admin_cluster['load_assignment']['endpoints'][0]['lb_endpoints'][0]['endpoint']['address']['socket_address'] == admin
for probe in ('readinessProbe', 'startupProbe'):
    envoy = next(c for c in egress['containers'] if c['name'] == 'envoy')
    assert envoy[probe]['httpGet'] == {'path': '/ready', 'port': 'readiness'}
assert {'name': 'readiness', 'containerPort': 15001} in envoy['ports']
assert all(p['containerPort'] != 15000 for p in envoy['ports'])

# Both cleartext and MITM stdout logs use Envoy's query-stripping formatter.
path_logs = []
def check_logs(value):
    if isinstance(value, dict):
        if value.get('path') == '/dev/stdout':
            fmt = value.get('log_format', {})
            if 'path' in fmt.get('json_format', {}):
                path_logs.append(fmt)
                assert fmt['json_format']['path'] == '%REQ_WITHOUT_QUERY(X-ENVOY-ORIGINAL-PATH?:PATH)%'
                assert fmt['formatters'] == [{'name': 'envoy.formatter.req_without_query', 'typed_config': {
                    '@type': 'type.googleapis.com/envoy.extensions.formatter.req_without_query.v3.ReqWithoutQuery'}}]
        for child in value.values():
            check_logs(child)
    elif isinstance(value, list):
        for child in value:
            check_logs(child)
check_logs(bootstrap)
assert len(path_logs) == 2
assert any(c['name'] == 'obot_mcp' for c in bootstrap['static_resources']['clusters'])
audit_cluster = next(c for c in bootstrap['static_resources']['clusters'] if c['name'] == 'obot_audit')
audit_tls = audit_cluster['transport_socket']['typed_config']['common_tls_context']
assert audit_tls['tls_params'] == {'tls_minimum_protocol_version': 'TLSv1_3', 'tls_maximum_protocol_version': 'TLSv1_3'}
assert audit_tls['alpn_protocols'] == ['h2']
assert audit_tls['tls_certificate_sds_secret_configs'][0]['name'] == 'obot_audit_client'
assert audit_tls['validation_context_sds_secret_config']['name'] == 'obot_audit_server'
chain = next(c for l in bootstrap['static_resources']['listeners'] for c in l.get('filter_chains', []) if c.get('name') == 'egress_cleartext')
hcm = chain['filters'][0]['typed_config']
assert hcm['route_config']['request_headers_to_remove'] == ['x-obot-egress-key']
assert [f['name'] for f in hcm['http_filters']][:3] == ['envoy.filters.http.ext_proc', 'envoy.filters.http.ext_proc', 'envoy.filters.http.lua']
assert hcm['route_config']['virtual_hosts'][0]['routes'][0]['route']['cluster'] == 'obot_mcp'
audit = hcm['http_filters'][0]['typed_config']
assert audit['stat_prefix'] == 'obot_audit'
assert audit['processing_mode']['response_body_mode'] == 'STREAMED'
assert not audit['failure_mode_allow']
assert any(c['name'] == 'sdsmint' for c in egress['initContainers'])
assert egress['securityContext']['fsGroup'] == 65532
inner = bootstrap['static_resources']['listeners'][1]
assert 'mitm' in inner['filter_chain_matcher']['matcher_tree']['exact_match_map']['map']
assert 'passthrough' not in inner['filter_chain_matcher']['matcher_tree']['exact_match_map']['map']
mitm = next(c for c in inner['filter_chains'] if c['name'] == 'egress_tls_mitm')
assert mitm['transport_socket']['name'] == 'envoy.transport_sockets.tls'

overrides = read('overrides')
config = resource(overrides, 'ConfigMap', 'obot-internal-config')['data']
assert config['OBOT_SERVER_SUBSTRATE_WORKER_IMAGE'] == 'custom/worker:test'
assert config['OBOT_SERVER_SUBSTRATE_RUNNER_IMAGE'] == 'custom/runner:test'
assert config['OBOT_SERVER_SUBSTRATE_ADDRESS'] == 'api.ate-system.svc:443'
assert config['OBOT_SERVER_SUBSTRATE_OBOT_URL'] == 'http://obot-obot.obot.svc:8081'
proxy = yaml.safe_load(resource(overrides, 'ConfigMap', 'atenet-egress')['data']['envoy.yaml'])
obot_cluster = next(c for c in proxy['static_resources']['clusters'] if c['name'] == 'obot_mcp')
assert obot_cluster['load_assignment']['endpoints'][0]['lb_endpoints'][0]['endpoint']['address']['socket_address'] == {'address': 'obot-obot.obot.svc', 'port_value': 8081}
assert config['OBOT_SERVER_SUBSTRATE_SNAPSHOT_LOCATION'] == 's3://custom'
assert config['OBOT_SERVER_SUBSTRATE_NAMESPACE'] == 'custom-workers'
assert resource(overrides, 'Namespace', 'custom-workers')
assert resource(overrides, 'Role', 'obot-obot-claude-pools')['metadata']['namespace'] == 'custom-workers'
obot_pod = resource(overrides, 'Deployment', 'obot-obot')['spec']['template']['spec']
assert any(volume['name'] == 'substrate-api' for volume in obot_pod['volumes'])
assert any(mount['name'] == 'substrate-api' for mount in obot_pod['containers'][0]['volumeMounts'])
assert sum(doc['kind'] == 'CustomResourceDefinition' for doc in docs) == 3
for name in ('ate-api-server', 'ate-controller', 'atenet-router', 'atenet-egress', 'podcertificate-controller'):
    assert resource(docs, 'Deployment', name)['metadata']['namespace'] in namespaces
for name in namespaces:
    assert resource(docs, 'Namespace', name)['metadata']['annotations']['helm.sh/resource-policy'] == 'keep'

bootstrap = resource(docs, 'Job', 'substrate-bootstrap-1')
assert not bootstrap['metadata'].get('annotations')
assert [container['args'][2] for container in bootstrap['spec']['template']['spec']['initContainers']] == [
    'podcertificate-controller-cas', 'jwt-authority-pool', 'actor-id-ca-pool', 'actor-id-ca-certs', 'egress-mitm-ca-pool']

api = resource(docs, 'Deployment', 'ate-api-server')
version = yaml.safe_load(Path('chart/charts/substrate/Chart.yaml').read_text())['appVersion']
atelet = resource(docs, 'DaemonSet', 'atelet-' + version.replace('.', '-'))
assert atelet['spec']['template']['spec']['nodeSelector']['kubernetes.io/os'] == 'linux'

scheduling = read('scheduling')
scheduling_config = resource(scheduling, 'ConfigMap', 'obot-internal-config')['data']
scheduling_pod = resource(scheduling, 'DaemonSet', atelet['metadata']['name'])['spec']['template']['spec']
assert scheduling_pod['nodeSelector'] == {'kubernetes.io/os': 'linux', 'workload': 'agents'}
assert json.loads(scheduling_config['OBOT_SERVER_SUBSTRATE_WORKER_NODE_SELECTOR']) == scheduling_pod['nodeSelector']
worker_tolerations = json.loads(scheduling_config['OBOT_SERVER_SUBSTRATE_WORKER_TOLERATIONS'])
assert worker_tolerations == [{'key': 'workload', 'operator': 'Equal', 'value': 'agents', 'effect': 'NoSchedule'}]
assert scheduling_pod['tolerations'] == [{'key': 'ate.dev/sandboxClass', 'operator': 'Exists', 'effect': 'NoSchedule'}] + worker_tolerations
assert json.loads(config['OBOT_SERVER_SUBSTRATE_WORKER_NODE_SELECTOR']) == {'kubernetes.io/os': 'linux'}
assert json.loads(config['OBOT_SERVER_SUBSTRATE_WORKER_TOLERATIONS']) == []
assert all(has_secret(workload, 'snapshots') for workload in (api, atelet))

# TLS must be gated by the upstream module's verdict, not just its transport.
egress = yaml.safe_load(resource(docs, 'ConfigMap', 'atenet-egress')['data']['envoy.yaml'])
outer, inner, readiness = egress['static_resources']['listeners']
filters = outer['filter_chains'][0]['filters'][0]['typed_config']['http_filters']
policy_filter = next(f['typed_config'] for f in filters if f['name'] == 'envoy.filters.http.ext_proc')
assert not policy_filter['failure_mode_allow']
assert 'dev.ate.policy.egress' in policy_filter['metadata_options']['receiving_namespaces']['untyped']
state = [v for f in filters if f['name'] == 'envoy.filters.http.set_filter_state'
         for v in f['typed_config']['on_request_headers']]
assert any(v['object_key'] == 'dev.ate.policy.egress' and v['shared_with_upstream'] == 'ONCE' for v in state)
pool_key = next(v for v in state if v['object_key'] == 'envoy.network.upstream_server_name')
assert pool_key['format_string']['text_format_source']['inline_string'] == '%DOWNSTREAM_PEER_URI_SAN%|%REQ(:AUTHORITY)%'
assert 'shared_with_upstream' not in pool_key

matcher = inner['filter_chain_matcher']['matcher_tree']
assert matcher['input']['typed_config']['key'] == 'dev.ate.egress.filter_chain'
routes = matcher['exact_match_map']['map']
assert set(routes) == {'mitm', 'cleartext', 'denied'}
chains = {c['name']: c for c in inner['filter_chains']}
assert routes['denied']['action']['typed_config']['value'] not in chains
assert 'on_no_match' not in inner['filter_chain_matcher']
assert all('filter_chain_match' not in c for c in chains.values())
assert chains[routes['mitm']['action']['typed_config']['value']]['transport_socket']['name'] == 'envoy.transport_sockets.tls'

postgres = resource(docs, 'StatefulSet', 'postgres')
pg_config = resource(docs, 'ConfigMap', 'postgres-config')['data']
assert 'hostssl atepg postgres all scram-sha-256 clientcert=verify-ca' in pg_config['pg_hba.conf']
assert 'trust clientcert' not in pg_config['pg_hba.conf']
password = resource(docs, 'Secret', 'substrate-postgres-auth')
assert len(base64.b64decode(password['data']['password'])) == 48
assert password['metadata']['annotations']['helm.sh/resource-policy'] == 'keep'
secret_ref = {'name': 'substrate-postgres-auth', 'key': 'password'}
pg_container = postgres['spec']['template']['spec']['containers'][0]
assert next(e for e in pg_container['env'] if e['name'] == 'POSTGRES_PASSWORD')['valueFrom']['secretKeyRef'] == secret_ref
assert next(e for e in api['spec']['template']['spec']['containers'][0]['env'] if e['name'] == 'PGPASSWORD')['valueFrom']['secretKeyRef'] == secret_ref
assert not any(e['name'] == 'POSTGRES_HOST_AUTH_METHOD' for e in pg_container['env'])
assert 'ALTER ROLE postgres PASSWORD' in pg_container['lifecycle']['postStart']['exec']['command'][-1]
policy = resource(docs, 'NetworkPolicy', 'substrate-postgres')['spec']
assert policy == {
    'podSelector': {'matchLabels': {'app': 'postgres'}},
    'policyTypes': ['Ingress'],
    'ingress': [{'from': [{'podSelector': {'matchLabels': {'app': 'ate-api-server'}}}],
                 'ports': [{'protocol': 'TCP', 'port': 5432}]}],
}

assert postgres['spec']['volumeClaimTemplates'][0]['spec']['resources']['requests']['storage'] == '20Gi'
assert any(volume.get('projected', {}).get('sources', [{}])[0].get('podCertificate') is not None
           for volume in postgres['spec']['template']['spec']['volumes'])

external = read('external')
assert not any(doc['metadata'].get('namespace') == 'ate-system' and
               doc['metadata']['name'] in ('postgres', 'postgres-config', 'substrate-postgres', 'substrate-postgres-auth') for doc in external)
assert resource(external, 'Secret', 'ate-api-server-secret-envvars')['stringData']['ATE_API_POSTGRES_READ_WRITE_CONNECTION_STRING'] == dsn
assert resource(external, 'ConfigMap', 'ate-api-server-envvars')['data'].get('ATE_API_POSTGRES_READ_WRITE_CONNECTION_STRING') is None
assert dsn not in json.dumps([doc for doc in external if doc['kind'] == 'ConfigMap'])
external_api = resource(external, 'Deployment', 'ate-api-server')
assert has_secret(external_api, 'ate-api-server-secret-envvars')
assert resource(external, 'Secret', 'ate-api-server-secret-envvars')['stringData']['ATE_API_POSTGRES_OWNER_CONNECTION_STRING'] == dsn
assert resource(external, 'ConfigMap', 'ate-api-server-envvars')['data']['ATE_API_POSTGRES_OWNER_ROLE'] == 'substrate'
assert resource(external, 'ConfigMap', 'ate-api-server-envvars')['data']['ATE_API_POSTGRES_READ_WRITE_ROLE'] == 'substrate'
assert resource(docs, 'ConfigMap', 'ate-api-server-envvars')['data']['ATE_API_POSTGRES_OWNER_ROLE'] == 'postgres'
api_args = external_api['spec']['template']['spec']['containers'][0]['args']
assert '--postgres-read-write-connection-string=@env' in api_args
assert '--postgres-owner-connection-string=@env' in api_args
assert not any(arg.startswith('--postgres-connection-string=') for arg in api_args)
assert not any(arg.startswith('--gcp-auth-for-image-pulls=') for arg in atelet['spec']['template']['spec']['containers'][0]['args'])
assert any(v['object_key'] == 'envoy.upstream.dynamic_port' for v in state)

assert external_api['spec']['template']['metadata']['annotations']['checksum/postgres-dsn'] != api['spec']['template']['metadata']['annotations']['checksum/postgres-dsn']

provider_pod = resource(read('image-provider'), 'DaemonSet', 'atelet-' + version.replace('.', '-'))['spec']['template']['spec']
provider_args = provider_pod['containers'][0]['args']
assert '--image-credential-provider-config=/run/image-credential-provider/config.yaml' in provider_args
assert '--image-credential-provider-bin-dir=/run/image-credential-provider/bin' in provider_args
volumes = {v['name']: v for v in provider_pod['volumes']}
assert volumes['image-credential-provider-config']['hostPath']['path'] == '/node/config.yaml'
assert volumes['image-credential-provider-bin']['hostPath']['path'] == '/node/bin'
mounts = {v['name']: v for v in provider_pod['containers'][0]['volumeMounts']}
assert mounts['image-credential-provider-config']['readOnly']
assert mounts['image-credential-provider-bin']['readOnly']
controller_role = resource(docs, 'ClusterRole', 'ate-controller')
assert any('pods' in rule['resources'] and 'delete' in rule['verbs'] for rule in controller_role['rules'])

# Obot generates random keys at render time; compare Substrate resources.
def substrate(docs):
    return [doc for doc in docs if doc['metadata']['name'] != 'substrate-postgres-auth' and
            (doc['metadata'].get('namespace') in namespaces or
             doc['kind'] in ('CustomResourceDefinition', 'SandboxConfig'))]

assert substrate(read('packaged')) == substrate(docs), 'Packaged Substrate resources differ'
print('Substrate chart checks passed (disabled, enabled, external PostgreSQL, bootstrap, storage, CRDs, TLS policy routing, package).')
PY
