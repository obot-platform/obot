<script lang="ts">
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import Layout from '$lib/components/Layout.svelte';
	import Table from '$lib/components/Table.svelte';
	import { BookOpenText, ChevronLeft, Plus, Trash2 } from 'lucide-svelte';
	import { fly } from 'svelte/transition';
	import { goto } from '$app/navigation';
	import { type MCPCatalog } from '$lib/services/admin/types';
	import { AdminService } from '$lib/services';
	import Confirm from '$lib/components/Confirm.svelte';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants.js';
	import { onMount } from 'svelte';
	import CatalogForm from '$lib/components/admin/CatalogForm.svelte';

	let { data } = $props();
	const { mcpCatalogs: initialCatalogs } = data;

	let mcpCatalogs = $state(initialCatalogs);
	let showCreateCatalog = $state(false);
	let catalogToDelete = $state<MCPCatalog>();

	let entriesCounts = $state<Record<string, number>>({});

	let mcpCatalogsTableData = $derived(
		mcpCatalogs.map((catalog) => ({
			...catalog,
			entries: typeof entriesCounts[catalog.id] === 'number' ? entriesCounts[catalog.id] : '-'
		}))
	);

	onMount(async () => {
		for (const catalog of mcpCatalogs) {
			const entries = await AdminService.listMCPCatalogEntries(catalog.id);
			const servers = await AdminService.listMCPCatalogServers(catalog.id);
			entriesCounts[catalog.id] = (entries.length ?? 0) + (servers.length ?? 0);
		}
	});

	function handleNavigation(url: string) {
		goto(url, { replaceState: false });
	}

	async function navigateToCreated(catalog: MCPCatalog) {
		showCreateCatalog = false;
		goto(`/v2/admin/access-control/${catalog.id}`, { replaceState: false });
	}

	const duration = PAGE_TRANSITION_DURATION;
</script>

<Layout>
	<div class="my-4" in:fly={{ x: 100, duration, delay: duration }} out:fly={{ x: -100, duration }}>
		{#if showCreateCatalog}
			{@render createCatalogScreen()}
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
								onclick={() => (showCreateCatalog = true)}
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

						<button class="button-primary w-fit text-sm" onclick={() => (showCreateCatalog = true)}
							>Add New Catalog</button
						>
					</div>
				{:else}
					<Table
						data={mcpCatalogsTableData}
						fields={['displayName', 'entries']}
						onSelectRow={(d) => {
							handleNavigation(`/v2/admin/access-control/${d.id}`);
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

{#snippet createCatalogScreen()}
	<div in:fly={{ x: 100, delay: duration, duration }} out:fly={{ x: -100, duration }}>
		<CatalogForm onCreate={navigateToCreated}>
			{#snippet topContent()}
				<button
					onclick={() => (showCreateCatalog = false)}
					class="button-text flex -translate-x-1 items-center gap-2 p-0 text-lg font-light"
				>
					<ChevronLeft class="size-6" />
					Back to MCP Catalogs
				</button>
			{/snippet}
		</CatalogForm>
	</div>
{/snippet}

<Confirm
	msg={'Are you sure you want to delete this catalog?'}
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
	<title>Obot | Access Control</title>
</svelte:head>
