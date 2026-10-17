import { page as appPage } from '$app/state';
import { Group } from '$lib/services';
import { CHAT_COMPLETIONS_ONLY_REQUESTS } from '$lib/services/llm-gateway/providers';
import { errors } from '$lib/stores';
import { goto } from '$lib/url';
import { createMockProfile, preparePageData } from '../../../../tests/helpers/pageData';
import { worker } from '../../../../tests/mocks/worker';
import ProviderPage from './+page.svelte';
import { http, HttpResponse } from 'msw';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

vi.mock(import('$lib/url'), async (importOriginal) => ({
	...(await importOriginal()),
	goto: vi.fn()
}));

const openai = {
	name: 'openai',
	network_config: { max_retries: 2, default_request_timeout_in_seconds: 300 },
	concurrency_and_buffer_size: { concurrency: 1000, buffer_size: 5000 },
	provider_status: 'active'
};

const existingKey = {
	id: 'k1',
	name: 'openai-key-1',
	value: { value: 'sk-***' },
	models: ['*'],
	weight: 1,
	enabled: true,
	status: 'success'
};

function mockProvider({
	provider = openai as Record<string, unknown>,
	keys = [existingKey] as unknown[]
} = {}) {
	const synced = vi.fn();
	worker.use(
		http.get('/api/llm-gateway/admin/providers/:name', () => HttpResponse.json(provider)),
		http.get('/api/llm-gateway/admin/providers', () =>
			HttpResponse.json({ providers: [provider], total: 1 })
		),
		http.get('/api/llm-gateway/admin/providers/:name/keys', () =>
			HttpResponse.json({ keys, total: keys.length })
		),
		// The page lists Obot's models for the provider, which profiles grant.
		http.get('/api/models', () =>
			HttpResponse.json({
				items: [
					{ id: 'm1', targetModel: 'openai/gpt-4o', modelProvider: 'llm-gateway-model-provider' },
					{
						id: 'm2',
						targetModel: 'anthropic/claude-sonnet-5',
						modelProvider: 'llm-gateway-model-provider'
					}
				]
			})
		),
		http.post('/api/llm-gateway/sync-models', () => {
			synced();
			return HttpResponse.json({});
		})
	);
	return { synced };
}

async function renderProviderPage(groups: string[] = [Group.ADMIN], name = 'openai') {
	(appPage.params as Record<string, string>).name = name;
	await preparePageData({ profile: createMockProfile(groups) });
	return render(ProviderPage);
}

afterEach(() => {
	delete (appPage.params as Record<string, string>).name;
});

describe('LLM gateway provider page', () => {
	it('shows the provider keys and models', async () => {
		mockProvider();
		await renderProviderPage();

		await expect.element(page.getByText('openai-key-1')).toBeInTheDocument();
		await expect.element(page.getByText('sk-***')).toBeInTheDocument();
		await expect.element(page.getByText('healthy')).toBeInTheDocument();
		await expect.element(page.getByRole('heading', { name: 'Models (1)' })).toBeInTheDocument();
		await expect.element(page.getByText('gpt-4o', { exact: true })).toBeInTheDocument();
		await expect.element(page.getByLabelText('Max retries')).toHaveValue(2);
	});

	it('adds a key with the next free name and re-syncs models', async () => {
		const { synced } = mockProvider();
		const created = vi.fn();
		worker.use(
			http.post('/api/llm-gateway/admin/providers/:name/keys', async ({ params, request }) => {
				created(params.name, await request.json());
				return HttpResponse.json({ id: 'k2', name: 'openai-key-2' });
			})
		);
		await renderProviderPage();

		await page.getByRole('button', { name: 'Add Key' }).click();
		const dialog = page.getByRole('dialog');
		await expect.element(dialog.getByLabelText('Name')).toHaveValue('openai-key-2');
		await dialog.getByPlaceholder('sk-...').fill('sk-new');
		await dialog.getByLabelText('Allowed models').fill('gpt-4o, gpt-4.1');
		await dialog.getByRole('button', { name: 'Save', exact: true }).click();

		await vi.waitFor(() => expect(created).toHaveBeenCalledOnce());
		expect(created).toHaveBeenCalledWith('openai', {
			name: 'openai-key-2',
			models: ['gpt-4o', 'gpt-4.1'],
			blacklisted_models: [],
			weight: 1,
			enabled: true,
			value: 'sk-new'
		});
		await vi.waitFor(() => expect(synced).toHaveBeenCalled());
	});

	it('edits an existing key', async () => {
		mockProvider();
		const updated = vi.fn();
		worker.use(
			http.put('/api/llm-gateway/admin/providers/:name/keys/:id', async ({ params, request }) => {
				updated(params.name, params.id, await request.json());
				return HttpResponse.json(existingKey);
			})
		);
		await renderProviderPage();

		await page.getByRole('button', { name: 'Edit key' }).click();
		const dialog = page.getByRole('dialog');
		await expect.element(dialog.getByLabelText('Name')).toHaveValue('openai-key-1');
		await dialog.getByLabelText('Blocked models').fill('o1-pro');
		await dialog.getByLabelText('Load-balancing weight').fill('2');
		await dialog.getByRole('button', { name: 'Save', exact: true }).click();

		await vi.waitFor(() => expect(updated).toHaveBeenCalledOnce());
		expect(updated).toHaveBeenCalledWith('openai', 'k1', {
			name: 'openai-key-1',
			models: ['*'],
			blacklisted_models: ['o1-pro'],
			weight: 2,
			enabled: true,
			// The redacted value the API returned is sent back unchanged.
			value: 'sk-***'
		});
	});

	it('disables a key from its row toggle', async () => {
		mockProvider();
		const updated = vi.fn();
		worker.use(
			http.put('/api/llm-gateway/admin/providers/:name/keys/:id', async ({ params, request }) => {
				updated(params.id, await request.json());
				return HttpResponse.json(existingKey);
			})
		);
		await renderProviderPage();

		await page.getByRole('switch', { name: 'Enabled' }).click();

		await vi.waitFor(() => expect(updated).toHaveBeenCalledOnce());
		expect(updated).toHaveBeenCalledWith(
			'k1',
			expect.objectContaining({ name: 'openai-key-1', enabled: false, models: ['*'], weight: 1 })
		);
	});

	it('deletes a key after confirmation', async () => {
		mockProvider();
		const deleted = vi.fn();
		worker.use(
			http.delete('/api/llm-gateway/admin/providers/:name/keys/:id', ({ params }) => {
				deleted(params.name, params.id);
				return new HttpResponse(null, { status: 204 });
			})
		);
		await renderProviderPage();

		await page.getByRole('button', { name: 'Delete key' }).click();
		await expect.element(page.getByText('Delete key openai-key-1?')).toBeInTheDocument();
		await page.getByRole('button', { name: "Yes, I'm sure" }).click();

		await vi.waitFor(() => expect(deleted).toHaveBeenCalledWith('openai', 'k1'));
		expect(deleted).toHaveBeenCalledOnce();
	});

	it('deletes the provider and returns to the provider list', async () => {
		mockProvider();
		const deleted = vi.fn();
		worker.use(
			http.delete('/api/llm-gateway/admin/providers/:name', ({ params }) => {
				deleted(params.name);
				return new HttpResponse(null, { status: 204 });
			})
		);
		await renderProviderPage();

		await page.getByRole('button', { name: 'Delete provider' }).click();
		await page.getByRole('button', { name: "Yes, I'm sure" }).click();

		await vi.waitFor(() => expect(deleted).toHaveBeenCalledWith('openai'));
		await vi.waitFor(() => expect(goto).toHaveBeenCalledWith('/models?view=providers'));
	});

	it('does not offer keys for a keyless custom provider', async () => {
		mockProvider({
			provider: {
				...openai,
				name: 'local-llm',
				custom_provider_config: { base_provider_type: 'openai', is_key_less: true }
			},
			keys: []
		});
		await renderProviderPage([Group.ADMIN], 'local-llm');

		await expect.element(page.getByText('This provider does not use keys.')).toBeInTheDocument();
		await expect.element(page.getByRole('button', { name: 'Add Key' })).not.toBeInTheDocument();
	});

	it('saves network settings from the sticky footer', async () => {
		mockProvider();
		const updated = vi.fn();
		worker.use(
			http.put('/api/llm-gateway/admin/providers/:name', async ({ params, request }) => {
				expect(new URL(request.url).searchParams.has('removeKeys')).toBe(false);
				updated(params.name, await request.json());
				return HttpResponse.json(openai);
			})
		);
		await renderProviderPage();

		await page.getByLabelText('Max retries').fill('5');
		await page.getByRole('button', { name: 'Save', exact: true }).click();

		await vi.waitFor(() =>
			expect(updated).toHaveBeenCalledWith(
				'openai',
				expect.objectContaining({
					network_config: expect.objectContaining({ max_retries: 5 })
				})
			)
		);
	});

	it('turns on the Responses API for a custom OpenAI-compatible provider', async () => {
		const local = {
			...openai,
			name: 'local-llm',
			custom_provider_config: {
				base_provider_type: 'openai',
				allowed_requests: CHAT_COMPLETIONS_ONLY_REQUESTS
			}
		};
		mockProvider({ provider: local });
		const updated = vi.fn();
		worker.use(
			http.put('/api/llm-gateway/admin/providers/:name', async ({ request }) => {
				updated(await request.json());
				return HttpResponse.json({
					...local,
					custom_provider_config: { base_provider_type: 'openai' }
				});
			})
		);
		await renderProviderPage([Group.ADMIN], 'local-llm');

		const responses = page.getByLabelText('Serves the OpenAI Responses API');
		await expect.element(responses).not.toBeChecked();
		await responses.click();
		await page.getByRole('button', { name: 'Save', exact: true }).click();

		await vi.waitFor(() => expect(updated).toHaveBeenCalledOnce());
		expect(updated.mock.calls[0][0].custom_provider_config).toEqual({
			base_provider_type: 'openai'
		});
		await expect.element(responses).toBeChecked();
	});

	it('turns off the Responses API, and leaves it alone when unchanged', async () => {
		const local = {
			...openai,
			name: 'local-llm',
			custom_provider_config: { base_provider_type: 'openai' }
		};
		mockProvider({ provider: local });
		const updated = vi.fn();
		worker.use(
			http.put('/api/llm-gateway/admin/providers/:name', async ({ request }) => {
				updated(await request.json());
				return HttpResponse.json(local);
			})
		);
		await renderProviderPage([Group.ADMIN], 'local-llm');

		const responses = page.getByLabelText('Serves the OpenAI Responses API');
		await expect.element(responses).toBeChecked();
		await page.getByRole('button', { name: 'Save', exact: true }).click();
		await vi.waitFor(() => expect(updated).toHaveBeenCalledOnce());
		expect(updated.mock.calls[0][0]).not.toHaveProperty('custom_provider_config');

		await responses.click();
		await page.getByRole('button', { name: 'Save', exact: true }).click();
		await vi.waitFor(() => expect(updated).toHaveBeenCalledTimes(2));
		expect(updated.mock.calls[1][0].custom_provider_config).toEqual({
			base_provider_type: 'openai',
			allowed_requests: CHAT_COMPLETIONS_ONLY_REQUESTS
		});
	});

	it('only offers the Responses API setting on custom OpenAI-compatible providers', async () => {
		mockProvider();
		await renderProviderPage();

		await expect.element(page.getByLabelText('Max retries')).toBeInTheDocument();
		await expect
			.element(page.getByLabelText('Serves the OpenAI Responses API'))
			.not.toBeInTheDocument();
	});

	it('requires a queue size of at least the concurrency before saving', async () => {
		mockProvider();
		const updated = vi.fn();
		worker.use(
			http.put('/api/llm-gateway/admin/providers/:name', () => {
				updated();
				return HttpResponse.json(openai);
			})
		);
		await renderProviderPage();

		await page.getByLabelText('Queue size').fill('10');
		await expect
			.element(page.getByText('Queue size must be at least the concurrency.'))
			.toBeVisible();
		await expect.element(page.getByLabelText('Queue size')).toHaveAttribute('aria-invalid', 'true');
		await expect.element(page.getByRole('button', { name: 'Save', exact: true })).toBeDisabled();

		await page.getByLabelText('Concurrency').fill('0');
		await expect.element(page.getByText('Concurrency must be at least 1.')).toBeVisible();

		await page.getByLabelText('Concurrency').fill('10');
		await expect
			.element(page.getByText('Queue size must be at least the concurrency.'))
			.not.toBeInTheDocument();
		await page.getByRole('button', { name: 'Save', exact: true }).click();
		await vi.waitFor(() => expect(updated).toHaveBeenCalledOnce());
	});

	it("reports the gateway's queue size error in the field's words, without a page error", async () => {
		mockProvider();
		worker.use(
			http.put('/api/llm-gateway/admin/providers/:name', () =>
				HttpResponse.json(
					{ error: 'Concurrency must be less than or equal to buffer size' },
					{ status: 400 }
				)
			)
		);
		errors.items = [];
		await renderProviderPage();

		await page.getByRole('button', { name: 'Save', exact: true }).click();
		await vi.waitFor(() => expect(errors.items).toHaveLength(1));
		expect(errors.items[0].message).toBe('Queue size must be at least the concurrency.');
		await expect.element(page.getByRole('button', { name: 'Save', exact: true })).toBeEnabled();
	});

	it('removes the keys, after confirmation, when the base URL changes', async () => {
		mockProvider();
		const updated = vi.fn();
		worker.use(
			http.put('/api/llm-gateway/admin/providers/:name', async ({ request }) => {
				updated(new URL(request.url).searchParams.get('removeKeys'), await request.json());
				return HttpResponse.json(openai);
			})
		);
		await renderProviderPage();

		await page.getByLabelText('Base URL').fill('https://llm-proxy.example.com/v1');
		await page.getByRole('button', { name: 'Save', exact: true }).click();
		await expect
			.element(page.getByText(/^Changing the base URL removes this provider's keys \(1\)/))
			.toBeInTheDocument();
		expect(updated).not.toHaveBeenCalled();
		await page.getByRole('button', { name: "Yes, I'm sure" }).click();

		await vi.waitFor(() =>
			expect(updated).toHaveBeenCalledWith(
				'true',
				expect.objectContaining({
					network_config: expect.objectContaining({
						base_url: 'https://llm-proxy.example.com/v1'
					})
				})
			)
		);
	});

	it('links models to the Models tab and goes back to providers on cancel', async () => {
		mockProvider();
		await renderProviderPage();

		await expect
			.element(page.getByRole('main').getByRole('link', { name: 'LLM Gateway', exact: true }))
			.toHaveAttribute('href', '/models?view=connect');
		await page.getByRole('button', { name: 'Cancel' }).click();
		expect(goto).toHaveBeenCalledWith('/models?view=providers');
	});

	it('hides key management from auditors', async () => {
		mockProvider();
		await renderProviderPage([Group.USER, Group.AUDITOR]);

		await expect.element(page.getByText('openai-key-1')).toBeInTheDocument();
		await expect.element(page.getByRole('switch', { name: 'Enabled' })).toBeDisabled();
		await expect.element(page.getByRole('button', { name: 'Add Key' })).not.toBeInTheDocument();
		await expect.element(page.getByRole('button', { name: 'Edit key' })).not.toBeInTheDocument();
		await expect.element(page.getByRole('button', { name: 'Delete key' })).not.toBeInTheDocument();
		await expect
			.element(page.getByRole('button', { name: 'Delete provider' }))
			.not.toBeInTheDocument();
		await expect
			.element(page.getByRole('button', { name: 'Save', exact: true }))
			.not.toBeInTheDocument();
	});

	it('allows Claude subscriptions from the Anthropic provider', async () => {
		mockProvider({ provider: { ...openai, name: 'anthropic' } });
		const saved = vi.fn();
		worker.use(
			http.get('/api/llm-gateway/settings', () =>
				HttpResponse.json({ subscriptionPassthrough: false })
			),
			http.put('/api/llm-gateway/settings', async ({ request }) => {
				const body = await request.json();
				saved(body);
				return HttpResponse.json(body);
			})
		);
		await renderProviderPage([Group.ADMIN], 'anthropic');

		const toggle = page.getByRole('switch', { name: 'Allow Claude subscriptions' });
		await expect.element(toggle).not.toBeChecked();
		await toggle.click();

		await vi.waitFor(() => expect(saved).toHaveBeenCalledOnce());
		expect(saved).toHaveBeenCalledWith({ subscriptionPassthrough: true });
		await expect.element(toggle).toBeChecked();
	});

	it('only offers Claude subscriptions on the built-in Anthropic provider', async () => {
		mockProvider({
			provider: {
				...openai,
				name: 'my-anthropic',
				custom_provider_config: { base_provider_type: 'anthropic' }
			}
		});
		await renderProviderPage([Group.ADMIN], 'my-anthropic');

		await expect.element(page.getByText('openai-key-1')).toBeInTheDocument();
		await expect
			.element(page.getByRole('switch', { name: 'Allow Claude subscriptions' }))
			.not.toBeInTheDocument();
	});

	it('shows the Claude subscription setting read-only to auditors', async () => {
		mockProvider({ provider: { ...openai, name: 'anthropic' } });
		worker.use(
			http.get('/api/llm-gateway/settings', () =>
				HttpResponse.json({ subscriptionPassthrough: true })
			)
		);
		await renderProviderPage([Group.USER, Group.AUDITOR], 'anthropic');

		const toggle = page.getByRole('switch', { name: 'Allow Claude subscriptions' });
		await expect.element(toggle).toBeChecked();
		await expect.element(toggle).toBeDisabled();
	});
});
