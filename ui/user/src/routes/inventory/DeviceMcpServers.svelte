<script lang="ts">
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import Search from '$lib/components/Search.svelte';
	import Skeleton from '$lib/components/Skeleton.svelte';
	import Table from '$lib/components/table/Table.svelte';
	import { PAGE_SIZE } from '$lib/constants';
	import { m } from '$lib/i18n';
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
	let urlFilters = $derived.by(() => {
		const f = getTableUrlParamsFilters();
		delete f.start;
		delete f.end;
		delete f.offset;
		return f;
	});
	let initSort = $derived(getTableUrlParamsSort({ property: 'deviceCount', order: 'desc' }));
	let timeFilter = $derived({
		start: page.url.searchParams.get('start') ?? undefined,
		end: page.url.searchParams.get('end') ?? undefined
	});

	let loading = $state(true);
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
</script>

<Search
	value={nameFilter}
	class="dark:bg-base-200 dark:border-base-400 bg-base-100 border border-transparent shadow-sm"
	onChange={updateName}
	placeholder={m.inventory_enforcement_device_mcp_servers_search_servers()}
/>

{#if loading}
	<Skeleton type="table" />
{:else if allRows.length === 0}
	<div class="mx-auto mt-12 flex w-md flex-col items-center gap-4 text-center">
		<Server class="text-muted-content size-24 opacity-50" />
		<h4 class="text-muted-content text-lg font-semibold">
			{m.inventory_enforcement_device_mcp_servers_no_servers_title()}
		</h4>
		<p class="text-muted-content text-sm font-light">
			{m.inventory_enforcement_device_mcp_servers_no_servers_prefix()}<code class="font-mono"
				>obot scan</code
			>{m.inventory_enforcement_device_mcp_servers_no_servers_suffix()}
		</p>
	</div>
{:else}
	<Table
		data={rows}
		pageSize={PAGE_SIZE}
		fields={['name', 'transport', 'deviceCount', 'userCount', 'observationCount']}
		headers={[
			{ title: m.core_name(), property: 'name' },
			{ title: m.inventory_enforcement_col_transport(), property: 'transport' },
			{ title: m.inventory_enforcement_devices_tab(), property: 'deviceCount' },
			{ title: m.inventory_enforcement_col_users(), property: 'userCount' },
			{ title: m.inventory_enforcement_col_observations(), property: 'observationCount' }
		]}
		sortable={['name', 'transport', 'deviceCount', 'userCount', 'observationCount']}
		filterable={['name', 'transport']}
		filters={urlFilters}
		{initSort}
		onSort={setSortUrlParams}
		onFilter={setFilterUrlParams}
		onClearAllFilters={() => clearUrlParams(['name', 'transport'])}
		onClickRow={(d, isCtrlClick) => {
			openUrl(resolve(`/inventory/mcp-servers/${encodeURIComponent(d.configHash)}`), isCtrlClick);
		}}
	>
		{#snippet onRenderColumn(property, d: Row)}
			{#if property === 'name'}
				{#if d.name?.trim()}
					{d.name.trim()}
				{:else}
					<span class="text-muted-content italic">{m.inventory_enforcement_unnamed()}</span>
				{/if}
			{:else if property === 'transport'}
				<span class="pill-primary bg-primary text-xs">{d.transport}</span>
			{:else}
				{d[property as keyof Row]}
			{/if}
		{/snippet}
	</Table>
{/if}
