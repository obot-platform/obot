import { Group } from '$lib/services';
import type { ModelAccessPolicy } from '$lib/services/admin/types';
import { goto, replaceState } from '$lib/url';
import { createMockProfile, preparePageData } from '../../../tests/helpers/pageData';
import { worker } from '../../../tests/mocks/worker';
import ProfileForm from './ProfileForm.svelte';
import { http, HttpResponse } from 'msw';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

vi.mock(import('$lib/url'), async (importOriginal) => ({
	...(await importOriginal()),
	goto: vi.fn(),
	replaceState: vi.fn()
}));

const gatewayModel = (id: string, target: string) => ({
	id,
	name: id,
	targetModel: target,
	modelProvider: 'llm-gateway-model-provider',
	modelProviderName: 'LLM Gateway',
	usage: 'llm'
});

const existing: ModelAccessPolicy = {
	id: 'p1',
	created: '2026-01-01T00:00:00Z',
	displayName: 'Engineering',
	subjects: [{ type: 'selector', id: '*' }],
	models: [{ id: 'm-opus' }, { id: 'm-haiku' }]
};

type Calls = ReturnType<typeof vi.fn<(method: string, path: string, body: unknown) => void>>;

// Mocks the gateway API. Each write is recorded as [method, path, body] on `calls`, in order.
function mockGateway(opts: { failBudgetCreate?: boolean } = {}) {
	const calls: Calls = vi.fn();
	const record =
		(status = 200, reply?: (body: Record<string, unknown>) => object) =>
		async ({ request, params }: { request: Request; params: Record<string, unknown> }) => {
			const body = request.method === 'DELETE' ? undefined : await request.json();
			calls(request.method, new URL(request.url).pathname, body);
			if (status >= 400) return HttpResponse.json({ error: 'limit is too high' }, { status });
			if (request.method === 'DELETE') return new HttpResponse(null, { status: 204 });
			return HttpResponse.json(
				reply ? reply(body as Record<string, unknown>) : { ...body, id: params.id ?? 'new-id' }
			);
		};
	worker.use(
		http.get('/api/models', () =>
			HttpResponse.json({
				items: [
					gatewayModel('m-opus', 'anthropic/claude-opus-4-5'),
					gatewayModel('m-haiku', 'anthropic/claude-haiku-4-5')
				]
			})
		),
		http.get('/api/llm-gateway/budgets', () =>
			HttpResponse.json({
				items: [
					{
						id: 'b1',
						name: 'Monthly spend',
						enabled: true,
						perUser: true,
						exemptAdmins: false,
						limits: [{ metric: 'spend', amount: 50, window: '1M', calendar: true }],
						profileID: 'p1'
					},
					{
						id: 'b2',
						name: 'Daily tokens',
						enabled: true,
						perUser: false,
						exemptAdmins: false,
						limits: [{ metric: 'tokens', amount: 1000, window: '1d' }],
						profileID: 'p1'
					}
				]
			})
		),
		http.get('/api/llm-gateway/budgets/:id/usage', () => HttpResponse.json({ items: [] })),
		http.get('/api/llm-gateway/guardrail-policies', () => HttpResponse.json({ items: [] })),
		http.get('/api/llm-gateway/guardrails', () =>
			HttpResponse.json({
				items: [{ id: 'g1', name: 'Common PII', type: 'regex', config: {} }]
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
					}
				]
			})
		),
		http.post(
			'/api/model-access-policies',
			record(200, (body) => ({ ...body, id: 'p-new' }))
		),
		http.put('/api/model-access-policies/:id', record()),
		http.post('/api/llm-gateway/budgets', record(opts.failBudgetCreate ? 400 : 200)),
		http.put('/api/llm-gateway/budgets/:id', record()),
		http.delete('/api/llm-gateway/budgets/:id', record()),
		http.post('/api/llm-gateway/guardrail-policies', record())
	);
	return { calls };
}

async function addEveryone() {
	await page.getByRole('button', { name: 'Add User/Group' }).click();
	await page.getByRole('button', { name: /All Obot Users/ }).click();
	await page.getByRole('button', { name: 'Confirm' }).click();
}

async function addBudget(name: string) {
	await page.getByRole('button', { name: 'Add Budget' }).click();
	const dialog = page.getByRole('dialog', { name: 'Add Budget' });
	await dialog.getByLabelText('Name', { exact: true }).fill(name);
	await dialog.getByRole('button', { name: 'Apply' }).click();
	await expect.element(dialog).not.toBeInTheDocument();
}

async function addGuardrail(name: string) {
	await page.getByRole('button', { name: 'Add Guardrail' }).click();
	const dialog = page.getByRole('dialog', { name: 'Add Guardrail' });
	await dialog.getByLabelText('Name', { exact: true }).fill(name);
	await dialog.getByText('Common PII').click();
	await dialog.getByRole('button', { name: 'Apply' }).click();
	await expect.element(dialog).not.toBeInTheDocument();
}

beforeEach(async () => {
	vi.mocked(goto).mockClear();
	vi.mocked(replaceState).mockClear();
	await preparePageData({ profile: createMockProfile([Group.ADMIN]) });
});

describe('Profile form', () => {
	it('creates a profile with a budget and an applied guardrail in one save', async () => {
		const { calls } = mockGateway();
		render(ProfileForm, {});

		await page.getByLabelText('Name', { exact: true }).fill('Sales');
		await addEveryone();
		await addBudget('Monthly spend');
		await addGuardrail('Block PII');
		// Neither is saved until the profile is.
		expect(calls).not.toHaveBeenCalled();
		await expect.element(page.getByText('Monthly spend')).toBeVisible();
		await expect.element(page.getByText('Block PII')).toBeVisible();

		await page.getByRole('button', { name: 'Save', exact: true }).click();

		await vi.waitFor(() => expect(goto).toHaveBeenCalledWith('/models?view=profiles'));
		const writes = calls.mock.calls;
		expect(writes.map(([method, path]) => `${method} ${path}`)).toEqual([
			'POST /api/model-access-policies',
			'POST /api/llm-gateway/budgets',
			'POST /api/llm-gateway/guardrail-policies'
		]);
		expect(writes[0][2]).toMatchObject({
			displayName: 'Sales',
			subjects: [{ type: 'selector', id: '*' }]
		});
		expect(writes[1][2]).toMatchObject({
			name: 'Monthly spend',
			enabled: true,
			perUser: true,
			limits: [{ metric: 'spend', amount: 50, window: '1M', calendar: true }],
			models: [],
			profileID: 'p-new'
		});
		expect(writes[2][2]).toMatchObject({
			name: 'Block PII',
			target: 'llm',
			guardrailIDs: ['g1'],
			profileID: 'p-new'
		});
	});

	it('stays on the new profile and keeps what failed when a budget cannot be saved', async () => {
		const { calls } = mockGateway({ failBudgetCreate: true });
		render(ProfileForm, {});

		await page.getByLabelText('Name', { exact: true }).fill('Sales');
		await addEveryone();
		await addBudget('Too much');
		await page.getByRole('button', { name: 'Save', exact: true }).click();

		await expect
			.element(page.getByText('The profile was saved, but some of its budgets', { exact: false }))
			.toBeVisible();
		await expect.element(page.getByText('Too much: limit is too high')).toBeVisible();
		expect(replaceState).toHaveBeenCalledWith('/models/profiles/p-new', {});
		expect(goto).not.toHaveBeenCalled();
		// The budget is still in the form, and saving again retries only it.
		await expect.element(page.getByRole('button', { name: 'Edit Too much' })).toBeVisible();
		await expect.element(page.getByRole('heading', { name: 'Edit Profile' })).toBeVisible();
		calls.mockClear();
		await page.getByRole('button', { name: 'Save', exact: true }).click();
		await vi.waitFor(() => expect(calls).toHaveBeenCalled());
		expect(calls.mock.calls.map(([method, path]) => `${method} ${path}`)).toEqual([
			'POST /api/llm-gateway/budgets'
		]);
	});

	it('edits and deletes budgets within the form', async () => {
		const { calls } = mockGateway();
		render(ProfileForm, { modelAccessPolicy: existing });

		await expect.element(page.getByText('Daily tokens')).toBeVisible();
		await page.getByRole('button', { name: 'Edit Monthly spend' }).click();
		const dialog = page.getByRole('dialog', { name: 'Edit Budget' });
		// Saved budgets can be turned off.
		await expect.element(dialog.getByRole('switch', { name: 'Enabled' })).toBeChecked();
		await dialog.getByLabelText('Limit', { exact: true }).fill('75');
		await dialog.getByRole('button', { name: 'Apply' }).click();

		const daily = page.getByRole('button', { name: 'Edit Daily tokens' });
		const row = daily.element().closest('li') as HTMLElement;
		(row.querySelector('button[aria-label="Delete budget"]') as HTMLElement).click();
		await expect.element(daily).not.toBeInTheDocument();
		expect(calls).not.toHaveBeenCalled();

		await page.getByRole('button', { name: 'Save', exact: true }).click();
		await vi.waitFor(() => expect(goto).toHaveBeenCalledWith('/models?view=profiles'));
		const writes = calls.mock.calls;
		// The profile itself didn't change, so only the budgets are written.
		expect(writes.map(([method, path]) => `${method} ${path}`)).toEqual([
			'DELETE /api/llm-gateway/budgets/b2',
			'PUT /api/llm-gateway/budgets/b1'
		]);
		expect(writes[1][2]).toMatchObject({
			name: 'Monthly spend',
			limits: [{ metric: 'spend', amount: 75, window: '1M', calendar: true }],
			profileID: 'p1'
		});
	});

	it("ties budgets to the profile: no profile choice, and only the form's models", async () => {
		const { calls } = mockGateway();
		render(ProfileForm, { modelAccessPolicy: existing });

		await page.getByRole('button', { name: 'Add Budget' }).click();
		let dialog = page.getByRole('dialog', { name: 'Add Budget' });
		await expect.element(dialog.getByLabelText('Name', { exact: true })).toBeVisible();
		// New budgets start enabled, so there's no toggle.
		await expect.element(dialog.getByRole('switch', { name: 'Enabled' })).not.toBeInTheDocument();
		await expect.element(page.getByRole('combobox', { name: 'Profile' })).not.toBeInTheDocument();
		await dialog.getByText('Specific models').click();
		await expect.element(dialog.getByText('anthropic/claude-opus-4-5')).toBeVisible();
		await expect.element(dialog.getByText('anthropic/claude-haiku-4-5')).toBeVisible();
		await dialog.getByLabelText('Name', { exact: true }).fill('Opus only');
		await dialog.getByText('anthropic/claude-opus-4-5').click();
		await dialog.getByRole('button', { name: 'Apply' }).click();

		// Removing Opus from the profile leaves the budget without models, which blocks saving.
		await expect.element(page.getByText('Opus only')).toBeVisible();
		const opusRow = page
			.getByText('m-opus', { exact: true })
			.element()
			.closest('li') as HTMLElement;
		(opusRow.querySelector('button[aria-label="Remove Model"]') as HTMLElement).click();
		await vi.waitFor(() =>
			expect(document.querySelector('[role=alert]')?.textContent).toContain(
				"This profile no longer grants any of this budget's models."
			)
		);
		await expect.element(page.getByRole('button', { name: 'Save', exact: true })).toBeDisabled();

		// The budget's model picker follows the form's models.
		await page.getByRole('button', { name: 'Edit Opus only' }).click();
		dialog = page.getByRole('dialog', { name: 'Edit Budget' });
		await expect.element(dialog.getByText('anthropic/claude-haiku-4-5')).toBeVisible();
		await expect.element(dialog.getByText('anthropic/claude-opus-4-5')).not.toBeInTheDocument();
		await expect.element(dialog.getByText('Select at least one model.')).toBeVisible();
		await expect.element(dialog.getByRole('button', { name: 'Apply' })).toBeDisabled();
		expect(calls).not.toHaveBeenCalled();
	});

	it('summarizes budgets in words', async () => {
		mockGateway();
		render(ProfileForm, { modelAccessPolicy: existing });

		const row = page.getByRole('button', { name: 'Edit Monthly spend' });
		await expect.element(row.getByText(/\$50\.00 spend per/)).toBeVisible();
		await expect
			.element(
				page.getByRole('button', { name: 'Edit Daily tokens' }).getByText(/1K total tokens per/)
			)
			.toBeVisible();
	});

	it("counts a budget's models without those the profile no longer grants", async () => {
		mockGateway();
		worker.use(
			http.get('/api/llm-gateway/budgets', () =>
				HttpResponse.json({
					items: [
						{
							id: 'b3',
							name: 'Two models',
							enabled: true,
							perUser: true,
							exemptAdmins: false,
							limits: [{ metric: 'requests', amount: 10, window: '1d', calendar: true }],
							models: ['m-opus', 'm-haiku'],
							profileID: 'p1'
						}
					]
				})
			)
		);
		render(ProfileForm, { modelAccessPolicy: existing });

		const row = page.getByRole('button', { name: 'Edit Two models' });
		await expect.element(row.getByText(/2 models$/)).toBeVisible();
		const opusRow = page
			.getByText('m-opus', { exact: true })
			.element()
			.closest('li') as HTMLElement;
		(opusRow.querySelector('button[aria-label="Remove Model"]') as HTMLElement).click();
		await expect.element(row.getByText(/1 model$/)).toBeVisible();
	});

	it('shows the profile read-only to auditors', async () => {
		mockGateway();
		await preparePageData({ profile: createMockProfile([Group.USER, Group.AUDITOR]) });
		render(ProfileForm, { modelAccessPolicy: existing, readonly: true });

		await expect.element(page.getByText('Monthly spend')).toBeVisible();
		await expect.element(page.getByLabelText('Name', { exact: true })).toBeDisabled();
		await expect.element(page.getByRole('button', { name: 'Add Budget' })).not.toBeInTheDocument();
		await expect
			.element(page.getByRole('button', { name: 'Add Guardrail' }))
			.not.toBeInTheDocument();
		await expect
			.element(page.getByRole('button', { name: 'Save', exact: true }))
			.not.toBeInTheDocument();
		await expect
			.element(page.getByRole('button', { name: 'Delete profile' }))
			.not.toBeInTheDocument();
	});

	it('says when budgets need the profile to grant models', async () => {
		mockGateway();
		render(ProfileForm, { modelAccessPolicy: { ...existing, models: [] } });

		await expect.element(page.getByText('Monthly spend')).toBeVisible();
		await expect
			.element(page.getByText('Add models to this profile for its budgets to apply.'))
			.toBeVisible();
	});
});
