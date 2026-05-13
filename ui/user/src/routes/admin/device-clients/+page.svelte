<script lang="ts">
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import Layout from '$lib/components/Layout.svelte';
	import Search from '$lib/components/Search.svelte';
	import Pagination from '$lib/components/table/Pagination.svelte';
	import Table from '$lib/components/table/Table.svelte';
	import { PAGE_SIZE, PAGE_TRANSITION_DURATION } from '$lib/constants';
	import { AdminService } from '$lib/services/index.js';
	import { setFilterUrlParams, setUrlParam } from '$lib/url';
	import { openUrl } from '$lib/utils';
	import { MonitorCheck } from 'lucide-svelte';
	import { untrack } from 'svelte';
	import { fly } from 'svelte/transition';

	let { data } = $props();
	let clientsData = $state(untrack(() => data?.clients ?? { items: [], total: 0, offset: 0 }));
	let clients = $derived(clientsData.items ?? []);
	let total = $derived(clientsData.total ?? 0);
	let userMap = $derived(new Map(data?.users?.map((u) => [u.id, u]) ?? []));
	let rows = $derived(
		clients.map((c) => ({
			id: c.name ?? '',
			name: c.name ?? '',
			mcpServers: c.mcpServers ?? [],
			skills: c.skills ?? [],
			users:
				c.users?.map((u) => userMap.get(u) ?? { id: u, displayName: u, email: u, username: u }) ??
				[]
		}))
	);
	let pageSize = $derived(
		parseInt(page.url.searchParams.get('pageSize') ?? String(PAGE_SIZE), 10) ?? PAGE_SIZE
	);
	let pageIndex = $derived(Math.floor((clientsData.offset ?? 0) / pageSize));
	let lastPageIndex = $derived(total > 0 ? Math.ceil(total / pageSize) - 1 : 0);
	let nameFilter = $state(untrack(() => page.url.searchParams.get('name') ?? ''));
	let loading = $state(false);

	let filteredRows = $derived(
		nameFilter ? rows.filter((c) => c.name.toLowerCase().includes(nameFilter.toLowerCase())) : rows
	);

	async function reload(idx: number) {
		loading = true;
		try {
			clientsData = await AdminService.listDeviceClients({
				limit: pageSize,
				offset: idx * pageSize,
				name: nameFilter
			});
		} finally {
			loading = false;
		}
	}

	function updateName(value: string) {
		nameFilter = value;
		setFilterUrlParams('name', value ? [value] : []);
		reload(0);
	}

	function fetchPage(idx: number) {
		setUrlParam(page.url, 'name', nameFilter);
		setFilterUrlParams('offset', idx > 0 ? [String(idx * pageSize)] : []);
		reload(idx);
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
			class="dark:bg-surface1 dark:border-surface3 bg-background border border-transparent shadow-sm"
			onChange={updateName}
			placeholder="Search by client name..."
		/>

		{#if clients.length === 0}
			<div class="mx-auto mt-12 flex w-md flex-col items-center gap-4 text-center">
				<MonitorCheck class="text-on-surface1 size-24 opacity-50" />
				<h4 class="text-on-surface1 text-lg font-semibold">No clients observed yet</h4>
				<p class="text-on-surface1 text-sm font-light">
					Run <code class="font-mono">obot scan</code> from a managed device with clients to populate
					this view.
				</p>
			</div>
		{:else}
			<Table
				data={filteredRows}
				{pageSize}
				fields={['name', 'mcpServers', 'skills', 'users']}
				headers={[
					{ title: 'Name', property: 'name' },
					{ title: 'MCP Servers', property: 'mcpServers' },
					{ title: 'Skills', property: 'skills' },
					{ title: 'Users', property: 'users' }
				]}
				onClickRow={(d, isCtrlClick) => {
					openUrl(resolve(`/admin/device-clients/${encodeURIComponent(d.name)}`), isCtrlClick);
				}}
			>
				{#snippet onRenderColumn(property, d)}
					{#if property === 'name'}
						{#if d.name?.trim()}
							<span class="font-medium">{d.name}</span>
						{:else}
							<span class="text-on-surface2 italic">(unnamed)</span>
						{/if}
					{:else if property === 'skills'}
						{d.skills.length}
					{:else if property === 'mcpServers'}
						{d.mcpServers.length}
					{:else if property === 'users'}
						{d.users.length}
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
