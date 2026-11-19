import type { AgentVMCPReadiness } from '$lib/services/agent-identities/types';
import { emptyAgentAccess } from '../../../tests/mocks/agentIdentities';
import { worker } from '../../../tests/mocks/worker';
import AgentAccessReadiness from './AgentAccessReadiness.svelte';
import { http, HttpResponse } from 'msw';
import { describe, expect, it } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

const notReady: AgentVMCPReadiness = {
	vmcpID: 'vmcp1mail',
	vmcpDisplayName: 'Mail vMCP',
	ready: false,
	disabled: false,
	reasons: [
		{ code: 'configuration_required', componentID: 'gmail', field: 'TENANT' },
		{ code: 'oauth_required', componentID: 'gmail' },
		{ code: 'oauth_invalid', componentID: 'calendar' }
	]
};

function mockAgent(readiness: AgentVMCPReadiness[]) {
	worker.use(
		http.get('/api/agents/7/access', () =>
			HttpResponse.json({
				...emptyAgentAccess,
				vmcpProfiles: [
					{ vmcpID: 'vmcp1mail', profileID: 'vp1own', profileName: 'Build bots' },
					{ vmcpID: 'vmcp1mail', profileID: 'vp1more', profileName: 'More bots' }
				],
				modelAccessPolicies: [{ id: 'map1', displayName: 'Bot models' }],
				skillAccessRules: [{ id: 'sar1', displayName: 'Bot skills' }]
			})
		),
		http.get('/api/agents/7/vmcps', () => HttpResponse.json({ items: readiness }))
	);
}

describe('AgentAccessReadiness', () => {
	// Every policy listed names the agent by ID, so none is marked with how it matches.
	it('lists the profiles and policies that name the agent', async () => {
		mockAgent([notReady]);
		render(AgentAccessReadiness, { agentId: '7' });

		// An agent may be in several profiles of one vMCP, and gets the union of what they grant.
		await expect.element(page.getByText('vmcp1mail · Build bots')).toBeVisible();
		await expect.element(page.getByText('vmcp1mail · More bots')).toBeVisible();
		await expect.element(page.getByText('Bot models')).toBeVisible();
		await expect.element(page.getByText('Bot skills')).toBeVisible();
		await expect.element(page.getByText('Named', { exact: true })).not.toBeInTheDocument();
		await expect.element(page.getByText('All agents', { exact: true })).not.toBeInTheDocument();
		await expect.element(page.getByText('Everyone', { exact: true })).not.toBeInTheDocument();
	});

	it("links each reason to the vMCP's agent setup, at the component or button it is about", async () => {
		mockAgent([notReady]);
		render(AgentAccessReadiness, { agentId: '7' });

		// Agents get only the values the vMCP or its catalog entry fix, so that is where it is fixed, or the
		// component stops being granted to the agent.
		await expect
			.element(
				page.getByText(
					"gmail needs TENANT, which agents can't set: fix it on the vMCP or its catalog entry, or stop granting gmail to the agent"
				)
			)
			.toBeVisible();
		const links = page.getByRole('link', { name: "Fix in the vMCP's agent setup" });
		await expect
			.element(links.first())
			.toHaveAttribute(
				'href',
				`/vmcps/vmcp1mail?view=agent-setup#${encodeURIComponent('agent-component-gmail')}`
			);
		await expect
			.element(links.nth(1))
			.toHaveAttribute(
				'href',
				`/vmcps/vmcp1mail?view=agent-setup#${encodeURIComponent('agent-oauth-gmail')}`
			);
		// An invalid grant is fixed by authorizing it again on the vMCP, never in a profile.
		await expect
			.element(links.nth(2))
			.toHaveAttribute(
				'href',
				`/vmcps/vmcp1mail?view=agent-setup#${encodeURIComponent('agent-oauth-calendar')}`
			);
		await expect.element(page.getByRole('link', { name: /profile/ })).not.toBeInTheDocument();
	});

	it('names components as the vMCP does, falling back to their IDs', async () => {
		mockAgent([
			{
				...notReady,
				reasons: [...notReady.reasons, { code: 'oauth_required', componentID: 'gone' }]
			}
		]);
		worker.use(
			http.get('/api/vmcps/vmcp1mail', () =>
				HttpResponse.json({
					id: 'vmcp1mail',
					components: [
						{ id: 'gmail', name: 'Gmail' },
						{
							mcpServerCatalogEntryID: 'calendar',
							catalogEntry: { manifest: { name: 'Calendar' } }
						}
					]
				})
			)
		);
		render(AgentAccessReadiness, { agentId: '7' });

		await expect
			.element(page.getByText(/^Gmail needs TENANT, which agents can't set/))
			.toBeVisible();
		await expect.element(page.getByText('Gmail needs OAuth authorization')).toBeVisible();
		await expect
			.element(page.getByText(/The OAuth grant for Calendar no longer works/))
			.toBeVisible();
		await expect.element(page.getByText(/^gmail /)).not.toBeInTheDocument();

		// A component the vMCP no longer has keeps its ID.
		await expect.element(page.getByText('gone needs OAuth authorization')).toBeVisible();
	});
});
