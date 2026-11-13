import {
	authenticationFields,
	catalogEntry,
	savedAuthenticationMethod
} from '$lib/services/llm-gateway/providers';
import { renderOpenDialog } from '../../../tests/helpers/openDialog';
import { worker } from '../../../tests/mocks/worker';
import AddProviderDialog from './AddProviderDialog.svelte';
import ProviderKeyDialog from './ProviderKeyDialog.svelte';
import { http, HttpResponse } from 'msw';
import { describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

describe('provider authentication', () => {
	it('Azure uses an API key when any Entra field is missing', () => {
		const paths = ['client_id', 'client_secret', 'tenant_id'];
		for (const missing of paths) {
			const values = Object.fromEntries(
				paths
					.filter((path) => path !== missing)
					.map((path) => [`azure_key_config.${path}`, 'stored'])
			);
			expect(
				savedAuthenticationMethod(catalogEntry('azure')!, { ...values, value: 'api-key' })
			).toBe('api-key');
		}
	});

	it('discovers Azure deployments using the three account fields and Entra credentials', async () => {
		const saved = vi.fn();
		worker.use(
			http.post('/api/llm-gateway/admin/providers/:provider/keys', async ({ request }) => {
				saved(await request.json());
				return HttpResponse.json({ id: 'key' });
			})
		);
		const dialog = await renderOpenDialog(ProviderKeyDialog, { provider: 'azure' });
		await dialog.getByRole('radio', { name: 'Microsoft Entra ID', exact: true }).click();
		await dialog
			.getByLabelText('Endpoint', { exact: false })
			.fill('https://account.openai.azure.com');
		await dialog.getByLabelText('Entra Client ID', { exact: false }).fill('client');
		await dialog.getByLabelText('Entra Client Secret', { exact: false }).fill('secret');
		await dialog.getByLabelText('Entra Tenant ID', { exact: false }).fill('tenant');
		await dialog.getByLabelText('Subscription ID', { exact: true }).fill('sub');
		await expect.element(dialog.getByRole('button', { name: 'Save', exact: true })).toBeDisabled();
		await dialog.getByLabelText('Resource Group', { exact: true }).fill('group');
		await dialog.getByLabelText('Account Name', { exact: true }).fill('account');
		await dialog.getByRole('button', { name: 'Save', exact: true }).click();
		await vi.waitFor(() => expect(saved).toHaveBeenCalledOnce());
		expect(saved.mock.calls[0][0]).toMatchObject({
			azure_subscription_id: 'sub',
			azure_resource_group: 'group',
			azure_account_name: 'account',
			models: ['*']
		});
	});

	for (const provider of ['bedrock', 'azure']) {
		const aws = provider === 'bedrock';
		const method = aws ? 'AWS credentials' : 'Microsoft Entra ID';
		// The required marker distinguishes Azure’s credential field from its API Key radio.
		const apiKeyLabel = aws ? /^Bedrock API Key/ : /^API Key\s*\*/;
		const shared = aws ? 'Region' : 'Endpoint';
		const sharedValue = aws ? 'us-east-1' : 'https://example.openai.azure.com';
		const credentials = aws
			? { 'Access Key ID': 'access', 'Secret Access Key': 'secret' }
			: {
					'Entra Client ID': 'client',
					'Entra Client Secret': 'secret',
					'Entra Tenant ID': 'tenant'
				};

		for (const addingProvider of [true, false]) {
			it(`${provider}: selects and validates credentials in ${addingProvider ? 'provider creation' : 'key creation'}`, async () => {
				const saved = vi.fn();
				worker.use(
					http.post('/api/llm-gateway/admin/providers', () =>
						HttpResponse.json({ name: provider })
					),
					http.post('/api/llm-gateway/admin/providers/:provider/keys', async ({ request }) => {
						saved(await request.json());
						return HttpResponse.json({ id: 'key' });
					})
				);
				const dialog = addingProvider
					? await renderOpenDialog(AddProviderDialog, { configured: [], existingKeyNames: [] })
					: await renderOpenDialog(ProviderKeyDialog, { provider });
				if (addingProvider)
					await dialog
						.getByRole('button', { name: aws ? /^AWS Bedrock/ : /^Azure OpenAI/ })
						.click();
				const submit = dialog.getByRole('button', {
					name: addingProvider ? 'Add Provider' : 'Save',
					exact: true
				});
				await expect
					.element(dialog.getByRole('radio', { name: 'API Key', exact: true }))
					.toBeChecked();
				await dialog.getByLabelText(apiKeyLabel).fill('api-secret');
				await dialog.getByLabelText(new RegExp('^' + shared)).fill(sharedValue);
				await expect.element(submit).toBeEnabled();
				await dialog.getByRole('radio', { name: method, exact: true }).click();
				await expect.element(submit).toBeDisabled();
				await expect.element(dialog.getByLabelText(apiKeyLabel)).not.toBeInTheDocument();
				for (const [label, value] of Object.entries(credentials)) {
					await dialog.getByLabelText(new RegExp('^' + label)).fill(value);
				}
				await dialog.getByRole('radio', { name: 'API Key', exact: true }).click();
				await expect.element(dialog.getByLabelText(apiKeyLabel)).toHaveValue('api-secret');
				await dialog.getByRole('radio', { name: method, exact: true }).click();
				if (!aws) {
					await expect
						.element(dialog.getByLabelText('Allowed models', { exact: true }))
						.toHaveValue('');
					await dialog.getByLabelText('Allowed models', { exact: true }).fill('my-deployment');
				}
				await submit.click();
				await vi.waitFor(() => expect(saved).toHaveBeenCalledOnce());
				if (!aws) expect(saved.mock.calls[0][0].models).toEqual(['my-deployment']);
				expect(saved.mock.calls[0][0]).toMatchObject({
					value: '',
					[aws ? 'bedrock_key_config' : 'azure_key_config']: aws
						? { access_key: 'access', secret_key: 'secret', region: sharedValue }
						: {
								endpoint: sharedValue,
								client_id: 'client',
								client_secret: 'secret',
								tenant_id: 'tenant'
							}
				});
			});
		}

		it(`${provider}: edits masked credentials, switches methods, and resets on reopening`, async () => {
			const saved = vi.fn();
			worker.use(
				http.put('/api/llm-gateway/admin/providers/:provider/keys/:id', async ({ request }) => {
					saved(await request.json());
					return HttpResponse.json({ id: 'key' });
				})
			);
			const { component } = await render(ProviderKeyDialog, { provider });
			const key = {
				id: 'key',
				name: 'existing',
				azure_subscription_id: aws ? undefined : 'sub',
				azure_resource_group: aws ? undefined : 'group',
				azure_account_name: aws ? undefined : 'account',
				value: '',
				models: ['*'],
				weight: 1,
				[aws ? 'bedrock_key_config' : 'azure_key_config']: aws
					? {
							access_key: { value: '********' },
							secret_key: { value: '********' },
							region: sharedValue
						}
					: {
							endpoint: sharedValue,
							client_id: 'client',
							client_secret: { value: '********' },
							tenant_id: 'tenant'
						}
			};
			component.open(key);
			const dialog = page.getByRole('dialog');
			await expect.element(dialog.getByRole('radio', { name: method, exact: true })).toBeChecked();
			if (!aws) {
				await expect
					.element(dialog.getByLabelText('Subscription ID', { exact: true }))
					.toHaveValue('sub');
				await expect
					.element(dialog.getByLabelText('Resource Group', { exact: true }))
					.toHaveValue('group');
				await expect
					.element(dialog.getByLabelText('Account Name', { exact: true }))
					.toHaveValue('account');
			}
			const submit = dialog.getByRole('button', { name: 'Save', exact: true });
			await expect.element(submit).toBeEnabled();
			await submit.click();
			await vi.waitFor(() => expect(saved).toHaveBeenCalledOnce());
			expect(saved.mock.calls[0][0]).toMatchObject({ value: '' });
			await expect.element(dialog).not.toBeInTheDocument();
			component.open(key);
			await dialog.getByRole('radio', { name: 'API Key', exact: true }).click();
			await expect.element(submit).toBeDisabled();
			await dialog.getByLabelText(apiKeyLabel).fill('replacement');
			await submit.click();
			await vi.waitFor(() => expect(saved).toHaveBeenCalledTimes(2));
			expect(saved.mock.calls[1][0]).toMatchObject({
				value: 'replacement',
				[aws ? 'bedrock_key_config' : 'azure_key_config']: aws
					? { region: sharedValue }
					: { endpoint: sharedValue }
			});
			expect(
				saved.mock.calls[1][0][aws ? 'bedrock_key_config' : 'azure_key_config']
			).toHaveProperty(aws ? 'access_key' : 'client_id', '');
			component.open();
			await expect
				.element(dialog.getByRole('radio', { name: 'API Key', exact: true }))
				.toBeChecked();
			await expect.element(dialog.getByLabelText(apiKeyLabel)).toHaveValue('');
		});

		it(`${provider}: metadata edits keep the active authentication when both methods are saved`, async () => {
			const saved = vi.fn();
			worker.use(
				http.put('/api/llm-gateway/admin/providers/:provider/keys/:id', async ({ request }) => {
					saved(await request.json());
					return HttpResponse.json({ id: 'key' });
				})
			);
			const masked = 'abcd************************wxyz';
			const { component } = await render(ProviderKeyDialog, { provider });
			const key = {
				id: 'key',
				name: 'existing',
				value: { value: masked, type: 'env', ref: 'env.OBOT_TEST_API_KEY' },
				models: ['*'],
				weight: 1,
				[aws ? 'bedrock_key_config' : 'azure_key_config']: aws
					? { region: sharedValue, access_key: 'access', secret_key: { value: masked } }
					: {
							endpoint: sharedValue,
							client_id: { value: 'client' },
							client_secret: { value: masked, type: 'env', ref: 'env.OBOT_TEST_ENTRA_SECRET' },
							tenant_id: { value: 'tenant' }
						}
			};
			component.open(key);
			const dialog = page.getByRole('dialog');
			await expect
				.element(dialog.getByRole('radio', { name: aws ? 'API Key' : method, exact: true }))
				.toBeChecked();
			await dialog.getByLabelText('Name', { exact: true }).fill('renamed');
			await dialog.getByLabelText('Load-balancing weight').fill('2');
			await dialog.getByRole('button', { name: 'Save', exact: true }).click();
			await vi.waitFor(() => expect(saved).toHaveBeenCalledOnce());
			expect(saved.mock.calls[0][0]).toMatchObject({
				name: 'renamed',
				weight: 2,
				value: aws ? { value: masked } : '',
				[aws ? 'bedrock_key_config' : 'azure_key_config']: aws
					? { region: sharedValue, access_key: '', secret_key: '', session_token: '', role_arn: '' }
					: {
							endpoint: sharedValue,
							client_id: { value: 'client' },
							client_secret: { value: masked },
							tenant_id: { value: 'tenant' }
						}
			});
			expect(saved.mock.calls[0][0].value).toEqual(aws ? { value: masked } : '');
			if (!aws)
				expect(saved.mock.calls[0][0].azure_key_config.client_secret).toEqual({ value: masked });
			await expect.element(dialog).not.toBeInTheDocument();
			component.open(key);
			await dialog.getByLabelText(aws ? apiKeyLabel : /^Entra Client Secret/).fill('replacement');
			await dialog.getByRole('button', { name: 'Save', exact: true }).click();
			await vi.waitFor(() => expect(saved).toHaveBeenCalledTimes(2));
			expect(
				aws ? saved.mock.calls[1][0].value : saved.mock.calls[1][0].azure_key_config.client_secret
			).toBe('replacement');
		});

		it(`${provider}: selects saved credentials and excludes inactive fields`, () => {
			const entry = catalogEntry(provider)!;
			const path = entry.authenticationMethods![1].paths[0];
			expect(savedAuthenticationMethod(entry, { [path]: '********' })).toBe('credentials');
			expect(savedAuthenticationMethod(entry, { value: '********', [path]: 'stored' })).toBe(
				'api-key'
			);
			expect(authenticationFields(entry, 'api-key').map((field) => field.path)).not.toContain(path);
		});
	}
});
