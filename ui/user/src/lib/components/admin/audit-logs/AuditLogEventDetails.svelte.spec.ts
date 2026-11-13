import type { AuditLogEvent } from '$lib/services';
import AuditLogEventDetails from './AuditLogEventDetails.svelte';
import { describe, expect, it } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

function localAgentEvent(details: Partial<NonNullable<AuditLogEvent['details']>>) {
	return {
		id: 1,
		user: '',
		timestamp: {
			occurredAt: '2026-11-13T21:14:11Z',
			recordedAt: '2026-11-13T21:14:11Z',
			source: 'client_reported'
		},
		eventType: 'local_agent_tool_call',
		actor: { actorType: 'device', id: 'device-1' },
		action: { operation: 'tools/call', name: 'Bash', kind: 'shell' },
		target: { targetType: 'local_tool', name: 'Bash' },
		outcome: { status: 'success' },
		details: {
			request: { body: { command: 'uname -a' }, mutated: false },
			response: { body: 'Linux sbx 6.12 aarch64\n', mutated: false },
			payloadRedacted: false,
			...details
		}
	} as AuditLogEvent & { user: string };
}

// The raw event renders in a CodeMirror editor; read its text once the editor is on the page.
async function rawEventText() {
	const rawEvent = page.getByLabelText('Raw Event JSON');
	await expect.element(rawEvent).toBeInTheDocument();
	return rawEvent.element().textContent ?? '';
}

describe('AuditLogEventDetails raw event', () => {
	it('shows the tool input and output in the raw event instead of the Obot Sentry markers', async () => {
		render(AuditLogEventDetails, {
			auditLog: localAgentEvent({
				rawEvent: {
					hook_event_name: 'PostToolUse',
					tool_input: { obot_sentry_omitted: 'details.request.body' },
					tool_response: { obot_sentry_omitted: 'details.response.body' }
				}
			}),
			onClose: () => {}
		});

		const text = await rawEventText();
		expect(text).toMatch(/"hook_event_name":\s*"PostToolUse"/);
		expect(text).toMatch(/"tool_input":\s*\{\s*"command":\s*"uname -a"\s*\}/);
		expect(text).toMatch(/"tool_response":\s*"Linux sbx 6\.12 aarch64\\n"/);
		expect(text).not.toContain('obot_sentry_omitted');
	});

	it('keeps a marker when the log does not hold the value it names', async () => {
		render(AuditLogEventDetails, {
			auditLog: localAgentEvent({
				response: undefined,
				rawEvent: { tool_response: { obot_sentry_omitted: 'details.response.body' } }
			}),
			onClose: () => {}
		});

		expect(await rawEventText()).toMatch(/"obot_sentry_omitted":\s*"details\.response\.body"/);
	});
});
