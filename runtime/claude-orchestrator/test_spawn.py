import json
import os
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch
import urllib.error

import spawn


class SpawnTest(unittest.TestCase):
    def test_provisioning_request_and_exit_codes(self):
        with tempfile.TemporaryDirectory() as directory:
            token = Path(directory) / 'token'
            order = Path(directory) / 'order'
            token.write_text('secret\n')
            order.write_text('work-order\n')
            with patch.dict(os.environ, {
                'OBOT_URL': 'http://obot',
                'OBOT_PROVISIONING_TOKEN_FILE': str(token),
                'OBOT_CLAUDE_ENVIRONMENT': 'environment',
                'CLAUDE_RUNNER_WORK_ORDER_FILE': str(order),
                'CLAUDE_RUNNER_SESSION_ID': 'session',
                'CLAUDE_RUNNER_ACCOUNT_EMAIL': 'person@example.com',
                'CLAUDE_RUNNER_REPO_SOURCES': '[{"url":"https://github.com/obot-platform/obot","revision":"main"},{"url":"https://github.com/agent-substrate/substrate","revision":"v0.3.0"}]',
            }), patch.object(spawn.urllib.request, 'urlopen') as send:
                send.return_value.__enter__.return_value.read.return_value = b'{}'
                self.assertEqual(spawn.spawn(), 0)
                request = send.call_args.args[0]
                self.assertEqual(request.full_url, 'http://obot/api/claude-runners/spawn')
                self.assertEqual(request.get_header('X-obot-orchestrator-token'), 'secret')
                self.assertEqual(request.get_header('X-obot-claude-environment'), 'environment')
                self.assertEqual(json.loads(request.data)['session'], 'session')
                self.assertEqual(json.loads(request.data)['email'], 'person@example.com')
                self.assertEqual(json.loads(request.data)['workOrder'], 'work-order')
                self.assertEqual(json.loads(request.data)['repositories'], json.loads(os.environ['CLAUDE_RUNNER_REPO_SOURCES']))

                with patch('sys.stderr'):
                    for status, expected in ((403, 2), (429, 1), (500, 1)):
                        send.side_effect = urllib.error.HTTPError(request.full_url, status, '', {}, None)
                        self.assertEqual(spawn.spawn(), expected)
