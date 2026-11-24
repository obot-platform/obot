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
	],
	preconfiguredComponents: []
};

// The vMCP editor's names for the components of vmcp1mail.
function mockMailVMCP() {
	worker.use(
		http.get('/api/vmcps/vmcp1mail', () =>
			HttpResponse.json({
				id: 'vmcp1mail',
				components: [
					{ id: 'gmail', name: 'Gmail' },
					{ id: 'drive', name: 'Drive' },
					{
						mcpServerCatalogEntryID: 'calendar',
						catalogEntry: { manifest: { name: 'Calendar' } }
					}
				]
			})
		)
	);
}

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

	it("links each reason to the component's settings in the vMCP editor, at the part it is about", async () => {
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
		const links = page.getByRole('link', { name: "Fix in the component's settings on the vMCP" });
		await expect
			.element(links.first())
			.toHaveAttribute('href', '/vmcps/vmcp1mail?component=gmail&settings=configuration');
		await expect
			.element(links.nth(1))
			.toHaveAttribute('href', '/vmcps/vmcp1mail?component=gmail&settings=oauth');
		// An invalid grant is fixed by authorizing the component's preconfigured OAuth again, never in a
		// profile.
		await expect
			.element(links.nth(2))
			.toHaveAttribute('href', '/vmcps/vmcp1mail?component=calendar&settings=oauth');
		await expect.element(page.getByRole('link', { name: /profile/ })).not.toBeInTheDocument();
		await expect.element(page.getByRole('link', { name: /agent setup/i })).not.toBeInTheDocument();
	});

	it('links a reason about no component to the vMCP editor', async () => {
		mockAgent([{ ...notReady, reasons: [{ code: 'oauth_required' }] }]);
		render(AgentAccessReadiness, { agentId: '7' });

		await expect
			.element(page.getByRole('link', { name: "Fix in the component's settings on the vMCP" }))
			.toHaveAttribute('href', '/vmcps/vmcp1mail');
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
			.element(page.getByText(/The preconfigured OAuth for Calendar no longer works/))
			.toBeVisible();
		await expect.element(page.getByText(/^gmail /)).not.toBeInTheDocument();

		// A component the vMCP no longer has keeps its ID.
		await expect.element(page.getByText('gone needs OAuth authorization')).toBeVisible();
	});

	// Only components with working preconfigured OAuth are listed: a broken one is a reason instead.
	it('lists the components whose OAuth is preconfigured, named as the vMCP does', async () => {
		mockAgent([
			{
				...notReady,
				ready: true,
				reasons: [],
				preconfiguredComponents: ['drive']
			}
		]);
		mockMailVMCP();
		render(AgentAccessReadiness, { agentId: '7' });

		await expect.element(page.getByText('Ready', { exact: true })).toBeVisible();
		// The vMCP is loaded for the names even though no reason names a component.
		await expect.element(page.getByText('Drive', { exact: true })).toBeVisible();
		await expect.element(page.getByText('Preconfigured', { exact: true })).toBeVisible();
		await expect.element(page.getByText('drive', { exact: true })).not.toBeInTheDocument();
		await expect.element(page.getByRole('link')).not.toBeInTheDocument();
	});

	it('marks no component as preconfigured when none is', async () => {
		mockAgent([notReady]);
		mockMailVMCP();
		render(AgentAccessReadiness, { agentId: '7' });

		await expect
			.element(page.getByText(/The preconfigured OAuth for Calendar no longer works/))
			.toBeVisible();
		await expect.element(page.getByText('Preconfigured', { exact: true })).not.toBeInTheDocument();
	});
});
