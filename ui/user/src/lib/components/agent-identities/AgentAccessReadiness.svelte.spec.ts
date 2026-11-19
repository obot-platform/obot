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
	profileID: 'vp1own',
	reasons: [
		{ code: 'configuration_required', componentID: 'gmail', field: 'TENANT', profileID: 'vp1own' },
		{ code: 'oauth_required', componentID: 'gmail', profileID: 'vp1own' },
		{ code: 'oauth_invalid', componentID: 'calendar', profileID: 'vp1own' }
	],
	pendingSync: false
};

function mockAgent(readiness: AgentVMCPReadiness[]) {
	worker.use(
		http.get('/api/agents/7/access', () =>
			HttpResponse.json({
				...emptyAgentAccess,
				vmcpProfiles: [{ vmcpID: 'vmcp1mail', profileID: 'vp1own', profileName: 'Build bots' }],
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

		await expect.element(page.getByText('vmcp1mail · Build bots')).toBeVisible();
		await expect.element(page.getByText('Bot models')).toBeVisible();
		await expect.element(page.getByText('Bot skills')).toBeVisible();
		await expect.element(page.getByText('Named', { exact: true })).not.toBeInTheDocument();
		await expect.element(page.getByText('All agents', { exact: true })).not.toBeInTheDocument();
		await expect.element(page.getByText('Everyone', { exact: true })).not.toBeInTheDocument();
	});

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
		await expect
			.element(page.getByRole('link', { name: 'Fix in profile Build bots' }).nth(1))
			.toHaveAttribute(
				'href',
				`/vmcps/vmcp1mail?view=profiles&profileID=vp1own#${encodeURIComponent('agent-oauth-gmail')}`
			);
		// An invalid grant is fixed by authorizing it again in the same profile.
		await expect
			.element(page.getByRole('link', { name: 'Fix in profile Build bots' }).nth(2))
			.toHaveAttribute(
				'href',
				`/vmcps/vmcp1mail?view=profiles&profileID=vp1own#${encodeURIComponent('agent-oauth-calendar')}`
			);
	});

	it('shows which profile each value comes from', async () => {
		mockAgent([notReady]);
		worker.use(
			http.get('/api/agents/7/vmcps/vmcp1mail/readiness', () =>
				HttpResponse.json({
					...notReady,
					sources: [
						{ kind: 'configuration', componentID: 'gmail', field: 'REGION', profileID: 'vp1own' },
						{ kind: 'oauth', componentID: 'gmail', profileID: 'vp1own' }
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
				page
					.getByRole('listitem')
					.filter({ hasText: 'gmail: OAuth authorization, from profile Build bots' })
					.last()
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
