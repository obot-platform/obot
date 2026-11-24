import { Group } from '$lib/services';
import { createMockProfile, preparePageData } from '../../../tests/helpers/pageData';
import { worker } from '../../../tests/mocks/worker';
import AgentDelegations from './AgentDelegations.svelte';
import { http, HttpResponse } from 'msw';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

const agent = { id: '7', displayName: 'Build Bot', status: 'active' };
const delegations = [
	{
		id: 'adl1ada',
		agent,
		user: { id: 'u1', email: 'ada@example.com', displayName: 'Ada Admin' },
		firstApprovedAt: '2026-01-01T00:00:00Z',
		lastApprovedAt: '2026-01-02T00:00:00Z',
		lastUsedAt: '2026-01-03T00:00:00Z'
	},
	{
		id: 'adl1bob',
		agent,
		user: { id: 'u2', email: 'bob@example.com' },
		firstApprovedAt: '2026-01-01T00:00:00Z',
		lastApprovedAt: '2026-01-01T00:00:00Z'
	}
];
const revoked = vi.fn();

beforeEach(async () => {
	revoked.mockReset();
	worker.use(
		http.get('/api/agents/7/delegations', () => HttpResponse.json({ items: delegations })),
		http.delete('/api/agents/7/delegations/:id', ({ params }) => {
			revoked(params.id);
			return new HttpResponse(null, { status: 204 });
		})
	);
	await preparePageData({ profile: createMockProfile([Group.ADMIN]) });
});

describe('AgentDelegations', () => {
	it('lists the users who approved the agent, with when', async () => {
		render(AgentDelegations, { agentId: '7', agentName: 'Build Bot' });

		await expect.element(page.getByText('Ada Admin')).toBeVisible();
		// A user without a name is shown by email.
		await expect.element(page.getByText('bob@example.com')).toBeVisible();
		// An agent that never got a token from a delegation says so.
		await expect.element(page.getByRole('row', { name: /bob@example.com/ })).toBeVisible();
		await expect.poll(() => page.getByText('Never').elements().length).toBe(1);
	});

	it('says when no user approved the agent', async () => {
		worker.use(http.get('/api/agents/7/delegations', () => HttpResponse.json({ items: [] })));
		render(AgentDelegations, { agentId: '7', agentName: 'Build Bot' });

		await expect.element(page.getByText('No user has approved this agent.')).toBeVisible();
	});

	it('revokes a delegation only after a confirmation that explains it', async () => {
		render(AgentDelegations, { agentId: '7', agentName: 'Build Bot' });

		await page
			.getByRole('row', { name: /Ada Admin/ })
			.getByRole('button', { name: 'Revoke' })
			.click();
		await expect
			.element(page.getByText('Revoke Ada Admin\'s approval of "Build Bot"?'))
			.toBeVisible();
		await expect
			.element(page.getByText(/deletes what they set up through the agent/))
			.toBeVisible();
		expect(revoked).not.toHaveBeenCalled();

		await page.getByRole('button', { name: 'Revoke', exact: true }).last().click();

		await vi.waitFor(() => expect(revoked).toHaveBeenCalledWith('adl1ada'));
		await expect.element(page.getByText('Ada Admin')).not.toBeInTheDocument();
		await expect.element(page.getByText('bob@example.com')).toBeVisible();
	});

	it('is read-only for read-only admins and auditors', async () => {
		render(AgentDelegations, { agentId: '7', agentName: 'Build Bot', readonly: true });

		await expect.element(page.getByText('Ada Admin')).toBeVisible();
		await expect.element(page.getByRole('button', { name: 'Revoke' })).not.toBeInTheDocument();
	});
});
