<script lang="ts">
	import { page } from '$app/state';
	import Confirm from '$lib/components/Confirm.svelte';
	import DotDotDot from '$lib/components/DotDotDot.svelte';
	import Table, { type InitSort, type InitSortFn } from '$lib/components/table/Table.svelte';
	import {
		fetchMcpServerAndEntries,
		getAdminMcpServerAndEntries
	} from '$lib/context/admin/mcpServerAndEntries.svelte';
	import {
		AdminService,
		ChatService,
		type MCPCatalog,
		type MCPCatalogEntry,
		type MCPCatalogServer,
		type OrgUser
	} from '$lib/services';
	import { convertEntriesAndServersToTableData } from '$lib/services/chat/mcp';
	import { formatTimeAgo } from '$lib/time';
	import { setSearchParamsToLocalStorage } from '$lib/url';
	import { openUrl } from '$lib/utils';
	import { Captions, Ellipsis, LoaderCircle, Server, Trash2 } from 'lucide-svelte';
	import type { Snippet } from 'svelte';

	type Item = ReturnType<typeof convertEntriesAndServersToTableData>[number];

	interface Props {
		catalog?: MCPCatalog;
		readonly?: boolean;
		emptyContentButton?: Snippet;
		usersMap?: Map<string, OrgUser>;
		query?: string;
		urlFilters?: Record<string, (string | number)[]>;
		onFilter?: (property: string, values: string[]) => void;
		onClearAllFilters?: () => void;
		onSort?: InitSortFn;
		initSort?: InitSort;
	}

	let {
		catalog = $bindable(),
		readonly,
		emptyContentButton,
		usersMap,
		query,
		urlFilters: filters,
		onFilter,
		onClearAllFilters,
		onSort,
		initSort
	}: Props = $props();

	let deletingEntry = $state<MCPCatalogEntry>();
	let deletingServer = $state<MCPCatalogServer>();
	let selected = $state<Record<string, Item>>({});
	let confirmBulkDelete = $state(false);
	let loadingBulkDelete = $state(false);

	const mcpServerAndEntries = getAdminMcpServerAndEntries();
	let tableData = $derived(
		convertEntriesAndServersToTableData(
			mcpServerAndEntries.entries,
			mcpServerAndEntries.servers,
			usersMap
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
		const isCatalogEntry = d.type === 'single' || d.type === 'remote' || d.type === 'composite';
		if (isCatalogEntry) {
			return d.data.powerUserWorkspaceID
				? `/admin/mcp-servers/w/${d.data.powerUserWorkspaceID}/c/${d.id}?view=audit-logs`
				: `/admin/mcp-servers/c/${d.id}?view=audit-logs`;
		}

		return d.data.powerUserWorkspaceID
			? `/admin/mcp-servers/w/${d.data.powerUserWorkspaceID}/s/${d.id}?view=audit-logs`
			: `/admin/mcp-servers/s/${d.id}?view=audit-logs`;
	}
</script>

<div class="flex flex-col gap-2">
	{#if mcpServerAndEntries.loading}
		<div class="my-2 flex items-center justify-center">
			<LoaderCircle class="size-6 animate-spin" />
		</div>
	{:else if mcpServerAndEntries.entries.length + mcpServerAndEntries.servers.length === 0}
		<div class="my-12 flex w-md flex-col items-center gap-4 self-center text-center">
			<Server class="dark:text-surface3 size-24 text-gray-200" />
			<h4 class="text-lg font-semibold text-gray-400 dark:text-gray-600">No created MCP servers</h4>
			<p class="text-sm font-light text-gray-400 dark:text-gray-600">
				Looks like you don't have any servers created yet. <br />
				Click the button below to get started.
			</p>

			{#if !readonly && emptyContentButton}
				{@render emptyContentButton()}
			{/if}
		</div>
	{:else}
		<Table
			data={filteredTableData}
			fields={['name', 'type', 'users', 'created', 'registry']}
			filterable={['name', 'type', 'registry']}
			{filters}
			onClickRow={(d, isCtrlClick) => {
				let url = '';
				if (d.type === 'single' || d.type === 'remote' || d.type === 'composite') {
					url = d.data.powerUserWorkspaceID
						? `/admin/mcp-servers/w/${d.data.powerUserWorkspaceID}/c/${d.id}`
						: `/admin/mcp-servers/c/${d.id}`;
				} else {
					url = d.data.powerUserWorkspaceID
						? `/admin/mcp-servers/w/${d.data.powerUserWorkspaceID}/s/${d.id}`
						: `/admin/mcp-servers/s/${d.id}`;
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
				thead: 'top-31'
			}}
			validateSelect={(d) => d.editable}
			disabledSelectMessage="This entry is managed by Git; changes cannot be made."
		>
			{#snippet onRenderColumn(property, d)}
				{#if property === 'name'}
					<div class="flex flex-shrink-0 items-center gap-2">
						<div
							class="bg-surface1 flex items-center justify-center rounded-sm p-0.5 dark:bg-gray-600"
						>
							{#if d.icon}
								<img src={d.icon} alt={d.name} class="size-6" />
							{:else}
								<Server class="size-6" />
							{/if}
						</div>
						<p class="flex items-center gap-1">
							{d.name}
						</p>
					</div>
				{:else if property === 'type'}
					{d.type === 'single'
						? 'Single User'
						: d.type === 'multi'
							? 'Multi-User'
							: d.type === 'remote'
								? 'Remote'
								: 'Composite'}
				{:else if property === 'created'}
					{formatTimeAgo(d.created).relativeTime}
				{:else}
					{d[property as keyof typeof d]}
				{/if}
			{/snippet}
			{#snippet actions(d)}
				{@const url = getAuditLogsUrl(d)}
				<DotDotDot class="icon-button hover:dark:bg-black/50">
					{#snippet icon()}
						<Ellipsis class="size-4" />
					{/snippet}

					<div class="default-dialog flex min-w-max flex-col gap-1 p-2">
						<button
							onclick={(e) => {
								e.stopPropagation();
								const isCtrlClick = e.ctrlKey || e.metaKey;
								setSearchParamsToLocalStorage(page.url.pathname, page.url.search);
								openUrl(url, isCtrlClick);
							}}
							class="menu-button"
						>
							<Captions class="size-4" /> View Audit Logs
						</button>
						{#if d.editable && !readonly}
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

<Confirm
	msg="Are you sure you want to delete this server?"
	show={Boolean(deletingEntry)}
	onsuccess={async () => {
		if (!deletingEntry || !catalog) {
			return;
		}

		if (deletingEntry.powerUserWorkspaceID) {
			await ChatService.deleteWorkspaceMCPCatalogEntry(
				deletingEntry.powerUserWorkspaceID,
				deletingEntry.id
			);
		} else {
			await AdminService.deleteMCPCatalogEntry(catalog.id, deletingEntry.id);
		}
		await fetchMcpServerAndEntries(catalog.id, mcpServerAndEntries);
		deletingEntry = undefined;
	}}
	oncancel={() => (deletingEntry = undefined)}
/>

<Confirm
	msg="Are you sure you want to delete this server?"
	show={Boolean(deletingServer)}
	onsuccess={async () => {
		if (!deletingServer || !catalog) {
			return;
		}

		if (deletingServer.powerUserWorkspaceID) {
			await ChatService.deleteWorkspaceMCPCatalogServer(
				deletingServer.powerUserWorkspaceID,
				deletingServer.id
			);
		} else {
			await AdminService.deleteMCPCatalogServer(catalog.id, deletingServer.id);
		}
		await fetchMcpServerAndEntries(catalog.id, mcpServerAndEntries);
		deletingServer = undefined;
	}}
	oncancel={() => (deletingServer = undefined)}
/>

<Confirm
	msg="Are you sure you want to delete these servers?"
	show={confirmBulkDelete}
	onsuccess={async () => {
		if (!catalog) return;
		loadingBulkDelete = true;
		for (const item of Object.values(selected)) {
			if (item.type === 'multi') {
				if (item.data.powerUserWorkspaceID) {
					await ChatService.deleteWorkspaceMCPCatalogServer(
						item.data.powerUserWorkspaceID,
						item.data.id
					);
				} else {
					await AdminService.deleteMCPCatalogServer(catalog.id, item.data.id);
				}
			} else {
				if (item.data.powerUserWorkspaceID) {
					await ChatService.deleteWorkspaceMCPCatalogEntry(
						item.data.powerUserWorkspaceID,
						item.data.id
					);
				} else {
					await AdminService.deleteMCPCatalogEntry(catalog.id, item.data.id);
				}
			}
		}
		await fetchMcpServerAndEntries(catalog.id, mcpServerAndEntries);
		confirmBulkDelete = false;
		loadingBulkDelete = false;
	}}
	oncancel={() => (confirmBulkDelete = false)}
	loading={loadingBulkDelete}
/>
