import { page as appPage } from '$app/state';
import { Group } from '$lib/services';
import { createMockProfile, preparePageData } from '../../tests/helpers/pageData';
import { worker } from '../../tests/mocks/worker';
import AuditLogsPage from './+page.svelte';
import { http, HttpResponse } from 'msw';
import { describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

// The pages load their users after a navigation, which a component test never makes, so it is made on mount.
vi.mock('$app/navigation', () => ({
	goto: vi.fn(),
	invalidate: vi.fn(),
	invalidateAll: vi.fn(),
	preloadCode: vi.fn(),
	preloadData: vi.fn(),
	beforeNavigate: vi.fn(),
	afterNavigate: (callback: (navigation: unknown) => void) => callback({}),
	onNavigate: vi.fn(),
	pushState: vi.fn(),
	replaceState: vi.fn(),
	disableScrollHandling: vi.fn()
}));

async function renderAuditLogsPage(groups: string[] = [Group.ADMIN], view = 'llm') {
	(appPage.url as URL).searchParams.set('view', view);
	await preparePageData({ profile: createMockProfile(groups) });
	return render(AuditLogsPage);
}

describe('Audit logs page rows of an agent that acted for a user', () => {
	function mockUsers() {
		worker.use(
			http.get('*/api/users', () =>
				HttpResponse.json({ items: [{ id: '3', displayName: 'Ada Admin', status: 'active' }] })
			),
			http.get('*/api/agents', () =>
				HttpResponse.json({
					items: [{ id: '7', displayName: 'Build Bot', status: 'active', activeAPIKeyCount: 1 }]
				})
			)
		);
	}

	async function expectAgentForUser() {
		const agentLink = page.getByRole('link', { name: 'Build Bot' });
		await expect.element(agentLink).toBeVisible();
		await expect
			.element(agentLink)
			.toHaveAttribute('href', expect.stringContaining('/identity-access/agent-identities/7'));
		const userLink = page.getByRole('link', { name: 'Ada Admin' });
		await expect.element(userLink).toBeVisible();
		await expect.element(userLink).toHaveAttribute('href', expect.stringContaining('view=users'));
		// Between the two links the row reads "for".
		await expect
			.poll(() => agentLink.element().parentElement?.textContent?.replace(/\s+/g, ' ').trim())
			.toBe('Build Bot for Ada Admin');
	}

	it('reads "<agent> for <user>" in an LLM audit row, linking to both', async () => {
		mockUsers();
		worker.use(
			http.get('/api/llm-audit-logs', () =>
				HttpResponse.json({
					items: [
						{
							id: 1,
							createdAt: '2026-01-01T00:00:00Z',
							userID: '',
							principal: { kind: 'agent', id: '7' },
							onBehalfOf: { kind: 'user', id: '3' },
							agentDisplayName: 'Build Bot',
							modelProvider: 'openai',
							targetModel: 'gpt',
							responseStatus: 200
						}
					],
					total: 1
				})
			)
		);
		await renderAuditLogsPage([Group.ADMIN], 'llm');

		await expectAgentForUser();
	});

	it('reads "<agent> for <user>" in an MCP audit row, linking to both', async () => {
		mockUsers();
		worker.use(
			http.get('/api/mcp-audit-logs', () =>
				HttpResponse.json({
					items: [
						{
							id: 1,
							timestamp: {
								occurredAt: '2026-01-01T00:00:00Z',
								recordedAt: '2026-01-01T00:00:00Z',
								source: 'gateway'
							},
							eventType: 'mcp_call',
							actor: {
								actorType: 'agent',
								id: '7',
								displayName: 'Build Bot',
								onBehalfOf: { kind: 'user', id: '3' }
							},
							action: { operation: 'tools/call', name: 'send' },
							target: { targetType: 'mcp_server', id: 'ms1', name: 'Mail' },
							outcome: { status: 'success', durationMs: 5 },
							client: 'cli'
						}
					],
					total: 1
				})
			)
		);
		await renderAuditLogsPage([Group.ADMIN], 'mcp');

		await expectAgentForUser();
	});
});
