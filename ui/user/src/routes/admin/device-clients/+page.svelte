<script lang="ts">
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import Layout from '$lib/components/Layout.svelte';
	import Search from '$lib/components/Search.svelte';
	import Table from '$lib/components/table/Table.svelte';
	import { PAGE_SIZE, PAGE_TRANSITION_DURATION } from '$lib/constants';
	import { setFilterUrlParams } from '$lib/url';
	import { openUrl } from '$lib/utils';
	import { type DeviceClient } from './utils';
	import { Server } from 'lucide-svelte';
	import { untrack } from 'svelte';
	import { fly } from 'svelte/transition';

	let { data } = $props();
	let clients = $derived(data?.clients ?? []);

	let nameFilter = $state(untrack(() => page.url.searchParams.get('name') ?? ''));

	let rows = $derived<DeviceClient[]>(
		nameFilter
			? clients.filter((c) => c.name.toLowerCase().includes(nameFilter.toLowerCase()))
			: clients
	);

	$effect(() => {
		console.log(clients);
	});

	function updateName(value: string) {
		nameFilter = value;
		setFilterUrlParams('name', value ? [value] : []);
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
			placeholder="Search by server name..."
		/>

		{#if clients.length === 0}
			<div class="mx-auto mt-12 flex w-md flex-col items-center gap-4 text-center">
				<Server class="text-on-surface1 size-24 opacity-50" />
				<h4 class="text-on-surface1 text-lg font-semibold">No clients observed yet</h4>
				<p class="text-on-surface1 text-sm font-light">
					Run <code class="font-mono">obot scan</code> from a managed device with clients to populate
					this view.
				</p>
			</div>
		{:else}
			<Table
				data={rows}
				pageSize={PAGE_SIZE}
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
						{d[property as keyof DeviceClient]}
					{/if}
				{/snippet}
			</Table>
		{/if}
	</div>
</Layout>
