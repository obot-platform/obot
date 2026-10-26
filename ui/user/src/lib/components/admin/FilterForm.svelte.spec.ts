import type { MCPFilterInput, MCPFilterManifest } from '$lib/services';
import { preparePageData } from '../../../tests/helpers/pageData';
import { worker } from '../../../tests/mocks/worker';
import FilterForm from './FilterForm.svelte';
import { http, HttpResponse } from 'msw';
import { afterEach, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

function filter(disabled = false): MCPFilterInput {
	return {
		id: 'filter-test',
		name: 'Original filter',
		url: 'https://example.com/filter',
		disabled,
		resources: [{ id: 'default', type: 'mcpCatalog' }],
		selectors: [{ method: 'tools/call', identifiers: ['echo'] }],
		created: '2026-01-01T00:00:00Z',
		type: 'mcpwebhookvalidation',
		hasSecret: false,
		configured: true
	};
}
function mockApis() {
	const writes = vi.fn<(body: MCPFilterManifest) => void>();
	const launch = vi.fn();
	const create = vi.fn();
	worker.use(
		http.post('/api/mcp-webhook-validations/:id/reveal', () => HttpResponse.json({})),
		http.put('/api/mcp-webhook-validations/:id', async ({ request, params }) => {
			const body = (await request.json()) as MCPFilterManifest;
			writes(body);
			return HttpResponse.json({ ...body, id: params.id });
		}),
		http.post('/api/mcp-webhook-validations', async ({ request }) => {
			const body = (await request.json()) as MCPFilterManifest;
			writes(body);
			create();
			return HttpResponse.json({ ...body, id: 'filter-test' });
		}),
		http.post('/api/mcp-webhook-validations/:id/launch', () => {
			launch();
			return HttpResponse.json({});
		})
	);
	return { writes, launch, create };
}
afterEach(() => vi.restoreAllMocks());

it('shows saved enabled state, tracks edits and applies them in one click', async () => {
	await preparePageData();
	const { writes, launch } = mockApis();
	const onUpdate = vi.fn();
	await render(FilterForm, { filter: filter(), onUpdate });
	const status = page.getByRole('status');
	await expect.element(status.getByText('Enabled', { exact: true })).toBeVisible();
	await expect.element(status.getByText('Saved', { exact: true })).toBeVisible();
	await expect.element(page.getByRole('button', { name: 'Save & apply changes' })).toBeDisabled();
	await page.getByLabelText('Name', { exact: true }).fill('Updated filter');
	await expect.element(status.getByText('Unsaved changes', { exact: true })).toBeVisible();
	await expect
		.element(status.getByText('Your edits are not active yet.', { exact: true }))
		.toBeVisible();
	await page.getByRole('button', { name: 'Save & apply changes' }).click();
	await vi.waitFor(() => expect(onUpdate).toHaveBeenCalled());
	expect(writes).toHaveBeenCalledWith(
		expect.objectContaining({ name: 'Updated filter', disabled: false })
	);
	expect(launch).toHaveBeenCalledOnce();
	await expect.element(status.getByText('Saved', { exact: true })).toBeVisible();
	await expect
		.element(status.getByText('Unsaved changes', { exact: true }))
		.not.toBeInTheDocument();
	await expect.element(page.getByLabelText('Name', { exact: true })).toHaveValue('Updated filter');
	await page.getByLabelText('Name', { exact: true }).fill('Another edit');
	await expect.element(status.getByText('Unsaved changes', { exact: true })).toBeVisible();
});

it.each([true, false])('creates with disabled=%s in one action', async (disabled) => {
	await preparePageData();
	const { writes, launch } = mockApis();
	const onCreate = vi.fn();
	await render(FilterForm, { onCreate });
	await expect
		.element(page.getByRole('status').getByText('Unsaved changes', { exact: true }))
		.toBeVisible();
	await page.getByLabelText('Name', { exact: true }).fill('New filter');
	await page.getByLabelText('Webhook URL', { exact: true }).fill('https://example.com/filter');
	await page
		.getByRole('button', { name: disabled ? 'Save disabled' : 'Save & enable', exact: true })
		.click();
	await vi.waitFor(() => expect(onCreate).toHaveBeenCalled());
	expect(writes).toHaveBeenCalledWith(expect.objectContaining({ disabled }));
	expect(launch).toHaveBeenCalledTimes(disabled ? 0 : 1);
	await expect
		.element(page.getByRole('status').getByText(disabled ? 'Disabled' : 'Enabled', { exact: true }))
		.toBeVisible();
	await expect.element(page.getByRole('status').getByText('Saved', { exact: true })).toBeVisible();
});

it('saves disabled edits and later enables the saved settings', async () => {
	await preparePageData();
	const { writes, launch } = mockApis();
	const onUpdate = vi.fn();
	await render(FilterForm, { filter: filter(true), onUpdate });
	await page.getByLabelText('Name', { exact: true }).fill('Saved disabled filter');
	await page.getByRole('button', { name: 'Save disabled', exact: true }).click();
	await vi.waitFor(() => expect(onUpdate).toHaveBeenCalledTimes(1));
	expect(writes).toHaveBeenLastCalledWith(
		expect.objectContaining({ disabled: true, name: 'Saved disabled filter' })
	);
	expect(launch).not.toHaveBeenCalled();
	await expect
		.element(page.getByRole('status').getByText('Disabled', { exact: true }))
		.toBeVisible();
	await page.getByRole('button', { name: 'Enable Filter', exact: true }).click();
	await vi.waitFor(() => expect(onUpdate).toHaveBeenCalledTimes(2));
	expect(writes).toHaveBeenLastCalledWith(
		expect.objectContaining({ disabled: false, name: 'Saved disabled filter' })
	);
	expect(launch).toHaveBeenCalledOnce();
});

it('does not flip the enabled state when validation rejects enable', async () => {
	await preparePageData();
	const { writes } = mockApis();
	await render(FilterForm, { filter: filter(true) });
	await page.getByLabelText('Name', { exact: true }).fill('');
	await page.getByRole('button', { name: 'Save & enable', exact: true }).click();
	await expect.element(page.getByText('Name is required')).toBeVisible();
	await expect
		.element(page.getByRole('status').getByText('Disabled', { exact: true }))
		.toBeVisible();
	await expect
		.element(page.getByRole('button', { name: 'Disable Filter', exact: true }))
		.not.toBeInTheDocument();
	expect(writes).not.toHaveBeenCalled();
});

it('disables the latest saved settings without saving or losing nested draft edits', async () => {
	await preparePageData();
	const { writes, launch } = mockApis();
	const onUpdate = vi.fn();
	await render(FilterForm, { filter: filter(), onUpdate });
	await page.getByLabelText('Name', { exact: true }).fill('Latest saved name');
	await page.getByRole('button', { name: 'Save & apply changes' }).click();
	await vi.waitFor(() => expect(onUpdate).toHaveBeenCalledTimes(1));
	await page.getByLabelText('Name', { exact: true }).fill('Unsaved name');
	await page.getByLabelText('Method (Optional)', { exact: true }).fill('resources/read');
	await page.getByRole('button', { name: 'Disable Filter', exact: true }).click();
	await vi.waitFor(() => expect(onUpdate).toHaveBeenCalledTimes(2));
	expect(writes).toHaveBeenLastCalledWith(
		expect.objectContaining({
			name: 'Latest saved name',
			disabled: true,
			selectors: [{ method: 'tools/call', identifiers: ['echo'] }]
		})
	);
	expect(launch).toHaveBeenCalledOnce();
	await expect.element(page.getByLabelText('Name', { exact: true })).toHaveValue('Unsaved name');
	await expect
		.element(page.getByLabelText('Method (Optional)', { exact: true }))
		.toHaveValue('resources/read');
	await expect
		.element(page.getByRole('status').getByText('Disabled', { exact: true }))
		.toBeVisible();
	await expect
		.element(page.getByRole('status').getByText('Unsaved changes', { exact: true }))
		.toBeVisible();
});

it('keeps draft edits and saved state when the save API fails', async () => {
	await preparePageData();
	const { launch } = mockApis();
	worker.use(
		http.put('/api/mcp-webhook-validations/:id', () =>
			HttpResponse.json({ message: 'Save unavailable' }, { status: 500 })
		)
	);
	await render(FilterForm, { filter: filter(true) });
	await page.getByLabelText('Name', { exact: true }).fill('Unsaved filter');
	await page.getByRole('button', { name: 'Save & enable' }).click();
	await expect.element(page.getByText('Your changes were not saved.')).toBeVisible();
	await page.getByRole('button', { name: 'Close', exact: true }).click();
	await expect
		.element(page.getByRole('status').getByText('Disabled', { exact: true }))
		.toBeVisible();
	await expect
		.element(page.getByRole('status').getByText('Unsaved changes', { exact: true }))
		.toBeVisible();
	expect(launch).not.toHaveBeenCalled();
});

it('reports saved activation failures and retries the same newly created filter', async () => {
	await preparePageData();
	const { create } = mockApis();
	const onCreate = vi.fn();
	worker.use(
		http.post('/api/mcp-webhook-validations/:id/launch', () =>
			HttpResponse.json({ message: 'Launch unavailable' }, { status: 500 })
		),
		http.get(
			'/api/mcp-webhook-validations/:id/logs',
			() => new HttpResponse('', { headers: { 'Content-Type': 'text/event-stream' } })
		)
	);
	await render(FilterForm, { onCreate });
	await page.getByLabelText('Name', { exact: true }).fill('New filter');
	await page.getByLabelText('Webhook URL', { exact: true }).fill('https://example.com/filter');
	await page.getByRole('button', { name: 'Save & enable' }).click();
	await expect
		.element(
			page.getByText(
				'Your changes were saved, but activation failed. Retry to activate the saved configuration.'
			)
		)
		.toBeVisible();
	await page.getByRole('button', { name: 'Close', exact: true }).click();
	await expect
		.element(page.getByRole('status').getByText('Activation failed', { exact: true }))
		.toBeVisible();
	await expect.element(page.getByRole('status').getByText('Saved', { exact: true })).toBeVisible();
	expect(onCreate).not.toHaveBeenCalled();
	worker.use(http.post('/api/mcp-webhook-validations/:id/launch', () => HttpResponse.json({})));
	await page.getByRole('button', { name: 'Save & apply changes' }).click();
	await vi.waitFor(() => expect(onCreate).toHaveBeenCalledOnce());
	expect(create).toHaveBeenCalledOnce();
	await expect
		.element(page.getByRole('status').getByText('Enabled', { exact: true }))
		.toBeVisible();
});

it('asks before discarding unsaved changes', async () => {
	await preparePageData();
	mockApis();
	const onUpdate = vi.fn();
	const confirm = vi.spyOn(window, 'confirm').mockReturnValue(false);
	await render(FilterForm, { filter: filter(), onUpdate });
	await page.getByLabelText('Name', { exact: true }).fill('Unsaved name');
	await page.getByRole('button', { name: 'Cancel', exact: true }).click();
	expect(confirm).toHaveBeenCalledWith('Discard unsaved filter changes?');
	expect(onUpdate).not.toHaveBeenCalled();
	confirm.mockReturnValue(true);
	await page.getByRole('button', { name: 'Cancel', exact: true }).click();
	expect(onUpdate).toHaveBeenCalledWith(undefined);
});

it('keeps the saved enabled state when disabling fails', async () => {
	await preparePageData();
	const { launch } = mockApis();
	worker.use(
		http.put('/api/mcp-webhook-validations/:id', () =>
			HttpResponse.json({ error: 'Disable unavailable' }, { status: 500 })
		)
	);
	await render(FilterForm, { filter: filter() });
	await page.getByRole('button', { name: 'Disable Filter', exact: true }).click();
	await expect.element(page.getByRole('alert')).toBeVisible();
	await expect
		.element(page.getByRole('status').getByText('Enabled', { exact: true }))
		.toBeVisible();
	await expect
		.element(page.getByRole('button', { name: 'Disable Filter', exact: true }))
		.toBeEnabled();
	expect(launch).not.toHaveBeenCalled();
});

it('shows saved status without editing actions to read-only viewers', async () => {
	await preparePageData();
	await render(FilterForm, { filter: filter(true), readonly: true });
	await expect
		.element(page.getByRole('status').getByText('Disabled', { exact: true }))
		.toBeVisible();
	await expect.element(page.getByRole('status').getByText('Saved', { exact: true })).toBeVisible();
	await expect.element(page.getByLabelText('Name', { exact: true })).toBeDisabled();
	await expect
		.element(page.getByRole('button', { name: 'Enable Filter', exact: true }))
		.not.toBeInTheDocument();
	await expect
		.element(page.getByRole('button', { name: 'Save disabled', exact: true }))
		.not.toBeInTheDocument();
});

it('can leave a newly created filter after activation fails without deleting saved settings', async () => {
	await preparePageData();
	const { create } = mockApis();
	const onCreate = vi.fn();
	const deleted = vi.fn();
	worker.use(
		http.post('/api/mcp-webhook-validations/:id/launch', () =>
			HttpResponse.json({ error: 'Launch failed' }, { status: 500 })
		),
		http.get(
			'/api/mcp-webhook-validations/:id/logs',
			() => new HttpResponse('', { headers: { 'Content-Type': 'text/event-stream' } })
		),
		http.delete('/api/mcp-webhook-validations/:id', () => {
			deleted();
			return HttpResponse.json({});
		})
	);
	await render(FilterForm, { onCreate });
	await page.getByLabelText('Name', { exact: true }).fill('New filter');
	await page.getByLabelText('Webhook URL', { exact: true }).fill('https://example.com/filter');
	await page.getByRole('button', { name: 'Save & enable', exact: true }).click();
	await expect.element(page.getByText('MCP Filter Launch Failed')).toBeVisible();
	await page.getByRole('button', { name: 'Close', exact: true }).click();
	await page.getByRole('button', { name: 'Cancel', exact: true }).click();
	expect(onCreate).toHaveBeenCalledWith(undefined);
	expect(create).toHaveBeenCalledOnce();
	expect(deleted).not.toHaveBeenCalled();
});

it('uses normalized API values for the saved editor and subsequent disable', async () => {
	await preparePageData();
	mockApis();
	const onUpdate = vi.fn();
	const writes = vi.fn();
	worker.use(
		http.put('/api/mcp-webhook-validations/:id', async ({ request }) => {
			const body = (await request.json()) as MCPFilterManifest;
			writes(body);
			return HttpResponse.json({
				...filter(),
				...body,
				name: 'Canonical name',
				selectors: [{ method: '*' }]
			});
		})
	);
	await render(FilterForm, { filter: filter(), onUpdate });
	await page.getByLabelText('Name', { exact: true }).fill('Submitted name');
	await page.getByRole('button', { name: 'Save & apply changes' }).click();
	await vi.waitFor(() => expect(onUpdate).toHaveBeenCalledTimes(1));
	await expect.element(page.getByLabelText('Name', { exact: true })).toHaveValue('Canonical name');
	await expect.element(page.getByLabelText('Method (Optional)', { exact: true })).toHaveValue('*');
	await expect.element(page.getByRole('status').getByText('Saved', { exact: true })).toBeVisible();
	await page.getByLabelText('Name', { exact: true }).fill('Unsubmitted edit');
	await page.getByRole('button', { name: 'Disable Filter', exact: true }).click();
	await vi.waitFor(() => expect(onUpdate).toHaveBeenCalledTimes(2));
	expect(writes).toHaveBeenLastCalledWith(
		expect.objectContaining({
			name: 'Canonical name',
			selectors: [{ method: '*' }],
			disabled: true
		})
	);
	await expect
		.element(page.getByLabelText('Name', { exact: true }))
		.toHaveValue('Unsubmitted edit');
});

it.each([false, true])(
	'removing a saved secret preserves unrelated dirty=%s state',
	async (otherEdits) => {
		await preparePageData();
		mockApis();
		const onUpdate = vi.fn();
		worker.use(
			http.put('/api/mcp-webhook-validations/:id', async ({ request }) =>
				HttpResponse.json({ ...filter(), ...((await request.json()) as object), hasSecret: true })
			),
			http.post(
				'/api/mcp-webhook-validations/:id/deconfigure',
				() => new HttpResponse(null, { status: 204 })
			)
		);
		await render(FilterForm, { filter: filter(), onUpdate });
		await page.getByLabelText('Secret (Optional)', { exact: true }).fill('a-saved-test-secret');
		await page.getByRole('button', { name: 'Save & apply changes' }).click();
		await vi.waitFor(() => expect(onUpdate).toHaveBeenCalledOnce());
		if (otherEdits) await page.getByLabelText('Name', { exact: true }).fill('Unsaved name');
		await page.getByRole('button', { name: 'Remove Secret', exact: true }).click();
		await expect
			.element(page.getByRole('button', { name: 'Remove Secret', exact: true }))
			.not.toBeInTheDocument();
		await expect
			.element(
				page
					.getByRole('status')
					.getByText(otherEdits ? 'Unsaved changes' : 'Saved', { exact: true })
			)
			.toBeVisible();
	}
);

it('uses the final configure response and retains separately saved credential values', async () => {
	await preparePageData();
	mockApis();
	const onUpdate = vi.fn();
	const writes = vi.fn();
	const configured = vi.fn();
	const initial = filter();
	initial.url = '';
	initial.toolName = 'check';
	initial.mcpServerManifest = {
		runtime: 'containerized',
		containerizedConfig: { image: 'old-image', port: 8080, path: '/mcp' },
		config: [
			{
				key: 'TOKEN',
				name: 'Token',
				value: '',
				usage: 'env',
				required: false,
				sensitive: true,
				description: ''
			}
		]
	};
	let saved = initial;
	worker.use(
		http.post('/api/mcp-webhook-validations/:id/reveal', () =>
			HttpResponse.json({ TOKEN: 'saved-token' })
		),
		http.put('/api/mcp-webhook-validations/:id', async ({ request }) => {
			const body = (await request.json()) as MCPFilterManifest;
			writes(body);
			saved = { ...initial, ...body };
			return HttpResponse.json(saved);
		}),
		http.post('/api/mcp-webhook-validations/:id/configure', async ({ request }) => {
			configured(await request.json());
			saved = {
				...saved,
				mcpServerManifest: {
					...saved.mcpServerManifest!,
					containerizedConfig: { image: 'canonical-image', port: 8080, path: '/mcp' }
				}
			};
			return HttpResponse.json(saved);
		})
	);
	await render(FilterForm, { filter: initial, onUpdate });
	await expect.element(page.getByLabelText('Image', { exact: true })).toBeEnabled();
	await page.getByLabelText('Name', { exact: true }).fill('Updated filter');
	await page.getByRole('button', { name: 'Save & apply changes' }).click();
	await vi.waitFor(() => expect(onUpdate).toHaveBeenCalledTimes(1));
	expect(configured).toHaveBeenCalledWith({ TOKEN: 'saved-token' });
	await expect
		.element(page.getByLabelText('Image', { exact: true }))
		.toHaveValue('canonical-image');
	await expect.element(page.getByRole('status').getByText('Saved', { exact: true })).toBeVisible();
	await page.getByRole('button', { name: 'Disable Filter', exact: true }).click();
	await vi.waitFor(() => expect(onUpdate).toHaveBeenCalledTimes(2));
	expect(writes).toHaveBeenLastCalledWith(
		expect.objectContaining({
			disabled: true,
			mcpServerManifest: expect.objectContaining({
				containerizedConfig: expect.objectContaining({ image: 'canonical-image' })
			})
		})
	);
});
