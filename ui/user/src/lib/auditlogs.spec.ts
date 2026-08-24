import { formatAuditLogCredentialLabel } from './auditlogs';
import { describe, expect, it } from 'vitest';

describe('formatAuditLogCredentialLabel', () => {
	it('leaves active API-key credentials unchanged', () => {
		expect(formatAuditLogCredentialLabel('Claude Code (ok1-7-42-*****)', false)).toBe(
			'Claude Code (ok1-7-42-*****)'
		);
	});

	it('identifies revoked API-key credentials', () => {
		expect(formatAuditLogCredentialLabel('Claude Code (ok1-7-42-*****)', true)).toBe(
			'Claude Code (ok1-7-42-*****) · Revoked'
		);
	});
});
