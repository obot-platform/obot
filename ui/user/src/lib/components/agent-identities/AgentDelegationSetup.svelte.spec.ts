import * as navigation from '$lib/navigation';
import { Group } from '$lib/services';
import type {
	AgentDelegationSetupScope,
	AgentDelegationSetupVMCP
} from '$lib/services/agent-identities/types';
import { createMockProfile, preparePageData } from '../../../tests/helpers/pageData';
import { createDelegationSetup } from '../../../tests/mocks/agentIdentities';
import { worker } from '../../../tests/mocks/worker';
import AgentDelegationSetup from './AgentDelegationSetup.svelte';
import { http, HttpResponse } from 'msw';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

vi.mock(import('$lib/navigation'), { spy: true });

const request: AgentDelegationSetupScope = { kind: 'request', requestId: 'adr1req' };
const delegation: AgentDelegationSetupScope = { kind: 'delegation', delegationId: 'adl1mine' };
const requestPrefix = '/api/agent-delegation-requests/adr1req/setup';
const delegationPrefix = '/api/me/agent-delegations/adl1mine/setup';

function dev(): AgentDelegationSetupVMCP {
	return createDelegationSetup().vmcps[0];
}

function mockSetup(prefix: string) {
	worker.use(http.get(prefix, () => HttpResponse.json(createDelegationSetup())));
}

beforeEach(async () => {
	vi.mocked(navigation.navigateTo).mockImplementation(() => {});
	await preparePageData({ profile: createMockProfile([Group.USER]) });
});

afterEach(() => {
	vi.mocked(navigation.navigateTo).mockRestore();
});

describe('AgentDelegationSetup', () => {
	it('lists what needs setup, with progress, and only whether values are set', async () => {
		mockSetup(requestPrefix);
		render(AgentDelegationSetup, { scope: request });

		await expect.element(page.getByText('1 of 2 vMCPs ready')).toBeVisible();
		const tools = page.getByRole('region', { name: 'Developer tools' });
		await expect.element(tools.getByRole('group', { name: 'GitHub' })).toBeVisible();
		await expect.element(tools.getByRole('group', { name: 'Slack' })).toBeVisible();
		// A component that needs nothing from the user is ready, with nothing to do.
		await expect.element(tools.getByRole('group', { name: 'Clock' })).not.toBeInTheDocument();
		await expect.element(tools.getByText('Ready, nothing to do: Clock')).toBeVisible();

		const github = tools.getByRole('group', { name: 'GitHub' });
		await expect.element(github.getByText('Token is required.')).toBeVisible();
		await expect.element(github.getByText('Not set')).toBeVisible();
		await expect.element(github.getByText('Set', { exact: true })).toBeVisible();

		const docs = page.getByRole('region', { name: 'Docs' });
		await expect.element(docs.getByText('Account connected')).toBeVisible();
	});

	it('says when a component uses the account the vMCP preconfigured, with nothing to connect', async () => {
		const setup = createDelegationSetup();
		setup.vmcps[0].components[0].oauth = 'preconfigured';
		worker.use(http.get(requestPrefix, () => HttpResponse.json(setup)));
		render(AgentDelegationSetup, { scope: request });

		const github = page
			.getByRole('region', { name: 'Developer tools' })
			.getByRole('group', { name: 'GitHub' });
		await expect
			.element(
				github.getByText(
					'Uses the account an administrator preconfigured for everyone who uses this vMCP.'
				)
			)
			.toBeVisible();
		await expect.element(github.getByRole('button', { name: 'Connect' })).not.toBeInTheDocument();
	});

	it('saves only the values entered, through the scope of the setup', async () => {
		mockSetup(requestPrefix);
		const saved = vi.fn();
		worker.use(
			http.put(`${requestPrefix}/vmcps/vmcp1dev/configuration`, async ({ request }) => {
				saved(await request.json());
				const next = dev();
				next.components[0].configuration[0].set = true;
				next.components[0].reasons = [];
				next.components[0].ready = true;
				return HttpResponse.json(next);
			})
		);
		render(AgentDelegationSetup, { scope: request });

		const tools = page.getByRole('region', { name: 'Developer tools' });
		await tools.getByRole('button', { name: 'Set values' }).click();
		// A value that is set is never shown: its field starts empty, and empty keeps it.
		await expect.element(page.getByRole('textbox', { name: /Organization/ })).toHaveValue('');
		await expect
			.element(page.getByText('A value is set; leave this empty to keep it.'))
			.toBeVisible();
		await page.getByLabelText('Token').fill('new-token');
		await page.getByRole('button', { name: 'Save' }).click();

		await vi.waitFor(() =>
			expect(saved).toHaveBeenCalledWith({ components: { 'c-github': { TOKEN: 'new-token' } } })
		);
		await expect
			.element(tools.getByRole('group', { name: 'GitHub' }).getByText('Not set'))
			.not.toBeInTheDocument();
	});

	it('clears a value that is set', async () => {
		mockSetup(requestPrefix);
		const saved = vi.fn();
		worker.use(
			http.put(`${requestPrefix}/vmcps/vmcp1dev/configuration`, async ({ request }) => {
				saved(await request.json());
				const next = dev();
				next.components[0].configuration[1].set = false;
				return HttpResponse.json(next);
			})
		);
		render(AgentDelegationSetup, { scope: request });

		await page.getByRole('button', { name: 'Clear Organization' }).click();

		await vi.waitFor(() =>
			expect(saved).toHaveBeenCalledWith({ components: { 'c-github': { ORG: null } } })
		);
		await expect
			.element(page.getByRole('button', { name: 'Clear Organization' }))
			.not.toBeInTheDocument();
	});

	it('connects an account by going to the URL the server returns', async () => {
		mockSetup(requestPrefix);
		const started = vi.fn();
		worker.use(
			http.post(`${requestPrefix}/vmcps/vmcp1dev/components/c-slack/oauth`, () => {
				started();
				return HttpResponse.json({ authURL: 'https://slack.example.com/oauth?state=abc' });
			})
		);
		render(AgentDelegationSetup, { scope: request });

		await page
			.getByRole('group', { name: 'Slack' })
			.getByRole('button', { name: 'Connect' })
			.click();

		await vi.waitFor(() =>
			expect(navigation.navigateTo).toHaveBeenCalledWith(
				'https://slack.example.com/oauth?state=abc'
			)
		);
		expect(started).toHaveBeenCalledOnce();
	});

	it('never opens an authorization URL that is not a web page', async () => {
		mockSetup(requestPrefix);
		worker.use(
			http.post(`${requestPrefix}/vmcps/vmcp1dev/components/c-slack/oauth`, () =>
				HttpResponse.json({ authURL: 'javascript:alert(1)' })
			)
		);
		render(AgentDelegationSetup, { scope: request });

		await page
			.getByRole('group', { name: 'Slack' })
			.getByRole('button', { name: 'Connect' })
			.click();

		await expect
			.element(page.getByText('The account connection could not be started.'))
			.toBeVisible();
		expect(navigation.navigateTo).not.toHaveBeenCalled();
	});

	it('runs a localhost-only login through the CLI instead of opening it', async () => {
		mockSetup(requestPrefix);
		worker.use(
			http.post(`${requestPrefix}/vmcps/vmcp1dev/components/c-slack/oauth`, () =>
				HttpResponse.json({ authURL: '/mcp-connect/login?id=1', localhostCallback: true })
			)
		);
		render(AgentDelegationSetup, { scope: request });

		const slack = page.getByRole('group', { name: 'Slack' });
		await slack.getByRole('button', { name: 'Connect' }).click();

		await expect.element(slack.getByRole('button', { name: 'Authenticate' })).toBeVisible();
		expect(navigation.navigateTo).not.toHaveBeenCalled();
	});

	it('disconnects an account through the delegation, after a confirmation', async () => {
		mockSetup(delegationPrefix);
		const disconnected = vi.fn();
		worker.use(
			http.delete(`${delegationPrefix}/vmcps/vmcp1docs/components/c-notion/oauth`, () => {
				disconnected();
				const next = createDelegationSetup().vmcps[1];
				next.components[0].oauth = 'needsAuthorization';
				next.ready = false;
				return HttpResponse.json(next);
			})
		);
		render(AgentDelegationSetup, { scope: delegation });

		const docs = page.getByRole('region', { name: 'Docs' });
		await docs.getByRole('button', { name: 'Disconnect' }).click();
		await expect.element(page.getByText('Disconnect your account for Notion?')).toBeVisible();
		expect(disconnected).not.toHaveBeenCalled();
		await page.getByRole('button', { name: 'Disconnect' }).last().click();

		await vi.waitFor(() => expect(disconnected).toHaveBeenCalledOnce());
		await expect.element(docs.getByText('No account connected')).toBeVisible();
		await expect.element(page.getByText('0 of 2 vMCPs ready')).toBeVisible();
	});
});
