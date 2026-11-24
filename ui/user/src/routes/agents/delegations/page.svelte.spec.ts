import { goto } from '$app/navigation';
import { Group } from '$lib/services';
import type { AgentDelegation } from '$lib/services/agent-identities/types';
import { createMockProfile, preparePageData } from '../../../tests/helpers/pageData';
import { worker } from '../../../tests/mocks/worker';
import type { PageData } from './$types';
import DelegationsPage from './+page.svelte';
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

const me = { id: '3', email: 'ada@example.com', displayName: 'Ada' };
const delegations: AgentDelegation[] = [
	{
		id: 'adl1build',
		agent: { id: '7', displayName: 'Build Bot', status: 'active' },
		user: me,
		firstApprovedAt: '2026-01-01T00:00:00Z',
		lastApprovedAt: '2026-01-02T00:00:00Z',
		lastUsedAt: '2026-01-03T00:00:00Z'
	},
	{
		id: 'adl1mail',
		agent: { id: '8', displayName: 'Mail Helper', status: 'active' },
		user: me,
		firstApprovedAt: '2026-01-01T00:00:00Z',
		lastApprovedAt: '2026-01-01T00:00:00Z'
	}
];
const revoked = vi.fn();

beforeEach(() => {
	revoked.mockReset();
	vi.mocked(goto).mockReset();
	worker.use(
		http.delete('/api/me/agent-delegations/:id', ({ params }) => {
			revoked(params.id);
			return new HttpResponse(null, { status: 204 });
		})
	);
});

async function renderPage(items = delegations) {
	const data = await preparePageData<PageData>({
		profile: createMockProfile([Group.USER]),
		delegations: items
	});
	return render(DelegationsPage, { data, params: {} });
}

describe('Agents acting for you', () => {
	it('lists the agents acting for the user, with when', async () => {
		await renderPage();

		await expect.element(page.getByRole('row', { name: /Build Bot/ })).toBeVisible();
		await expect.element(page.getByRole('row', { name: /Mail Helper/ })).toBeVisible();
		await expect
			.element(page.getByRole('row', { name: /Mail Helper/ }).getByText('Never'))
			.toBeVisible();
	});

	it('opens a delegation', async () => {
		await renderPage();

		await page.getByRole('cell', { name: 'Build Bot' }).click();

		await vi.waitFor(() =>
			expect(goto).toHaveBeenCalledWith('/agents/delegations/adl1build', expect.anything())
		);
	});

	it('says when no agent acts for the user', async () => {
		await renderPage([]);

		await expect.element(page.getByText('No agent acts on your behalf.')).toBeVisible();
	});

	it('revokes an agent only after a confirmation', async () => {
		await renderPage();

		await page
			.getByRole('row', { name: /Build Bot/ })
			.getByRole('button', { name: 'Revoke' })
			.click();
		await expect.element(page.getByText('Revoke "Build Bot"?')).toBeVisible();
		expect(revoked).not.toHaveBeenCalled();
		expect(goto).not.toHaveBeenCalled();

		await page.getByRole('button', { name: 'Revoke', exact: true }).last().click();

		await vi.waitFor(() => expect(revoked).toHaveBeenCalledWith('adl1build'));
		await expect.element(page.getByRole('row', { name: /Build Bot/ })).not.toBeInTheDocument();
		await expect.element(page.getByRole('row', { name: /Mail Helper/ })).toBeVisible();
	});
});
