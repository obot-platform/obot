import type { Model } from '$lib/services';
import { worker } from '../../../tests/mocks/worker';
import SearchModels from './SearchModels.svelte';
import { http, HttpResponse } from 'msw';
import { describe, expect, it } from 'vitest';
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

describe('SearchModels', () => {
	it('finds a model by its ID as well as its display name', async () => {
		worker.use(
			http.get('/api/model-providers', () =>
				HttpResponse.json({
					items: [{ id: 'llm-gateway-model-provider', name: 'LLM Gateway', configured: true }]
				})
			)
		);
		const view = await render(SearchModels, {
			onAdd: () => {},
			defaultAliases: [],
			models: [
				model('m1', 'openai/gpt-4o-mini', 'GPT-4o Mini (openai)'),
				model('m2', 'openai/gpt-4o', 'GPT-4o (openai)')
			]
		});
		view.component.open();

		await page.getByPlaceholder('Search models...').fill('gpt-4o-mini');
		// The search is debounced: wait for it to filter, then check what it kept.
		await expect.element(page.getByText('GPT-4o (openai)')).not.toBeInTheDocument();
		await expect.element(page.getByText('GPT-4o Mini (openai)')).toBeVisible();
	});
});
