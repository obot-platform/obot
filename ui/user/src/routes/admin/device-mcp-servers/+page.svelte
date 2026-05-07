<script lang="ts">
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import Layout from '$lib/components/Layout.svelte';
	import Search from '$lib/components/Search.svelte';
	import Table from '$lib/components/table/Table.svelte';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants';
	import { type DeviceMCPServerStat } from '$lib/services';
	import { setFilterUrlParams } from '$lib/url';
	import { openUrl } from '$lib/utils';
	import { Server } from 'lucide-svelte';
	import { fly } from 'svelte/transition';

	let { data } = $props();

	let nameFilter = $derived(page.url.searchParams.get('name') ?? '');

	type Row = DeviceMCPServerStat & { id: string };

	let allRows = $derived<Row[]>(
		(data?.stats?.mcp_servers ?? []).map((s) => ({
			...s,
			id: s.config_hash
		}))
	);

	let rows = $derived<Row[]>(
		nameFilter
			? allRows.filter((r) => r.name.toLowerCase().includes(nameFilter.toLowerCase()))
			: allRows
	);

	function updateName(value: string) {
		setFilterUrlParams('name', value ? [value] : []);
	}

	const duration = PAGE_TRANSITION_DURATION;
</script>

<svelte:head>
	<title>Obot | MCP Servers</title>
</svelte:head>

<Layout title="MCP Servers">
	<div
		class="flex h-full w-full flex-col gap-4"
		in:fly={{ x: 100, duration, delay: duration }}
		out:fly={{ x: -100, duration }}
	>
		<Search
			value={nameFilter}
			class="dark:bg-surface1 dark:border-surface3 bg-background border border-transparent shadow-sm"
			onChange={updateName}
			placeholder="Search by server name..."
		/>

		{#if allRows.length === 0}
			<div class="mx-auto mt-12 flex w-md flex-col items-center gap-4 text-center">
				<Server class="text-on-surface1 size-24 opacity-50" />
				<h4 class="text-on-surface1 text-lg font-semibold">No MCP servers observed yet</h4>
				<p class="text-on-surface1 text-sm font-light">
					Run <code class="font-mono">obot scan</code> from a managed device with configured MCP servers
					to populate this view.
				</p>
			</div>
		{:else}
			<Table
				data={rows}
				fields={['name', 'transport', 'device_count', 'user_count', 'observation_count']}
				headers={[
					{ title: 'Name', property: 'name' },
					{ title: 'Transport', property: 'transport' },
					{ title: 'Devices', property: 'device_count' },
					{ title: 'Users', property: 'user_count' },
					{ title: 'Observations', property: 'observation_count' }
				]}
				onClickRow={(d, isCtrlClick) => {
					openUrl(
						resolve(`/admin/device-mcp-servers/${encodeURIComponent(d.config_hash)}`),
						isCtrlClick
					);
				}}
			>
				{#snippet onRenderColumn(property, d: Row)}
					{#if property === 'name'}
						{#if d.name?.trim()}
							<span class="font-medium">{d.name}</span>
						{:else}
							<span class="text-on-surface2 italic">(unnamed)</span>
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
