import { page as appPage } from '$app/state';
import { Group, UserService, type OrgUser } from '$lib/services';
import { profile } from '$lib/stores';
import { createMockProfile } from '../../../../tests/helpers/pageData';
import { worker } from '../../../../tests/mocks/worker';
import AuditLogsPageContent from './AuditLogsPageContent.svelte';
import { http, HttpResponse } from 'msw';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { cleanup, render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

// The URL is reactive, and applying filters changes it, so a filter can be followed from the drawer to the request.
vi.mock('$app/state', async (importOriginal) => {
	const actual = await importOriginal<typeof import('$app/state')>();
	const { createAppState } = await import('../../../../tests/helpers/navigation.svelte');
	return createAppState(actual);
});

vi.mock('$app/navigation', async () => {
	const { applyTestGoto } = await import('../../../../tests/helpers/navigation.svelte');
	return {
		goto: (url: string | URL) => applyTestGoto(url),
		replaceState: (url: string | URL) => applyTestGoto(url),
		invalidate: vi.fn(),
		invalidateAll: vi.fn(),
		preloadCode: vi.fn(),
		preloadData: vi.fn(),
		beforeNavigate: vi.fn(),
		// The users are loaded after a navigation, which a component test never makes.
		afterNavigate: (callback: (navigation: unknown) => void) => callback({}),
		onNavigate: vi.fn(),
		pushState: vi.fn(),
		disableScrollHandling: vi.fn()
	};
});

vi.mock('$lib/url', async (importOriginal) => {
	const actual = await importOriginal<typeof import('$lib/url')>();
	const { applyTestGoto } = await import('../../../../tests/helpers/navigation.svelte');
	return {
		...actual,
		goto: (url: string | URL) => applyTestGoto(url),
		replaceState: (url: string | URL) => applyTestGoto(url)
	};
});

afterEach(() => {
	// Unmounted first, so that resetting the URL does not make it ask for anything after the handlers are reset.
	cleanup();
	for (const key of ['actor', 'on_behalf_of', 'operation']) {
		(appPage.url as URL).searchParams.delete(key);
	}
	vi.restoreAllMocks();
});

function mockApis() {
	const auditLogs: URLSearchParams[] = [];
	const filterOptions: { filter: string; params: URLSearchParams }[] = [];
	vi.spyOn(UserService, 'listUsersIncludeDeleted').mockResolvedValue([
		{ id: '3', displayName: 'Ada Admin' } as OrgUser,
		{ id: '4', displayName: 'Bea Builder' } as OrgUser
	]);
	worker.use(
		http.get('/api/mcp-audit-logs', ({ request }) => {
			auditLogs.push(new URL(request.url).searchParams);
			return HttpResponse.json({ items: [], total: 0 });
		}),
		http.get('/api/mcp-audit-logs/filter-options/:filter', ({ request, params }) => {
			const filter = String(params.filter);
			filterOptions.push({ filter, params: new URL(request.url).searchParams });
			return HttpResponse.json({ options: filter === 'on_behalf_of' ? ['3', '4'] : [] });
		})
	);
	return { auditLogs, filterOptions };
}

const onBehalfOfPill = () => page.getByCSS('.filter-primary').filter({ hasText: 'On behalf of' });

describe('AuditLogsPageContent on-behalf-of filter', () => {
	it('lists the users agents acted for by name, and asks for the one picked', async () => {
		profile.initialize(createMockProfile([Group.ADMIN]));
		const requests = mockApis();
		render(AuditLogsPageContent);

		await page.getByRole('button', { name: 'Filters', exact: true }).click();
		await page.getByCSS('#filter-on_behalf_of').click();
		await page.getByRole('button', { name: 'Ada Admin', exact: true }).click();
		await page.getByRole('button', { name: 'Apply Filters', exact: true }).click();

		expect(appPage.url.searchParams.get('on_behalf_of')).toBe('3');
		await vi.waitFor(() => expect(requests.auditLogs.at(-1)?.get('on_behalf_of')).toBe('3'));
		await expect.element(onBehalfOfPill().getByText('Ada Admin')).toBeVisible();
		// The options of the filter itself are not narrowed by it.
		expect(
			requests.filterOptions
				.filter(({ filter }) => filter === 'on_behalf_of')
				.every(({ params }) => !params.has('on_behalf_of'))
		).toBe(true);
	});

	it('reads the filter from the URL, with an agent as the actor, and narrows the other options', async () => {
		profile.initialize(createMockProfile([Group.ADMIN]));
		(appPage.url as URL).searchParams.set('actor', 'agent:7');
		(appPage.url as URL).searchParams.set('on_behalf_of', '3,4');
		const requests = mockApis();
		render(AuditLogsPageContent);

		await vi.waitFor(() => expect(requests.auditLogs.length).toBeGreaterThan(0));
		const params = requests.auditLogs.at(-1)!;
		expect(params.get('actor')).toBe('agent:7');
		expect(params.get('on_behalf_of')).toBe('3,4');
		await expect.element(onBehalfOfPill().first()).toBeVisible();
		await expect.element(page.getByText('Bea Builder', { exact: true })).toBeVisible();

		await page.getByRole('button', { name: 'Filters', exact: true }).click();
		await vi.waitFor(() =>
			expect(
				requests.filterOptions.find(({ filter }) => filter === 'actor')?.params.get('on_behalf_of')
			).toBe('3,4')
		);
	});

	it('offers no on-behalf-of filter to basic users', async () => {
		profile.initialize(createMockProfile([]));
		mockApis();
		render(AuditLogsPageContent);

		await page.getByRole('button', { name: 'Filters', exact: true }).click();
		await expect.element(page.getByCSS('#filter-operation')).toBeVisible();
		await expect.element(page.getByCSS('#filter-on_behalf_of')).not.toBeInTheDocument();
	});
});
