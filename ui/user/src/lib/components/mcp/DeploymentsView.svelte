<script lang="ts">
	import { resolve } from '$app/paths';
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import Confirm from '$lib/components/Confirm.svelte';
	import DotDotDot from '$lib/components/DotDotDot.svelte';
	import DiffDialog from '$lib/components/admin/DiffDialog.svelte';
	import McpConfirmDelete from '$lib/components/mcp/McpConfirmDelete.svelte';
	import McpMultiDeleteBlockedDialog from '$lib/components/mcp/McpMultiDeleteBlockedDialog.svelte';
	import Table, { type InitSort, type InitSortFn } from '$lib/components/table/Table.svelte';
	import { ADMIN_SESSION_STORAGE } from '$lib/constants';
	import Loading from '$lib/icons/Loading.svelte';
	import {
		AdminService,
		UserService,
		type MCPCatalogEntry,
		type MCPCatalogServer,
		type OrgUser,
		MCPCompositeDeletionDependencyError
	} from '$lib/services';
	import {
		getMCPDisplayName,
		getMcpServerDeploymentStatus,
		getServerTypeLabel,
		getServerUrl,
		isMultiUserServer
	} from '$lib/services/user/mcp';
	import { profile, mcpServersAndEntries, version } from '$lib/stores';
	import { formatTimeAgo } from '$lib/time';
	import { getUserDisplayName, openUrl } from '$lib/utils';
	import CapacityBanner from './CapacityBanner.svelte';
	import ConnectToServer from './ConnectToServer.svelte';
	import EditExistingDeployment from './EditExistingDeployment.svelte';
	import { delay } from 'es-toolkit';
	import {
		Captions,
		CircleAlert,
		CircleFadingArrowUp,
		Ellipsis,
		ExternalLink,
		GitCompare,
		Power,
		Server,
		ServerCog,
		Trash2,
		TriangleAlert,
		UsersIcon
	} from 'lucide-svelte';
	import { onDestroy, onMount, type Snippet } from 'svelte';

	interface Props {
		usersMap?: Map<string, OrgUser>;
		entity?: 'workspace' | 'catalog';
		classes?: {
			tableHeader?: string;
		};
		id?: string;
		readonly?: boolean;
		query?: string;
		urlFilters?: Record<string, (string | number)[]>;
		onFilter?: (property: string, values: string[]) => void;
		onClearAllFilters?: () => void;
		onSort?: InitSortFn;
		onReload?: () => void;
		initSort?: InitSort;
		noDataContent?: Snippet;
		onlyMyServers?: boolean;
		servers?: MCPCatalogServer[];
	}

	let {
		entity = 'catalog',
		usersMap = new Map(),
		id,
		readonly,
		query,
		urlFilters: filters,
		classes,
		onFilter,
		onClearAllFilters,
		onSort,
		onReload,
		initSort = { property: 'created', order: 'desc' },
		noDataContent,
		onlyMyServers,
		servers: initialServers
	}: Props = $props();

	const doesSupportK8sUpdates = $derived(version.current.engine === 'kubernetes');

	const hasAdminAccess = $derived(profile.current.hasAdminAccess?.() ?? false);

	let loading = $state(false);

	let diffDialog = $state<ReturnType<typeof DiffDialog>>();
	let existingServer = $state<MCPCatalogServer>();
	let updatedServer = $state<MCPCatalogServer | MCPCatalogEntry>();

	let showUpgradeConfirm = $state<
		{ type: 'multi' } | { type: 'single'; server: MCPCatalogServer } | undefined
	>();
	let showK8sUpgradeConfirm = $state<
		{ type: 'multi' } | { type: 'single'; server: MCPCatalogServer } | undefined
	>(undefined);

	let showDeleteConfirm = $state<
		| { type: 'multi' }
		| { type: 'single'; server: MCPCatalogServer; onConfirm?: () => void }
		| undefined
	>();
	let selected = $state<Record<string, MCPCatalogServer>>({});
	let updating = $state<Record<string, { inProgress: boolean; error: string }>>({});
	let deleting = $state(false);
	let restarting = $state(false);

	let deleteConflictError = $state<MCPCompositeDeletionDependencyError | undefined>();

	let deployedCatalogEntryServers = $state<MCPCatalogServer[]>([]);
	let deployedWorkspaceCatalogEntryServers = $state<MCPCatalogServer[]>([]);
	let serversData = $derived.by(() => {
		if (initialServers) return initialServers;
		if (entity === 'workspace') {
			return mcpServersAndEntries.current.userConfiguredServers.filter((server) => !server.deleted);
		}
		const seen: Record<string, boolean> = {};
		return [
			...deployedCatalogEntryServers,
			...deployedWorkspaceCatalogEntryServers,
			...mcpServersAndEntries.current.servers
		].filter((server) => {
			if (server.deleted || seen[server.id]) return false;
			seen[server.id] = true;
			return true;
		});
	});

	let instancesMap = $derived(
		new Map(
			mcpServersAndEntries.current.userInstances.map((instance) => [instance.mcpServerID, instance])
		)
	);
	let tableRef = $state<ReturnType<typeof Table>>();

	let entriesMap = $derived(
		mcpServersAndEntries.current.entries.reduce<Record<string, MCPCatalogEntry>>((acc, entry) => {
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
				const compositeParentName = compositeParent ? getMCPDisplayName(compositeParent) : '';

				const instance = instancesMap.get(deployment.id);
				const { updateStatus, updatesAvailable, updateStatusTooltip } =
					instance?.configured === false
						? {
								updateStatus: 'Not Configured',
								updatesAvailable: ['Not Configured'],
								updateStatusTooltip: undefined
							}
						: getMcpServerDeploymentStatus(deployment, doesSupportK8sUpdates);

				return {
					...deployment,
					displayName: getMCPDisplayName(deployment),
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
						: false,
					isMyServer:
						(deployment.catalogEntryID && deployment.userID === profile.current.id) ||
						(powerUserID === profile.current.id && powerUserWorkspaceID === id),
					updateStatus,
					updatesAvailable,
					updateStatusTooltip
				};
			})
			.filter((d) => !d.disabled && (onlyMyServers ? d.isMyServer : true));

		return query
			? transformedData.filter((d) => d.displayName.toLowerCase().includes(query.toLowerCase()))
			: transformedData;
	});

	let connectToServerDialog = $state<ReturnType<typeof ConnectToServer>>();
	let editExistingDialog = $state<ReturnType<typeof EditExistingDeployment>>();
	let capacityBanner = $state<ReturnType<typeof CapacityBanner>>();

	let pollingInterval: ReturnType<typeof setInterval> | undefined;
	const POLL_INTERVAL_MS = 10000; // 10 seconds

	async function checkAndPoll() {
		const hasActiveStartupState = tableData.some((row) => row.deploymentStatus === 'Progressing');

		if (hasActiveStartupState) {
			await reload(false);
		}
	}

	onMount(async () => {
		// Start checking for progressing servers
		pollingInterval = setInterval(() => checkAndPoll(), POLL_INTERVAL_MS);
	});

	onDestroy(() => {
		if (pollingInterval) {
			clearInterval(pollingInterval);
		}
	});

	async function reload(isInitialLoad: boolean = false) {
		// Only show loading spinner on initial load, not during background polling
		if (isInitialLoad) {
			loading = true;
		}

		if (onReload) {
			await onReload();
		} else {
			if (entity === 'catalog' && profile.current.hasAdminAccess?.() && id) {
				deployedCatalogEntryServers =
					await AdminService.listAllCatalogDeployedSingleRemoteServers(id);
				deployedWorkspaceCatalogEntryServers =
					await AdminService.listAllWorkspaceDeployedSingleRemoteServers();
				// Refresh multi-user servers too
				await mcpServersAndEntries.refreshAll();
				// Refresh capacity banner when server list changes
				if (!isInitialLoad) {
					capacityBanner?.refresh();
				}
			} else if (!isInitialLoad && entity === 'workspace') {
				await mcpServersAndEntries.refreshAll();
			}
		}

		if (isInitialLoad) {
			loading = false;
		}
	}

	function hasInstanceConfiguration(server: MCPCatalogServer) {
		return (server.manifest.multiUserConfig?.userDefinedHeaders?.length ?? 0) > 0;
	}

	function canTriggerUpdate(server: MCPCatalogServer) {
		if (server.compositeName) return false;
		if (!isMultiUserServer(server)) return true;
		return !!server.catalogEntryID && (!!server.powerUserWorkspaceID || !!id);
	}

	async function handleBulkUpdate() {
		for (const serverId of Object.keys(selected)) {
			const server = selected[serverId];
			// if doesn't need update or is child server of composite mcp
			if (!server.needsUpdate || !canTriggerUpdate(server)) {
				continue;
			}
			const prompted = await updateServer(server);
			if (prompted) {
				selected = {};
				tableRef?.clearSelectAll();
				return;
			}
		}

		selected = {};
		tableRef?.clearSelectAll();
	}

	async function handleK8sBulkUpdate(selections: typeof selected) {
		const serversToUpdate = Object.values(selections).filter(
			(server) => server.needsK8sUpdate && !server.compositeName
		);
		return Promise.all(serversToUpdate.map((server) => updateK8sSettings(server)));
	}

	async function handleBulkRestart() {
		restarting = true;
		try {
			for (const id of Object.keys(selected)) {
				if (!selected[id].configured) {
					// skip unconfigured servers
					continue;
				}
				if (selected[id].powerUserWorkspaceID) {
					await UserService.restartWorkspaceK8sServerDeployment(
						selected[id].powerUserWorkspaceID,
						id
					);
				} else {
					await UserService.restartK8sDeployment(id);
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

	async function updateCatalogServerAndPromptForConfiguration(server: MCPCatalogServer) {
		if (!isMultiUserServer(server) || !server.catalogEntryID) return false;

		const entry = entriesMap[server.catalogEntryID];
		if (!entry) {
			// Without the entry manifest loaded, fall back to the plain update path.
			if (server.powerUserWorkspaceID) {
				await UserService.triggerWorkspaceMcpServerUpdate(
					server.powerUserWorkspaceID,
					server.catalogEntryID,
					server.id
				);
			} else if (id) {
				await AdminService.triggerMcpCatalogServerUpdate(id, server.id);
			}
			return false;
		}

		return (
			(await editExistingDialog?.updateFromCatalogEntry({
				server,
				entry
			})) ?? false
		);
	}

	async function updateServer(server?: MCPCatalogServer) {
		if (!server || !canTriggerUpdate(server)) return false;

		updating[server.id] = { inProgress: true, error: '' };
		let prompted = false;
		try {
			if (isMultiUserServer(server)) {
				if (server.catalogEntryID) {
					// Catalog-backed multi-user servers may need shared config after the manifest update.
					prompted = await updateCatalogServerAndPromptForConfiguration(server);
				} else if (server.powerUserWorkspaceID) {
					prompted = false;
				} else if (id) {
					await AdminService.triggerMcpCatalogServerUpdate(id, server.id);
				}
			} else {
				await UserService.triggerMcpServerUpdate(server.id);
			}
		} catch (err) {
			updating[server.id] = {
				inProgress: false,
				error: err instanceof Error ? err.message : 'An unknown error occurred'
			};
		}

		delete updating[server.id];
		return prompted;
	}
	async function updateK8sSettings(server?: MCPCatalogServer) {
		if (!server) return;
		updating[server.id] = { inProgress: true, error: '' };

		const mcpServerId = server.id;
		const catalogEntryId = server.catalogEntryID;
		// Use powerUserWorkspaceID if available, otherwise use the component's workspace id
		const workspaceId = server.powerUserWorkspaceID || (entity === 'workspace' ? id : undefined);

		let result: unknown | undefined = undefined;

		try {
			result = await (workspaceId
				? catalogEntryId
					? UserService.redeployWorkspaceCatalogEntryServerWithK8sSettings(
							workspaceId,
							catalogEntryId,
							mcpServerId
						)
					: UserService.redeployWorkspaceK8sServerWithK8sSettings(workspaceId, mcpServerId)
				: catalogEntryId
					? AdminService.redeployMCPCatalogServerWithK8sSettings(catalogEntryId, mcpServerId)
					: AdminService.redeployWithK8sSettings(mcpServerId, server.mcpCatalogID));
		} catch (err) {
			updating[server.id] = {
				inProgress: false,
				error: err instanceof Error ? err.message : 'An unknown error occurred'
			};

			return undefined;
		}

		delete updating[server.id];
		return result;
	}

	async function handleSingleDelete(server: MCPCatalogServer) {
		if (server.compositeName) {
			return;
		}
		if (!isMultiUserServer(server) && server.catalogEntryID) {
			await UserService.deleteSingleOrRemoteMcpServer(server.id);
			// Decrement the count of servers in the catalog
			const entry = mcpServersAndEntries.current.entries.find(
				(entry) => entry.id === server.catalogEntryID
			);
			if (entry?.userCount) entry.userCount--;
		} else {
			// multi-user
			try {
				if (server.powerUserWorkspaceID) {
					await UserService.deleteWorkspaceMCPCatalogServer(server.powerUserWorkspaceID, server.id);
				} else if (profile.current.hasAdminAccess?.() && id) {
					await AdminService.deleteMCPCatalogServer(id, server.id);
				}
				// Remove server from list
				mcpServersAndEntries.current.servers = mcpServersAndEntries.current.servers.filter(
					(s) => s.id !== server.id
				);
			} catch (error) {
				if (error instanceof MCPCompositeDeletionDependencyError) {
					deleteConflictError = error;
					return;
				}

				throw error;
			}
		}

		// Immediately refresh capacity banner for admin users after any server deletion
		if (entity === 'catalog' && profile.current.hasAdminAccess?.()) {
			capacityBanner?.refresh();
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
				entityId: belongsToWorkspace ? item.powerUserWorkspaceID : id
			})
		);
	}

	function getAuditLogsUrl(d: MCPCatalogServer) {
		const isMultiUser = !d.catalogEntryID;
		const isComposite = !!d.compositeName;

		const useAdminUrl = profile.current.hasAdminAccess?.();
		if (isComposite) {
			return useAdminUrl
				? `/admin/audit-logs?mcp_id=${d.compositeName}`
				: `/audit-logs?mcp_id=${d.compositeName}`;
		}
		return isMultiUser
			? useAdminUrl
				? `/admin/audit-logs?mcp_server_display_name=${d.manifest.name}`
				: `/audit-logs?mcp_server_display_name=${d.manifest.name}`
			: useAdminUrl
				? `/admin/audit-logs?mcp_id=${d.id}`
				: `/audit-logs?mcp_id=${d.id}`;
	}

	function getMcpCatalogUrl(d: MCPCatalogServer) {
		// If this is a component of a composite server, link to the parent composite server
		if (d.compositeName) {
			const parent = compositeMapping[d.compositeName];
			if (parent) {
				// Recursively get the parent's catalog URL
				return getMcpCatalogUrl(parent);
			}
		}

		// The menu label is "View Catalog Entry" whenever the deployment has a
		// catalogEntryID, so link to the catalog entry in that case. This includes
		// multi-user servers deployed from a catalog entry, which carry both a
		// catalogEntryID and a multiUser server type. Servers without a catalog
		// entry link to the server itself ("View Server").

		// Workspace-specific server (power user workspace)
		if (d.powerUserWorkspaceID) {
			// Workspace catalog entry deployment
			if (d.catalogEntryID) {
				return `/admin/mcp-catalog/w/${d.powerUserWorkspaceID}/c/${d.catalogEntryID}`;
			}

			// Workspace multi-user server
			return `/admin/mcp-catalog/w/${d.powerUserWorkspaceID}/s/${d.id}`;
		}

		// Global catalog entry deployment
		if (d.catalogEntryID) {
			return `/admin/mcp-catalog/c/${d.catalogEntryID}`;
		}

		// Global multi-user server
		return `/admin/mcp-catalog/s/${d.id}`;
	}
</script>

<div class="flex flex-col gap-0.5">
	{#if loading}
		<div class="my-2 flex items-center justify-center h-72">
			<Loading class="size-6" />
		</div>
	{:else}
		{#if entity === 'catalog' && profile.current.hasAdminAccess?.()}
			<CapacityBanner bind:this={capacityBanner} />
		{/if}
		{#if tableData.length > 0}
			<Table
				bind:this={tableRef}
				data={tableData}
				fields={entity === 'workspace'
					? [
							'displayName',
							'type',
							...(doesSupportK8sUpdates ? ['deploymentStatus'] : []),
							'updatesAvailable',
							'created'
						]
					: [
							'displayName',
							'type',
							...(doesSupportK8sUpdates ? ['deploymentStatus'] : []),
							'updatesAvailable',
							'userName',
							'registry',
							'created'
						]}
				filterable={[
					'displayName',
					'type',
					'deploymentStatus',
					'updatesAvailable',
					'userName',
					'registry'
				].filter(Boolean) as string[]}
				{filters}
				headers={[
					{ title: 'Name', property: 'displayName' },
					{ title: 'User', property: 'userName' },
					{ title: 'Health', property: 'deploymentStatus' },
					{ title: 'Update Status', property: 'updatesAvailable' }
				]}
				onClickRow={(d, isCtrlClick) => {
					setLastVisitedMcpServer(d);

					const url = getServerUrl(d);
					openUrl(url, isCtrlClick);
				}}
				{onFilter}
				{onClearAllFilters}
				{onSort}
				{initSort}
				sortable={['displayName', 'type', 'updatesAvailable', 'userName', 'registry', 'created']}
				noDataMessage="No catalog servers added."
				classes={{
					root: 'rounded-none rounded-b-md shadow-none',
					thead: classes?.tableHeader
				}}
				setRowClasses={(d) => {
					if (d.needsUpdate && d.needsK8sUpdate) {
						return 'bg-orange-500/5 hover:bg-orange-500/10 border-orange-500/20';
					}

					if (d.needsUpdate) {
						return 'bg-primary/5 hover:bg-primary/10 border-primary/20';
					}

					if (d.needsK8sUpdate) {
						return 'bg-warning/5 hover:bg-warning/10 border-warning/20';
					}

					return '';
				}}
			>
				{#snippet onRenderColumn(property, d)}
					{#if property === 'displayName'}
						<div class="flex shrink-0 items-center gap-2">
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
									<span class="text-muted-content text-xs">
										({d.compositeParentName})
									</span>
								{/if}
							</p>
							{#if 'missingKubernetesSecret' in d && d.missingKubernetesSecret}
								<div
									class="text-warning"
									use:tooltip={{
										text: 'Missing Kubernetes Secret.',
										classes: ['break-words', 'w-58']
									}}
								>
									<TriangleAlert class="size-4" />
								</div>
							{:else if d.needsUpdate}
								<div
									use:tooltip={{
										text: 'This deployment needs an update. View Diff to see the changes.',
										classes: ['wrap-break-word', 'w-58']
									}}
								>
									<CircleFadingArrowUp class="text-primary size-4" />
								</div>
							{/if}
						</div>
					{:else if property === 'created'}
						{formatTimeAgo(d.created).relativeTime}
					{:else if property === 'updatesAvailable'}
						<div
							use:tooltip={{ text: d.updateStatusTooltip ?? '', classes: ['whitespace-pre-line'] }}
						>
							{d.updateStatus || '--'}
						</div>
					{:else if property === 'deploymentStatus'}
						{d.deploymentStatus || '--'}
					{:else if property === 'type'}
						{d.type}
						{#if d.serverUserType === 'multiUser'}
							<div class="p-2" use:tooltip={{ text: 'Multi-tenant' }}>
								<UsersIcon class="size-3 text-muted-content" />
							</div>
						{/if}
					{:else}
						{d[property as keyof typeof d]}
					{/if}
				{/snippet}

				{#snippet actions(d)}
					{@const isComposite = !!d.compositeName}
					{@const auditLogsUrl = getAuditLogsUrl(d)}
					{@const instance = instancesMap.get(d.id)}
					<DotDotDot class="hover:dark:bg-base-100/50" classes={{ menu: 'p-0 gap-0' }}>
						{#snippet icon()}
							<Ellipsis class="size-4" />
						{/snippet}

						{#snippet children({ toggle })}
							<div class="flex flex-col gap-1 p-2">
								<a
									class="menu-button"
									href={resolve(getMcpCatalogUrl(d) as `/${string}`)}
									onclick={(ev) => {
										ev.stopPropagation();
										const hasAdminAccess = profile.current.hasAdminAccess?.();
										if (!hasAdminAccess) {
											ev.preventDefault();
										}
									}}
								>
									<ExternalLink class="size-4" />
									<span>
										{#if d.catalogEntryID}
											View Catalog Entry
										{:else}
											View Server
										{/if}
									</span>
								</a>
								{#if instance && hasInstanceConfiguration(d)}
									<button
										class="menu-button"
										onclick={(e) => {
											e.stopPropagation();
											connectToServerDialog?.open({
												server: d,
												instance,
												configureInstance: true
											});
											toggle(false);
										}}
									>
										<ServerCog class="size-4" /> Edit Configuration
									</button>
								{/if}
								{#if d.needsUpdate && canTriggerUpdate(d) && (d.isMyServer || (hasAdminAccess && !readonly))}
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
											toggle(false);
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
											<Loading class="size-4" />
										{:else}
											<CircleFadingArrowUp class="size-4" />
										{/if}
										Update Server
									</button>
								{/if}

								{#if d.catalogEntryID && d.needsUpdate}
									<button
										class="menu-button-primary"
										disabled={updating[d.id]?.inProgress || readonly || !!d.compositeName}
										onclick={(e) => {
											e.stopPropagation();
											if (!d.catalogEntryID) return;

											existingServer = d;
											updatedServer = entriesMap[d.catalogEntryID];
											diffDialog?.open();
											toggle(false);
										}}
									>
										<GitCompare class="size-4" /> View Diff
									</button>
								{/if}

								{#if (d.isMyServer || (hasAdminAccess && !readonly)) && d.needsK8sUpdate}
									<button
										class="menu-button-primary bg-warning/10 text-warning hover:bg-warning/20"
										disabled={updating[d.id]?.inProgress || readonly || !!d.compositeName}
										onclick={(e) => {
											e.stopPropagation();
											if (!d) return;
											showK8sUpgradeConfirm = {
												type: 'single',
												server: d
											};
											toggle(false);
										}}
									>
										{#if updating[d.id]?.inProgress}
											<Loading class="size-4" />
										{:else}
											<CircleFadingArrowUp class="size-4" />
										{/if}
										Update Scheduling Config
									</button>
								{/if}

								{#if d.isMyServer || (hasAdminAccess && !readonly)}
									<button
										class="menu-button"
										disabled={restarting}
										onclick={async (e) => {
											e.stopPropagation();
											restarting = true;
											if (d.powerUserWorkspaceID) {
												await UserService.restartWorkspaceK8sServerDeployment(
													d.powerUserWorkspaceID,
													d.id
												);
											} else {
												await UserService.restartK8sDeployment(d.id);
											}

											await delay(1000);

											toggle((restarting = false));
										}}
									>
										{#if restarting}
											<Loading class="size-4" /> Restarting...
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

								{#if d.isMyServer || (hasAdminAccess && !readonly)}
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
						(s) => s.configured
					).length}
					{@const upgradeableCount = Object.values(currentSelected).filter(
						(s) => s.needsUpdate && canTriggerUpdate(s)
					).length}
					{@const k8sUpgradeableCount = Object.values(currentSelected).filter(
						(s) => s.needsK8sUpdate && !s.compositeName
					).length}
					{@const deletableCount = Object.values(currentSelected).filter(
						(s) => !s.compositeName
					).length}

					<div class="flex grow items-center justify-end gap-2 px-4 py-2">
						<button
							class="btn btn-secondary flex items-center gap-1 text-sm font-normal"
							onclick={() => {
								selected = currentSelected;
								handleBulkRestart();
							}}
							disabled={restarting || readonly || restartableCount === 0}
						>
							{#if restarting}
								<Loading class="size-4 self-center" /> Restarting...
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
							class="btn btn-secondary flex items-center gap-1 text-sm font-normal"
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
							class="btn btn-secondary flex items-center gap-1 text-sm font-normal"
							onclick={() => {
								selected = currentSelected;
								const type = Object.keys(selected).length > 1 ? 'multi' : 'single';

								if (type === 'multi') {
									showK8sUpgradeConfirm = {
										type: 'multi'
									};
								} else {
									const server = type === 'single' ? Object.values(selected)[0] : undefined;
									showK8sUpgradeConfirm = {
										type: 'single',
										server: server!
									};
								}
							}}
							disabled={readonly || k8sUpgradeableCount === 0}
						>
							<CircleFadingArrowUp class="size-4" /> Kubernetes Upgrade
							{#if k8sUpgradeableCount > 0 && !readonly}
								<span class="pill-primary">
									{k8sUpgradeableCount}
								</span>
							{/if}
						</button>
						<button
							class="btn btn-secondary flex items-center gap-1 text-sm font-normal"
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
		{:else}
			{@render noDataContent?.()}
		{/if}
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
		await reload();
		showUpgradeConfirm = undefined;
	}}
	oncancel={() => (showUpgradeConfirm = undefined)}
	loading={Object.values(updating).some((u) => u.inProgress)}
	type="info"
	title="Confirm Update"
>
	{#snippet msgContent()}
		<h4 class="flex items-center justify-center gap-2 text-lg font-semibold">
			<CircleAlert class="size-5" />
			{`Update ${showUpgradeConfirm?.type === 'single' ? showUpgradeConfirm.server.id : 'selected server(s)'}?`}
		</h4>
	{/snippet}
	{#snippet note()}
		<p class="text-sm font-light">
			If this update introduces new required shared configuration parameters, you will be prompted
			to supply them after the update is applied.
		</p>
	{/snippet}
</Confirm>

<Confirm
	show={!!showK8sUpgradeConfirm}
	onsuccess={async () => {
		if (!showK8sUpgradeConfirm) return;

		const { type } = showK8sUpgradeConfirm;

		if (type === 'single') {
			const { server } = showK8sUpgradeConfirm;
			await updateK8sSettings(server);
		} else {
			await handleK8sBulkUpdate(selected);
			selected = {};
			tableRef?.clearSelectAll();
		}

		await reload();
		showK8sUpgradeConfirm = undefined;
	}}
	oncancel={() => (showK8sUpgradeConfirm = undefined)}
	loading={Object.values(updating).some((u) => u.inProgress)}
	type="info"
	title="Confirm Update"
>
	{#snippet msgContent()}
		<h4 class="flex items-center justify-center gap-2 text-lg font-semibold">
			<CircleAlert class="size-5" />
			Update Kubernetes Settings
		</h4>
	{/snippet}
	{#snippet note()}
		<p class="text-sm font-light">
			{#if showK8sUpgradeConfirm?.type === 'multi'}
				The selected servers ({Object.values(selected).filter(
					(s) => s.needsK8sUpdate && !s.compositeName
				).length})
			{:else}
				The <span class="font-medium"
					>{showK8sUpgradeConfirm?.server.compositeName ??
						showK8sUpgradeConfirm?.server.manifest.name}</span
				> server
			{/if}

			will be redeployed with the latest Kubernetes settings.
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

<ConnectToServer
	bind:this={connectToServerDialog}
	userConfiguredServers={mcpServersAndEntries.current.userConfiguredServers}
	onConnect={async () => {
		await reload();
	}}
/>

<EditExistingDeployment
	bind:this={editExistingDialog}
	onUpdateConfigure={async () => {
		await reload();
	}}
/>
