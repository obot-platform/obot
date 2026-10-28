import type { Model } from '$lib/services';
import { defaultModelAliases } from '$lib/stores';
import DefaultModels from './DefaultModels.svelte';
import { beforeEach, describe, expect, it } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

function model(id: string, targetModel: string, displayName: string): Model {
	return {
		id,
		active: true,
		aliasAssigned: false,
		created: 0,
		modelProvider: 'llm-gateway-model-provider',
		modelProviderName: 'LLM Gateway',
		name: targetModel.replaceAll('/', '-'),
		displayName,
		targetModel,
		usage: 'llm'
	};
}

// Gateway models are shown by the display name the gateway knows them by; models it doesn't
// know, such as fine-tuned ones, by their ID.
const models = [
	model('m1', 'openai/gpt-4o-mini', 'GPT-4o Mini (openai)'),
	model(
		'm2',
		'openai/ft:gpt-4o-mini-2024-07-18:acme::abc123',
		'ft:gpt-4o-mini-2024-07-18:acme::abc123 (openai)'
	),
	model('m3', 'openai/gpt-4o', 'GPT-4o (openai)')
];

describe('Default model aliases', () => {
	beforeEach(() => {
		defaultModelAliases.current = [{ alias: 'llm-mini', model: 'm3' }];
	});

	it('finds a model by its ID as well as its display name', async () => {
		render(DefaultModels, { availableModels: models });

		await page.getByRole('button', { name: 'Set Default Models' }).click();
		await page.getByRole('combobox').click();
		await page.getByPlaceholder('Search models...').fill('gpt-4o-mini');

		await expect.element(page.getByRole('button', { name: 'GPT-4o Mini (openai)' })).toBeVisible();
		await expect
			.element(page.getByRole('button', { name: /^ft:gpt-4o-mini-2024-07-18/ }))
			.toBeVisible();
		await expect
			.element(page.getByRole('button', { name: 'GPT-4o (openai)' }))
			.not.toBeInTheDocument();

		await page.getByPlaceholder('Search models...').fill('mini (openai');
		await expect.element(page.getByRole('button', { name: 'GPT-4o Mini (openai)' })).toBeVisible();
	});
});
