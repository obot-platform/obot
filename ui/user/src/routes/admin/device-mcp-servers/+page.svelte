<script lang="ts">
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import Layout from '$lib/components/Layout.svelte';
	import Search from '$lib/components/Search.svelte';
	import Table from '$lib/components/table/Table.svelte';
	import { PAGE_SIZE, PAGE_TRANSITION_DURATION } from '$lib/constants';
	import { type DeviceMCPServerStat } from '$lib/services';
	import { setFilterUrlParams } from '$lib/url';
	import { openUrl } from '$lib/utils';
	import { Server } from 'lucide-svelte';
	import { untrack } from 'svelte';
	import { fly } from 'svelte/transition';

	let { data } = $props();

	let nameFilter = $state(untrack(() => page.url.searchParams.get('name') ?? ''));

	type Row = DeviceMCPServerStat & { id: string };

	let allRows = $derived<Row[]>(
		(data?.stats?.mcpServers ?? []).map((s) => ({
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
		setFilterUrlParams('name', value ? [value] : []);
	}

	const duration = PAGE_TRANSITION_DURATION;
</script>

<svelte:head>
	<title>Obot | Device MCP Servers</title>
</svelte:head>

<Layout title="Device MCP Servers">
	<div
		class="flex h-full w-full flex-col gap-4"
		in:fly={{ x: 100, duration, delay: duration }}
		out:fly={{ x: -100, duration }}
	>
		<Search
			value={nameFilter}
			class="dark:bg-base-200 dark:border-base-400 bg-base-100 border border-transparent shadow-sm"
			onChange={updateName}
			placeholder="Search by server name..."
		/>

		{#if allRows.length === 0}
			<div class="mx-auto mt-12 flex w-md flex-col items-center gap-4 text-center">
				<Server class="text-muted-content size-24 opacity-50" />
				<h4 class="text-muted-content text-lg font-semibold">No MCP servers observed yet</h4>
				<p class="text-muted-content text-sm font-light">
					Run <code class="font-mono">obot scan</code> from a managed device with configured MCP servers
					to populate this view.
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
							<span class="font-medium">{d.name}</span>
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
	</div>
</Layout>
