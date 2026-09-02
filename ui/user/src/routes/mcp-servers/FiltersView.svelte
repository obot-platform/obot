<script lang="ts">
	import { page } from '$app/state';
	import Confirm from '$lib/components/Confirm.svelte';
	import ResponsiveDialog from '$lib/components/ResponsiveDialog.svelte';
	import Search from '$lib/components/Search.svelte';
	import Skeleton from '$lib/components/Skeleton.svelte';
	import FilterForm from '$lib/components/admin/FilterForm.svelte';
	import IconButton from '$lib/components/primitives/IconButton.svelte';
	import Table from '$lib/components/table/Table.svelte';
	import { AdminService, type MCPFilter, type SystemMCPServerCatalogEntry } from '$lib/services';
	import { profile } from '$lib/stores';
	import {
		clearUrlParams,
		getTableUrlParamsFilters,
		getTableUrlParamsSort,
		goto,
		replaceState,
		setFilterUrlParams,
		setSortUrlParams
	} from '$lib/url';
	import { openUrl } from '$lib/utils';
	import BuiltInFilters from './BuiltInFilters.svelte';
	import { Funnel, Trash2 } from '@lucide/svelte';
	import { untrack } from 'svelte';

	interface Props {
		filters?: MCPFilter[];
		systemCatalogEntries?: SystemMCPServerCatalogEntry[];
		loading?: boolean;
	}

	let {
		filters = $bindable([]),
		systemCatalogEntries = [],
		loading = $bindable(false)
	}: Props = $props();

	let showCreateFilter = $derived(page.url.searchParams.has('new'));
	let filterToDelete = $state<MCPFilter>();
	let builtInFiltersDialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let query = $derived(page.url.searchParams.get('query') || '');
	let localFilters = $state<MCPFilter[]>(untrack(() => filters));

	let tableData = $derived(
		localFilters.map((filter) => ({
			...filter,
			status: filter.disabled ? 'Disabled' : 'Enabled'
		}))
	);
	let filteredTableData = $derived.by(() =>
		tableData.filter((filter) => filter.name?.toLowerCase().includes(query.toLowerCase()))
	);

	let urlFilters = $derived(getTableUrlParamsFilters());
	let initSort = $derived(getTableUrlParamsSort());

	function listUrl() {
		return `${page.url.pathname}?view=filters`;
	}

	async function refresh() {
		loading = true;
		localFilters = await AdminService.listMCPFilters();
		filters = localFilters;
		loading = false;
	}

	async function navigateAfterCreated() {
		goto(listUrl(), { replaceState: true });
		await refresh();
	}

	const updateQuery = (value: string) => {
		if (value) {
			page.url.searchParams.set('query', value);
		} else {
			page.url.searchParams.delete('query');
		}

		replaceState(page.url, { query: value });
	};

	export function openBuiltInPicker() {
		builtInFiltersDialog?.open();
	}
</script>

{#if showCreateFilter}
	<FilterForm onCreate={navigateAfterCreated} />
{:else if loading}
	<Skeleton
		type="table"
		count={10}
		classes={{ header: 'h-14 rounded-none', body: 'rounded-none' }}
	/>
{:else if localFilters.length === 0}
	<div class="mt-12 flex w-md flex-col items-center gap-4 self-center text-center">
		<Funnel class="text-muted-content size-24 opacity-50" />
		<h4 class="text-muted-content text-lg font-semibold">No created filters</h4>
		<p class="text-muted-content text-sm font-light">
			Looks like you don't have any filters created yet. <br />
			Click the "Add New Filter" button above to get started.
		</p>
	</div>
{:else}
	<div class="flex flex-col gap-2">
		<div class="bg-base-200 dark:bg-base-100 sticky top-16 left-0 z-20 w-full py-1">
			<Search
				value={query}
				class="dark:bg-base-200 dark:border-base-400 bg-base-100 border border-transparent shadow-sm"
				onChange={updateQuery}
				placeholder="Search filters..."
			/>
		</div>

		<Table
			data={filteredTableData}
			fields={['name', 'selectors', 'status']}
			onClickRow={(d, isCtrlClick) => {
				openUrl(`/mcp-servers/filters/${d.id}`, isCtrlClick);
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
					<IconButton
						variant="danger"
						onclick={(e) => {
							e.stopPropagation();
							filterToDelete = d;
						}}
						tooltip={{ text: 'Delete Filter' }}
					>
						<Trash2 class="size-4" />
					</IconButton>
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
							? 'text-muted-content font-light italic text-xs'
							: 'pill-primary bg-primary'}>{d.status}</span
					>
				{:else}
					-
				{/if}
			{/snippet}
		</Table>
	</div>
{/if}

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
	class="bg-base-200 dark:bg-base-100 md:max-w-dvw md:w-6xl"
	title="Select Built-in Filter"
	bind:this={builtInFiltersDialog}
>
	<BuiltInFilters
		{query}
		entries={systemCatalogEntries}
		onSelect={(d) => {
			goto(`/mcp-servers/filters/c/${d.id}`);
			builtInFiltersDialog?.close();
		}}
	/>
</ResponsiveDialog>
