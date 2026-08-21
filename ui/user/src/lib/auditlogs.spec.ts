import { batchAuditLogAPIKeyIDs, formatAuditLogCredentialLabel } from './auditlogs';
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

describe('batchAuditLogAPIKeyIDs', () => {
	it('deduplicates IDs and omits missing attribution', () => {
		expect(batchAuditLogAPIKeyIDs([42, undefined, 17, 42])).toEqual(['42,17']);
	});

	it('batches IDs at the filter-options endpoint limit', () => {
		expect(batchAuditLogAPIKeyIDs([1, 2, 3, 4, 5], 2)).toEqual(['1,2', '3,4', '5']);
	});
});
