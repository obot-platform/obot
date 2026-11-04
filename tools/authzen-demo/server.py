#!/usr/bin/env python3
"""Local AuthZEN playground. No external Python dependencies; not a production PDP."""
import argparse
import hashlib
import hmac
import json
import math
import os
from pathlib import Path
import ssl
import threading
import time
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from http.cookies import SimpleCookie
from urllib.request import Request, urlopen
from urllib.error import HTTPError

DEFAULT = {"defaultAllow": True, "blockedSubjects": [], "blockedActions": [], "rateLimits": []}


class Policy:
    def __init__(self, path=None):
        self.path = Path(path) if path else None
        self.config = json.loads(self.path.read_text()) if self.path and self.path.exists() else DEFAULT.copy()
        if 'rateLimits' not in self.config:
            limit = self.config.pop('limit', 0)
            window = self.config.pop('windowSeconds', 60)
            self.config['rateLimits'] = [{'subjectType': '*', 'subjectID': '*', 'limit': limit, 'windowSeconds': window}] if limit else []
        self.lock = threading.RLock()
        self.counts = {}
        self.events = []
        self.content_count = 0

    def update(self, config):
        if type(config.get('defaultAllow')) is not bool:
            raise ValueError('Default decision must be a boolean')
        for key in ['blockedSubjects', 'blockedActions']:
            if not isinstance(config.get(key), list) or any(not isinstance(x, str) for x in config[key]):
                raise ValueError('Blocked identities and actions must be lists of strings')
        rows = config.get('rateLimits', self.config['rateLimits'])
        if not isinstance(rows, list) or len(rows) > 100:
            raise ValueError('Expected at most 100 rate-limit rows')
        seen = set()
        for row in rows:
            if not isinstance(row, dict) or row.get('subjectType') not in ['user', 'agent', '*']:
                raise ValueError('Choose user, agent, or all identity types')
            if not isinstance(row.get('subjectID'), str) or not row['subjectID'].strip():
                raise ValueError('An identity ID or * is required')
            key = (row['subjectType'], row['subjectID'])
            if key in seen:
                raise ValueError('Duplicate identity rows are not allowed')
            seen.add(key)
            for field, low, high in [('limit', 0, 10000), ('windowSeconds', 1, 3600)]:
                if type(row.get(field)) is not int or not low <= row[field] <= high:
                    raise ValueError(f'{field} must be between {low} and {high}')
        config = {**config, 'rateLimits': rows}
        with self.lock:
            if self.config['rateLimits'] != rows:
                self.counts.clear()
            self.config = {key: config[key] for key in DEFAULT}
            if self.path:
                self.path.parent.mkdir(parents=True, exist_ok=True)
                temp = self.path.with_suffix('.tmp')
                temp.write_text(json.dumps(self.config, indent=2))
                temp.replace(self.path)

    def evaluate(self, envelope, signed=False, now=None):
        subject = envelope['subject']
        action = envelope['action']['name']
        resource = envelope['resource']
        properties = envelope['action'].get('properties', {})
        tool = properties.get('tool', '') if action == 'tools/call' else ''
        if not isinstance(tool, str):
            raise ValueError('Tool selector must be a string')
        if not all(isinstance(v, str) and v for v in [subject['type'], subject['id'], action, resource['type'], resource['id']]):
            raise ValueError('Expected AuthZEN subject, action and resource')
        now = time.time() if now is None else now
        with self.lock:
            cfg = self.config
            allowed, reason = cfg['defaultAllow'], 'Default policy'
            retry_after = None
            if subject['id'] in cfg['blockedSubjects']:
                allowed, reason = False, 'Identity blocked'
            elif action in cfg['blockedActions'] or (tool and tool in cfg['blockedActions']):
                allowed, reason = False, 'Action blocked'
            if allowed and action == 'tools/call' and tool:
                matches = [row for row in cfg['rateLimits'] if row['subjectType'] in ['*', subject['type']] and row['subjectID'] in ['*', subject['id']]]
                rule = max(matches, key=lambda r: (r['subjectID'] != '*', r['subjectType'] != '*'), default=None)
                if rule and rule['limit']:
                    allowed, reason, retry_after = self.quota(subject, resource, tool, rule, now)
            self.events.insert(0, {'time': now, 'allowed': allowed, 'reason': reason, 'signed': signed, 'envelope': envelope})
            del self.events[100:]
            decision = {'decision': allowed}
            if retry_after is not None:
                decision['context'] = {'obot': {'reason': 'rate_limited', 'retry_after_seconds': retry_after}}
            return decision

    def quota(self, subject, resource, action, rule, now):
        # Separate quota for each verified subject, server, and tool.
        key = (subject['type'], subject['id'], resource['id'], action)
        self.counts = {k: v for k, v in self.counts.items() if now < v[2]}
        start, count, _ = self.counts.get(key, (now, 0, now + rule['windowSeconds']))
        allowed = count < rule['limit']
        if allowed:
            count += 1
            self.counts[key] = (start, count, start + rule['windowSeconds'])
        reason = f"Quota {count}/{rule['limit']} · resets in {max(1, int(start + rule['windowSeconds'] - now))}s"
        retry = None
        if not allowed:
            retry = max(1, math.ceil(start + rule['windowSeconds'] - now))
            reason = f"Rate limit reached: {subject['type']} {subject['id']} has used {count}/{rule['limit']} calls to {action} in {rule['windowSeconds']} seconds. Try again in {retry} seconds."
        return allowed, reason, retry


def verify_signature(body, signature, secret):
    expected = 'sha256=' + hmac.new(secret.encode(), body, hashlib.sha256).hexdigest()
    return hmac.compare_digest(expected, signature)


def rpc(server_id, method, params, cookie=''):
    """Use the real Obot gateway with the browser's local Obot session."""
    url = 'http://127.0.0.1:8080/mcp-connect/' + server_id
    headers = {'Content-Type': 'application/json', 'Accept': 'application/json, text/event-stream'}
    if cookie:
        headers['Cookie'] = cookie

    def send(message):
        data = json.dumps(message).encode()
        try:
            with urlopen(Request(url, data=data, headers=headers), timeout=30) as response:
                session = response.headers.get('Mcp-Session-Id')
                if session:
                    headers['Mcp-Session-Id'] = session
                text = response.read().decode()
                if response.headers.get('Content-Type', '').startswith('text/event-stream'):
                    messages = [json.loads(line[5:]) for line in text.splitlines() if line.startswith('data:')]
                    result = next((m for m in messages if m.get('id') == message.get('id')), messages)
                else:
                    result = json.loads(text) if text else None
                return {'status': response.status, 'body': result}
        except HTTPError as error:
            text = error.read().decode()
            try:
                text = json.loads(text)
            except ValueError:
                pass
            return {'status': error.code, 'body': text}

    init = send({'jsonrpc': '2.0', 'id': 1, 'method': 'initialize', 'params': {'protocolVersion': '2025-03-26', 'capabilities': {}, 'clientInfo': {'name': 'authzen-playground', 'version': '1.0'}}})
    if init['status'] != 200:
        return init
    headers['MCP-Protocol-Version'] = '2025-03-26'
    try:
        send({'jsonrpc': '2.0', 'method': 'notifications/initialized'})
        return send({'jsonrpc': '2.0', 'id': 2, 'method': method, 'params': params})
    finally:
        if 'Mcp-Session-Id' in headers:
            try:
                with urlopen(Request(url, method='DELETE', headers=headers), timeout=5):
                    pass
            except Exception:
                pass  # Session cleanup must not obscure the authorization result.


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--port', type=int, default=8090)
    parser.add_argument('--tls-port', type=int, default=9443)
    parser.add_argument('--cert', required=True)
    parser.add_argument('--key', required=True)
    parser.add_argument('--state', default='/tmp/obot-authzen-demo/policy.json')
    parser.add_argument('--connection', default='/tmp/obot-authzen-demo/connection.json')
    args = parser.parse_args()
    policy = Policy(args.state)
    secret = os.environ.get('AUTHZEN_DEMO_SIGNING_SECRET', '')

    class Handler(BaseHTTPRequestHandler):
        def reply(self, status, value):
            body = json.dumps(value).encode()
            self.send_response(status)
            self.send_header('Content-Type', 'application/json')
            self.send_header('Content-Length', str(len(body)))
            self.send_header('Cache-Control', 'no-store')
            self.end_headers()
            self.wfile.write(body)

        def do_GET(self):
            if self.server.server_port != args.port:
                return self.reply(404, {'error': 'Webhook endpoint only'})
            if self.path == '/':
                body = Path(__file__).with_name('index.html').read_bytes()
                self.send_response(200)
                self.send_header('Content-Type', 'text/html; charset=utf-8')
                self.send_header('Content-Length', str(len(body)))
                self.end_headers()
                self.wfile.write(body)
            elif self.path == '/api/state':
                connection = Path(args.connection)
                with policy.lock:
                    self.reply(200, {'policy': policy.config, 'events': policy.events, 'contentCount': policy.content_count, 'signatureRequired': bool(secret), 'connection': json.loads(connection.read_text()) if connection.exists() else {}})
            else:
                self.reply(404, {'error': 'Not found'})

        def do_POST(self):
            try:
                length = int(self.headers.get('Content-Length', '0'))
                if not 0 < length <= 1048576:
                    return self.reply(413, {'error': 'Expected a JSON body up to 1 MiB'})
                body = self.rfile.read(length)
                if self.path == '/authorize':
                    if self.server.server_port != args.tls_port:
                        return self.reply(400, {'error': 'Use HTTPS for authorization'})
                    signature = self.headers.get('X-Obot-Signature-256', '')
                    valid = bool(secret) and verify_signature(body, signature, secret)
                    if secret and not valid:
                        return self.reply(401, {'error': 'Invalid or missing signature'})
                    return self.reply(200, policy.evaluate(json.loads(body), valid))
                if self.path == '/filter':
                    json.loads(body)
                    with policy.lock:
                        policy.content_count += 1
                    # Existing content contract: HTTP 200 accepts without mutation.
                    return self.reply(200, {})
                if self.server.server_port != args.port:
                    return self.reply(404, {'error': 'Not found'})
                # JSON-only same-origin admin API; no CORS, no cross-origin forms.
                if self.headers.get('Content-Type', '').split(';')[0] != 'application/json':
                    return self.reply(415, {'error': 'JSON required'})
                origin = self.headers.get('Origin')
                if origin and origin != 'http://' + self.headers.get('Host', ''):
                    return self.reply(403, {'error': 'Same-origin requests only'})
                data = json.loads(body)
                if self.path == '/api/policy':
                    policy.update(data)
                    return self.reply(200, {'ok': True})
                if self.path == '/api/rate-limits':
                    with policy.lock:
                        policy.update({**policy.config, 'rateLimits': data['rateLimits']})
                    return self.reply(200, {'ok': True})
                if self.path == '/api/reset':
                    with policy.lock:
                        policy.counts.clear()
                    return self.reply(200, {'ok': True})
                if self.path == '/api/try':
                    cookies = SimpleCookie(self.headers.get('Cookie', ''))
                    session = cookies.get('obot_access_token')
                    if not session:
                        return self.reply(401, {'error': 'Sign in to Obot in this browser first, then retry.'})
                    connection = json.loads(Path(args.connection).read_text())
                    method = data.get('method', 'tools/call')
                    if method not in ['tools/call', 'tools/list', 'resources/list', 'prompts/list']:
                        raise ValueError('Unsupported playground method')
                    return self.reply(200, rpc(connection['serverID'], method, data.get('params', {}), session.OutputString()))
                self.reply(404, {'error': 'Not found'})
            except (ValueError, KeyError, TypeError) as error:
                self.reply(400, {'error': str(error)})
            except Exception as error:
                self.reply(502, {'error': str(error)})

    https = ThreadingHTTPServer(('0.0.0.0', args.tls_port), Handler)
    context = ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER)
    context.load_cert_chain(args.cert, args.key)
    https.socket = context.wrap_socket(https.socket, server_side=True)
    threading.Thread(target=https.serve_forever, daemon=True).start()
    print(f'Playground http://localhost:{args.port}; HTTPS authorization on {args.tls_port}', flush=True)
    ThreadingHTTPServer(('0.0.0.0', args.port), Handler).serve_forever()


if __name__ == '__main__':
    main()
