<script lang="ts">
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import Confirm from '$lib/components/Confirm.svelte';
	import DotDotDot from '$lib/components/DotDotDot.svelte';
	import ConnectToServer from '$lib/components/mcp/ConnectToServer.svelte';
	import McpConfirmDelete from '$lib/components/mcp/McpConfirmDelete.svelte';
	import McpMultiDeleteBlockedDialog from '$lib/components/mcp/McpMultiDeleteBlockedDialog.svelte';
	import StaticOAuthConfigureModal from '$lib/components/mcp/StaticOAuthConfigureModal.svelte';
	import Table, { type InitSort, type InitSortFn } from '$lib/components/table/Table.svelte';
	import Loading from '$lib/icons/Loading.svelte';
	import {
		AdminService,
		UserService,
		type MCPCatalog,
		type MCPCatalogEntry,
		type MCPCatalogServer,
		type OrgUser,
		MCPCompositeDeletionDependencyError,
		Group,
		type MCPServerInstance,
		type MCPServerOAuthCredentialStatus
	} from '$lib/services';
	import {
		convertEntriesToTableData,
		deleteMcpServerDeployment,
		isMultiUserCatalogEntry,
		isMultiUserServer,
		getMCPDisplayName
	} from '$lib/services/user/mcp';
	import { mcpServersAndEntries, profile } from '$lib/stores';
	import { formatTimeAgo } from '$lib/time';
	import { openUrl } from '$lib/utils';
	import {
		Captions,
		CircleFadingArrowUp,
		Ellipsis,
		GitBranch,
		Server,
		Settings,
		Trash2,
		TriangleAlert
	} from 'lucide-svelte';
	import type { Snippet } from 'svelte';

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
		initSort = { property: 'name', order: 'asc' },
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

	let oauthConfigModal = $state<ReturnType<typeof StaticOAuthConfigureModal>>();
	let oauthConfigEntry = $state<MCPCatalogEntry>();
	let oauthStatus = $state<MCPServerOAuthCredentialStatus>();

	let confirmNewDeployForMultiUserEntry = $state<{
		entry: MCPCatalogEntry;
		servers: MCPCatalogServer[];
	}>();

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

	function getAuditLogsUrl(d: Item) {
		let useAdminUrl =
			window.location.pathname.includes('/admin') && profile.current.hasAdminAccess?.();

		if (!profile.current.groups.includes(Group.POWERUSER)) {
			return null;
		}

		if (useAdminUrl) {
			return d.data.powerUserWorkspaceID
				? `/admin/mcp-catalog/w/${d.data.powerUserWorkspaceID}/c/${d.id}?view=audit-logs`
				: `/admin/mcp-catalog/c/${d.id}?view=audit-logs`;
		}

		return `/mcp-catalog/c/${d.id}?view=audit-logs`;
	}

	async function fetch() {
		await mcpServersAndEntries.refreshAll();
	}

	async function deleteServerDeployment(server: MCPCatalogServer) {
		await deleteMcpServerDeployment(server, catalog?.id);
	}

	function handleConnectToServer({
		server,
		instance
	}: {
		server?: MCPCatalogServer;
		instance?: MCPServerInstance;
	}) {
		if (instance || server) {
			mcpServersAndEntries.refreshAll();
		}
		onConnect?.({ instance });
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
</script>

<div class="flex flex-col gap-2">
	{#if mcpServersAndEntries.current.loading}
		<div class="my-2 flex items-center justify-center h-72">
			<Loading class="size-6" />
		</div>
	{:else if filteredTableData.length === 0}
		{#if noDataContent}
			{@render noDataContent?.()}
		{/if}
	{:else}
		<Table
			data={filteredTableData}
			fields={profile.current.hasAdminAccess?.()
				? ['name', 'type', 'users', 'created', 'source']
				: ['name', 'created']}
			filterable={['name', 'type', 'source']}
			{filters}
			onClickRow={(d, isCtrlClick) => {
				const prefix = profile.current.hasAdminAccess?.() ? '/admin' : '';
				let url = `${prefix}/mcp-catalog/c/${d.data.id}`;

				if (profile.current.hasAdminAccess?.() && d.data.powerUserWorkspaceID) {
					url += '?wid=' + encodeURIComponent(d.data.powerUserWorkspaceID);
				}

				openUrl(url, isCtrlClick);
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
				return d.data.needsUpdate && !missingSecretBinding ? 'bg-primary/10' : '';
			}}
		>
			{#snippet onRenderColumn(property, d)}
				{@const isCatalogEntry = 'isCatalogEntry' in d.data}
				{@const catalogEntry = isCatalogEntry ? (d.data as MCPCatalogEntry) : undefined}
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
						</p>
					</div>
				{:else if property === 'type'}
					{d.type}
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
							class="btn btn-ghost hover:text-blue-500 btn-xs shrink-0"
						>
							<GitBranch class="size-4" />
							{d.source?.split('/').pop()}
						</a>
					{:else}
						<p class="px-2 text-xs">{d.source}</p>
					{/if}
				{:else}
					{d[property as keyof typeof d]}
				{/if}
			{/snippet}
			{#snippet actions(d)}
				{@const isCatalogEntry = 'isCatalogEntry' in d.data}
				{@const catalogEntry = isCatalogEntry ? (d.data as MCPCatalogEntry) : undefined}
				{@const auditLogUrl = getAuditLogsUrl(d)}
				{@const belongsToUser = entity === 'workspace' && id && d.data.powerUserWorkspaceID === id}
				{@const canDelete =
					d.editable && !readonly && (belongsToUser || profile.current?.hasAdminAccess?.())}
				{@const requiresOAuth =
					catalogEntry?.manifest?.runtime === 'remote' &&
					catalogEntry.manifest?.remoteConfig?.staticOAuthRequired}
				{#if catalogEntry && isMultiUserCatalogEntry(catalogEntry) && ((!!catalog && profile.current?.hasAdminAccess?.()) || (entity === 'workspace' && !!id))}
					<button
						class="btn btn-xs btn-primary self-center mr-2"
						onclick={(e) => {
							e.stopPropagation();

							const deployedServers = mcpServersAndEntries.current.servers.filter(
								(s) => !s.deleted && isMultiUserServer(s) && s.catalogEntryID === catalogEntry.id
							);
							if (deployedServers.length >= 1) {
								confirmNewDeployForMultiUserEntry = {
									entry: catalogEntry,
									servers: deployedServers
								};
							} else {
								connectToServerDialog?.open({ entry: catalogEntry });
							}
						}}
					>
						Launch
					</button>
				{/if}
				<DotDotDot class="hover:dark:bg-base-100/50" classes={{ menu: 'p-0' }}>
					{#snippet icon()}
						<Ellipsis class="size-4" />
					{/snippet}

					{#snippet children({ toggle })}
						<div class="flex flex-col gap-1 p-2">
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
										deletingEntry = catalogEntry;
										toggle(false);
									}}
								>
									<Trash2 class="size-4" />
									Delete Entry
								</button>
							{/if}
						</div>
					{/snippet}
				</DotDotDot>
			{/snippet}
		</Table>
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

		try {
			await deleteServerDeployment(deletingServer);

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
	entity="server"
	entityPlural="servers"
/>

<McpConfirmDelete
	names={Object.values(selected).map((s) => s.name)}
	show={confirmBulkDelete}
	onsuccess={async () => {
		loadingBulkDelete = true;
		try {
			for (const item of Object.values(selected)) {
				if (item.data.powerUserWorkspaceID) {
					await UserService.deleteWorkspaceMCPCatalogEntry(
						item.data.powerUserWorkspaceID,
						item.data.id
					);
				} else if (catalog) {
					await AdminService.deleteMCPCatalogEntry(catalog.id, item.data.id);
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
	catalogID={catalog?.id}
	workspaceID={entity === 'workspace' ? id : undefined}
	onConnect={handleConnectToServer}
/>

<StaticOAuthConfigureModal
	bind:this={oauthConfigModal}
	{oauthStatus}
	onSave={handleSaveOAuth}
	onDelete={handleDeleteOAuth}
/>

<Confirm
	show={Boolean(confirmNewDeployForMultiUserEntry)}
	title="Launch New Server?"
	msg=""
	type="info"
	onsuccess={() => {
		if (!confirmNewDeployForMultiUserEntry?.entry) {
			console.error('Entry required to launch a new server');
			return;
		}
		connectToServerDialog?.open({ entry: confirmNewDeployForMultiUserEntry.entry });
		confirmNewDeployForMultiUserEntry = undefined;
	}}
	oncancel={() => (confirmNewDeployForMultiUserEntry = undefined)}
	submitText="Launch New Server"
>
	{#snippet msgContent()}{/snippet}
	{#snippet note()}
		<p>
			There {confirmNewDeployForMultiUserEntry?.servers.length === 1
				? 'is an existing deployment'
				: `are ${confirmNewDeployForMultiUserEntry?.servers.length} existing deployments`}
			of this catalog entry:
		</p>

		{#if confirmNewDeployForMultiUserEntry?.servers && confirmNewDeployForMultiUserEntry?.servers.length > 0}
			<ul class="my-4">
				{#each confirmNewDeployForMultiUserEntry?.servers as server (server.id)}
					<li>{getMCPDisplayName(server)}</li>
				{/each}
			</ul>
		{/if}

		<p>Would you like to launch a new server?</p>
	{/snippet}
</Confirm>
