#!/usr/bin/env python3
"""Register Meridian Operations and prepare unapproved AI capability suggestions."""
import json
from pathlib import Path
import subprocess
import time
from urllib.parse import urlparse

from client import OBOT, call
from demo import api, backup

HERE = Path(__file__).resolve().parent
ROOT = HERE.parents[1]
LOCAL = HERE / '.local'
FIXTURE = 'http://127.0.0.1:9125'
NAME = 'Meridian Operations'


def main():
    if urlparse(OBOT).hostname not in ('localhost', '127.0.0.1', '::1'):
        raise RuntimeError('This fixture targets a local development instance.')
    if not call('GET', '/api/healthz', timeout=5).ok:
        raise RuntimeError(f'Obot is not healthy at {OBOT}.')
    backup()
    response = call('GET', FIXTURE + '/healthz', timeout=2)
    if response.ok and response.text.strip() != 'meridian-operations-v1':
        raise RuntimeError('Port 9125 belongs to another service; leaving it untouched.')
    if not response.ok:
        binary = LOCAL / 'meridian-operations-mcp'
        subprocess.run(['go', 'build', '-o', str(binary), './tools/demo/labelserver'], cwd=ROOT, check=True)
        with (LOCAL / 'meridian-operations.log').open('a') as log:
            process = subprocess.Popen([str(binary)], stdin=subprocess.DEVNULL, stdout=log, stderr=log, start_new_session=True)
        (LOCAL / 'meridian-operations.pid').write_text(str(process.pid))
        for _ in range(40):
            response = call('GET', FIXTURE + '/healthz', timeout=1)
            if response.ok and response.text.strip() == 'meridian-operations-v1':
                break
            time.sleep(.25)
        else:
            raise RuntimeError('Meridian Operations did not start; inspect its local log.')
    entries = api('GET', '/api/mcp-catalogs/default/entries').get('items') or []
    entry = next((e for e in entries if (e.get('manifest') or {}).get('name') == NAME), None)
    if not entry:
        entry = api('POST', '/api/mcp-catalogs/default/entries', {
            'name': NAME,
            'shortDescription': 'Account operations, supplier intake, finance workflows and access leases',
            'runtime': 'remote', 'remoteConfig': {'fixedURL': FIXTURE + '/mcp'},
        })
    if not (entry.get('manifest') or {}).get('toolPreview'):
        path = f'/api/mcp-catalogs/default/entries/{entry["id"]}/generate-tool-previews'
        # Initial catalog reconciliation can race the preview update. Retrying
        # this idempotent discovery operation preserves the latest resource.
        for attempt in range(5):
            response = call('POST', path, {}, timeout=60)
            if response.ok:
                entry = response.json
                break
            if response.status != 409:
                raise RuntimeError(f'Generate tool previews: HTTP {response.status}: {response.text[:500]}')
            time.sleep(.5 * (attempt + 1))
        else:
            raise RuntimeError('Catalog reconciliation is still busy; run this command again.')
    resource = {'resourceType': 'catalog-entry', 'resourceID': entry['id']}
    print('Asking the Supervisor to review seven tools...', flush=True)
    result = api('POST', '/api/provenance/tool-label-suggestions', resource, timeout=150)
    items = result.get('items') or []
    if len(items) != 7 or not all(i.get('proposal') and not i.get('stale') for i in items):
        raise RuntimeError('Expected seven current tool-label proposals.')
    additions = 0
    for item in items:
        current = set(item.get('labels') or [])
        suggested = set(item['proposal'].get('labels') or [])
        added = sorted(suggested - current)
        additions += bool(added)
        print(f"  {item['tool']}: {', '.join(sorted(current)) or '(unlabeled)'} -> {', '.join(sorted(suggested)) or '(none)'}")
    if not additions:
        raise RuntimeError('No additional labels were proposed; inspect the model response.')
    (LOCAL / 'meridian-operations.json').write_text(json.dumps(resource, indent=2))
    print('\nRecommendations are pending; no labels have been approved.')
    print(f"Review: {OBOT}/tool-labels?resourceType=catalog-entry&resourceID={entry['id']}")
    print(f"MCP: {OBOT}/mcp-servers/c/{entry['id']}?view=tools")


if __name__ == '__main__':
    main()
