#!/usr/bin/env python3
"""Add realistic denied activity using actual MCP gateway decisions over invented data."""
from datetime import datetime, timedelta, timezone
import json
import sys

from client import MCPClient, MCPError, OBOT, call, find_tool, tool_result_text
from demo import LOCAL, api, backup, session_for

CARE_TITLES = ['Prepare an external referral packet', 'Reconcile patient follow-up contacts']
FINANCE_TITLE = 'Send the Westbrook supplier renewal brief'
WORKSPACE_TITLE = 'Retire the Westbrook account workspace'
CONTAIN_TITLE = 'Reconcile the Westbrook payment ledger'


def profile(name):
    profiles = api('GET', '/api/vmcps').get('items') or []
    found = next((p for p in profiles if p.get('displayName') == name), None)
    if found:
        return found['id']
    entries = api('GET', '/api/mcp-catalogs/default/entries').get('items') or []
    entry = next((e for e in entries if (e.get('manifest') or {}).get('name') == name), None)
    if not entry:
        raise RuntimeError(f'Missing {name} fixture catalog entry; leave existing profiles and labels unchanged.')
    created = api('POST', '/api/vmcps', {
        'displayName': name,
        'components': [{'name': 'care' if name == 'Northstar Health' else 'accounts',
                        'mcpCatalogID': 'default', 'mcpServerCatalogEntryID': entry['id']}],
    })
    return created['id']


def policies():
    rules = [
        {
            'name': 'Patient records remain inside the care team',
            'description': 'Deny external patient-record delivery from restricted care-coordination tasks.',
            'condition': 'subject.agent == "northstar-care-assistant" && action.server == "Northstar Health" && session.warrant.present && session.warrant.task in ' + json.dumps(CARE_TITLES) + ' && action.tool.endsWith("send_report")',
        },
        {
            'name': 'Restricted billing contacts cannot be distributed externally',
            'description': 'Deny distribution of a restricted account brief outside its account team.',
            'condition': 'subject.agent == "meridian-account-assistant" && action.server == "Meridian Operations" && session.warrant.present && session.warrant.task == ' + json.dumps(FINANCE_TITLE) + ' && action.tool.endsWith("distribute_brief")',
        },
        {
            'name': 'Retained contract workspaces cannot be erased',
            'description': 'Deny irreversible workspace retirement while account documents are retained for review.',
            'condition': 'subject.agent == "meridian-account-assistant" && action.server == "Meridian Operations" && session.warrant.present && session.warrant.task == ' + json.dumps(WORKSPACE_TITLE) + ' && action.tool.endsWith("retire_workspace")',
        },
    ]
    existing = {p['name']: p for p in api('GET', '/api/provenance/policies').get('items') or []}
    for rule in rules:
        rule.update({'effect': 'deny', 'mode': 'enforce', 'priority': 20000, 'enabled': True})
        if rule['name'] in existing:
            api('PUT', '/api/provenance/policies/' + existing[rule['name']]['id'], rule)
        else:
            api('POST', '/api/provenance/policies', rule)


def run(profile_id, title, read_tool, read_args, denied_tool, denied_args, actor, keep_open=False, contain=False):
    client = MCPClient(f'{OBOT}/mcp-connect/{profile_id}', actor)
    client.initialize()
    tools = client.list_tools()
    def invoke(base, args):
        tool = find_tool(tools, base)
        if not tool:
            raise RuntimeError(f'Missing fixture tool: {base}')
        return client.call_tool(tool, args)
    read_result = invoke(read_tool, read_args)
    if read_result.get('isError'):
        raise RuntimeError('Fixture read failed: ' + tool_result_text(read_result))
    sid = session_for(client)
    api('POST', f'/api/provenance/sessions/{sid}/warrant', {
        'task': title, 'scopes': ['mcp:*:*'], 'ttlSeconds': 3600,
    })
    if contain:
        api('POST', '/api/provenance/kills', {
            'scope': 'session', 'target': sid,
            'reason': 'Payment-ledger reconciliation paused by the account security reviewer.',
            'expiresAt': (datetime.now(timezone.utc) + timedelta(hours=1)).isoformat().replace('+00:00', 'Z'),
        })
    blocked = False
    try:
        result = invoke(denied_tool, denied_args)
        blocked = bool(result.get('isError'))
    except MCPError:
        blocked = True
    if not blocked:
        raise RuntimeError(f'{title}: gateway unexpectedly allowed {denied_tool}')
    events = api('GET', f'/api/provenance/sessions/{sid}/events?ascending=true&limit=200').get('items') or []
    decisions = [e for e in events if e.get('decision')]
    event = decisions[-1]
    decision = event['decision']
    if decision.get('allow') or decision.get('effect') != 'deny' or decision.get('approvalID'):
        raise RuntimeError('Expected an explicit deny without an approval: ' + json.dumps(decision))
    if contain and not decision.get('killed'):
        raise RuntimeError('Expected session containment to deny the ledger action.')
    if not event.get('action', {}).get('tool', '').endswith(denied_tool):
        raise RuntimeError('The final decision does not correspond to the requested operation.')
    if not keep_open:
        api('POST', f'/api/provenance/sessions/{sid}/end')
    call('DELETE', client.url, headers=client._headers())
    print(f'DENIED: {title}')
    print('  ' + decision.get('reason', ''))
    print('  ' + OBOT + '/sessions/' + sid)
    return {'title': title, 'sessionID': sid, 'eventID': event['id'], 'effect': decision['effect'],
            'replay': OBOT + '/sessions/' + sid, 'killed': bool(decision.get('killed'))}


def main():
    backup()
    before = api('GET', '/api/provenance/approvals?status=pending&limit=1')['total']
    policies()
    care, finance = profile('Northstar Health'), profile('Meridian Operations')
    results = []
    for i, title in enumerate(CARE_TITLES):
        results.append(run(care, title, 'lookup_patient', {'patient_id': ['MRN-104821', 'MRN-108734'][i]},
            'send_report', {'destination_url': ['mailto:care.request@outside.example', 'https://intake.outside.example/archive'][i],
                            'body': 'Patient care summary and contact details.'},
            'northstar-care-assistant', keep_open=i == 1))
    results.append(run(finance, FINANCE_TITLE, 'account_context', {'account_id': 'WB-48271'},
        'distribute_brief', {'account_id': 'WB-48271', 'destination_url': 'https://uploads.vendor-review.example/brief',
                             'body': 'Billing contact: emma.chen@westbrook.example. Negotiated renewal pricing is restricted to the account team.'},
        'meridian-account-assistant'))
    results.append(run(finance, WORKSPACE_TITLE, 'account_context', {'account_id': 'WB-48271'},
        'retire_workspace', {'workspace_id': 'westbrook-contracts', 'confirmation': 'westbrook-contracts'},
        'meridian-account-assistant'))
    results.append(run(finance, CONTAIN_TITLE, 'account_context', {'account_id': 'WB-48271'},
        'calculate_variance', {'program': 'print("Ledger reconciliation pending")'},
        'meridian-account-assistant', keep_open=True, contain=True))
    after = api('GET', '/api/provenance/approvals?status=pending&limit=1')['total']
    if after != before:
        raise RuntimeError(f'Pending review count changed unexpectedly ({before} -> {after})')
    LOCAL.mkdir(exist_ok=True)
    (LOCAL / 'last-blocked.json').write_text(json.dumps(results, indent=2))
    print(f'\nCreated {len(results)} sessions with explicit deny decisions; pending reviews unchanged at {after}.')
    print('Activity:', OBOT + '/supervision')


if __name__ == '__main__':
    try:
        main()
    except (RuntimeError, OSError, MCPError) as exc:
        print('Activity seeding stopped:', exc, file=sys.stderr)
        sys.exit(1)
