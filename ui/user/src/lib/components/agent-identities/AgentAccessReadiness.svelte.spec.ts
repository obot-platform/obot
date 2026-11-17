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
	explicitProfileID: 'vp1own',
	wildcardProfileID: 'vp1all',
	reasons: [
		{ code: 'configuration_required', componentID: 'gmail', field: 'TENANT', profileID: 'vp1own' },
		{ code: 'oauth_required', componentID: 'gmail', profileID: 'vp1all' },
		{
			code: 'oauth_invalid',
			componentID: 'calendar',
			profileID: 'vp1own',
			urlProfileID: 'vp1all'
		}
	],
	pendingSync: false
};

function mockAgent(readiness: AgentVMCPReadiness[]) {
	worker.use(
		http.get('/api/agents/7/access', () =>
			HttpResponse.json({
				...emptyAgentAccess,
				vmcpProfiles: [
					{
						vmcpID: 'vmcp1mail',
						profileID: 'vp1own',
						profileName: 'Build bots',
						match: 'explicit'
					},
					{
						vmcpID: 'vmcp1mail',
						profileID: 'vp1all',
						profileName: 'All agents',
						match: 'kindWildcard'
					}
				]
			})
		),
		http.get('/api/agents/7/vmcps', () => HttpResponse.json({ items: readiness }))
	);
}

describe('AgentAccessReadiness', () => {
	it('links each reason to the profile that fixes it, at the field or button', async () => {
		mockAgent([notReady]);
		render(AgentAccessReadiness, { agentId: '7' });

		await expect.element(page.getByText('gmail needs a configuration value: TENANT')).toBeVisible();
		const fieldLink = page.getByRole('link', { name: 'Fix in profile Build bots' }).first();
		await expect
			.element(fieldLink)
			.toHaveAttribute(
				'href',
				`/vmcps/vmcp1mail?view=profiles&profileID=vp1own#${encodeURIComponent('agent-config-gmail-TENANT')}`
			);
		// A reason that falls back to the wildcard profile names that profile.
		await expect
			.element(page.getByRole('link', { name: 'Fix in profile All agents' }))
			.toHaveAttribute(
				'href',
				`/vmcps/vmcp1mail?view=profiles&profileID=vp1all#${encodeURIComponent('agent-oauth-gmail')}`
			);
		// An invalid grant authorized for another URL also points at the profile that sets the URL.
		await expect
			.element(
				page.getByRole('link', { name: /URL it was authorized for is set in profile All agents/ })
			)
			.toHaveAttribute('href', '/vmcps/vmcp1mail?view=profiles&profileID=vp1all#agent-setup');
	});

	it('tells the admin what to do about an ambiguous selection, without a link', async () => {
		mockAgent([
			{
				...notReady,
				reasons: [{ code: 'selection_ambiguous' }]
			}
		]);
		render(AgentAccessReadiness, { agentId: '7' });

		await expect.element(page.getByText(/More than one profile names every agent/)).toBeVisible();
		await expect
			.element(page.getByText('An administrator must leave one profile that names all agents.'))
			.toBeVisible();
		await expect.element(page.getByRole('link', { name: /Fix in/ })).not.toBeInTheDocument();
	});

	it('shows where values come from, marking the fallback profile', async () => {
		mockAgent([notReady]);
		worker.use(
			http.get('/api/agents/7/vmcps/vmcp1mail/readiness', () =>
				HttpResponse.json({
					...notReady,
					sources: [
						{ kind: 'configuration', componentID: 'gmail', field: 'REGION', profileID: 'vp1own' },
						{ kind: 'oauth', componentID: 'gmail', profileID: 'vp1all', fallback: true }
					]
				})
			)
		);
		render(AgentAccessReadiness, { agentId: '7' });

		await page.getByRole('button', { name: 'Show where its values come from' }).click();
		await expect
			.element(
				page
					.getByRole('listitem')
					.filter({ hasText: 'gmail: REGION, from profile Build bots' })
					.last()
			)
			.toBeVisible();
		await expect
			.element(
				page.getByRole('link', { name: 'from profile All agents (the fallback for all agents)' })
			)
			.toBeVisible();
		await page.getByRole('button', { name: 'Hide sources' }).click();
		await expect
			.element(page.getByRole('link', { name: 'from profile Build bots' }))
			.not.toBeInTheDocument();
	});

	it('names components as the vMCP does, falling back to their IDs', async () => {
		mockAgent([notReady]);
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
			),
			http.get('/api/agents/7/vmcps/vmcp1mail/readiness', () =>
				HttpResponse.json({
					...notReady,
					sources: [
						{ kind: 'oauth', componentID: 'gmail', profileID: 'vp1own' },
						{ kind: 'oauth', componentID: 'gone', profileID: 'vp1own' }
					]
				})
			)
		);
		render(AgentAccessReadiness, { agentId: '7' });

		await expect.element(page.getByText('Gmail needs a configuration value: TENANT')).toBeVisible();
		await expect.element(page.getByText('Gmail needs OAuth authorization')).toBeVisible();
		await expect
			.element(page.getByText(/The OAuth grant for Calendar no longer works/))
			.toBeVisible();
		await expect.element(page.getByText(/^gmail /)).not.toBeInTheDocument();

		await page.getByRole('button', { name: 'Show where its values come from' }).click();
		await expect
			.element(page.getByRole('listitem').filter({ hasText: 'Gmail: OAuth authorization' }).last())
			.toBeVisible();
		// A component the vMCP no longer has keeps its ID.
		await expect
			.element(page.getByRole('listitem').filter({ hasText: 'gone: OAuth authorization' }).last())
			.toBeVisible();
	});

	it('says so when the sources cannot be loaded', async () => {
		mockAgent([notReady]);
		worker.use(
			http.get('/api/agents/7/vmcps/vmcp1mail/readiness', () =>
				HttpResponse.json({ error: 'nope' }, { status: 500 })
			)
		);
		render(AgentAccessReadiness, { agentId: '7' });

		await page.getByRole('button', { name: 'Show where its values come from' }).click();
		await expect.element(page.getByText('Failed to load the sources.')).toBeVisible();
	});
});
