import type { OAuthConsent } from '$lib/services';
import { createMCPCatalogEntry, createVMCPComponent } from '../../../../../tests/helpers/mcp';
import { preparePageData } from '../../../../../tests/helpers/pageData';
import { worker } from '../../../../../tests/mocks/worker';
import type { PageData } from './$types';
import ConsentPage from './+page.svelte';
import { http, HttpResponse } from 'msw';
import { describe, expect, it } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

const github = createMCPCatalogEntry({
	id: 'github',
	name: 'GitHub',
	manifest: {
		config: [
			{
				key: 'TOKEN',
				name: 'Token',
				description: '',
				required: true,
				sensitive: false,
				value: '',
				usage: 'env'
			},
			{
				key: 'FIXED',
				name: 'Fixed',
				description: '',
				required: false,
				sensitive: false,
				value: 'fixed',
				usage: 'env'
			}
		]
	}
});
const slack = createMCPCatalogEntry({
	id: 'slack',
	name: 'Slack',
	manifest: {
		config: [
			{
				key: 'X-Api-Key',
				name: 'API key',
				description: '',
				required: false,
				sensitive: false,
				value: '',
				usage: 'header'
			}
		]
	}
});

const components = [
	createVMCPComponent(github, {
		id: 'component-github',
		configuration: [
			{ key: 'TOKEN', policy: 'userAllowed' },
			{ key: 'FIXED', policy: 'fixed' }
		]
	}),
	createVMCPComponent(slack, {
		id: 'component-slack',
		configuration: [{ key: 'X-Api-Key', policy: 'userAllowed' }]
	})
];

const hostnameOnly = createVMCPComponent(
	createMCPCatalogEntry({
		id: 'github-hostname',
		name: 'GitHub hostname',
		runtime: 'remote',
		manifest: {
			remoteConfig: { hostname: 'github.example.com' },
			config: [
				{
					key: '__url',
					name: 'Server URL',
					description: 'URL for github.example.com',
					required: true,
					sensitive: false,
					value: '',
					usage: 'interpolated'
				}
			]
		}
	}),
	{
		id: 'component-github-hostname',
		configuration: [{ key: '__url', policy: 'userAllowed' }]
	}
);

function consent(mcpConfigRequired: boolean, vmcpComponents = components): OAuthConsent {
	return {
		authRequestID: 'auth-1',
		continueURL: '/oauth/continue',
		cancelURL: '/oauth/cancel',
		clientName: 'Test client',
		clientCredentialSource: 'dynamic_client',
		redirectURI: 'https://client.example/callback',
		mcpConfigRequired,
		mcpAuthRequired: true,
		userHasSecondLevelOAuthed: false,
		mcpServerName: 'Developer tools',
		vmcpInstanceID: 'instance-1',
		vmcpComponents
	};
}

async function renderConsent(mcpConfigRequired: boolean) {
	const data = await preparePageData<PageData>({ consent: consent(mcpConfigRequired) });
	return render(ConsentPage, { data });
}

describe('vMCP OAuth consent configuration', () => {
	it('requires and saves component configuration before OAuth authorization', async () => {
		let saved: unknown;
		worker.use(
			http.post('/api/vmcp-instances/instance-1/reveal', () =>
				HttpResponse.json({
					components: {
						'component-github': {},
						'component-slack': { 'X-Api-Key': 'saved-key' }
					}
				})
			),
			http.post('/api/vmcp-instances/instance-1/configure', async ({ request }) => {
				saved = await request.json();
				return HttpResponse.json({});
			}),
			http.get('/oauth/consent/auth-1', () => HttpResponse.json(consent(false)))
		);

		await renderConsent(true);
		await expect.element(page.getByRole('button', { name: 'Continue' })).not.toBeInTheDocument();
		await page.getByRole('button', { name: 'Configure' }).click();
		await expect.element(page.getByLabelText('Token')).toHaveValue('');
		await expect.element(page.getByLabelText('API key')).toHaveValue('saved-key');
		await expect.element(page.getByLabelText('Fixed')).not.toBeInTheDocument();
		await expect.element(page.getByText('Enable')).not.toBeInTheDocument();

		await page.getByRole('button', { name: 'Save' }).click();
		await expect
			.element(page.getByText('Please complete all configuration fields with valid values.'))
			.toBeVisible();

		await page.getByLabelText('Token').fill('updated-token');
		await page.getByRole('button', { name: 'Save' }).click();
		await expect.element(page.getByRole('button', { name: 'Continue' })).toBeVisible();

		expect(saved).toEqual({
			components: {
				'component-github': { TOKEN: 'updated-token' },
				'component-slack': { 'X-Api-Key': 'saved-key' }
			}
		});
		await expect.element(page.getByText('third-party OAuth authorization').first()).toBeVisible();
	});

	it('allows optional configuration before OAuth and reports save failures', async () => {
		worker.use(
			http.post('/api/vmcp-instances/instance-1/reveal', () =>
				HttpResponse.json({
					components: { 'component-github': { TOKEN: 'saved-token' }, 'component-slack': {} }
				})
			),
			http.post('/api/vmcp-instances/instance-1/configure', () =>
				HttpResponse.json({ message: 'Unable to save configuration' }, { status: 500 })
			)
		);

		await renderConsent(false);
		await expect.element(page.getByRole('button', { name: 'Configure' })).toBeVisible();
		await page.getByRole('button', { name: 'Configure' }).click();
		await expect.element(page.getByLabelText('Token')).toHaveValue('saved-token');
		await page.getByLabelText('API key').fill('key');
		await page.getByRole('button', { name: 'Save' }).click();

		await expect.element(page.getByText('Unable to save configuration').first()).toBeVisible();
		await expect.element(page.getByRole('button', { name: 'Continue' })).toBeVisible();
	});

	it('requires and saves an editable hostname-constrained server URL', async () => {
		let saved: unknown;
		worker.use(
			http.post('/api/vmcp-instances/instance-1/reveal', () =>
				HttpResponse.json({
					components: { 'component-github-hostname': { __url: 'https://github.example.com' } }
				})
			),
			http.post('/api/vmcp-instances/instance-1/configure', async ({ request }) => {
				saved = await request.json();
				return HttpResponse.json({});
			}),
			http.get('/oauth/consent/auth-1', () => HttpResponse.json(consent(false, [hostnameOnly])))
		);

		const data = await preparePageData<PageData>({ consent: consent(true, [hostnameOnly]) });
		render(ConsentPage, { data });
		await page.getByRole('button', { name: 'Configure' }).click();
		await expect
			.element(page.getByLabelText('Server URL'))
			.toHaveValue('https://github.example.com');

		await page.getByLabelText('Server URL').fill('');
		await page.getByRole('button', { name: 'Save' }).click();
		await expect
			.element(page.getByText('Please complete all configuration fields with valid values.'))
			.toBeVisible();

		await page.getByLabelText('Server URL').fill('https://api.github.example.com');
		await page.getByRole('button', { name: 'Save' }).click();
		await expect.element(page.getByRole('button', { name: 'Continue' })).toBeVisible();
		expect(saved).toEqual({
			components: { 'component-github-hostname': { __url: 'https://api.github.example.com' } }
		});
	});
});
