<script lang="ts">
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import Search from '$lib/components/Search.svelte';
	import Table from '$lib/components/table/Table.svelte';
	import { PAGE_SIZE } from '$lib/constants';
	import { AdminService, type DeviceMCPServerStat, type DeviceScanStats } from '$lib/services';
	import {
		clearUrlParams,
		getTableUrlParamsFilters,
		getTableUrlParamsSort,
		replaceState,
		setFilterUrlParams,
		setSortUrlParams
	} from '$lib/url';
	import { openUrl } from '$lib/utils';
	import { Server } from '@lucide/svelte';
	import { untrack } from 'svelte';

	type Row = DeviceMCPServerStat & { id: string };

	let nameFilter = $state(untrack(() => page.url.searchParams.get('query') ?? ''));
	let urlFilters = $derived(getTableUrlParamsFilters());
	let initSort = $derived(getTableUrlParamsSort({ property: 'deviceCount', order: 'desc' }));
	let timeFilter = $derived({
		start: page.url.searchParams.get('start') ?? undefined,
		end: page.url.searchParams.get('end') ?? undefined
	});

	let loading = $state(false);
	let stats = $state<DeviceScanStats>();

	$effect(() => {
		loading = true;
		AdminService.getDeviceScanStats(timeFilter)
			.then((response) => {
				stats = response;
			})
			.finally(() => {
				loading = false;
			});
	});

	let allRows = $derived<Row[]>(
		(stats?.mcpServers ?? []).map((s) => ({
			...s,
			id: s.configHash
		}))
	);

	let rows = $derived<Row[]>(
		nameFilter
			? allRows.filter((r) => r.name.toLowerCase().includes(nameFilter.toLowerCase()))
			: allRows
	);

	function updateName(value: string) {
		nameFilter = value;
		const next = new URL(page.url);
		if (value) next.searchParams.set('query', value);
		else next.searchParams.delete('query');
		replaceState(next, {});
	}

	$effect(() => {});
</script>

<Search
	value={nameFilter}
	class="dark:bg-base-200 dark:border-base-400 bg-base-100 border border-transparent shadow-sm"
	onChange={updateName}
	placeholder="Search by server name..."
/>

{#if loading}
	<div class="flex flex-col gap-0.5">
		<div class="skeleton h-9 w-full rounded-none"></div>
		{#each Array.from({ length: 4 }) as _, i (i)}
			<div class="skeleton h-14 w-full rounded-none"></div>
		{/each}
	</div>
{:else if allRows.length === 0}
	<div class="mx-auto mt-12 flex w-md flex-col items-center gap-4 text-center">
		<Server class="text-muted-content size-24 opacity-50" />
		<h4 class="text-muted-content text-lg font-semibold">No MCP servers observed yet</h4>
		<p class="text-muted-content text-sm font-light">
			Run <code class="font-mono">obot scan</code> from a managed device with configured MCP servers to
			populate this view.
		</p>
	</div>
{:else}
	<Table
		data={rows}
		pageSize={PAGE_SIZE}
		fields={['name', 'transport', 'deviceCount', 'userCount', 'observationCount']}
		headers={[
			{ title: 'Name', property: 'name' },
			{ title: 'Transport', property: 'transport' },
			{ title: 'Devices', property: 'deviceCount' },
			{ title: 'Users', property: 'userCount' },
			{ title: 'Observations', property: 'observationCount' }
		]}
		sortable={['name', 'transport', 'deviceCount', 'userCount', 'observationCount']}
		filterable={['name', 'transport']}
		filters={urlFilters}
		{initSort}
		onSort={setSortUrlParams}
		onFilter={setFilterUrlParams}
		onClearAllFilters={clearUrlParams}
		onClickRow={(d, isCtrlClick) => {
			openUrl(
				resolve(`/admin/device-mcp-servers/${encodeURIComponent(d.configHash)}`),
				isCtrlClick
			);
		}}
	>
		{#snippet onRenderColumn(property, d: Row)}
			{#if property === 'name'}
				{#if d.name?.trim()}
					{d.name.trim()}
				{:else}
					<span class="text-muted-content italic">(unnamed)</span>
				{/if}
			{:else if property === 'transport'}
				<span class="pill-primary bg-primary text-xs">{d.transport}</span>
			{:else}
				{d[property as keyof Row]}
			{/if}
		{/snippet}
	</Table>
{/if}
