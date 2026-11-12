import type { APIKey, Model } from '$lib/services';
import { Group } from '$lib/services';
import accessibleModels from '$lib/stores/accessibleModels.svelte';
import { createMockProfile, preparePageData } from '../../tests/helpers/pageData';
import { worker } from '../../tests/mocks/worker';
import ModelsView from './ModelsView.svelte';
import { http, HttpResponse } from 'msw';
import { describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

function model(overrides: Partial<Model>): Model {
	return {
		id: 'm',
		active: true,
		aliasAssigned: false,
		created: 0,
		modelProvider: 'llm-gateway-model-provider',
		modelProviderName: 'LLM Gateway',
		name: 'model',
		displayName: 'model',
		targetModel: 'model',
		usage: 'llm',
		...overrides
	};
}

const gatewayModels = [
	model({
		id: 'g1',
		name: 'openai-gpt-4o',
		targetModel: 'openai/gpt-4o',
		dialect: 'OpenAIResponses'
	}),
	model({
		id: 'g2',
		name: 'anthropic-claude-opus',
		targetModel: 'anthropic/claude-opus-4-5',
		dialect: 'AnthropicMessages'
	})
];

const legacyAnthropicModel = model({
	id: 'a1',
	modelProvider: 'anthropic-model-provider',
	modelProviderName: 'Anthropic',
	name: 'claude-sonnet-4-5',
	targetModel: 'claude-sonnet-4-5',
	dialect: 'AnthropicMessages'
});

function llmKey(overrides: Partial<APIKey> = {}): APIKey {
	return {
		id: 1,
		userId: 1,
		name: 'Agent Identity API Key',
		canAccessAPI: false,
		canAccessLLMProxy: true,
		canAccessSkills: false,
		canAccessDeviceScans: false,
		createdAt: '2026-01-01T00:00:00.000Z',
		...overrides
	};
}

async function renderModelsView(models: Model[], groups = [Group.ADMIN], apiKeys: APIKey[] = []) {
	await preparePageData({ profile: createMockProfile(groups) });
	// The store is a module singleton shared by every test in this file.
	accessibleModels.set(models);
	return render(ModelsView, { apiKeys });
}

describe('ModelsView', () => {
	it('shows an LLM gateway section with its models and client setup', async () => {
		await renderModelsView(gatewayModels);

		await expect
			.element(page.getByRole('heading', { name: 'LLM Gateway', exact: true }))
			.toBeInTheDocument();
		await expect
			.element(page.getByRole('button', { name: '2 models available', exact: true }))
			.toBeInTheDocument();
		await expect
			.element(page.getByRole('textbox', { name: 'OpenAI base URL' }))
			.toHaveValue(`${window.location.origin}/api/llm-proxy/v1`);
		await expect
			.element(page.getByRole('textbox', { name: 'Anthropic base URL' }))
			.toHaveValue(`${window.location.origin}/api/llm-proxy`);
		await expect
			.element(
				page.getByText('Every model is available in the OpenAI Chat Completions', { exact: false })
			)
			.toBeVisible();
		for (const client of ['Claude Code', 'Codex CLI', 'OpenAI SDK', 'Anthropic SDK', 'curl']) {
			await expect.element(page.getByRole('button', { name: client })).toBeVisible();
		}
	});

	it('opens client setup pointed at this server from a card', async () => {
		await renderModelsView(gatewayModels);

		await page.getByRole('button', { name: 'Claude Code' }).click();

		await expect.element(page.getByRole('heading', { name: 'Claude Code' })).toBeVisible();
		await expect
			.element(
				page.getByText(`export ANTHROPIC_BASE_URL="${window.location.origin}/api/llm-proxy"`, {
					exact: false
				})
			)
			.toBeVisible();
	});

	it('counts a single model in the singular', async () => {
		await renderModelsView([gatewayModels[0]]);

		await expect
			.element(page.getByRole('button', { name: '1 model available', exact: true }))
			.toBeVisible();
	});

	it('ignores models from providers other than the gateway', async () => {
		await renderModelsView([...gatewayModels, legacyAnthropicModel]);

		await expect
			.element(page.getByRole('button', { name: '2 models available', exact: true }))
			.toBeInTheDocument();
		await expect
			.element(page.getByRole('heading', { name: 'Anthropic', exact: true }))
			.not.toBeInTheDocument();
		await expect
			.element(page.getByRole('button', { name: '3 models available', exact: true }))
			.not.toBeInTheDocument();
	});

	it('opens the available models, grouped by provider and searchable, from the model count', async () => {
		const many = Array.from({ length: 21 }, (_, i) =>
			model({ id: `o${i}`, targetModel: `openai/gpt-${String(i).padStart(2, '0')}` })
		);
		await renderModelsView([...many, gatewayModels[1]]);

		// The models aren't listed on the page itself.
		await expect
			.element(page.getByText('anthropic/claude-opus-4-5', { exact: true }))
			.not.toBeVisible();
		await page.getByRole('button', { name: '22 models available', exact: true }).click();
		await expect.element(page.getByRole('heading', { name: 'Available models' })).toBeVisible();
		await expect
			.element(page.getByRole('heading', { name: 'Anthropic', exact: true }))
			.toBeVisible();
		await expect.element(page.getByRole('heading', { name: 'OpenAI', exact: true })).toBeVisible();

		await page
			.getByRole('dialog', { name: 'Available models' })
			.getByPlaceholder('Search models...')
			.fill('claude');
		await expect
			.element(page.getByText('anthropic/claude-opus-4-5', { exact: true }))
			.toBeVisible();
		await expect
			.element(page.getByRole('heading', { name: 'OpenAI', exact: true }))
			.not.toBeInTheDocument();
	});

	it('explains there is nothing to connect to without gateway models', async () => {
		await renderModelsView([legacyAnthropicModel], [Group.USER]);

		await expect.element(page.getByRole('heading', { name: 'No models available' })).toBeVisible();
		await expect
			.element(page.getByRole('button', { name: 'Add Provider' }))
			.not.toBeInTheDocument();
		await expect
			.element(page.getByRole('heading', { name: 'LLM Gateway' }))
			.not.toBeInTheDocument();
		await expect.element(page.getByRole('button', { name: 'Claude Code' })).not.toBeInTheDocument();
	});

	it('creates an LLM-scoped token and uses it in client setup', async () => {
		const created = vi.fn();
		const apiKey = 'ok1-1-1-secret';
		worker.use(
			http.post('/api/api-keys', async ({ request }) => {
				created(await request.json());
				return HttpResponse.json({
					id: 1,
					userId: 1,
					name: 'Agent Identity API Key',
					canAccessAPI: false,
					canAccessLLMProxy: true,
					canAccessSkills: false,
					canAccessDeviceScans: false,
					createdAt: '2026-11-11T00:00:00Z',
					mcpServerIds: [],
					key: apiKey
				});
			})
		);
		await renderModelsView(gatewayModels);

		await page.getByRole('button', { name: 'Generate Key' }).click();

		await vi.waitFor(() => expect(created).toHaveBeenCalledOnce());
		expect(created).toHaveBeenCalledWith({
			name: 'LLM Gateway API Key',
			mcpServerIds: [],
			canAccessLLMProxy: true
		});
		await expect.element(page.getByText('Save this key now')).toBeVisible();
		await expect
			.element(
				page.getByText('This is the only time you will be able to see this API key', {
					exact: false
				})
			)
			.toBeVisible();
		await expect.element(page.getByCSS('#llm-gateway-api-key')).toHaveValue(apiKey);

		await page.getByRole('button', { name: 'Claude Code' }).click();
		const dialog = page.getByRole('dialog');
		await expect.element(dialog.getByRole('textbox', { name: 'Your API Key' })).toHaveValue(apiKey);
		await expect
			.element(
				dialog.getByText('export ANTHROPIC_AUTH_TOKEN="[REPLACE_WITH_API_KEY]"', {
					exact: false
				})
			)
			.toBeVisible();
		await expect
			.element(dialog.getByText(`ANTHROPIC_AUTH_TOKEN="${apiKey}"`, { exact: false }))
			.not.toBeInTheDocument();
		await expect
			.element(dialog.getByText('obot login --url', { exact: false }))
			.not.toBeInTheDocument();
	});

	it('defaults client setup to an API key placeholder and offers the Obot CLI login command', async () => {
		await renderModelsView(gatewayModels);

		await page.getByRole('button', { name: 'Claude Code' }).click();

		await expect
			.element(
				page.getByText('export ANTHROPIC_AUTH_TOKEN="[REPLACE_WITH_API_KEY]"', { exact: false })
			)
			.toBeVisible();
		await expect
			.element(page.getByText('Supply your existing API key for', { exact: false }))
			.toBeVisible();
		await expect
			.element(page.getByRole('dialog').getByRole('button', { name: 'Generate Key', exact: true }))
			.toBeVisible();

		await page
			.getByRole('group', { name: 'How to provide the access key' })
			.first()
			.getByRole('button', { name: 'CLI' })
			.click();

		await expect
			.element(
				page.getByText(`$(obot login --url ${window.location.origin} --scope llm --print-token)`, {
					exact: false
				})
			)
			.toBeVisible();
		await expect
			.element(page.getByText('[REPLACE_WITH_API_KEY]', { exact: false }))
			.not.toBeInTheDocument();
	});

	it('creates a token from the client setup notice when a key already exists', async () => {
		const created = vi.fn();
		const apiKey = 'ok1-from-dialog';
		worker.use(
			http.post('/api/api-keys', async ({ request }) => {
				created(await request.json());
				return HttpResponse.json({
					id: 4,
					userId: 1,
					name: 'Agent Identity API Key',
					canAccessAPI: false,
					canAccessLLMProxy: true,
					canAccessSkills: false,
					canAccessDeviceScans: false,
					createdAt: '2026-11-11T00:00:00Z',
					mcpServerIds: [],
					key: apiKey
				});
			})
		);
		await renderModelsView(gatewayModels, [Group.ADMIN], [llmKey()]);

		await page.getByRole('button', { name: 'Claude Code' }).click();
		await page.getByRole('dialog').getByRole('button', { name: 'Generate New Key' }).click();

		await vi.waitFor(() => expect(created).toHaveBeenCalledOnce());
		expect(created).toHaveBeenCalledWith({
			name: 'LLM Gateway API Key',
			mcpServerIds: [],
			canAccessLLMProxy: true
		});
		// The notice reveals the new key, so the key field stays hidden until the dialog reopens.
		await expect
			.element(page.getByRole('dialog').getByRole('textbox', { name: 'Your API Key' }))
			.not.toBeInTheDocument();
		await expect
			.element(
				page.getByText('export ANTHROPIC_AUTH_TOKEN="[REPLACE_WITH_API_KEY]"', { exact: false })
			)
			.toBeVisible();
		await expect
			.element(page.getByText(`ANTHROPIC_AUTH_TOKEN="${apiKey}"`, { exact: false }))
			.not.toBeInTheDocument();
		await expect.element(page.getByRole('dialog').getByText('Save this key now')).toBeVisible();

		(page.getByRole('dialog').element() as HTMLDialogElement).close();
		await expect.element(page.getByRole('dialog')).not.toBeInTheDocument();
		await page.getByRole('button', { name: 'Claude Code' }).click();
		await expect
			.element(page.getByRole('dialog').getByText('Save this key now'))
			.not.toBeInTheDocument();
		await expect.element(page.getByText('Save this key now')).toBeVisible();
		await expect
			.element(page.getByRole('dialog').getByRole('textbox', { name: 'Your API Key' }))
			.toHaveValue(apiKey);
	});

	it('offers key management and a new key with an existing unexpired LLM identity key', async () => {
		await renderModelsView(
			gatewayModels,
			[Group.ADMIN],
			[
				llmKey({ expiresAt: '2000-01-01T00:00:00.000Z' }),
				llmKey({ id: 2, canAccessLLMProxy: false }),
				llmKey({ id: 3, expiresAt: '2999-01-01T00:00:00.000Z' })
			]
		);

		await expect.element(page.getByRole('link', { name: 'Manage API Keys' })).toBeVisible();
		await expect
			.element(page.getByRole('button', { name: 'Generate Key', exact: true }))
			.not.toBeInTheDocument();

		await page.getByRole('button', { name: 'Claude Code' }).click();
		await expect
			.element(
				page.getByRole('dialog').getByRole('button', { name: 'Generate New Key', exact: true })
			)
			.toBeVisible();
	});

	it('ignores expired and non-LLM identity keys', async () => {
		await renderModelsView(
			gatewayModels,
			[Group.ADMIN],
			[
				llmKey({ expiresAt: '2000-01-01T00:00:00.000Z' }),
				llmKey({ id: 2, canAccessLLMProxy: false, expiresAt: '2999-01-01T00:00:00.000Z' })
			]
		);

		await expect
			.element(page.getByRole('button', { name: 'Generate Key', exact: true }))
			.toBeVisible();
		await expect
			.element(page.getByRole('link', { name: 'Manage API Keys' }))
			.not.toBeInTheDocument();
	});

	it('asks an administrator to add a provider without gateway models', async () => {
		await renderModelsView([]);

		await expect
			.element(page.getByRole('heading', { name: 'Add a model provider to get started' }))
			.toBeVisible();
		await expect.element(page.getByRole('button', { name: 'Add Provider' })).toBeVisible();
		await expect
			.element(page.getByRole('heading', { name: 'No models available' }))
			.not.toBeInTheDocument();
	});
});
