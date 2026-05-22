<script lang="ts">
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import Layout from '$lib/components/Layout.svelte';
	import Search from '$lib/components/Search.svelte';
	import Pagination from '$lib/components/table/Pagination.svelte';
	import Table from '$lib/components/table/Table.svelte';
	import { PAGE_SIZE, PAGE_TRANSITION_DURATION } from '$lib/constants';
	import { AdminService } from '$lib/services/index.js';
	import { getTableUrlParamsSort, replaceState } from '$lib/url';
	import { getSortParams, openUrl } from '$lib/utils';
	import { defaultSort, sortFields } from './constants';
	import { MonitorCheck } from 'lucide-svelte';
	import { untrack } from 'svelte';
	import { fly } from 'svelte/transition';

	let { data } = $props();
	let clientsData = $state(untrack(() => data?.clients ?? { items: [], total: 0, offset: 0 }));
	let clients = $derived(clientsData.items ?? []);
	let total = $derived(clientsData.total ?? 0);
	let userMap = $derived(new Map(data?.users?.map((u) => [u.id, u]) ?? []));

	function isSortProperty(property: string | undefined): property is keyof typeof sortFields {
		return property != null && Object.hasOwn(sortFields, property);
	}

	let initSort = $derived.by(() => {
		const sort = getTableUrlParamsSort(defaultSort);
		return isSortProperty(sort?.property) ? sort : defaultSort;
	});
	let rows = $derived(
		clients.map((c) => ({
			id: c.name ?? '',
			name: c.name ?? '',
			mcpServers: c.mcpServers ?? [],
			mcpServerCount: c.mcpServers?.length ?? 0,
			skills: c.skills ?? [],
			skillCount: c.skills?.length ?? 0,
			users:
				c.users?.map((u) => userMap.get(u) ?? { id: u, displayName: u, email: u, username: u }) ??
				[],
			userCount: c.users?.length ?? 0
		}))
	);
	let pageSize = $derived(
		parseInt(page.url.searchParams.get('pageSize') ?? String(PAGE_SIZE), 10) ?? PAGE_SIZE
	);
	let pageIndex = $derived(Math.floor((clientsData.offset ?? 0) / pageSize));
	let lastPageIndex = $derived(total > 0 ? Math.ceil(total / pageSize) - 1 : 0);
	let nameFilter = $state(untrack(() => page.url.searchParams.get('name') ?? ''));
	let loading = $state(false);

	function syncUrl(nextPageIndex: number, sort = initSort) {
		const next = new URL(page.url);
		if (nameFilter) next.searchParams.set('name', nameFilter);
		else next.searchParams.delete('name');
		if (nextPageIndex > 0) next.searchParams.set('offset', String(nextPageIndex * pageSize));
		else next.searchParams.delete('offset');
		if (isSortProperty(sort?.property)) {
			next.searchParams.set('sort', sort.property);
			next.searchParams.set('sortDirection', sort.order);
		}
		replaceState(next, {});
	}

	async function reload(idx: number, sort = initSort) {
		loading = true;
		try {
			clientsData = await AdminService.listDeviceClients({
				limit: pageSize,
				offset: idx * pageSize,
				name: nameFilter,
				...getSortParams(sort, sortFields, defaultSort)
			});
		} finally {
			loading = false;
		}
	}

	function updateName(value: string) {
		nameFilter = value;
		syncUrl(0);
		reload(0);
	}

	function fetchPage(idx: number) {
		syncUrl(idx);
		reload(idx);
	}

	function handleSort(property: string, order: 'asc' | 'desc') {
		const sort = { property, order };
		syncUrl(0, sort);
		reload(0, sort);
	}

	const duration = PAGE_TRANSITION_DURATION;
</script>

<svelte:head>
	<title>Obot | Device Clients</title>
</svelte:head>

<Layout title="Device Clients">
	<div
		class="flex h-full w-full flex-col gap-4"
		in:fly={{ x: 100, duration, delay: duration }}
		out:fly={{ x: -100, duration }}
	>
		<Search
			value={nameFilter}
			class="dark:bg-base-200 dark:border-base-400 bg-base-100 border border-transparent shadow-sm"
			onChange={updateName}
			placeholder="Search by client name..."
		/>

		{#if clients.length === 0}
			<div class="mx-auto mt-12 flex w-md flex-col items-center gap-4 text-center">
				<MonitorCheck class="text-muted-content size-24 opacity-50" />
				<h4 class="text-muted-content text-lg font-semibold">No clients observed yet</h4>
				<p class="text-muted-content text-sm font-light">
					Run <code class="font-mono">obot scan</code> from a managed device with clients to populate
					this view.
				</p>
			</div>
		{:else}
			<Table
				data={rows}
				{pageSize}
				fields={['name', 'mcpServerCount', 'skillCount', 'userCount']}
				headers={[
					{ title: 'Name', property: 'name' },
					{ title: 'MCP Servers', property: 'mcpServerCount' },
					{ title: 'Skills', property: 'skillCount' },
					{ title: 'Users', property: 'userCount' }
				]}
				sortable={['name', 'mcpServerCount', 'skillCount', 'userCount']}
				{initSort}
				onSort={handleSort}
				onClickRow={(d, isCtrlClick) => {
					openUrl(resolve(`/admin/device-clients/${encodeURIComponent(d.name)}`), isCtrlClick);
				}}
			>
				{#snippet onRenderColumn(property, d)}
					{#if property === 'name'}
						{#if d.name?.trim()}
							{d.name.trim()}
						{:else}
							<span class="text-muted-content italic">(unnamed)</span>
						{/if}
					{:else}
						{d[property as keyof (typeof rows)[number]]}
					{/if}
				{/snippet}
			</Table>
		{/if}
		{#if total > pageSize}
			<Pagination
				{pageIndex}
				{lastPageIndex}
				{total}
				itemLabelSingular="client"
				{loading}
				onPageChange={fetchPage}
			/>
		{/if}
	</div>
</Layout>
