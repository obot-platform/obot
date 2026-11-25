import { page as appPage } from '$app/state';
import { Group, UserService, type OrgUser } from '$lib/services';
import { profile } from '$lib/stores';
import { createMockProfile } from '../../../../tests/helpers/pageData';
import { worker } from '../../../../tests/mocks/worker';
import UsageGraphs from './UsageGraphs.svelte';
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
	for (const key of ['user_ids', 'on_behalf_of']) {
		(appPage.url as URL).searchParams.delete(key);
	}
	vi.restoreAllMocks();
});

function mockApis() {
	const stats: URLSearchParams[] = [];
	vi.spyOn(UserService, 'listUsersIncludeDeleted').mockResolvedValue([
		{ id: '3', displayName: 'Ada Admin' } as OrgUser,
		{ id: '4', displayName: 'Bea Builder' } as OrgUser
	]);
	worker.use(
		http.get('/api/mcp-stats', ({ request }) => {
			stats.push(new URL(request.url).searchParams);
			return HttpResponse.json({ totalCalls: 0, uniqueUsers: 0, items: [] });
		}),
		http.get('/api/mcp-audit-logs/filter-options/:filter', ({ params }) =>
			HttpResponse.json({ options: params.filter === 'on_behalf_of' ? ['3', '4'] : [] })
		)
	);
	return stats;
}

const onBehalfOfPill = () => page.getByCSS('.filter-primary').filter({ hasText: 'On behalf of' });

describe('UsageGraphs on-behalf-of filter', () => {
	it('lists the users agents acted for by name, and asks for the calls made for the one picked', async () => {
		profile.initialize(createMockProfile([Group.ADMIN]));
		const stats = mockApis();
		render(UsageGraphs);

		await page.getByRole('button', { name: 'Filters', exact: true }).click();
		await page.getByCSS('#filter-on_behalf_of').click();
		await page.getByRole('button', { name: 'Ada Admin', exact: true }).click();
		await page.getByRole('button', { name: 'Apply Filters', exact: true }).click();

		expect(appPage.url.searchParams.get('on_behalf_of')).toBe('3');
		await vi.waitFor(() => expect(stats.at(-1)?.get('on_behalf_of')).toBe('3'));
		await expect.element(onBehalfOfPill().getByText('Ada Admin')).toBeVisible();
	});

	it('reads the filter from the URL, alongside the user filter', async () => {
		profile.initialize(createMockProfile([Group.ADMIN]));
		(appPage.url as URL).searchParams.set('user_ids', 'agent:7');
		(appPage.url as URL).searchParams.set('on_behalf_of', '4');
		const stats = mockApis();
		render(UsageGraphs);

		await vi.waitFor(() => expect(stats.length).toBeGreaterThan(0));
		expect(stats.at(-1)!.get('user_ids')).toBe('agent:7');
		expect(stats.at(-1)!.get('on_behalf_of')).toBe('4');
		await expect.element(onBehalfOfPill().getByText('Bea Builder')).toBeVisible();
	});
});
