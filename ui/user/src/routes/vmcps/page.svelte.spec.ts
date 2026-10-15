import { page as appPage } from '$app/state';
import { Group, type MCPCatalogEntry, type VMCPInstance } from '$lib/services';
import { mcpServersAndEntries, vmcpInstances } from '$lib/stores';
import { createMCPCatalogEntry, createVMCP } from '../../tests/helpers/mcp';
import { createMockProfile, preparePageData } from '../../tests/helpers/pageData';
import { getProfileResponse } from '../../tests/mocks/data';
import { worker } from '../../tests/mocks/worker';
import type { PageData } from './$types';
import VMcpsPage from './+page.svelte';
import { http, HttpResponse } from 'msw';
import { tick } from 'svelte';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

vi.mock('$app/navigation', async (importOriginal) => {
	const actual = await importOriginal<typeof import('$app/navigation')>();
	return {
		...actual,
		replaceState: vi.fn()
	};
});

const componentEntry = createMCPCatalogEntry({
	id: 'entry-github',
	name: 'GitHub'
});

function createIssueTrackerVMcp() {
	return createVMCP({ id: 'vmcp-1', displayName: 'Issue Tracker vMCP' }, [componentEntry]);
}

async function renderPageWithEntries(
	entries: MCPCatalogEntry[],
	creating = false,
	vmcps = [createIssueTrackerVMcp()],
	groups: string[] = [Group.ADMIN]
) {
	if (creating) {
		appPage.url.searchParams.set('new', 'true');
	} else {
		appPage.url.searchParams.delete('new');
	}
	mcpServersAndEntries.current = {
		entries,
		servers: [],
		userInstances: [],
		userConfiguredServers: [],
		loading: false,
		lastFetched: null,
		isInitialized: true
	};
	const data = await preparePageData<PageData>({
		vmcps,
		profile: createMockProfile(groups)
	});
	return render(VMcpsPage, { data });
}

afterEach(() => {
	for (const key of [
		'new',
		'query',
		'variant',
		'status',
		'components',
		'creator',
		'sort',
		'sortDirection'
	]) {
		appPage.url.searchParams.delete(key);
	}
	vmcpInstances.current = { items: [], loading: false };
});

describe('vMCPs Page', () => {
	describe('table view', () => {
		const slack = createMCPCatalogEntry({ id: 'entry-slack', name: 'Slack' });

		it.each(['name', 'componentServers'])(
			'preserves search relevance with %s sorting',
			async (sortBy) => {
				appPage.url.searchParams.set('variant', 'table');
				appPage.url.searchParams.set('query', 'GitHub');
				appPage.url.searchParams.set('sort', sortBy);
				await renderPageWithEntries([componentEntry, slack], false, [
					createVMCP(
						{ id: 'description-match', displayName: 'A Tools', description: 'GitHub integration' },
						[slack]
					),
					createVMCP({ id: 'name-match', displayName: 'Z GitHub', description: 'Issue tracker' }, [
						componentEntry,
						slack
					])
				]);
				await expect.element(page.getByText('Z GitHub', { exact: true })).toBeVisible();
				expect(document.querySelector('tbody tr')?.textContent).toContain('Z GitHub');
				await page.getByRole('button', { name: 'Grid View' }).click();
				await expect
					.element(page.getByRole('button', { name: /^Open / }).first())
					.toHaveAccessibleName('Open Z GitHub');
				await page.getByRole('button', { name: 'Table View' }).click();
				expect(document.querySelector('tbody tr')?.textContent).toContain('Z GitHub');
			}
		);

		it.each([
			['Servers', 'GitHub', 'Slack', 'components', 'entry-github,entry-slack'],
			['Created By', 'Alice', 'Bob', 'creator', 'alice,bob'],
			['Status', 'Connected', 'Not Connected', 'status', 'connected,not-connected']
		])(
			'keeps all %s options available for OR selection',
			async (header, first, second, param, selected) => {
				appPage.url.searchParams.set('variant', 'table');
				worker.use(
					http.get('/api/users', () =>
						HttpResponse.json({
							items: [
								{ id: 'alice', displayName: 'Alice' },
								{ id: 'bob', displayName: 'Bob' }
							]
						})
					)
				);
				vmcpInstances.current = {
					items: [
						{
							id: 'instance-1',
							vmcpID: 'github-vmcp',
							userID: getProfileResponse.id
						} as VMCPInstance
					],
					loading: false
				};
				await renderPageWithEntries([componentEntry, slack], false, [
					createVMCP({ id: 'github-vmcp', displayName: 'GitHub vMCP', creatorUserID: 'alice' }, [
						componentEntry
					]),
					createVMCP({ id: 'slack-vmcp', displayName: 'Slack vMCP', creatorUserID: 'bob' }, [slack])
				]);
				await page.getByRole('button', { name: header, exact: true }).click();
				await page.getByRole('combobox', { name: `Filter by ${header}...` }).click();
				await page.getByRole('button', { name: first, exact: true }).click();
				await expect.element(page.getByText('Slack vMCP', { exact: true })).not.toBeInTheDocument();
				await page.getByRole('combobox', { name: `Filter by ${header}...` }).click();
				await page.getByRole('button', { name: second, exact: true }).click();
				expect(appPage.url.searchParams.get(param)).toBe(selected);
				await expect.element(page.getByText('GitHub vMCP', { exact: true })).toBeVisible();
				await expect.element(page.getByText('Slack vMCP', { exact: true })).toBeVisible();
			}
		);

		it('keeps header options available when combined filters match no rows', async () => {
			appPage.url.searchParams.set('variant', 'table');
			appPage.url.searchParams.set('components', 'entry-github');
			appPage.url.searchParams.set('creator', 'bob');
			await renderPageWithEntries([componentEntry, slack], false, [
				createVMCP({ id: 'github-vmcp', displayName: 'GitHub vMCP', creatorUserID: 'alice' }, [
					componentEntry
				]),
				createVMCP({ id: 'slack-vmcp', displayName: 'Slack vMCP', creatorUserID: 'bob' }, [slack])
			]);
			await expect.element(page.getByText('No results found.')).toBeVisible();
			await page.getByRole('button', { name: /^Servers/ }).click();
			await page.getByRole('combobox', { name: 'Filter by Servers...' }).click();
			await page.getByRole('button', { name: 'Slack', exact: true }).click();
			await expect.element(page.getByText('Slack vMCP', { exact: true })).toBeVisible();
		});

		it('shares server header filters with settings, grid view, and reloads', async () => {
			appPage.url.searchParams.set('variant', 'table');
			const vmcps = [
				createIssueTrackerVMcp(),
				createVMCP({ id: 'vmcp-slack', displayName: 'Alerts vMCP' }, [slack])
			];
			const screen = await renderPageWithEntries([componentEntry, slack], false, vmcps);
			await page.getByRole('button', { name: 'Servers', exact: true }).click();
			await page.getByRole('combobox', { name: 'Filter by Servers...' }).click();
			await page.getByRole('button', { name: 'GitHub', exact: true }).click();
			await page.getByCSS('#click-catch').click({ position: { x: 1, y: 1 } });

			expect(appPage.url.searchParams.get('components')).toBe('entry-github');
			await expect.element(page.getByRole('button', { name: 'Remove GitHub' })).toBeVisible();
			await page.getByRole('button', { name: 'Grid View' }).click();
			await expect
				.element(page.getByRole('button', { name: 'Open Issue Tracker vMCP' }))
				.toBeVisible();
			await expect
				.element(page.getByRole('button', { name: 'Open Alerts vMCP' }))
				.not.toBeInTheDocument();

			await screen.unmount();
			await renderPageWithEntries([componentEntry, slack], false, vmcps);
			await expect
				.element(page.getByRole('button', { name: 'Open Issue Tracker vMCP' }))
				.toBeVisible();
			await expect
				.element(page.getByRole('button', { name: 'Open Alerts vMCP' }))
				.not.toBeInTheDocument();
			await page.getByRole('button', { name: 'Remove GitHub' }).click();
			await page.getByRole('button', { name: 'Table View' }).click();
			await expect.element(page.getByText('Alerts vMCP', { exact: true })).toBeVisible();
		});

		it('shares creator header filters with settings and grid view', async () => {
			appPage.url.searchParams.set('variant', 'table');
			worker.use(
				http.get('/api/users', () =>
					HttpResponse.json({
						items: [
							{ id: 'alice', displayName: 'Alice' },
							{ id: 'bob', displayName: 'Bob' }
						]
					})
				)
			);
			await renderPageWithEntries([componentEntry], false, [
				createVMCP({ id: 'alice-vmcp', displayName: 'Alice vMCP', creatorUserID: 'alice' }, [
					componentEntry
				]),
				createVMCP({ id: 'bob-vmcp', displayName: 'Bob vMCP', creatorUserID: 'bob' }, [
					componentEntry
				])
			]);
			await page.getByRole('button', { name: 'Created By', exact: true }).click();
			await page.getByRole('combobox', { name: 'Filter by Created By...' }).click();
			await page.getByRole('button', { name: 'Alice', exact: true }).click();
			await page.getByCSS('#click-catch').click({ position: { x: 1, y: 1 } });
			expect(appPage.url.searchParams.get('creator')).toBe('alice');
			await expect.element(page.getByRole('button', { name: 'Remove Alice' })).toBeVisible();
			await page.getByRole('button', { name: 'Grid View' }).click();
			await expect.element(page.getByRole('button', { name: 'Open Alice vMCP' })).toBeVisible();
			await expect
				.element(page.getByRole('button', { name: 'Open Bob vMCP' }))
				.not.toBeInTheDocument();
			await page.getByRole('button', { name: 'Remove Alice' }).click();
			await page.getByRole('button', { name: 'Table View' }).click();
			await expect.element(page.getByText('Bob vMCP', { exact: true })).toBeVisible();
		});

		it.each([
			['connected', 'Connected'],
			['not-connected', 'Not Connected']
		])('persists %s status and restores it after reload', async (id, label) => {
			appPage.url.searchParams.set('variant', 'table');
			const screen = await renderPageWithEntries([componentEntry]);
			vmcpInstances.current = {
				items:
					id === 'connected'
						? [
								{
									id: 'instance-1',
									vmcpID: 'vmcp-1',
									userID: getProfileResponse.id
								} as VMCPInstance
							]
						: [],
				loading: false
			};
			await page.getByRole('button', { name: 'Status', exact: true }).click();
			await page.getByRole('combobox', { name: 'Filter by Status...' }).click();
			await page.getByRole('button', { name: label, exact: true }).click();

			expect(appPage.url.searchParams.get('status')).toBe(id);
			await expect.element(page.getByRole('button', { name: `Remove ${label}` })).toBeVisible();
			await screen.unmount();
			await renderPageWithEntries([componentEntry]);
			await expect.element(page.getByText('Issue Tracker vMCP', { exact: true })).toBeVisible();
			await page.getByRole('button', { name: 'Grid View' }).click();
			await expect
				.element(page.getByRole('button', { name: 'Open Issue Tracker vMCP' }))
				.toBeVisible();
		});

		it('keeps server count sorting consistent across settings, views, and reloads', async () => {
			appPage.url.searchParams.set('variant', 'table');
			const vmcps = [
				createVMCP({ id: 'many', displayName: 'Many Servers' }, [componentEntry, slack]),
				createVMCP({ id: 'one', displayName: 'One Server' }, [slack])
			];
			const screen = await renderPageWithEntries([componentEntry, slack], false, vmcps);
			const serverSort = page
				.getByRole('columnheader')
				.filter({ hasText: 'Servers' })
				.getByRole('button')
				.last();
			const tableNames = () =>
				[...document.querySelectorAll('tbody tr')].map((row) => row.textContent);
			await serverSort.click();
			expect(appPage.url.searchParams.get('sort')).toBe('componentServers');
			expect(tableNames()[0]).toContain('One Server');
			await serverSort.click();
			expect(appPage.url.searchParams.get('sortDirection')).toBe('desc');
			expect(tableNames()[0]).toContain('Many Servers');
			await page.getByRole('button', { name: 'Filters', exact: true }).click();
			await expect
				.element(page.getByRole('combobox', { name: 'Sort By', exact: true }))
				.toHaveTextContent('MCP Servers');
			await expect
				.element(page.getByRole('combobox', { name: 'Sort Direction', exact: true }))
				.toHaveTextContent('Descending');
			await screen.unmount();
			await renderPageWithEntries([componentEntry, slack], false, vmcps);
			expect(tableNames()[0]).toContain('Many Servers');
			await page.getByRole('button', { name: 'Grid View' }).click();
			await expect
				.element(page.getByRole('button', { name: /^Open / }).first())
				.toHaveAccessibleName('Open Many Servers');
		});

		it('shows the vMCP list and create action', async () => {
			await renderPageWithEntries([componentEntry]);

			await expect
				.element(page.getByRole('button', { name: 'Open Issue Tracker vMCP' }))
				.toBeVisible();
			await expect.element(page.getByRole('button', { name: 'Create vMCP' })).toBeVisible();
			await expect
				.element(page.getByRole('button', { name: 'Sources', exact: true }))
				.not.toBeInTheDocument();
		});

		it('takes no drops, so it offers no servers to drag', async () => {
			await renderPageWithEntries([componentEntry, slack]);
			await expect
				.element(page.getByRole('button', { name: 'Open Issue Tracker vMCP' }))
				.toBeVisible();

			await expect
				.element(page.getByRole('button', { name: new RegExp('View Slack details') }))
				.not.toBeInTheDocument();
		});

		it('filters to creator-owned vMCPs when "Show Only My vMCPs" is checked', async () => {
			const myCreated = createVMCP(
				{
					id: 'vmcp-mine',
					displayName: 'My Created vMCP',
					creatorUserID: getProfileResponse.id,
					userID: getProfileResponse.id
				},
				[componentEntry]
			);
			const ownedNotCreated = createVMCP(
				{
					id: 'vmcp-owned',
					displayName: 'Owned Not Created vMCP',
					creatorUserID: 'someone-else',
					userID: getProfileResponse.id
				},
				[componentEntry]
			);
			const other = createVMCP(
				{
					id: 'vmcp-other',
					displayName: 'Other vMCP',
					creatorUserID: 'someone-else'
				},
				[componentEntry]
			);

			await renderPageWithEntries(
				[componentEntry],
				false,
				[myCreated, ownedNotCreated, other],
				[Group.USER]
			);

			await expect
				.element(page.getByRole('button', { name: 'Open My Created vMCP' }))
				.toBeVisible();
			await expect
				.element(page.getByRole('button', { name: 'Open Owned Not Created vMCP' }))
				.toBeVisible();
			await expect.element(page.getByRole('button', { name: 'Open Other vMCP' })).toBeVisible();

			await page.getByRole('button', { name: 'Filters' }).click();
			await page.getByRole('checkbox', { name: 'Show Only My vMCPs' }).click();

			await expect
				.element(page.getByRole('button', { name: 'Open My Created vMCP' }))
				.toBeVisible();
			await expect
				.element(page.getByRole('button', { name: 'Open Owned Not Created vMCP' }))
				.not.toBeInTheDocument();
			await expect
				.element(page.getByRole('button', { name: 'Open Other vMCP' }))
				.not.toBeInTheDocument();
		});
	});

	describe('create view', () => {
		const slack = createMCPCatalogEntry({ id: 'entry-slack', name: 'Slack' });

		function panelCard(name: string) {
			return page.getByRole('button', { name: new RegExp(`View ${name} details`) });
		}

		it('opens the designer canvas when creating a vMCP', async () => {
			await renderPageWithEntries([componentEntry], true, []);

			await expect.element(page.getByRole('button', { name: 'Designer' })).toBeVisible();
			await expect.element(page.getByCSS('[data-vmcp-world]')).not.toBeInTheDocument();
			await expect
				.element(page.getByRole('button', { name: 'Open Issue Tracker vMCP' }))
				.not.toBeInTheDocument();
		});

		it('creates a vMCP from a drop anywhere on the empty canvas', async () => {
			worker.use(
				http.get(`/api/mcp-catalogs/default/entries/${slack.id}`, () => HttpResponse.json(slack)),
				http.get(`/api/mcp-catalogs/default/entries/${slack.id}/servers`, () =>
					HttpResponse.json({ items: [] })
				)
			);
			await renderPageWithEntries([componentEntry, slack], true, []);

			const canvas = await page.getByCSS('[data-vmcp-canvas]').element();
			const rect = canvas.getBoundingClientRect();
			const el = await panelCard('Slack').element();
			if (!(el instanceof HTMLElement)) throw new Error('Expected an HTMLElement');
			el.setPointerCapture = () => {};
			const from = el.getBoundingClientRect();
			const start = { x: from.left + from.width / 2, y: from.top + from.height / 2 };
			const to = { x: rect.left + 16, y: rect.top + 16 };
			el.dispatchEvent(
				new PointerEvent('pointerdown', {
					bubbles: true,
					cancelable: true,
					button: 0,
					pointerId: 17,
					clientX: start.x,
					clientY: start.y
				})
			);
			el.dispatchEvent(
				new PointerEvent('pointermove', {
					bubbles: true,
					cancelable: true,
					button: 0,
					pointerId: 17,
					clientX: to.x,
					clientY: to.y
				})
			);
			await tick();
			el.dispatchEvent(
				new PointerEvent('pointerup', {
					bubbles: true,
					cancelable: true,
					button: 0,
					pointerId: 17,
					clientX: to.x,
					clientY: to.y
				})
			);

			await expect.element(page.getByRole('dialog').getByText('Create vMCP').first()).toBeVisible();
		});
	});
});
