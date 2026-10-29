import { goto } from '$lib/url';
import { preparePageData } from '../../tests/helpers/pageData';
import { kills, overview, redTeam } from '../../tests/mocks/provenance';
import { worker } from '../../tests/mocks/worker';
import SupervisionPage from './+page.svelte';
import { http, HttpResponse } from 'msw';
import { describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

vi.mock(import('$lib/url'), async (importOriginal) => ({
	...(await importOriginal()),
	goto: vi.fn()
}));

function mockOverview() {
	worker.use(
		http.get('/api/provenance/overview', () => HttpResponse.json(overview)),
		http.get('/api/provenance/kills', () => HttpResponse.json({ items: kills }))
	);
}

async function renderPage() {
	// Layout's setup splash would otherwise sit on top of the page for admins.
	localStorage.setItem('seenSplashDialog', new Date().toISOString());
	await preparePageData();
	return render(SupervisionPage);
}

describe('Supervision overview', () => {
	it('shows coverage, decisions, the riskiest sessions and status cards', async () => {
		mockOverview();
		await renderPage();

		const coverage = page.getByRole('region', { name: 'Coverage' });
		await expect.element(coverage.getByText('Blind sessions')).toBeInTheDocument();
		await expect.element(coverage.getByText('12', { exact: true })).toBeInTheDocument();
		await expect.element(coverage.getByText('MCP gateway')).toBeInTheDocument();

		const decisions = page.getByRole('region', { name: 'Decisions' });
		await expect.element(decisions.getByText('140', { exact: true }).first()).toBeInTheDocument();
		await expect
			.element(page.getByRole('link', { name: /Pending approvals/ }))
			.toHaveAttribute('href', '/approvals');

		await expect
			.element(page.getByRole('link', { name: 'Triage GitHub issue #4411' }))
			.toHaveAttribute('href', '/sessions/ses-risky');
		await expect.element(page.getByText('2 enforcing')).toBeInTheDocument();
		await expect.element(page.getByText('5 in shadow')).toBeInTheDocument();
		await expect.element(page.getByText('Model: claude-sonnet-4')).toBeInTheDocument();
		await expect.element(page.getByText('rogue_agent')).toBeInTheDocument();
	});

	it('navigates to a session from the riskiest table', async () => {
		mockOverview();
		await renderPage();

		await page.getByRole('cell', { name: 'cursor' }).click();
		expect(goto).toHaveBeenCalledWith('/sessions/ses-quiet');
	});

	it('activates a kill switch from the dialog', async () => {
		mockOverview();
		const created = vi.fn();
		worker.use(
			http.post('/api/provenance/kills', async ({ request }) => {
				const body = (await request.json()) as Record<string, unknown>;
				created(body);
				return HttpResponse.json({
					id: 'kill-2',
					...body,
					createdBy: 'me@example.com',
					createdAt: new Date().toISOString()
				});
			})
		);
		await renderPage();

		await page.getByRole('button', { name: 'Kill switch' }).first().click();
		await expect.element(page.getByRole('dialog')).toBeVisible();
		await expect.element(page.getByRole('button', { name: 'Kill', exact: true })).toBeDisabled();

		await page.getByLabelText('Target').fill('ses-risky');
		await page.getByLabelText('Reason').fill('Exfiltration in progress');
		await page.getByRole('button', { name: 'Kill', exact: true }).click();

		await vi.waitFor(() =>
			expect(created).toHaveBeenCalledWith({
				scope: 'session',
				target: 'ses-risky',
				reason: 'Exfiltration in progress'
			})
		);
		await expect.element(page.getByText('Exfiltration in progress')).toBeInTheDocument();
	});

	it('revokes an active kill', async () => {
		mockOverview();
		const revoked = vi.fn();
		worker.use(
			http.delete('/api/provenance/kills/:id', ({ params }) => {
				revoked(params.id);
				return new HttpResponse(null, { status: 204 });
			})
		);
		await renderPage();

		await page.getByRole('button', { name: 'Revoke' }).click();
		await vi.waitFor(() => expect(revoked).toHaveBeenCalledWith('kill-1'));
		await expect.element(page.getByText('No active kills.')).toBeInTheDocument();
	});

	it('runs the red team and renders the scorecard', async () => {
		mockOverview();
		worker.use(http.post('/api/provenance/redteam/run', () => HttpResponse.json(redTeam)));
		await renderPage();

		await page.getByRole('button', { name: 'Run red team' }).click();
		await expect.element(page.getByText('Prompt injection via issue comment')).toBeInTheDocument();
		await expect.element(page.getByText('1 of 2 scenarios blocked')).toBeInTheDocument();
		await expect.element(page.getByText('Device hook not enrolled')).toBeInTheDocument();
	});
});
