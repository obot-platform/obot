import { page as appPage } from '$app/state';
import { goto } from '$lib/url';
import { preparePageData } from '../../tests/helpers/pageData';
import { sessions } from '../../tests/mocks/provenance';
import { worker } from '../../tests/mocks/worker';
import SessionsPage from './+page.svelte';
import { http, HttpResponse } from 'msw';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

vi.mock(import('$lib/url'), async (importOriginal) => ({
	...(await importOriginal()),
	goto: vi.fn()
}));

function mockSessions(requested = vi.fn()) {
	worker.use(
		http.get('/api/provenance/sessions', ({ request }) => {
			requested(new URL(request.url).searchParams);
			return HttpResponse.json({ items: sessions, total: sessions.length, limit: 50, offset: 0 });
		})
	);
	return requested;
}

async function renderPage() {
	localStorage.setItem('seenSplashDialog', new Date().toISOString());
	await preparePageData();
	return render(SessionsPage);
}

afterEach(() => {
	for (const key of Array.from((appPage.url as URL).searchParams.keys())) {
		(appPage.url as URL).searchParams.delete(key);
	}
});

describe('Sessions list', () => {
	it('lists sessions with risk, surfaces and labels, sorted by risk by default', async () => {
		const requested = mockSessions();
		await renderPage();

		await expect
			.element(page.getByRole('link', { name: 'Triage GitHub issue #4411' }))
			.toHaveAttribute('href', '/sessions/ses-risky');
		await expect.element(page.getByText('Refactor billing module')).toBeInTheDocument();
		await expect.element(page.getByLabelText('Risk 72')).toBeInTheDocument();
		await expect.element(page.getByText('untrusted', { exact: true })).toBeInTheDocument();
		await expect.element(page.getByText('egress', { exact: true })).toBeInTheDocument();
		await expect.element(page.getByText('Ended', { exact: true })).toBeInTheDocument();

		const params = requested.mock.calls[0][0] as URLSearchParams;
		expect(params.get('sort_by')).toBe('risk');
		expect(params.get('sort_order')).toBe('desc');
		expect(params.get('limit')).toBe('50');
		expect(params.get('start_time')).toBeTruthy();
	});

	it('passes URL filters to the API', async () => {
		(appPage.url as URL).searchParams.set('min_risk', '50');
		(appPage.url as URL).searchParams.set('surface', 'egress');
		(appPage.url as URL).searchParams.set('open', 'true');
		(appPage.url as URL).searchParams.set('range', 'all');
		(appPage.url as URL).searchParams.set('query', 'paste');
		const requested = mockSessions();
		await renderPage();

		await vi.waitFor(() => expect(requested).toHaveBeenCalled());
		const params = requested.mock.calls[0][0] as URLSearchParams;
		expect(params.get('min_risk')).toBe('50');
		expect(params.get('surface')).toBe('egress');
		expect(params.get('open')).toBe('true');
		expect(params.get('query')).toBe('paste');
		expect(params.get('start_time')).toBeNull();
		await expect.element(page.getByRole('button', { name: 'Clear filters' })).toBeInTheDocument();
	});

	it('shows an empty state when nothing matches', async () => {
		await renderPage();
		await expect.element(page.getByText('No sessions match these filters.')).toBeInTheDocument();
	});

	it('opens a session when its row is clicked', async () => {
		mockSessions();
		await renderPage();

		await page.getByRole('cell', { name: 'bob@example.com' }).click();
		expect(goto).toHaveBeenCalledWith('/sessions/ses-quiet');
	});
});
