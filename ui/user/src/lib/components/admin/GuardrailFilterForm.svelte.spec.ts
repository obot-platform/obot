import type {
	MCPFilter,
	MCPFilterManifest,
	MCPGuardrail,
	MCPGuardrailProvider
} from '$lib/services';
import { worker } from '../../../tests/mocks/worker';
import GuardrailFilterForm from './GuardrailFilterForm.svelte';
import { http, HttpResponse } from 'msw';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

const guardrails: MCPGuardrail[] = [
	{ id: 'gp1secrets', name: 'Secrets scanner', type: 'secrets' },
	{ id: 'gp1moderation', name: 'Moderation', type: 'moderation' }
];

const providers: MCPGuardrailProvider[] = [
	{ type: 'secrets', label: 'Secrets detection', redacts: true },
	{ type: 'moderation', label: 'Content moderation', redacts: false }
];

function createGuardrailFilter(overrides: Partial<MCPFilter> = {}): MCPFilter {
	return {
		id: 'mwv1guardrail',
		created: '2026-01-01T00:00:00Z',
		type: 'mcpwebhookvalidation',
		hasSecret: false,
		configured: true,
		name: 'Block secrets',
		resources: [{ id: 'default', type: 'mcpCatalog' }],
		selectors: [],
		disabled: false,
		guardrail: {
			guardrailIDs: ['gp1secrets'],
			action: 'redact',
			direction: 'response',
			failClosed: true,
			samplingRate: 50,
			subjects: [],
			condition: '',
			blockMessage: ''
		},
		...overrides
	};
}

beforeEach(() => {
	worker.use(
		http.get('/api/llm-gateway/guardrails', () => HttpResponse.json({ items: guardrails })),
		http.get('/api/llm-gateway/guardrails/providers', () => HttpResponse.json({ items: providers }))
	);
});

async function save() {
	await page.getByRole('button', { name: 'Save', exact: true }).click();
}

describe('GuardrailFilterForm.svelte', () => {
	it('lists guardrails with their type label and links to guardrail management', async () => {
		await render(GuardrailFilterForm, {});

		await expect
			.element(page.getByRole('heading', { name: 'Create Guardrail Filter' }))
			.toBeVisible();
		await expect.element(page.getByRole('checkbox', { name: /Secrets scanner/ })).toBeVisible();
		await expect.element(page.getByText('Secrets detection')).toBeVisible();
		await expect.element(page.getByText('Content moderation')).toBeVisible();
		await expect
			.element(page.getByRole('link', { name: 'Manage guardrails' }))
			.toHaveAttribute('href', expect.stringContaining('/models?view=guardrails'));
	});

	it('creates a guardrail in place and selects it', async () => {
		const created = vi.fn();
		worker.use(
			http.get('/api/llm-gateway/guardrails/providers', () =>
				HttpResponse.json({
					items: [
						{
							type: 'secrets',
							label: 'Secrets detection',
							description: 'Finds credentials.',
							category: 'Built-in',
							fields: [],
							redacts: true
						}
					]
				})
			),
			http.post('/api/llm-gateway/guardrails', async ({ request }) => {
				const body = (await request.json()) as Record<string, unknown>;
				created(body);
				return HttpResponse.json({ ...body, id: 'gp1new' });
			})
		);
		await render(GuardrailFilterForm, {});

		await page.getByRole('button', { name: 'New guardrail' }).click();
		await page.getByRole('button', { name: /Secrets detection/ }).click();
		const dialog = page.getByRole('dialog');
		await dialog.getByLabelText('Name', { exact: true }).fill('More secrets');
		await dialog.getByRole('button', { name: 'Save', exact: true }).click();

		await vi.waitFor(() =>
			expect(created).toHaveBeenCalledWith(
				expect.objectContaining({ name: 'More secrets', type: 'secrets' })
			)
		);
		await expect.element(page.getByRole('checkbox', { name: /More secrets/ })).toBeChecked();
	});

	it('does not show server, runtime, URL, secret or tool name fields', async () => {
		await render(GuardrailFilterForm, {});

		await expect.element(page.getByRole('checkbox', { name: /Secrets scanner/ })).toBeVisible();
		await expect.element(page.getByLabelText('Webhook URL')).not.toBeInTheDocument();
		await expect.element(page.getByLabelText('Secret (Optional)')).not.toBeInTheDocument();
		await expect.element(page.getByLabelText('Tool Name')).not.toBeInTheDocument();
		await expect.element(page.getByText('Enable Mutable Response')).not.toBeInTheDocument();
	});

	it('requires a name and at least one guardrail', async () => {
		const create = vi.fn();
		worker.use(
			http.post('/api/mcp-webhook-validations', () => {
				create();
				return HttpResponse.json({});
			})
		);
		await render(GuardrailFilterForm, {});

		await save();
		await expect.element(page.getByText('Name is required')).toBeVisible();
		await expect.element(page.getByText('Select at least one guardrail')).toBeVisible();

		await page.getByLabelText('Name').fill('My guardrail');
		await save();
		await expect.element(page.getByText('Name is required')).not.toBeInTheDocument();
		await expect.element(page.getByText('Select at least one guardrail')).toBeVisible();
		expect(create).not.toHaveBeenCalled();
	});

	it('creates a guardrail filter with only guardrail fields', async () => {
		let body: MCPFilterManifest | undefined;
		worker.use(
			http.post('/api/mcp-webhook-validations', async ({ request }) => {
				body = (await request.json()) as MCPFilterManifest;
				return HttpResponse.json(createGuardrailFilter({ name: body.name }));
			})
		);
		const onCreate = vi.fn();
		await render(GuardrailFilterForm, { onCreate });

		await page.getByLabelText('Name').fill('  Secrets  ');
		await page.getByRole('checkbox', { name: /Secrets scanner/ }).click();
		await page.getByRole('combobox', { name: 'Action' }).click();
		await page.getByRole('button', { name: 'Block', exact: true }).click();
		await page.getByRole('combobox', { name: 'Direction' }).click();
		await page.getByRole('button', { name: 'Requests (tool call arguments)', exact: true }).click();
		await page.getByLabelText('Sampling rate (%)').fill('25');
		await page.getByLabelText('Block Message (Optional)').fill('Secrets are not allowed');
		await save();

		await vi.waitFor(() => expect(onCreate).toHaveBeenCalled());
		expect(body).toEqual({
			name: 'Secrets',
			resources: [{ id: 'default', type: 'mcpCatalog' }],
			disabled: false,
			guardrail: {
				guardrailIDs: ['gp1secrets'],
				action: 'block',
				direction: 'request',
				failClosed: false,
				samplingRate: 25,
				subjects: [],
				blockMessage: 'Secrets are not allowed'
			}
		});
		expect(body).not.toHaveProperty('url');
		expect(body).not.toHaveProperty('toolName');
		expect(body).not.toHaveProperty('mcpServerManifest');
		expect(body).not.toHaveProperty('systemMCPServerCatalogEntryID');
	});

	it('rejects sampling rates outside 1-100', async () => {
		await render(GuardrailFilterForm, {});

		await page.getByLabelText('Name').fill('Sampled');
		await page.getByRole('checkbox', { name: /Secrets scanner/ }).click();
		await page.getByLabelText('Sampling rate (%)').fill('0');
		await save();

		await expect.element(page.getByText('Sampling rate must be between 1 and 100')).toBeVisible();
	});

	it('validates the condition against the MCP target', async () => {
		const requests: { target: string; condition: string }[] = [];
		worker.use(
			http.post('/api/llm-gateway/guardrails/validate-condition', async ({ request }) => {
				const req = (await request.json()) as { target: string; condition: string };
				requests.push(req);
				return HttpResponse.json({ valid: false, error: 'undeclared reference to "foo"' });
			})
		);
		await render(GuardrailFilterForm, {});

		await page.getByRole('textbox', { name: 'Condition' }).fill('foo == 1');

		await expect.element(page.getByText('undeclared reference to "foo"')).toBeVisible();
		expect(requests.at(-1)).toEqual({ target: 'mcp', condition: 'foo == 1' });
		await expect.element(page.getByRole('button', { name: 'Save', exact: true })).toBeDisabled();
	});

	it('warns when redacting with a guardrail that cannot redact', async () => {
		await render(GuardrailFilterForm, {});

		await page.getByRole('checkbox', { name: /Moderation/ }).click();
		await expect.element(page.getByText(/locate findings for redaction/)).not.toBeInTheDocument();

		await page.getByRole('combobox', { name: 'Action' }).click();
		await page.getByRole('button', { name: 'Redact', exact: true }).click();
		await expect.element(page.getByText(/locate findings for redaction/)).toBeVisible();
	});

	it('loads an existing guardrail filter and updates it without calling server endpoints', async () => {
		const reveal = vi.fn();
		let body: MCPFilterManifest | undefined;
		const filter = createGuardrailFilter();
		worker.use(
			http.post('/api/mcp-webhook-validations/:id/reveal', () => {
				reveal();
				return HttpResponse.json({});
			}),
			http.put('/api/mcp-webhook-validations/:id', async ({ request }) => {
				body = (await request.json()) as MCPFilterManifest;
				return HttpResponse.json(filter);
			})
		);
		const onUpdate = vi.fn();
		await render(GuardrailFilterForm, { filter, onUpdate });

		await expect.element(page.getByLabelText('Name')).toHaveValue('Block secrets');
		await expect.element(page.getByRole('checkbox', { name: /Secrets scanner/ })).toBeChecked();
		await expect.element(page.getByRole('checkbox', { name: /Moderation/ })).not.toBeChecked();
		await expect
			.element(page.getByRole('combobox', { name: 'Action' }))
			.toHaveTextContent('Redact');
		await expect
			.element(page.getByRole('combobox', { name: 'Direction' }))
			.toHaveTextContent('Responses (tool results)');
		await expect.element(page.getByLabelText('Sampling rate (%)')).toHaveValue(50);

		await save();
		await vi.waitFor(() => expect(onUpdate).toHaveBeenCalled());
		expect(body?.guardrail).toEqual({
			guardrailIDs: ['gp1secrets'],
			action: 'redact',
			direction: 'response',
			failClosed: true,
			samplingRate: 50,
			subjects: []
		});
		expect(reveal).not.toHaveBeenCalled();
	});
});
