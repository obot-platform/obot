import { profile } from '$lib/stores';
import { createMockProfile } from '../../tests/helpers/pageData';
import { worker } from '../../tests/mocks/worker';
import ProvidersView from './ProvidersView.svelte';
import { http, HttpResponse } from 'msw';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

beforeEach(() => {
	profile.initialize(createMockProfile());
	worker.use(
		http.get('/api/llm-gateway/admin/providers', () =>
			HttpResponse.json({ providers: [{ name: 'bedrock_mantle' }] })
		),
		http.get('/api/llm-gateway/admin/providers/:provider/keys', () =>
			HttpResponse.json({ keys: [{ id: 'migrated', name: 'imported-key' }] })
		),
		http.get('/api/llm-gateway/migration', () =>
			HttpResponse.json({
				version: 2,
				choices: { bedrock_mantle: 'amazon-bedrock-api-key-model-provider' },
				mappings: {},
				sources: {
					'amazon-bedrock-model-provider': {
						provider: 'bedrock_mantle',
						status: 'not-selected',
						models: []
					},
					'amazon-bedrock-api-key-model-provider': {
						provider: 'bedrock_mantle',
						status: 'imported',
						models: []
					}
				}
			})
		)
	);
});

describe('ProvidersView migration results', () => {
	it('does not show a migration warning for a completed selection', async () => {
		render(ProvidersView);
		await expect.element(page.getByText('1 key', { exact: true })).toBeVisible();
		await expect
			.element(page.getByText('Some legacy model providers need attention.'))
			.not.toBeInTheDocument();
		await expect
			.element(page.getByRole('button', { name: 'Review migration' }))
			.not.toBeInTheDocument();
	});
	it('shows the imported Mantle provider as configured AWS Bedrock without a duplicate', async () => {
		render(ProvidersView);
		await expect.element(page.getByRole('button', { name: 'Modify', exact: true })).toBeVisible();
		await expect
			.element(page.getByRole('heading', { name: 'bedrock_mantle', exact: true }))
			.not.toBeInTheDocument();
		expect(page.getByRole('heading', { name: 'AWS Bedrock', exact: true }).elements()).toHaveLength(
			1
		);
		await expect
			.element(page.getByRole('img', { name: 'AWS Bedrock', exact: true }))
			.toHaveAttribute('src', '/admin/assets/amazon-bedrock.png');
	});
	it('keeps unresolved choices actionable', async () => {
		worker.use(
			http.get('/api/llm-gateway/migration', () =>
				HttpResponse.json({
					version: 0,
					choices: {},
					mappings: {},
					sources: {
						iam: { provider: 'bedrock_mantle', status: 'pending', models: [] },
						api: { provider: 'bedrock_mantle', status: 'pending', models: [] }
					}
				})
			)
		);
		render(ProvidersView);
		await expect
			.element(page.getByText('Some legacy model providers need attention.'))
			.toBeVisible();
		await expect.element(page.getByRole('button', { name: 'Review migration' })).toBeVisible();
	});
	it('keeps failed imports visible without offering a nonexistent choice', async () => {
		worker.use(
			http.get('/api/llm-gateway/migration', () =>
				HttpResponse.json({
					version: 0,
					choices: {},
					mappings: {},
					sources: {
						openai: { name: 'OpenAI', provider: 'openai', status: 'error', models: [] }
					}
				})
			)
		);
		render(ProvidersView);
		await expect
			.element(page.getByText('Some legacy model providers need attention.'))
			.toBeVisible();
		await expect
			.element(page.getByText('OpenAI: The import failed.', { exact: false }))
			.toBeVisible();
		await expect
			.element(page.getByRole('button', { name: 'Review migration' }))
			.not.toBeInTheDocument();
	});
	it('clears the pending warning after a saved choice finishes reconciling', async () => {
		let reads = 0;
		worker.use(
			http.get('/api/llm-gateway/migration', () => {
				reads++;
				return HttpResponse.json({
					version: 1,
					choices: { bedrock_mantle: 'api' },
					mappings: {},
					sources: {
						iam: { provider: 'bedrock_mantle', status: 'not-selected', models: [] },
						api: {
							provider: 'bedrock_mantle',
							status: reads === 1 ? 'pending' : 'imported',
							models: []
						}
					}
				});
			})
		);
		render(ProvidersView);
		await expect
			.element(page.getByText('Some legacy model providers need attention.'))
			.toBeVisible();
		await vi.waitFor(() => expect(reads).toBeGreaterThanOrEqual(2), { timeout: 3500 });
		await expect
			.element(page.getByText('Some legacy model providers need attention.'))
			.not.toBeInTheDocument();
	}, 5000);
});
