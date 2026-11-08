import { Group } from '$lib/services';
import { createMockProfile, preparePageData } from '../../tests/helpers/pageData';
import { worker } from '../../tests/mocks/worker';
import AIPage from './+page.svelte';
import { http, HttpResponse } from 'msw';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

async function renderPage(groups: string[] = [Group.ADMIN]) {
	localStorage.setItem('seenSplashDialog', new Date().toISOString());
	await preparePageData({ profile: createMockProfile(groups) });
	return render(AIPage);
}

beforeEach(() => sessionStorage.clear());

describe('Obot AI', () => {
	it('offers administrators briefing suggestions and answers with the tools it used', async () => {
		const asked = vi.fn();
		worker.use(
			http.post('/api/ai/chat', async ({ request }) => {
				asked(await request.json());
				return HttpResponse.json({
					reply: '**Spend** is $12 this week.',
					model: 'openai/gpt-5.4',
					steps: [{ tool: 'llm_usage', arguments: { days: 7 } }]
				});
			})
		);
		await renderPage();

		await page.getByRole('button', { name: /Where is our LLM spend going/ }).click();

		await vi.waitFor(() =>
			expect(asked).toHaveBeenCalledWith({
				messages: [
					{ role: 'user', content: 'Where is our LLM spend going, and how could we lower it?' }
				]
			})
		);
		await expect.element(page.getByText('Spend', { exact: true })).toBeInTheDocument();
		await expect.element(page.getByText('Used 1 tools')).toBeInTheDocument();
		await expect.element(page.getByRole('button', { name: 'New chat' })).toBeInTheDocument();
	});

	it('sends the whole conversation on follow-ups', async () => {
		const asked = vi.fn();
		worker.use(
			http.post('/api/ai/chat', async ({ request }) => {
				const body = (await request.json()) as { messages: unknown[] };
				asked(body);
				return HttpResponse.json({ reply: `answer ${body.messages.length}`, steps: [] });
			})
		);
		await renderPage();

		const box = page.getByRole('textbox', { name: 'Ask Obot AI…' });
		await box.fill('first');
		await page.getByRole('button', { name: 'Send' }).click();
		await expect.element(page.getByText('answer 1')).toBeInTheDocument();
		await box.fill('second');
		await page.getByRole('button', { name: 'Send' }).click();
		await expect.element(page.getByText('answer 3')).toBeInTheDocument();
		expect(asked).toHaveBeenLastCalledWith({
			messages: [
				{ role: 'user', content: 'first' },
				{ role: 'assistant', content: 'answer 1' },
				{ role: 'user', content: 'second' }
			]
		});
	});

	it('streams tool calls and the answer as they happen', async () => {
		let release!: () => void;
		const gate = new Promise<void>((r) => (release = r));
		worker.use(
			http.post('/api/ai/chat', () => {
				const encoder = new TextEncoder();
				const send = (c: ReadableStreamDefaultController, event: object) =>
					c.enqueue(encoder.encode(`data: ${JSON.stringify(event)}\n\n`));
				const body = new ReadableStream({
					async start(c) {
						send(c, { type: 'thinking' });
						send(c, { type: 'tool_start', tool: 'llm_usage', arguments: { days: 7 } });
						await gate;
						send(c, { type: 'tool_end', tool: 'llm_usage' });
						send(c, { type: 'thinking' });
						send(c, { type: 'delta', text: 'Spend is ' });
						send(c, { type: 'delta', text: '**$12**' });
						send(c, {
							type: 'done',
							reply: 'Spend is **$12** this week.',
							steps: [{ tool: 'llm_usage', arguments: { days: 7 } }]
						});
						c.close();
					}
				});
				return new HttpResponse(body, { headers: { 'Content-Type': 'text/event-stream' } });
			})
		);
		await renderPage();

		await page.getByRole('textbox', { name: 'Ask Obot AI…' }).fill('spend?');
		await page.getByRole('button', { name: 'Send' }).click();

		// The tool shows while it runs, before any answer text.
		await expect.element(page.getByText('Checking LLM spend')).toBeInTheDocument();
		await expect.element(page.getByText('running')).toBeInTheDocument();
		release();

		await expect.element(page.getByText('this week.', { exact: false })).toBeInTheDocument();
		await expect.element(page.getByText('Used 1 tools')).toBeInTheDocument();
	});

	it('shows basic users suggestions about finding servers', async () => {
		await renderPage([Group.USER]);

		await expect
			.element(page.getByRole('button', { name: /What MCP servers can I use/ }))
			.toBeInTheDocument();
		await expect
			.element(page.getByRole('button', { name: /Where is our LLM spend going/ }))
			.not.toBeInTheDocument();
	});

	it('shows an error without losing the question', async () => {
		worker.use(
			http.post('/api/ai/chat', () =>
				HttpResponse.json({ error: 'no model configured' }, { status: 502 })
			)
		);
		await renderPage();

		await page.getByRole('textbox', { name: 'Ask Obot AI…' }).fill('hello');
		await page.getByRole('button', { name: 'Send' }).click();

		await expect.element(page.getByText(/Obot AI couldn't answer/)).toBeInTheDocument();
		await expect.element(page.getByText('hello', { exact: true })).toBeInTheDocument();
	});
});
