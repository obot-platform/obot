import hashlib
import hmac
import tempfile
import unittest
from pathlib import Path
from server import DEFAULT, Policy, verify_signature


def envelope(subject='alice', action='echo', server='crm', method='tools/call'):
    return {'subject': {'type': 'user', 'id': subject}, 'action': {'name': method, 'properties': {'tool': action} if method == 'tools/call' else {}}, 'resource': {'type': 'mcp-server', 'id': server}, 'context': {'obot': {'mapping': 'obot-mcp-requests-v2'}}}


class PolicyTests(unittest.TestCase):
    def test_default_and_explicit_denies(self):
        p = Policy()
        self.assertTrue(p.evaluate(envelope())['decision'])
        p.update({**DEFAULT, 'blockedSubjects': ['alice']})
        self.assertFalse(p.evaluate(envelope())['decision'])
        self.assertTrue(p.evaluate(envelope('bob'))['decision'])
        p.update({**DEFAULT, 'blockedActions': ['echo']})
        self.assertFalse(p.evaluate(envelope())['decision'])
        p.update({**DEFAULT, 'defaultAllow': False})
        self.assertFalse(p.evaluate(envelope('bob', 'add'))['decision'])

    def test_quota_is_per_identity_server_action_and_expires(self):
        p = Policy()
        p.update({**DEFAULT, 'rateLimits': [{'subjectType': '*', 'subjectID': '*', 'limit': 1, 'windowSeconds': 60}]})
        self.assertTrue(p.evaluate(envelope(), now=100)['decision'])
        self.assertFalse(p.evaluate(envelope(), now=101)['decision'])
        for e in [envelope('bob'), envelope(server='other'), envelope(action='add')]:
            self.assertTrue(p.evaluate(e, now=101)['decision'])
        self.assertTrue(p.evaluate(envelope(), now=160)['decision'])

    def test_discovery_does_not_consume_tool_quota(self):
        p = Policy()
        p.update({**DEFAULT, 'rateLimits': [{'subjectType': '*', 'subjectID': '*', 'limit': 1, 'windowSeconds': 60}]})
        for _ in range(3):
            self.assertTrue(p.evaluate(envelope(action='tools/list', method='tools/list'))['decision'])
        self.assertFalse(p.counts)

    def test_saved_policy_survives_restart(self):
        with tempfile.TemporaryDirectory() as tmp:
            path = Path(tmp) / 'policy.json'
            p = Policy(path)
            p.update({**DEFAULT, 'blockedActions': ['echo']})
            self.assertFalse(Policy(path).evaluate(envelope())['decision'])

    def test_validation_does_not_replace_policy(self):
        p = Policy()
        for change in [{'rateLimits': [{'subjectType':'user','subjectID':'1','limit':-1,'windowSeconds':60}]}, {'rateLimits': 'invalid'}, {'defaultAllow': 'false'}, {'blockedActions': 'echo'}]:
            with self.assertRaises(ValueError):
                p.update({**DEFAULT, **change})
            self.assertEqual(p.config, DEFAULT)

    def test_individual_limits_override_wildcard(self):
        p = Policy()
        p.update({**DEFAULT, 'rateLimits': [
            {'subjectType': '*', 'subjectID': '*', 'limit': 1, 'windowSeconds': 60},
            {'subjectType': 'user', 'subjectID': '2', 'limit': 2, 'windowSeconds': 10},
        ]})
        self.assertTrue(p.evaluate(envelope('1'), now=100)['decision'])
        self.assertFalse(p.evaluate(envelope('1'), now=101)['decision'])
        self.assertTrue(p.evaluate(envelope('2'), now=100)['decision'])
        self.assertTrue(p.evaluate(envelope('2'), now=101)['decision'])
        decision = p.evaluate(envelope('2'), now=102)
        self.assertFalse(decision['decision'])
        self.assertEqual(decision['context']['obot'], {'reason': 'rate_limited', 'retry_after_seconds': 8})
        self.assertIn('Try again in 8 seconds', p.events[0]['reason'])
        self.assertTrue(p.evaluate(envelope('2'), now=110)['decision'])
        self.assertFalse(p.evaluate(envelope('1'), now=110)['decision'])

    def test_signature_uses_exact_body_and_rejects_tampering(self):
        body = b'{"test":1}'
        signature = 'sha256=' + hmac.new(b'secret', body, hashlib.sha256).hexdigest()
        self.assertTrue(verify_signature(body, signature, 'secret'))
        self.assertFalse(verify_signature(body + b' ', signature, 'secret'))
        self.assertFalse(verify_signature(body, '', 'secret'))
        self.assertFalse(verify_signature(body, signature, 'wrong'))


if __name__ == '__main__':
    unittest.main()
