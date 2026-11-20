import { worker } from '../../../tests/mocks/worker';
import ProviderTestDialog from './ProviderTestDialog.svelte';
import { http, HttpResponse } from 'msw';
import { describe, expect, it } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

function apiKey(id: number, name: string, overrides: Record<string, unknown> = {}) {
	return {
		id,
		userId: 1,
		name,
		canAccessAPI: false,
		canAccessLLMProxy: true,
		canAccessSkills: false,
		canAccessDeviceScans: false,
		createdAt: '2026-01-01T00:00:00.000Z',
		...overrides
	};
}

const DEFAULT_KEY = apiKey(2, 'default key');

const models = [
	{ id: 'm1gpt', name: 'gpt', targetModel: 'openai/gpt-5-mini', usage: 'llm', active: true },
	{ id: 'm1old', name: 'old', targetModel: 'openai/gpt-old', usage: 'llm', active: false },
	{ id: 'm1emb', name: 'emb', targetModel: 'openai/embed', usage: 'text-embedding', active: true },
	{ id: 'm1claude', name: 'claude', targetModel: 'anthropic/claude', usage: 'llm', active: true }
];

function sse(...events: unknown[]) {
	return new HttpResponse(events.map((event) => `data: ${JSON.stringify(event)}\n\n`).join(''), {
		headers: { 'Content-Type': 'text/event-stream' }
	});
}

function mockModels(apiKeys: unknown[] = [DEFAULT_KEY]) {
	worker.use(
		http.get('/api/models', () => HttpResponse.json({ items: models })),
		http.get('/api/api-keys', () => HttpResponse.json({ items: apiKeys }))
	);
}

async function openDialog(provider = 'openai') {
	const result = await render(ProviderTestDialog);
	await result.component.open(provider);
	return result;
}

function keyPicker() {
	return page.getByRole('combobox', { name: 'LLM Gateway API Key' });
}

describe('ProviderTestDialog', () => {
	it('offers only active LLM models of the provider', async () => {
		mockModels();
		await openDialog();

		const picker = page.getByRole('combobox', { name: 'Model' });
		await expect.element(picker).toHaveTextContent('openai/gpt-5-mini');
		await picker.click();
		await expect
			.element(page.getByRole('button', { name: 'openai/gpt-5-mini', exact: true }))
			.toBeVisible();
		await expect.element(page.getByText('openai/gpt-old')).not.toBeInTheDocument();
		await expect.element(page.getByText('openai/embed')).not.toBeInTheDocument();
		await expect.element(page.getByText('anthropic/claude')).not.toBeInTheDocument();
	});

	it('requires an API key before sending', async () => {
		mockModels([DEFAULT_KEY, apiKey(3, 'second key')]);
		await openDialog();

		await expect.element(page.getByRole('button', { name: 'Send' })).toBeDisabled();
		await keyPicker().click();
		await page.getByRole('button', { name: 'second key (ok1-1-3-*****)', exact: true }).click();
		await expect.element(keyPicker()).toHaveTextContent('second key (ok1-1-3-*****)');
		await expect.element(page.getByRole('button', { name: 'Send' })).toBeEnabled();
	});

	it('offers only own keys that can use the LLM proxy now', async () => {
		mockModels([
			DEFAULT_KEY,
			apiKey(3, 'api only', { canAccessLLMProxy: false }),
			apiKey(4, 'expired', { expiresAt: '2020-01-01T00:00:00.000Z' }),
			apiKey(5, 'future', { expiresAt: '2999-01-01T00:00:00.000Z' })
		]);
		await openDialog();

		await keyPicker().click();
		await expect
			.element(page.getByRole('button', { name: 'default key (ok1-1-2-*****)', exact: true }))
			.toBeVisible();
		await expect
			.element(page.getByRole('button', { name: 'future (ok1-1-5-*****)', exact: true }))
			.toBeVisible();
		await expect.element(page.getByText('api only')).not.toBeInTheDocument();
		await expect.element(page.getByText('expired')).not.toBeInTheDocument();
	});

	it('selects the only usable key automatically', async () => {
		mockModels();
		await openDialog();

		await expect.element(keyPicker()).toHaveTextContent('default key (ok1-1-2-*****)');
		await expect.element(page.getByRole('button', { name: 'Send' })).toBeEnabled();
	});

	it('explains when there is no usable key', async () => {
		mockModels([]);
		await openDialog();

		await expect
			.element(
				page.getByText('In order to test the given provider, you will need to provide an API key.')
			)
			.toBeVisible();
		await expect.element(page.getByRole('button', { name: 'Send' })).toBeDisabled();
	});

	it('sends the test message and shows the streamed reply with its duration', async () => {
		mockModels();
		let body: Record<string, unknown> | undefined;
		worker.use(
			http.post('/api/llm-gateway/admin/providers/openai/tester/chat', async ({ request }) => {
				body = (await request.json()) as Record<string, unknown>;
				return sse(
					{ type: 'text_delta', delta: 'Hel' },
					{ type: 'text_delta', delta: 'lo!' },
					{
						type: 'reply',
						reply: 'Hello!',
						provider: 'openai',
						servedModel: 'gpt-5-mini',
						durationMs: 321
					}
				);
			})
		);
		await openDialog();
		await expect
			.element(page.getByText('Select which API key to use or generate a new one.'))
			.toBeVisible();
		await page.getByRole('button', { name: 'Send' }).click();

		await expect.element(page.getByText('Hello!')).toBeInTheDocument();
		await expect.element(page.getByText('321 ms')).toBeInTheDocument();
		await expect.element(page.getByText('Served by openai/gpt-5-mini')).toBeInTheDocument();
		expect(body).toEqual({ model: 'm1gpt', apiKeyId: 2, message: 'say hello' });
	});

	it('shows duration in milliseconds, seconds, or minutes', async () => {
		mockModels();
		const durations = [30_000, 300_000];
		worker.use(
			http.post('/api/llm-gateway/admin/providers/openai/tester/chat', () =>
				sse({ type: 'reply', reply: 'Done', durationMs: durations.shift() })
			)
		);
		await openDialog();
		await page.getByRole('button', { name: 'Send' }).click();

		await expect.element(page.getByText('30s')).toBeInTheDocument();

		await page.getByRole('button', { name: 'Send' }).click();
		await expect.element(page.getByText('5m')).toBeInTheDocument();
		await expect.element(page.getByText('30s')).not.toBeInTheDocument();
	});

	it('shows the latest run and keeps earlier runs on tabs', async () => {
		mockModels();
		worker.use(
			http.post('/api/llm-gateway/admin/providers/openai/tester/chat', async ({ request }) => {
				const body = (await request.json()) as { message: string };
				return sse({
					type: 'reply',
					reply: body.message === 'say hello' ? 'Hello!' : 'Goodbye!',
					durationMs: 10
				});
			})
		);
		await openDialog();
		await page.getByRole('button', { name: 'Send' }).click();

		await expect.element(page.getByText('Hello!')).toBeInTheDocument();
		await expect.element(page.getByRole('tab', { name: 'Run 1' })).not.toBeInTheDocument();

		await page.getByRole('textbox', { name: 'Message' }).fill('second try');
		await page.getByRole('button', { name: 'Send' }).click();

		await expect.element(page.getByText('Goodbye!')).toBeInTheDocument();
		await expect.element(page.getByText('Hello!')).not.toBeInTheDocument();
		await expect
			.element(page.getByRole('tab', { name: 'Run 2' }))
			.toHaveAttribute('aria-selected', 'true');

		await page.getByRole('tab', { name: 'Run 1' }).click();
		await expect.element(page.getByText('Hello!')).toBeInTheDocument();
		await expect.element(page.getByText('Goodbye!')).not.toBeInTheDocument();

		await page.getByRole('button', { name: 'Send' }).click();
		await expect
			.element(page.getByRole('tab', { name: 'Run 3' }))
			.toHaveAttribute('aria-selected', 'true');
		await expect.element(page.getByText('Hello!')).not.toBeInTheDocument();
	});

	it('shows the upstream error as returned', async () => {
		mockModels();
		const upstream = '{"error":{"message":"Incorrect API key provided"}}';
		worker.use(
			http.post('/api/llm-gateway/admin/providers/openai/tester/chat', () =>
				sse({ type: 'error', status: 401, error: upstream, durationMs: 12 })
			)
		);
		await openDialog();
		await page.getByRole('button', { name: 'Send' }).click();

		await expect.element(page.getByText('Request failed with status 401')).toBeInTheDocument();
		await expect.element(page.getByText(upstream)).toBeInTheDocument();
		await expect.element(page.getByText('12 ms')).toBeInTheDocument();
	});

	it('shows a rejected request', async () => {
		mockModels();
		worker.use(
			http.post(
				'/api/llm-gateway/admin/providers/openai/tester/chat',
				() => new HttpResponse('API key "default key" is revoked', { status: 400 })
			)
		);
		await openDialog();
		await page.getByRole('button', { name: 'Send' }).click();

		await expect.element(page.getByText('API key "default key" is revoked')).toBeInTheDocument();
	});

	it('generates a short-lived LLM gateway key, reveals it once, and selects it', async () => {
		mockModels([]);
		let created: Record<string, unknown> | undefined;
		let body: Record<string, unknown> | undefined;
		worker.use(
			http.post('/api/api-keys', async ({ request }) => {
				created = (await request.json()) as Record<string, unknown>;
				return HttpResponse.json({
					...apiKey(5, String(created.name), { expiresAt: created.expiresAt }),
					key: 'ok1-1-5-generated'
				});
			}),
			http.post('/api/llm-gateway/admin/providers/openai/tester/chat', async ({ request }) => {
				body = (await request.json()) as Record<string, unknown>;
				return sse({ type: 'reply', reply: 'Hi', durationMs: 1 });
			})
		);
		await openDialog();
		await page.getByRole('button', { name: 'Generate Key' }).click();

		await expect
			.element(keyPicker())
			.toHaveTextContent('LLM Gateway Provider Test Key (ok1-1-5-*****)');
		await expect.element(page.getByText('Save this key now')).toBeVisible();
		await expect
			.element(page.getByRole('textbox', { name: 'Your API Key Copy' }))
			.toHaveValue('ok1-1-5-generated');
		await page.getByRole('button', { name: 'Send' }).click();
		await expect.poll(() => body).toEqual({ model: 'm1gpt', apiKeyId: 5, message: 'say hello' });
		expect(created).toMatchObject({ canAccessLLMProxy: true, mcpServerIds: [] });
		const expiresAt = Date.parse(String(created?.expiresAt));
		expect(expiresAt).toBeGreaterThan(Date.now());
		expect(expiresAt).toBeLessThanOrEqual(Date.now() + 24 * 60 * 60 * 1000);
	});

	it('offers key management when a usable LLM key already exists', async () => {
		mockModels([apiKey(1, 'existing')]);
		let created: Record<string, unknown> | undefined;
		worker.use(
			http.post('/api/api-keys', async ({ request }) => {
				created = (await request.json()) as Record<string, unknown>;
				return HttpResponse.json({
					...apiKey(6, 'new test key'),
					key: 'ok1-1-6-generated'
				});
			})
		);
		await openDialog();

		await expect.element(page.getByRole('link', { name: 'Manage API Keys' })).toBeVisible();
		await expect
			.element(page.getByRole('button', { name: 'Generate Key', exact: true }))
			.not.toBeInTheDocument();

		await page.getByRole('button', { name: 'Row actions' }).click();
		await page.getByRole('button', { name: 'Generate New Key' }).click();

		await expect.element(keyPicker()).toHaveTextContent('new test key (ok1-1-6-*****)');
		await expect.element(page.getByText('Save this key now')).toBeVisible();
		expect(created).toMatchObject({ canAccessLLMProxy: true, mcpServerIds: [] });
	});

	it('drops a key generated after the dialog was reset', async () => {
		mockModels([]);
		let release: () => void = () => {};
		const released = new Promise<void>((resolve) => (release = resolve));
		let requested = false;
		let responded = false;
		worker.use(
			http.post('/api/api-keys', async () => {
				requested = true;
				await released;
				responded = true;
				return HttpResponse.json({ ...apiKey(7, 'late'), key: 'ok1-1-7-late' });
			})
		);
		const { component } = await openDialog();
		await page.getByRole('button', { name: 'Generate Key' }).click();
		await expect.poll(() => requested).toBe(true);

		await component.open('openai');
		release();

		await expect.poll(() => responded).toBe(true);
		await expect
			.element(
				page.getByText('In order to test the given provider, you will need to provide an API key.')
			)
			.toBeVisible();
		await expect.element(page.getByText('late')).not.toBeInTheDocument();
		await expect.element(page.getByText('Save this key now')).not.toBeInTheDocument();
	});

	it('hides the revealed key when the dialog is reopened', async () => {
		mockModels([]);
		worker.use(
			http.post('/api/api-keys', () =>
				HttpResponse.json({ ...apiKey(8, 'once'), key: 'ok1-1-8-once' })
			)
		);
		const { component } = await openDialog();
		await page.getByRole('button', { name: 'Generate Key' }).click();
		await expect.element(page.getByText('Save this key now')).toBeVisible();

		await component.open('openai');

		await expect.element(page.getByText('Save this key now')).not.toBeInTheDocument();
	});

	it('blocks messages longer than the server byte limit', async () => {
		mockModels();
		await openDialog();

		// 1366 three-byte characters fit the character limit but exceed 4096 bytes.
		await page.getByRole('textbox', { name: 'Message' }).fill('あ'.repeat(1366));
		await expect.element(page.getByRole('button', { name: 'Send' })).toBeDisabled();
		await expect
			.element(page.getByText('Message is too long. Please reduce size of message and try again.'))
			.toBeVisible();

		await page.getByRole('textbox', { name: 'Message' }).fill('あ'.repeat(1365));
		await expect.element(page.getByRole('button', { name: 'Send' })).toBeEnabled();
		await expect.element(page.getByText('Message is too long.')).not.toBeInTheDocument();
	});
});
