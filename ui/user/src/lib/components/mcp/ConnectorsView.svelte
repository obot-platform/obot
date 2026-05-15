<script lang="ts">
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import DotDotDot from '$lib/components/DotDotDot.svelte';
	import ConnectToServer from '$lib/components/mcp/ConnectToServer.svelte';
	import McpConfirmDelete from '$lib/components/mcp/McpConfirmDelete.svelte';
	import McpMultiDeleteBlockedDialog from '$lib/components/mcp/McpMultiDeleteBlockedDialog.svelte';
	import StaticOAuthConfigureModal from '$lib/components/mcp/StaticOAuthConfigureModal.svelte';
	import Table, { type InitSort, type InitSortFn } from '$lib/components/table/Table.svelte';
	import Loading from '$lib/icons/Loading.svelte';
	import {
		AdminService,
		ChatService,
		type MCPCatalog,
		type MCPCatalogEntry,
		type MCPCatalogServer,
		type OrgUser,
		MCPCompositeDeletionDependencyError,
		Group,
		type MCPServerInstance
	} from '$lib/services';
	import type { MCPServerOAuthCredentialStatus } from '$lib/services/admin/types';
	import {
		convertEntriesAndServersToTableData,
		getServerTypeLabelByType,
		hasEditableConfiguration,
		hasMissingSecretBindingConfig,
		requiresUserUpdate
	} from '$lib/services/chat/mcp';
	import { mcpServersAndEntries, profile, version } from '$lib/stores';
	import { formatTimeAgo } from '$lib/time';
	import { openUrl, isOwnSingleUserServer } from '$lib/utils';
	import ResponsiveDialog from '../ResponsiveDialog.svelte';
	import IconButton from '../primitives/IconButton.svelte';
	import EditExistingDeployment from './EditExistingDeployment.svelte';
	import {
		Captions,
		CircleFadingArrowUp,
		Ellipsis,
		KeyRound,
		MessageCircle,
		PencilLine,
		ReceiptText,
		RefreshCw,
		SatelliteDish,
		Server,
		ServerCog,
		Settings,
		StepForward,
		Trash2,
		TriangleAlert,
		Unplug
	} from 'lucide-svelte';
	import type { Snippet } from 'svelte';
	import { slide } from 'svelte/transition';
	import { twMerge } from 'tailwind-merge';

	type Item = ReturnType<typeof convertEntriesAndServersToTableData>[number];
	type ServerSelectMode =
		| 'connect'
		| 'rename'
		| 'edit'
		| 'disconnect'
		| 'chat'
		| 'server-details'
		| 'restart'
		| 'reauthenticate';

	interface Props {
		entity?: 'workspace' | 'catalog';
		id?: string;
		catalog?: MCPCatalog;
		readonly?: boolean;
		noDataContent?: Snippet;
		usersMap?: Map<string, OrgUser>;
		query?: string;
		urlFilters?: Record<string, (string | number)[]>;
		onFilter?: (property: string, values: string[]) => void;
		onClearAllFilters?: () => void;
		onSort?: InitSortFn;
		initSort?: InitSort;
		classes?: {
			tableHeader?: string;
		};
		onConnect?: ({ instance }: { instance?: MCPServerInstance }) => void;
	}

	let {
		entity,
		id,
		catalog = $bindable(),
		readonly,
		noDataContent,
		query,
		urlFilters: filters,
		onFilter,
		onClearAllFilters,
		onSort,
		initSort = { property: 'connected', order: 'desc' },
		classes,
		onConnect,
		usersMap
	}: Props = $props();

	let deletingEntry = $state<MCPCatalogEntry>();
	let deletingServer = $state<MCPCatalogServer>();
	let selected = $state<Record<string, Item>>({});
	let confirmBulkDelete = $state(false);
	let loadingBulkDelete = $state(false);
	let deleteConflictError = $state<MCPCompositeDeletionDependencyError | undefined>();

	let connectToServerDialog = $state<ReturnType<typeof ConnectToServer>>();
	let editExistingDialog = $state<ReturnType<typeof EditExistingDeployment>>();

	let selectedConfiguredServers = $state<MCPCatalogServer[]>([]);
	let selectedEntry = $state<MCPCatalogEntry>();
	let selectServerDialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let selectServerMode = $state<ServerSelectMode>('connect');

	let oauthConfigModal = $state<ReturnType<typeof StaticOAuthConfigureModal>>();
	let oauthConfigEntry = $state<MCPCatalogEntry>();
	let oauthStatus = $state<MCPServerOAuthCredentialStatus>();

	let instancesMap = $derived(
		new Map(
			mcpServersAndEntries.current.userInstances.map((instance) => [instance.mcpServerID, instance])
		)
	);

	let entriesMap = $derived(
		new Map(mcpServersAndEntries.current.entries.map((entry) => [entry.id, entry]))
	);

	let tableData = $derived(
		convertEntriesAndServersToTableData(
			mcpServersAndEntries.current.entries,
			mcpServersAndEntries.current.servers,
			usersMap,
			mcpServersAndEntries.current.userConfiguredServers,
			mcpServersAndEntries.current.userInstances
		)
	);

	let filteredTableData = $derived.by(() => {
		const sorted = tableData.sort((a, b) => {
			return a.name.localeCompare(b.name);
		});
		return query
			? sorted.filter(
					(d) =>
						d.name.toLowerCase().includes(query.toLowerCase()) ||
						d.registry.toLowerCase().includes(query.toLowerCase())
				)
			: sorted;
	});

	function getAuditLogsUrl(d: Item) {
		let useAdminUrl =
			window.location.pathname.includes('/admin') && profile.current.hasAdminAccess?.();

		// Basic users can access audit logs for their own servers
		// Check if this is a server (not a catalog entry) belonging to the user
		let isOwnServer = 'userID' in d.data && isOwnSingleUserServer(d.data, profile.current?.id);

		let hasAuditLogUrlsAccess = isOwnServer || profile.current.groups.includes(Group.POWERUSER);

		if (!hasAuditLogUrlsAccess) {
			return null;
		}

		const isCatalogEntry = d.type === 'single' || d.type === 'remote' || d.type === 'composite';
		if (isCatalogEntry) {
			if (useAdminUrl) {
				return d.data.powerUserWorkspaceID
					? `/admin/mcp-servers/w/${d.data.powerUserWorkspaceID}/c/${d.id}?view=audit-logs`
					: `/admin/mcp-servers/c/${d.id}?view=audit-logs`;
			}

			return `/mcp-servers/c/${d.id}?view=audit-logs`;
		}

		if (useAdminUrl) {
			return d.data.powerUserWorkspaceID
				? `/admin/mcp-servers/w/${d.data.powerUserWorkspaceID}/s/${d.id}?view=audit-logs`
				: `/admin/mcp-servers/s/${d.id}?view=audit-logs`;
		}
		return `/mcp-servers/s/${d.id}?view=audit-logs`;
	}

	async function fetch() {
		mcpServersAndEntries.refreshAll();
	}

	function getConfiguredServersForCatalogEntry(entry: MCPCatalogEntry): MCPCatalogServer[] {
		return mcpServersAndEntries.current.userConfiguredServers.filter(
			(server) => server.catalogEntryID === entry.id
		);
	}

	function getUsableConfiguredServersForCatalogEntry(entry: MCPCatalogEntry): MCPCatalogServer[] {
		return getConfiguredServersForCatalogEntry(entry).filter(
			(server) =>
				!hasMissingSecretBindingConfig(
					server.manifest,
					server.missingRequiredEnvVars,
					server.missingRequiredHeader
				)
		);
	}

	function hasInstanceConfiguration(server: MCPCatalogServer) {
		return (server.manifest.multiUserConfig?.userDefinedHeaders?.length ?? 0) > 0;
	}

	function hasOAuth(server: MCPCatalogServer) {
		return (
			server.manifest.runtime === 'remote' && Object.keys(server.oauthMetadata ?? {}).length > 0
		);
	}

	async function reauthenticateServer(server: MCPCatalogServer) {
		await ChatService.clearMcpServerOAuth(server.id);
		await connectToServerDialog?.authenticate(
			server,
			server.catalogEntryID ? entriesMap.get(server.catalogEntryID) : undefined
		);
		mcpServersAndEntries.refreshUserConfiguredServers();
	}

	function handleShowSelectServerDialog(
		entry: MCPCatalogEntry,
		mode: ServerSelectMode = 'connect'
	) {
		const allServers =
			mode === 'connect' || mode === 'chat'
				? getUsableConfiguredServersForCatalogEntry(entry)
				: getConfiguredServersForCatalogEntry(entry);
		selectedConfiguredServers = allServers;
		selectedEntry = entry;
		selectServerDialog?.open();
		selectServerMode = mode;
	}

	function handleConnectToServer({ instance }: { instance?: MCPServerInstance }) {
		if (instance) {
			mcpServersAndEntries.refreshUserInstances();
		}
		onConnect?.({ instance });
	}

	async function handleConfigureOAuth(entry: MCPCatalogEntry) {
		oauthConfigEntry = entry;
		try {
			const catalogId = entry.powerUserWorkspaceID ? undefined : 'default';
			oauthStatus = entry.powerUserWorkspaceID
				? await ChatService.getWorkspaceMCPCatalogEntryOAuthCredentials(
						entry.powerUserWorkspaceID,
						entry.id
					)
				: await AdminService.getMCPCatalogEntryOAuthCredentials(catalogId!, entry.id);
		} catch {
			oauthStatus = { configured: false };
		}
		oauthConfigModal?.open();
	}

	async function handleSaveOAuth(credentials: {
		clientID: string;
		clientSecret: string;
		authorizationServerURL?: string;
	}) {
		if (!oauthConfigEntry) return;
		if (oauthConfigEntry.powerUserWorkspaceID) {
			await ChatService.setWorkspaceMCPCatalogEntryOAuthCredentials(
				oauthConfigEntry.powerUserWorkspaceID,
				oauthConfigEntry.id,
				credentials
			);
		} else {
			await AdminService.setMCPCatalogEntryOAuthCredentials(
				'default',
				oauthConfigEntry.id,
				credentials
			);
		}
		// Refresh the table to update status
		mcpServersAndEntries.refreshAll();
	}

	async function handleDeleteOAuth() {
		if (!oauthConfigEntry) return;
		if (oauthConfigEntry.powerUserWorkspaceID) {
			await ChatService.deleteWorkspaceMCPCatalogEntryOAuthCredentials(
				oauthConfigEntry.powerUserWorkspaceID,
				oauthConfigEntry.id
			);
		} else {
			await AdminService.deleteMCPCatalogEntryOAuthCredentials('default', oauthConfigEntry.id);
		}
		// Refresh the table to update status
		mcpServersAndEntries.refreshAll();
	}
</script>

<div class="flex flex-col gap-2">
	{#if mcpServersAndEntries.current.loading}
		<div class="my-2 flex items-center justify-center h-72">
			<Loading class="size-6" />
		</div>
	{:else if mcpServersAndEntries.current.entries.length + mcpServersAndEntries.current.servers.length === 0}
		{#if noDataContent}
			{@render noDataContent?.()}
		{/if}
	{:else}
		{@const hasCatalogErrors = catalog && Object.keys(catalog?.syncErrors ?? {}).length > 0}
		{#if hasCatalogErrors && !catalog?.isSyncing}
			<div class="w-full p-4" in:slide={{ axis: 'y' }} out:slide={{ axis: 'y', duration: 0 }}>
				<div class="notification-alert flex w-full items-center gap-2 rounded-md p-3 text-sm">
					<TriangleAlert class="size-" />
					<p class="">Some servers failed to sync. See "Registry Sources" tab for more details.</p>
				</div>
			</div>
		{/if}

		<Table
			data={filteredTableData}
			fields={profile.current.hasAdminAccess?.()
				? ['name', 'status', 'type', 'users', 'created', 'registry']
				: ['name', 'status', 'created', 'registry']}
			filterable={['name', 'type', 'registry', 'status']}
			{filters}
			onClickRow={(d, isCtrlClick) => {
				let url = '';
				const useAdminUrl =
					window.location.pathname.includes('/admin') && profile.current.hasAdminAccess?.();

				const matchedEntry =
					!('isCatalogEntry' in d.data) && d.data.catalogEntryID
						? entriesMap.get(d.data.catalogEntryID as string)
						: undefined;
				const powerUserWorkspaceID =
					matchedEntry?.powerUserWorkspaceID || d.data.powerUserWorkspaceID;
				if (useAdminUrl) {
					if ('isCatalogEntry' in d.data) {
						url = powerUserWorkspaceID
							? `/admin/mcp-servers/w/${powerUserWorkspaceID}/c/${d.data.id}`
							: `/admin/mcp-servers/c/${d.data.id}`;
					} else if (d.data.catalogEntryID) {
						url = powerUserWorkspaceID
							? `/admin/mcp-servers/w/${powerUserWorkspaceID}/c/${d.data.catalogEntryID}/instance/${d.id}`
							: `/admin/mcp-servers/c/${d.data.catalogEntryID}/instance/${d.id}`;
					} else {
						url = powerUserWorkspaceID
							? `/admin/mcp-servers/w/${powerUserWorkspaceID}/s/${d.id}`
							: `/admin/mcp-servers/s/${d.id}`;
					}
				} else {
					if ('isCatalogEntry' in d.data) {
						url = `/mcp-servers/c/${d.data.id}`;
					} else if (d.data.catalogEntryID) {
						url = `/mcp-servers/c/${d.data.catalogEntryID}/instance/${d.id}`;
					} else {
						url = `/mcp-servers/s/${d.id}`;
					}
				}

				openUrl(url, isCtrlClick);
			}}
			{initSort}
			{onFilter}
			{onClearAllFilters}
			{onSort}
			sortable={['name', 'status', 'type', 'users', 'created', 'registry']}
			noDataMessage="No catalog servers added."
			classes={{
				root: 'rounded-none rounded-b-md shadow-none',
				thead: classes?.tableHeader
			}}
			setRowClasses={(d) => {
				const matchingServers =
					'isCatalogEntry' in d.data ? getConfiguredServersForCatalogEntry(d.data) : [];
				const missingSecretBinding = 'missingKubernetesSecret' in d && d.missingKubernetesSecret;
				return 'isCatalogEntry' in d.data && d.data.needsUpdate && !missingSecretBinding
					? 'bg-primary/10'
					: matchingServers.some(requiresUserUpdate)
						? 'bg-warning/10'
						: '';
			}}
		>
			{#snippet onRenderColumn(property, d)}
				{@const isCatalogEntry = 'isCatalogEntry' in d.data}
				{@const catalogEntry = isCatalogEntry ? (d.data as MCPCatalogEntry) : undefined}
				{@const matchingServers = catalogEntry
					? getConfiguredServersForCatalogEntry(catalogEntry)
					: []}
				{#if property === 'name'}
					<div class="flex shrink-0 items-center gap-2">
						<div class="icon">
							{#if d.icon}
								<img src={d.icon} alt={d.name} class="size-6" />
							{:else}
								<Server class="size-6" />
							{/if}
						</div>
						<p class="flex items-center gap-2">
							{d.name}
							{#if catalogEntry?.needsUpdate && !('missingKubernetesSecret' in d && d.missingKubernetesSecret)}
								<span
									use:tooltip={{
										classes: ['border-primary', 'bg-primary/10', 'dark:bg-primary/50'],
										text: 'An update requires your attention'
									}}
								>
									<CircleFadingArrowUp class="text-primary size-4" />
								</span>
							{:else if ('missingKubernetesSecret' in d && d.missingKubernetesSecret) || matchingServers.some(requiresUserUpdate)}
								<span
									class="text-warning"
									use:tooltip={{
										text:
											'missingKubernetesSecret' in d && d.missingKubernetesSecret
												? 'Missing Kubernetes Secret.'
												: 'Server requires an update.'
									}}
								>
									<TriangleAlert class="size-4" />
								</span>
							{/if}
						</p>
					</div>
				{:else if property === 'status'}
					{#if d.status}
						<div
							class={d.status === 'Requires OAuth Config' || d.status === 'Configuration Required'
								? 'pill-warning'
								: 'pill-primary bg-primary'}
						>
							{d.status}
						</div>
					{/if}
				{:else if property === 'type'}
					{getServerTypeLabelByType(d.type)}
				{:else if property === 'created'}
					{formatTimeAgo(d.created).relativeTime}
				{:else}
					{d[property as keyof typeof d]}
				{/if}
			{/snippet}
			{#snippet actions(d)}
				{@const isCatalogEntry = 'isCatalogEntry' in d.data}
				{@const catalogEntry = isCatalogEntry ? (d.data as MCPCatalogEntry) : undefined}
				{@const auditLogUrl = getAuditLogsUrl(d)}
				{@const belongsToUser =
					(entity === 'workspace' && id && d.data.powerUserWorkspaceID === id) ||
					('catalogEntryID' in d.data && d.data.userID === profile.current.id)}
				{@const canDelete =
					d.editable && !readonly && (belongsToUser || profile.current?.hasAdminAccess?.())}
				{@const matchingServers = catalogEntry
					? getConfiguredServersForCatalogEntry(catalogEntry)
					: []}
				{@const usableMatchingServers = catalogEntry
					? getUsableConfiguredServersForCatalogEntry(catalogEntry)
					: []}
				{@const oauthServers = matchingServers.filter(hasOAuth)}
				{@const matchingInstance =
					d.connected && d.type === 'multi' ? instancesMap.get(d.data.id) : undefined}
				{@const hasConnectedOptions = isCatalogEntry
					? matchingServers.length > 0
					: !!matchingInstance}
				{@const requiresOAuth =
					catalogEntry?.manifest?.runtime === 'remote' &&
					catalogEntry.manifest?.remoteConfig?.staticOAuthRequired}
				<DotDotDot class="hover:dark:bg-base-100/50" classes={{ menu: 'p-0' }}>
					{#snippet icon()}
						<Ellipsis class="size-4" />
					{/snippet}

					{#snippet children({ toggle })}
						{#if hasConnectedOptions}
							<div
								class="bg-base-100 dark:bg-base-300 rounded-t-xl p-2 pl-4 text-[11px] font-semibold uppercase"
							>
								My Connection(s)
							</div>
							<div class="bg-base-200 flex flex-col gap-1 p-2">
								{#if !isCatalogEntry || usableMatchingServers.length > 0}
									{#if !requiresOAuth || catalogEntry?.oauthCredentialConfigured}
										{@render connectToServerAction(d.data, toggle)}
									{/if}
									{#if version.current.disableLegacyChat !== true}
										<button
											class="menu-button hover:bg-base-400"
											onclick={async (e) => {
												e.stopPropagation();
												if (catalogEntry) {
													if (usableMatchingServers.length === 1) {
														connectToServerDialog?.handleSetupChat(usableMatchingServers[0]);
													} else {
														handleShowSelectServerDialog(catalogEntry, 'chat');
													}
												} else {
													const server = d.data as MCPCatalogServer;
													const instance = instancesMap.get(d.id);
													if (instance && !instance.configured) {
														connectToServerDialog?.open({ server, instance });
													} else {
														connectToServerDialog?.handleSetupChat(server, instance);
													}
												}
												toggle(false);
											}}
										>
											<MessageCircle class="size-4" /> Chat
										</button>
									{/if}
								{/if}

								{#if catalogEntry}
									{@render editCatalogEntryAction(catalogEntry, matchingServers)}
									{@render renameCatalogEntryAction(catalogEntry, matchingServers)}
								{/if}

								{#if oauthServers.length > 0 && catalogEntry}
									<button
										class="menu-button hover:bg-base-400"
										onclick={async (e) => {
											e.stopPropagation();
											if (oauthServers.length === 1) {
												await reauthenticateServer(oauthServers[0]);
											} else {
												selectedConfiguredServers = oauthServers;
												selectedEntry = catalogEntry;
												selectServerMode = 'reauthenticate';
												selectServerDialog?.open();
											}
											toggle(false);
										}}
									>
										<KeyRound class="size-4" /> Reauthenticate
									</button>
								{/if}

								{#if matchingInstance && !isCatalogEntry && hasInstanceConfiguration(d.data as MCPCatalogServer)}
									<button
										class="menu-button hover:bg-base-400"
										onclick={(e) => {
											e.stopPropagation();
											connectToServerDialog?.open({
												server: d.data as MCPCatalogServer,
												instance: matchingInstance,
												configureInstance: true
											});
											toggle(false);
										}}
									>
										<ServerCog class="size-4" /> Edit Configuration
									</button>
								{/if}

								{#if matchingServers.length > 0 && catalogEntry}
									<button
										class="menu-button hover:bg-base-400"
										onclick={async (e) => {
											e.stopPropagation();
											if (matchingServers.length === 1) {
												goto(
													resolve(
														`/mcp-servers/c/${catalogEntry.id}/instance/${matchingServers[0].id}`
													)
												);
											} else {
												handleShowSelectServerDialog(catalogEntry, 'server-details');
											}
											toggle(false);
										}}
									>
										<ReceiptText class="size-4" /> Server Details
									</button>
								{/if}

								{#if matchingServers.length > 0 && catalogEntry}
									<button
										class="menu-button hover:bg-base-400"
										onclick={async (e) => {
											e.stopPropagation();
											if (matchingServers.length === 1) {
												await ChatService.restartMcpServer(matchingServers[0].id);
												mcpServersAndEntries.refreshUserConfiguredServers();
											} else {
												handleShowSelectServerDialog(catalogEntry, 'restart');
											}
											toggle(false);
										}}
									>
										<RefreshCw class="size-4" /> Restart Server
									</button>
								{/if}

								{#if matchingServers.length > 0 && catalogEntry}
									<button
										class="menu-button hover:bg-base-400"
										onclick={async (e) => {
											e.stopPropagation();

											if (matchingServers.length === 1) {
												await ChatService.deleteSingleOrRemoteMcpServer(matchingServers[0].id);
												mcpServersAndEntries.refreshUserConfiguredServers();
											} else {
												handleShowSelectServerDialog(catalogEntry, 'disconnect');
											}

											toggle(false);
										}}
									>
										<Unplug class="size-4" /> Disconnect
									</button>
								{:else if matchingInstance}
									<button
										class="menu-button hover:bg-base-400"
										onclick={async (e) => {
											e.stopPropagation();
											await ChatService.deleteMcpServerInstance(matchingInstance.id);
											mcpServersAndEntries.refreshUserInstances();
											toggle(false);
										}}
									>
										<Unplug class="size-4" /> Disconnect
									</button>
								{/if}
							</div>
						{/if}
						<div class="flex flex-col gap-1 p-2">
							{#if !hasConnectedOptions}
								{#if !requiresOAuth || catalogEntry?.oauthCredentialConfigured}
									{@render connectToServerAction(d.data, toggle, true)}
								{/if}
							{/if}
							{#if requiresOAuth && catalogEntry}
								<button
									class="menu-button hover:bg-base-400"
									onclick={async (e) => {
										e.stopPropagation();
										await handleConfigureOAuth(catalogEntry);
										toggle(false);
									}}
								>
									<Settings class="size-4" /> Configure OAuth
								</button>
							{/if}
							{#if auditLogUrl && (belongsToUser || profile.current?.hasAdminAccess?.())}
								<button
									onclick={(e) => {
										e.stopPropagation();
										const isCtrlClick = e.ctrlKey || e.metaKey;
										openUrl(auditLogUrl, isCtrlClick);
									}}
									class="menu-button"
								>
									<Captions class="size-4" /> View Audit Logs
								</button>
							{/if}
							{#if canDelete}
								<button
									class="menu-button-destructive"
									onclick={(e) => {
										e.stopPropagation();
										if (catalogEntry) {
											deletingEntry = catalogEntry;
										} else {
											deletingServer = d.data as MCPCatalogServer;
										}
										toggle(false);
									}}
								>
									<Trash2 class="size-4" /> Delete Entry
								</button>
							{/if}
						</div>
					{/snippet}
				</DotDotDot>
			{/snippet}
		</Table>
	{/if}
</div>

{#snippet editCatalogEntryAction(d: MCPCatalogEntry, configuredServers: MCPCatalogServer[])}
	{@const canConfigure = d.manifest.runtime === 'composite' || hasEditableConfiguration(d)}
	{@const requiresUpdate = configuredServers.some(requiresUserUpdate)}
	{#if canConfigure}
		<button
			class={twMerge(
				'menu-button hover:bg-base-400',
				requiresUpdate && 'bg-warning/10 text-warning hover:bg-warning/30'
			)}
			onclick={() => {
				if (configuredServers.length === 1) {
					editExistingDialog?.edit({
						server: configuredServers[0],
						entry: d
					});
				} else {
					handleShowSelectServerDialog(d, 'edit');
				}
			}}
		>
			<ServerCog class="size-4" /> Edit Configuration
		</button>
	{/if}
{/snippet}

{#snippet renameCatalogEntryAction(d: MCPCatalogEntry, configuredServers: MCPCatalogServer[])}
	<button
		class="menu-button hover:bg-base-400"
		onclick={() => {
			if (configuredServers.length === 1) {
				editExistingDialog?.rename({
					server: configuredServers[0],
					entry: d
				});
			} else {
				handleShowSelectServerDialog(d, 'rename');
			}
		}}
	>
		<PencilLine class="size-4" /> Rename
	</button>
{/snippet}

{#snippet connectToServerAction(
	d: MCPCatalogEntry | MCPCatalogServer,
	toggle: (value: boolean) => void,
	isCreateFirst?: boolean
)}
	{@const canConnect = d.canConnect !== false}
	<button
		class="menu-button disabled:cursor-not-allowed disabled:opacity-50"
		disabled={!canConnect}
		use:tooltip={{
			text: canConnect ? '' : 'See MCP Registries to grant connect access to this server',
			disablePortal: true
		}}
		onclick={async (e) => {
			e.stopPropagation();

			if ('isCatalogEntry' in d) {
				const matchingServers = getUsableConfiguredServersForCatalogEntry(d);
				if (isCreateFirst || matchingServers.length === 1) {
					connectToServerDialog?.open({
						entry: d,
						server: matchingServers[0]
					});
				} else {
					handleShowSelectServerDialog(d);
				}
			} else {
				const entry = d.catalogEntryID ? entriesMap.get(d.catalogEntryID) : undefined;
				const server = 'isCatalogEntry' in d ? undefined : d;
				connectToServerDialog?.open({
					entry,
					server,
					instance: instancesMap.get(d.id)
				});
			}
			toggle(false);
		}}
	>
		<SatelliteDish class="size-4" /> Connect To Server
	</button>
{/snippet}

<McpConfirmDelete
	names={[deletingEntry?.manifest?.name ?? '']}
	show={Boolean(deletingEntry)}
	onsuccess={async () => {
		if (!deletingEntry) {
			return;
		}

		if (deletingEntry.powerUserWorkspaceID) {
			await ChatService.deleteWorkspaceMCPCatalogEntry(
				deletingEntry.powerUserWorkspaceID,
				deletingEntry.id
			);
		} else if (catalog) {
			await AdminService.deleteMCPCatalogEntry(catalog.id, deletingEntry.id);
		}

		await fetch();
		deletingEntry = undefined;
	}}
	oncancel={() => (deletingEntry = undefined)}
	entity="entry"
	entityPlural="entries"
/>

<McpConfirmDelete
	names={[deletingServer?.alias || deletingServer?.manifest?.name || '']}
	show={Boolean(deletingServer)}
	onsuccess={async () => {
		if (!deletingServer) {
			return;
		}

		try {
			if (deletingServer.catalogEntryID) {
				await ChatService.deleteSingleOrRemoteMcpServer(deletingServer.id);
			} else if (deletingServer.powerUserWorkspaceID) {
				await ChatService.deleteWorkspaceMCPCatalogServer(
					deletingServer.powerUserWorkspaceID,
					deletingServer.id
				);
			} else if (catalog) {
				await AdminService.deleteMCPCatalogServer(catalog.id, deletingServer.id);
			}

			await fetch();
			deletingServer = undefined;
		} catch (error) {
			if (error instanceof MCPCompositeDeletionDependencyError) {
				deleteConflictError = error;
				return;
			}

			throw error;
		}
	}}
	oncancel={() => (deletingServer = undefined)}
	entity="entry"
	entityPlural="entries"
/>

<McpConfirmDelete
	names={Object.values(selected).map((s) => s.name)}
	show={confirmBulkDelete}
	onsuccess={async () => {
		loadingBulkDelete = true;
		try {
			for (const item of Object.values(selected)) {
				if ('isCatalogEntry' in item.data) {
					if (item.data.powerUserWorkspaceID) {
						await ChatService.deleteWorkspaceMCPCatalogEntry(
							item.data.powerUserWorkspaceID,
							item.data.id
						);
					} else if (catalog) {
						await AdminService.deleteMCPCatalogEntry(catalog.id, item.data.id);
					}
				} else if (!item.data.catalogEntryID) {
					try {
						if (item.data.powerUserWorkspaceID) {
							await ChatService.deleteWorkspaceMCPCatalogServer(
								item.data.powerUserWorkspaceID,
								item.data.id
							);
						} else if (catalog) {
							await AdminService.deleteMCPCatalogServer(catalog.id, item.data.id);
						}
					} catch (error) {
						if (error instanceof MCPCompositeDeletionDependencyError) {
							deleteConflictError = error;
							// Stop processing further deletes; user must resolve dependencies first.
							break;
						}

						throw error;
					}
				} else {
					await ChatService.deleteSingleOrRemoteMcpServer(item.data.id);
				}
			}

			await fetch();
		} finally {
			confirmBulkDelete = false;
			loadingBulkDelete = false;
		}
	}}
	oncancel={() => (confirmBulkDelete = false)}
	loading={loadingBulkDelete}
	entity="entry"
	entityPlural="entries"
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
	onConnect={handleConnectToServer}
/>

<ResponsiveDialog
	class="bg-base-200 dark:bg-base-100"
	bind:this={selectServerDialog}
	title="Select Your Server"
>
	<Table
		data={selectedConfiguredServers || []}
		fields={['name', 'created']}
		onClickRow={async (d) => {
			selectServerDialog?.close();
			switch (selectServerMode) {
				case 'chat': {
					connectToServerDialog?.handleSetupChat(d);
					break;
				}
				case 'server-details': {
					goto(resolve(`/mcp-servers/c/${d.catalogEntryID}/instance/${d.id}`));
					break;
				}
				case 'rename': {
					editExistingDialog?.rename({
						server: d,
						entry: d.catalogEntryID ? entriesMap.get(d.catalogEntryID) : undefined
					});
					break;
				}
				case 'edit': {
					editExistingDialog?.edit({
						server: d,
						entry: d.catalogEntryID ? entriesMap.get(d.catalogEntryID) : undefined
					});
					break;
				}
				case 'disconnect': {
					await ChatService.deleteSingleOrRemoteMcpServer(d.id);
					mcpServersAndEntries.refreshUserConfiguredServers();
					break;
				}
				case 'restart': {
					await ChatService.restartMcpServer(d.id);
					mcpServersAndEntries.refreshUserConfiguredServers();
					break;
				}
				case 'reauthenticate': {
					await reauthenticateServer(d);
					break;
				}
				default:
					connectToServerDialog?.open({
						entry: selectedEntry,
						server: d
					});
					break;
			}
		}}
		disablePortal
	>
		{#snippet onRenderColumn(property, d)}
			{#if property === 'name'}
				<div class="flex shrink-0 items-center gap-2">
					<div class="icon">
						{#if d.manifest.icon}
							<img src={d.manifest.icon} alt={d.manifest.name} class="size-6" />
						{:else}
							<Server class="size-6" />
						{/if}
					</div>
					<p class="flex items-center gap-2">
						{d.alias || d.manifest.name}
						{#if 'needsUpdate' in d && d.needsUpdate}
							<span
								use:tooltip={{
									classes: ['border-primary', 'bg-primary/10', 'dark:bg-primary/50'],
									text: 'An update requires your attention'
								}}
							>
								<CircleFadingArrowUp class="text-primary size-4" />
							</span>
						{/if}
					</p>
				</div>
			{:else if property === 'created'}
				{formatTimeAgo(d.created).relativeTime}
			{/if}
		{/snippet}
		{#snippet actions()}
			<IconButton class="hover:dark:bg-base-100/50">
				<StepForward class="size-4" />
			</IconButton>
		{/snippet}
	</Table>
</ResponsiveDialog>

<EditExistingDeployment
	bind:this={editExistingDialog}
	onUpdateConfigure={() => {
		mcpServersAndEntries.refreshUserConfiguredServers();
	}}
/>

<StaticOAuthConfigureModal
	bind:this={oauthConfigModal}
	{oauthStatus}
	onSave={handleSaveOAuth}
	onDelete={handleDeleteOAuth}
/>
