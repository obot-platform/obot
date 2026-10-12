import { m } from '$lib/i18n';
import { AdminService, type MCPCatalogEntry, type MCPConfig } from '$lib/services';
import { createMCPCatalogEntry } from '../../../tests/helpers/mcp';
import { createMockProfile, preparePageData } from '../../../tests/helpers/pageData';
import { getVersionResponse } from '../../../tests/mocks/data';
import { worker } from '../../../tests/mocks/worker';
import VMcpComponentConfigurationDialog from './VMcpComponentConfigurationDialog.svelte';
import { http, HttpResponse } from 'msw';
import { describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

function field(overrides: Partial<MCPConfig> & Pick<MCPConfig, 'key' | 'name'>): MCPConfig {
	return {
		description: overrides.description ?? overrides.name,
		required: overrides.required ?? false,
		sensitive: overrides.sensitive ?? false,
		value: overrides.value ?? '',
		usage: overrides.usage ?? 'env',
		...overrides
	};
}

function configurableEntry(
	overrides: { runtime?: MCPCatalogEntry['manifest']['runtime']; config?: MCPConfig[] } = {}
): MCPCatalogEntry {
	return createMCPCatalogEntry({
		id: 'entry-configurable',
		name: 'Configurable server',
		runtime: overrides.runtime,
		manifest: {
			config: overrides.config ?? [
				field({
					key: 'API_TOKEN',
					name: 'API token',
					description: 'Secret token',
					required: true,
					sensitive: true
				}),
				field({
					key: 'REGION',
					name: 'Region',
					description: 'Deployment region',
					required: true
				}),
				field({
					key: 'X-Org',
					name: 'Org header',
					description: 'Organization header',
					usage: 'header'
				})
			]
		}
	});
}

async function selectSecretSource(fieldKey: string, source: 'Manual Value' | 'Kubernetes Secret') {
	const picker = page.getByCSS(`#secret-binding-source-${fieldKey}`);
	await expect.element(picker).toBeVisible();
	await picker.click();
	await page
		.getByCSS(`#secret-binding-source-${fieldKey}-popover`)
		.getByRole('button', { name: source, exact: true })
		.click();
}

async function openAdvanced() {
	const toggle = page.getByRole('dialog').getByRole('checkbox', { name: 'Advanced', exact: true });
	await expect.element(toggle).toBeInTheDocument();
	const input = toggle.element() as HTMLInputElement;
	if (!input.checked) input.click();
}

describe('VMcpComponentConfigurationDialog.svelte', () => {
	it('lets non-administrator editors select the override', async () => {
		await preparePageData({ profile: createMockProfile([]) });
		const onNext = vi.fn();
		const result = await render(VMcpComponentConfigurationDialog, { onNext });
		result.component.open(configurableEntry({ config: [] }));
		await openAdvanced();
		await page.getByRole('checkbox', { name: 'Force single-user' }).click();
		await page.getByRole('button', { name: 'Next' }).click();
		expect(onNext).toHaveBeenCalledWith([], true);
	});
	it('requires values for fixed fields before Next', async () => {
		await preparePageData();
		const onNext = vi.fn();
		const result = await render(VMcpComponentConfigurationDialog, { onNext });
		result.component.open(configurableEntry());

		await expect.element(page.getByText('Configure Configurable server')).toBeVisible();
		await page.getByRole('button', { name: 'Next' }).click();
		await expect
			.element(page.getByText('Please complete all fixed configuration fields with valid values.'))
			.toBeVisible();
		expect(onNext).not.toHaveBeenCalled();
	});

	it('omits a catalog secret binding from the configuration draft', async () => {
		await preparePageData();
		const onNext = vi.fn();
		const result = await render(VMcpComponentConfigurationDialog, { onNext });
		result.component.open(
			configurableEntry({
				config: [
					field({
						key: 'BOUND',
						name: 'Bound',
						description: 'Catalog token',
						required: true,
						secretBinding: { name: 'config', key: 'token' }
					}),
					field({ key: 'REGION', name: 'Region', required: true })
				]
			})
		);

		await expect.element(page.getByText('Bound', { exact: true })).not.toBeInTheDocument();
		await expect
			.element(page.getByRole('combobox', { name: 'Bound policy' }))
			.not.toBeInTheDocument();
		await expect.element(page.getByCSS('#secret-binding-source-BOUND')).not.toBeInTheDocument();
		await expect.element(page.getByCSS('#fixed-BOUND')).not.toBeInTheDocument();
		await page.getByCSS('#fixed-REGION').fill('us-east-1');
		await page.getByRole('button', { name: 'Next' }).click();
		expect(onNext).toHaveBeenCalledWith(
			[{ key: 'REGION', policy: 'fixed', value: 'us-east-1' }],
			false
		);
	});

	it('omits a catalog secret binding from non-administrators', async () => {
		await preparePageData({ profile: createMockProfile([]) });
		const result = await render(VMcpComponentConfigurationDialog);
		result.component.open(
			configurableEntry({
				config: [
					field({
						key: 'BOUND',
						name: 'Bound',
						secretBinding: { name: 'config', key: 'token' }
					})
				]
			})
		);

		await expect.element(page.getByText('Configure Configurable server')).toBeVisible();
		await expect.element(page.getByText('Bound', { exact: true })).not.toBeInTheDocument();
		await expect.element(page.getByText(/config \/ token/)).not.toBeInTheDocument();
		await expect.element(page.getByCSS('#secret-binding-source-BOUND')).not.toBeInTheDocument();
		await expect.element(page.getByCSS('#fixed-BOUND')).not.toBeInTheDocument();
	});

	it('omits a static catalog field from the configuration draft', async () => {
		await preparePageData();
		const onNext = vi.fn();
		const result = await render(VMcpComponentConfigurationDialog, { onNext });
		result.component.open(
			configurableEntry({
				config: [
					field({
						key: 'ENDPOINT',
						name: 'Endpoint',
						description: 'Fixed endpoint',
						required: true,
						static: true,
						value: 'https://example.test'
					}),
					field({ key: 'REGION', name: 'Region', required: true })
				]
			})
		);

		await expect.element(page.getByText('Endpoint', { exact: true })).not.toBeInTheDocument();
		await expect
			.element(page.getByRole('combobox', { name: 'Endpoint policy' }))
			.not.toBeInTheDocument();
		await expect.element(page.getByCSS('#fixed-ENDPOINT')).not.toBeInTheDocument();
		await page.getByCSS('#fixed-REGION').fill('us-east-1');
		await page.getByRole('button', { name: 'Next' }).click();
		expect(onNext).toHaveBeenCalledWith(
			[{ key: 'REGION', policy: 'fixed', value: 'us-east-1' }],
			false
		);
	});

	it('hides configuration with static values', async () => {
		await preparePageData();
		const onNext = vi.fn();
		const result = await render(VMcpComponentConfigurationDialog, { onNext });
		result.component.open(
			configurableEntry({
				config: [
					field({ key: 'SECRET', name: 'Secret', sensitive: true, value: '******' }),
					field({ key: 'REGION', name: 'Region', value: 'us-east-1' }),
					field({
						key: 'BOUND',
						name: 'Bound',
						secretBinding: { name: 'config', key: 'token' }
					})
				]
			})
		);

		await expect
			.element(page.getByRole('combobox', { name: 'Secret policy' }))
			.not.toBeInTheDocument();
		await expect
			.element(page.getByRole('combobox', { name: 'Region policy' }))
			.not.toBeInTheDocument();
		await expect
			.element(page.getByRole('combobox', { name: 'Bound policy' }))
			.not.toBeInTheDocument();
		await page.getByRole('button', { name: 'Next' }).click();
		expect(onNext).toHaveBeenCalledWith([], false);
	});

	it('shows vMCP preconfigured values and secret bindings', async () => {
		await preparePageData();
		const onNext = vi.fn();
		const result = await render(VMcpComponentConfigurationDialog, { onNext });
		result.component.open(
			configurableEntry({
				config: [
					field({
						key: 'ENDPOINT',
						name: 'Endpoint',
						required: true,
						static: true,
						value: 'https://example.test'
					}),
					field({ key: 'REGION', name: 'Region', required: true }),
					field({ key: 'API_TOKEN', name: 'API token', required: true, sensitive: true })
				]
			}),
			{
				configuration: [
					{ key: 'ENDPOINT', policy: 'fixed', value: 'https://override.example' },
					{ key: 'REGION', policy: 'fixed', value: 'us-west-2' },
					{
						key: 'API_TOKEN',
						policy: 'fixed',
						secretBinding: { name: 'gitlab-secret', key: 'gitlab_key' }
					}
				]
			}
		);

		await expect.element(page.getByText('Endpoint', { exact: true })).not.toBeInTheDocument();
		await expect
			.element(page.getByRole('combobox', { name: 'Region policy' }))
			.toHaveValue('fixed');
		await expect.element(page.getByCSS('#fixed-REGION')).toHaveValue('us-west-2');
		await expect
			.element(page.getByText('Kubernetes Secret: gitlab-secret / gitlab_key'))
			.toBeVisible();
		await page.getByRole('button', { name: 'Next' }).click();
		expect(onNext).toHaveBeenCalledWith(
			[
				{ key: 'REGION', policy: 'fixed', value: 'us-west-2' },
				{
					key: 'API_TOKEN',
					policy: 'fixed',
					secretBinding: { name: 'gitlab-secret', key: 'gitlab_key' }
				}
			],
			false
		);
	});

	it('offers only Close for a remote server with no catalog fields', async () => {
		await preparePageData();
		const onNext = vi.fn();
		const result = await render(VMcpComponentConfigurationDialog, { onNext });
		result.component.open(configurableEntry({ runtime: 'remote', config: [] }));

		await expect.element(page.getByRole('button', { name: 'Close' })).toBeVisible();
		await expect.element(page.getByRole('button', { name: 'Next' })).not.toBeInTheDocument();
		expect(onNext).not.toHaveBeenCalled();
	});

	it('shows a value field when Fixed is selected and submits policies on Next', async () => {
		await preparePageData();
		const onNext = vi.fn();
		const result = await render(VMcpComponentConfigurationDialog, { onNext });
		result.component.open(configurableEntry());

		await page
			.getByRole('combobox', { name: 'API token policy' })
			.selectOptions('Provided at connection');
		await page.getByRole('combobox', { name: 'Region policy' }).selectOptions('Preconfigured');
		await page.getByRole('combobox', { name: 'Org header policy' }).selectOptions('Ignore');

		await page.getByCSS('#fixed-REGION').fill('us-east-1');
		await page.getByRole('button', { name: 'Next' }).click();

		await vi.waitFor(() => expect(onNext).toHaveBeenCalledOnce());
		expect(onNext).toHaveBeenCalledWith(
			[
				{ key: 'API_TOKEN', policy: 'userAllowed' },
				{ key: 'REGION', policy: 'fixed', value: 'us-east-1' },
				{ key: 'X-Org', policy: 'prohibited' }
			],
			false
		);
	});

	it('defaults to unchecked and saves the single-user choice without configuration fields', async () => {
		await preparePageData();
		const onNext = vi.fn();
		const onClose = vi.fn();
		const result = await render(VMcpComponentConfigurationDialog, { onNext, onClose });
		result.component.open(configurableEntry({ config: [] }));
		await openAdvanced();
		const checkbox = page.getByRole('checkbox', { name: 'Force single-user' });
		await expect.element(checkbox).not.toBeChecked();
		await checkbox.click();
		await page.getByRole('button', { name: 'Next' }).click();
		expect(onNext).toHaveBeenCalledWith([], true);
		await vi.waitFor(() => expect(onClose).toHaveBeenCalledOnce());
		result.component.open(configurableEntry({ config: [] }));
		await openAdvanced();
		await expect.element(checkbox).not.toBeChecked();
	});

	it('clears and hides the choice when a non-header policy becomes user-allowed', async () => {
		await preparePageData();
		const onNext = vi.fn();
		const result = await render(VMcpComponentConfigurationDialog, { onNext });
		result.component.open(configurableEntry({ config: [field({ key: 'TOKEN', name: 'Token' })] }), {
			forceSingleUser: true
		});
		await openAdvanced();
		const checkbox = page.getByRole('checkbox', { name: 'Force single-user' });
		await expect.element(checkbox).toBeChecked();
		const policy = page.getByRole('combobox', { name: 'Token policy' });
		await policy.selectOptions('Provided at connection');
		await expect.element(checkbox).not.toBeInTheDocument();
		await policy.selectOptions('Ignore');
		await openAdvanced();
		await expect.element(checkbox).not.toBeChecked();
		await page.getByRole('button', { name: 'Next' }).click();
		expect(onNext).toHaveBeenCalledWith([{ key: 'TOKEN', policy: 'prohibited' }], false);
	});

	it('preserves and saves the override with user-allowed headers', async () => {
		await preparePageData();
		const onNext = vi.fn();
		const onClose = vi.fn();
		const result = await render(VMcpComponentConfigurationDialog, { onNext, onClose });
		const entry = configurableEntry({
			config: [field({ key: 'HEADER', name: 'Header', usage: 'header' })]
		});
		result.component.open(entry, { forceSingleUser: true });
		await openAdvanced();
		const checkbox = page.getByRole('checkbox', { name: 'Force single-user' });
		await page
			.getByRole('combobox', { name: 'Header policy' })
			.selectOptions('Provided at connection');
		await expect.element(checkbox).toBeChecked();
		await page.getByRole('button', { name: 'Next' }).click();
		expect(onNext).toHaveBeenCalledWith([{ key: 'HEADER', policy: 'userAllowed' }], true);
		await vi.waitFor(() => expect(onClose).toHaveBeenCalledOnce());

		result.component.open(entry, {
			configuration: [{ key: 'HEADER', policy: 'userAllowed' }],
			forceSingleUser: true
		});
		await openAdvanced();
		await expect.element(checkbox).toBeChecked();
		await checkbox.click();
		await page.getByRole('button', { name: 'Next' }).click();
		expect(onNext).toHaveBeenLastCalledWith([{ key: 'HEADER', policy: 'userAllowed' }], false);
	});

	it('ignores a saved override when existing configuration is user-allowed', async () => {
		await preparePageData();
		const onNext = vi.fn();
		const result = await render(VMcpComponentConfigurationDialog, { onNext });
		result.component.open(configurableEntry({ config: [field({ key: 'TOKEN', name: 'Token' })] }), {
			configuration: [{ key: 'TOKEN', policy: 'userAllowed' }],
			forceSingleUser: true
		});
		await expect
			.element(page.getByRole('checkbox', { name: 'Force single-user' }))
			.not.toBeInTheDocument();
		await page.getByRole('button', { name: 'Next' }).click();
		expect(onNext).toHaveBeenCalledWith([{ key: 'TOKEN', policy: 'userAllowed' }], false);
	});

	it('omits Ignore from required field policies', async () => {
		await preparePageData();
		const result = await render(VMcpComponentConfigurationDialog);
		result.component.open(configurableEntry());

		const requiredPolicy = page.getByRole('combobox', { name: 'API token policy' });
		const optionalPolicy = page.getByRole('combobox', { name: 'Org header policy' });
		await expect.element(requiredPolicy).toBeVisible();
		expect(
			Array.from((requiredPolicy.element() as HTMLSelectElement).options).map(
				(option) => option.value
			)
		).toEqual(['fixed', 'userAllowed']);
		expect(
			Array.from((optionalPolicy.element() as HTMLSelectElement).options).map(
				(option) => option.value
			)
		).toEqual(['fixed', 'userAllowed', 'prohibited']);
	});

	it('prefills existing policies when editing configuration', async () => {
		await preparePageData();
		const onNext = vi.fn();
		const result = await render(VMcpComponentConfigurationDialog, { onNext });
		result.component.open(configurableEntry(), {
			configuration: [
				{ key: 'API_TOKEN', policy: 'userAllowed' },
				{ key: 'REGION', policy: 'fixed', value: 'us-west-2' },
				{ key: 'X-Org', policy: 'prohibited' }
			],
			submitLabel: m.core_save()
		});

		await expect
			.element(page.getByRole('combobox', { name: 'API token policy' }))
			.toHaveValue('userAllowed');
		await expect
			.element(page.getByRole('combobox', { name: 'Region policy' }))
			.toHaveValue('fixed');
		await expect.element(page.getByCSS('#fixed-REGION')).toHaveValue('us-west-2');
		await expect
			.element(page.getByRole('combobox', { name: 'Org header policy' }))
			.toHaveValue('prohibited');
		await expect.element(page.getByRole('button', { name: 'Save' })).toBeVisible();
	});

	it('hides cancel when the configuration cannot be dismissed', async () => {
		await preparePageData();
		const onNext = vi.fn();
		const onClose = vi.fn();
		const result = await render(VMcpComponentConfigurationDialog, { onNext, onClose });
		result.component.open(configurableEntry(), { hideCancel: true });

		await expect.element(page.getByRole('button', { name: 'Next' })).toBeVisible();
		await expect.element(page.getByRole('button', { name: 'Cancel' })).not.toBeInTheDocument();
		await expect.element(page.getByRole('button', { name: 'Close' })).not.toBeInTheDocument();
		expect(onClose).not.toHaveBeenCalled();
	});

	it('shows configuration without letting it be changed', async () => {
		await preparePageData();
		const onNext = vi.fn();
		const result = await render(VMcpComponentConfigurationDialog, { onNext, readonly: true });
		result.component.open(
			configurableEntry({
				config: [
					field({ key: 'REGION', name: 'Region', description: 'Deployment region', required: true })
				]
			}),
			{
				configuration: [{ key: 'REGION', policy: 'fixed', value: 'us-west-2' }],
				forceSingleUser: true,
				submitLabel: m.core_save()
			}
		);

		await expect
			.element(page.getByRole('heading', { name: 'View Configurable server' }))
			.toBeVisible();
		await expect.element(page.getByText(/cannot be changed here/)).toBeVisible();
		const policy = page.getByRole('combobox', { name: 'Region policy' });
		await expect.element(policy).toBeDisabled();
		await expect.element(policy).toHaveValue('fixed');
		const value = page.getByCSS('#fixed-REGION');
		await expect.element(value).toHaveValue('us-west-2');
		await expect.element(value).toHaveAttribute('readonly', '');
		await openAdvanced();
		await expect.element(page.getByRole('checkbox', { name: 'Force single-user' })).toBeDisabled();
		await expect.element(page.getByRole('button', { name: 'Save' })).not.toBeInTheDocument();
		await expect.element(page.getByRole('button', { name: 'Next' })).not.toBeInTheDocument();
		await page.getByRole('button', { name: 'Close' }).click();
		expect(onNext).not.toHaveBeenCalled();
	});

	it('submits a Kubernetes secret binding instead of a manual fixed value', async () => {
		await preparePageData({ version: { ...getVersionResponse, engine: 'kubernetes' } });
		worker.use(
			http.get('/api/mcp-server-binding-secrets', () =>
				HttpResponse.json({
					items: [{ name: 'gitlab-secret', keys: ['gitlab_key', 'other_key'] }]
				})
			)
		);
		const onNext = vi.fn();
		const result = await render(VMcpComponentConfigurationDialog, { onNext });
		result.component.open(configurableEntry(), { isShared: true });

		await selectSecretSource('API_TOKEN', 'Kubernetes Secret');
		await expect.element(page.getByCSS('[id="API token"]')).not.toBeInTheDocument();
		await expect
			.element(page.getByCSS('#secret-binding-secret-API_TOKEN'))
			.toHaveTextContent('gitlab-secret');
		await page.getByCSS('#secret-binding-key-API_TOKEN').click();
		await page
			.getByCSS('#secret-binding-key-API_TOKEN-popover')
			.getByRole('button', { name: 'other_key', exact: true })
			.click();
		await page.getByCSS('#fixed-REGION').fill('us-east-1');
		await page.getByRole('button', { name: 'Next' }).click();

		await vi.waitFor(() => expect(onNext).toHaveBeenCalledOnce());
		expect(onNext).toHaveBeenCalledWith(
			[
				{
					key: 'API_TOKEN',
					policy: 'fixed',
					secretBinding: { name: 'gitlab-secret', key: 'other_key' }
				},
				{ key: 'REGION', policy: 'fixed', value: 'us-east-1' },
				{ key: 'X-Org', policy: 'prohibited' }
			],
			false
		);
	});

	it('submits a manual value after switching away from a secret binding', async () => {
		await preparePageData({ version: { ...getVersionResponse, engine: 'kubernetes' } });
		worker.use(
			http.get('/api/mcp-server-binding-secrets', () =>
				HttpResponse.json({
					items: [{ name: 'gitlab-secret', keys: ['gitlab_key'] }]
				})
			)
		);
		const onNext = vi.fn();
		const result = await render(VMcpComponentConfigurationDialog, { onNext });
		result.component.open(configurableEntry(), {
			isShared: true,
			configuration: [
				{
					key: 'API_TOKEN',
					policy: 'fixed',
					secretBinding: { name: 'gitlab-secret', key: 'gitlab_key' }
				},
				{ key: 'REGION', policy: 'fixed', value: 'us-east-1' }
			]
		});

		await expect
			.element(page.getByCSS('#secret-binding-source-API_TOKEN'))
			.toHaveTextContent('Kubernetes Secret');
		await selectSecretSource('API_TOKEN', 'Manual Value');
		await page.getByCSS('[id="API token"]').fill('manual-token');
		await page.getByRole('button', { name: 'Next' }).click();

		await vi.waitFor(() => expect(onNext).toHaveBeenCalledOnce());
		expect(onNext).toHaveBeenCalledWith(
			[
				{ key: 'API_TOKEN', policy: 'fixed', value: 'manual-token' },
				{ key: 'REGION', policy: 'fixed', value: 'us-east-1' },
				{ key: 'X-Org', policy: 'prohibited' }
			],
			false
		);
	});

	it('offers secret bindings only for supported fixed fields', async () => {
		await preparePageData({ version: { ...getVersionResponse, engine: 'kubernetes' } });
		worker.use(
			http.get('/api/mcp-server-binding-secrets', () =>
				HttpResponse.json({ items: [{ name: 'gitlab-secret', keys: ['gitlab_key'] }] })
			)
		);
		const result = await render(VMcpComponentConfigurationDialog);
		result.component.open(
			configurableEntry({
				runtime: 'remote',
				config: [
					field({
						key: 'REGION',
						name: 'Region',
						required: true,
						options: [{ name: 'US East', value: 'us-east-1' }]
					}),
					field({ key: 'TOKEN', name: 'Token', required: true, sensitive: true }),
					field({ key: 'X-Org', name: 'Org header', usage: 'header', required: true })
				]
			}),
			{ isShared: true }
		);

		await expect.element(page.getByCSS('#secret-binding-source-X-Org')).toBeVisible();
		await expect.element(page.getByCSS('#fixed-REGION')).toBeVisible();
		await expect.element(page.getByCSS('#secret-binding-source-REGION')).not.toBeInTheDocument();
		await expect.element(page.getByCSS('#secret-binding-source-TOKEN')).not.toBeInTheDocument();
	});

	it('aborts secret binding lookup when the dialog closes', async () => {
		await preparePageData({ version: { ...getVersionResponse, engine: 'kubernetes' } });
		let signal: AbortSignal | undefined;
		const listTargets = vi.spyOn(AdminService, 'listMCPSecretBindingTargets').mockImplementation(
			(opts) =>
				new Promise((_resolve, reject) => {
					signal = opts?.signal;
					opts?.signal?.addEventListener('abort', () => {
						reject(new DOMException('The operation was aborted.', 'AbortError'));
					});
				})
		);
		try {
			const result = await render(VMcpComponentConfigurationDialog);
			result.component.open(configurableEntry(), { isShared: true });

			await vi.waitFor(() => expect(signal).toBeDefined());
			await page.getByRole('button', { name: 'Cancel' }).click();
			await vi.waitFor(() => expect(signal?.aborted).toBe(true));
			await expect
				.element(page.getByText(/Failed to load Kubernetes Secrets for binding/))
				.not.toBeInTheDocument();
		} finally {
			listTargets.mockRestore();
		}
	});

	it('keeps manual entry when no Kubernetes secrets are available', async () => {
		await preparePageData({ version: { ...getVersionResponse, engine: 'kubernetes' } });
		let listed = false;
		worker.use(
			http.get('/api/mcp-server-binding-secrets', () => {
				listed = true;
				return HttpResponse.json({ items: [] });
			})
		);
		const result = await render(VMcpComponentConfigurationDialog);
		result.component.open(configurableEntry(), { isShared: true });

		await vi.waitFor(() => expect(listed).toBe(true));
		await expect.element(page.getByCSS('#fixed-REGION')).toBeVisible();
		await expect.element(page.getByCSS('#secret-binding-source-API_TOKEN')).not.toBeInTheDocument();
	});

	it('does not offer secret bindings to non-administrators', async () => {
		await preparePageData({
			profile: createMockProfile([]),
			version: { ...getVersionResponse, engine: 'kubernetes' }
		});
		const result = await render(VMcpComponentConfigurationDialog);
		result.component.open(configurableEntry());

		await expect.element(page.getByCSS('#fixed-REGION')).toBeVisible();
		await expect.element(page.getByCSS('#secret-binding-source-API_TOKEN')).not.toBeInTheDocument();
	});

	it('does not offer secret bindings for personal vMCPs to administrators', async () => {
		await preparePageData({ version: { ...getVersionResponse, engine: 'kubernetes' } });
		const listTargets = vi.spyOn(AdminService, 'listMCPSecretBindingTargets');
		try {
			const result = await render(VMcpComponentConfigurationDialog);
			result.component.open(configurableEntry(), { isShared: false });

			await expect.element(page.getByCSS('#fixed-REGION')).toBeVisible();
			await expect
				.element(page.getByCSS('#secret-binding-source-API_TOKEN'))
				.not.toBeInTheDocument();
			expect(listTargets).not.toHaveBeenCalled();
		} finally {
			listTargets.mockRestore();
		}
	});

	it('shows an existing secret binding when Kubernetes secrets cannot be selected', async () => {
		await preparePageData({
			version: { ...getVersionResponse, engine: 'kubernetes', hideK8sDetails: true }
		});
		const onNext = vi.fn();
		const result = await render(VMcpComponentConfigurationDialog, { onNext });
		result.component.open(configurableEntry(), {
			configuration: [
				{
					key: 'API_TOKEN',
					policy: 'fixed',
					secretBinding: { name: 'gitlab-secret', key: 'gitlab_key' }
				},
				{ key: 'REGION', policy: 'fixed', value: 'us-east-1' }
			]
		});

		await expect
			.element(page.getByText('Kubernetes Secret: gitlab-secret / gitlab_key'))
			.toBeVisible();
		await expect.element(page.getByCSS('#secret-binding-source-API_TOKEN')).not.toBeInTheDocument();
		await page.getByRole('button', { name: 'Next' }).click();
		await vi.waitFor(() => expect(onNext).toHaveBeenCalledOnce());
		expect(onNext).toHaveBeenCalledWith(
			[
				{
					key: 'API_TOKEN',
					policy: 'fixed',
					secretBinding: { name: 'gitlab-secret', key: 'gitlab_key' }
				},
				{ key: 'REGION', policy: 'fixed', value: 'us-east-1' },
				{ key: 'X-Org', policy: 'prohibited' }
			],
			false
		);
	});
});
