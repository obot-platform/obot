import type { GuardrailPolicy } from '$lib/services/llm-gateway/guardrails';
import { worker } from '../../../tests/mocks/worker';
import GuardrailPolicyEditor from './GuardrailPolicyEditor.svelte';
import { http, HttpResponse } from 'msw';
import { describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

function mockGuardrailsAndProviders() {
	worker.use(
		http.get('/api/llm-gateway/guardrails', () =>
			HttpResponse.json({
				items: [
					{ id: 'p1', name: 'Common PII', type: 'regex', config: {}, configuredSecrets: [] },
					{ id: 'p2', name: 'Content safety', type: 'azure-content-safety', config: {} }
				]
			})
		),
		http.get('/api/llm-gateway/guardrails/providers', () =>
			HttpResponse.json({
				items: [
					{
						type: 'regex',
						label: 'Custom Regex',
						description: '',
						category: 'Built-in',
						fields: [],
						redacts: true
					},
					{
						type: 'azure-content-safety',
						label: 'Azure AI Content Safety',
						description: '',
						category: 'Integrations',
						fields: [],
						redacts: false
					}
				]
			})
		),
		http.post('/api/llm-gateway/guardrails/validate-condition', async ({ request }) => {
			const { condition } = (await request.json()) as { condition: string };
			return HttpResponse.json(
				condition.includes('bogus')
					? { valid: false, error: "undeclared reference to 'bogus'" }
					: { valid: true }
			);
		})
	);
}

const redacting: GuardrailPolicy = {
	id: 'r2',
	name: 'Redact everything',
	enabled: true,
	target: 'llm',
	applyTo: 'output',
	action: 'redact',
	samplingRate: 50,
	failClosed: true,
	guardrailIDs: ['p1', 'p2'],
	profileID: 'gp1'
};

describe('Applied guardrail editor', () => {
	it('builds a model guardrail policy from the selected guardrails and condition', async () => {
		mockGuardrailsAndProviders();
		const onSave = vi.fn();
		render(GuardrailPolicyEditor, { onSave, onCancel: vi.fn() });

		const apply = page.getByRole('button', { name: 'Apply' });
		await page.getByLabelText('Name', { exact: true }).fill('Block PII');
		await expect.element(apply).toBeDisabled();
		// New ones start enabled, so there's no toggle.
		await expect.element(page.getByRole('switch', { name: 'Enabled' })).not.toBeInTheDocument();
		await page.getByText('Common PII').click();
		await page.getByLabelText('Condition').fill('model.startsWith("openai/")');
		await expect.element(apply).toBeEnabled();
		await apply.click();

		expect(onSave).toHaveBeenCalledWith(
			expect.objectContaining({
				name: 'Block PII',
				enabled: true,
				target: 'llm',
				applyTo: 'both',
				action: 'block',
				condition: 'model.startsWith("openai/")',
				samplingRate: 100,
				inputScope: 'latest',
				failClosed: false,
				guardrailIDs: ['p1']
			}),
			expect.any(Array)
		);
	});

	it('shows condition errors as the condition is typed', async () => {
		mockGuardrailsAndProviders();
		render(GuardrailPolicyEditor, { onSave: vi.fn(), onCancel: vi.fn() });

		await page.getByLabelText('Name', { exact: true }).fill('Bad condition');
		await page.getByText('Common PII').click();
		await page.getByLabelText('Condition').fill('bogus == 1');

		await expect
			.element(page.getByRole('alert'))
			.toHaveTextContent("undeclared reference to 'bogus'");
		await expect.element(page.getByRole('button', { name: 'Apply' })).toBeDisabled();
	});

	it('creates a guardrail in place and selects it', async () => {
		mockGuardrailsAndProviders();
		const created = vi.fn();
		worker.use(
			http.post('/api/llm-gateway/guardrails', async ({ request }) => {
				const body = (await request.json()) as Record<string, unknown>;
				created(body);
				return HttpResponse.json({ ...body, id: 'p3' });
			})
		);
		render(GuardrailPolicyEditor, { onSave: vi.fn(), onCancel: vi.fn() });

		await page.getByRole('button', { name: 'New guardrail' }).click();
		await page.getByRole('button', { name: /Custom Regex/ }).click();
		const dialog = page.getByRole('dialog');
		await dialog.getByLabelText('Name', { exact: true }).fill('Card numbers');
		await dialog.getByRole('button', { name: 'Save', exact: true }).click();

		await vi.waitFor(() =>
			expect(created).toHaveBeenCalledWith(
				expect.objectContaining({ name: 'Card numbers', type: 'regex' })
			)
		);
		await expect.element(page.getByRole('checkbox', { name: /Card numbers/ })).toBeChecked();
	});

	it('loads a saved one, and warns when redacting uses a guardrail that cannot redact', async () => {
		mockGuardrailsAndProviders();
		const onSave = vi.fn();
		render(GuardrailPolicyEditor, { policy: redacting, onSave, onCancel: vi.fn() });

		await expect
			.element(page.getByLabelText('Name', { exact: true }))
			.toHaveValue('Redact everything');
		await expect.element(page.getByRole('switch', { name: 'Enabled' })).toBeChecked();
		await expect.element(page.getByLabelText('Sampling rate (%)')).toHaveValue(50);
		await expect.element(page.getByRole('combobox', { name: 'Profile' })).not.toBeInTheDocument();
		// Input scope only matters when input is checked.
		await expect.element(page.getByLabelText('Input scope')).not.toBeInTheDocument();

		const warning = page.getByText('locate findings for redaction', { exact: false });
		await expect.element(warning).toBeInTheDocument();
		await page.getByRole('combobox', { name: 'Action' }).click();
		await page.getByRole('button', { name: 'Block', exact: true }).click();
		await expect.element(warning).not.toBeInTheDocument();

		await page.getByRole('button', { name: 'Apply' }).click();
		expect(onSave).toHaveBeenCalledWith(
			expect.objectContaining({ id: 'r2', action: 'block', profileID: 'gp1' }),
			expect.any(Array)
		);
	});

	it('links to the guardrails list', async () => {
		mockGuardrailsAndProviders();
		render(GuardrailPolicyEditor, { onSave: vi.fn(), onCancel: vi.fn() });

		await expect.element(page.getByText('mcp_tool', { exact: true })).not.toBeInTheDocument();
		await expect
			.element(page.getByRole('link', { name: 'Manage guardrails' }))
			.toHaveAttribute('href', '/models?view=guardrails');
	});
});
