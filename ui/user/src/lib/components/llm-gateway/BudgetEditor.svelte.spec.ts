import type { Model } from '$lib/services';
import { worker } from '../../../tests/mocks/worker';
import BudgetEditor from './BudgetEditor.svelte';
import { http, HttpResponse } from 'msw';
import { describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

const models = [
	{ id: 'm1-opus', name: 'opus', targetModel: 'anthropic/claude-opus-4-5', usage: 'llm' },
	{ id: 'm1-haiku', name: 'haiku', targetModel: 'anthropic/claude-haiku-4-5', usage: 'llm' }
] as Model[];

describe('Budget editor', () => {
	it('starts a new budget enabled with a monthly spend limit', async () => {
		const onSave = vi.fn();
		render(BudgetEditor, { models, onSave, onCancel: vi.fn() });

		// New budgets start enabled, so there's no toggle.
		await expect.element(page.getByRole('switch', { name: 'Enabled' })).not.toBeInTheDocument();
		await page.getByLabelText('Name', { exact: true }).fill('Engineering');
		await page.getByLabelText('Limit', { exact: true }).fill('25');
		await page.getByRole('button', { name: 'Apply' }).click();

		expect(onSave).toHaveBeenCalledWith(
			expect.objectContaining({
				name: 'Engineering',
				enabled: true,
				perUser: true,
				limits: [{ metric: 'spend', amount: 25, window: '1M', calendar: true }],
				models: []
			})
		);
	});

	it('says why a negative or empty limit cannot be applied', async () => {
		render(BudgetEditor, { models, onSave: vi.fn(), onCancel: vi.fn() });

		await page.getByLabelText('Name', { exact: true }).fill('Engineering');
		const limit = page.getByLabelText('Limit', { exact: true });
		await limit.fill('-5');
		await expect.element(page.getByRole('alert')).toHaveTextContent('Enter a limit of 0 or more.');
		await expect.element(limit).toHaveAttribute('aria-invalid', 'true');
		await expect.element(page.getByRole('button', { name: 'Apply' })).toBeDisabled();
		await limit.fill('');
		await expect.element(page.getByRole('alert')).toHaveTextContent('Enter a limit of 0 or more.');
		await limit.fill('0');
		await expect.element(page.getByRole('alert')).not.toBeInTheDocument();
		await expect.element(page.getByRole('button', { name: 'Apply' })).toBeEnabled();
	});

	it('needs a model when limited to specific models, and drops models the profile no longer grants', async () => {
		const onSave = vi.fn();
		render(BudgetEditor, {
			budget: {
				name: 'Opus',
				enabled: true,
				perUser: true,
				exemptAdmins: false,
				limits: [{ metric: 'spend', amount: 10, window: '1M', calendar: true }],
				models: ['m1-opus', 'm-gone'],
				profileID: 'p1'
			},
			models,
			onSave,
			onCancel: vi.fn()
		});

		await expect.element(page.getByText('1 selected')).toBeVisible();
		await page.getByText('anthropic/claude-opus-4-5').click();
		await expect.element(page.getByText('Select at least one model.')).toBeVisible();
		await expect.element(page.getByRole('button', { name: 'Apply' })).toBeDisabled();

		await page.getByText('anthropic/claude-haiku-4-5').click();
		await page.getByRole('button', { name: 'Apply' }).click();
		expect(onSave).toHaveBeenCalledWith(
			expect.objectContaining({ models: ['m1-haiku'], profileID: 'p1' })
		);
	});

	it('says when the profile grants no models', async () => {
		render(BudgetEditor, { models: [], onSave: vi.fn(), onCancel: vi.fn() });

		await page.getByText('Specific models').click();
		await expect.element(page.getByText("This profile doesn't grant any models.")).toBeVisible();
	});

	it('saves a rolling token limit chosen with the dropdowns', async () => {
		const onSave = vi.fn();
		render(BudgetEditor, { models, onSave, onCancel: vi.fn() });

		await page.getByLabelText('Name', { exact: true }).fill('Tokens');
		await page.getByRole('combobox', { name: 'Metric' }).click();
		await page.getByRole('button', { name: 'Total tokens', exact: true }).click();
		await page.getByLabelText('Limit', { exact: true }).fill('5000');
		await page.getByRole('combobox', { name: 'Window type' }).click();
		await page.getByRole('button', { name: 'Rolling', exact: true }).click();
		await page.getByLabelText('Window length').fill('2');
		await page.getByRole('combobox', { name: 'Window unit' }).click();
		await page.getByRole('button', { name: 'days', exact: true }).click();
		await page.getByRole('button', { name: 'Apply' }).click();

		expect(onSave).toHaveBeenCalledWith(
			expect.objectContaining({
				limits: [{ metric: 'tokens', amount: 5000, window: '2d', calendar: false }]
			})
		);
	});

	it('shows a saved shared budget with its current usage', async () => {
		worker.use(
			http.get('/api/llm-gateway/budgets/b2/usage', () =>
				HttpResponse.json({ items: [{ used: [250] }] })
			)
		);
		render(BudgetEditor, {
			budget: {
				id: 'b2',
				name: 'Sales',
				enabled: true,
				perUser: false,
				exemptAdmins: false,
				limits: [{ metric: 'tokens', amount: 1000, window: '6h' }],
				profileID: 'p2'
			},
			models,
			onSave: vi.fn(),
			onCancel: vi.fn()
		});

		await expect.element(page.getByLabelText('Name', { exact: true })).toHaveValue('Sales');
		await expect.element(page.getByRole('switch', { name: 'Enabled' })).toBeChecked();
		await expect.element(page.getByText('Everyone (shared)')).toBeInTheDocument();
		await expect.element(page.getByText('250 / 1K')).toBeInTheDocument();
		await expect.element(page.getByLabelText('Window length')).toHaveValue(6);
	});

	it('is read-only for auditors', async () => {
		render(BudgetEditor, { models, readonly: true, onSave: vi.fn(), onCancel: vi.fn() });

		await expect.element(page.getByLabelText('Name', { exact: true })).toBeDisabled();
		await expect.element(page.getByRole('button', { name: 'Apply' })).not.toBeInTheDocument();
		await expect.element(page.getByRole('button', { name: 'Add limit' })).not.toBeInTheDocument();
	});
});
