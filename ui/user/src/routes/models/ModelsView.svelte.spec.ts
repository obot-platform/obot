import type { Model } from '$lib/services';
import { Group } from '$lib/services';
import accessibleModels from '$lib/stores/accessibleModels.svelte';
import { createMockProfile, preparePageData } from '../../tests/helpers/pageData';
import ModelsView from './ModelsView.svelte';
import { describe, expect, it } from 'vitest';
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

async function renderModelsView(models: Model[], groups = [Group.ADMIN]) {
	await preparePageData({ profile: createMockProfile(groups) });
	// The store is a module singleton shared by every test in this file.
	accessibleModels.set(models);
	return render(ModelsView);
}

describe('ModelsView', () => {
	it('shows an LLM gateway section with its models and client setup', async () => {
		await renderModelsView(gatewayModels);

		await expect.element(page.getByRole('heading', { name: 'LLM Gateway' })).toBeInTheDocument();
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
