#!/usr/bin/env python3
"""Keep two real care-sharing review requests available alongside tool-label reviews."""
from datetime import datetime, timezone
import json
import sys

from client import MCPError, OBOT
from demo import LOCAL, api, backup, fixture, workflow
from blocked import profile

TASKS = [
    ('Prepare a specialist referral summary', 'MRN-104821'),
    ('Share follow-up details with an outside care coordinator', 'MRN-108734'),
]
POLICY = 'Review external specialist coordination'
DAY_NS = 24 * 60 * 60 * 1_000_000_000


def reviews():
    rows = api('GET', '/api/provenance/tool-label-reviews').get('items') or []
    return sorted((r['resourceType'], r['resourceID'], r['toolCount']) for r in rows)


def main():
    backup()
    before = reviews()
    fixture()
    pid = profile('Northstar Health')
    rule = {
        'name': POLICY,
        'description': 'Require a care-team review before these referral tasks share patient information with an outside recipient.',
        'condition': 'subject.agent == "claude_code" && action.server == "Northstar Health" && session.warrant.present && session.warrant.task in ' + json.dumps([t for t, _ in TASKS]) + ' && action.tool.endsWith("send_report") && action.trifecta',
        'effect': 'require_approval', 'mode': 'enforce', 'enabled': True,
        'priority': 30000, 'approvalTTL': DAY_NS,
    }
    old = next((p for p in api('GET', '/api/provenance/policies')['items'] if p['name'] == POLICY), None)
    saved = api('PUT', '/api/provenance/policies/' + old['id'], rule) if old else api('POST', '/api/provenance/policies', rule)
    pending = api('GET', '/api/provenance/approvals?status=pending&limit=1000').get('items') or []
    results = []
    for title, patient_id in TASKS:
        existing = None
        for approval in pending:
            if approval.get('policyID') != saved['id']:
                continue
            session = api('GET', '/api/provenance/sessions/' + approval['sessionID'])
            if session.get('title') == title and not session.get('endedAt') and not session.get('state', {}).get('killed'):
                existing = approval
                break
        if existing:
            sid, aid = existing['sessionID'], existing['id']
            print('Reusing pending request:', title)
        else:
            sid, aid = workflow(pid, protect=True, title=title, patient_id=patient_id, verbose=False)
            print('Held for care-team review:', title)
        approval = api('GET', '/api/provenance/approvals/' + aid)
        session = api('GET', '/api/provenance/sessions/' + sid)
        assert approval['status'] == 'pending' and not session.get('endedAt')
        created = datetime.fromisoformat(approval['createdAt'].replace('Z', '+00:00'))
        expires = datetime.fromisoformat(approval['expiresAt'].replace('Z', '+00:00'))
        assert (expires - created).total_seconds() == 86400
        event = next(e for e in api('GET', f'/api/provenance/sessions/{sid}/events?ascending=true&limit=200')['items'] if e['id'] == approval['eventID'])
        assert not event['decision']['allow'] and event['decision']['effect'] == 'require_approval' and event.get('lineage')
        results.append({'title': title, 'sessionID': sid, 'approvalID': aid, 'expiresAt': approval['expiresAt']})
        print('  ' + OBOT + '/approvals?approval=' + aid)
        print('  ' + OBOT + '/sessions/' + sid)
        print('  Expires:', approval['expiresAt'])
    after = reviews()
    assert after == before, 'Tool-label review inventory changed while seeding; inspect before proceeding.'
    LOCAL.mkdir(exist_ok=True)
    (LOCAL / 'last-held.json').write_text(json.dumps(results, indent=2))
    print('Two held sessions remain pending and open. Tool-label reviews are unchanged.')
    print('Dashboard:', OBOT + '/supervision')


if __name__ == '__main__':
    try:
        main()
    except (RuntimeError, OSError, MCPError) as exc:
        print('Review preparation stopped:', exc, file=sys.stderr)
        sys.exit(1)
