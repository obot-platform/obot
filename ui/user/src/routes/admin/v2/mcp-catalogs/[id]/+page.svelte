<script lang="ts">
	import { goto } from '$app/navigation';
	import { clickOutside } from '$lib/actions/clickoutside.js';
	import { tooltip } from '$lib/actions/tooltip.svelte.js';
	import SearchUsers from '$lib/components/admin/SearchUsers.svelte';
	import Confirm from '$lib/components/Confirm.svelte';
	import Layout from '$lib/components/Layout.svelte';
	import ProjectMcpConfig from '$lib/components/mcp/ProjectMcpConfig.svelte';
	import Search from '$lib/components/Search.svelte';
	import Table from '$lib/components/Table.svelte';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants.js';
	import {
		Role,
		type MCPCatalog,
		type MCPCatalogEntry,
		type OrgUser
	} from '$lib/services/admin/types.js';
	import { AdminService } from '$lib/services/index.js';
	import { ChevronLeft, Eye, LoaderCircle, Plus, RefreshCcw, Trash2, X } from 'lucide-svelte';
	import { onMount } from 'svelte';
	import { fly } from 'svelte/transition';

	let { data } = $props();
	const { mcpCatalog: initialMcpCatalog } = data;
	let mcpCatalog = $state(initialMcpCatalog);

	let loadingEntries = $state<Promise<MCPCatalogEntry[]>>();
	let loadingUsers = $state<Promise<OrgUser[]>>();
	let saving = $state<boolean | undefined>();
	let refreshing = $state(false);

	let editingSource = $state<{ index: number; value: string }>();
	let sourceDialog = $state<HTMLDialogElement>();
	let selectedEntry = $state<MCPCatalogEntry>();
	let searchEntries = $state('');
	let addUserGroupDialog = $state<ReturnType<typeof SearchUsers>>();

	let deletingUserGroup = $state<{ id: string; email: string }>();
	let deletingSource = $state<string>();
	let deletingCatalog = $state(false);
	let deletingEntry = $state<MCPCatalogEntry>();

	const duration = PAGE_TRANSITION_DURATION;

	onMount(async () => {
		if (mcpCatalog) {
			loadingEntries = AdminService.listMCPCatalogEntries(mcpCatalog.id);
		}

		loadingUsers = AdminService.listUsers();
	});

	function closeSourceDialog() {
		editingSource = undefined;
		sourceDialog?.close();
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

	function convertEntriesToTableData(entries: MCPCatalogEntry[] | undefined) {
		if (!entries) {
			return [];
		}

		return entries.map((entry) => {
			return {
				id: entry.id,
				type: entry?.commandManifest ? 'Hosted' : entry?.urlManifest ? 'Remote' : '',
				source: '',
				name: entry.commandManifest?.server.name ?? entry.urlManifest?.server.name ?? '',
				data: entry,
				editable: entry.editable ?? false,
				deployments: 0
			};
		});
	}
</script>

<Layout>
	{#if selectedEntry}
		{@render configureEntryScreen(selectedEntry)}
	{:else if mcpCatalog}
		{@render configureCatalogScreen(mcpCatalog)}
	{/if}
</Layout>

{#snippet configureCatalogScreen(config: MCPCatalog)}
	<div class="flex flex-col gap-8 py-8" out:fly={{ x: -100, duration }} in:fly={{ x: -100 }}>
		<a
			href={`/admin/v2/mcp-catalogs`}
			class="button-text flex -translate-x-1 items-center gap-2 p-0 text-lg font-light"
		>
			<ChevronLeft class="size-6" />
			Back to MCP Catalogs
		</a>

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

		<div class="flex flex-col gap-2">
			<div class="mb-2 flex items-center justify-between">
				<h2 class="text-lg font-semibold">Catalog Sources</h2>
				<div class="relative flex items-center gap-4">
					<button
						class="button-primary flex items-center gap-1 text-sm"
						onclick={() => {
							editingSource = {
								index: -1,
								value: ''
							};
							sourceDialog?.showModal();
						}}
					>
						<Plus class="size-4" /> Add Source
					</button>
				</div>
			</div>

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

		<div class="flex flex-col gap-2">
			<div class="mb-2 flex items-center justify-between">
				<h2 class="text-lg font-semibold">Catalog Entries</h2>

				<div class="relative flex items-center gap-4">
					<button
						class="button-primary flex items-center gap-1 text-sm"
						onclick={() => {
							selectedEntry = {
								id: '',
								created: ''
							};
						}}
					>
						<Plus class="size-4" /> Add Entry
					</button>
				</div>
			</div>

			<Search
				class="dark:bg-surface1 dark:border-surface3 bg-white shadow-sm dark:border"
				onChange={(val) => {
					searchEntries = val;
				}}
				placeholder="Search by name..."
			/>

			{#await loadingEntries}
				<div class="my-2 flex items-center justify-center">
					<LoaderCircle class="size-6 animate-spin" />
				</div>
			{:then entries}
				{@const entrieTableData = convertEntriesToTableData(entries)}
				{@const filteredTableData = searchEntries
					? entrieTableData.filter((entry) => {
							return entry.name.toLowerCase().includes(searchEntries.toLowerCase());
						})
					: entrieTableData}
				<Table
					data={filteredTableData}
					fields={['name', 'type']}
					onSelectRow={(d) => (selectedEntry = d.data)}
					noDataMessage={'No catalog entries.'}
				>
					{#snippet actions(d)}
						{#if d.editable}
							<button
								class="icon-button hover:text-red-500"
								onclick={(e) => {
									e.stopPropagation();
									deletingEntry = d.data;
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

		<div class="flex flex-col gap-2">
			<div class="mb-2 flex items-center justify-between">
				<h2 class="text-lg font-semibold">Users & Groups</h2>
				<div class="relative flex items-center gap-4">
					{#await loadingUsers}
						<button class="button-primary flex items-center gap-1 text-sm" disabled>
							<Plus class="size-4" /> Add User/Group
						</button>
					{:then _users}
						<button
							class="button-primary flex items-center gap-1 text-sm"
							onclick={() => {
								addUserGroupDialog?.show();
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
	</div>
	<div
		class="bg-surface1 sticky bottom-0 left-0 flex w-full justify-end gap-2 py-4 text-gray-400 dark:bg-black dark:text-gray-600"
	>
		{#if saving === true}
			<div class="flex items-center justify-center font-light">
				<LoaderCircle class="size-6 animate-spin" /> Saving...
			</div>
		{:else if saving === false}
			<div class="flex items-center justify-center font-light">Saved.</div>
		{/if}
	</div>
{/snippet}

{#snippet configureEntryScreen(entry: typeof selectedEntry)}
	{#if entry}
		<div class="flex flex-col gap-6 py-8" in:fly={{ x: 100, delay: duration, duration }}>
			<button
				onclick={() => (selectedEntry = undefined)}
				class="button-text flex -translate-x-1 items-center gap-2 p-0 text-lg font-light"
			>
				<ChevronLeft class="size-6" />
				Back to {mcpCatalog?.displayName ?? 'Source'}
			</button>

			<ProjectMcpConfig
				catalogID={mcpCatalog?.id}
				{entry}
				readonly={entry.id ? !entry.editable : false}
				onClose={() => {
					selectedEntry = undefined;
					if (mcpCatalog?.id) {
						loadingEntries = AdminService.listMCPCatalogEntries(mcpCatalog.id);
					}
				}}
			/>
		</div>
	{/if}
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
			<label for="catalog-source-name" class="flex-1 text-sm font-light capitalize"> URL </label>
			<input id="catalog-source-name" bind:value={editingSource.value} class="text-input-filled" />
		</div>

		<div class="flex w-full justify-end gap-2">
			<button class="button" onclick={() => closeSourceDialog()}>Cancel</button>
			<button
				class="button-primary"
				onclick={async () => {
					if (!mcpCatalog || !editingSource) {
						return;
					}

					saving = true;
					if (editingSource.index === -1) {
						mcpCatalog.sourceURLs = [...(mcpCatalog.sourceURLs ?? []), editingSource.value];
					} else {
						mcpCatalog.sourceURLs[editingSource.index] = editingSource.value;
					}

					const response = await AdminService.updateMCPCatalog(mcpCatalog.id, mcpCatalog);
					mcpCatalog = response;
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
		if (!mcpCatalog) {
			return;
		}

		saving = true;
		const existingEmails = new Set(mcpCatalog?.allowedUserIDs ?? []);
		const newUsers = users.filter((user) => !existingEmails.has(user.id));
		mcpCatalog.allowedUserIDs = [
			...(mcpCatalog?.allowedUserIDs ?? []),
			...newUsers.map((user) => user.id)
		];

		const response = await AdminService.updateMCPCatalog(mcpCatalog.id, mcpCatalog);
		mcpCatalog = response;
		saving = false;
	}}
/>

<Confirm
	msg={`Delete ${deletingUserGroup?.email}?`}
	show={Boolean(deletingUserGroup)}
	onsuccess={async () => {
		if (!mcpCatalog) {
			return;
		}
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
		if (!mcpCatalog) {
			return;
		}
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
		if (!mcpCatalog) {
			return;
		}
		saving = true;
		await AdminService.deleteMCPCatalog(mcpCatalog.id);
		goto('/admin/v2/mcp-catalogs');
	}}
	oncancel={() => (deletingCatalog = false)}
/>

<Confirm
	msg={`Are you sure you want to delete this catalog entry?`}
	show={Boolean(deletingEntry)}
	onsuccess={async () => {
		if (!mcpCatalog || !deletingEntry) {
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

<svelte:head>
	<title>Obot | {mcpCatalog?.displayName ?? 'MCP Catalog'}</title>
</svelte:head>
