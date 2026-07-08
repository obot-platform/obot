<script lang="ts">
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import Search from '$lib/components/Search.svelte';
	import Skeleton from '$lib/components/Skeleton.svelte';
	import Pagination from '$lib/components/table/Pagination.svelte';
	import Table from '$lib/components/table/Table.svelte';
	import { PAGE_SIZE } from '$lib/constants';
	import {
		AdminService,
		UserService,
		type DeviceClientFleetSummaryResponse,
		type OrgUser
	} from '$lib/services';
	import { getTableUrlParamsSort, replaceState } from '$lib/url';
	import { getSortParams, openUrl } from '$lib/utils';
	import { defaultClientSort, deviceClientSortFields } from './constants';
	import { MonitorCheck } from '@lucide/svelte';
	import { onMount, untrack } from 'svelte';

	let users = $state<OrgUser[]>([]);
	let clientsData = $state<DeviceClientFleetSummaryResponse>({
		items: [],
		total: 0,
		offset: 0,
		limit: 0
	});
	let clients = $derived(clientsData.items ?? []);
	let total = $derived(clientsData.total ?? 0);
	let userMap = $derived(new Map(users?.map((u) => [u.id, u]) ?? []));

	function isSortProperty(
		property: string | undefined
	): property is keyof typeof deviceClientSortFields {
		return property != null && Object.hasOwn(deviceClientSortFields, property);
	}

	let initSort = $derived.by(() => {
		const sort = getTableUrlParamsSort(defaultClientSort);
		return isSortProperty(sort?.property) ? sort : defaultClientSort;
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
	let loading = $state(true);

	onMount(async () => {
		UserService.listUsers().then((response) => {
			users = response;
		});
		reload(0);
	});

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
				...getSortParams(sort, deviceClientSortFields, defaultClientSort)
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
</script>

<Search
	value={nameFilter}
	class="dark:bg-base-200 dark:border-base-400 bg-base-100 border border-transparent shadow-sm"
	onChange={updateName}
	placeholder="Search by client name..."
/>

{#if loading}
	<Skeleton type="table" />
{:else if clients.length === 0}
	<div class="mx-auto mt-12 flex w-md flex-col items-center gap-4 text-center">
		<MonitorCheck class="text-muted-content size-24 opacity-50" />
		<h4 class="text-muted-content text-lg font-semibold">No clients observed yet</h4>
		<p class="text-muted-content text-sm font-light">
			Run <code class="font-mono">obot scan</code> from a managed device with clients to populate this
			view.
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
