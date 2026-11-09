import { page as appPage } from '$app/state';
import { preparePageData } from '../../../tests/helpers/pageData';
import {
	judgedEvent,
	narrative,
	quietSession,
	riskyEvents,
	riskySession,
	supervisorAnswer
} from '../../../tests/mocks/provenance';
import { worker } from '../../../tests/mocks/worker';
import SessionPage from './+page.svelte';
import { http, HttpResponse } from 'msw';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

vi.mock(import('$lib/url'), async (importOriginal) => ({
	...(await importOriginal()),
	goto: vi.fn()
}));

function mockSession() {
	worker.use(
		http.get('/api/provenance/sessions/ses-risky', () => HttpResponse.json(riskySession)),
		http.get('/api/provenance/sessions/ses-risky/events', () =>
			HttpResponse.json({ items: riskyEvents, total: riskyEvents.length })
		),
		http.get('/api/provenance/sessions/ses-risky/narrative', () => HttpResponse.json(narrative))
	);
}

async function renderPage(id = 'ses-risky') {
	(appPage.params as Record<string, string>).id = id;
	localStorage.setItem('seenSplashDialog', new Date().toISOString());
	await preparePageData();
	return render(SessionPage);
}

afterEach(() => {
	delete (appPage.params as Record<string, string>).id;
});

describe('Session replay', () => {
	it('shows the session header, warrant, labels and coverage', async () => {
		mockSession();
		await renderPage();

		await expect
			.element(page.getByRole('heading', { name: 'Triage GitHub issue #4411' }).first())
			.toBeInTheDocument();
		await expect.element(page.getByLabelText('Risk 72')).toBeInTheDocument();
		await expect
			.element(page.getByText('Triage the flaky test in obot-platform/obot'))
			.toBeInTheDocument();
		await expect.element(page.getByText('Drifted')).toBeInTheDocument();
		await expect.element(page.getByText('mcp:github:*')).toBeInTheDocument();
		await expect
			.element(page.getByText('First drift: tools/call salesforce.query'))
			.toBeInTheDocument();

		const coverage = page.getByRole('region', { name: 'Coverage' });
		await expect.element(coverage.getByText('Not observed').first()).toBeInTheDocument();
		await expect.element(coverage.getByText('Reported').first()).toBeInTheDocument();
	});

	it('renders the timeline with decisions and expands a decision', async () => {
		mockSession();
		await renderPage();

		const timeline = page.getByRole('list', { name: 'Timeline' });
		await expect.element(timeline.getByText('POST paste.example')).toBeInTheDocument();
		await expect.element(timeline.getByText('Approval pending')).toBeInTheDocument();
		await expect.element(timeline.getByText('Shadow: would Judge')).toBeInTheDocument();

		await timeline.getByRole('button', { name: 'Show details' }).last().click();
		await expect
			.element(page.getByText('14 PII records from salesforce.query', { exact: false }))
			.toBeInTheDocument();
		await expect
			.element(page.getByRole('link', { name: 'PII lineage to external destination' }))
			.toHaveAttribute('href', '/policies/builtin-pii-lineage');
		await expect.element(page.getByText('pii ×14 from salesforce.query')).toBeInTheDocument();
		await expect
			.element(page.getByRole('link', { name: 'View approval' }))
			.toHaveAttribute('href', '/approvals?approval=apr-1');
	});

	it("shows a held action's approval as it stands now, not as it was when held", async () => {
		mockSession();
		const asked = vi.fn();
		worker.use(
			http.get('/api/provenance/approvals', ({ request }) => {
				asked(new URL(request.url).searchParams.get('session_id'));
				return HttpResponse.json({ items: [{ id: 'apr-1', status: 'approved' }], total: 1 });
			})
		);
		await renderPage();

		const timeline = page.getByRole('list', { name: 'Timeline' });
		await expect.element(timeline.getByText('Approved', { exact: true })).toBeInTheDocument();
		await expect.element(timeline.getByText('Approval pending')).not.toBeInTheDocument();
		expect(asked).toHaveBeenCalledWith('ses-risky');
	});

	it('shows the Supervisor commentary with concerns', async () => {
		mockSession();
		await renderPage();

		const panel = page.getByRole('region', { name: 'Supervisor commentary' });
		await expect
			.element(panel.getByText('tried to post them to paste.example', { exact: false }))
			.toBeInTheDocument();
		await expect
			.element(panel.getByText('PII lineage reaches an external host'))
			.toBeInTheDocument();
		await expect.element(panel.getByText('Narrated by claude-sonnet-4')).toBeInTheDocument();
	});

	it('ends the session after confirmation', async () => {
		mockSession();
		const ended = vi.fn();
		worker.use(
			http.post('/api/provenance/sessions/ses-risky/end', () => {
				ended();
				return new HttpResponse(null, { status: 204 });
			})
		);
		await renderPage();

		await page.getByRole('button', { name: 'End session' }).click();
		await page.getByRole('button', { name: 'End session' }).last().click();
		await vi.waitFor(() => expect(ended).toHaveBeenCalled());
	});

	it('opens the kill dialog scoped to this session', async () => {
		mockSession();
		await renderPage();

		await page.getByRole('button', { name: 'Kill session' }).click();
		await expect.element(page.getByLabelText('Target')).toHaveValue('ses-risky');
	});

	it('issues a warrant', async () => {
		mockSession();
		const issued = vi.fn();
		worker.use(
			http.post('/api/provenance/sessions/ses-risky/warrant', async ({ request }) => {
				const body = (await request.json()) as Record<string, unknown>;
				issued(body);
				return HttpResponse.json({ id: 'war-2', sessionID: 'ses-risky', ...body });
			})
		);
		await renderPage();

		await page.getByRole('button', { name: 'Issue warrant' }).first().click();
		await page.getByLabelText('Task').fill('Fix the build');
		await page.getByLabelText('Scopes (one per line)').fill('shell\nmcp:github:*');
		await page.getByRole('dialog').getByRole('button', { name: 'Issue warrant' }).click();

		await vi.waitFor(() =>
			expect(issued).toHaveBeenCalledWith({
				task: 'Fix the build',
				scopes: ['shell', 'mcp:github:*'],
				ttlSeconds: 3600
			})
		);
	});

	it('says when the session does not exist', async () => {
		await renderPage('ses-missing');
		await expect.element(page.getByText('Session not found.')).toBeInTheDocument();
	});

	it('follows an open session live', async () => {
		mockSession();
		await renderPage();

		const panel = page.getByRole('region', { name: 'Supervisor commentary' });
		await expect.element(panel.getByText('Following this session live')).toBeInTheDocument();
	});

	it('does not follow an ended session', async () => {
		worker.use(
			http.get('/api/provenance/sessions/ses-quiet', () => HttpResponse.json(quietSession))
		);
		await renderPage('ses-quiet');

		const panel = page.getByRole('region', { name: 'Supervisor commentary' });
		await expect.element(panel.getByText('What I see in this session')).toBeInTheDocument();
		await expect.element(panel.getByText('Following this session live')).not.toBeInTheDocument();
	});

	it('answers questions scoped to the session', async () => {
		mockSession();
		const asked = vi.fn();
		worker.use(
			http.post('/api/provenance/supervisor/ask', async ({ request }) => {
				asked(await request.json());
				return HttpResponse.json(supervisorAnswer);
			})
		);
		await renderPage();

		const panel = page.getByRole('region', { name: 'Supervisor commentary' });
		await panel.getByRole('button', { name: 'Is it safe to let this session continue?' }).click();
		await vi.waitFor(() =>
			expect(asked).toHaveBeenCalledWith({
				question: 'Is it safe to let this session continue?',
				sessionID: 'ses-risky'
			})
		);
		await expect
			.element(
				panel.getByText("The riskiest thing right now is alice's session.", { exact: false })
			)
			.toBeInTheDocument();
	});

	it('speaks for held and judged decisions in the timeline', async () => {
		const events = [...riskyEvents, judgedEvent];
		worker.use(
			http.get('/api/provenance/sessions/ses-risky', () => HttpResponse.json(riskySession)),
			http.get('/api/provenance/sessions/ses-risky/events', () =>
				HttpResponse.json({ items: events, total: events.length })
			),
			http.get('/api/provenance/sessions/ses-risky/narrative', () => HttpResponse.json(narrative))
		);
		await renderPage();

		const timeline = page.getByRole('list', { name: 'Timeline' });
		await expect
			.element(
				timeline.getByText(
					'I held POST paste.example for approval under PII lineage to external destination.'
				)
			)
			.toBeInTheDocument();
		await expect
			.element(
				timeline.getByText('I reviewed tools/call acme-crm.send_report and held it', {
					exact: false
				})
			)
			.toBeInTheDocument();
		await expect.element(timeline.getByText('My call')).toBeInTheDocument();
		await expect
			.element(timeline.getByText('the report carries customer PII', { exact: false }))
			.toBeInTheDocument();
		await expect
			.element(timeline.getByText('I allowed tools/call github.search_issues.'))
			.not.toBeInTheDocument();
	});
});
