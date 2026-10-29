import { goto } from '$lib/url';
import { preparePageData } from '../../tests/helpers/pageData';
import { policies } from '../../tests/mocks/provenance';
import { worker } from '../../tests/mocks/worker';
import PoliciesPage from './+page.svelte';
import { http, HttpResponse } from 'msw';
import { describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

vi.mock(import('$lib/url'), async (importOriginal) => ({
	...(await importOriginal()),
	goto: vi.fn()
}));

function mockPolicies() {
	worker.use(http.get('/api/provenance/policies', () => HttpResponse.json({ items: policies })));
}

async function renderPage() {
	localStorage.setItem('seenSplashDialog', new Date().toISOString());
	await preparePageData();
	return render(PoliciesPage);
}

describe('Policies list', () => {
	it('lists policies with effect, mode, surfaces and built-in tags', async () => {
		mockPolicies();
		await renderPage();

		await expect
			.element(page.getByRole('link', { name: 'Honeytoken exfiltration' }))
			.toHaveAttribute('href', '/policies/builtin-honeytoken');
		await expect.element(page.getByText('Built-in').first()).toBeInTheDocument();
		await expect.element(page.getByText('Require approval')).toBeInTheDocument();
		await expect.element(page.getByText('egress', { exact: true })).toBeInTheDocument();
		await expect
			.element(page.getByRole('switch', { name: 'Enforce Honeytoken exfiltration' }))
			.toBeChecked();
		await expect
			.element(page.getByRole('switch', { name: 'Enforce Lethal trifecta' }))
			.not.toBeChecked();
		await expect
			.element(page.getByRole('switch', { name: 'Enable No pastes from CI' }))
			.not.toBeChecked();
	});

	it('switches a policy to enforce inline', async () => {
		mockPolicies();
		const updated = vi.fn();
		worker.use(
			http.put('/api/provenance/policies/:id', async ({ params, request }) => {
				const body = (await request.json()) as Record<string, unknown>;
				updated(params.id, body);
				return HttpResponse.json(body);
			})
		);
		await renderPage();

		await page.getByRole('switch', { name: 'Enforce Lethal trifecta' }).click();
		await vi.waitFor(() =>
			expect(updated).toHaveBeenCalledWith(
				'builtin-lethal-trifecta',
				expect.objectContaining({ mode: 'enforce', name: 'Lethal trifecta' })
			)
		);
		await expect
			.element(page.getByRole('switch', { name: 'Enforce Lethal trifecta' }))
			.toBeChecked();
	});

	it('deletes only custom policies', async () => {
		mockPolicies();
		const deleted = vi.fn();
		worker.use(
			http.delete('/api/provenance/policies/:id', ({ params }) => {
				deleted(params.id);
				return new HttpResponse(null, { status: 204 });
			})
		);
		await renderPage();

		await expect.element(page.getByText('No pastes from CI')).toBeInTheDocument();
		const deleteButtons = page.getByRole('button', { name: 'Delete policy' });
		await expect.element(deleteButtons).toBeInTheDocument();
		expect((await deleteButtons.elements()).length).toBe(1);

		await deleteButtons.click();
		await page.getByRole('button', { name: "Yes, I'm sure" }).click();
		await vi.waitFor(() => expect(deleted).toHaveBeenCalledWith('pol-custom'));
		await expect.element(page.getByText('No pastes from CI')).not.toBeInTheDocument();
	});

	it('offers new and AI-drafted policies', async () => {
		mockPolicies();
		await renderPage();

		await page.getByRole('button', { name: 'New policy' }).click();
		expect(goto).toHaveBeenCalledWith('/policies/new');
		await page.getByRole('button', { name: 'Draft with AI' }).click();
		expect(goto).toHaveBeenCalledWith('/policies/new?draft=1');
	});

	it('shows an empty state without policies', async () => {
		await renderPage();
		await expect.element(page.getByText('No policies yet.')).toBeInTheDocument();
	});
});
