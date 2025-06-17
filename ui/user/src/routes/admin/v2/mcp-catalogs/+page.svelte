<script lang="ts">
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import Layout from '$lib/components/Layout.svelte';
	import Table from '$lib/components/Table.svelte';
	import { BookOpenText, ChevronLeft, LoaderCircle, Plus, Trash2 } from 'lucide-svelte';
	import { fly } from 'svelte/transition';
	import { goto } from '$app/navigation';
	import {
		Role,
		type MCPCatalog,
		type MCPCatalogManifest,
		type OrgUser
	} from '$lib/services/admin/types';
	import SearchUsers from '$lib/components/admin/SearchUsers.svelte';
	import { AdminService } from '$lib/services';
	import Confirm from '$lib/components/Confirm.svelte';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants.js';
	import { onMount } from 'svelte';

	let { data } = $props();
	const { mcpCatalogs: initialCatalogs } = data;

	type CreateCatalogData = Omit<MCPCatalogManifest, 'allowedUserIDs'> & { allowedUsers: OrgUser[] };

	let mcpCatalogs = $state(initialCatalogs);
	let createCatalog = $state<CreateCatalogData>();
	let addUserGroupDialog = $state<ReturnType<typeof SearchUsers>>();
	let loading = $state(false);
	let catalogToDelete = $state<MCPCatalog>();

	let entriesCounts = $state<Record<string, number>>({});

	let displayableUsers = $derived(
		(createCatalog?.allowedUsers ?? []).map((user) => ({
			...user,
			role: user.role === Role.ADMIN ? 'Admin' : 'User',
			type: 'User'
		}))
	);

	let mcpCatalogsTableData = $derived(
		mcpCatalogs.map((catalog) => ({
			...catalog,
			entries: entriesCounts[catalog.id] ?? 0
		}))
	);

	onMount(async () => {
		for (const catalog of mcpCatalogs) {
			const entries = await AdminService.listMCPCatalogEntries(catalog.id);
			entriesCounts[catalog.id] = entries.length ?? 0;
		}
	});

	function handleNavigation(url: string) {
		goto(url, { replaceState: false });
	}

	function handleCreateCatalog() {
		createCatalog = {
			displayName: '',
			sourceURLs: [''],
			allowedUsers: []
		};
	}

	function validate(catalog: CreateCatalogData) {
		if (!catalog) return false;

		return (
			catalog.displayName.length > 0 &&
			catalog.sourceURLs.length > 0 &&
			catalog.allowedUsers.length > 0
		);
	}

	async function handleSaveCatalog() {
		if (!createCatalog || !validate(createCatalog)) return;

		loading = true;
		const catalogManifest: MCPCatalogManifest = {
			displayName: createCatalog.displayName,
			sourceURLs: createCatalog.sourceURLs,
			allowedUserIDs: createCatalog.allowedUsers.map((user) => user.id)
		};

		await AdminService.createMCPCatalog(catalogManifest);
		mcpCatalogs = await AdminService.listMCPCatalogs();

		createCatalog = undefined;
		loading = false;
	}

	const duration = PAGE_TRANSITION_DURATION;
</script>

<Layout>
	<div class="my-8" in:fly={{ x: 100, duration }} out:fly={{ x: -100, duration }}>
		{#if createCatalog}
			{@render createCatalogScreen(createCatalog)}
		{:else}
			<div
				class="flex flex-col gap-8"
				in:fly={{ x: 100, delay: duration, duration }}
				out:fly={{ x: -100, duration }}
			>
				<div class="flex items-center justify-between">
					<h1 class="text-2xl font-semibold">MCP Catalogs</h1>
					{#if mcpCatalogs.length > 0}
						<div class="relative flex items-center gap-4">
							<button
								class="button-primary flex items-center gap-1 text-sm"
								onclick={handleCreateCatalog}
							>
								<Plus class="size-6" /> Create New Catalog
							</button>
						</div>
					{/if}
				</div>
				{#if mcpCatalogs.length === 0}
					<div class="mt-12 flex w-md flex-col items-center gap-4 self-center text-center">
						<BookOpenText class="size-24 text-gray-200 dark:text-gray-900" />
						<h4 class="text-lg font-semibold text-gray-400 dark:text-gray-600">
							No created catalogs
						</h4>
						<p class="text-sm font-light text-gray-400 dark:text-gray-600">
							Looks like you don't have any catalogs created yet. <br />
							Click the button below to get started.
						</p>

						<button class="button-primary w-fit text-sm" onclick={handleCreateCatalog}
							>Add New Catalog</button
						>
					</div>
				{:else}
					<Table
						data={mcpCatalogsTableData}
						fields={['displayName', 'entries']}
						onSelectRow={(d) => {
							handleNavigation(`/admin/v2/mcp-catalogs/${d.id}`);
						}}
						headers={[{ title: 'Name', property: 'displayName' }]}
					>
						{#snippet actions(d)}
							<button
								class="icon-button hover:text-red-500"
								onclick={(e) => {
									e.stopPropagation();
									catalogToDelete = d;
								}}
								use:tooltip={'Delete Catalog'}
							>
								<Trash2 class="size-4" />
							</button>
						{/snippet}
					</Table>
				{/if}
			</div>
		{/if}
	</div>
</Layout>

{#snippet createCatalogScreen(catalog: CreateCatalogData)}
	<div
		class="flex flex-col gap-8"
		in:fly={{ x: 100, delay: duration, duration }}
		out:fly={{ x: -100, duration }}
	>
		<button
			onclick={() => (createCatalog = undefined)}
			class="button-text flex -translate-x-1 items-center gap-2 p-0 text-lg font-light"
		>
			<ChevronLeft class="size-6" />
			Back to MCP Catalogs
		</button>

		<h1 class="text-2xl font-semibold">Create MCP Catalog</h1>

		<div
			class="dark:bg-surface2 dark:border-surface3 rounded-lg border border-transparent bg-white p-4"
		>
			<div class="flex flex-col gap-6">
				<div class="flex flex-col gap-2">
					<label for="mcp-catalog-name" class="flex-1 text-sm font-light capitalize"> Name </label>
					<input
						id="mcp-catalog-name"
						bind:value={catalog.displayName}
						class="text-input-filled mt-0.5"
					/>
				</div>

				<div class="flex flex-col gap-2">
					<label
						for="catalog-source-url"
						class="mb-2 flex flex-1 items-center justify-between gap-4 text-sm font-light capitalize"
					>
						Source URL(s)
						<button
							class="button-primary flex items-center gap-1 text-sm"
							onclick={() => {
								catalog?.sourceURLs.push('');
							}}
						>
							<Plus class="size-4" /> Add Source
						</button>
					</label>
					{#each catalog.sourceURLs as _url, index}
						<div class="flex items-center justify-between gap-2">
							<input
								id="catalog-source-url"
								bind:value={catalog.sourceURLs[index]}
								class="text-input-filled mt-0.5"
							/>

							<button
								class="icon-button"
								onclick={() => {
									catalog?.sourceURLs.splice(index, 1);
								}}
							>
								<Trash2 class="size-4" />
							</button>
						</div>
					{/each}
				</div>
			</div>
		</div>

		<div class="flex flex-col gap-2">
			<div class="mb-2 flex items-center justify-between">
				<h2 class="text-lg font-semibold">Users & Groups</h2>
				<div class="relative flex items-center gap-4">
					<button
						class="button-primary flex items-center gap-1 text-sm"
						onclick={() => {
							addUserGroupDialog?.show();
						}}
					>
						<Plus class="size-4" /> Add User/Group
					</button>
				</div>
			</div>
			<Table
				data={displayableUsers}
				fields={['email', 'type', 'role']}
				noDataMessage={'No users or groups added'}
			>
				{#snippet actions(d)}
					<button
						class="icon-button"
						onclick={() => {
							const index = catalog.allowedUsers.findIndex((user) => user.id === d.id);
							if (index !== -1) {
								catalog.allowedUsers.splice(index, 1);
							}
						}}
					>
						<Trash2 class="size-4" />
					</button>
				{/snippet}
			</Table>
		</div>
		<div class="flex w-full justify-end gap-2">
			<button class="button">Cancel</button>
			<button
				class="button-primary disabled:opacity-75"
				disabled={!validate(catalog)}
				onclick={handleSaveCatalog}
			>
				{#if loading}
					<LoaderCircle class="size-4 animate-spin" />
				{:else}
					Save Catalog
				{/if}
			</button>
		</div>
	</div>
{/snippet}

<SearchUsers
	bind:this={addUserGroupDialog}
	onAdd={(users) => {
		if (createCatalog) {
			const existingEmails = new Set(createCatalog.allowedUsers.map((user) => user.email));
			const newUsers = users.filter((user) => !existingEmails.has(user.email));
			createCatalog.allowedUsers = [...createCatalog.allowedUsers, ...newUsers];
		}
	}}
/>

<Confirm
	msg={'Are you sure you want to delete all memories?'}
	show={Boolean(catalogToDelete)}
	onsuccess={async () => {
		if (!catalogToDelete) return;
		await AdminService.deleteMCPCatalog(catalogToDelete.id);
		mcpCatalogs = await AdminService.listMCPCatalogs();
		catalogToDelete = undefined;
	}}
	oncancel={() => (catalogToDelete = undefined)}
/>

<svelte:head>
	<title>Obot | MCP Catalogs</title>
</svelte:head>
