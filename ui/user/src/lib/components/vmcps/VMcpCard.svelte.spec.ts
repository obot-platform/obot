import { Group, type VMCPInstance } from '$lib/services';
import { mcpServersAndEntries, vmcpInstances } from '$lib/stores';
import { createMCPCatalogEntry, createVMCP } from '../../../tests/helpers/mcp';
import { createMockProfile, preparePageData } from '../../../tests/helpers/pageData';
import { getProfileResponse } from '../../../tests/mocks/data';
import { worker } from '../../../tests/mocks/worker';
import VMcpCardHost from './VMcpCard.svelte.spec.host.svelte';
import { http, HttpResponse } from 'msw';
import { createRawSnippet } from 'svelte';
import { describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

const icon = createRawSnippet(() => ({ render: () => '<span>icon</span>' }));

function createInstance(id: string): VMCPInstance {
	return {
		id,
		vmcpID: 'vmcp-1',
		userID: getProfileResponse.id,
		created: '2026-01-01T00:00:00Z'
	};
}

async function renderCard(options: {
	groups: string[];
	userID?: string;
	instances?: VMCPInstance[];
	vmcp?: ReturnType<typeof createVMCP>;
	onUpdate?: (vmcp: unknown) => void;
	provideSelectInstance?: boolean;
	provideDiff?: boolean;
	provideUpdateConfirm?: boolean;
}) {
	const vmcp = options.vmcp
		? options.userID !== undefined
			? { ...options.vmcp, userID: options.userID }
			: options.vmcp
		: createVMCP({
				id: 'vmcp-1',
				displayName: 'Issue Tracker vMCP',
				userID: options.userID
			});
	await preparePageData({
		profile: createMockProfile(options.groups)
	});
	vmcpInstances.current = {
		items: options.instances ?? [],
		loading: false
	};
	return render(VMcpCardHost, {
		vmcp,
		selectAriaLabel: 'Open Issue Tracker vMCP',
		onDelete: () => {},
		onConnect: () => {},
		onUpdate: options.onUpdate,
		icon,
		provideSelectInstance: options.provideSelectInstance,
		provideDiff: options.provideDiff,
		provideUpdateConfirm: options.provideUpdateConfirm
	});
}

function createNeedsUpdateVmcp() {
	const entry = createMCPCatalogEntry({ id: 'entry-1', name: 'GitHub' });
	return createVMCP(
		{
			id: 'vmcp-1',
			displayName: 'Issue Tracker vMCP',
			status: {
				components: [{ name: 'GitHub', needsUpdate: true }]
			}
		},
		[entry]
	);
}

async function expectDeleteVisible(visible: boolean) {
	await page.getByRole('button', { name: 'Actions for Issue Tracker vMCP' }).click();
	const deleteButton = page.getByRole('button', { name: 'Delete', exact: true });
	if (visible) {
		await expect.element(deleteButton).toBeVisible();
	} else {
		await expect.element(deleteButton).not.toBeInTheDocument();
	}
}

async function expectConnectEnabled(enabled: boolean) {
	const connect = page.getByRole('button', { name: 'Connect', exact: true });
	await expect.element(connect).toBeVisible();
	if (enabled) {
		await expect.element(connect).toBeEnabled();
	} else {
		await expect.element(connect).toBeDisabled();
	}
}

describe('VMcpCard.svelte', () => {
	it.each([
		{
			name: 'lets the creator delete and connect',
			groups: [Group.USER],
			userID: getProfileResponse.id,
			deleteVisible: true,
			connectEnabled: true
		},
		{
			name: "lets an admin delete someone else's vMCP while disabling connect",
			groups: [Group.ADMIN],
			userID: 'someone-else',
			deleteVisible: true,
			connectEnabled: false
		},
		{
			name: "hides delete and disables connect for a non-admin viewing someone else's vMCP",
			groups: [Group.USER],
			userID: 'someone-else',
			deleteVisible: false,
			connectEnabled: false
		},
		{
			name: "hides delete and disables connect for a readonly admin viewing someone else's vMCP",
			groups: [Group.AUDITOR],
			userID: 'someone-else',
			deleteVisible: false,
			connectEnabled: false
		},
		{
			name: 'lets a non-admin connect to an unowned vMCP without deleting',
			groups: [Group.USER],
			userID: undefined,
			deleteVisible: false,
			connectEnabled: true
		},
		{
			name: 'lets an admin delete and connect to an unowned vMCP',
			groups: [Group.ADMIN],
			userID: undefined,
			deleteVisible: true,
			connectEnabled: true
		},
		{
			name: 'lets a readonly admin connect to an unowned vMCP without deleting',
			groups: [Group.AUDITOR],
			userID: undefined,
			deleteVisible: false,
			connectEnabled: true
		},
		{
			name: 'treats an empty userID as unowned',
			groups: [Group.USER],
			userID: '',
			deleteVisible: false,
			connectEnabled: true
		}
	] as const)('$name', async ({ groups, userID, deleteVisible, connectEnabled }) => {
		await renderCard({ groups: [...groups], userID });
		await expectConnectEnabled(connectEnabled);
		await expectDeleteVisible(deleteVisible);
	});

	it('shows disconnect when connected and deletes a single instance', async () => {
		const instance = createInstance('vmcpi-1');
		const deleted = vi.fn();
		worker.use(
			http.delete('/api/vmcp-instances/vmcpi-1', () => {
				deleted();
				return HttpResponse.json({});
			})
		);

		await renderCard({
			groups: [Group.USER],
			instances: [instance]
		});

		await page.getByRole('button', { name: 'Actions for Issue Tracker vMCP' }).click();
		await page.getByRole('button', { name: 'Disconnect', exact: true }).click();
		await vi.waitFor(() => {
			expect(deleted).toHaveBeenCalledOnce();
			expect(vmcpInstances.current.items).toEqual([]);
		});
	});

	it('shows update action for owners when an update is available', async () => {
		const updated = vi.fn();
		const vmcp = createNeedsUpdateVmcp();
		worker.use(
			http.post('/api/vmcps/vmcp-1/trigger-update', () => {
				updated();
				return HttpResponse.json({ id: 'vmcp-1' });
			})
		);

		await renderCard({
			groups: [Group.USER],
			userID: getProfileResponse.id,
			vmcp,
			onUpdate: updated
		});

		await page.getByRole('button', { name: 'Actions for Issue Tracker vMCP' }).click();
		await page.getByRole('button', { name: 'Update VMCP', exact: true }).click();
		await page.getByRole('button', { name: "Yes, I'm sure", exact: true }).click();
		await vi.waitFor(() => expect(updated).toHaveBeenCalledOnce());
	});

	it('shows view diff when an update is available', async () => {
		const entry = createMCPCatalogEntry({ id: 'entry-1', name: 'GitHub' });
		const vmcp = createNeedsUpdateVmcp();
		mcpServersAndEntries.current = {
			entries: [entry],
			servers: [],
			userConfiguredServers: [],
			userInstances: [],
			loading: false,
			lastFetched: null,
			isInitialized: true
		};

		await renderCard({
			groups: [Group.USER],
			userID: 'someone-else',
			vmcp
		});

		await page.getByRole('button', { name: 'Actions for Issue Tracker vMCP' }).click();
		await page.getByRole('button', { name: 'View Diff', exact: true }).click();
		await expect.element(page.getByText('Issue Tracker vMCP | vmcp-1')).toBeVisible();
	});

	it('hides update action for non-owners without admin access', async () => {
		await renderCard({
			groups: [Group.USER],
			userID: 'someone-else',
			vmcp: createNeedsUpdateVmcp()
		});

		await page.getByRole('button', { name: 'Actions for Issue Tracker vMCP' }).click();
		await expect
			.element(page.getByRole('button', { name: 'Update VMCP', exact: true }))
			.not.toBeInTheDocument();
	});

	it('opens instance selection when disconnecting with multiple connections', async () => {
		const instances = [createInstance('vmcpi-1'), createInstance('vmcpi-2')];
		await renderCard({
			groups: [Group.USER],
			instances
		});

		await page.getByRole('button', { name: 'Actions for Issue Tracker vMCP' }).click();
		await page.getByRole('button', { name: 'Disconnect', exact: true }).click();
		await expect
			.element(page.getByRole('heading', { name: 'Select Connection to Disconnect' }))
			.toBeVisible();
		await expect.element(page.getByText('vmcpi-1')).toBeVisible();
		await expect.element(page.getByText('vmcpi-2')).toBeVisible();
	});
});
