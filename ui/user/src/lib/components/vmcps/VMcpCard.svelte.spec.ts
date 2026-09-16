import { Group, type VMCPInstance, type VMCPManifest } from '$lib/services';
import { mcpServersAndEntries, vmcpInstances } from '$lib/stores';
import { createMCPCatalogEntry, createVMCP, createVMCPComponent } from '../../../tests/helpers/mcp';
import { createMockProfile, preparePageData } from '../../../tests/helpers/pageData';
import { getProfileResponse } from '../../../tests/mocks/data';
import { worker } from '../../../tests/mocks/worker';
import VMcpCardHost from './VMcpCard.svelte.spec.host.svelte';
import { http, HttpResponse } from 'msw';
import { createRawSnippet } from 'svelte';
import { beforeEach, describe, expect, it, vi } from 'vitest';
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

function createUnconfiguredVmcp() {
	const vmcp = createVMCP({
		id: 'vmcp-1',
		displayName: 'Issue Tracker vMCP',
		userID: getProfileResponse.id
	});
	vmcp.components![0].configuration = [{ key: 'API_TOKEN', policy: 'userAllowed' }];
	vmcp.components![0].catalogEntry = {
		manifest: {
			...vmcp.components![0].catalogEntry!.manifest,
			config: [
				{
					key: 'API_TOKEN',
					name: 'API token',
					description: 'Token',
					required: true,
					sensitive: true,
					value: '',
					usage: 'env'
				}
			]
		}
	};
	return vmcp;
}

function createUnconfiguredInstance(id: string): VMCPInstance {
	return {
		...createInstance(id),
		status: { missingRequiredConfiguration: ['component-entry-default.API_TOKEN'] }
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
	owner?: string;
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
	const instances = options.instances ?? [];
	await preparePageData({
		profile: createMockProfile(options.groups)
	});
	worker.use(http.get('/api/vmcp-instances', () => HttpResponse.json({ items: instances })));
	vmcpInstances.current = {
		items: instances,
		loading: false
	};
	await vmcpInstances.refresh();
	return render(VMcpCardHost, {
		vmcp,
		selectAriaLabel: 'Open Issue Tracker vMCP',
		onDelete: () => {},
		onConnect: () => {},
		onUpdate: options.onUpdate,
		icon,
		provideSelectInstance: options.provideSelectInstance,
		provideDiff: options.provideDiff,
		provideUpdateConfirm: options.provideUpdateConfirm,
		owner: options.owner
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

function actionsButton() {
	return page.getByRole('button', { name: 'Actions for Issue Tracker vMCP' });
}

async function expectDeleteVisible(visible: boolean, actionsVisible = true) {
	if (!actionsVisible) {
		await expect.element(actionsButton()).not.toBeInTheDocument();
		return;
	}
	await actionsButton().click();
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
	beforeEach(async () => {
		await vmcpInstances.refresh();
		mcpServersAndEntries.current = {
			entries: [],
			servers: [],
			userConfiguredServers: [],
			userInstances: [],
			loading: false,
			lastFetched: null,
			isInitialized: true
		};
		vmcpInstances.current = { items: [], loading: false };
	});
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
			connectEnabled: false,
			actionsVisible: false
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
			connectEnabled: true,
			actionsVisible: false
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
			connectEnabled: true,
			actionsVisible: false
		}
	] as const)('$name', async ({ groups, userID, deleteVisible, connectEnabled, ...rest }) => {
		const actionsVisible = 'actionsVisible' in rest ? rest.actionsVisible : true;
		await renderCard({ groups: [...groups], userID });
		await expectConnectEnabled(connectEnabled);
		await expectDeleteVisible(deleteVisible, actionsVisible);
	});

	it('shows reset when connected and deletes a single instance', async () => {
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
		await page.getByRole('button', { name: 'Reset', exact: true }).click();
		await vi.waitFor(() => {
			expect(deleted).toHaveBeenCalledOnce();
			expect(vmcpInstances.current.items).toEqual([]);
		});
	});

	it('shows update action for owners when an update is available', async () => {
		const triggered = vi.fn();
		const onUpdate = vi.fn();
		const vmcp = createNeedsUpdateVmcp();
		const refreshed = {
			...vmcp,
			status: { components: [{ name: 'GitHub', needsUpdate: false }] }
		};
		worker.use(
			http.post('/api/vmcps/vmcp-1/trigger-update', () => {
				triggered();
				return HttpResponse.json({});
			}),
			http.get('/api/vmcps/vmcp-1', () => HttpResponse.json(refreshed))
		);

		await renderCard({
			groups: [Group.USER],
			userID: getProfileResponse.id,
			vmcp,
			onUpdate
		});

		await page.getByRole('button', { name: 'Actions for Issue Tracker vMCP' }).click();
		await page.getByRole('button', { name: 'Update vMCP', exact: true }).click();
		await page.getByRole('button', { name: "Yes, I'm sure", exact: true }).click();
		await vi.waitFor(() => {
			expect(triggered).toHaveBeenCalledOnce();
			expect(onUpdate).toHaveBeenCalledWith(refreshed);
		});
	});

	it('collects latest catalog configuration before updating', async () => {
		const snapshotEntry = createMCPCatalogEntry({ id: 'entry-1', name: 'GitHub' });
		const latestEntry = createMCPCatalogEntry({
			id: 'entry-1',
			name: 'GitHub',
			manifest: {
				config: [
					{
						key: 'API_TOKEN',
						name: 'API token',
						description: 'Token',
						required: true,
						sensitive: true,
						value: '',
						usage: 'env'
					}
				]
			}
		});
		const vmcp = createVMCP(
			{
				id: 'vmcp-1',
				displayName: 'Issue Tracker vMCP',
				status: {
					components: [{ name: 'GitHub', needsUpdate: true }]
				},
				components: [
					createVMCPComponent(snapshotEntry, {
						configuration: [{ key: 'OLD_KEY', policy: 'fixed', value: 'stale' }]
					})
				]
			},
			[snapshotEntry]
		);
		mcpServersAndEntries.current = {
			...mcpServersAndEntries.current,
			entries: [latestEntry]
		};

		const put = vi.fn();
		const triggered = vi.fn();
		const onUpdate = vi.fn();
		const refreshed = {
			...vmcp,
			status: { components: [{ name: 'GitHub', needsUpdate: false }] }
		};
		worker.use(
			http.get('/api/vmcps/vmcp-1', () => HttpResponse.json(refreshed)),
			http.post('/api/vmcps/vmcp-1/reveal', () => HttpResponse.json({ components: {} })),
			http.put('/api/vmcps/vmcp-1', async ({ request }) => {
				put(await request.json());
				return HttpResponse.json(vmcp);
			}),
			http.post('/api/vmcps/vmcp-1/trigger-update', () => {
				triggered();
				return HttpResponse.json({});
			})
		);

		await renderCard({
			groups: [Group.ADMIN],
			userID: getProfileResponse.id,
			vmcp,
			onUpdate
		});

		await page.getByRole('button', { name: 'Actions for Issue Tracker vMCP' }).click();
		await page.getByRole('button', { name: 'Update vMCP', exact: true }).click();
		await page.getByRole('button', { name: "Yes, I'm sure", exact: true }).click();

		await expect.element(page.getByRole('heading', { name: /Configure GitHub/ })).toBeVisible();
		expect(triggered).not.toHaveBeenCalled();
		expect(onUpdate).not.toHaveBeenCalled();
		await expect.element(page.getByRole('combobox', { name: 'API token policy' })).toBeVisible();
		await expect
			.element(page.getByRole('combobox', { name: 'OLD_KEY policy' }))
			.not.toBeInTheDocument();

		await page
			.getByRole('combobox', { name: 'API token policy' })
			.selectOptions('Provided at connection');
		await page.getByRole('button', { name: 'Update', exact: true }).click();

		await vi.waitFor(() => {
			expect(put).toHaveBeenCalledOnce();
			expect(triggered).toHaveBeenCalledOnce();
			expect(onUpdate).toHaveBeenCalledWith(refreshed);
		});
		expect(((put.mock.calls[0][0] as VMCPManifest).components ?? [])[0]?.configuration).toEqual([
			{ key: 'API_TOKEN', policy: 'userAllowed' }
		]);
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
			groups: [Group.ADMIN],
			userID: 'someone-else',
			vmcp
		});

		await actionsButton().click();
		await page.getByRole('button', { name: 'View Diff', exact: true }).click();
		await expect.element(page.getByText('Issue Tracker vMCP | vmcp-1')).toBeVisible();
	});

	it('hides update action for non-owners without admin access', async () => {
		await renderCard({
			groups: [Group.USER],
			userID: 'someone-else',
			vmcp: createNeedsUpdateVmcp()
		});

		await expect.element(actionsButton()).not.toBeInTheDocument();
	});

	it('opens instance selection when resetting with multiple connections', async () => {
		const instances = [createInstance('vmcpi-1'), createInstance('vmcpi-2')];
		await renderCard({
			groups: [Group.USER],
			instances
		});

		await page.getByRole('button', { name: 'Actions for Issue Tracker vMCP' }).click();
		await page.getByRole('button', { name: 'Reset', exact: true }).click();
		await expect
			.element(page.getByRole('heading', { name: 'Select Connection to Disconnect' }))
			.toBeVisible();
		await expect.element(page.getByText('vmcpi-1')).toBeVisible();
		await expect.element(page.getByText('vmcpi-2')).toBeVisible();
	});

	it('resets the selected instance when multiple connections exist', async () => {
		const deleted = vi.fn();
		worker.use(
			http.delete('/api/vmcp-instances/:id', ({ params }) => {
				deleted(params.id);
				return HttpResponse.json({});
			})
		);

		await renderCard({
			groups: [Group.USER],
			instances: [createInstance('vmcpi-1'), createInstance('vmcpi-2')]
		});

		await page.getByRole('button', { name: 'Actions for Issue Tracker vMCP' }).click();
		await page.getByRole('button', { name: 'Reset', exact: true }).click();
		await page.getByRole('button', { name: 'Select connection' }).nth(1).click();
		await vi.waitFor(() => {
			expect(deleted).toHaveBeenCalledWith('vmcpi-2');
			expect(vmcpInstances.current.items.map((instance) => instance.id)).toEqual(['vmcpi-1']);
		});
	});

	it('opens Edit Configuration when the current instance is missing required fields', async () => {
		const vmcp = createVMCP({
			id: 'vmcp-1',
			displayName: 'Issue Tracker vMCP',
			userID: getProfileResponse.id
		});
		vmcp.components![0].configuration = [{ key: 'API_TOKEN', policy: 'userAllowed' }];
		vmcp.components![0].catalogEntry.manifest.config = [
			{
				key: 'API_TOKEN',
				name: 'API token',
				description: 'Token',
				required: true,
				sensitive: true,
				value: '',
				usage: 'env'
			}
		];
		worker.use(
			http.post('/api/vmcp-instances/vmcpi-1/reveal', () =>
				HttpResponse.json({ components: { [vmcp.components![0].id!]: { API_TOKEN: '' } } })
			)
		);

		await renderCard({
			groups: [Group.USER],
			userID: getProfileResponse.id,
			vmcp,
			instances: [
				{
					...createInstance('vmcpi-1'),
					status: { missingRequiredConfiguration: ['component-entry-default.API_TOKEN'] }
				}
			]
		});

		await page.getByRole('button', { name: 'Actions for Issue Tracker vMCP' }).click();
		await page.getByRole('button', { name: 'Edit Configuration', exact: true }).click();
		await expect.element(page.getByCSS('input[name="API token"]')).toBeVisible();
	});

	it('clears Not Configured after Edit Configuration succeeds', async () => {
		const vmcp = createVMCP({
			id: 'vmcp-1',
			displayName: 'Issue Tracker vMCP',
			userID: getProfileResponse.id
		});
		vmcp.components![0].configuration = [{ key: 'API_TOKEN', policy: 'userAllowed' }];
		vmcp.components![0].catalogEntry.manifest.config = [
			{
				key: 'API_TOKEN',
				name: 'API token',
				description: 'Token',
				required: true,
				sensitive: true,
				value: '',
				usage: 'env'
			}
		];
		const stale = {
			...createInstance('vmcpi-1'),
			status: {
				configured: false,
				missingRequiredConfiguration: ['component-entry-default.API_TOKEN']
			}
		};
		worker.use(
			http.post('/api/vmcp-instances/vmcpi-1/reveal', () =>
				HttpResponse.json({
					components: { [vmcp.components![0].id!]: { API_TOKEN: 'saved-token' } }
				})
			),
			http.post('/api/vmcp-instances/vmcpi-1/configure', () => HttpResponse.json(stale)),
			http.get('/api/vmcp-instances/vmcpi-1', () =>
				HttpResponse.json({
					...stale,
					status: { configured: true }
				})
			),
			http.post('/api/vmcps/vmcp-1/launch', () => HttpResponse.json({})),
			http.get('/api/vmcps/vmcp-1/oauth-url', () => HttpResponse.json({ oauthURL: '' }))
		);

		await renderCard({
			groups: [Group.USER],
			userID: getProfileResponse.id,
			vmcp,
			instances: [stale],
			owner: 'Owner'
		});

		await expect.element(page.getByText('Not Configured')).toBeVisible();
		await page.getByRole('button', { name: 'Actions for Issue Tracker vMCP' }).click();
		await page.getByRole('button', { name: 'Edit Configuration', exact: true }).click();
		const tokenField = page.getByCSS('input[name="API token"]');
		await expect.element(tokenField).toBeVisible();
		await tokenField.click();
		await tokenField.fill('secret-token');
		await page.getByRole('button', { name: 'Update', exact: true }).click();

		await vi.waitFor(() => {
			expect(vmcpInstances.current.items[0]?.status?.missingRequiredConfiguration ?? []).toEqual(
				[]
			);
		});
		await expect.element(page.getByText('Not Configured')).not.toBeInTheDocument();
	});

	it('opens Edit Configuration for the selected unconfigured instance', async () => {
		const vmcp = createUnconfiguredVmcp();
		const revealed = vi.fn();
		worker.use(
			http.post('/api/vmcp-instances/:id/reveal', ({ params }) => {
				revealed(params.id);
				return HttpResponse.json({
					components: { [vmcp.components![0].id!]: { API_TOKEN: '' } }
				});
			})
		);

		await renderCard({
			groups: [Group.USER],
			userID: getProfileResponse.id,
			vmcp,
			instances: [createUnconfiguredInstance('vmcpi-1'), createUnconfiguredInstance('vmcpi-2')]
		});

		await page.getByRole('button', { name: 'Actions for Issue Tracker vMCP' }).click();
		await page.getByRole('button', { name: 'Edit Configuration', exact: true }).click();
		await expect
			.element(page.getByRole('heading', { name: 'Select Connection to Configure' }))
			.toBeVisible();
		await page.getByRole('button', { name: 'Select connection' }).nth(1).click();
		await expect.element(page.getByCSS('input[name="API token"]')).toBeVisible();
		expect(revealed).toHaveBeenCalledWith('vmcpi-2');
		expect(revealed).not.toHaveBeenCalledWith('vmcpi-1');
	});

	it('shows instance selection when multiple connections exist', async () => {
		const vmcp = createUnconfiguredVmcp();
		const revealed = vi.fn();
		worker.use(
			http.post('/api/vmcp-instances/:id/reveal', ({ params }) => {
				revealed(params.id);
				return HttpResponse.json({
					components: { [vmcp.components![0].id!]: { API_TOKEN: '' } }
				});
			})
		);

		await renderCard({
			groups: [Group.USER],
			userID: getProfileResponse.id,
			vmcp,
			instances: [createInstance('vmcpi-1'), createUnconfiguredInstance('vmcpi-2')]
		});

		await page.getByRole('button', { name: 'Actions for Issue Tracker vMCP' }).click();
		await page.getByRole('button', { name: 'Edit Configuration', exact: true }).click();
		await expect
			.element(page.getByRole('heading', { name: 'Select Connection to Configure' }))
			.toBeVisible();
		await page.getByText('vmcpi-1', { exact: true }).click();
		await expect.element(page.getByCSS('input[name="API token"]')).toBeVisible();
		expect(revealed).toHaveBeenCalledWith('vmcpi-1');
		expect(revealed).not.toHaveBeenCalledWith('vmcpi-2');
	});

	it('opens Edit Configuration when the instance is fully configured', async () => {
		const vmcp = createVMCP({
			id: 'vmcp-1',
			displayName: 'Issue Tracker vMCP',
			userID: getProfileResponse.id
		});
		vmcp.components![0].configuration = [{ key: 'API_TOKEN', policy: 'userAllowed' }];
		vmcp.components![0].catalogEntry.manifest.config = [
			{
				key: 'API_TOKEN',
				name: 'API token',
				description: 'Token',
				required: true,
				sensitive: true,
				value: '',
				usage: 'env'
			}
		];
		worker.use(
			http.post('/api/vmcp-instances/vmcpi-1/reveal', () =>
				HttpResponse.json({
					components: { [vmcp.components![0].id!]: { API_TOKEN: 'saved-token' } }
				})
			)
		);

		await renderCard({
			groups: [Group.USER],
			userID: getProfileResponse.id,
			vmcp,
			instances: [createInstance('vmcpi-1')]
		});

		await page.getByRole('button', { name: 'Actions for Issue Tracker vMCP' }).click();
		await page.getByRole('button', { name: 'Edit Configuration', exact: true }).click();
		await expect.element(page.getByCSS('input[name="API token"]')).toBeVisible();
	});
});
