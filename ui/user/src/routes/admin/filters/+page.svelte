<script lang="ts">
	import { page } from '$app/state';
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import Confirm from '$lib/components/Confirm.svelte';
	import Layout from '$lib/components/Layout.svelte';
	import Search from '$lib/components/Search.svelte';
	import FilterForm from '$lib/components/admin/FilterForm.svelte';
	import Table from '$lib/components/table/Table.svelte';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants.js';
	import { AdminService, type MCPFilter } from '$lib/services/index.js';
	import { profile } from '$lib/stores';
	import { replaceState } from '$lib/url';
	import {
		clearUrlParams,
		getTableUrlParamsFilters,
		getTableUrlParamsSort,
		setSortUrlParams,
		setFilterUrlParams,
		goto
	} from '$lib/url';
	import { openUrl } from '$lib/utils';
	import BuiltInFilters from './BuiltInFilters.svelte';
	import { debounce } from 'es-toolkit';
	import { BookOpenText, LoaderCircle, Plus, Trash2 } from 'lucide-svelte';
	import { untrack } from 'svelte';
	import { fly } from 'svelte/transition';

	let showCreateFilter = $derived(page.url.searchParams.has('new'));
	let loading = $state(false);
	let filterToDelete = $state<MCPFilter>();
	let { data } = $props();

	let query = $derived(page.url.searchParams.get('query') || '');
	let filters = $state<MCPFilter[]>(untrack(() => data?.filters ?? []));
	let { myFilters, connectedBuiltInFilters } = $derived(
		filters.reduce<{ myFilters: MCPFilter[]; connectedBuiltInFilters: MCPFilter[] }>(
			(acc, filter) => {
				if (filter.systemMCPServerCatalogEntryID) {
					acc.connectedBuiltInFilters.push(filter);
				} else {
					acc.myFilters.push(filter);
				}
				return acc;
			},
			{ myFilters: [], connectedBuiltInFilters: [] }
		)
	);
	let filteredMyFilters = $derived.by(() =>
		myFilters.filter((filter) => filter.name?.toLowerCase().includes(query.toLowerCase()))
	);

	let urlFilters = $derived(getTableUrlParamsFilters());
	let initSort = $derived(getTableUrlParamsSort());

	async function refresh() {
		loading = true;
		filters = await AdminService.listMCPFilters();
		loading = false;
	}

	async function navigateAfterCreated() {
		goto('/admin/filters', { replaceState: true });
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

	function handleFilter(property: string, values: string[]) {
		if (values.length === 0) {
			delete urlFilters[property];
			urlFilters = { ...urlFilters };
		} else {
			urlFilters[property] = values;
		}
		setFilterUrlParams(property, values);
	}

	function handleClearAllFilters() {
		urlFilters = {};
		clearUrlParams();
	}

	async function handleRefresh() {
		filters = await AdminService.listMCPFilters();
	}

	const duration = PAGE_TRANSITION_DURATION;
</script>

<Layout
	title="Filters"
	showBackButton={showCreateFilter}
	onBackButtonClick={() => {
		goto('/admin/filters', { replaceState: true });
	}}
>
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
				<Search
					value={query}
					class="dark:bg-surface1 dark:border-surface3 bg-background border border-transparent shadow-sm"
					onChange={updateQuery}
					placeholder="Search filters..."
				/>

				<div class="flex flex-col gap-2">
					<div class="flex flex-col gap-2">
						<h4 class="text-lg font-semibold">My Filters</h4>
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
								data={filteredMyFilters}
								fields={['name', 'selectors']}
								onClickRow={(d, isCtrlClick) => {
									const url = `/admin/filters/${d.id}`;
									openUrl(url, isCtrlClick);
								}}
								filterable={['name']}
								filters={urlFilters}
								onFilter={setFilterUrlParams}
								onClearAllFilters={clearUrlParams}
								headers={[
									{
										title: 'Name',
										property: 'name'
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

				<div class="flex flex-col gap-2">
					<h4 class="text-lg font-semibold">Built-in Filters</h4>
					<BuiltInFilters
						{query}
						connectedFilters={connectedBuiltInFilters}
						{urlFilters}
						onFilter={handleFilter}
						onClearAllFilters={handleClearAllFilters}
						onRefresh={handleRefresh}
						onSort={setSortUrlParams}
						{initSort}
					/>
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
		onclick={() => {
			goto('/admin/filters?new=true');
		}}
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
		<FilterForm onCreate={navigateAfterCreated} />
	</div>
{/snippet}

<Confirm
	msg={`Delete ${filterToDelete?.name || 'this filter'}?`}
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
