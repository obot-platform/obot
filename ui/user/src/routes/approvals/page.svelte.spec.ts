import { page as appPage } from '$app/state';
import { preparePageData } from '../../tests/helpers/pageData';
import { approvedApproval, pendingApproval } from '../../tests/mocks/provenance';
import { worker } from '../../tests/mocks/worker';
import ApprovalsPage from './+page.svelte';
import { http, HttpResponse } from 'msw';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

vi.mock(import('$lib/url'), async (importOriginal) => ({
	...(await importOriginal()),
	goto: vi.fn()
}));

function mockApprovals(requested = vi.fn()) {
	worker.use(
		http.get('/api/provenance/approvals', ({ request }) => {
			const status = new URL(request.url).searchParams.get('status');
			requested(status);
			const items =
				status === 'pending' ? [pendingApproval] : status === 'approved' ? [approvedApproval] : [];
			return HttpResponse.json({ items, total: items.length });
		})
	);
	return requested;
}

async function renderPage(view?: string) {
	if (view) (appPage.url as URL).searchParams.set('view', view);
	localStorage.setItem('seenSplashDialog', new Date().toISOString());
	await preparePageData();
	return render(ApprovalsPage);
}

afterEach(() => {
	for (const key of Array.from((appPage.url as URL).searchParams.keys())) {
		(appPage.url as URL).searchParams.delete(key);
	}
});

describe('Approvals inbox', () => {
	it('shows pending requests with reason, risk, triage and actions', async () => {
		const requested = mockApprovals();
		await renderPage();

		await vi.waitFor(() => expect(requested).toHaveBeenCalledWith('pending'));
		const card = page.getByRole('article', { name: 'PII lineage to external destination' });
		await expect
			.element(card.getByText('14 PII records from salesforce.query', { exact: false }))
			.toBeInTheDocument();
		await expect.element(card.getByLabelText('Risk 72')).toBeInTheDocument();
		await expect.element(card.getByText('POST paste.example')).toBeInTheDocument();
		await expect.element(card.getByText('Recommends deny')).toBeInTheDocument();
		await expect.element(card.getByText('92% confidence')).toBeInTheDocument();
		await expect
			.element(card.getByRole('link', { name: 'View session' }))
			.toHaveAttribute('href', '/sessions/ses-risky');
		await expect.element(card.getByRole('button', { name: 'Approve' })).toBeInTheDocument();
		await expect.element(card.getByRole('button', { name: 'Deny' })).toBeInTheDocument();
	});

	it('approves a request with a note and removes it from the inbox', async () => {
		mockApprovals();
		const approved = vi.fn();
		worker.use(
			http.post('/api/provenance/approvals/:id/approve', async ({ params, request }) => {
				approved(params.id, (await request.json()) as { note: string });
				return HttpResponse.json({ ...pendingApproval, status: 'approved' });
			})
		);
		await renderPage();

		await page.getByLabelText('Note (optional)').fill('Known fixture');
		await page.getByRole('button', { name: 'Approve' }).click();
		await vi.waitFor(() =>
			expect(approved).toHaveBeenCalledWith('apr-1', { note: 'Known fixture' })
		);
		await expect.element(page.getByText('No pending approvals.')).toBeInTheDocument();
	});

	it('denies a request', async () => {
		mockApprovals();
		const denied = vi.fn();
		worker.use(
			http.post('/api/provenance/approvals/:id/deny', ({ params }) => {
				denied(params.id);
				return HttpResponse.json({ ...pendingApproval, status: 'denied' });
			})
		);
		await renderPage();

		await page.getByRole('button', { name: 'Deny' }).click();
		await vi.waitFor(() => expect(denied).toHaveBeenCalledWith('apr-1'));
	});

	it('shows resolved requests without actions on the other tabs', async () => {
		const requested = mockApprovals();
		await renderPage('approved');

		await vi.waitFor(() => expect(requested).toHaveBeenCalledWith('approved'));
		await expect
			.element(page.getByText('Approved by admin@example.com', { exact: false }))
			.toBeInTheDocument();
		await expect.element(page.getByText('Known test fixture')).toBeInTheDocument();
		await expect.element(page.getByRole('button', { name: 'Approve' })).not.toBeInTheDocument();
	});

	it('shows an empty state on the denied tab', async () => {
		mockApprovals();
		await renderPage('denied');
		await expect.element(page.getByText('No denied approvals.')).toBeInTheDocument();
	});
});
