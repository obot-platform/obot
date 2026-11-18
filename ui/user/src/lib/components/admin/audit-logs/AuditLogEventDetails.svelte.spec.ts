import type { AuditLogDetails, AuditLogEvent } from '$lib/services';
import AuditLogEventDetails from './AuditLogEventDetails.svelte';
import { describe, expect, it } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

function auditLog(details: Partial<AuditLogDetails>): AuditLogEvent & { user: string } {
	return {
		id: 7,
		user: 'user-1',
		timestamp: {
			occurredAt: '2026-11-12T18:54:06Z',
			recordedAt: '2026-11-12T18:54:06Z',
			source: 'server'
		},
		eventType: 'mcp_call',
		actor: { actorType: 'user', id: 'user-1' },
		action: { operation: 'tools/list' },
		target: { targetType: 'mcp_server', name: 'GitHub' },
		outcome: { status: 'success', httpStatus: 200 },
		details: {
			payloadRedacted: false,
			request: { body: { jsonrpc: '2.0', id: 2, method: 'tools/list' }, mutated: false },
			...details
		}
	};
}

describe('AuditLogEventDetails unavailable payloads', () => {
	it('names the payloads that could not be loaded', async () => {
		render(AuditLogEventDetails, {
			auditLog: auditLog({ unavailablePayloads: ['response.body', 'rawEvent'] }),
			onClose: () => {}
		});

		await expect
			.element(
				page.getByText('Some payloads for this event could not be loaded: response.body, rawEvent.')
			)
			.toBeVisible();
		// The bodies that were read are still shown.
		await expect.element(page.getByText('Request / Tool Input')).toBeVisible();
	});

	it('shows no notice when every payload was loaded', async () => {
		render(AuditLogEventDetails, {
			auditLog: auditLog({
				response: { body: { jsonrpc: '2.0', id: 2, result: { tools: [] } }, mutated: false }
			}),
			onClose: () => {}
		});

		await expect.element(page.getByText('Response / Tool Output')).toBeVisible();
		await expect.element(page.getByText(/could not be loaded/)).not.toBeInTheDocument();
	});
});
