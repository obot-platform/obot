#!/usr/bin/env bash
set -euo pipefail
# Requires an existing Envoy dataplane image, Docker, Helm, OpenSSL, and PyYAML.
cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.."
image=${1:?Usage: test-obot-egress.sh ENVOY_IMAGE}
work=$(mktemp -d)
trap 'rm -rf -- "$work"' EXIT
helm template obot chart --namespace obot --set dev.useEmbeddedDb=true \
    --set substrate.enabled=true --set substrate.image.repository=example/substrate \
    --set config.OBOT_SERVER_SUBSTRATE_ADDRESS=api.ate-system.svc:443 \
    --set config.OBOT_SERVER_SUBSTRATE_SNAPSHOT_LOCATION=s3://bucket > "$work/chart.yaml"
openssl req -x509 -newkey rsa:2048 -nodes -keyout "$work/key.pem" \
    -out "$work/cert.pem" -days 1 -subj /CN=test 2>/dev/null
python3 - "$work" <<'PY'
from pathlib import Path
import sys
import yaml
root = Path(sys.argv[1])
data = next(d['data'] for d in yaml.safe_load_all((root / 'chart.yaml').read_text())
            if d and d['kind'] == 'ConfigMap' and d['metadata']['name'] == 'atenet-egress')
(root / 'bundle.pem').write_text((root / 'cert.pem').read_text() + (root / 'key.pem').read_text())
for name, source in data.items():
    doc = yaml.safe_load(source)
    def substitute(value):
        if isinstance(value, dict):
            for key, item in value.items():
                if key == 'filename' and '/run/' in item:
                    value[key] = '/test/bundle.pem'
                elif key == 'path' and isinstance(item, str) and item.startswith('/etc/envoy/'):
                    value[key] = item.replace('/etc/envoy/', '/test/')
                else:
                    substitute(item)
        elif isinstance(value, list):
            for item in value:
                substitute(item)
    substitute(doc)
    if name == 'envoy.yaml':
        doc['node'] = {'id': 'test', 'cluster': 'test'}
        inner = next(listener for listener in doc['static_resources']['listeners']
                     if listener['name'] == 'inner_listener')
        denial_log = next(log for log in inner['access_log']
                          if log['name'] == 'envoy.access_loggers.tcp_grpc')
        assert denial_log['filter']['response_flag_filter']['flags'] == ['NR']
        config = denial_log['typed_config']['common_config']
        assert config['grpc_service']['envoy_grpc']['cluster_name'] == 'obot_audit'
        assert 'dev.ate.actor.identity' in config['filter_state_objects_to_log']
        for listener in doc['static_resources']['listeners']:
            for chain in listener.get('filter_chains', []):
                for filter in chain['filters']:
                    for http in filter.get('typed_config', {}).get('http_filters', []):
                        if http['name'] == 'envoy.filters.http.lua':
                            # The assertions execute in Envoy's Lua runtime during validation.
                            http['typed_config']['default_source_code']['inline_string'] = (
                                'local obot_authority = "obot.obot.svc:8080"\n'
                                + Path('chart/files/obot-egress.lua').read_text()
                                + Path('tools/test-obot-egress.lua').read_text())
    (root / name).write_text(yaml.safe_dump(doc))
PY
docker run --rm -v "$work:/test:ro" "$image" --mode validate -c /test/envoy.yaml
