<script lang="ts">
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import DiffDialog from '$lib/components/admin/DiffDialog.svelte';
	import Confirm from '$lib/components/Confirm.svelte';
	import DotDotDot from '$lib/components/DotDotDot.svelte';
	import Table from '$lib/components/table/Table.svelte';
	import { ADMIN_SESSION_STORAGE } from '$lib/constants';
	import { getAdminMcpServerAndEntries } from '$lib/context/admin/mcpServerAndEntries.svelte';
	import {
		AdminService,
		ChatService,
		type MCPCatalogEntry,
		type MCPCatalogServer,
		type OrgUser
	} from '$lib/services';
	import { formatTimeAgo } from '$lib/time';
	import { getUserDisplayName, openUrl } from '$lib/utils';
	import {
		Captions,
		CircleAlert,
		CircleFadingArrowUp,
		Ellipsis,
		GitCompare,
		LoaderCircle,
		Power,
		Server,
		Trash2
	} from 'lucide-svelte';
	import { onMount } from 'svelte';

	interface Props {
		usersMap?: Map<string, OrgUser>;
		catalogId: string;
		readonly?: boolean;
		query?: string;
	}

	let { usersMap = new Map(), catalogId, readonly, query }: Props = $props();
	let loading = $state(false);

	let diffDialog = $state<ReturnType<typeof DiffDialog>>();
	let existingServer = $state<MCPCatalogServer>();
	let updatedServer = $state<MCPCatalogServer | MCPCatalogEntry>();

	let showUpgradeConfirm = $state<
		{ type: 'multi' } | { type: 'single'; server: MCPCatalogServer } | undefined
	>();
	let showDeleteConfirm = $state<
		{ type: 'multi' } | { type: 'single'; server: MCPCatalogServer } | undefined
	>();
	let selected = $state<Record<string, MCPCatalogServer>>({});
	let updating = $state<Record<string, { inProgress: boolean; error: string }>>({});
	let deleting = $state(false);

	let bulkRestarting = $state(false);

	let serversData = $state<MCPCatalogServer[]>([]);
	let mcpServerAndEntries = getAdminMcpServerAndEntries();

	let tableRef = $state<ReturnType<typeof Table>>();

	let tableData = $derived.by(() => {
		const entriesMap = mcpServerAndEntries.entries.reduce<Record<string, MCPCatalogEntry>>(
			(acc, entry) => {
				acc[entry.id] = entry;
				return acc;
			},
			{}
		);

		const transformedData = serversData.map((deployment) => {
			const powerUserWorkspaceID =
				deployment.powerUserWorkspaceID ||
				(deployment.catalogEntryID
					? entriesMap[deployment.catalogEntryID]?.powerUserWorkspaceID
					: undefined);
			const powerUserID = deployment.catalogEntryID
				? entriesMap[deployment.catalogEntryID]?.powerUserID
				: undefined;
			return {
				...deployment,
				displayName: deployment.manifest.name ?? '',
				userName: getUserDisplayName(usersMap, deployment.userID),
				registry: powerUserID ? getUserDisplayName(usersMap, powerUserID) : 'Global Registry',
				type:
					deployment.manifest.runtime === 'remote'
						? 'Remote'
						: deployment.catalogEntryID
							? 'Single User'
							: 'Multi-User',
				powerUserWorkspaceID
			};
		});

		return query
			? transformedData.filter((d) => d.displayName.toLowerCase().includes(query.toLowerCase()))
			: transformedData;
	});

	onMount(async () => {
		await reload();
	});

	async function reload() {
		loading = true;
		const deployedCatalogEntryServers =
			await AdminService.listAllCatalogDeployedSingleRemoteServers(catalogId);
		const deployedWorkspaceCatalogEntryServers =
			await AdminService.listAllWorkspaceDeployedSingleRemoteServers();

		serversData = [
			...deployedCatalogEntryServers,
			...deployedWorkspaceCatalogEntryServers,
			...mcpServerAndEntries.servers
		];
		loading = false;
	}

	async function handleBulkUpdate() {
		for (const id of Object.keys(selected)) {
			if (!selected[id].needsUpdate) {
				continue;
			}
			updating[id] = { inProgress: true, error: '' };
			try {
				await ChatService.triggerMcpServerUpdate(id);
				updating[id] = { inProgress: false, error: '' };
			} catch (error) {
				updating[id] = {
					inProgress: false,
					error: error instanceof Error ? error.message : 'An unknown error occurred'
				};
			} finally {
				delete updating[id];
			}
		}

		selected = {};
		tableRef?.clearSelectAll();
		await reload();
	}

	async function handleBulkRestart() {
		bulkRestarting = true;
		try {
			for (const id of Object.keys(selected)) {
				if (selected[id].manifest.runtime === 'remote') {
					// skip remote servers
					continue;
				}
				if (selected[id].powerUserWorkspaceID) {
					await ChatService.restartWorkspaceK8sServerDeployment(
						selected[id].powerUserWorkspaceID,
						id
					);
				} else {
					await AdminService.restartK8sDeployment(id);
				}
			}
		} catch (err) {
			console.error('Failed to restart deployments:', err);
		} finally {
			bulkRestarting = false;
			selected = {};
			tableRef?.clearSelectAll();
		}
	}

	async function updateServer(server?: MCPCatalogServer) {
		if (!server) return;
		updating[server.id] = { inProgress: true, error: '' };
		try {
			await ChatService.triggerMcpServerUpdate(server.id);
			await reload();
		} catch (err) {
			updating[server.id] = {
				inProgress: false,
				error: err instanceof Error ? err.message : 'An unknown error occurred'
			};
		}

		delete updating[server.id];
	}

	async function handleSingleDelete(server: MCPCatalogServer) {
		if (server.catalogEntryID) {
			await ChatService.deleteSingleOrRemoteMcpServer(server.id);
		} else {
			// multi-user
			if (server.powerUserWorkspaceID) {
				await ChatService.deleteWorkspaceMCPCatalogServer(server.powerUserWorkspaceID, server.id);
			} else {
				await AdminService.deleteMCPCatalogServer(catalogId, server.id);
			}
		}
	}

	async function handleBulkDelete() {
		for (const id of Object.keys(selected)) {
			await handleSingleDelete(selected[id]);
		}
		selected = {};
	}

	function setLastVisitedMcpServer(item: (typeof tableData)[0]) {
		if (!item) return;
		const belongsToWorkspace = item.powerUserWorkspaceID ? true : false;
		sessionStorage.setItem(
			ADMIN_SESSION_STORAGE.LAST_VISITED_MCP_SERVER,
			JSON.stringify({
				id: item.id,
				name: item.manifest?.name,
				type:
					item.manifest?.runtime === 'remote' ? 'remote' : item.catalogEntryID ? 'single' : 'multi',
				entity: belongsToWorkspace ? 'workspace' : 'catalog',
				entityId: belongsToWorkspace ? item.powerUserWorkspaceID : catalogId
			})
		);
	}
</script>

<div class="flex flex-col gap-2">
	{#if loading || mcpServerAndEntries.loading}
		<div class="my-2 flex items-center justify-center">
			<LoaderCircle class="size-6 animate-spin" />
		</div>
	{:else if serversData.length === 0}
		<div class="my-12 flex w-md flex-col items-center gap-4 self-center text-center">
			<Server class="dark:text-surface3 size-24 text-gray-200" />
			<h4 class="text-lg font-semibold text-gray-400 dark:text-gray-600">
				No current deployments.
			</h4>
			<p class="text-sm font-light text-gray-400 dark:text-gray-600">
				Once a server has been deployed, its <br />
				information will be quickly accessible here.
			</p>
		</div>
	{:else}
		<Table
			bind:this={tableRef}
			data={tableData}
			fields={['displayName', 'type', 'deploymentStatus', 'userName', 'registry', 'created']}
			filterable={['displayName', 'type', 'deploymentStatus', 'userName', 'registry']}
			headers={[
				{ title: 'Name', property: 'displayName' },
				{ title: 'User', property: 'userName' },
				{ title: 'Status', property: 'deploymentStatus' }
			]}
			onClickRow={(d, isCtrlClick) => {
				const isMulti = !d.catalogEntryID;
				setLastVisitedMcpServer(d);

				const belongsToWorkspace = d.powerUserWorkspaceID ? true : false;

				let url = '';
				if (isMulti) {
					url = belongsToWorkspace
						? `/admin/mcp-servers/w/${d.powerUserWorkspaceID}/s/${d.id}/details`
						: `/admin/mcp-servers/s/${d.id}/details`;
				} else {
					url = belongsToWorkspace
						? `/admin/mcp-servers/w/${d.powerUserWorkspaceID}/c/${d.catalogEntryID}/instance/${d.id}?from=deployed-servers`
						: `/admin/mcp-servers/c/${d.catalogEntryID}/instance/${d.id}?from=deployed-servers`;
				}
				openUrl(url, isCtrlClick);
			}}
			sortable={['displayName', 'type', 'deploymentStatus', 'userName', 'registry', 'created']}
			noDataMessage="No catalog servers added."
			setRowClasses={(d) => {
				if (d.needsUpdate) {
					return 'bg-blue-500/10';
				}
				return '';
			}}
			classes={{
				root: 'rounded-none rounded-b-md shadow-none',
				thead: 'top-31'
			}}
		>
			{#snippet onRenderColumn(property, d)}
				{#if property === 'displayName'}
					<div class="flex flex-shrink-0 items-center gap-2">
						<div
							class="bg-surface1 flex items-center justify-center rounded-sm p-0.5 dark:bg-gray-600"
						>
							{#if d.manifest.icon}
								<img src={d.manifest.icon} alt={d.manifest.name} class="size-6" />
							{:else}
								<Server class="size-6" />
							{/if}
						</div>
						<p class="flex items-center gap-1">
							{d.displayName}
						</p>
					</div>
				{:else if property === 'created'}
					{formatTimeAgo(d.created).relativeTime}
				{:else if property === 'deploymentStatus'}
					<div class="flex items-center gap-2">
						{d.deploymentStatus || '--'}
						{#if d.needsUpdate}
							<div use:tooltip={'Upgrade available'}>
								<CircleFadingArrowUp class="size-4 text-blue-500" />
							</div>
						{/if}
					</div>
				{:else}
					{d[property as keyof typeof d]}
				{/if}
			{/snippet}
			{#snippet actions(d)}
				<DotDotDot class="icon-button hover:dark:bg-black/50">
					{#snippet icon()}
						<Ellipsis class="size-4" />
					{/snippet}

					<div class="default-dialog flex min-w-max flex-col gap-1 p-2">
						{#if d.needsUpdate}
							{#if !readonly}
								<button
									class="menu-button-primary"
									disabled={updating[d.id]?.inProgress || readonly}
									onclick={(e) => {
										e.stopPropagation();
										if (!d) return;
										showUpgradeConfirm = {
											type: 'single',
											server: d
										};
									}}
								>
									{#if updating[d.id]?.inProgress}
										<LoaderCircle class="size-4 animate-spin" />
									{:else}
										<CircleFadingArrowUp class="size-4" />
									{/if}
									Update Server
								</button>
							{/if}
							<button
								class="menu-button"
								onclick={(e) => {
									e.stopPropagation();
									existingServer = d;
									// updatedServer = parent;
									diffDialog?.open();
								}}
							>
								<GitCompare class="size-4" /> View Diff
							</button>
						{/if}
						{#if d.manifest.runtime !== 'remote' && !readonly}
							<button
								class="menu-button"
								onclick={async (e) => {
									e.stopPropagation();
									if (d.powerUserWorkspaceID) {
										await ChatService.restartWorkspaceK8sServerDeployment(
											d.powerUserWorkspaceID,
											d.id
										);
									} else {
										await AdminService.restartK8sDeployment(d.id);
									}
								}}
							>
								<Power class="size-4" /> Restart Server
							</button>
						{/if}
						<a href={`/admin/audit-logs?mcp_id=${d.id}`} class="menu-button">
							<Captions class="size-4" /> View Audit Logs
						</a>
						{#if !readonly}
							<button
								class="menu-button-destructive"
								onclick={async (e) => {
									e.stopPropagation();
									showDeleteConfirm = {
										type: 'single',
										server: d
									};
								}}
							>
								<Trash2 class="size-4" /> Delete Server
							</button>
						{/if}
					</div>
				</DotDotDot>
			{/snippet}
			{#snippet tableSelectActions(currentSelected)}
				<div class="flex grow items-center justify-end gap-2 px-4 py-2">
					<button
						class="button flex items-center gap-1 text-sm font-normal"
						onclick={() => {
							selected = currentSelected;
							handleBulkRestart();
						}}
						disabled={bulkRestarting || readonly}
					>
						{#if bulkRestarting}
							<LoaderCircle class="size-4 animate-spin" />
						{:else}
							<Power class="size-4" /> Restart
						{/if}
					</button>
					<button
						class="button flex items-center gap-1 text-sm font-normal"
						onclick={() => {
							selected = currentSelected;
							showUpgradeConfirm = {
								type: 'multi'
							};
						}}
						disabled={readonly}
					>
						<CircleFadingArrowUp class="size-4" /> Upgrade
					</button>
					<button
						class="button flex items-center gap-1 text-sm font-normal"
						onclick={() => {
							selected = currentSelected;
							showDeleteConfirm = {
								type: 'multi'
							};
						}}
						disabled={readonly}
					>
						<Trash2 class="size-4" /> Delete
					</button>
				</div>
			{/snippet}
		</Table>
	{/if}
</div>

<DiffDialog bind:this={diffDialog} fromServer={existingServer} toServer={updatedServer} />

<Confirm
	show={!!showUpgradeConfirm}
	onsuccess={async () => {
		if (!showUpgradeConfirm) return;
		if (showUpgradeConfirm.type === 'single') {
			await updateServer(showUpgradeConfirm.server);
		} else {
			await handleBulkUpdate();
		}
		showUpgradeConfirm = undefined;
	}}
	oncancel={() => (showUpgradeConfirm = undefined)}
	classes={{
		confirm: 'bg-blue-500 hover:bg-blue-400 transition-colors duration-200'
	}}
	loading={Object.values(updating).some((u) => u.inProgress)}
>
	{#snippet title()}
		<h4 class="mb-4 flex items-center justify-center gap-2 text-lg font-semibold">
			<CircleAlert class="size-5" />
			{`Update ${showUpgradeConfirm?.type === 'single' ? showUpgradeConfirm.server.id : 'selected server(s)'}?`}
		</h4>
	{/snippet}
	{#snippet note()}
		<p class="mb-8 text-sm font-light">
			If this update introduces new required configuration parameters, users will have to supply
			them before they can use {showUpgradeConfirm?.type === 'multi'
				? 'these servers'
				: 'this server'} again.
		</p>
	{/snippet}
</Confirm>

<Confirm
	msg={showDeleteConfirm?.type === 'single'
		? 'Are you sure you want to delete this server?'
		: 'Are you sure you want to delete the selected servers?'}
	show={!!showDeleteConfirm}
	onsuccess={async () => {
		if (!showDeleteConfirm) return;
		deleting = true;
		if (showDeleteConfirm.type === 'single') {
			await handleSingleDelete(showDeleteConfirm.server);
		} else {
			await handleBulkDelete();
		}
		tableRef?.clearSelectAll();
		await reload();
		deleting = false;
		showDeleteConfirm = undefined;
	}}
	oncancel={() => (showDeleteConfirm = undefined)}
	loading={deleting}
>
	{#snippet title()}
		<h4 class="mb-4 flex items-center justify-center gap-2 text-lg font-semibold">
			<CircleAlert class="size-5" />
			{`Delete ${showDeleteConfirm?.type === 'single' ? showDeleteConfirm.server.id : 'selected server(s)'}?`}
		</h4>
	{/snippet}
	{#snippet note()}
		<div class="mb-8 text-sm font-light">
			The following servers will be permanently deleted: <span class="font-semibold"
				>{Object.values(selected)
					.map((s) => s.id)
					.join(', ')}</span
			>
		</div>
	{/snippet}
</Confirm>
