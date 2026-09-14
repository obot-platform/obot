import { Group, type VMCPInstance } from '$lib/services';
import { vmcpInstances } from '$lib/stores';
import { createMockProfile, preparePageData } from '../../../tests/helpers/pageData';
import { getProfileResponse } from '../../../tests/mocks/data';
import { worker } from '../../../tests/mocks/worker';
import VMcpCard from './VMcpCard.svelte';
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
	connected?: boolean;
	instances?: VMCPInstance[];
}) {
	await preparePageData({
		profile: createMockProfile(options.groups)
	});
	vmcpInstances.current = {
		items: options.instances ?? [],
		loading: false
	};
	return render(VMcpCard, {
		id: 'vmcp-1',
		name: 'Issue Tracker vMCP',
		selectAriaLabel: 'Open Issue Tracker vMCP',
		userID: options.userID,
		connected: options.connected,
		onDelete: () => {},
		onConnect: () => {},
		icon
	});
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
			connected: true,
			instances: [instance]
		});

		await page.getByRole('button', { name: 'Actions for Issue Tracker vMCP' }).click();
		await page.getByRole('button', { name: 'Disconnect', exact: true }).click();
		await vi.waitFor(() => expect(deleted).toHaveBeenCalledOnce());
		expect(vmcpInstances.current.items).toEqual([]);
	});

	it('opens instance selection when disconnecting with multiple connections', async () => {
		const instances = [createInstance('vmcpi-1'), createInstance('vmcpi-2')];
		await renderCard({
			groups: [Group.USER],
			connected: true,
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
