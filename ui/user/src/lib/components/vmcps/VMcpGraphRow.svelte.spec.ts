import type { EntryDrag } from '$lib/runes/vmcps/entryDrag.svelte';
import type { VMCPInstance } from '$lib/services';
import { resolveVMcpComponents } from '$lib/services/vmcps/utils';
import { vmcpInstances } from '$lib/stores';
import { createMCPCatalogEntry, createVMCP } from '../../../tests/helpers/mcp';
import { preparePageData } from '../../../tests/helpers/pageData';
import { getProfileResponse } from '../../../tests/mocks/data';
import { worker } from '../../../tests/mocks/worker';
import VMcpGraphRow from './VMcpGraphRow.svelte';
import { http, HttpResponse } from 'msw';
import { describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

const vmcp = createVMCP({ id: 'vmcp1mail', displayName: 'Mail vMCP' }, [
	createMCPCatalogEntry({ id: 'entry-outlook', name: 'Outlook', runtime: 'remote' }),
	createMCPCatalogEntry({ id: 'entry-brave', name: 'Brave Search', runtime: 'remote' })
]);

const instance: VMCPInstance = {
	id: 'vmcpi-mail',
	vmcpID: vmcp.id,
	userID: getProfileResponse.id,
	created: '2026-01-01T00:00:00Z',
	status: { configured: true }
};

function createDragStub() {
	return {
		active: false,
		isLinked: () => false,
		isComponentLinked: () => false,
		vmcpTarget: () => undefined,
		componentTarget: () => undefined
	} as unknown as EntryDrag;
}

async function renderRow(options: {
	connected: boolean;
	onReauthenticateComponent?: (componentID: string) => Promise<void>;
}) {
	await preparePageData();
	vmcpInstances.current = { items: options.connected ? [instance] : [], loading: false };
	// `context` is also a Svelte mount option, so the props must be nested.
	return render(VMcpGraphRow, {
		props: {
			vmcp,
			components: resolveVMcpComponents(vmcp),
			context: { rowY: 0, viewTop: 0, viewBottom: 10000 },
			drag: createDragStub(),
			onConnect: vi.fn(),
			onReauthenticateComponent: options.onReauthenticateComponent,
			usersMap: new Map()
		}
	});
}

describe('VMcpGraphRow.svelte', () => {
	it('reauthenticates a component the user has authenticated from its actions menu', async () => {
		const statusRequests = vi.fn();
		worker.use(
			http.get(`/api/vmcps/${vmcp.id}/oauth-status`, () => {
				statusRequests();
				return HttpResponse.json({ authenticatedComponents: ['component-entry-outlook'] });
			})
		);
		let finishReauthentication = () => {};
		const onReauthenticateComponent = vi.fn(
			() => new Promise<void>((resolve) => (finishReauthentication = resolve))
		);

		await renderRow({ connected: true, onReauthenticateComponent });

		await expect.element(page.getByRole('button', { name: 'Actions for Outlook' })).toBeVisible();
		await expect
			.element(page.getByRole('button', { name: 'Actions for Brave Search' }))
			.not.toBeInTheDocument();

		await page.getByRole('button', { name: 'Actions for Outlook' }).click();
		await page.getByRole('button', { name: 'Reauthenticate' }).click();

		await vi.waitFor(() =>
			expect(onReauthenticateComponent).toHaveBeenCalledExactlyOnceWith('component-entry-outlook')
		);
		// The menu reflects the new authorization state once the flow finishes.
		const requestsBeforeFinish = statusRequests.mock.calls.length;
		finishReauthentication();
		await vi.waitFor(() =>
			expect(statusRequests.mock.calls.length).toBeGreaterThan(requestsBeforeFinish)
		);
	});

	it('offers reauthentication once the user returns from authenticating', async () => {
		let authenticated: string[] = [];
		const answered = vi.fn();
		worker.use(
			http.get(`/api/vmcps/${vmcp.id}/oauth-status`, () => {
				answered();
				return HttpResponse.json({ authenticatedComponents: authenticated });
			})
		);

		await renderRow({ connected: true, onReauthenticateComponent: vi.fn() });
		await vi.waitFor(() => expect(answered).toHaveBeenCalledOnce());
		await expect.element(page.getByText('Outlook', { exact: true })).toBeVisible();
		await expect
			.element(page.getByRole('button', { name: 'Actions for Outlook' }))
			.not.toBeInTheDocument();

		// OAuth completes in another tab; the user then comes back to this one.
		authenticated = ['component-entry-outlook'];
		document.dispatchEvent(new Event('visibilitychange'));

		await expect.element(page.getByRole('button', { name: 'Actions for Outlook' })).toBeVisible();
	});

	it('does not offer reauthentication before the user connects', async () => {
		const statusRequests = vi.fn();
		worker.use(
			http.get(`/api/vmcps/${vmcp.id}/oauth-status`, () => {
				statusRequests();
				return HttpResponse.json({ authenticatedComponents: ['component-entry-outlook'] });
			})
		);

		await renderRow({ connected: false, onReauthenticateComponent: vi.fn() });

		await expect.element(page.getByText('Outlook', { exact: true })).toBeVisible();
		await expect
			.element(page.getByRole('button', { name: 'Actions for Outlook' }))
			.not.toBeInTheDocument();
		expect(statusRequests).not.toHaveBeenCalled();
	});
});
