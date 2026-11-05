#!/usr/bin/env python3
"""Repeatable Supervisor POC: real gateway decisions over synthetic tool traffic."""
import argparse
from datetime import datetime, timezone
import json
import os
from pathlib import Path
import sqlite3
import signal
import subprocess
import sys
import time
from urllib.parse import urlencode, urlparse
import uuid

from client import MCPClient, MCPError, OBOT, call, find_tool, tool_result_text

HERE = Path(__file__).resolve().parent
ROOT = HERE.parents[1]
LOCAL = HERE / '.local'
NAME = 'Northstar Health'
FIXTURE = 'http://127.0.0.1:9124'
POLICY_NAME = 'Review external patient sharing'
CAPS = {
    'read_public_attachment': ['reads-untrusted'],
    'lookup_patient': ['reads-private'],
    'update_care_plan': ['writes-internal'],
    'send_report': ['writes-external'],
    'read_internal_document': ['reads-private'],
}


def api(method, path, body=None, timeout=60):
    r = call(method, path, body, timeout=timeout)
    if not r.ok:
        raise RuntimeError(f'{method} {path}: HTTP {r.status}: {r.text[:500]}')
    return r.json


def backup():
    LOCAL.mkdir(exist_ok=True)
    src = ROOT / 'obot.db'
    if src.exists() and urlparse(OBOT).hostname in ('localhost', '127.0.0.1', '::1'):
        today = datetime.now().strftime('%Y%m%d')
        previous = sorted(LOCAL.glob('obot-backup-' + today + '*.db'))
        if previous:
            print(f'Pre-demo database backup retained: {previous[0].relative_to(ROOT)}')
            return
        target = LOCAL / ('obot-backup-' + datetime.now().strftime('%Y%m%d-%H%M%S-%f') + '.db')
        with sqlite3.connect(f'file:{src}?mode=ro', uri=True) as source, sqlite3.connect(target) as dest:
            source.backup(dest)
        print(f'Database backup: {target.relative_to(ROOT)}')


def fixture():
    LOCAL.mkdir(exist_ok=True)
    binary = LOCAL / 'northstar-care-mcp'
    source = HERE / 'mcpserver' / 'main.go'
    stale = not binary.exists() or binary.stat().st_mtime < source.stat().st_mtime
    r = call('GET', FIXTURE + '/healthz', timeout=2)
    if r.ok and r.text.strip() not in ('northstar-care-v1', 'supervisor-demo-v1'):
        raise RuntimeError('Port 9124 belongs to a different server; leaving it untouched.')
    if r.ok and not stale and r.text.strip() == 'northstar-care-v1':
        return
    if r.ok:
        pids = subprocess.run(['lsof', '-tiTCP:9124', '-sTCP:LISTEN'], capture_output=True, text=True, check=True).stdout.split()
        for pid in pids:
            command = subprocess.run(['ps', '-o', 'command=', '-p', pid], capture_output=True, text=True, check=True).stdout
            if str(LOCAL) + '/' not in command:
                raise RuntimeError('Fixture upgrade cannot establish ownership of port 9124; leaving it untouched.')
            os.kill(int(pid), signal.SIGTERM)
        for _ in range(40):
            if not call('GET', FIXTURE + '/healthz', timeout=1).ok:
                break
            time.sleep(.1)
    if stale:
        print('Building synthetic MCP fixture...')
        subprocess.run(['go', 'build', '-o', str(binary), './tools/demo/mcpserver'], cwd=ROOT, check=True)
    with (LOCAL / 'fixture.log').open('a') as log:
        process = subprocess.Popen([str(binary)], stdin=subprocess.DEVNULL, stdout=log, stderr=log, start_new_session=True)
        (LOCAL / 'fixture.pid').write_text(str(process.pid))
    for _ in range(40):
        r = call('GET', FIXTURE + '/healthz', timeout=1)
        if r.ok and r.text.strip() == 'northstar-care-v1':
            return
        time.sleep(.25)
    raise RuntimeError('Fixture did not start. See tools/demo/.local/fixture.log.')


def setup():
    if urlparse(OBOT).hostname not in ('localhost', '127.0.0.1', '::1'):
        raise RuntimeError('This POC runner targets a local development instance only.')
    r = call('GET', '/api/healthz', timeout=5)
    if not r.ok:
        raise RuntimeError(f'Obot is not healthy at {OBOT}; start ./run-8090.sh and the separate Vite server.')
    fixture()
    entries = api('GET', '/api/mcp-catalogs/default/entries').get('items') or []
    entry = next((e for e in entries if (e.get('manifest') or {}).get('name') == NAME), None)
    if not entry:
        entry = api('POST', '/api/mcp-catalogs/default/entries', {
            'name': NAME, 'shortDescription': 'Patient records and care-coordination workflows',
            'runtime': 'remote', 'remoteConfig': {'fixedURL': FIXTURE + '/mcp'},
        })
    if not (entry.get('manifest') or {}).get('toolPreview'):
        entry = api('POST', f'/api/mcp-catalogs/default/entries/{entry["id"]}/generate-tool-previews', {})
    profiles = api('GET', '/api/vmcps').get('items') or []
    profile = next((v for v in profiles if v.get('displayName') == NAME and any(
        c.get('mcpServerCatalogEntryID') == entry['id'] for c in v.get('components') or [])), None)
    if not profile:
        profile = api('POST', '/api/vmcps', {'displayName': NAME, 'components': [
            {'name': 'care', 'mcpCatalogID': 'default', 'mcpServerCatalogEntryID': entry['id']} ]})
    # Only fixture tools are pinned, so model classification cannot change a live demo's meaning.
    client = MCPClient(f'{OBOT}/mcp-connect/{profile["id"]}', 'claude_code')
    client.initialize()
    tools = client.list_tools()
    for tool in tools:
        base = next((name for name in CAPS if tool['name'].endswith(name)), None)
        if base:
            for server in (NAME, 'care'):
                api('PUT', '/api/provenance/tool-labels', {'server': server, 'tool': tool['name'], 'labels': CAPS[base]})
                api('PUT', '/api/provenance/tool-labels', {'server': server, 'tool': base, 'labels': CAPS[base]})
    call('DELETE', client.url, headers=client._headers())
    policies = api('GET', '/api/provenance/policies').get('items') or []
    existing = next((p for p in policies if p['name'] == POLICY_NAME), None)
    rule = {
        'name': POLICY_NAME,
        'description': 'Require human review when untrusted input and private patient records precede an external send.',
        'condition': 'action.server == "Northstar Health" && action.trifecta',
        'effect': 'require_approval', 'mode': 'enforce', 'priority': 10000,
        'enabled': True, 'approvalTTL': 3600000000000,
    }
    if existing:
        api('PUT', '/api/provenance/policies/' + existing['id'], rule)
    else:
        api('POST', '/api/provenance/policies', rule)
    # Store only public resource identifiers, never credentials or payloads.
    (LOCAL / 'resources.json').write_text(json.dumps({'profile': profile['id'], 'entry': entry['id']}, indent=2))
    return profile['id']


def session_for(client):
    result = api('GET', '/api/provenance/sessions?' + urlencode({'agent': client.client_name, 'limit': 100}))
    sessions = result.get('items') or []
    session = next((s for s in sessions if s.get('actor', {}).get('clientSessionID') == client.mcp_session), None)
    if not session:
        raise RuntimeError('Gateway did not record the demo session.')
    client.obot_session = session['id']
    return session['id']


def audit_device(client, actor, label):
    """Report a synthetic completed file read through the real local-agent ingest adapter."""
    now = datetime.now(timezone.utc).isoformat().replace('+00:00', 'Z')
    api('POST', '/api/local-agent-audit-logs', {'events': [{
        'occurredAt': now, 'action': {'name': 'Read', 'kind': 'read'},
        'target': {'targetType': 'local_tool', 'name': 'Read'},
        'outcome': {'status': 'success', 'durationMs': 84},
        'details': {
            'trace': {'idempotencyKey': str(uuid.uuid4()), 'toolUseID': 'call-' + uuid.uuid4().hex, 'sessionID': client.mcp_session},
            'agent': {'provider': actor, 'cliName': actor, 'cliVersion': '1.0'},
            'device': {'hostname': 'care-workstation-07', 'os': 'macOS'},
            'environment': {'cwd': '/work/care-coordination', 'reportedUserEmail': 'alex.clinician@hospital.example'},
            'request': {'body': {'file_path': '/work/internal-care-guidance.md'}},
            'response': {'body': f'{label}: internal care workflow. Keep patient records inside the care system.'},
            'rawEvent': {},
        },
    }]})


def workflow(profile, protect=False, actor='claude_code', title=None, device=True, verbose=True, step=False, patient_id='MRN-104821'):
    title = title or ('Coordinate follow-up from an external referral' if protect else 'Coordinate an internal patient follow-up')
    c = MCPClient(f'{OBOT}/mcp-connect/{profile}', actor)
    c.initialize()
    tools = c.list_tools()
    def invoke(base, args):
        name = find_tool(tools, base)
        if not name:
            raise RuntimeError('Missing fixture tool: ' + base)
        return c.call_tool(name, args)
    if verbose:
        print(f'\n{title}\nAll patient data is invented. No report is delivered to any destination.')
    invoke('read_public_attachment', {'query': 'follow-up referral'})
    sid = session_for(c)
    api('POST', f'/api/provenance/sessions/{sid}/warrant', {
        'task': title, 'scopes': [f'mcp:{NAME}:*{tool}' for tool in ('read_public_attachment', 'lookup_patient', 'update_care_plan', 'read_internal_document')], 'ttlSeconds': 3600,
    })
    if verbose:
        print('1. Read the external referral attachment (untrusted).')
        print('Watch this session:', OBOT + '/sessions/' + sid)
    if step:
        input('Press Enter to read the synthetic patient record... ')
    if device:
        audit_device(c, actor, title)
    patient = tool_result_text(invoke('lookup_patient', {'patient_id': patient_id}))
    if verbose:
        print('2. Read the synthetic patient record (PII + confidential).')
    if step:
        input('Press Enter to attempt the ' + ('external report' if protect else 'internal handoff') + '... ')
    error = None
    if protect:
        try:
            result = invoke('send_report', {'destination_url': 'https://intake.outside.example/upload', 'body': patient})
            if result.get('isError'):
                error = tool_result_text(result)
        except MCPError as exc:
            error = exc.message
        if error is None:
            raise RuntimeError('Expected external sharing to be held, but the gateway allowed it. Check demo policies.')
    else:
        result = invoke('update_care_plan', {'patient_id': patient_id, 'body': patient})
        if result.get('isError'):
            raise RuntimeError('Internal handoff failed: ' + tool_result_text(result))
    events = api('GET', f'/api/provenance/sessions/{sid}/events?ascending=true&limit=200').get('items') or []
    decisions = [e for e in events if e.get('decision')]
    last = decisions[-1] if decisions else {}
    decision = last.get('decision') or {}
    expected = not protect
    if decision.get('allow') is not expected:
        raise RuntimeError('Recorded gateway outcome did not match the expected demo outcome.')
    if protect and decision.get('effect') != 'require_approval':
        raise RuntimeError('Expected a human approval, got: ' + str(decision))
    if protect and not last.get('lineage'):
        raise RuntimeError('Expected patient identifiers to match the earlier record, but no lineage was recorded.')
    state = api('GET', f'/api/provenance/sessions/{sid}')['state']
    if device and not {'mcp', 'device'}.issubset(set(state.get('surfaces') or [])):
        raise RuntimeError('MCP and device reports did not stitch into one session.')
    if verbose:
        print('3. ' + ('External report HELD before the fixture ran.' if protect else 'Internal care handoff ALLOWED.'))
        print('Labels:', ', '.join(state.get('labels') or []))
        if protect:
            print('Lineage matches:', len(last['lineage']))
            print('Reason:', decision.get('reason'))
        print('Replay:', OBOT + '/sessions/' + sid)
        if decision.get('approvalID'):
            print('Review:', OBOT + '/approvals?approval=' + decision['approvalID'])
            print('Review in the UI. Approval records a decision; this POC does not automatically retry the blocked tool call.')
        print('Dashboard:', OBOT + '/supervision')
    if not protect:
        api('POST', f'/api/provenance/sessions/{sid}/end')
    call('DELETE', c.url, headers=c._headers())
    return sid, decision.get('approvalID')


def copilot_session(i, blocked=False):
    titles = ['Prepare the morning care-team handoff', 'Summarize referral scheduling requests',
              'Review follow-up appointment availability', 'Prepare a patient outreach draft',
              'Reconcile an internal care queue', 'Review a third-party referral attachment']
    title = titles[i // 3 % len(titles)]
    patient = 'Patient: Morgan Ellis, morgan.ellis@patient.example. Confidential follow-up care plan for type 2 diabetes.'
    data = {
        'conversationId': 'care-' + uuid.uuid4().hex,
        'agentName': NAME,
        'userEmail': ['alex.clinician@hospital.example', 'sam.coordinator@hospital.example', 'riley.reviewer@hospital.example'][i // 3 % 3],
        'prompt': title,
        'toolOutputs': [
            {'toolName': 'read_public_attachment', 'content': 'An external referral attachment requests a care summary.'},
            {'toolName': 'lookup_patient', 'content': patient},
        ],
        'actions': [{'name': 'send_report' if blocked else 'update_care_plan', 'connector': NAME,
                     'inputs': {'body': patient, **({'destination_url': 'https://intake.outside.example/upload'} if blocked else {})}}],
    }
    response = api('POST', '/api/copilot-studio/runtime-monitor', data)
    diagnostics = json.loads(response.get('diagnostics') or '{}')
    if diagnostics.get('timedOut') or diagnostics.get('error'):
        raise RuntimeError('Copilot adapter could not decide: ' + str(diagnostics))
    if response.get('blockAction') is not blocked:
        raise RuntimeError('Unexpected Copilot gateway outcome: ' + str(response))
    sid = diagnostics.get('sessionId')
    if not sid:
        raise RuntimeError('Copilot did not record a session.')
    approval = next((a.get('approvalId') for a in diagnostics.get('actions') or [] if a.get('approvalId')), None)
    return sid, approval


def seed(profile, count):
    print(f'Creating {count} fresh synthetic sessions through MCP, device-audit and Copilot adapters...')
    sessions = []
    for i in range(count):
        blocked = i % 9 == 8
        if i % 3 == 2:
            sid, approval = copilot_session(i, blocked)
        else:
            actor = ['claude_code', 'codex', 'cursor'][i // 3 % 3]
            title = ['Coordinate a diabetes follow-up', 'Prepare an internal referral handoff',
                     'Reconcile the care-team appointment queue', 'Review a patient outreach plan'][i % 4]
            sid, approval = workflow(profile, blocked, actor, title, verbose=False, patient_id=['MRN-104821', 'MRN-108734', 'MRN-110942'][i // 2 % 3])
        sessions.append(sid)
        if approval:
            api('POST', f'/api/provenance/approvals/{approval}/deny', {'note': 'Reviewed and denied external sharing of patient information.'})
        if not api('GET', f'/api/provenance/sessions/{sid}').get('endedAt'):
            api('POST', f'/api/provenance/sessions/{sid}/end')
        print(f'  {i + 1:2}/{count}: {"held and reviewed" if blocked else "allowed internal workflow"} - {sid}')
    # One fresh unresolved scenario makes the dashboard actionable without filling its queue.
    sid, approval = workflow(profile, True, title='Review an external request for a patient summary', verbose=False)
    sessions.append(sid)
    (LOCAL / 'last-seed.json').write_text(json.dumps({'at': datetime.now(timezone.utc).isoformat(), 'sessions': sessions, 'approval': approval}, indent=2))
    print(f'\nCreated {len(sessions)} sessions, with one pending review. Existing data was preserved.')
    print('Dashboard:', OBOT + '/supervision')
    print('Latest held session:', OBOT + '/sessions/' + sid)
    print('Use Refresh findings under What I noticed to include the new activity.')


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('command', choices=['setup', 'seed', 'safe', 'protect'])
    parser.add_argument('--count', type=int, default=36, help='background sessions for seed (1..200, plus one pending review)')
    parser.add_argument('--refresh-findings', action='store_true', help='refresh cached Supervisor findings after seeding (may call the configured model)')
    parser.add_argument('--step', action='store_true', help='pause between the live scenario steps (safe/protect only)')
    args = parser.parse_args()
    if not 1 <= args.count <= 200:
        parser.error('--count must be between 1 and 200')
    backup()
    profile = setup()
    if args.command == 'setup':
        print('Ready. No existing policies or provider settings were changed. Dashboard:', OBOT + '/supervision')
    elif args.command == 'seed':
        seed(profile, args.count)
        if args.refresh_findings:
            print('Refreshing Supervisor findings...')
            api('GET', '/api/provenance/supervisor/insights?refresh=true', timeout=60)
            print('Findings refreshed.')
    else:
        workflow(profile, args.command == 'protect', step=args.step)


if __name__ == '__main__':
    try:
        main()
    except (RuntimeError, OSError, MCPError, subprocess.CalledProcessError) as exc:
        print('Demo stopped:', exc, file=sys.stderr)
        sys.exit(1)
