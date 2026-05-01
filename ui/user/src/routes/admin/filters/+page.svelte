<script lang="ts">
	import { page } from '$app/state';
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import Confirm from '$lib/components/Confirm.svelte';
	import DotDotDot from '$lib/components/DotDotDot.svelte';
	import Layout from '$lib/components/Layout.svelte';
	import ResponsiveDialog from '$lib/components/ResponsiveDialog.svelte';
	import Search from '$lib/components/Search.svelte';
	import FilterForm from '$lib/components/admin/FilterForm.svelte';
	import Table from '$lib/components/table/Table.svelte';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants.js';
	import { AdminService, type MCPFilter, type SystemMCPServerCatalogEntry } from '$lib/services';
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
	import { Filter, LoaderCircle, Plus, Trash2 } from 'lucide-svelte';
	import { untrack } from 'svelte';
	import { fly } from 'svelte/transition';

	let showCreateFilter = $derived(page.url.searchParams.has('new'));
	let loading = $state(false);
	let filterToDelete = $state<MCPFilter>();
	let { data } = $props();

	let query = $derived(page.url.searchParams.get('query') || '');
	let filters = $state<MCPFilter[]>(untrack(() => data?.filters ?? []));
	let builtInFiltersDialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let systemCatalogEntries = $state<SystemMCPServerCatalogEntry[]>(
		untrack(() => data?.systemCatalogEntries ?? [])
	);

	let tableData = $derived(
		filters.map((filter) => ({
			...filter,
			status: filter.disabled ? 'Disabled' : 'Enabled'
		}))
	);
	let filteredTableData = $derived.by(() =>
		tableData.filter((filter) => filter.name?.toLowerCase().includes(query.toLowerCase()))
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
				{#if filters.length === 0}
					<div class="mt-12 flex w-md flex-col items-center gap-4 self-center text-center">
						<Filter class="text-on-surface1 size-24 opacity-50" />
						<h4 class="text-on-surface1 text-lg font-semibold">No created filters</h4>
						<p class="text-on-surface1 text-sm font-light">
							Looks like you don't have any filters created yet. <br />
							Click the "Add New Filter" button above to get started.
						</p>
					</div>
				{:else}
					<div class="flex flex-col gap-2">
						<Search
							value={query}
							class="dark:bg-surface1 dark:border-surface3 bg-background border border-transparent shadow-sm"
							onChange={updateQuery}
							placeholder="Search filters..."
						/>

						<Table
							data={filteredTableData}
							fields={['name', 'selectors', 'status']}
							onClickRow={(d, isCtrlClick) => {
								const url = `/admin/filters/${d.id}`;
								openUrl(url, isCtrlClick);
							}}
							filterable={['name', 'status']}
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
							sortable={['name', 'status']}
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
							{#snippet onRenderColumn(property, d: (typeof tableData)[number])}
								{#if property === 'name'}
									{d.name || '-'}
								{:else if property === 'url'}
									{d.url || '-'}
								{:else if property === 'selectors'}
									{@const count = d.selectors?.length || 0}
									{count > 0 ? `${count} selector${count > 1 ? 's' : ''}` : '-'}
								{:else if property === 'status'}
									<span
										class={d.status === 'Disabled'
											? 'text-on-surface1 font-light italic text-xs'
											: 'pill-primary bg-primary'}>{d.status}</span
									>
								{:else}
									-
								{/if}
							{/snippet}
						</Table>
					</div>
				{/if}
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
	<DotDotDot
		class="button-primary w-full text-sm md:w-fit"
		placement="bottom"
		classes={{ popover: 'z-50' }}
	>
		{#snippet icon()}
			<span class="flex items-center justify-center gap-1">
				<Plus class="size-4" /> Add New Filter
			</span>
		{/snippet}
		<button
			class="menu-button"
			onclick={() => {
				goto('/admin/filters?new=true');
			}}
		>
			Create Custom
		</button>
		<button
			class="menu-button"
			disabled={systemCatalogEntries.length === 0}
			onclick={() => {
				builtInFiltersDialog?.open();
			}}
		>
			Create From Built-in
		</button>
	</DotDotDot>
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

<ResponsiveDialog
	class="bg-surface1 dark:bg-background md:max-w-dvw md:w-6xl"
	title="Select Built-in Filter"
	bind:this={builtInFiltersDialog}
>
	<BuiltInFilters
		{query}
		entries={systemCatalogEntries}
		onSelect={(d) => {
			goto(`/admin/filters/c/${d.id}`);
			builtInFiltersDialog?.close();
		}}
	/>
</ResponsiveDialog>

<svelte:head>
	<title>Obot | Filters</title>
</svelte:head>
