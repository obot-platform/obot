import { savedAuthenticationMethod, catalogEntry } from '$lib/services/llm-gateway/providers';
import { renderOpenDialog } from '../../../tests/helpers/openDialog';
import { worker } from '../../../tests/mocks/worker';
import AddProviderDialog from './AddProviderDialog.svelte';
import ProviderKeyDialog from './ProviderKeyDialog.svelte';
import { http, HttpResponse } from 'msw';
import { describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

describe('server identity authentication', () => {
	for (const provider of ['bedrock', 'azure']) {
		const aws = provider === 'bedrock';
		const configName = aws ? 'bedrock_key_config' : 'azure_key_config';
		const sharedLabel = aws ? /^Region/ : /^Endpoint/;
		const sharedValue = aws ? 'us-east-1' : 'https://resource.openai.azure.com';
		const sharedConfig = aws ? { region: sharedValue } : { endpoint: sharedValue };
		const emptyCredentials = aws
			? { access_key: '', secret_key: '', session_token: '', role_arn: '' }
			: { client_id: '', client_secret: '', tenant_id: '' };
		const roleARN = 'arn:aws:iam::123456789012:role/bedrock';

		for (const addingProvider of [true, false]) {
			for (const role of aws ? ['', roleARN] : ['']) {
				it(`${provider}: creates ${addingProvider ? 'a provider' : 'a key'} with server identity${role ? ' and a role' : ''}`, async () => {
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
					await dialog.getByRole('radio', { name: 'Server identity', exact: true }).click();
					const submit = dialog.getByRole('button', {
						name: addingProvider ? 'Add Provider' : 'Save',
						exact: true
					});
					await expect.element(submit).toBeDisabled();
					await dialog.getByLabelText(sharedLabel).fill(sharedValue);
					await expect
						.element(dialog.getByLabelText(aws ? /^Access Key ID/ : /^Entra Client ID/))
						.not.toBeInTheDocument();
					if (role) await dialog.getByLabelText('Role ARN').fill(role);
					await submit.click();
					await vi.waitFor(() => expect(saved).toHaveBeenCalledOnce());
					expect(saved.mock.calls[0][0]).toMatchObject({ value: '' });
					expect(saved.mock.calls[0][0][configName]).toEqual({
						...sharedConfig,
						...(addingProvider ? {} : { ...emptyCredentials, ...(!aws ? { scopes: [] } : {}) }),
						...(role ? { role_arn: role } : {})
					});
				});
			}
		}

		for (const role of aws ? ['', roleARN] : ['']) {
			it(`${provider}: detects saved server identity${role ? ' with a role' : ''} and allows metadata edits`, async () => {
				const saved = vi.fn();
				worker.use(
					http.put('/api/llm-gateway/admin/providers/:provider/keys/:id', async ({ request }) => {
						saved(await request.json());
						return HttpResponse.json({ id: 'key' });
					})
				);
				const { component } = await render(ProviderKeyDialog, { provider });
				const config = { ...sharedConfig, ...(role ? { role_arn: role } : {}) };
				component.open({
					id: 'key',
					name: 'existing',
					value: { value: '' },
					models: ['*'],
					weight: 1,
					[configName]: config
				});
				const dialog = page.getByRole('dialog');
				await expect
					.element(dialog.getByRole('radio', { name: 'Server identity', exact: true }))
					.toBeChecked();
				await dialog.getByLabelText('Name', { exact: true }).fill('renamed');
				await dialog.getByLabelText('Load-balancing weight').fill('2');
				await dialog.getByRole('switch', { name: 'Enabled' }).click();
				await dialog.getByRole('button', { name: 'Save', exact: true }).click();
				await vi.waitFor(() => expect(saved).toHaveBeenCalledOnce());
				expect(saved.mock.calls[0][0]).toMatchObject({
					name: 'renamed',
					value: '',
					weight: 2,
					enabled: false
				});
				expect(saved.mock.calls[0][0][configName]).toEqual({
					...emptyCredentials,
					...config,
					...(!aws ? { scopes: [] } : {})
				});
			});
		}

		for (const explicit of ['api-key', 'credentials']) {
			it(`${provider}: clears ${explicit} when switching to server identity and validates when switching back`, async () => {
				const saved = vi.fn();
				worker.use(
					http.put('/api/llm-gateway/admin/providers/:provider/keys/:id', async ({ request }) => {
						saved(await request.json());
						return HttpResponse.json({ id: 'key' });
					})
				);
				const { component } = await render(ProviderKeyDialog, { provider });
				component.open({
					id: 'key',
					name: 'existing',
					models: ['*'],
					weight: 1,
					value: explicit === 'api-key' ? { value: '********' } : '',
					[configName]: {
						...sharedConfig,
						...(explicit === 'credentials'
							? aws
								? { access_key: 'access', secret_key: { value: '********' } }
								: { client_id: 'client', client_secret: { value: '********' }, tenant_id: 'tenant' }
							: {})
					}
				});
				const dialog = page.getByRole('dialog');
				await dialog.getByRole('radio', { name: 'Server identity', exact: true }).click();
				await dialog.getByRole('button', { name: 'Save', exact: true }).click();
				await vi.waitFor(() => expect(saved).toHaveBeenCalledOnce());
				expect(saved.mock.calls[0][0]).toMatchObject({ value: '' });
				expect(saved.mock.calls[0][0][configName]).toEqual({
					...emptyCredentials,
					...sharedConfig,
					...(!aws ? { scopes: [] } : {})
				});
				component.open({
					id: 'key',
					name: 'existing',
					value: '',
					models: ['*'],
					weight: 1,
					[configName]: sharedConfig
				});
				await dialog
					.getByRole('radio', {
						name:
							explicit === 'api-key' ? 'API Key' : aws ? 'AWS credentials' : 'Microsoft Entra ID',
						exact: true
					})
					.click();
				await expect
					.element(dialog.getByRole('button', { name: 'Save', exact: true }))
					.toBeDisabled();
			});
		}

		it(`${provider}: keeps partial explicit credentials on their own method`, () => {
			const entry = catalogEntry(provider)!;
			const credentialPath = aws ? 'bedrock_key_config.access_key' : 'azure_key_config.client_id';
			expect(savedAuthenticationMethod(entry, { [credentialPath]: 'partial' })).toBe('credentials');
		});
	}
});
