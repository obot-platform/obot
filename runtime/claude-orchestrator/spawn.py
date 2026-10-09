#!/usr/bin/env python3
"""Provision Claude sessions through Obot without logging credentials."""
import json
import os
from pathlib import Path
import sys
import urllib.error
import urllib.request


def request(body):
    headers = {
        'Content-Type': 'application/json',
        'X-Obot-Orchestrator-Token': Path(os.environ['OBOT_PROVISIONING_TOKEN_FILE']).read_text().strip(),
        'X-Obot-Claude-Environment': os.environ['OBOT_CLAUDE_ENVIRONMENT'],
    }
    url = os.environ['OBOT_URL'] + '/api/claude-runners/spawn'
    req = urllib.request.Request(url, json.dumps(body).encode(), headers, method='POST')
    with urllib.request.urlopen(req, timeout=20) as response:
        data = response.read()
        return json.loads(data) if data else {}


def spawn():
    body = {key: os.environ.get('CLAUDE_RUNNER_' + variable, '') for key, variable in (
        ('order', 'ORDER_ID'), ('session', 'SESSION_ID'), ('account', 'ACCOUNT_ID'), ('email', 'ACCOUNT_EMAIL'), ('pool', 'POOL_ID'))}
    body['workOrder'] = Path(os.environ['CLAUDE_RUNNER_WORK_ORDER_FILE']).read_text().strip()
    try:
        sources = os.environ.get('CLAUDE_RUNNER_REPO_SOURCES')
        if sources is not None:
            body['repositories'] = json.loads(sources) if sources else []
        elif os.environ.get('CLAUDE_RUNNER_PRIMARY_REPO_URL'):
            body['repositories'] = [{'url': os.environ['CLAUDE_RUNNER_PRIMARY_REPO_URL'],
                                     'revision': os.environ.get('CLAUDE_RUNNER_PRIMARY_REPO_REVISION', '')}]
        request(body)
    except urllib.error.HTTPError as error:
        if error.code == 403:
            print('Claude identity or agent is unavailable in Obot. Ask an administrator to check account mappings or Claude Tag configuration, then retry from the environment Activity tab.', file=sys.stderr)
        else:
            print('Obot rejected provisioning (HTTP %s); check environment, account admission, and capacity.' % error.code, file=sys.stderr)
        return 1 if error.code >= 500 or error.code == 429 else 2
    except Exception:
        print('Obot provisioning endpoint unavailable.', file=sys.stderr)
        return 1
    return 0


if __name__ == '__main__':
    try:
        sys.exit(spawn())
    except Exception:
        print('Obot runner integration failed; check the environment configuration and Obot status.', file=sys.stderr)
        sys.exit(1)
