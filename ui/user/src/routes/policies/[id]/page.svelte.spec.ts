import { page as appPage } from '$app/state';
import { goto } from '$lib/url';
import { preparePageData } from '../../../tests/helpers/pageData';
import { draft, policies, simulation } from '../../../tests/mocks/provenance';
import { worker } from '../../../tests/mocks/worker';
import PolicyPage from './+page.svelte';
import { http, HttpResponse } from 'msw';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

vi.mock(import('$lib/url'), async (importOriginal) => ({
	...(await importOriginal()),
	goto: vi.fn()
}));

async function renderPage(id = 'new', search?: Record<string, string>) {
	(appPage.params as Record<string, string>).id = id;
	for (const [k, v] of Object.entries(search ?? {})) (appPage.url as URL).searchParams.set(k, v);
	localStorage.setItem('seenSplashDialog', new Date().toISOString());
	await preparePageData();
	return render(PolicyPage);
}

afterEach(() => {
	delete (appPage.params as Record<string, string>).id;
	for (const key of Array.from((appPage.url as URL).searchParams.keys())) {
		(appPage.url as URL).searchParams.delete(key);
	}
});

describe('Policy editor', () => {
	it('validates the condition as it is typed and creates the policy', async () => {
		const validated = vi.fn();
		const created = vi.fn();
		worker.use(
			http.post('/api/provenance/policies/validate', async ({ request }) => {
				const { condition } = (await request.json()) as { condition: string };
				validated(condition);
				return condition.includes('(')
					? HttpResponse.json({ valid: false, error: 'unexpected token' })
					: HttpResponse.json({ valid: true });
			}),
			http.post('/api/provenance/policies', async ({ request }) => {
				const body = (await request.json()) as Record<string, unknown>;
				created(body);
				return HttpResponse.json({ ...body, id: 'pol-new' }, { status: 201 });
			})
		);
		await renderPage();

		await expect.element(page.getByRole('heading', { name: 'New policy' }).first()).toBeVisible();
		const save = page.getByRole('button', { name: 'Save' });
		await expect.element(save).toBeDisabled();

		await page.getByLabelText('Name', { exact: true }).fill('Block pastes');
		const condition = page.getByLabelText('Condition');
		await condition.fill('action.sink.external && (');
		await expect.element(page.getByText('unexpected token')).toBeInTheDocument();
		await expect.element(save).toBeDisabled();

		await condition.fill('action.sink.external');
		await expect.element(page.getByText('Condition is valid.')).toBeInTheDocument();
		await expect.element(save).toBeEnabled();
		expect(validated).toHaveBeenLastCalledWith('action.sink.external');

		await page.getByRole('radio', { name: /Enforce/ }).click();
		await page.getByRole('checkbox', { name: 'Egress' }).click();
		await save.click();

		await vi.waitFor(() =>
			expect(created).toHaveBeenCalledWith(
				expect.objectContaining({
					name: 'Block pastes',
					condition: 'action.sink.external',
					effect: 'deny',
					mode: 'enforce',
					surfaces: ['egress'],
					enabled: true
				})
			)
		);
		await vi.waitFor(() =>
			expect(goto).toHaveBeenCalledWith('/policies/pol-new', { replaceState: true })
		);
	});

	it('loads an existing policy and shows effect-specific fields', async () => {
		worker.use(
			http.get('/api/provenance/policies/pol-custom', () => HttpResponse.json(policies[2]))
		);
		await renderPage('pol-custom');

		await expect
			.element(page.getByLabelText('Name', { exact: true }))
			.toHaveValue('No pastes from CI');
		await expect.element(page.getByLabelText('Condition')).toHaveValue(policies[2].condition);
		await expect.element(page.getByRole('radio', { name: /Shadow/ })).toBeChecked();
		await expect.element(page.getByRole('checkbox', { name: 'Egress' })).toBeChecked();
		await expect.element(page.getByText('Drafted from', { exact: false })).toBeInTheDocument();

		// Guidance appears only for judge; approvers only for require_approval.
		await expect
			.element(page.getByLabelText('Guidance for the Supervisor'))
			.not.toBeInTheDocument();
		await page.getByRole('combobox', { name: 'Effect' }).click();
		await page.getByRole('button', { name: 'Judge', exact: true }).click();
		await expect.element(page.getByLabelText('Guidance for the Supervisor')).toBeInTheDocument();
		await page.getByRole('combobox', { name: 'Effect' }).click();
		await page.getByRole('button', { name: 'Require approval', exact: true }).click();
		await expect.element(page.getByLabelText('Approvers')).toBeInTheDocument();
		await expect.element(page.getByLabelText('Approval TTL (minutes)')).toBeInTheDocument();
	});

	it('drafts a policy from plain language and shows its tests', async () => {
		const drafted = vi.fn();
		worker.use(
			http.post('/api/provenance/policies/draft', async ({ request }) => {
				drafted((await request.json()) as { text: string });
				return HttpResponse.json(draft);
			})
		);
		await renderPage('new', { draft: '1' });

		await page.getByLabelText('Rule').fill('Never let customer data reach a paste site');
		await page.getByRole('button', { name: 'Draft', exact: true }).click();

		await vi.waitFor(() =>
			expect(drafted).toHaveBeenCalledWith({ text: 'Never let customer data reach a paste site' })
		);
		await expect
			.element(page.getByLabelText('Name', { exact: true }))
			.toHaveValue('Customer data never reaches paste sites');
		await expect.element(page.getByLabelText('Condition')).toHaveValue(draft.policy.condition);
		await expect.element(page.getByText(draft.explanation)).toBeInTheDocument();
		await expect.element(page.getByText('2 of 2 tests pass')).toBeInTheDocument();
		await expect.element(page.getByText('customer data to internal wiki')).toBeInTheDocument();
	});

	it('explains when the Supervisor cannot draft', async () => {
		await renderPage();
		await page.getByLabelText('Rule').fill('anything');
		await page.getByRole('button', { name: 'Draft', exact: true }).click();
		await expect
			.element(page.getByText('The Supervisor is not available', { exact: false }))
			.toBeInTheDocument();
	});

	it('simulates the policy against recorded decisions', async () => {
		const simulated = vi.fn();
		worker.use(
			http.post('/api/provenance/policies/simulate', async ({ request }) => {
				simulated((await request.json()) as Record<string, unknown>);
				return HttpResponse.json(simulation);
			})
		);
		await renderPage();

		await page.getByLabelText('Condition').fill('action.sink.external');
		await expect.element(page.getByText('Condition is valid.')).toBeInTheDocument();
		await page.getByLabelText('Days').fill('7');
		await page.getByRole('button', { name: 'Simulate', exact: true }).click();

		await vi.waitFor(() =>
			expect(simulated).toHaveBeenCalledWith(
				expect.objectContaining({
					sinceHours: 168,
					policy: expect.objectContaining({ condition: 'action.sink.external' })
				})
			)
		);
		const region = page.getByRole('region', { name: 'Simulate' });
		await expect.element(region.getByText('154')).toBeInTheDocument();
		await expect.element(region.getByText('33%')).toBeInTheDocument();
		await expect.element(region.getByText('POST paste.example')).toBeInTheDocument();
	});

	it('shows the policy cheat sheet on demand', async () => {
		await renderPage();
		const toggle = page.getByRole('button', { name: 'Policy cheat sheet' });
		await expect
			.element(page.getByText('action.lineage.labels', { exact: true }))
			.not.toBeInTheDocument();
		await toggle.click();
		await expect
			.element(page.getByText('action.lineage.labels', { exact: true }))
			.toBeInTheDocument();
		await expect.element(page.getByText('host_matches(host, patterns)')).toBeInTheDocument();
	});
});
