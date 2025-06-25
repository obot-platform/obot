<script lang="ts">
	import { PAGE_TRANSITION_DURATION } from '$lib/constants';
	import { AdminService, type MCPCatalogServer } from '$lib/services';
	import {
		Role,
		type MCPCatalog,
		type MCPCatalogEntry,
		type OrgUser
	} from '$lib/services/admin/types';
	import {
		ChevronLeft,
		Container,
		Eye,
		LoaderCircle,
		Plus,
		RefreshCcw,
		Trash2,
		User,
		Users,
		X
	} from 'lucide-svelte';
	import { onMount, type Snippet } from 'svelte';
	import { fly, slide } from 'svelte/transition';
	import DotDotDot from '../DotDotDot.svelte';
	import Search from '../Search.svelte';
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import Table from '../Table.svelte';
	import { clickOutside } from '$lib/actions/clickoutside';
	import SearchUsers from './SearchUsers.svelte';
	import Confirm from '../Confirm.svelte';
	import { goto } from '$app/navigation';
	import CatalogServerForm from './CatalogServerForm.svelte';
	import ResponsiveDialog from '../ResponsiveDialog.svelte';

	interface Props {
		topContent?: Snippet;
		mcpCatalog?: MCPCatalog;
		onCreate?: (catalog: MCPCatalog) => void;
	}

	let { topContent, mcpCatalog: initialMcpCatalog, onCreate }: Props = $props();
	const duration = PAGE_TRANSITION_DURATION;
	let refreshing = $state(false);
	let mcpCatalog = $state(
		initialMcpCatalog ??
			({
				displayName: '',
				sourceURLs: [],
				allowedUserIDs: [],
				id: ''
			} satisfies MCPCatalog)
	);

	let saving = $state<boolean | undefined>();
	let loadingEntries = $state<Promise<MCPCatalogEntry[]>>();
	let loadingServers = $state<Promise<MCPCatalogServer[]>>();
	let loadingUsers = $state<Promise<OrgUser[]>>();

	let editingSource = $state<{ index: number; value: string }>();
	let sourceDialog = $state<HTMLDialogElement>();
	let selectServerTypeDialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let selectedServerType = $state<'single' | 'multi' | 'remote'>();
	let showServerForm = $state(false);
	let selectedEntryServer = $state<MCPCatalogEntry | MCPCatalogServer>();
	let searchEntries = $state('');
	let addUserGroupDialog = $state<ReturnType<typeof SearchUsers>>();

	let deletingUserGroup = $state<{ id: string; email: string }>();
	let deletingSource = $state<string>();
	let deletingCatalog = $state(false);
	let deletingEntry = $state<MCPCatalogEntry>();
	let deletingServer = $state<MCPCatalogServer>();

	onMount(async () => {
		if (mcpCatalog?.id) {
			loadingEntries = AdminService.listMCPCatalogEntries(mcpCatalog.id);
			loadingServers = AdminService.listMCPCatalogServers(mcpCatalog.id);
		}

		loadingUsers = AdminService.listUsers();
	});

	function closeSourceDialog() {
		editingSource = undefined;
		sourceDialog?.close();
	}

	function convertEntriesToTableData(entries: MCPCatalogEntry[] | undefined) {
		if (!entries) {
			return [];
		}

		return entries.map((entry) => {
			return {
				id: entry.id,
				source: entry.sourceURL || 'manual',
				name: entry.commandManifest?.name ?? entry.urlManifest?.name ?? '',
				data: entry,
				editable: !entry.sourceURL,
				type: 'Single-user',
				deployments: 0
			};
		});
	}

	function convertServersToTableData(servers: MCPCatalogServer[] | undefined) {
		if (!servers) {
			return [];
		}

		return servers
			.filter((server) => !server.catalogEntryID)
			.map((server) => {
				return {
					id: server.id,
					name: server.manifest.name ?? '',
					source: 'manual',
					type: server.manifest.fixedURL ? 'Remote' : 'Multi-user',
					data: server,
					editable: true,
					deployments: 0
				};
			});
	}

	function convertEntriesAndServersToTableData(
		entries: MCPCatalogEntry[],
		servers: MCPCatalogServer[]
	) {
		const entriesTableData = convertEntriesToTableData(entries);
		const serversTableData = convertServersToTableData(servers);
		return [...entriesTableData, ...serversTableData];
	}

	function convertUsersToTableData(userIds: string[], users: OrgUser[]) {
		const userMap = new Map(users?.map((user) => [user.id, user]));
		return (
			userIds
				.map((id) => {
					if (id === '*') {
						return {
							id: '*',
							username: 'everyone',
							email: 'Everyone',
							role: 'User',
							iconURL: '',
							created: new Date().toISOString(),
							explicitAdmin: false,
							type: 'Group'
						};
					}

					const user = userMap.get(id);
					if (!user) {
						return undefined;
					}

					return {
						...user,
						role: user.role === Role.ADMIN ? 'Admin' : 'User',
						type: 'User'
					};
				})
				.filter((user) => user !== undefined) ?? []
		);
	}

	function validate(catalog: typeof mcpCatalog) {
		if (!catalog) return false;

		return catalog.displayName.length > 0 && catalog.allowedUserIDs.length > 0;
	}

	function selectServerType(type: 'single' | 'multi' | 'remote') {
		selectedServerType = type;
		selectServerTypeDialog?.close();
		showServerForm = true;
	}
</script>

<div
	class="flex flex-col gap-8"
	out:fly={{ x: 100, duration }}
	in:fly={{ x: 100, delay: duration }}
>
	{#if showServerForm}
		{@render configureEntryScreen(selectedEntryServer)}
	{:else}
		{@render configureCatalogScreen(mcpCatalog)}
	{/if}
</div>

{#snippet configureCatalogScreen(config: MCPCatalog)}
	<div class="flex flex-col gap-8" out:fly={{ x: -100, duration }} in:fly={{ x: -100 }}>
		{#if topContent}
			{@render topContent()}
		{/if}
		{#if mcpCatalog.id}
			<div class="flex w-full items-center justify-between gap-4">
				<h1 class="flex items-center gap-4 text-2xl font-semibold">
					{config.displayName}
					<button
						class="button-small flex items-center gap-1 text-xs font-normal"
						onclick={async () => {
							refreshing = true;
							await AdminService.refreshMCPCatalog(config.id);
							loadingEntries = AdminService.listMCPCatalogEntries(config.id);
							refreshing = false;
						}}
					>
						{#if refreshing}
							<LoaderCircle class="size-4 animate-spin" /> Refreshing...
						{:else}
							<RefreshCcw class="size-4" />
							Refresh Catalog
						{/if}
					</button>
				</h1>
				<button
					class="button-destructive flex items-center gap-1 text-xs font-normal"
					use:tooltip={'Delete Catalog'}
					onclick={() => {
						deletingCatalog = true;
					}}
				>
					<Trash2 class="size-4" />
				</button>
			</div>
		{:else}
			<h1 class="text-2xl font-semibold">Create MCP Catalog</h1>
		{/if}

		{#if mcpCatalog.id}
			<div class="flex flex-col gap-2">
				<div class="mb-2 flex items-center justify-between">
					<h2 class="text-lg font-semibold">MCP Servers</h2>

					<DotDotDot class="button-primary text-sm">
						{#snippet icon()}
							<span class="flex items-center gap-1">
								<Plus class="size-4" /> Add MCP Server
							</span>
						{/snippet}
						<div class="default-dialog flex min-w-max flex-col p-2">
							<button
								class="menu-button"
								onclick={() => {
									selectServerTypeDialog?.open();
								}}
							>
								Add server
							</button>
							<button
								class="menu-button"
								onclick={() => {
									editingSource = {
										index: -1,
										value: ''
									};
									sourceDialog?.showModal();
								}}
							>
								Add server(s) from Git
							</button>
						</div>
					</DotDotDot>
				</div>

				{#if mcpCatalog.id}
					<Search
						class="dark:bg-surface1 dark:border-surface3 bg-white shadow-sm dark:border"
						onChange={(val) => {
							searchEntries = val;
						}}
						placeholder="Search by name..."
					/>
				{/if}

				{#await Promise.all([loadingEntries, loadingServers])}
					<div class="my-2 flex items-center justify-center">
						<LoaderCircle class="size-6 animate-spin" />
					</div>
				{:then [entries, servers]}
					{@const tableData = convertEntriesAndServersToTableData(entries ?? [], servers ?? [])}
					{@const filteredTableData = searchEntries
						? tableData.filter((item) => {
								return item.name.toLowerCase().includes(searchEntries.toLowerCase());
							})
						: tableData}
					<Table
						data={filteredTableData}
						fields={['name', 'type']}
						onSelectRow={(d) => {
							selectedEntryServer = d.data;
							showServerForm = true;
						}}
						noDataMessage={'No catalog servers added.'}
					>
						{#snippet onRenderColumn(property, d)}
							{#if property === 'name'}
								<p class="flex items-center gap-1">
									{d.name}
									{#if d.source !== 'manual'}
										<span class="text-xs text-gray-500">({d.source.split('/').pop()})</span>{/if}
								</p>
							{:else}
								{d[property as keyof typeof d]}
							{/if}
						{/snippet}
						{#snippet actions(d)}
							{#if d.editable}
								<button
									class="icon-button hover:text-red-500"
									onclick={(e) => {
										e.stopPropagation();
										if (d.data.type === 'mcpserver') {
											deletingServer = d.data as MCPCatalogServer;
										} else {
											deletingEntry = d.data as MCPCatalogEntry;
										}
									}}
									use:tooltip={'Delete Entry'}
								>
									<Trash2 class="size-4" />
								</button>
							{/if}
							<button class="icon-button hover:text-blue-500" use:tooltip={'View Entry'}>
								<Eye class="size-4" />
							</button>
						{/snippet}
					</Table>
				{/await}
			</div>
		{:else}
			<div
				class="dark:bg-surface2 dark:border-surface3 rounded-lg border border-transparent bg-white p-4"
			>
				<div class="flex flex-col gap-6">
					<div class="flex flex-col gap-2">
						<label for="mcp-catalog-name" class="flex-1 text-sm font-light capitalize">
							Name
						</label>
						<input
							id="mcp-catalog-name"
							bind:value={mcpCatalog.displayName}
							class="text-input-filled mt-0.5"
						/>
					</div>
				</div>
			</div>
		{/if}

		<div class="flex flex-col gap-2">
			<div class="mb-2 flex items-center justify-between">
				<h2 class="text-lg font-semibold">Access Control</h2>
				<div class="relative flex items-center gap-4">
					{#await loadingUsers}
						<button class="button-primary flex items-center gap-1 text-sm" disabled>
							<Plus class="size-4" /> Add User/Group
						</button>
					{:then _users}
						<button
							class="button-primary flex items-center gap-1 text-sm"
							onclick={() => {
								addUserGroupDialog?.open();
							}}
						>
							<Plus class="size-4" /> Add User/Group
						</button>
					{/await}
				</div>
			</div>
			{#await loadingUsers}
				<div class="my-2 flex items-center justify-center">
					<LoaderCircle class="size-6 animate-spin" />
				</div>
			{:then users}
				{@const userData = convertUsersToTableData(mcpCatalog?.allowedUserIDs ?? [], users ?? [])}
				<Table
					data={userData}
					fields={['email', 'type', 'role']}
					noDataMessage={'No users or groups added.'}
				>
					{#snippet actions(d)}
						<button
							class="icon-button hover:text-red-500"
							onclick={() => {
								deletingUserGroup = d;
							}}
							use:tooltip={'Delete User/Group'}
						>
							<Trash2 class="size-4" />
						</button>
					{/snippet}
				</Table>
			{/await}
		</div>

		{#if mcpCatalog?.sourceURLs && mcpCatalog.sourceURLs.length > 0 && mcpCatalog.id}
			<div class="flex flex-col gap-2" in:slide={{ axis: 'y', duration }}>
				<h2 class="mb-2 text-lg font-semibold">Catalog Sources</h2>

				<Table
					data={mcpCatalog?.sourceURLs?.map((url, index) => ({ id: index, url })) ?? []}
					fields={['url']}
					noDataMessage={'No catalog sources.'}
				>
					{#snippet actions(d)}
						<button
							class="icon-button hover:text-red-500"
							onclick={() => {
								deletingSource = d.url;
							}}
						>
							<Trash2 class="size-4" />
						</button>
					{/snippet}
				</Table>
			</div>
		{/if}
	</div>
	<div
		class="bg-surface1 sticky bottom-0 left-0 flex w-full justify-end gap-2 py-4 text-gray-400 dark:bg-black dark:text-gray-600"
		out:fly={{ x: -100, duration }}
		in:fly={{ x: -100 }}
	>
		{#if mcpCatalog.id}
			{#if saving === true}
				<div class="flex items-center justify-center font-light">
					<LoaderCircle class="size-6 animate-spin" /> Saving...
				</div>
			{:else if saving === false}
				<div class="flex items-center justify-center font-light">Saved.</div>
			{/if}
		{:else}
			<div class="flex w-full justify-end gap-2">
				<button class="button">Cancel</button>
				<button
					class="button-primary disabled:opacity-75"
					disabled={!validate(mcpCatalog)}
					onclick={async () => {
						saving = true;
						const response = await AdminService.createMCPCatalog(mcpCatalog);
						mcpCatalog = response;
						onCreate?.(mcpCatalog);
						saving = false;
					}}
				>
					{#if saving}
						<LoaderCircle class="size-4 animate-spin" />
					{:else}
						Create Catalog
					{/if}
				</button>
			</div>
		{/if}
	</div>
{/snippet}

{#snippet configureEntryScreen(entry?: typeof selectedEntryServer)}
	<div class="flex flex-col gap-6" in:fly={{ x: 100, delay: duration, duration }}>
		<button
			onclick={() => {
				selectedEntryServer = undefined;
				showServerForm = false;
			}}
			class="button-text flex -translate-x-1 items-center gap-2 p-0 text-lg font-light"
		>
			<ChevronLeft class="size-6" />
			Back to {mcpCatalog?.displayName ?? 'Source'}
		</button>

		<CatalogServerForm
			{entry}
			type={selectedServerType}
			readonly={entry && 'sourceURL' in entry && !!entry.sourceURL}
			catalogId={mcpCatalog?.id}
			onCancel={() => {
				selectedEntryServer = undefined;
				showServerForm = false;
			}}
			onSubmit={async () => {
				loadingEntries = AdminService.listMCPCatalogEntries(mcpCatalog.id);
				loadingServers = AdminService.listMCPCatalogServers(mcpCatalog.id);
				selectedEntryServer = undefined;
				showServerForm = false;
			}}
		/>
	</div>
{/snippet}

<dialog
	bind:this={sourceDialog}
	use:clickOutside={() => closeSourceDialog()}
	class="w-full max-w-md p-4"
>
	{#if editingSource}
		<h3 class="default-dialog-title">
			{editingSource.index === -1 ? 'Add Source URL' : 'Edit Source URL'}
			<button onclick={() => closeSourceDialog()} class="icon-button">
				<X class="size-5" />
			</button>
		</h3>

		<div class="my-4 flex flex-col gap-1">
			<label for="catalog-source-name" class="flex-1 text-sm font-light capitalize"
				>Source URL
			</label>
			<input id="catalog-source-name" bind:value={editingSource.value} class="text-input-filled" />
		</div>

		<div class="flex w-full justify-end gap-2">
			<button class="button" onclick={() => closeSourceDialog()}>Cancel</button>
			<button
				class="button-primary"
				onclick={async () => {
					if (!editingSource) {
						return;
					}

					saving = true;
					if (editingSource.index === -1) {
						mcpCatalog.sourceURLs = [...(mcpCatalog.sourceURLs ?? []), editingSource.value];
					} else {
						mcpCatalog.sourceURLs[editingSource.index] = editingSource.value;
					}

					if (mcpCatalog.id) {
						const response = await AdminService.updateMCPCatalog(mcpCatalog.id, mcpCatalog);
						mcpCatalog = response;
					}
					saving = false;
					closeSourceDialog();
				}}
			>
				Add
			</button>
		</div>
	{/if}
</dialog>

<SearchUsers
	bind:this={addUserGroupDialog}
	filterIds={mcpCatalog?.allowedUserIDs}
	onAdd={async (users) => {
		saving = true;
		const existingEmails = new Set(mcpCatalog.allowedUserIDs ?? []);
		const newUsers = users.filter((user) => !existingEmails.has(user.id));
		mcpCatalog.allowedUserIDs = [
			...(mcpCatalog?.allowedUserIDs ?? []),
			...newUsers.map((user) => user.id)
		];

		if (mcpCatalog.id) {
			const response = await AdminService.updateMCPCatalog(mcpCatalog.id, mcpCatalog);
			mcpCatalog = response;
		}
		saving = false;
	}}
/>

<Confirm
	msg={`Delete ${deletingUserGroup?.email}?`}
	show={Boolean(deletingUserGroup)}
	onsuccess={async () => {
		saving = true;
		mcpCatalog.allowedUserIDs = mcpCatalog.allowedUserIDs.filter(
			(id) => id !== deletingUserGroup?.id
		);
		const response = await AdminService.updateMCPCatalog(mcpCatalog.id, mcpCatalog);
		mcpCatalog = response;
		deletingUserGroup = undefined;
		saving = false;
	}}
	oncancel={() => (deletingUserGroup = undefined)}
/>

<Confirm
	msg={`Delete ${deletingSource}?`}
	show={Boolean(deletingSource)}
	onsuccess={async () => {
		saving = true;
		mcpCatalog.sourceURLs = mcpCatalog.sourceURLs.filter((url) => url !== deletingSource);
		const response = await AdminService.updateMCPCatalog(mcpCatalog.id, mcpCatalog);
		mcpCatalog = response;
		deletingSource = undefined;
		saving = false;
	}}
	oncancel={() => (deletingSource = undefined)}
/>

<Confirm
	msg="Are you sure you want to delete this catalog?"
	show={deletingCatalog}
	onsuccess={async () => {
		saving = true;
		await AdminService.deleteMCPCatalog(mcpCatalog.id);
		goto('/v2/admin/mcp-catalogs');
	}}
	oncancel={() => (deletingCatalog = false)}
/>

<Confirm
	msg={`Are you sure you want to delete this catalog entry?`}
	show={Boolean(deletingEntry)}
	onsuccess={async () => {
		if (!deletingEntry) {
			return;
		}
		saving = true;
		await AdminService.deleteMCPCatalogEntry(mcpCatalog.id, deletingEntry.id);
		loadingEntries = AdminService.listMCPCatalogEntries(mcpCatalog.id);
		deletingEntry = undefined;
		saving = false;
	}}
	oncancel={() => (deletingEntry = undefined)}
/>

<Confirm
	msg={`Are you sure you want to delete this catalog server?`}
	show={Boolean(deletingServer)}
	onsuccess={async () => {
		if (!deletingServer) {
			return;
		}
		saving = true;
		await AdminService.deleteMCPCatalogServer(mcpCatalog.id, deletingServer.id);
		loadingServers = AdminService.listMCPCatalogServers(mcpCatalog.id);
		deletingServer = undefined;
		saving = false;
	}}
	oncancel={() => (deletingServer = undefined)}
/>

<ResponsiveDialog title="Select Server Type" class="md:w-lg" bind:this={selectServerTypeDialog}>
	<div class="my-4 flex flex-col gap-4">
		<button
			class="group dark:bg-surface2 hover:bg-surface1 dark:hover:bg-surface3 dark:border-surface3 border-surface2 flex cursor-pointer items-center gap-4 rounded-md border bg-white px-2 py-4 text-left transition-colors duration-300"
			onclick={() => selectServerType('single')}
		>
			<User
				class="size-12 flex-shrink-0 pl-1 text-gray-500 transition-colors group-hover:text-inherit"
			/>
			<div>
				<p class="mb-1 text-sm font-semibold">Single User Server</p>
				<span class="block text-xs leading-4 text-gray-400 dark:text-gray-600">
					This option is appropriate for servers that require individualized configuration or were
					not designed for multi-user access, such as most studio MCP servers. When a user selects
					this server, a private instance will be created for them.
				</span>
			</div>
		</button>
		<button
			class="group dark:bg-surface2 hover:bg-surface1 dark:hover:bg-surface3 dark:border-surface3 border-surface2 flex cursor-pointer items-center gap-4 rounded-md border bg-white px-2 py-4 text-left transition-colors duration-300"
			onclick={() => selectServerType('multi')}
		>
			<Users
				class="size-12 flex-shrink-0 pl-1 text-gray-500 transition-colors group-hover:text-inherit"
			/>
			<div>
				<p class="mb-1 text-sm font-semibold">Multi-User Server</p>
				<span class="block text-xs leading-4 text-gray-400 dark:text-gray-600">
					This option is appropriate for servers designed to handle multiple user connections, such
					as most Streamable HTTP servers. When you create this server, a running instance will be
					deployed and any user with access to this catlog will be able to connect to it.
				</span>
			</div>
		</button>
		<button
			class="group dark:bg-surface2 hover:bg-surface1 dark:hover:bg-surface3 dark:border-surface3 border-surface2 flex cursor-pointer items-center gap-4 rounded-md border bg-white px-2 py-4 text-left transition-colors duration-300"
			onclick={() => selectServerType('remote')}
		>
			<Container
				class="size-12 flex-shrink-0 pl-1 text-gray-500 transition-colors group-hover:text-inherit"
			/>
			<div>
				<p class="mb-1 text-sm font-semibold">Remote Server</p>
				<span class="block text-xs leading-4 text-gray-400 dark:text-gray-600">
					This option is appropriate for allowing users to connect to MCP servers that are already
					elsewhere. When a user selects this server, their connection to the remote MCP server will
					go through the Obot gateway.
				</span>
			</div>
		</button>
	</div>
</ResponsiveDialog>
