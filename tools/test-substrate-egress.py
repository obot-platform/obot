#!/usr/bin/env python3
"""Exercise the chart's readiness boundary and stdout log formats using Envoy.

Usage: python3 tools/test-substrate-egress.py ENVOY_IMAGE
Requires Docker, Helm, and PyYAML. Does not build images or change deployments.
"""

import copy
import json
from pathlib import Path
import subprocess
import sys
import tempfile
import time
import urllib.error
import urllib.request

import yaml


def docker(*args):
    return subprocess.check_output(["docker", *args], text=True).strip()


def request(port, path, method="GET", headers=None):
    req = urllib.request.Request(f"http://127.0.0.1:{port}{path}", method=method, headers=headers or {})
    try:
        with urllib.request.urlopen(req, timeout=1) as response:
            response.read()
            return response.status
    except urllib.error.HTTPError as error:
        return error.code


def main(image):
    root = Path(__file__).resolve().parents[1]
    rendered = subprocess.check_output([
        "helm", "template", "obot", str(root / "chart"), "--set", "dev.useEmbeddedDb=true",
        "--set", "substrate.enabled=true",
        "--set", "config.OBOT_SERVER_SUBSTRATE_SNAPSHOT_LOCATION=s3://test",
    ], text=True)
    docs = list(yaml.safe_load_all(rendered))
    config = next(d for d in docs if d and d['kind'] == 'ConfigMap' and d['metadata']['name'] == 'atenet-egress')
    bootstrap = yaml.safe_load(config['data']['envoy.yaml'])
    resources = bootstrap['static_resources']
    readiness = next(l for l in resources['listeners'] if l['name'] == 'readiness')
    cluster = next(c for c in resources['clusters'] if c['name'] == 'local_admin')
    inner = resources['listeners'][1]
    logs = [log for chain in inner['filter_chains'] for log in chain['filters'][0]['typed_config'].get('access_log', [])
            if log['typed_config'].get('path') == '/dev/stdout' and
            'path' in log['typed_config'].get('log_format', {}).get('json_format', {})]
    assert len(logs) == 2

    # Run the production readiness listener unchanged; exercise the production
    # log formats on a separate direct-response listener without external traffic.
    traffic = copy.deepcopy(readiness)
    traffic['name'] = 'test_traffic'
    traffic['address']['socket_address']['port_value'] = 15002
    hcm = traffic['filter_chains'][0]['filters'][0]['typed_config']
    hcm['access_log'] = logs
    hcm['route_config']['virtual_hosts'][0]['routes'] = [{'match': {'prefix': '/'}, 'direct_response': {'status': 200}}]
    config = {'admin': bootstrap['admin'], 'static_resources': {'listeners': [readiness, traffic], 'clusters': [cluster]}}

    with tempfile.TemporaryDirectory() as work:
        Path(work, 'envoy.yaml').write_text(yaml.safe_dump(config))
        container = docker('run', '--rm', '-d', '--entrypoint', 'envoy',
                           '-p', '127.0.0.1::15000', '-p', '127.0.0.1::15001', '-p', '127.0.0.1::15002',
                           '-v', f'{work}:/test:ro', image, '-c', '/test/envoy.yaml', '--log-level', 'error', '--concurrency', '1', '--file-flush-interval-msec', '50')
        try:
            ports = {p: int(docker('port', container, str(p)).rsplit(':', 1)[1]) for p in (15000, 15001, 15002)}
            deadline = time.monotonic() + 20
            while True:
                try:
                    if request(ports[15001], '/ready') == 200:
                        break
                except (OSError, urllib.error.URLError):
                    pass
                if time.monotonic() > deadline:
                    raise AssertionError('Envoy did not become ready: ' + docker('logs', container))
                time.sleep(0.1)

            for path, method in (('/quitquitquit', 'POST'), ('/config_dump', 'GET'), ('/ready', 'POST')):
                assert request(ports[15001], path, method) == 404
            try:
                request(ports[15000], '/server_info')
            except (OSError, urllib.error.URLError):
                pass
            else:
                raise AssertionError('Admin listener is reachable through the pod network')

            signed = '/download?X-Amz-Signature=synthetic-secret&token=synthetic-token'
            assert request(ports[15002], signed) == 200
            assert request(ports[15002], signed, headers={'x-envoy-original-path': signed}) == 200
            deadline = time.monotonic() + 5
            while True:
                output = docker('logs', container)
                entries = [json.loads(line) for line in output.splitlines() if line.startswith('{')]
                if len(entries) >= 4 or time.monotonic() > deadline:
                    break
                time.sleep(0.1)
            assert len(entries) == 4, output
            assert all(entry['path'] == '/download' for entry in entries), output
            assert 'synthetic-secret' not in output and 'synthetic-token' not in output, output
            print('Envoy readiness isolation and signed-URL log redaction passed.')
        finally:
            docker('rm', '-f', container)


if __name__ == '__main__':
    if len(sys.argv) != 2:
        sys.exit(__doc__)
    main(sys.argv[1])
