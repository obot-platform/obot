import { Group } from '$lib/services';
import { goto } from '$lib/url';
import { worker } from '../../tests/mocks/worker';
import { readyStatus, renderModelsTab, resetModelsTab } from './testing';
import { http, HttpResponse } from 'msw';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { page } from 'vitest/browser';

vi.mock(import('$lib/url'), async (importOriginal) => ({
	...(await importOriginal()),
	goto: vi.fn()
}));

const openai = {
	name: 'openai',
	network_config: { base_url: '', max_retries: 2 },
	concurrency_and_buffer_size: { concurrency: 1000, buffer_size: 5000 },
	provider_status: 'active'
};

function mockGateway({
	status = readyStatus,
	providers = [openai],
	keys = [{ id: 'k1', name: 'openai-key-1', value: { value: 'sk-***' }, models: ['*'], weight: 1 }]
}: {
	status?: typeof readyStatus;
	providers?: (typeof openai)[];
	keys?: unknown[];
} = {}) {
	worker.use(
		http.get('/api/llm-gateway/status', () => HttpResponse.json(status)),
		http.get('/api/llm-gateway/admin/providers', () =>
			HttpResponse.json({ providers, total: providers.length })
		),
		http.get('/api/llm-gateway/admin/providers/:name/keys', () =>
			HttpResponse.json({ keys, total: keys.length })
		)
	);
}

// The card of one provider, found by its heading.
function card(name: string) {
	return page
		.getByRole('heading', { name, exact: true })
		.element()
		.closest('div.rounded-lg') as HTMLElement;
}

async function renderProvidersPage(groups: string[] = [Group.ADMIN]) {
	return renderModelsTab('providers', groups);
}

afterEach(resetModelsTab);

describe('Models providers tab', () => {
	it('enables Set Default Models once the gateway has models', async () => {
		mockGateway();
		worker.use(
			http.get('/api/models', ({ request }) =>
				HttpResponse.json({
					items:
						new URL(request.url).searchParams.get('all') === 'true'
							? [{ id: 'm1', name: 'gpt-5-mini', usage: 'llm', active: true }]
							: []
				})
			)
		);
		await renderProvidersPage();

		await expect.element(page.getByRole('button', { name: 'Set Default Models' })).toBeEnabled();
	});

	it('lists gateway providers as provider cards with their key counts', async () => {
		mockGateway();
		await renderProvidersPage();

		await expect.element(page.getByRole('heading', { name: 'OpenAI' })).toBeInTheDocument();
		await expect.element(page.getByText('1 key', { exact: true })).toBeInTheDocument();
		await expect.element(page.getByText('Configured', { exact: true })).toBeInTheDocument();
	});

	it('removes a provider and its keys from its card', async () => {
		let deleted = '';
		mockGateway();
		worker.use(
			http.delete('/api/llm-gateway/admin/providers/:name', ({ params }) => {
				deleted = String(params.name);
				// Once removed, the provider is no longer listed.
				mockGateway({ providers: [] });
				return HttpResponse.json({});
			})
		);
		await renderProvidersPage();

		await page.getByRole('button', { name: 'Row actions' }).click();
		await page.getByRole('button', { name: 'Deconfigure Provider' }).click();
		await page.getByRole('button', { name: "Yes, I'm sure" }).click();

		await expect.poll(() => deleted).toBe('openai');
		await expect.element(page.getByText('No providers configured')).toBeInTheDocument();
	});

	it('warns when a provider has no keys', async () => {
		mockGateway({ keys: [] });
		await renderProvidersPage();

		await expect
			.element(page.getByText('No keys: add one to use it', { exact: true }))
			.toBeInTheDocument();
	});

	it('opens a provider from its card', async () => {
		mockGateway();
		await renderProvidersPage();

		await page.getByRole('button', { name: 'Modify' }).click();

		expect(goto).toHaveBeenCalledWith('/models/providers/openai');
	});

	it('does not offer legacy providers or a legacy import', async () => {
		mockGateway();
		await renderProvidersPage();

		await expect.element(page.getByRole('heading', { name: 'OpenAI' })).toBeInTheDocument();
		await expect
			.element(page.getByRole('heading', { name: 'Legacy providers' }))
			.not.toBeInTheDocument();
		await expect
			.element(page.getByRole('button', { name: /Import legacy providers/ }))
			.not.toBeInTheDocument();
	});

	it('shows every available provider, configured ones first', async () => {
		mockGateway();
		await renderProvidersPage();

		await expect.element(page.getByRole('heading', { name: 'Anthropic' })).toBeInTheDocument();
		const headings = page.getByRole('heading', { level: 4 }).elements();
		expect(headings[0].textContent).toBe('OpenAI');
		expect(headings.map((h) => h.textContent)).toEqual(
			expect.arrayContaining(['Anthropic', 'Groq', 'AWS Bedrock'])
		);
		await expect
			.element(page.getByRole('button', { name: 'Configure' }).first())
			.toBeInTheDocument();
	});

	it('shows a notice above the providers when none are configured', async () => {
		mockGateway({ providers: [] });
		await renderProvidersPage();

		await expect.element(page.getByText('No providers configured')).toBeInTheDocument();
		await expect.element(page.getByRole('heading', { name: 'OpenAI' })).toBeInTheDocument();
	});

	it('does not warn about keys when Anthropic is used with Claude subscriptions only', async () => {
		mockGateway({ providers: [{ ...openai, name: 'anthropic' }], keys: [] });
		worker.use(
			http.get('/api/llm-gateway/settings', () =>
				HttpResponse.json({ subscriptionPassthrough: true })
			)
		);
		await renderProvidersPage();

		await expect.element(page.getByText('Claude subscriptions only')).toBeInTheDocument();
		await expect.element(page.getByText('No keys: add one to use it')).not.toBeInTheDocument();
	});

	it('shows a starting banner and no actions while the gateway is starting', async () => {
		mockGateway({ status: { ...readyStatus, ready: false } });
		await renderProvidersPage();

		await expect.element(page.getByText('The LLM gateway is starting…')).toBeInTheDocument();
		await expect
			.element(page.getByRole('button', { name: 'Add Custom Provider' }))
			.not.toBeInTheDocument();
	});

	it('hides management actions from auditors', async () => {
		mockGateway();
		await renderProvidersPage([Group.AUDITOR]);

		await expect.element(page.getByRole('heading', { name: 'OpenAI' })).toBeInTheDocument();
		await expect.element(page.getByRole('button', { name: 'View' }).first()).toBeInTheDocument();
		await expect.element(page.getByRole('button', { name: 'Configure' })).not.toBeInTheDocument();
		await expect
			.element(page.getByRole('button', { name: 'Add Custom Provider' }))
			.not.toBeInTheDocument();
	});

	it('configures a provider from its card', async () => {
		mockGateway({ providers: [] });
		const created = vi.fn();
		const keyCreated = vi.fn();
		worker.use(
			http.post('/api/llm-gateway/admin/providers', async ({ request }) => {
				created(await request.json());
				return HttpResponse.json({ ...openai, name: 'groq' });
			}),
			http.post('/api/llm-gateway/admin/providers/groq/keys', async ({ request }) => {
				keyCreated(await request.json());
				return HttpResponse.json({ id: 'k2', name: 'groq-key-1' });
			})
		);
		await renderProvidersPage();

		await expect.element(page.getByRole('heading', { name: 'Groq' })).toBeInTheDocument();
		card('Groq').querySelector('button')?.click();
		await expect.element(page.getByRole('heading', { name: 'Add Groq' })).toBeInTheDocument();
		await expect.element(page.getByText('All providers')).not.toBeInTheDocument();
		await page.getByPlaceholder('gsk_...').fill('gsk_secret');
		await page.getByRole('button', { name: 'Add Provider' }).click();

		await vi.waitFor(() => expect(keyCreated).toHaveBeenCalledOnce());
		expect(created).toHaveBeenCalledWith(expect.objectContaining({ provider: 'groq' }));
		expect(keyCreated).toHaveBeenCalledWith(
			expect.objectContaining({ name: 'groq-key-1', value: 'gsk_secret', models: ['*'] })
		);
		await vi.waitFor(() => expect(goto).toHaveBeenCalledWith('/models/providers/groq'));
	});
});
