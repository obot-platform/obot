#!/usr/bin/env python3
"""Inspect/refresh local POC display text offline, preserving stable IDs and evidence hashes.

Dry run by default. --apply refuses while either Go-server port is listening and
backs up the database before its transaction. Does not print payloads or credentials.
"""
import argparse
from datetime import datetime
import json
from pathlib import Path
import re
import sqlite3
import subprocess

ROOT = Path(__file__).resolve().parents[2]
TABLES = (
    'provenance_sessions', 'provenance_events', 'provenance_policies',
    'provenance_approvals', 'provenance_kills', 'provenance_honeytokens',
    'provenance_tool_labels', 'provenance_session_links',
    'mcp_audit_logs', 'llm_audit_logs', 'egress_audit_logs',
    'mcpservercatalogentry', 'mcpserver', 'vmcp', 'mcpclientsession',
)
# These identify existing objects or exact evidence. Updating prose must not
# silently retarget resources or change the meaning of historical lineage.
IDENTITY_KEYS = {
    'id', 'uid', 'sessionid', 'eventid', 'parentid', 'policyid',
    'serverid', 'mcpserverid', 'mcpservercatalogentryid', 'mcpcatalogid',
    'parentwarrantid', 'hash', 'fingerprints', 'h', 'origineventid',
}
IDENTITY_COLUMNS = {
    'id', 'uid', 'name', 'namespace', 'previous_id', 'session_id', 'user_id',
    'event_id', 'parent_id', 'policy_id', 'api_key', 'api_key_id', 'provider_key',
    'mcp_id', 'mcp_server_catalog_entry_name', 'device_id',
}
EXACT_VALUE = re.compile(r'\b(?:sk|pk|rk|ghp|gho|ghu|ghs|xox[bap]|sk-ant|hf)[-_][A-Za-z0-9_-]{12,}\b|\b(?:AKIA|ASIA)[A-Z0-9]{16}\b')
WORD = re.compile(r'(?i)\bdemo\b|demo_|\bsynthetic\b|\bfictional\b')


def text(value):
    # Preserve exact synthetic credential identifiers, which may be the basis
    # of stored hashes/lineage. The output contains only fixture values.
    protected = []
    def protect(match):
        protected.append(match.group(0))
        return f'\x01{len(protected)-1}\x02'
    value = EXACT_VALUE.sub(protect, value)
    for old, new in (
        ('Supervisor demo', 'Northstar Health'),
        ('Demo: review external patient sharing', 'Review external patient sharing'),
        ('Synthetic POC only: require a human review when a care demo combines untrusted input, private patient data and an external send.', 'Require human review when untrusted input and private patient records precede an external send.'),
        ('Synthetic care-coordination tools for a repeatable Supervisor POC', 'Patient records and care-coordination workflows'),
        ('All names and records are fictional.', ''),
        ('Demo only, no actual writes.', ''),
        ('Demo only, never performs network delivery.', ''),
        ('Acme CRM (demo)', 'Acme CRM'),
        ('Demo: ', ''), ('demo: ', ''),
        ('SYNTHETIC DEMO: ', ''), ('SYNTHETIC DEMO ', ''),
        ('SYNTHETIC PATIENT:', 'Patient:'),
        ('for a fictional patient', 'for a patient'),
        ('a fictional diabetes', 'a diabetes'),
        ('fictional patient', 'patient'),
        ('simulated internal handoff', 'queued'),
        ('simulated external send', 'queued'),
    ):
        value = value.replace(old, new)
    value = re.sub(r'(?i)\bsynthetic\s+|\bfictional\s+', '', value)
    value = re.sub(r'(?i)demo_', 'care_', value)
    value = re.sub(r'(?i)\bdemo\b', 'care', value)
    for i, original in enumerate(protected):
        value = value.replace(f'\x01{i}\x02', original)
    return value


def transform(value, path=()):
    if isinstance(value, dict):
        out = {}
        for key, item in value.items():
            normalized = key.lower().replace('_', '')
            if normalized in IDENTITY_KEYS or (path and path[-1] == 'metadata' and key == 'name'):
                out[key] = item
            else:
                out[key] = transform(item, path + (key,))
        return out
    if isinstance(value, list):
        return [transform(item, path) for item in value]
    if isinstance(value, str):
        # Audit envelopes sometimes hold another serialized JSON payload.
        if value.startswith(('{', '[')):
            try:
                decoded = json.loads(value)
                transformed = transform(decoded, path)
                if transformed != decoded:
                    return json.dumps(transformed, ensure_ascii=False, separators=(',', ':'))
                return value
            except (json.JSONDecodeError, TypeError):
                pass
        return text(value)
    return value


def changed(value):
    binary = isinstance(value, bytes)
    if binary:
        try:
            value = value.decode('utf-8')
        except UnicodeDecodeError:
            return None
    if not isinstance(value, str) or not WORD.search(value):
        return None
    try:
        decoded = json.loads(value)
        out = transform(decoded)
        new = json.dumps(out, ensure_ascii=False, separators=(',', ':')) if out != decoded else value
    except (json.JSONDecodeError, TypeError):
        new = text(value)
    if new == value:
        return None
    return new.encode('utf-8') if binary else new


def plans(db):
    existing = {r[0] for r in db.execute("select name from sqlite_master where type='table'")}
    for table in TABLES:
        if table not in existing:
            continue
        info = db.execute(f'pragma table_info("{table}")').fetchall()
        columns = [r[1] for r in info if not r[5] and r[2].upper() in ('TEXT', 'JSON', 'BLOB', 'VARCHAR(255)')]
        columns = [c for c in columns if c not in IDENTITY_COLUMNS or (table.startswith('provenance_') and c == 'name')]
        if table == 'provenance_session_links':
            # Link keys are client-scoped join keys, not session object IDs.
            columns.append('key')
        if not columns:
            continue
        projection = ','.join('"' + c + '"' for c in columns)
        for row in db.execute(f'select rowid,{projection} from "{table}"'):
            changes = {}
            for col, val in zip(columns, row[1:]):
                new = changed(val)
                if new is not None:
                    changes[col] = new
            if changes:
                yield table, row[0], changes


def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument('--apply', action='store_true')
    args = p.parse_args()
    path = ROOT / 'obot.db'
    if args.apply:
        for port in (8090, 8091):
            result = subprocess.run(['lsof', '-nP', f'-iTCP:{port}', '-sTCP:LISTEN'], capture_output=True)
            if result.returncode == 0:
                p.error(f'stop the Go server before applying changes: port {port} is listening')
    db = sqlite3.connect(str(path) if args.apply else f'file:{path}?mode=ro', uri=not args.apply)
    updates = list(plans(db))
    counts = {}
    for table, _, _ in updates:
        counts[table] = counts.get(table, 0) + 1
    print(json.dumps({'mode': 'apply' if args.apply else 'inspect', 'affectedRows': counts}, indent=2))
    if not args.apply or not updates:
        return
    folder = ROOT / 'tools/demo/.local'
    folder.mkdir(exist_ok=True)
    backup = folder / ('obot-before-content-refresh-' + datetime.now().strftime('%Y%m%d-%H%M%S') + '.db')
    with sqlite3.connect(backup) as dest:
        db.backup(dest)
    with db:
        for table, rowid, changes in updates:
            assignments = ','.join('"' + c + '"=?' for c in changes)
            db.execute(f'update "{table}" set {assignments} where rowid=?', [*changes.values(), rowid])
    print('Committed display-text refresh. Stable IDs, fingerprint hashes and rows preserved.')
    print('Backup:', backup.relative_to(ROOT))


if __name__ == '__main__':
    main()
