<script lang="ts">
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import Layout from '$lib/components/Layout.svelte';
	import Table from '$lib/components/table/Table.svelte';
	import { BookOpenText, ChevronLeft, LoaderCircle, Plus, Trash2 } from 'lucide-svelte';
	import { fly } from 'svelte/transition';
	import Confirm from '$lib/components/Confirm.svelte';
	import { DEFAULT_MCP_CATALOG_ID, PAGE_TRANSITION_DURATION } from '$lib/constants.js';
	import { onMount } from 'svelte';
	import {
		fetchMcpServerAndEntries,
		getAdminMcpServerAndEntries,
		initMcpServerAndEntries
	} from '$lib/context/admin/mcpServerAndEntries.svelte';
	import { AdminService, type MCPFilter } from '$lib/services/index.js';
	import FilterForm from '$lib/components/admin/FilterForm.svelte';
	import { openUrl } from '$lib/utils';
	import { profile } from '$lib/stores';

	initMcpServerAndEntries();

	let showCreateFilter = $state(false);
	let loading = $state(true);
	let filterToDelete = $state<MCPFilter>();

	let filters = $state<MCPFilter[]>([]);

	async function refresh() {
		loading = true;
		filters = await AdminService.listMCPFilters();
		loading = false;
	}

	onMount(() => {
		const url = new URL(window.location.href);
		const queryParams = new URLSearchParams(url.search);
		if (queryParams.get('new')) {
			showCreateFilter = true;
		}
	});

	async function navigateAfterCreated() {
		showCreateFilter = false;
		// Refresh the filters list to ensure we have the latest data
		await refresh();
	}

	const duration = PAGE_TRANSITION_DURATION;
	onMount(async () => {
		await fetchMcpServerAndEntries(DEFAULT_MCP_CATALOG_ID);
		await refresh();
	});
</script>

<Layout>
	<div
		class="my-4 h-full w-full"
		in:fly={{ x: 100, duration, delay: duration }}
		out:fly={{ x: -100, duration }}
	>
		{#if showCreateFilter}
			{@render createFilterScreen()}
		{:else}
			<div
				class="flex flex-col gap-8"
				in:fly={{ x: 100, delay: duration, duration }}
				out:fly={{ x: -100, duration }}
			>
				<div class="flex items-center justify-between">
					<h1 class="text-2xl font-semibold">Filters</h1>
					<div class="relative flex items-center gap-4">
						{#if loading}
							<LoaderCircle class="size-4 animate-spin" />
						{/if}
						{#if !profile.current.isAdminReadonly?.()}
							{@render addFilterButton()}
						{/if}
					</div>
				</div>
				{#if filters.length === 0}
					<div class="mt-12 flex w-md flex-col items-center gap-4 self-center text-center">
						<BookOpenText class="size-24 text-gray-200 dark:text-gray-900" />
						<h4 class="text-lg font-semibold text-gray-400 dark:text-gray-600">
							No created filters
						</h4>
						<p class="text-sm font-light text-gray-400 dark:text-gray-600">
							Looks like you don't have any filters created yet. <br />
							Click the "Add New Filter" button above to get started.
						</p>
					</div>
				{:else}
					<Table
						data={filters}
						fields={['name', 'url', 'selectors']}
						onSelectRow={(d, isCtrlClick) => {
							const url = `/admin/filters/${d.id}`;
							openUrl(url, isCtrlClick);
						}}
						headers={[
							{
								title: 'Name',
								property: 'name'
							},
							{
								title: 'Webhook URL',
								property: 'url'
							},
							{
								title: 'Selectors',
								property: 'selectors'
							}
						]}
					>
						{#snippet actions(d: MCPFilter)}
							<button
								class="icon-button hover:text-red-500"
								onclick={(e) => {
									e.stopPropagation();
									filterToDelete = d;
								}}
								use:tooltip={'Delete Filter'}
							>
								<Trash2 class="size-4" />
							</button>
						{/snippet}
						{#snippet onRenderColumn(property, d: MCPFilter)}
							{#if property === 'name'}
								{d.name || '-'}
							{:else if property === 'url'}
								{d.url || '-'}
							{:else if property === 'selectors'}
								{@const count = d.selectors?.length || 0}
								{count > 0 ? `${count} selector${count > 1 ? 's' : ''}` : '-'}
							{:else}
								-
							{/if}
						{/snippet}
					</Table>
				{/if}
			</div>
		{/if}
	</div>
</Layout>

{#snippet addFilterButton()}
	<button
		class="button-primary flex items-center gap-1 text-sm"
		onclick={() => (showCreateFilter = true)}
	>
		<Plus class="size-4" /> Add New Filter
	</button>
{/snippet}

{#snippet createFilterScreen()}
	<div
		class="h-full w-full"
		in:fly={{ x: 100, delay: duration, duration }}
		out:fly={{ x: -100, duration }}
	>
		<FilterForm onCreate={navigateAfterCreated} mcpEntriesContextFn={getAdminMcpServerAndEntries}>
			{#snippet topContent()}
				<button
					onclick={() => (showCreateFilter = false)}
					class="button-text flex -translate-x-1 items-center gap-2 p-0 text-lg font-light"
				>
					<ChevronLeft class="size-6" />
					Filters
				</button>
			{/snippet}
		</FilterForm>
	</div>
{/snippet}

<Confirm
	msg="Are you sure you want to delete this filter?"
	show={!!filterToDelete}
	onsuccess={async () => {
		if (!filterToDelete) return;
		await AdminService.deleteMCPFilter(filterToDelete.id);
		await refresh();
		filterToDelete = undefined;
	}}
	oncancel={() => (filterToDelete = undefined)}
/>

<svelte:head>
	<title>Obot | Filters</title>
</svelte:head>
