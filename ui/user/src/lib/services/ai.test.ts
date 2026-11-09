import { splitAnswer } from './ai';
import { describe, expect, it } from 'vitest';

describe('splitAnswer', () => {
	it('splits markdown and widgets in order', () => {
		const content =
			'Here it is:\n```obot-widget\n{"type":"session","sessionId":"s1"}\n```\nAnd the queue:\n```obot-widget\n{"type":"approvals"}\n```';
		expect(splitAnswer(content)).toEqual([
			{ kind: 'markdown', text: 'Here it is:\n' },
			{ kind: 'widget', widget: { type: 'session', sessionId: 's1' } },
			{ kind: 'markdown', text: '\nAnd the queue:\n' },
			{ kind: 'widget', widget: { type: 'approvals' } }
		]);
	});

	it('treats an unterminated widget as still being written', () => {
		expect(splitAnswer('Look:\n```obot-widget\n{"type":"ses')).toEqual([
			{ kind: 'markdown', text: 'Look:\n' },
			{ kind: 'pending' }
		]);
	});

	it('marks unknown or malformed widgets invalid and leaves other code blocks alone', () => {
		expect(splitAnswer('```obot-widget\n{"type":"chart"}\n```')[0].kind).toBe('invalid');
		expect(splitAnswer('```obot-widget\nnot json\n```')[0].kind).toBe('invalid');
		expect(splitAnswer('```go\nfmt.Println()\n```')).toEqual([
			{ kind: 'markdown', text: '```go\nfmt.Println()\n```' }
		]);
	});

	it('caps a sessions list and drops empty IDs', () => {
		const ids = Array.from({ length: 30 }, (_, i) => `s${i}`);
		const [seg] = splitAnswer(
			'```obot-widget\n' + JSON.stringify({ type: 'sessions', sessionIds: ['', ...ids] }) + '\n```'
		);
		expect(seg).toEqual({
			kind: 'widget',
			widget: { type: 'sessions', sessionIds: ids.slice(0, 20) }
		});
	});
});
