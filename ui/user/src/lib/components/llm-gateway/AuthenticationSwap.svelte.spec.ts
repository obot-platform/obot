import { worker } from '../../../tests/mocks/worker';
import ProviderKeyDialog from './ProviderKeyDialog.svelte';
import { http, HttpResponse } from 'msw';
import { describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

describe('authentication configuration replacement', () => {
	for (const provider of ['azure', 'bedrock']) {
		const aws = provider === 'bedrock';
		const configName = aws ? 'bedrock_key_config' : 'azure_key_config';
		const emptyConfig = aws
			? { access_key: '', secret_key: '', session_token: '', role_arn: '' }
			: { client_id: '', client_secret: '', tenant_id: '' };
		const sharedConfig = aws
			? { region: 'us-east-1' }
			: { endpoint: 'https://old.openai.azure.com' };
		const oldCredentials = aws
			? {
					access_key: 'old-access',
					secret_key: { value: '********' },
					session_token: { value: '********' },
					role_arn: 'old-role'
				}
			: {
					client_id: 'old-client',
					client_secret: { value: '********' },
					tenant_id: 'old-tenant',
					scopes: ['https://ai.azure.com/.default']
				};
		const labels = {
			api: 'API Key',
			credentials: aws ? 'AWS credentials' : 'Microsoft Entra ID'
		};

		for (const from of ['api', 'credentials'] as const) {
			for (const to of ['api', 'credentials'] as const) {
				if (from === to) continue;
				it(`${provider}: replaces ${from} with ${to} and clears the previous credentials`, async () => {
					const saved = vi.fn();
					worker.use(
						http.put('/api/llm-gateway/admin/providers/:provider/keys/key', async ({ request }) => {
							saved(await request.json());
							return HttpResponse.json({ id: 'key' });
						})
					);
					const { component } = await render(ProviderKeyDialog, { provider });
					component.open({
						id: 'key',
						name: 'existing',
						value: from === 'api' ? { value: '********' } : '',
						models: ['*'],
						weight: 1,
						[configName]: { ...sharedConfig, ...(from === 'credentials' ? oldCredentials : {}) }
					});
					const dialog = page.getByRole('dialog');
					await expect
						.element(dialog.getByRole('radio', { name: labels[from], exact: true }))
						.toBeChecked();
					await dialog.getByRole('radio', { name: labels[to], exact: true }).click();
					const expected = { ...emptyConfig, ...sharedConfig };
					if (!aws) {
						await dialog.getByLabelText(/^Endpoint/).fill('https://new.openai.azure.com');
						Object.assign(expected, {
							endpoint: 'https://new.openai.azure.com'
						});
					}
					if (to === 'api') {
						await dialog
							.getByLabelText(aws ? /^Bedrock API Key/ : /^API Key\s*\*/)
							.fill('new-api-key');
					} else if (to === 'credentials') {
						if (aws) {
							await dialog.getByLabelText(/^Access Key ID/).fill('new-access');
							await dialog.getByLabelText(/^Secret Access Key/).fill('new-secret');
							await dialog.getByLabelText('Session Token').fill('new-session');
							await dialog.getByLabelText('Role ARN').fill('new-role');
							Object.assign(expected, {
								access_key: 'new-access',
								secret_key: 'new-secret',
								session_token: 'new-session',
								role_arn: 'new-role'
							});
						} else {
							await dialog.getByLabelText(/^Entra Client ID/).fill('new-client');
							await dialog.getByLabelText(/^Entra Client Secret/).fill('new-secret');
							await dialog.getByLabelText(/^Entra Tenant ID/).fill('new-tenant');
							Object.assign(expected, {
								client_id: 'new-client',
								client_secret: 'new-secret',
								tenant_id: 'new-tenant'
							});
						}
					}
					await dialog.getByRole('button', { name: 'Save', exact: true }).click();
					await vi.waitFor(() => expect(saved).toHaveBeenCalledOnce());
					expect(saved.mock.calls[0][0]).toMatchObject({
						value: to === 'api' ? 'new-api-key' : ''
					});
					expect(saved.mock.calls[0][0][configName]).toEqual(expected);
				});
			}
		}
	}
});
