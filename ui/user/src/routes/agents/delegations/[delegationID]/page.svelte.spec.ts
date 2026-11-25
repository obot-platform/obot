import { goto } from '$app/navigation';
import { Group } from '$lib/services';
import type { AgentDelegation } from '$lib/services/agent-identities/types';
import { createMockProfile, preparePageData } from '../../../../tests/helpers/pageData';
import { createDelegationSetup } from '../../../../tests/mocks/agentIdentities';
import { worker } from '../../../../tests/mocks/worker';
import type { PageData } from './$types';
import DelegationPage from './+page.svelte';
import { http, HttpResponse } from 'msw';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

vi.mock('$app/navigation', () => ({
	goto: vi.fn(),
	invalidate: vi.fn(),
	invalidateAll: vi.fn(),
	preloadCode: vi.fn(),
	preloadData: vi.fn(),
	beforeNavigate: vi.fn(),
	afterNavigate: vi.fn(),
	onNavigate: vi.fn(),
	pushState: vi.fn(),
	replaceState: vi.fn(),
	disableScrollHandling: vi.fn()
}));

const prefix = '/api/me/agent-delegations/adl1build';
const delegation: AgentDelegation = {
	id: 'adl1build',
	agent: { id: '7', displayName: 'Build Bot', description: 'Runs builds', status: 'active' },
	user: { id: '3', email: 'ada@example.com' },
	firstApprovedAt: '2026-01-01T00:00:00Z',
	lastApprovedAt: '2026-01-02T00:00:00Z'
};

beforeEach(() => {
	vi.mocked(goto).mockReset();
	worker.use(http.get(`${prefix}/setup`, () => HttpResponse.json(createDelegationSetup())));
});

async function renderPage(oauthError: string | null = null) {
	const data = await preparePageData<PageData>({
		profile: createMockProfile([Group.USER]),
		delegation,
		oauthError
	});
	return render(DelegationPage, { data, params: { delegationID: 'adl1build' } });
}

describe('an agent acting for the user', () => {
	it('shows the agent and its setup, with values only as set or not', async () => {
		await renderPage();

		await expect.element(page.getByText('Runs builds')).toBeVisible();
		await expect.element(page.getByText('1 of 2 vMCPs ready')).toBeVisible();
		await expect
			.element(page.getByRole('group', { name: 'GitHub' }).getByText('Not set'))
			.toBeVisible();
	});

	it('edits values through the delegation-scoped routes', async () => {
		const saved = vi.fn();
		worker.use(
			http.put(`${prefix}/setup/vmcps/vmcp1dev/configuration`, async ({ request }) => {
				saved(await request.json());
				return HttpResponse.json(createDelegationSetup().vmcps[0]);
			})
		);
		await renderPage();

		await page
			.getByRole('region', { name: 'Developer tools' })
			.getByRole('button', { name: 'Set values' })
			.click();
		await page.getByLabelText('Token').fill('t0ken');
		await page.getByRole('button', { name: 'Save' }).click();
		await vi.waitFor(() =>
			expect(saved).toHaveBeenCalledWith({ components: { 'c-github': { TOKEN: 't0ken' } } })
		);

		await page.getByRole('button', { name: 'Clear Organization' }).click();
		await vi.waitFor(() =>
			expect(saved).toHaveBeenLastCalledWith({ components: { 'c-github': { ORG: null } } })
		);
	});

	it('revokes the agent only after a confirmation, then goes back to the list', async () => {
		const revoked = vi.fn();
		worker.use(
			http.delete(prefix, () => {
				revoked();
				return new HttpResponse(null, { status: 204 });
			})
		);
		await renderPage();

		await page.getByRole('button', { name: 'Revoke' }).click();
		await expect.element(page.getByText('Revoke "Build Bot"?')).toBeVisible();
		expect(revoked).not.toHaveBeenCalled();

		await page.getByRole('button', { name: 'Revoke', exact: true }).last().click();

		await vi.waitFor(() => expect(revoked).toHaveBeenCalledOnce());
		await vi.waitFor(() =>
			expect(goto).toHaveBeenCalledWith('/agents/delegations', expect.anything())
		);
	});

	it('shows an account connection the provider refused', async () => {
		await renderPage('access_denied');

		await expect.element(page.getByText('The account connection was denied.')).toBeVisible();
	});
});
