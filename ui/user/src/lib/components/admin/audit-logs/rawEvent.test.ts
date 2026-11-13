import type { AuditLogDetails } from '$lib/services';
import { restoreRawEvent } from './rawEvent';
import { describe, expect, it } from 'vitest';

function details(request: unknown, response: unknown): AuditLogDetails {
	return {
		request: request === undefined ? undefined : { body: request, mutated: false },
		response: response === undefined ? undefined : { body: response, mutated: false },
		payloadRedacted: false
	};
}

// Hook payloads from Obot Sentry's own fixtures, one per parser shape it marks. outputKey is the
// field Sentry reads the output from; jsonString says the entry carries the JSON inside it.
const agentPayloads: {
	agent: string;
	payload: Record<string, unknown>;
	outputKey?: string;
	jsonString?: boolean;
}[] = [
	{
		agent: 'Claude Code',
		payload: {
			session_id: 'session-1',
			hook_event_name: 'PostToolUse',
			tool_name: 'mcp__everything__echo',
			tool_input: { message: 'ping' },
			tool_response: [{ type: 'text', text: 'Echo: ping' }],
			tool_use_id: 'toolu_1',
			mcp_server: { name: 'everything', source: 'dynamic' }
		},
		outputKey: 'tool_response'
	},
	{
		agent: 'Claude Code failure, which has no output to omit',
		payload: {
			hook_event_name: 'PostToolUseFailure',
			tool_name: 'Bash',
			tool_input: { command: 'cat /nonexistent/file' },
			error: 'Exit code 1'
		}
	},
	{
		agent: 'Codex',
		payload: {
			session_id: 'session-1',
			hook_event_name: 'PostToolUse',
			tool_name: 'Bash',
			tool_input: { command: 'echo hi' },
			tool_response: 'hi\n',
			tool_use_id: 'exec-1'
		},
		outputKey: 'tool_response'
	},
	{
		agent: 'Cursor',
		payload: {
			conversation_id: 'conversation-1',
			tool_name: 'Shell',
			tool_input: { command: 'echo hi', cwd: '', timeout: 30000 },
			tool_output: '{"output":"hi\\n","exitCode":0}',
			duration: 554.376,
			hook_event_name: 'postToolUse'
		},
		outputKey: 'tool_output',
		jsonString: true
	},
	{
		agent: 'VS Code and Copilot CLI',
		payload: {
			hook_event_name: 'PostToolUse',
			session_id: 'session-vs',
			tool_use_id: 'tool-vs',
			tool_name: 'Bash',
			tool_input: { command: 'echo hi' },
			tool_result: { result_type: 'success', text_result_for_llm: 'hi' }
		},
		outputKey: 'tool_result'
	}
];

describe('restoreRawEvent', () => {
	it.each(agentPayloads)(
		'rebuilds the hook payload $agent sent',
		({ payload, outputKey, jsonString }) => {
			const raw: Record<string, unknown> = { ...payload };
			raw.tool_input = { obot_sentry_omitted: 'details.request.body' };
			let response: unknown = null;
			if (outputKey) {
				const output = payload[outputKey];
				response = jsonString ? JSON.parse(output as string) : output;
				raw[outputKey] = jsonString
					? { obot_sentry_omitted: 'details.response.body', encoding: 'json_string' }
					: { obot_sentry_omitted: 'details.response.body' };
			}

			const restored = restoreRawEvent(raw, details(payload.tool_input, response));

			expect(JSON.stringify(restored)).toBe(JSON.stringify(payload));
		}
	);

	it('puts the request and response bodies back in place of the markers, keeping key order', () => {
		const raw = {
			session_id: 's1',
			tool_name: 'Bash',
			tool_input: { obot_sentry_omitted: 'details.request.body' },
			tool_response: { obot_sentry_omitted: 'details.response.body' },
			tool_use_id: 'call_1'
		};

		const restored = restoreRawEvent(raw, details({ command: 'uname -a' }, 'Linux\n'));

		expect(restored).toEqual({
			session_id: 's1',
			tool_name: 'Bash',
			tool_input: { command: 'uname -a' },
			tool_response: 'Linux\n',
			tool_use_id: 'call_1'
		});
		expect(Object.keys(restored as object)).toEqual(Object.keys(raw));
	});

	it('restores a value sent as a JSON-encoded string as that string', () => {
		const raw = {
			tool_output: { obot_sentry_omitted: 'details.response.body', encoding: 'json_string' }
		};

		const restored = restoreRawEvent(raw, details(undefined, { content: [{ text: 'hi' }] }));

		expect(restored).toEqual({ tool_output: '{"content":[{"text":"hi"}]}' });
	});

	it('restores null and empty bodies', () => {
		const raw = {
			tool_input: { obot_sentry_omitted: 'details.request.body' },
			tool_response: { obot_sentry_omitted: 'details.response.body' }
		};

		expect(restoreRawEvent(raw, details(null, {}))).toEqual({
			tool_input: null,
			tool_response: {}
		});
	});

	it('leaves a marker whose value is not in the log', () => {
		const raw = { tool_response: { obot_sentry_omitted: 'details.response.body' } };

		expect(restoreRawEvent(raw, details({ command: 'x' }, undefined))).toBe(raw);
	});

	it('leaves objects that are not exactly a marker', () => {
		const raw = {
			a: { obot_sentry_omitted: 'details.request.body', other: true },
			b: { obot_sentry_omitted: 'details.response.body', encoding: 'base64' },
			c: { obot_sentry_omitted: 'details.somewhere.else' },
			d: [{ obot_sentry_omitted: 'details.request.body' }]
		};

		expect(restoreRawEvent(raw, details('req', 'resp'))).toBe(raw);
	});

	it('returns a raw event that is not an object unchanged', () => {
		expect(restoreRawEvent('text', details('req', 'resp'))).toBe('text');
		expect(restoreRawEvent(undefined, details('req', 'resp'))).toBeUndefined();
	});
});
