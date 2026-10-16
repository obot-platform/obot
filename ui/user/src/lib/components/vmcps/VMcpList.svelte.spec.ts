import { UserService } from '$lib/services';
import { resolveVMcpComponents } from '$lib/services/vmcps/utils';
import { vmcpInstances } from '$lib/stores';
import { setUrlParamAndUpdateUrl } from '$lib/url';
import { createMCPCatalogEntry, createVMCP } from '../../../tests/helpers/mcp';
import { preparePageData } from '../../../tests/helpers/pageData';
import { getProfileResponse } from '../../../tests/mocks/data';
import { worker } from '../../../tests/mocks/worker';
import VMcpList from './VMcpList.svelte';
import { http, HttpResponse } from 'msw';
import { afterEach, describe, expect, it, vi } from 'vitest';
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

describe.each(['grid', 'table'] as const)('VMcpList %s reauthentication', (variant) => {
	afterEach(() => vi.restoreAllMocks());
	async function renderConnectedList(connected = true) {
		await preparePageData();
		const vmcp = createVMCP({ id: 'vmcp-mail', displayName: 'Mail vMCP' }, [github, slack]);
		vmcpInstances.current = {
			items: connected
				? [
						{
							id: 'vmcpi-mail',
							vmcpID: vmcp.id,
							userID: getProfileResponse.id,
							created: '',
							status: { configured: true }
						}
					]
				: [],
			loading: false
		};
		await render(VMcpList, {
			items: [vmcp, createVMCP({ id: 'vmcp-other', displayName: 'Other vMCP' })],
			components: resolveVMcpComponents,
			usersMap: new Map(),
			variant
		});
		return vmcp;
	}

	it('loads only the opened menu and hands authenticated components to the chooser', async () => {
		const status = vi.spyOn(UserService, 'getVMCPOAuthStatus');
		worker.use(
			http.get('/api/vmcps/vmcp-mail/oauth-status', () =>
				HttpResponse.json({
					authenticatedComponents: ['component-entry-github', 'component-entry-slack']
				})
			)
		);
		await renderConnectedList();
		expect(status).not.toHaveBeenCalled();
		await page.getByRole('button', { name: 'Actions for Mail vMCP', exact: true }).click();
		await expect
			.element(page.getByRole('button', { name: 'Reauthenticate', exact: true }))
			.toBeVisible();
		expect(status).toHaveBeenCalledExactlyOnceWith(
			'vmcp-mail',
			expect.objectContaining({ signal: expect.any(AbortSignal) })
		);
		await page.getByRole('button', { name: 'Reauthenticate', exact: true }).click();
		await expect
			.element(page.getByRole('button', { name: 'Reauthenticate GitHub', exact: true }))
			.toBeVisible();
		await expect
			.element(page.getByRole('button', { name: 'Reauthenticate Slack', exact: true }))
			.toBeVisible();
		await expect
			.element(page.getByRole('button', { name: 'Reauthenticate All', exact: true }))
			.toBeVisible();
	});

	it('refreshes OAuth status on reopening the menu', async () => {
		let ids: string[] = [];
		const status = vi.spyOn(UserService, 'getVMCPOAuthStatus');
		worker.use(
			http.get('/api/vmcps/vmcp-mail/oauth-status', () =>
				HttpResponse.json({ authenticatedComponents: ids })
			)
		);
		await renderConnectedList();
		const menu = page.getByRole('button', { name: 'Actions for Mail vMCP', exact: true });
		await menu.click();
		await vi.waitFor(() => expect(status).toHaveResolvedWith([]));
		await expect
			.element(page.getByRole('button', { name: 'Reauthenticate', exact: true }))
			.not.toBeInTheDocument();
		await menu.click({ force: true });
		ids = ['component-entry-github'];
		await menu.click();
		await expect
			.element(page.getByRole('button', { name: 'Reauthenticate', exact: true }))
			.toBeVisible();
		expect(status).toHaveBeenCalledTimes(2);
	});

	it('reauthenticates the only authenticated component directly', async () => {
		worker.use(
			http.get('/api/vmcps/vmcp-mail/oauth-status', () =>
				HttpResponse.json({ authenticatedComponents: ['component-entry-github'] })
			)
		);
		const clearOAuth = vi
			.spyOn(UserService, 'clearVMCPComponentOAuth')
			.mockResolvedValue(undefined);
		vi.spyOn(UserService, 'getMcpServerOauthURL').mockResolvedValue('');
		await renderConnectedList();
		await page.getByRole('button', { name: 'Actions for Mail vMCP', exact: true }).click();
		await page.getByRole('button', { name: 'Reauthenticate', exact: true }).click();
		await vi.waitFor(() =>
			expect(clearOAuth).toHaveBeenCalledExactlyOnceWith('vmcp-mail', 'component-entry-github')
		);
	});

	it('cancels a pending lookup when the menu closes and ignores its late result', async () => {
		let finishStatus!: (ids: string[]) => void;
		const status = vi
			.spyOn(UserService, 'getVMCPOAuthStatus')
			.mockImplementationOnce(
				() =>
					new Promise<string[]>((resolve) => {
						finishStatus = resolve;
					})
			)
			.mockResolvedValue([]);
		await renderConnectedList();
		const menu = page.getByRole('button', { name: 'Actions for Mail vMCP', exact: true });
		await menu.click();
		await vi.waitFor(() => expect(status).toHaveBeenCalledOnce());
		const reauthenticate = page.getByRole('button', { name: 'Reauthenticate', exact: true });
		await expect.element(reauthenticate).toBeVisible();
		await expect.element(reauthenticate).toBeDisabled();
		const signal = status.mock.calls[0][1]?.signal;
		await menu.click({ force: true });
		expect(signal?.aborted).toBe(true);
		await menu.click();
		await vi.waitFor(() => expect(status).toHaveResolvedWith([]));
		finishStatus(['component-entry-github']);
		await vi.waitFor(() => expect(status).toHaveResolvedWith(['component-entry-github']));
		await expect
			.element(page.getByRole('button', { name: 'Reauthenticate', exact: true }))
			.not.toBeInTheDocument();
	});

	it('does not query OAuth status for an unconnected vMCP', async () => {
		const status = vi.spyOn(UserService, 'getVMCPOAuthStatus');
		await renderConnectedList(false);
		await page.getByRole('button', { name: 'Actions for Mail vMCP', exact: true }).click();
		await expect
			.element(page.getByRole('button', { name: 'Reauthenticate', exact: true }))
			.not.toBeInTheDocument();
		expect(status).not.toHaveBeenCalled();
	});

	it('keeps other actions available when OAuth status fails', async () => {
		const status = vi.spyOn(UserService, 'getVMCPOAuthStatus');
		worker.use(
			http.get('/api/vmcps/vmcp-mail/oauth-status', () => new HttpResponse(null, { status: 500 }))
		);
		await renderConnectedList();
		await page.getByRole('button', { name: 'Actions for Mail vMCP', exact: true }).click();
		await vi.waitFor(() => expect(status).toHaveBeenCalledOnce());
		await expect(status.mock.results[0].value).rejects.toBeDefined();
		await expect.element(page.getByRole('button', { name: 'Reset', exact: true })).toBeVisible();
		await expect
			.element(page.getByRole('button', { name: 'Reauthenticate', exact: true }))
			.not.toBeInTheDocument();
	});
});
