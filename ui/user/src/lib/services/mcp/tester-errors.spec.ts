import { testerErrorDetails } from './tester-errors';
import { McpError } from '@modelcontextprotocol/sdk/types.js';
import { describe, expect, it } from 'vitest';

describe('testerErrorDetails', () => {
	it('preserves protocol codes, error data, and wrapped causes', () => {
		const cause = new McpError(-32602, 'invalid parameters', { path: ['arguments', 'query'] });
		expect(testerErrorDetails(new Error('snapshot failed', { cause }))).toMatchObject({
			name: 'Error',
			message: 'snapshot failed',
			cause: { code: -32602, data: { path: ['arguments', 'query'] } }
		});
	});

	it('handles cyclic causes while retaining ordinary error text', () => {
		const error = new Error('failed');
		error.cause = error;
		expect(testerErrorDetails(error)).toEqual({
			name: 'Error',
			message: 'failed',
			cause: '[Circular]'
		});
		expect(testerErrorDetails('raw failure')).toBe('raw failure');
	});
});
