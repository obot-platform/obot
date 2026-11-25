import * as navigation from '$lib/navigation';
import { Group } from '$lib/services';
import type { AgentDelegationApprovalRequest } from '$lib/services/agent-identities/types';
import { createMockProfile, preparePageData } from '../../../../tests/helpers/pageData';
import {
	createDelegationRequest,
	createDelegationSetup
} from '../../../../tests/mocks/agentIdentities';
import { worker } from '../../../../tests/mocks/worker';
import type { PageData } from './$types';
import ApprovePage from './+page.svelte';
import { http, HttpResponse } from 'msw';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

vi.mock(import('$lib/navigation'), { spy: true });

const prefix = '/api/agent-delegation-requests/adr1req';
const returnURL = 'https://agent.example.com/callback?requestID=adr1req&code=c0de';

async function renderPage(
	overrides: {
		request?: AgentDelegationApprovalRequest;
		error?: string;
		oauthError?: string | null;
	} = {}
) {
	const profile = {
		...createMockProfile([Group.USER]),
		displayName: 'Ada Lovelace',
		email: 'ada@example.com'
	};
	const data = await preparePageData<PageData>({
		profile,
		returnPath: '/agents/approve/adr1req',
		oauthError: null,
		...('error' in overrides ? {} : { request: createDelegationRequest() }),
		...overrides
	});
	return render(ApprovePage, { data, params: { requestID: 'adr1req' } });
}

function mockSetup(ready = false) {
	const setup = createDelegationSetup();
	if (ready) {
		setup.vmcps = [setup.vmcps[1]];
	}
	worker.use(http.get(`${prefix}/setup`, () => HttpResponse.json(setup)));
}

beforeEach(() => {
	vi.mocked(navigation.navigateTo).mockImplementation(() => {});
});

afterEach(() => {
	vi.mocked(navigation.navigateTo).mockRestore();
});

describe('agent approval page', () => {
	it('shows the agent, the signed-in user and what the agent gets', async () => {
		mockSetup();
		await renderPage();

		await expect
			.element(page.getByRole('heading', { name: 'Let Build Bot act for you?' }))
			.toBeVisible();
		await expect.element(page.getByText('Runs builds')).toBeVisible();
		await expect.element(page.getByText(/You're signed in as Ada Lovelace\./)).toBeVisible();
		await expect.element(page.getByText('(ada@example.com)')).toBeVisible();
		await expect
			.element(page.getByText(/Build Bot will act as you, within the limits/))
			.toBeVisible();

		const access = page.getByRole('region', { name: 'What Build Bot will be able to do for you' });
		await expect.element(access.getByText(/^Developer tools/)).toBeVisible();
		await expect.element(access.getByText('(Read only)')).toBeVisible();
		await expect.element(access.getByText('Models under: Team models')).toBeVisible();
		// Skills: none are granted.
		await expect.element(access.getByText('None')).toBeVisible();
	});

	it('says when the user approved the agent before, and shows what is set', async () => {
		mockSetup();
		await renderPage({ request: createDelegationRequest({ delegationID: 'adl1mine' }) });

		await expect.element(page.getByText(/You approved this agent before/)).toBeVisible();
		await expect.element(page.getByRole('button', { name: 'Clear Organization' })).toBeVisible();
	});

	it('asks before approving with missing setup, then goes to the URL the server returns', async () => {
		mockSetup();
		const approved = vi.fn();
		worker.use(
			http.post(`${prefix}/approve`, () => {
				approved();
				return HttpResponse.json({ redirectURL: returnURL });
			})
		);
		await renderPage();
		await expect.element(page.getByText('1 of 2 vMCPs ready')).toBeVisible();

		await page.getByRole('button', { name: 'Approve' }).click();
		await expect
			.element(
				page.getByText('Some setup is missing; the agent will ask you again when it needs it.')
			)
			.toBeVisible();
		expect(approved).not.toHaveBeenCalled();

		await page.getByRole('button', { name: 'Approve' }).last().click();
		await vi.waitFor(() => expect(navigation.navigateTo).toHaveBeenCalledWith(returnURL));
		expect(approved).toHaveBeenCalledOnce();
	});

	it('does not approve when the missing-setup confirmation is cancelled', async () => {
		mockSetup();
		const approved = vi.fn();
		worker.use(
			http.post(`${prefix}/approve`, () => {
				approved();
				return HttpResponse.json({ redirectURL: returnURL });
			})
		);
		await renderPage();
		await expect.element(page.getByText('1 of 2 vMCPs ready')).toBeVisible();

		await page.getByRole('button', { name: 'Approve' }).click();
		await page.getByRole('button', { name: 'Cancel' }).last().click();

		expect(approved).not.toHaveBeenCalled();
		expect(navigation.navigateTo).not.toHaveBeenCalled();
	});

	it('approves at once when setup is complete', async () => {
		mockSetup(true);
		worker.use(http.post(`${prefix}/approve`, () => HttpResponse.json({ redirectURL: returnURL })));
		await renderPage();
		await expect.element(page.getByText('1 of 1 vMCPs ready')).toBeVisible();

		await page.getByRole('button', { name: 'Approve' }).click();

		await vi.waitFor(() => expect(navigation.navigateTo).toHaveBeenCalledWith(returnURL));
		await expect.element(page.getByText(/Some setup is missing/)).not.toBeVisible();
	});

	it('cancels by going to the URL the server returns', async () => {
		mockSetup();
		const cancelURL = 'https://agent.example.com/callback?requestID=adr1req&error=access_denied';
		worker.use(http.post(`${prefix}/cancel`, () => HttpResponse.json({ redirectURL: cancelURL })));
		await renderPage();

		await page.getByRole('button', { name: 'Cancel' }).click();

		await vi.waitFor(() => expect(navigation.navigateTo).toHaveBeenCalledWith(cancelURL));
	});

	it('shows a refused approval on a plain error page, without redirecting', async () => {
		mockSetup(true);
		worker.use(
			http.post(`${prefix}/approve`, () =>
				HttpResponse.text(
					'This approval link has expired or was already used. Go back to the agent to get a new one.',
					{ status: 403 }
				)
			)
		);
		await renderPage();
		await expect.element(page.getByText('1 of 1 vMCPs ready')).toBeVisible();

		await page.getByRole('button', { name: 'Approve' }).click();

		await expect
			.element(page.getByRole('heading', { name: "This approval can't continue" }))
			.toBeVisible();
		await expect.element(page.getByText(/expired or was already used/)).toBeVisible();
		await expect.element(page.getByRole('button', { name: 'Approve' })).not.toBeInTheDocument();
		expect(navigation.navigateTo).not.toHaveBeenCalled();
	});

	it('never sends the browser to a return URL that is not a web page', async () => {
		mockSetup(true);
		worker.use(
			http.post(`${prefix}/approve`, () =>
				HttpResponse.json({ redirectURL: 'javascript:alert(document.cookie)' })
			)
		);
		await renderPage();
		await expect.element(page.getByText('1 of 1 vMCPs ready')).toBeVisible();

		await page.getByRole('button', { name: 'Approve' }).click();

		await expect.element(page.getByText(/return address is not a web page/)).toBeVisible();
		expect(navigation.navigateTo).not.toHaveBeenCalled();
	});

	it.each([
		"This agent isn't allowed to act on your behalf. Ask an administrator to give it user access that includes you.",
		"This agent is disabled or was deleted, so it can't act on your behalf.",
		'This approval link has expired or was already used. Go back to the agent to get a new one.',
		'This approval link is for another account. Sign in as the account that approved the agent before, or go back to the agent to get a new link.'
	])('renders the error %j without redirecting', async (error) => {
		await renderPage({ error });

		await expect
			.element(page.getByRole('heading', { name: "This approval can't continue" }))
			.toBeVisible();
		await expect.element(page.getByText(error)).toBeVisible();
		await expect.element(page.getByText(/You're signed in as Ada Lovelace\./)).toBeVisible();
		await expect
			.element(page.getByRole('link', { name: 'Use a different account' }))
			.toHaveAttribute('href', '/oauth2/sign_out?rd=%2Fagents%2Fapprove%2Fadr1req');
		await expect.element(page.getByRole('button', { name: 'Approve' })).not.toBeInTheDocument();
		expect(navigation.navigateTo).not.toHaveBeenCalled();
	});

	it('shows an account connection the provider refused', async () => {
		mockSetup();
		await renderPage({ oauthError: 'server_error' });

		await expect.element(page.getByText('Connecting the account failed. Try again.')).toBeVisible();
	});
});
