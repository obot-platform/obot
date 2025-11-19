<script lang="ts">
	import { page } from '$app/state';
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import DotDotDot from '$lib/components/DotDotDot.svelte';
	import McpConfirmDelete from '$lib/components/mcp/McpConfirmDelete.svelte';
	import McpMultiDeleteBlockedDialog from '$lib/components/mcp/McpMultiDeleteBlockedDialog.svelte';
	import Table, { type InitSort, type InitSortFn } from '$lib/components/table/Table.svelte';
	import {
		fetchMcpServerAndEntries,
		getAdminMcpServerAndEntries
	} from '$lib/context/admin/mcpServerAndEntries.svelte';
	import {
		fetchUserMcpServerAndEntries,
		getUserMcpServerAndEntries
	} from '$lib/context/mcpServerAndEntries.svelte';
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
	import {
		convertEntriesAndServersToTableData,
		getServerTypeLabelByType
	} from '$lib/services/chat/mcp';
	import { profile } from '$lib/stores';
	import { formatTimeAgo } from '$lib/time';
	import { setSearchParamsToLocalStorage } from '$lib/url';
	import { openUrl } from '$lib/utils';
	import {
		AlertTriangle,
		Captions,
		CircleFadingArrowUp,
		Ellipsis,
		LoaderCircle,
		MessageCircle,
		SatelliteDish,
		Server,
		Trash2
	} from 'lucide-svelte';
	import type { Snippet } from 'svelte';
	import { slide } from 'svelte/transition';
	import ConnectToServer from '$lib/components/mcp/ConnectToServer.svelte';

	type Item = ReturnType<typeof convertEntriesAndServersToTableData>[number];

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
		entity = 'catalog',
		catalog = $bindable(),
		readonly,
		noDataContent,
		usersMap,
		query,
		urlFilters: filters,
		onFilter,
		onClearAllFilters,
		onSort,
		initSort,
		classes
	}: Props = $props();

	let deletingEntry = $state<MCPCatalogEntry>();
	let deletingServer = $state<MCPCatalogServer>();
	let selected = $state<Record<string, Item>>({});
	let confirmBulkDelete = $state(false);
	let loadingBulkDelete = $state(false);
	let deleteConflictError = $state<MCPCompositeDeletionDependencyError | undefined>();

	let connectToServerDialog = $state<ReturnType<typeof ConnectToServer>>();

	const mcpServerAndEntries =
		entity === 'workspace' ? getUserMcpServerAndEntries() : getAdminMcpServerAndEntries();
	let tableData = $derived(
		convertEntriesAndServersToTableData(
			mcpServerAndEntries.entries,
			mcpServerAndEntries.servers,
			usersMap,
			mcpServerAndEntries.userConfiguredServers
		)
	);

	let instances = $derived(mcpServerAndEntries.userServerInstances);
	let instancesMap = $derived(
		new Map(instances.map((instance) => [instance.mcpServerID, instance]))
	);

	let entriesMap = $derived(new Map(mcpServerAndEntries.entries.map((entry) => [entry.id, entry])));

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
		let hasAuditLogUrlsAccess = profile.current.groups.includes(Group.POWERUSER);

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
		if (catalog) {
			await fetchMcpServerAndEntries(catalog.id, mcpServerAndEntries);
		} else {
			await fetchUserMcpServerAndEntries(mcpServerAndEntries);
		}
	}
</script>

<div class="flex flex-col gap-2">
	{#if mcpServerAndEntries.loading}
		<div class="my-2 flex items-center justify-center">
			<LoaderCircle class="size-6 animate-spin" />
		</div>
	{:else if mcpServerAndEntries.entries.length + mcpServerAndEntries.servers.length === 0}
		{#if noDataContent}
			{@render noDataContent?.()}
		{/if}
	{:else}
		{@const hasCatalogErrors = catalog && Object.keys(catalog?.syncErrors ?? {}).length > 0}
		{#if hasCatalogErrors && !catalog?.isSyncing}
			<div class="w-full p-4" in:slide={{ axis: 'y' }} out:slide={{ axis: 'y', duration: 0 }}>
				<div class="notification-alert flex w-full items-center gap-2 rounded-md p-3 text-sm">
					<AlertTriangle class="size-" />
					<p class="">Some servers failed to sync. See "Registry Sources" tab for more details.</p>
				</div>
			</div>
		{/if}

		<Table
			data={filteredTableData}
			fields={['name', 'type', 'users', 'created', 'registry']}
			filterable={['name', 'type', 'registry']}
			{filters}
			onClickRow={(d, isCtrlClick) => {
				let url = '';
				let useAdminUrl =
					window.location.pathname.includes('/admin') && profile.current.hasAdminAccess?.();

				const id = 'catalogEntryID' in d.data ? d.data.catalogEntryID : d.id;
				if (useAdminUrl) {
					if (d.type === 'single' || d.type === 'remote' || d.type === 'composite') {
						url = d.data.powerUserWorkspaceID
							? `/admin/mcp-servers/w/${d.data.powerUserWorkspaceID}/c/${id}`
							: `/admin/mcp-servers/c/${id}`;
					} else {
						url = d.data.powerUserWorkspaceID
							? `/admin/mcp-servers/w/${d.data.powerUserWorkspaceID}/s/${id}`
							: `/admin/mcp-servers/s/${id}`;
					}
				} else {
					url =
						d.type === 'single' || d.type === 'remote' || d.type === 'composite'
							? `/mcp-servers/c/${id}`
							: `/mcp-servers/s/${id}`;
				}

				setSearchParamsToLocalStorage(page.url.pathname, page.url.search);
				openUrl(url, isCtrlClick);
			}}
			{initSort}
			{onFilter}
			{onClearAllFilters}
			{onSort}
			sortable={['name', 'type', 'users', 'created', 'registry']}
			noDataMessage="No catalog servers added."
			classes={{
				root: 'rounded-none rounded-b-md shadow-none',
				thead: classes?.tableHeader
			}}
			validateSelect={(d) => d.editable}
			disabledSelectMessage="This entry is managed by Git; changes cannot be made."
			setRowClasses={(d) => ('needsUpdate' in d && d.needsUpdate ? 'bg-primary/10' : '')}
		>
			{#snippet onRenderColumn(property, d)}
				{#if property === 'name'}
					<div class="flex flex-shrink-0 items-center gap-2">
						<div class="icon">
							{#if d.icon}
								<img src={d.icon} alt={d.name} class="size-6" />
							{:else}
								<Server class="size-6" />
							{/if}
						</div>
						<p class="flex items-center gap-2">
							{d.name}
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
				{:else if property === 'type'}
					{getServerTypeLabelByType(d.type)}
				{:else if property === 'created'}
					{formatTimeAgo(d.created).relativeTime}
				{:else}
					{d[property as keyof typeof d]}
				{/if}
			{/snippet}
			{#snippet actions(d)}
				{@const auditLogUrl = getAuditLogsUrl(d)}
				{@const canDelete = d.editable && !readonly}
				<DotDotDot class="icon-button hover:dark:bg-background/50">
					{#snippet icon()}
						<Ellipsis class="size-4" />
					{/snippet}

					{#snippet children({ toggle })}
						<div class="default-dialog flex min-w-max flex-col gap-1 p-2">
							<button
								class="menu-button-primary"
								onclick={async (e) => {
									e.stopPropagation();
									const entry =
										'catalogEntryID' in d.data
											? entriesMap.get(d.data.catalogEntryID as string)
											: 'isCatalogEntry' in d.data
												? d.data
												: undefined;
									const server = 'isCatalogEntry' in d.data ? undefined : d.data;
									const instance =
										'isCatalogEntry' in d.data
											? undefined
											: d.data.catalogEntryID
												? undefined
												: instancesMap.get(d.id);
									connectToServerDialog?.open({
										entry,
										server,
										instance
									});
									toggle(false);
								}}
							>
								<SatelliteDish class="size-4" /> Connect
							</button>
							{#if !('isCatalogEntry' in d.data)}
								<button
									class="menu-button"
									onclick={async (e) => {
										e.stopPropagation();
										const serverToUse = 'isCatalogEntry' in d.data ? undefined : d.data;
										if (serverToUse) {
											connectToServerDialog?.handleSetupChat(serverToUse, instancesMap.get(d.id));
										}
										toggle(false);
									}}
								>
									<MessageCircle class="size-4" /> Chat
								</button>
							{/if}

							{#if auditLogUrl}
								<button
									onclick={(e) => {
										e.stopPropagation();
										const isCtrlClick = e.ctrlKey || e.metaKey;
										setSearchParamsToLocalStorage(page.url.pathname, page.url.search);
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
										if (d.data.type === 'mcpserver') {
											deletingServer = d.data as MCPCatalogServer;
										} else {
											deletingEntry = d.data as MCPCatalogEntry;
										}
									}}
								>
									<Trash2 class="size-4" /> Delete Server
								</button>
							{/if}
						</div>
					{/snippet}
				</DotDotDot>
			{/snippet}
			{#snippet tableSelectActions(currentSelected)}
				<div class="flex grow items-center justify-end gap-2 px-4 py-2">
					<button
						class="button flex items-center gap-1 text-sm font-normal"
						onclick={() => {
							selected = currentSelected;
							confirmBulkDelete = true;
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
	names={[deletingServer?.manifest?.name ?? '']}
	show={Boolean(deletingServer)}
	onsuccess={async () => {
		if (!deletingServer || !catalog) {
			return;
		}

		try {
			if (deletingServer.powerUserWorkspaceID) {
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
				if (item.type === 'multi') {
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
					if (item.data.powerUserWorkspaceID) {
						await ChatService.deleteWorkspaceMCPCatalogEntry(
							item.data.powerUserWorkspaceID,
							item.data.id
						);
					} else if (catalog) {
						await AdminService.deleteMCPCatalogEntry(catalog.id, item.data.id);
					}
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
	userConfiguredServers={mcpServerAndEntries.userConfiguredServers}
/>
