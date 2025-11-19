<script lang="ts">
	import { page } from '$app/state';
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import DiffDialog from '$lib/components/admin/DiffDialog.svelte';
	import Confirm from '$lib/components/Confirm.svelte';
	import DotDotDot from '$lib/components/DotDotDot.svelte';
	import McpConfirmDelete from '$lib/components/mcp/McpConfirmDelete.svelte';
	import McpMultiDeleteBlockedDialog from '$lib/components/mcp/McpMultiDeleteBlockedDialog.svelte';
	import Table, { type InitSort, type InitSortFn } from '$lib/components/table/Table.svelte';
	import { ADMIN_SESSION_STORAGE } from '$lib/constants';
	import { getAdminMcpServerAndEntries } from '$lib/context/admin/mcpServerAndEntries.svelte';
	import { getUserMcpServerAndEntries } from '$lib/context/mcpServerAndEntries.svelte';
	import {
		AdminService,
		ChatService,
		type MCPCatalogEntry,
		type MCPCatalogServer,
		type OrgUser,
		MCPCompositeDeletionDependencyError,
		type MCPServerInstance
	} from '$lib/services';
	import { getServerTypeLabel } from '$lib/services/chat/mcp';
	import { profile } from '$lib/stores';
	import { formatTimeAgo } from '$lib/time';
	import { setSearchParamsToLocalStorage } from '$lib/url';
	import { getUserDisplayName, openUrl } from '$lib/utils';
	import { delay } from 'es-toolkit';
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
	import { onMount, type Snippet } from 'svelte';

	interface Props {
		usersMap?: Map<string, OrgUser>;
		entity?: 'workspace' | 'catalog';
		catalogId?: string;
		readonly?: boolean;
		query?: string;
		urlFilters?: Record<string, (string | number)[]>;
		onFilter?: (property: string, values: string[]) => void;
		onClearAllFilters?: () => void;
		onSort?: InitSortFn;
		initSort?: InitSort;
	}

	let {
		entity = 'catalog',
		usersMap = new Map(),
		catalogId,
		readonly,
		query,
		urlFilters: filters,
		onFilter,
		onClearAllFilters,
		onSort,
		initSort
	}: Props = $props();
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
	let restarting = $state(false);

	let deleteConflictError = $state<MCPCompositeDeletionDependencyError | undefined>();

	let mcpServerAndEntries =
		entity === 'workspace' ? getUserMcpServerAndEntries() : getAdminMcpServerAndEntries();
	let deployedCatalogEntryServers = $state<MCPCatalogServer[]>([]);
	let deployedWorkspaceCatalogEntryServers = $state<MCPCatalogServer[]>([]);
	let serversData = $derived(
		entity === 'workspace'
			? mcpServerAndEntries.servers.filter((server) => !server.deleted)
			: [
					...deployedCatalogEntryServers.filter((server) => !server.deleted),
					...deployedWorkspaceCatalogEntryServers.filter((server) => !server.deleted),
					...mcpServerAndEntries.servers.filter((server) => !server.deleted)
				]
	);

	let instances = $state<MCPServerInstance[]>([]);

	let tableRef = $state<ReturnType<typeof Table>>();

	let entriesMap = $derived(
		mcpServerAndEntries.entries.reduce<Record<string, MCPCatalogEntry>>((acc, entry) => {
			acc[entry.id] = entry;
			return acc;
		}, {})
	);

	let compositeMapping = $derived(
		serversData
			.filter((server) => 'compositeConfig' in server.manifest)
			.reduce<Record<string, MCPCatalogServer>>((acc, server) => {
				acc[server.id] = server;
				return acc;
			}, {})
	);

	let tableData = $derived.by(() => {
		function isCompositeDescendantDisabled(parent: MCPCatalogServer, id: string) {
			const match = parent.manifest.compositeConfig?.componentServers.find(
				(component) => component.catalogEntryID === id || component.mcpServerID === id
			);
			return match ? match.disabled : false;
		}

		const transformedData = serversData
			.map((deployment) => {
				const powerUserWorkspaceID =
					deployment.powerUserWorkspaceID ||
					(deployment.catalogEntryID
						? entriesMap[deployment.catalogEntryID]?.powerUserWorkspaceID
						: undefined);
				const powerUserID = deployment.catalogEntryID
					? entriesMap[deployment.catalogEntryID]?.powerUserID
					: powerUserWorkspaceID
						? deployment.userID
						: undefined;

				const compositeParent =
					deployment.compositeName && compositeMapping[deployment.compositeName];
				const compositeParentName = compositeParent
					? compositeParent.alias || compositeParent.manifest.name
					: '';
				return {
					...deployment,
					displayName: deployment.alias || deployment.manifest.name || '',
					userName: getUserDisplayName(usersMap, deployment.userID),
					registry: powerUserID ? getUserDisplayName(usersMap, powerUserID) : 'Global Registry',
					type: getServerTypeLabel(deployment),
					powerUserWorkspaceID,
					compositeParentName,
					disabled: compositeParent
						? isCompositeDescendantDisabled(
								compositeParent,
								deployment.catalogEntryID || deployment.mcpCatalogID || deployment.id
							)
						: false
				};
			})
			.filter((d) => !d.disabled);

		return query
			? transformedData.filter((d) => d.displayName.toLowerCase().includes(query.toLowerCase()))
			: transformedData;
	});

	onMount(() => {
		reload();
	});

	async function reload() {
		loading = true;

		if (entity === 'catalog' && profile.current.hasAdminAccess?.() && catalogId) {
			deployedCatalogEntryServers =
				await AdminService.listAllCatalogDeployedSingleRemoteServers(catalogId);
			deployedWorkspaceCatalogEntryServers =
				await AdminService.listAllWorkspaceDeployedSingleRemoteServers();
		}
		instances = await ChatService.listMcpServerInstances();
		loading = false;
	}

	async function handleBulkUpdate() {
		for (const id of Object.keys(selected)) {
			// if doesn't need update or is child server of composite mcp
			if (!selected[id].needsUpdate || (selected[id].needsUpdate && selected[id].compositeName)) {
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
		restarting = true;
		try {
			for (const id of Object.keys(selected)) {
				if (selected[id].manifest.runtime === 'remote' || !selected[id].configured) {
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
			restarting = false;
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
		if (server.compositeName) {
			return;
		}
		if (server.catalogEntryID) {
			await ChatService.deleteSingleOrRemoteMcpServer(server.id);
			// Decrement the count of servers in the catalog
			const entry = mcpServerAndEntries.entries.find((entry) => entry.id === server.catalogEntryID);
			if (entry?.userCount) entry.userCount--;
		} else {
			// multi-user
			try {
				if (server.powerUserWorkspaceID) {
					await ChatService.deleteWorkspaceMCPCatalogServer(server.powerUserWorkspaceID, server.id);
				} else if (profile.current.hasAdminAccess?.() && catalogId) {
					await AdminService.deleteMCPCatalogServer(catalogId, server.id);
				}
				// Remove server from list
				mcpServerAndEntries.servers = mcpServerAndEntries.servers.filter((s) => s.id !== server.id);
			} catch (error) {
				if (error instanceof MCPCompositeDeletionDependencyError) {
					deleteConflictError = error;
					return;
				}

				throw error;
			}
		}
	}

	async function handleBulkDelete() {
		for (const id of Object.keys(selected)) {
			// Skip descendants of composite servers; they cannot be deleted directly
			if (selected[id].compositeName) continue;
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

	function getAuditLogsUrl(d: MCPCatalogServer) {
		const isMultiUser = !d.catalogEntryID;
		const isComposite = !!d.compositeName;

		if (isComposite) {
			return `/admin/audit-logs?mcp_id=${d.compositeName}`;
		}
		return isMultiUser
			? `/admin/audit-logs?mcp_server_display_name=${d.manifest.name}`
			: `/admin/audit-logs?mcp_id=${d.id}`;
	}

	function getServerUrl(d: MCPCatalogServer) {
		const belongsToWorkspace = d.powerUserWorkspaceID ? true : false;
		const isMulti = !d.catalogEntryID;

		let url = '';
		if (entity === 'catalog' && profile.current.hasAdminAccess?.()) {
			if (isMulti) {
				url = belongsToWorkspace
					? `/admin/mcp-servers/w/${d.powerUserWorkspaceID}/s/${d.id}/details`
					: `/admin/mcp-servers/s/${d.id}/details`;
			} else {
				url = belongsToWorkspace
					? `/admin/mcp-servers/w/${d.powerUserWorkspaceID}/c/${d.catalogEntryID}/instance/${d.id}`
					: `/admin/mcp-servers/c/${d.catalogEntryID}/instance/${d.id}`;
			}
		} else {
			url = isMulti
				? `/mcp-servers/s/${d.id}/details`
				: `/mcp-servers/c/${d.catalogEntryID}/instance/${d.id}`;
		}
		return url;
	}
</script>

<div class="flex flex-col gap-2">
	{#if loading || mcpServerAndEntries.loading}
		<div class="my-2 flex items-center justify-center">
			<LoaderCircle class="size-6 animate-spin" />
		</div>
	{:else if serversData.length === 0}
		<div class="my-12 flex w-md flex-col items-center gap-4 self-center text-center">
			<Server class="text-surface3 size-24" />
			<h4 class="text-on-surface1 text-lg font-semibold">No current deployments.</h4>
			<p class="text-on-surface1 text-sm font-light">
				Once a server has been deployed, its <br />
				information will be quickly accessible here.
			</p>
		</div>
	{:else}
		<Table
			bind:this={tableRef}
			data={tableData}
			fields={entity === 'workspace'
				? ['displayName', 'type', 'deploymentStatus', 'created']
				: ['displayName', 'type', 'deploymentStatus', 'userName', 'registry', 'created']}
			filterable={['displayName', 'type', 'deploymentStatus', 'userName', 'registry']}
			{filters}
			headers={[
				{ title: 'Name', property: 'displayName' },
				{ title: 'User', property: 'userName' },
				{ title: 'Status', property: 'deploymentStatus' }
			]}
			onClickRow={(d, isCtrlClick) => {
				setLastVisitedMcpServer(d);

				const url = getServerUrl(d);

				setSearchParamsToLocalStorage(page.url.pathname, page.url.search);
				openUrl(url, isCtrlClick);
			}}
			{onFilter}
			{onClearAllFilters}
			{onSort}
			{initSort}
			sortable={['displayName', 'type', 'deploymentStatus', 'userName', 'registry', 'created']}
			noDataMessage="No catalog servers added."
			classes={{
				root: 'rounded-none rounded-b-md shadow-none',
				thead: 'top-31'
			}}
		>
			{#snippet onRenderColumn(property, d)}
				{#if property === 'displayName'}
					<div class="flex flex-shrink-0 items-center gap-2">
						<div class="icon">
							{#if d.manifest.icon}
								<img src={d.manifest.icon} alt={d.manifest.name} class="size-6" />
							{:else}
								<Server class="size-6" />
							{/if}
						</div>
						<p class="flex flex-col">
							{d.displayName}
							{#if d.compositeParentName}
								<span class="text-on-surface1 text-xs">
									({d.compositeParentName})
								</span>
							{/if}
						</p>
					</div>
				{:else if property === 'created'}
					{formatTimeAgo(d.created).relativeTime}
				{:else if property === 'deploymentStatus'}
					<div class="flex items-center gap-2">
						{d.deploymentStatus || '--'}
						{#if d.needsUpdate && !d.compositeName}
							<div use:tooltip={'Upgrade available'}>
								<CircleFadingArrowUp class="text-primary size-4" />
							</div>
						{/if}
					</div>
				{:else}
					{d[property as keyof typeof d]}
				{/if}
			{/snippet}

			{#snippet actions(d)}
				{@const isComposite = !!d.compositeName}
				{@const auditLogsUrl = getAuditLogsUrl(d)}
				<DotDotDot class="icon-button hover:dark:bg-background/50">
					{#snippet icon()}
						<Ellipsis class="size-4" />
					{/snippet}

					{#snippet children({ toggle })}
						<div class="default-dialog flex min-w-max flex-col gap-1 p-2">
							{#if d.needsUpdate}
								{#if !readonly}
									<button
										class="menu-button-primary"
										disabled={updating[d.id]?.inProgress || readonly || !!d.compositeName}
										onclick={(e) => {
											e.stopPropagation();
											if (!d) return;
											showUpgradeConfirm = {
												type: 'single',
												server: d
											};
										}}
										use:tooltip={d.compositeName
											? {
													text: 'This is a component of a composite server and cannot be updated independently; update the composite MCP server instead',
													classes: ['w-md'],
													disablePortal: true
												}
											: undefined}
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
									class="menu-button-primary"
									disabled={updating[d.id]?.inProgress || readonly || !!d.compositeName}
									onclick={(e) => {
										e.stopPropagation();
										if (!d.catalogEntryID) return;

										existingServer = d;
										updatedServer = entriesMap[d.catalogEntryID];
										diffDialog?.open();
									}}
								>
									<GitCompare class="size-4" /> View Diff
								</button>
							{/if}
							{#if d.manifest.runtime !== 'remote' && !readonly}
								<button
									class="menu-button"
									disabled={restarting}
									onclick={async (e) => {
										e.stopPropagation();
										restarting = true;
										if (d.powerUserWorkspaceID) {
											await ChatService.restartWorkspaceK8sServerDeployment(
												d.powerUserWorkspaceID,
												d.id
											);
										} else {
											await AdminService.restartK8sDeployment(d.id);
										}

										await delay(1000);

										toggle((restarting = false));
									}}
								>
									{#if restarting}
										<LoaderCircle class="size-4 animate-spin" /> Restarting...
									{:else}
										<Power class="size-4" />
										Restart Server
									{/if}
								</button>
							{/if}
							<button
								onclick={(e) => {
									e.stopPropagation();
									const isCtrlClick = e.ctrlKey || e.metaKey;
									setSearchParamsToLocalStorage(page.url.pathname, page.url.search);
									openUrl(auditLogsUrl, isCtrlClick);
								}}
								class="menu-button text-left"
							>
								<Captions class="size-4" />
								{#if isComposite}
									View Parent Server <br /> Audit Logs
								{:else}
									View Audit Logs
								{/if}
							</button>
							{#if !readonly}
								<button
									class="menu-button-destructive"
									onclick={async (e) => {
										e.stopPropagation();
										showDeleteConfirm = {
											type: 'single',
											server: d
										};

										toggle(false);
									}}
									use:tooltip={d.compositeName
										? {
												text: 'Cannot directly update a descendant of a composite server; update the composite MCP server instead.',
												classes: ['w-md'],
												disablePortal: true
											}
										: undefined}
									disabled={!!d.compositeName}
								>
									<Trash2 class="size-4" /> Delete Server
								</button>
							{/if}
						</div>
					{/snippet}
				</DotDotDot>
			{/snippet}

			{#snippet tableSelectActions(currentSelected)}
				{@const restartableCount = Object.values(currentSelected).filter(
					(s) => s.manifest.runtime !== 'remote' && s.configured
				).length}
				{@const upgradeableCount = Object.values(currentSelected).filter(
					(s) => s.needsUpdate && !s.compositeName
				).length}
				{@const deletableCount = Object.values(currentSelected).filter(
					(s) => !s.compositeName
				).length}
				<div class="flex grow items-center justify-end gap-2 px-4 py-2">
					<button
						class="button flex items-center gap-1 text-sm font-normal"
						onclick={() => {
							selected = currentSelected;
							handleBulkRestart();
						}}
						disabled={restarting || readonly || restartableCount === 0}
					>
						{#if restarting}
							<LoaderCircle class="size-4 animate-spin self-center" /> Restarting...
						{:else}
							<Power class="size-4" /> Restart
						{/if}
						{#if restartableCount > 0 && !readonly}
							<span class="pill-primary">
								{restartableCount}
							</span>
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
						disabled={readonly || upgradeableCount === 0}
					>
						<CircleFadingArrowUp class="size-4" /> Upgrade
						{#if upgradeableCount > 0 && !readonly}
							<span class="pill-primary">
								{upgradeableCount}
							</span>
						{/if}
					</button>
					<button
						class="button flex items-center gap-1 text-sm font-normal"
						onclick={() => {
							selected = currentSelected;
							showDeleteConfirm = {
								type: 'multi'
							};
						}}
						disabled={readonly || deletableCount === 0}
					>
						<Trash2 class="size-4" /> Delete
						{#if deletableCount > 0 && !readonly}
							<span class="pill-primary">
								{deletableCount}
							</span>
						{/if}
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
		confirm: 'bg-primary hover:bg-primary/50 transition-colors duration-200'
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

<McpConfirmDelete
	show={!!showDeleteConfirm}
	onsuccess={async () => {
		if (!showDeleteConfirm) return;
		deleting = true;
		if (showDeleteConfirm.type === 'single') {
			await handleSingleDelete(showDeleteConfirm.server);

			await delay(1000);
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
	names={showDeleteConfirm?.type === 'single'
		? [showDeleteConfirm.server.manifest.name ?? '']
		: Object.values(selected)
				.filter((s) => !s.compositeName)
				.map((s) => s.manifest.name ?? '')}
/>

<McpMultiDeleteBlockedDialog
	show={!!deleteConflictError}
	error={deleteConflictError}
	onClose={() => {
		deleteConflictError = undefined;
	}}
/>
