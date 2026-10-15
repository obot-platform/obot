import { resolveVMcpComponents } from '$lib/services/vmcps/utils';
import { vmcpInstances } from '$lib/stores';
import { setUrlParamAndUpdateUrl } from '$lib/url';
import { createMCPCatalogEntry, createVMCP } from '../../../tests/helpers/mcp';
import { preparePageData } from '../../../tests/helpers/pageData';
import VMcpList from './VMcpList.svelte';
import { describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

vi.mock('$lib/url', async (importOriginal) => ({
	...(await importOriginal<typeof import('$lib/url')>()),
	setUrlParamAndUpdateUrl: vi.fn()
}));

const github = createMCPCatalogEntry({ id: 'entry-github', name: 'GitHub' });
const slack = createMCPCatalogEntry({ id: 'entry-slack', name: 'Slack' });

async function renderTable() {
	await preparePageData();
	vmcpInstances.current = { items: [], loading: false };
	return render(VMcpList, {
		items: [
			createVMCP({ id: 'vmcp-github', displayName: 'Issue Tracker vMCP' }, [github]),
			createVMCP({ id: 'vmcp-slack', displayName: 'Alerts vMCP' }, [slack])
		],
		components: resolveVMcpComponents,
		usersMap: new Map(),
		variant: 'table'
	});
}

describe('VMcpList.svelte', () => {
	it('filters the servers column by catalog entry id and shows the server name', async () => {
		await renderTable();

		await expect.element(page.getByText('Issue Tracker vMCP')).toBeVisible();
		await expect.element(page.getByText('Alerts vMCP')).toBeVisible();

		await page.getByRole('button', { name: 'Servers', exact: true }).click();
		await page.getByRole('combobox', { name: 'Filter by Servers...' }).click();
		const githubOption = page.getByRole('button', { name: 'GitHub', exact: true });
		await expect.element(githubOption).toBeVisible();
		await expect
			.element(page.getByRole('button', { name: 'entry-github', exact: true }))
			.not.toBeInTheDocument();

		await githubOption.click();

		expect(setUrlParamAndUpdateUrl).toHaveBeenCalledWith(
			expect.any(URL),
			'components',
			'entry-github'
		);
		await expect.element(page.getByText('Issue Tracker vMCP')).toBeVisible();
		await expect.element(page.getByText('Alerts vMCP')).not.toBeInTheDocument();
		await expect.element(page.getByText('GitHub', { exact: true }).first()).toBeVisible();
	});
});
