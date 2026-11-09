import { Group } from '$lib/services';
import { createMockProfile, preparePageData } from '../../../tests/helpers/pageData';
import { worker } from '../../../tests/mocks/worker';
import AIAnswer from './AIAnswer.svelte';
import { http, HttpResponse } from 'msw';
import { describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

const widget = (w: object) => '```obot-widget\n' + JSON.stringify(w) + '\n```';

const session = {
	id: 'sess1',
	email: 'dev@obot.local',
	title: 'Refactor the billing service',
	actor: {},
	startedAt: new Date().toISOString(),
	lastActivityAt: new Date().toISOString(),
	endedAt: new Date().toISOString(),
	state: { risk: 72, events: 2, toolCalls: 1, denied: 1, shadowed: 0, approvals: 0, externalOut: 0 }
};

const request = {
	id: 'msrq1abc',
	created: new Date().toISOString(),
	name: 'Mux',
	runtime: 'remote',
	remoteURL: 'https://mcp.mux.com/mcp',
	reason: 'Check video encoding status',
	requesterID: 'u2',
	requesterEmail: 'dev@obot.local',
	state: 'pending',
	suggestedVMCPID: 'vmcp2'
};

const vmcps = [
	{ id: 'vmcp1', displayName: 'Research Tools', created: '' },
	{ id: 'vmcp2', displayName: 'Engineering Tools', created: '' },
	{ id: 'vmcp3', displayName: 'Personal', userID: 'u1', created: '' }
];

async function renderAnswer(content: string, groups: string[] = [Group.ADMIN]) {
	await preparePageData({ profile: createMockProfile(groups) });
	return render(AIAnswer, { content });
}

describe('AIAnswer', () => {
	it('renders a session widget with its event stream between the markdown', async () => {
		worker.use(
			http.get('/api/provenance/sessions/sess1', () => HttpResponse.json(session)),
			http.get('/api/provenance/sessions/sess1/events', () =>
				HttpResponse.json({
					items: [
						{
							id: 'e1',
							sessionID: 'sess1',
							kind: 'tool_call',
							surface: 'mcp',
							summary: 'Called github.create_pull_request',
							at: new Date().toISOString()
						}
					],
					total: 1
				})
			)
		);
		await renderAnswer(
			`This one is the riskiest:\n${widget({ type: 'session', sessionId: 'sess1' })}`
		);

		await expect.element(page.getByText('This one is the riskiest:')).toBeInTheDocument();
		await expect
			.element(page.getByRole('link', { name: 'Refactor the billing service' }))
			.toBeInTheDocument();
		await expect.element(page.getByText('Called github.create_pull_request')).toBeInTheDocument();
		await expect.element(page.getByRole('link', { name: 'Open session' })).toBeInTheDocument();
	});

	it('shows a placeholder while a widget is still being written', async () => {
		await renderAnswer('Look:\n```obot-widget\n{"type":"sess');
		await expect.element(page.getByText('Preparing a view…')).toBeInTheDocument();
	});

	it('says when it cannot show a widget', async () => {
		await renderAnswer(widget({ type: 'chart' }));
		await expect
			.element(page.getByText("Obot AI tried to show a view it doesn't have."))
			.toBeInTheDocument();
	});

	it('lets an administrator approve a request into the suggested shared vMCP', async () => {
		const approved = vi.fn();
		worker.use(
			http.get('/api/mcp-server-requests/msrq1abc', () => HttpResponse.json(request)),
			http.get('/api/vmcps', () => HttpResponse.json({ items: vmcps })),
			http.post('/api/mcp-server-requests/msrq1abc/approve', async ({ request: r }) => {
				approved(await r.json());
				return HttpResponse.json({ ...request, state: 'approved', vmcpID: 'vmcp2' });
			})
		);
		await renderAnswer(widget({ type: 'mcp_server_request', requestId: 'msrq1abc' }));

		const select = page.getByRole('combobox', { name: 'Add to' });
		await expect.element(select).toHaveValue('vmcp2');
		await expect.element(page.getByRole('option', { name: 'Personal' })).not.toBeInTheDocument();
		await page.getByRole('button', { name: 'Approve' }).click();

		await vi.waitFor(() =>
			expect(approved).toHaveBeenCalledWith({ vmcpID: 'vmcp2', grantRequesterAccess: true })
		);
		await expect.element(page.getByText('Added to Engineering Tools')).toBeInTheDocument();
		await expect.element(page.getByRole('button', { name: 'Approve' })).not.toBeInTheDocument();
	});

	it('shows a request without decision buttons to a basic user', async () => {
		worker.use(http.get('/api/mcp-server-requests/msrq1abc', () => HttpResponse.json(request)));
		await renderAnswer(widget({ type: 'mcp_server_request', requestId: 'msrq1abc' }), [Group.USER]);

		await expect.element(page.getByText('Mux')).toBeInTheDocument();
		await expect.element(page.getByRole('button', { name: 'Approve' })).not.toBeInTheDocument();
		await expect.element(page.getByRole('button', { name: 'Deny' })).not.toBeInTheDocument();
	});

	it('lists everything waiting for approval and denies a request in place', async () => {
		const denied = vi.fn();
		worker.use(
			http.get('/api/provenance/approvals', () =>
				HttpResponse.json({
					items: [
						{
							id: 'ap1',
							sessionID: 'sess1',
							eventID: 'e1',
							policyID: 'p1',
							policyName: 'Hold pushes to main',
							action: { surface: 'mcp', kind: 'tool_call', tool: 'push', server: 'github' },
							reason: 'Pushes to main need a second pair of eyes',
							createdAt: new Date().toISOString(),
							expiresAt: new Date(Date.now() + 3600_000).toISOString(),
							status: 'pending',
							risk: 60
						}
					],
					total: 1
				})
			),
			http.get('/api/mcp-server-requests', () => HttpResponse.json({ items: [request] })),
			http.get('/api/vmcps', () => HttpResponse.json({ items: vmcps })),
			http.post('/api/mcp-server-requests/msrq1abc/deny', async ({ request: r }) => {
				denied(await r.json());
				return HttpResponse.json({ ...request, state: 'denied' });
			})
		);
		await renderAnswer(widget({ type: 'approvals' }));

		await expect.element(page.getByText('Held agent actions (1)')).toBeInTheDocument();
		await expect.element(page.getByText('Hold pushes to main')).toBeInTheDocument();
		await expect.element(page.getByText('MCP server requests (1)')).toBeInTheDocument();

		await page.getByRole('listitem').getByRole('button', { name: 'Deny' }).click();
		await vi.waitFor(() => expect(denied).toHaveBeenCalled());
		await expect.element(page.getByText('Denied')).toBeInTheDocument();
	});

	it('says when nothing is waiting', async () => {
		worker.use(
			http.get('/api/provenance/approvals', () => HttpResponse.json({ items: [], total: 0 })),
			http.get('/api/mcp-server-requests', () => HttpResponse.json({ items: [] }))
		);
		await renderAnswer(widget({ type: 'approvals' }));
		await expect
			.element(page.getByText('Nothing is waiting for your approval.'))
			.toBeInTheDocument();
	});
});
