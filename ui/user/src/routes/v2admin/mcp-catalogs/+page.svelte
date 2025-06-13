<script lang="ts">
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import Layout from '$lib/components/Layout.svelte';
	import Table from '$lib/components/Table.svelte';
	import { ChevronLeft, Plus, Trash2 } from 'lucide-svelte';
	import { fly } from 'svelte/transition';
	import { goto } from '$app/navigation';
	import type { MCPCatalog, MCPCatalogManifest } from '$lib/services/admin/types';
	import SearchUsers from '$lib/components/admin/SearchUsers.svelte';

	const mockData: (MCPCatalog & { entries: number })[] = [
		{
			id: '1',
			displayName: 'Common',
			entries: 100,
			sourceURLs: ['https://example.com/common'],
			allowedUserIDs: ['1', '2', '3']
		},
		{
			id: '2',
			displayName: 'Engineering',
			entries: 32,
			sourceURLs: ['https://example.com/engineering'],
			allowedUserIDs: ['1', '2', '3']
		},
		{
			id: '3',
			displayName: 'Marketing',
			entries: 15,
			sourceURLs: ['https://example.com/marketing'],
			allowedUserIDs: ['1', '2', '3']
		}
	];

	let createCatalog = $state<MCPCatalogManifest>();
	let addUserGroupDialog = $state<ReturnType<typeof SearchUsers>>();

	function handleNavigation(url: string) {
		goto(url, { replaceState: false });
	}

	const duration = 200;
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
					<div class="relative flex items-center gap-4">
						<button
							class="button-primary flex items-center gap-1 text-sm"
							onclick={() => {
								createCatalog = {
									displayName: '',
									sourceURLs: [''],
									allowedUserIDs: []
								};
							}}
						>
							<Plus class="size-6" /> Create New Catalog
						</button>
					</div>
				</div>
				<Table
					data={mockData}
					fields={['displayName', 'entries']}
					onSelectRow={(d) => {
						handleNavigation(`/v2admin/mcp-catalogs/${d.id}`);
					}}
					headers={[{ title: 'Name', property: 'displayName' }]}
				>
					{#snippet actions(d)}
						<button
							class="icon-button hover:text-red-500"
							onclick={() => {
								console.log(d);
							}}
							use:tooltip={'Delete catalog'}
						>
							<Trash2 class="size-4" />
						</button>
					{/snippet}
				</Table>
			</div>
		{/if}
	</div>
</Layout>

{#snippet createCatalogScreen(catalog: MCPCatalogManifest)}
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
			<Table data={[]} fields={['name', 'type', 'role']}>
				{#snippet actions(d)}
					<button
						class="icon-button"
						onclick={() => {
							console.log(d);
						}}
					>
						<Trash2 class="size-4" />
					</button>
				{/snippet}
			</Table>
		</div>
		<div class="flex w-full justify-end gap-2">
			<button class="button">Cancel</button>
			<button class="button-primary">Save Catalog</button>
		</div>
	</div>
{/snippet}

<SearchUsers bind:this={addUserGroupDialog} onAdd={() => {}} />

<svelte:head>
	<title>Obot | MCP Catalogs</title>
</svelte:head>
