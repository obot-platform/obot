import { goto } from '$lib/url';
import { preparePageData } from '../../tests/helpers/pageData';
import {
	feedEvents,
	kills,
	overview,
	pendingApproval,
	policies,
	redTeam,
	supervisorAnswer,
	supervisorInsights,
	supervisorStatus
} from '../../tests/mocks/provenance';
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
		http.get('/api/provenance/kills', () => HttpResponse.json({ items: kills })),
		http.get('/api/provenance/policies', () => HttpResponse.json({ items: policies })),
		http.get('/api/provenance/tool-label-reviews', () => HttpResponse.json({ items: [], total: 0 }))
	);
}

function mockSupervisor(status = supervisorStatus) {
	mockOverview();
	worker.use(
		http.get('/api/provenance/supervisor/status', () => HttpResponse.json(status)),
		http.get('/api/provenance/supervisor/insights', () => HttpResponse.json(supervisorInsights)),
		http.get('/api/provenance/events', () =>
			HttpResponse.json({ items: feedEvents, total: feedEvents.length })
		),
		http.get('/api/provenance/approvals', () =>
			HttpResponse.json({ items: [pendingApproval], total: 2 })
		)
	);
}

async function renderPage() {
	// Layout's setup splash would otherwise sit on top of the page for admins.
	localStorage.setItem('seenSplashDialog', new Date().toISOString());
	await preparePageData();
	return render(SupervisionPage);
}

describe('Supervision overview', () => {
	it('shows coverage, the riskiest sessions and status cards', async () => {
		mockSupervisor();
		await renderPage();

		const coverage = page.getByRole('region', { name: 'Coverage' });
		await expect.element(coverage.getByText('Blind sessions')).toBeInTheDocument();
		await expect.element(coverage.getByText('12', { exact: true })).toBeInTheDocument();
		await expect.element(coverage.getByText('MCP gateway')).toBeInTheDocument();

		await expect
			.element(page.getByRole('link', { name: 'View all approvals' }))
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
		mockSupervisor();
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

	describe('the Supervisor', () => {
		it('shows that it is active, what it watches and what it did today', async () => {
			mockSupervisor();
			await renderPage();

			const presence = page.getByRole('region', { name: 'Supervisor', exact: true });
			await expect.element(presence.getByText('Active')).toBeInTheDocument();
			await expect
				.element(
					presence.getByText("I'm watching 12 sessions across 3 surfaces.", { exact: false })
				)
				.toBeInTheDocument();
			await expect.element(presence.getByText('3 are open right now.')).toBeInTheDocument();
			await expect.element(presence.getByText('Decides with claude-haiku-4')).toBeInTheDocument();
			await expect.element(presence.getByText('Judged by me')).toBeInTheDocument();
			await expect.element(presence.getByText('154', { exact: true })).toBeInTheDocument();
			await expect.element(presence.getByText(/^Last decision/)).toBeInTheDocument();
		});

		it('offers to turn itself on when it is off', async () => {
			mockSupervisor({ ...supervisorStatus, enabled: false });
			const saved = vi.fn();
			worker.use(
				http.get('/api/provenance/settings', () =>
					HttpResponse.json({ supervisorEnabled: false, retentionDays: 30 })
				),
				http.put('/api/provenance/settings', async ({ request }) => {
					const body = await request.json();
					saved(body);
					return HttpResponse.json(body);
				})
			);
			await renderPage();

			const presence = page.getByRole('region', { name: 'Supervisor', exact: true });
			await expect.element(presence.getByText('Off', { exact: true })).toBeInTheDocument();
			await expect.element(presence.getByText("I'm switched off.", { exact: false })).toBeVisible();
			await presence.getByRole('button', { name: 'Turn on the Supervisor' }).click();
			await vi.waitFor(() =>
				expect(saved).toHaveBeenCalledWith({ supervisorEnabled: true, retentionDays: 30 })
			);
		});

		it('points to model providers when it has no model', async () => {
			mockSupervisor({ ...supervisorStatus, enabled: false, configured: false });
			await renderPage();

			const presence = page.getByRole('region', { name: 'Supervisor', exact: true });
			await expect
				.element(presence.getByRole('link', { name: 'Configure model providers' }))
				.toHaveAttribute('href', '/admin/model-providers');
			await expect
				.element(presence.getByRole('button', { name: 'Turn on the Supervisor' }))
				.not.toBeInTheDocument();
		});

		it('describes itself from the overview when the status endpoint is missing', async () => {
			mockSupervisor();
			worker.use(
				http.get('/api/provenance/supervisor/status', () => new HttpResponse(null, { status: 404 }))
			);
			await renderPage();

			const presence = page.getByRole('region', { name: 'Supervisor', exact: true });
			await expect.element(presence.getByText('Active')).toBeInTheDocument();
			await expect
				.element(
					presence.getByText("I'm watching 12 sessions across 4 surfaces.", { exact: false })
				)
				.toBeInTheDocument();
		});

		it('keeps findings as the only summary and refreshes them on request', async () => {
			mockSupervisor();
			const refreshed = vi.fn();
			worker.use(
				http.get('/api/provenance/supervisor/insights', ({ request }) => {
					if (new URL(request.url).searchParams.get('refresh') === 'true') refreshed();
					return HttpResponse.json(supervisorInsights);
				})
			);
			await renderPage();

			await expect
				.element(page.getByRole('region', { name: "Here's what I'm seeing" }))
				.not.toBeInTheDocument();
			const findings = page.getByRole('region', { name: 'What I noticed' });
			await expect.element(findings.getByText(/^Updated /)).toBeInTheDocument();
			await findings.getByRole('button', { name: 'Refresh findings' }).click();
			await vi.waitFor(() => expect(refreshed).toHaveBeenCalled());
		});

		it('says when findings are unavailable', async () => {
			mockSupervisor();
			worker.use(
				http.get(
					'/api/provenance/supervisor/insights',
					() => new HttpResponse(null, { status: 404 })
				)
			);
			await renderPage();

			await expect
				.element(page.getByText("My findings aren't available right now."))
				.toBeInTheDocument();
		});

		it('summarizes held actions and links to the full review without inline decisions', async () => {
			mockSupervisor();
			await renderPage();
			const queue = page.getByRole('region', { name: 'Needs your attention' });
			await expect.element(queue.getByText('I recommend denying this.')).toBeInTheDocument();
			await expect.element(queue.getByText('1 more waiting')).toBeInTheDocument();
			await expect
				.element(queue.getByRole('link', { name: 'Review', exact: true }))
				.toHaveAttribute('href', '/approvals?approval=apr-1');
			await expect.element(queue.getByRole('button', { name: 'Approve' })).not.toBeInTheDocument();
			await expect
				.element(queue.getByRole('button', { name: 'Accept recommendation' }))
				.not.toBeInTheDocument();
			await expect
				.element(queue.getByText(pendingApproval.reason, { exact: true }))
				.not.toBeInTheDocument();
		});

		it('groups pending tool-label approvals by resource beside held actions', async () => {
			mockSupervisor();
			worker.use(
				http.get('/api/provenance/tool-label-reviews', () =>
					HttpResponse.json({
						items: [
							{
								resourceType: 'catalog-entry',
								resourceID: 'meridian',
								displayName: 'Meridian Operations',
								toolCount: 7,
								generatedAt: new Date().toISOString()
							},
							{
								resourceType: 'vmcp',
								resourceID: 'care',
								displayName: 'Northstar Health',
								toolCount: 5,
								generatedAt: new Date().toISOString()
							}
						],
						total: 2
					})
				)
			);
			await renderPage();
			const queue = page.getByRole('region', { name: 'Needs your attention' });
			await expect
				.element(queue.getByText('Meridian Operations', { exact: true }))
				.toBeInTheDocument();
			await expect
				.element(queue.getByText('MCP · Suggested labels for 7 tools'))
				.toBeInTheDocument();
			await expect
				.element(queue.getByText('vMCP · Suggested labels for 5 tools'))
				.toBeInTheDocument();
			await expect
				.element(queue.getByRole('link', { name: 'Review labels' }).first())
				.toHaveAttribute('href', '/tool-labels?resourceType=catalog-entry&resourceID=meridian');
			await expect
				.element(queue.getByRole('link', { name: 'Review', exact: true }))
				.toHaveAttribute('href', '/approvals?approval=apr-1');
			await expect.element(queue.getByRole('heading')).toHaveTextContent('Needs your attention 5');
			await expect.element(queue.getByRole('button', { name: /Approve/ })).not.toBeInTheDocument();
		});

		it('keeps label reviews visible even when no action is held', async () => {
			mockSupervisor();
			worker.use(
				http.get('/api/provenance/approvals', () => HttpResponse.json({ items: [], total: 0 })),
				http.get('/api/provenance/tool-label-reviews', () =>
					HttpResponse.json({
						items: [
							{
								resourceType: 'catalog-entry',
								resourceID: 'meridian',
								displayName: 'Meridian Operations',
								toolCount: 7
							}
						],
						total: 1
					})
				)
			);
			await renderPage();
			const queue = page.getByRole('region', { name: 'Needs your attention' });
			await expect.element(queue.getByRole('link', { name: 'Review labels' })).toBeInTheDocument();
			await expect.element(queue.getByText('Nothing is waiting on you.')).not.toBeInTheDocument();
		});

		it('keeps the queue compact and expands additional resource reviews on demand', async () => {
			mockSupervisor();
			const items = Array.from({ length: 5 }, (_, index) => ({
				resourceType: 'catalog-entry',
				resourceID: `resource-${index}`,
				displayName: `Operations ${index + 1}`,
				toolCount: 7
			}));
			worker.use(
				http.get('/api/provenance/tool-label-reviews', () => HttpResponse.json({ items, total: 5 }))
			);
			await renderPage();
			const queue = page.getByRole('region', { name: 'Needs your attention' });
			await expect.element(queue.getByText('Operations 3', { exact: true })).toBeInTheDocument();
			await expect
				.element(queue.getByText('Operations 4', { exact: true }))
				.not.toBeInTheDocument();
			await queue.getByRole('button', { name: 'Show all 5 label reviews' }).click();
			await expect.element(queue.getByText('Operations 5', { exact: true })).toBeInTheDocument();
		});

		it('retains action reviews and reports a label-inventory outage honestly', async () => {
			mockSupervisor();
			worker.use(
				http.get(
					'/api/provenance/tool-label-reviews',
					() => new HttpResponse(null, { status: 503 })
				)
			);
			await renderPage();
			const queue = page.getByRole('region', { name: 'Needs your attention' });
			await expect
				.element(queue.getByRole('link', { name: 'Review', exact: true }))
				.toBeInTheDocument();
			await expect
				.element(queue.getByRole('status'))
				.toHaveTextContent('Some pending reviews could not be loaded. Retrying shortly.');
		});

		it('queues suggested shadow policies for review without enforcing them from findings', async () => {
			mockSupervisor();
			const updated = vi.fn();
			worker.use(
				http.put('/api/provenance/policies/:id', async ({ params, request }) => {
					updated(params.id, await request.json());
					return HttpResponse.json({});
				})
			);
			await renderPage();

			const noticed = page.getByRole('region', { name: 'What I noticed' });
			await expect
				.element(noticed.getByRole('listitem').first())
				.toHaveAccessibleName('A honeytoken left a session');
			await expect
				.element(noticed.getByText('2 sessions were seen on only one surface', { exact: false }))
				.toBeInTheDocument();
			await expect.element(noticed.getByText(/^Updated /)).toBeInTheDocument();
			await expect
				.element(noticed.getByRole('button', { name: 'Enforce policy' }))
				.not.toBeInTheDocument();

			const queue = page.getByRole('region', { name: 'Needs your attention' });
			await expect.element(queue.getByText('Lethal trifecta', { exact: true })).toBeInTheDocument();
			await expect
				.element(queue.getByText('Lethal trifecta would have held 3 actions'))
				.toBeInTheDocument();
			await expect
				.element(queue.getByRole('link', { name: 'Review policy' }))
				.toHaveAttribute('href', '/policies/builtin-lethal-trifecta');
			expect(updated).not.toHaveBeenCalled();
		});

		it('includes drafted shadow policies but excludes enforced, disabled and unrecommended policies', async () => {
			mockSupervisor();
			const drafted = {
				...policies[1],
				id: 'draft-phi',
				name: 'Protect patient reports',
				builtIn: false,
				source: 'Ask before sharing patient reports externally.'
			};
			const enforced = {
				...policies[1],
				id: 'enforced-rule',
				name: 'Active sharing rule',
				mode: 'enforce',
				source: 'An already enforced draft.'
			};
			const disabled = {
				...policies[1],
				id: 'disabled-rule',
				name: 'Disabled sharing draft',
				enabled: false,
				source: 'A disabled draft.'
			};
			const unrecommended = {
				...policies[1],
				id: 'unrecommended-rule',
				name: 'Unrecommended shadow rule'
			};
			worker.use(
				http.get('/api/provenance/policies', () =>
					HttpResponse.json({ items: [policies[1], drafted, enforced, disabled, unrecommended] })
				)
			);
			await renderPage();

			const queue = page.getByRole('region', { name: 'Needs your attention' });
			await expect
				.element(queue.getByText('Protect patient reports', { exact: true }))
				.toBeInTheDocument();
			expect(await queue.getByRole('link', { name: 'Review policy' }).elements()).toHaveLength(2);
			await expect
				.element(queue.getByRole('link', { name: 'Review policy' }).last())
				.toHaveAttribute('href', '/policies/draft-phi');
			await expect.element(queue.getByText(enforced.name, { exact: true })).not.toBeInTheDocument();
			await expect.element(queue.getByText(disabled.name, { exact: true })).not.toBeInTheDocument();
			await expect
				.element(queue.getByText(unrecommended.name, { exact: true }))
				.not.toBeInTheDocument();
		});

		it('shows the five most severe findings until asked for all', async () => {
			mockSupervisor();
			const many = Array.from({ length: 7 }, (_, i) => ({
				id: `ins-${i}`,
				kind: 'coverage-gap',
				severity: 'info',
				title: `Finding ${i + 1}`,
				detail: ''
			}));
			worker.use(
				http.get('/api/provenance/supervisor/insights', () =>
					HttpResponse.json({ ...supervisorInsights, items: many })
				)
			);
			await renderPage();

			const noticed = page.getByRole('region', { name: 'What I noticed' });
			await expect.element(noticed.getByText('Finding 5', { exact: false })).toBeInTheDocument();
			await expect
				.element(noticed.getByText('Finding 6', { exact: false }))
				.not.toBeInTheDocument();
			await noticed.getByRole('button', { name: 'Show all 7' }).click();
			await expect.element(noticed.getByText('Finding 7', { exact: false })).toBeInTheDocument();
		});

		it('confirms before a one-click kill', async () => {
			mockSupervisor();
			const killed = vi.fn();
			worker.use(
				http.post('/api/provenance/kills', async ({ request }) => {
					const body = (await request.json()) as Record<string, unknown>;
					killed(body);
					return HttpResponse.json({ id: 'kill-9', ...body, createdAt: new Date().toISOString() });
				})
			);
			await renderPage();

			const noticed = page.getByRole('region', { name: 'What I noticed' });
			await noticed.getByRole('button', { name: 'Kill session' }).click();
			expect(killed).not.toHaveBeenCalled();
			await page.getByRole('dialog').getByRole('button', { name: 'Kill session' }).click();
			await vi.waitFor(() =>
				expect(killed).toHaveBeenCalledWith({
					scope: 'session',
					target: 'ses-risky',
					reason: 'Honeytoken tripped'
				})
			);
		});

		it('narrates decisions in the first person, with its own calls highlighted', async () => {
			mockSupervisor();
			await renderPage();

			const feed = page.getByRole('region', { name: 'Live activity' });
			const judged = feed
				.getByRole('listitem')
				.filter({ hasText: 'I reviewed tools/call acme-crm.send_report' });
			await expect
				.element(
					judged.getByText('I reviewed tools/call acme-crm.send_report and held it', {
						exact: false
					})
				)
				.toBeInTheDocument();
			await expect.element(judged.getByText('My call')).toBeInTheDocument();
			await expect.element(judged.getByText('81% confident', { exact: false })).toBeInTheDocument();
			await expect
				.element(
					judged.getByText('I held this because the report carries customer PII', { exact: false })
				)
				.toBeInTheDocument();
			await expect
				.element(judged.getByRole('link', { name: 'Decide now' }))
				.toHaveAttribute('href', '/approvals?approval=apr-3');
			await expect
				.element(
					feed.getByText(
						'I held POST paste.example for approval under PII lineage to external destination.'
					)
				)
				.toBeInTheDocument();
			await expect
				.element(feed.getByText('I allowed tools/call github.search_issues.'))
				.toBeInTheDocument();

			await feed.getByLabelText('Only my interventions').click();
			await expect
				.element(feed.getByText('I allowed tools/call github.search_issues.'))
				.not.toBeInTheDocument();
			await expect.element(judged.getByText('My call')).toBeInTheDocument();
		});

		it('answers a suggested question', async () => {
			mockSupervisor();
			const asked = vi.fn();
			worker.use(
				http.post('/api/provenance/supervisor/ask', async ({ request }) => {
					asked(await request.json());
					return HttpResponse.json(supervisorAnswer);
				})
			);
			await renderPage();

			const ask = page.getByRole('region', { name: 'Ask the Supervisor' });
			await ask
				.getByRole('button', { name: "What's the riskiest thing happening right now?" })
				.click();
			await vi.waitFor(() =>
				expect(asked).toHaveBeenCalledWith({
					question: "What's the riskiest thing happening right now?"
				})
			);
			await expect
				.element(
					ask.getByText("The riskiest thing right now is alice's session.", { exact: false })
				)
				.toBeInTheDocument();
			await expect
				.element(ask.getByText('Answered by claude-sonnet-4', { exact: false }))
				.toBeInTheDocument();
			await expect
				.element(ask.getByText('Based on 12 sessions and 40 events', { exact: false }))
				.toBeInTheDocument();
		});

		it('says when it cannot answer questions', async () => {
			mockSupervisor();
			await renderPage();

			const ask = page.getByRole('region', { name: 'Ask the Supervisor' });
			await ask.getByLabelText('Your question').fill('Anything risky?');
			await ask.getByRole('button', { name: 'Ask', exact: true }).click();
			await expect
				.element(ask.getByText("I can't answer questions until I'm configured with a model."))
				.toBeInTheDocument();
		});
	});
});
