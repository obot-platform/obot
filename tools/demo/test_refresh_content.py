import json
import sqlite3
import unittest

from refresh_content import changed, plans, transform


class RefreshContentTest(unittest.TestCase):
    def test_preserves_resource_and_lineage_identity_while_refreshing_display_text(self):
        original = {
            'metadata': {'name': 'default-supervisor-demo-123', 'uid': 'demo-uid'},
            'spec': {'manifest': {'name': 'Supervisor demo'}},
            'id': 'demo-id', 'sessionID': 'ses-demo-123', 'serverID': 'demo-server',
            'policyID': 'pol-demo-123',
            'summary': 'Demo: Read an attachment',
            'lineage': [{'originEventID': 'evt-demo', 'originSummary': 'Supervisor demo.lookup_patient result'}],
            'actor': {'agent': 'demo-agent', 'clientSessionID': 'demo-client'},
            'fingerprints': [{'h': 12345, 'o': 'evt-demo'}],
        }
        refreshed = transform(original)
        self.assertEqual(refreshed['metadata'], original['metadata'])
        for key in ['id', 'sessionID', 'serverID', 'policyID', 'fingerprints']:
            self.assertEqual(refreshed[key], original[key])
        self.assertEqual(refreshed['lineage'][0]['originEventID'], 'evt-demo')
        self.assertEqual(refreshed['spec']['manifest']['name'], 'Northstar Health')
        self.assertEqual(refreshed['summary'], 'Read an attachment')
        self.assertEqual(refreshed['actor']['clientSessionID'], 'care-client')

    def test_keeps_unrelated_demographics_and_exact_canary_values(self):
        self.assertIsNone(changed('Patient demographics'))
        value = 'sk-demo-' + 'a' * 20
        self.assertIsNone(changed(json.dumps({'credential': value})))

    def test_transforms_nested_json_and_unencrypted_blobs(self):
        blob = json.dumps({'request': json.dumps({'body': 'SYNTHETIC DEMO: care note'})}).encode()
        out = json.loads(changed(blob))
        self.assertEqual(json.loads(out['request'])['body'], 'care note')
        self.assertIsNone(changed(b'\xff\xfe\x00'))

    def test_is_idempotent(self):
        once = changed('{"title":"Demo: Review","agent":"demo-agent"}')
        self.assertIsNone(changed(once))

    def test_plan_is_bounded_and_preserves_primary_keys(self):
        db = sqlite3.connect(':memory:')
        db.execute('create table provenance_sessions (id text primary key, title text, actor text)')
        db.execute('insert into provenance_sessions values (?, ?, ?)', (
            'ses-demo-123', 'Demo: Care coordination', '{"agent":"demo-agent"}',
        ))
        db.execute('create table unrelated_credentials (id text primary key, value text)')
        db.execute('insert into unrelated_credentials values (?, ?)', ('credential', 'demo-value'))
        updates = list(plans(db))
        self.assertEqual(len(updates), 1)
        table, _, values = updates[0]
        self.assertEqual(table, 'provenance_sessions')
        self.assertNotIn('id', values)
        self.assertEqual(values['title'], 'Care coordination')
        self.assertEqual(json.loads(values['actor'])['agent'], 'care-agent')


if __name__ == '__main__':
    unittest.main()
