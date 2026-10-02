import type { MCPFilterInput } from '$lib/services';
import { preparePageData } from '../../../tests/helpers/pageData';
import { worker } from '../../../tests/mocks/worker';
import FilterForm from './FilterForm.svelte';
import { credentialCatalog, credentialKeys } from './credentialPolicy';
import { http, HttpResponse } from 'msw';
import { describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

function filter(id?: string): MCPFilterInput {
	return {
		id,
		name: 'Credential Filter',
		toolName: 'filter_credentials',
		allowedToMutate: false,
		mcpServerManifest: {
			name: 'Credential Filter',
			description: '',
			runtime: 'containerized',
			containerizedConfig: { image: credentialCatalog.image, port: 8080, path: '/mcp' },
			config: credentialKeys.map((key) => ({ key, name: key, value: '', usage: 'env' }))
		}
	} as MCPFilterInput;
}
function mockSave(launchFails = false) {
	const manifest = vi.fn();
	const configuration = vi.fn();
	worker.use(
		http.post('/api/mcp-webhook-validations', async ({ request }) => {
			manifest(await request.json());
			return HttpResponse.json({ id: 'credential-test' });
		}),
		http.put('/api/mcp-webhook-validations/credential-test', async ({ request }) => {
			manifest(await request.json());
			return HttpResponse.json({
				id: 'credential-test',
				disabled: manifest.mock.calls.at(-1)?.[0].disabled
			});
		}),
		http.post('/api/mcp-webhook-validations/credential-test/configure', async ({ request }) => {
			configuration(await request.json());
			return HttpResponse.json({ id: 'credential-test' });
		}),
		http.post('/api/mcp-webhook-validations/credential-test/launch', () =>
			launchFails
				? HttpResponse.json({ message: 'Scanner initialization failed' }, { status: 500 })
				: HttpResponse.json({})
		),
		http.get(
			'/api/mcp-webhook-validations/credential-test/logs',
			() => new HttpResponse('', { headers: { 'Content-Type': 'text/event-stream' } })
		)
	);
	return { manifest, configuration };
}
describe('managed credential filter', () => {
	it.each([false, true])(
		'disables without saving selector edits with reactive input %s',
		async (reactive) => {
			await preparePageData();
			const { manifest, configuration } = mockSave();
			worker.use(
				http.post('/api/mcp-webhook-validations/credential-test/reveal', () =>
					HttpResponse.json({})
				)
			);
			const initial = filter('credential-test');
			initial.selectors = [{ method: 'tools/call', identifiers: ['echo'] }];
			const reactiveInitial = $state(initial);
			const onUpdate = vi.fn();
			render(FilterForm, {
				filter: reactive ? reactiveInitial : initial,
				mcpSystemCatalogEntryId: 'credential-entry',
				onUpdate
			});
			await page.getByLabelText('Method (Optional)', { exact: true }).fill('resources/read');
			await page.getByPlaceholder('e.g.: tool name or resource URI').fill('unsaved');
			await page.getByRole('button', { name: 'Disable Filter', exact: true }).click();
			await vi.waitFor(() => expect(onUpdate).toHaveBeenCalled());
			expect(manifest).toHaveBeenCalledWith(
				expect.objectContaining({
					disabled: true,
					selectors: [{ method: 'tools/call', identifiers: ['echo'] }]
				})
			);
			expect(configuration).not.toHaveBeenCalled();
		}
	);
	it.each([
		{ disabled: false, button: 'Save', launches: true },
		{ disabled: true, button: 'Save', launches: false },
		{ disabled: true, button: 'Enable Filter', launches: true }
	])('updates an older instance: %j', async ({ disabled, button, launches }) => {
		await preparePageData();
		const { manifest, configuration } = mockSave();
		const launch = vi.fn();
		worker.use(
			http.post('/api/mcp-webhook-validations/credential-test/reveal', () =>
				HttpResponse.json({
					CREDENTIAL_DEFAULT_ACTION: 'allow',
					CREDENTIAL_ALLOW_RULES: 'np.slack.2',
					CREDENTIAL_BLOCK_RULES: 'np.github.1',
					CREDENTIAL_REDACT_RULES: 'np.age.2'
				})
			),
			http.post('/api/mcp-webhook-validations/credential-test/launch', () => {
				launch();
				return HttpResponse.json({});
			})
		);
		const initial = filter('credential-test');
		initial.disabled = disabled;
		initial.mcpServerManifest!.containerizedConfig!.image = 'previous-image';
		// Old manifest metadata must not hide saved configuration values.
		initial.mcpServerManifest!.config = [];
		const onUpdate = vi.fn();
		render(FilterForm, { filter: initial, mcpSystemCatalogEntryId: 'credential-entry', onUpdate });
		await expect
			.element(page.getByRole('button', { name: 'Remove Slack Bot Token' }))
			.toBeVisible();
		await page.getByRole('button', { name: button, exact: true }).click();
		await vi.waitFor(() => expect(onUpdate).toHaveBeenCalled());
		expect(manifest).toHaveBeenCalledWith(
			expect.objectContaining({
				systemMCPServerCatalogEntryID: 'credential-entry',
				disabled: !launches,
				allowedToMutate: true
			})
		);
		expect(manifest.mock.calls[0][0]).not.toHaveProperty('mcpServerManifest');
		expect(configuration).toHaveBeenCalledWith({
			CREDENTIAL_DEFAULT_ACTION: 'allow',
			CREDENTIAL_ALLOW_RULES: 'np.slack.2',
			CREDENTIAL_BLOCK_RULES: 'np.github.1',
			CREDENTIAL_REDACT_RULES: 'np.age.2'
		});
		expect(launch).toHaveBeenCalledTimes(launches ? 1 : 0);
	});
	it('requires removing an obsolete rule before upgrading', async () => {
		await preparePageData();
		const { configuration } = mockSave();
		worker.use(
			http.post('/api/mcp-webhook-validations/credential-test/reveal', () =>
				HttpResponse.json({ CREDENTIAL_ALLOW_RULES: 'removed.rule' })
			)
		);
		const initial = filter('credential-test');
		initial.mcpServerManifest!.containerizedConfig!.image = 'previous-image';
		initial.mcpServerManifest!.config = [];
		const onUpdate = vi.fn();
		render(FilterForm, { filter: initial, mcpSystemCatalogEntryId: 'credential-entry', onUpdate });
		await expect
			.element(
				page.getByText(
					'Unsupported credential rule removed.rule. Remove this override or use a compatible filter version.'
				)
			)
			.toBeVisible();
		await expect.element(page.getByRole('button', { name: 'Save', exact: true })).toBeDisabled();
		await page.getByRole('button', { name: 'Remove removed.rule' }).click();
		await page.getByRole('button', { name: 'Save', exact: true }).click();
		await vi.waitFor(() => expect(onUpdate).toHaveBeenCalled());
		expect(configuration).toHaveBeenCalledWith(
			expect.objectContaining({ CREDENTIAL_ALLOW_RULES: '' })
		);
	});
	it('does not report a failed upgrade deployment as successful', async () => {
		await preparePageData();
		mockSave(true);
		worker.use(
			http.post('/api/mcp-webhook-validations/credential-test/reveal', () => HttpResponse.json({}))
		);
		const initial = filter('credential-test');
		initial.mcpServerManifest!.containerizedConfig!.image = 'previous-image';
		const onUpdate = vi.fn();
		render(FilterForm, { filter: initial, mcpSystemCatalogEntryId: 'credential-entry', onUpdate });
		await expect.element(page.getByRole('button', { name: 'Save', exact: true })).toBeEnabled();
		await page.getByRole('button', { name: 'Save', exact: true }).click();
		await expect.element(page.getByText('MCP Filter Launch Failed')).toBeVisible();
		expect(onUpdate).not.toHaveBeenCalled();
	});
	it('surfaces catalog errors returned by the existing update API', async () => {
		await preparePageData();
		const { configuration } = mockSave();
		const onUpdate = vi.fn();
		worker.use(
			http.post('/api/mcp-webhook-validations/credential-test/reveal', () => HttpResponse.json({})),
			http.put('/api/mcp-webhook-validations/credential-test', () =>
				HttpResponse.json({ message: 'Catalog entry unavailable' }, { status: 404 })
			)
		);
		render(FilterForm, {
			filter: filter('credential-test'),
			mcpSystemCatalogEntryId: 'credential-entry',
			onUpdate
		});
		await expect.element(page.getByRole('button', { name: 'Save', exact: true })).toBeEnabled();
		await page.getByRole('button', { name: 'Save', exact: true }).click();
		await expect.element(page.getByText(/Catalog entry unavailable/)).toBeVisible();
		expect(configuration).not.toHaveBeenCalled();
		expect(onUpdate).not.toHaveBeenCalled();
	});
	it('does not call the restricted reveal endpoint for read-only viewers', async () => {
		await preparePageData();
		const reveal = vi.fn();
		worker.use(
			http.post('/api/mcp-webhook-validations/credential-test/reveal', () => {
				reveal();
				return HttpResponse.json({}, { status: 403 });
			})
		);
		render(FilterForm, {
			filter: filter('credential-test'),
			mcpSystemCatalogEntryId: 'credential-entry',
			readonly: true
		});
		await expect
			.element(page.getByText('Saved credential policy is available only to administrators.'))
			.toBeVisible();
		expect(reveal).not.toHaveBeenCalled();
	});
	it.each(['load failure', 'unsupported rule'])(
		'disables using saved settings despite %s',
		async (failure) => {
			await preparePageData();
			const { manifest, configuration } = mockSave();
			const launch = vi.fn();
			worker.use(
				http.post('/api/mcp-webhook-validations/credential-test/reveal', () =>
					failure === 'load failure'
						? HttpResponse.json({}, { status: 500 })
						: HttpResponse.json(
								failure === 'unsupported rule' ? { CREDENTIAL_ALLOW_RULES: 'removed.rule' } : {}
							)
				),
				http.post('/api/mcp-webhook-validations/credential-test/launch', () => {
					launch();
					return HttpResponse.json({});
				})
			);
			const initial = filter('credential-test');
			const onUpdate = vi.fn();
			render(FilterForm, {
				filter: initial,
				mcpSystemCatalogEntryId: 'credential-entry',
				onUpdate
			});
			await expect.element(page.getByRole('alert')).toBeVisible();
			await page.getByRole('textbox', { name: 'Name', exact: true }).fill('Unsaved name');
			await page.getByRole('button', { name: 'Disable Filter', exact: true }).click();
			await vi.waitFor(() => expect(onUpdate).toHaveBeenCalled());
			expect(manifest).toHaveBeenCalledWith(
				expect.objectContaining({
					name: 'Credential Filter',
					disabled: true,
					systemMCPServerCatalogEntryID: 'credential-entry'
				})
			);
			expect(manifest.mock.calls[0][0]).not.toHaveProperty('mcpServerManifest');
			expect(configuration).not.toHaveBeenCalled();
			expect(launch).not.toHaveBeenCalled();
		}
	);
	it('creates default Block without opening the picker', async () => {
		await preparePageData();
		const { configuration } = mockSave();
		const onCreate = vi.fn();
		render(FilterForm, { filter: filter(), mcpSystemCatalogEntryId: 'credential-entry', onCreate });
		await page.getByRole('button', { name: 'Save', exact: true }).click();
		await vi.waitFor(() => expect(onCreate).toHaveBeenCalled());
		expect(configuration).toHaveBeenCalledWith({
			CREDENTIAL_DEFAULT_ACTION: 'block',
			CREDENTIAL_ALLOW_RULES: '',
			CREDENTIAL_BLOCK_RULES: '',
			CREDENTIAL_REDACT_RULES: ''
		});
	});
	it('reconstructs explicit overrides, clears removed lists, and enables mutation', async () => {
		await preparePageData();
		const { manifest, configuration } = mockSave();
		worker.use(
			http.post('/api/mcp-webhook-validations/credential-test/reveal', () =>
				HttpResponse.json({
					CREDENTIAL_DEFAULT_ACTION: 'block',
					CREDENTIAL_ALLOW_RULES: 'np.slack.2',
					CREDENTIAL_REDACT_RULES: 'np.github.1'
				})
			)
		);
		const onUpdate = vi.fn();
		render(FilterForm, {
			filter: filter('credential-test'),
			mcpSystemCatalogEntryId: 'credential-entry',
			onUpdate
		});
		await page.getByRole('button', { name: 'Remove Slack Bot Token' }).click();
		await page.getByRole('button', { name: 'Save', exact: true }).click();
		await vi.waitFor(() => expect(onUpdate).toHaveBeenCalled());
		expect(configuration).toHaveBeenCalledWith({
			CREDENTIAL_DEFAULT_ACTION: 'block',
			CREDENTIAL_ALLOW_RULES: '',
			CREDENTIAL_BLOCK_RULES: '',
			CREDENTIAL_REDACT_RULES: 'np.github.1'
		});
		expect(manifest).toHaveBeenCalledWith(expect.objectContaining({ allowedToMutate: true }));
	});
	it('keeps launch failures visible instead of reporting success', async () => {
		await preparePageData();
		mockSave(true);
		const onCreate = vi.fn();
		render(FilterForm, { filter: filter(), mcpSystemCatalogEntryId: 'credential-entry', onCreate });
		await page.getByRole('button', { name: 'Save', exact: true }).click();
		await expect.element(page.getByText('MCP Filter Launch Failed')).toBeVisible();
		expect(onCreate).not.toHaveBeenCalled();
	});
	it.each([
		{ action: 'redact', existingMutation: false },
		{ action: 'block', existingMutation: true }
	])('preserves required or existing mutation: %j', async ({ action, existingMutation }) => {
		await preparePageData();
		const { manifest } = mockSave();
		const initial = filter();
		initial.allowedToMutate = existingMutation;
		const onCreate = vi.fn();
		render(FilterForm, { filter: initial, mcpSystemCatalogEntryId: 'credential-entry', onCreate });
		await page.getByRole('combobox', { name: 'Default action', exact: true }).click();
		await page
			.getByRole('button', { name: action === 'redact' ? 'Redact' : 'Block', exact: true })
			.click();
		await page.getByRole('button', { name: 'Save', exact: true }).click();
		await vi.waitFor(() => expect(onCreate).toHaveBeenCalled());
		expect(manifest).toHaveBeenCalledWith(expect.objectContaining({ allowedToMutate: true }));
	});
	it('prevents overwriting a policy when loading saved configuration fails', async () => {
		await preparePageData();
		worker.use(
			http.post('/api/mcp-webhook-validations/credential-test/reveal', () =>
				HttpResponse.json({ message: 'Unavailable' }, { status: 500 })
			)
		);
		render(FilterForm, {
			filter: filter('credential-test'),
			mcpSystemCatalogEntryId: 'credential-entry'
		});
		await expect
			.element(
				page.getByText(
					'Unable to load the saved credential policy. Reload this page before editing.'
				)
			)
			.toBeVisible();
		await expect.element(page.getByRole('button', { name: 'Save', exact: true })).toBeDisabled();
	});
	it.each([undefined, 'credential-test'])(
		'saves a different image version through the standard catalog flow (id: %s)',
		async (id) => {
			await preparePageData();
			const { manifest, configuration } = mockSave();
			const catalogLookup = vi.fn();
			worker.use(
				http.post('/api/mcp-webhook-validations/credential-test/reveal', () =>
					HttpResponse.json({})
				),
				http.get('/api/system-mcp-catalogs/default/entries/credential-entry', () => {
					catalogLookup();
					return HttpResponse.json({}, { status: 500 });
				})
			);
			const initial = filter(id);
			initial.mcpServerManifest!.containerizedConfig!.image =
				'ghcr.io/obot-platform/credential-filter:v2.0.0';
			const onSave = vi.fn();
			render(FilterForm, {
				filter: initial,
				mcpSystemCatalogEntryId: 'credential-entry',
				onCreate: onSave,
				onUpdate: onSave
			});
			await page.getByRole('combobox', { name: 'Default action', exact: true }).click();
			await page.getByRole('button', { name: 'Redact', exact: true }).click();
			await page.getByRole('button', { name: 'Save', exact: true }).click();
			await vi.waitFor(() => expect(onSave).toHaveBeenCalled());
			expect(catalogLookup).not.toHaveBeenCalled();
			expect(manifest).toHaveBeenCalledWith(
				expect.objectContaining({
					systemMCPServerCatalogEntryID: 'credential-entry',
					allowedToMutate: true
				})
			);
			expect(manifest.mock.calls[0][0]).not.toHaveProperty('mcpServerManifest');
			expect(configuration).toHaveBeenCalledWith({
				CREDENTIAL_DEFAULT_ACTION: 'redact',
				CREDENTIAL_BLOCK_RULES: '',
				CREDENTIAL_ALLOW_RULES: '',
				CREDENTIAL_REDACT_RULES: ''
			});
		}
	);
});
