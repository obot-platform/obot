<script lang="ts">
	import { page } from '$app/state';
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import DotDotDot from '$lib/components/DotDotDot.svelte';
	import Search from '$lib/components/Search.svelte';
	import Skeleton from '$lib/components/Skeleton.svelte';
	import McpConfirmDelete from '$lib/components/mcp/McpConfirmDelete.svelte';
	import McpDeprecatedNotice from '$lib/components/mcp/McpDeprecatedNotice.svelte';
	import McpDetachedNotice from '$lib/components/mcp/McpDetachedNotice.svelte';
	import McpTunnelDisconnectedStatus from '$lib/components/mcp/McpTunnelDisconnectedStatus.svelte';
	import StaticOAuthConfigureModal from '$lib/components/mcp/StaticOAuthConfigureModal.svelte';
	import Table, { type InitSort, type InitSortFn } from '$lib/components/table/Table.svelte';
	import {
		AdminService,
		UserService,
		type MCPCatalog,
		type MCPCatalogEntry,
		type MCPCatalogServer,
		type OrgUser,
		type MCPServerOAuthCredentialStatus
	} from '$lib/services';
	import { OBOT_PLATFORM_REPO } from '$lib/services/admin/constants';
	import {
		convertEntriesToTableData,
		deleteMcpServerDeployment,
		isMultiUserCatalogEntry,
		getMCPDisplayName,
		hasEditableConfiguration,
		isDeprecatedMCPServer
	} from '$lib/services/user/mcp';
	import {
		getMcpTunnelConnectionsKey,
		isMcpTunnelDisconnected
	} from '$lib/services/user/mcpTunnel';
	import { mcpServersAndEntries, mcpTunnelConnections, profile } from '$lib/stores';
	import { formatTimeAgo } from '$lib/time';
	import { setUrlParamAndUpdateUrl } from '$lib/url';
	import { openUrl } from '$lib/utils';
	import {
		CircleFadingArrowUp,
		Ellipsis,
		GitBranch,
		Info,
		Server,
		Settings,
		Trash2,
		TriangleAlert
	} from '@lucide/svelte';
	import type { Snippet } from 'svelte';
	import { slide } from 'svelte/transition';

	type Item = ReturnType<typeof convertEntriesToTableData>[number];

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
	}

	let {
		entity,
		id,
		catalog = $bindable(),
		readonly,
		noDataContent,
		urlFilters: filters,
		onFilter,
		onClearAllFilters,
		onSort,
		initSort = { property: 'name', order: 'asc' },
		classes,
		usersMap
	}: Props = $props();

	let deletingEntry = $state<MCPCatalogEntry>();
	let deletingServer = $state<MCPCatalogServer>();

	let oauthConfigModal = $state<ReturnType<typeof StaticOAuthConfigureModal>>();
	let oauthConfigEntry = $state<MCPCatalogEntry>();
	let oauthStatus = $state<MCPServerOAuthCredentialStatus>();

	let query = $derived(page.url.searchParams.get('query') ?? '');

	let tableData = $derived(
		convertEntriesToTableData(
			mcpServersAndEntries.current.entries,
			usersMap,
			mcpServersAndEntries.current.userConfiguredServers,
			mcpServersAndEntries.current.servers
		).filter((d) => {
			const isOwnedByUser =
				profile.current.hasAdminAccess?.() ||
				(entity === 'workspace' && id && d.data.powerUserWorkspaceID === id);
			return isOwnedByUser;
		})
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
	let tunnelConnectionsKey = $derived(
		getMcpTunnelConnectionsKey(mcpTunnelConnections.current.connections)
	);

	let deploymentsNeedingAttentionByCatalogEntry = $derived(
		new Set<string>(
			mcpServersAndEntries.current.servers
				.filter((s) => s.catalogEntryID && (s.needsUpdate || s.needsK8sUpdate))
				?.map((s) => s.catalogEntryID)
		)
	);

	function getEntryUrl(d: Item) {
		const params: Record<string, string> = {};
		if (profile.current.hasAdminAccess?.() && d.data.powerUserWorkspaceID) {
			params.wid = d.data.powerUserWorkspaceID;
		}
		const query = Object.entries(params)
			.map(([key, value]) => `${key}=${encodeURIComponent(value)}`)
			.join('&');
		return `/mcp-servers/c/${d.data.id}${query ? `?${query}` : ''}`;
	}

	async function fetch() {
		await mcpServersAndEntries.refreshAll();
	}

	async function deleteServerDeployment(server: MCPCatalogServer) {
		await deleteMcpServerDeployment(server, catalog?.id);
	}

	async function handleConfigureOAuth(entry: MCPCatalogEntry) {
		oauthConfigEntry = entry;
		try {
			const catalogId = entry.powerUserWorkspaceID ? undefined : 'default';
			oauthStatus = entry.powerUserWorkspaceID
				? await UserService.getWorkspaceMCPCatalogEntryOAuthCredentials(
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
			await UserService.setWorkspaceMCPCatalogEntryOAuthCredentials(
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
			await UserService.deleteWorkspaceMCPCatalogEntryOAuthCredentials(
				oauthConfigEntry.powerUserWorkspaceID,
				oauthConfigEntry.id
			);
		} else {
			await AdminService.deleteMCPCatalogEntryOAuthCredentials('default', oauthConfigEntry.id);
		}
		// Refresh the table to update status
		mcpServersAndEntries.refreshAll();
	}

	const updateSearchQuery = (value: string) => {
		setUrlParamAndUpdateUrl(page.url, 'query', value);
	};
</script>

<div class="flex h-full w-full gap-2 flex-col">
	{#if catalog?.isSyncing}
		<div class="notification-info p-3 text-sm font-light" transition:slide={{ axis: 'y' }}>
			<div class="flex items-center gap-3">
				<Info class="size-6" />
				<div>The system is currently syncing with your configured Git repositories.</div>
			</div>
		</div>
	{/if}

	{#if mcpServersAndEntries.current.loading && tableData.length === 0}
		<Skeleton
			type="table"
			count={10}
			classes={{ header: 'h-14 rounded-none', body: 'rounded-none' }}
		/>
	{/if}
	{#if mcpServersAndEntries.current.isInitialized}
		<div class="bg-base-200 dark:bg-base-100 sticky top-16 left-0 z-20 w-full py-1">
			<Search
				value={query}
				class="dark:bg-base-200 dark:border-base-400 bg-base-100 border border-transparent shadow-sm"
				onChange={updateSearchQuery}
				placeholder="Search MCP servers..."
			/>
		</div>

		{#if filteredTableData.length === 0 && !query}
			{#if noDataContent}
				<div class="flex flex-col gap-px">
					{@render noDataContent?.()}
				</div>
			{/if}
		{:else if filteredTableData.length === 0 && query}
			<div class="flex flex-col gap-px">
				<div class="text-sm text-muted-content">No results found for "{query}".</div>
			</div>
		{:else}
			<Table
				data={filteredTableData}
				remeasureKey={tunnelConnectionsKey}
				fields={profile.current.hasAdminAccess?.()
					? ['name', 'type', 'users', 'created', 'source']
					: ['name', 'created']}
				filterable={['name', 'type', 'source']}
				{filters}
				onClickRow={(d, isCtrlClick) => {
					openUrl(getEntryUrl(d), isCtrlClick);
				}}
				{initSort}
				{onFilter}
				{onClearAllFilters}
				{onSort}
				sortable={['name', 'type', 'users', 'created', 'source']}
				noDataMessage="No catalog servers added."
				classes={{
					root: 'rounded-none rounded-b-md shadow-none',
					thead: classes?.tableHeader
				}}
				setRowClasses={(d) => {
					const missingSecretBinding = 'missingKubernetesSecret' in d && d.missingKubernetesSecret;
					return (d.data.needsUpdate && !missingSecretBinding) ||
						deploymentsNeedingAttentionByCatalogEntry.has(d.data.id)
						? 'bg-primary/10'
						: '';
				}}
			>
				{#snippet onRenderColumn(property, d)}
					{@const attentionRequired =
						(d.data.needsUpdate &&
							!('missingKubernetesSecret' in d && d.missingKubernetesSecret)) ||
						deploymentsNeedingAttentionByCatalogEntry.has(d.data.id)}
					{@const deprecated = isDeprecatedMCPServer(d.data)}
					{@const tunnelDisconnected = isMcpTunnelDisconnected(
						d.data,
						mcpTunnelConnections.current.connections
					)}
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
								{#if tunnelDisconnected}
									<McpTunnelDisconnectedStatus />
								{/if}
								{#if attentionRequired}
									<span
										use:tooltip={{
											classes: ['border-primary', 'bg-primary/10', 'dark:bg-primary/50'],
											text: deploymentsNeedingAttentionByCatalogEntry.has(d.data.id)
												? 'One or multiple deployments require your attention'
												: 'Configuration requires your attention'
										}}
									>
										<CircleFadingArrowUp class="text-primary size-4" />
									</span>
								{:else if 'missingKubernetesSecret' in d && d.missingKubernetesSecret}
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
								{#if d.status.toLowerCase() === 'deployed'}
									<span class="badge badge-xs badge-secondary">Deployed</span>
								{/if}
								{#if entity === 'catalog'}
									<McpDetachedNotice
										detached={d.data.detached}
										sourceURL={'sourceURL' in d.data ? d.data.sourceURL : undefined}
									/>
								{/if}
								<McpDeprecatedNotice {deprecated} />
							</p>
						</div>
					{:else if property === 'type'}
						{d.type}
						{#if !isMultiUserCatalogEntry(d.data) && hasEditableConfiguration(d.data)}
							<div class="p-2" use:tooltip={{ text: 'Requires user configuration' }}>
								<Settings class="size-3 text-muted-content" />
							</div>
						{/if}
					{:else if property === 'created'}
						{formatTimeAgo(d.created).relativeTime}
					{:else if property === 'source'}
						{#if d.sourceType === 'git'}
							<a
								onclick={(e) => e.stopPropagation()}
								href={d.source}
								target="_blank"
								rel="external noopener noreferrer"
								use:tooltip={{
									text: 'View Source on Git'
								}}
								class="link link-hover flex items-center gap-1 shrink-0 hover:text-blue-500"
							>
								<GitBranch class="size-4" />
								<span class="font-light">
									{#if d.source.startsWith(OBOT_PLATFORM_REPO)}
										Obot Catalog
									{:else}
										{d.source?.split('/').pop()}
									{/if}
								</span>
							</a>
						{:else}
							{d.source}
						{/if}
					{:else}
						{d[property as keyof typeof d]}
					{/if}
				{/snippet}
				{#snippet actions(d)}
					{@const isCatalogEntry = 'isCatalogEntry' in d.data}
					{@const catalogEntry = isCatalogEntry ? (d.data as MCPCatalogEntry) : undefined}
					{@const belongsToUser =
						entity === 'workspace' && id && d.data.powerUserWorkspaceID === id}
					{@const canDelete =
						d.editable && !readonly && (belongsToUser || profile.current?.hasAdminAccess?.())}
					{@const requiresOAuth =
						catalogEntry?.manifest?.runtime === 'remote' &&
						catalogEntry.manifest?.remoteConfig?.staticOAuthRequired}
					{@const canConfigureOAuth = Boolean(requiresOAuth && catalogEntry && !readonly)}
					{#if canConfigureOAuth || canDelete}
						<DotDotDot class="hover:dark:bg-base-100/50" classes={{ menu: 'p-0' }}>
							{#snippet icon()}
								<Ellipsis class="size-4" />
							{/snippet}

							{#snippet children({ toggle })}
								<div class="flex flex-col gap-1 p-2">
									{#if requiresOAuth && catalogEntry && !readonly}
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
									{#if canDelete}
										<button
											class="menu-button-destructive"
											onclick={(e) => {
												e.stopPropagation();
												deletingEntry = catalogEntry;
												toggle(false);
											}}
										>
											<Trash2 class="size-4" />
											{catalogEntry ? 'Delete Entry' : 'Delete Server'}
										</button>
									{/if}
								</div>
							{/snippet}
						</DotDotDot>
					{/if}
				{/snippet}
			</Table>
		{/if}
	{/if}
</div>

<McpConfirmDelete
	names={[deletingEntry?.manifest?.name ?? '']}
	show={Boolean(deletingEntry)}
	onsuccess={async () => {
		if (!deletingEntry) {
			return;
		}

		if (deletingEntry.powerUserWorkspaceID) {
			await UserService.deleteWorkspaceMCPCatalogEntry(
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
	names={[getMCPDisplayName(deletingServer)]}
	show={Boolean(deletingServer)}
	onsuccess={async () => {
		if (!deletingServer) {
			return;
		}

		await deleteServerDeployment(deletingServer);

		await fetch();
		deletingServer = undefined;
	}}
	oncancel={() => (deletingServer = undefined)}
	entity="server"
	entityPlural="servers"
/>

<StaticOAuthConfigureModal
	bind:this={oauthConfigModal}
	{oauthStatus}
	deprecated={isDeprecatedMCPServer(oauthConfigEntry)}
	onSave={handleSaveOAuth}
	onDelete={handleDeleteOAuth}
/>
