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
	import Search from '$lib/components/Search.svelte';
	import { replaceState } from '$app/navigation';
	import { debounce } from 'es-toolkit';
	import { page } from '$app/state';
	import {
		clearUrlParams,
		getTableUrlParamsFilters,
		getTableUrlParamsSort,
		setSearchParamsToLocalStorage,
		setSortUrlParams,
		setFilterUrlParams
	} from '$lib/url';

	initMcpServerAndEntries();

	let showCreateFilter = $state(false);
	let loading = $state(true);
	let filterToDelete = $state<MCPFilter>();

	let filters = $state<MCPFilter[]>([]);
	let filteredFilters = $derived(
		filters.filter((filter) => filter.name?.toLowerCase().includes(query.toLowerCase()))
	);

	let query = $state('');
	let urlFilters = $derived(getTableUrlParamsFilters());
	let initSort = $derived(getTableUrlParamsSort());

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

	const updateQuery = debounce((value: string) => {
		query = value;

		if (value) {
			page.url.searchParams.set('query', value);
		} else {
			page.url.searchParams.delete('query');
		}

		replaceState(page.url, { query });
	}, 100);

	const duration = PAGE_TRANSITION_DURATION;
	onMount(async () => {
		await fetchMcpServerAndEntries(DEFAULT_MCP_CATALOG_ID);
		await refresh();

		if (page.url.searchParams.size > 0) {
			page.url.searchParams.forEach((value, key) => {
				urlFilters[key] = value.split(',');
			});
		}
	});
</script>

<Layout title="Filters">
	<div
		class="h-full w-full"
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
				<div class="flex flex-col gap-2">
					<Search
						value={query}
						class="dark:bg-surface1 dark:border-surface3 bg-background border border-transparent shadow-sm"
						onChange={updateQuery}
						placeholder="Search filters..."
					/>
					{#if filters.length === 0}
						<div class="mt-12 flex w-md flex-col items-center gap-4 self-center text-center">
							<BookOpenText class="text-on-surface1 size-24 opacity-50" />
							<h4 class="text-on-surface1 text-lg font-semibold">No created filters</h4>
							<p class="text-on-surface1 text-sm font-light">
								Looks like you don't have any filters created yet. <br />
								Click the "Add New Filter" button above to get started.
							</p>
						</div>
					{:else}
						<Table
							data={filteredFilters}
							fields={['name', 'url', 'selectors']}
							onClickRow={(d, isCtrlClick) => {
								setSearchParamsToLocalStorage(page.url.pathname, page.url.search);

								const url = `/admin/filters/${d.id}`;
								openUrl(url, isCtrlClick);
							}}
							filterable={['name', 'url']}
							filters={urlFilters}
							onFilter={setFilterUrlParams}
							onClearAllFilters={clearUrlParams}
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
							sortable={['name']}
							onSort={setSortUrlParams}
							{initSort}
						>
							{#snippet actions(d: MCPFilter)}
								{#if !profile.current.isAdminReadonly?.()}
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
								{/if}
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
			</div>
		{/if}
	</div>

	{#snippet rightNavActions()}
		{#if loading}
			<LoaderCircle class="size-4 animate-spin" />
		{/if}
		{#if !profile.current.isAdminReadonly?.()}
			{@render addFilterButton()}
		{/if}
	{/snippet}
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
