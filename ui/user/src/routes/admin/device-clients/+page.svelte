<script lang="ts">
	import { page } from '$app/state';
	import Layout from '$lib/components/Layout.svelte';
	import Search from '$lib/components/Search.svelte';
	import Table from '$lib/components/table/Table.svelte';
	import { PAGE_SIZE, PAGE_TRANSITION_DURATION } from '$lib/constants';
	import { setFilterUrlParams } from '$lib/url';
	import { compileDeviceClients } from './utils';
	import { Server } from 'lucide-svelte';
	import { untrack } from 'svelte';
	import { fly } from 'svelte/transition';

	let { data } = $props();
	let clientsMap = $derived(compileDeviceClients(data?.devices?.items ?? []));
	let clients = $derived(Array.from(clientsMap.values()));

	let nameFilter = $state(untrack(() => page.url.searchParams.get('name') ?? ''));

	let rows = $derived<ReturnType<typeof compileDeviceClients>>(
		nameFilter
			? clients.filter((c) => c.name.toLowerCase().includes(nameFilter.toLowerCase()))
			: clients
	);

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
				fields={['name', 'skills', 'mcpServers', 'userIds']}
				headers={[
					{ title: 'Name', property: 'name' },
					{ title: 'Skills', property: 'skills' },
					{ title: 'MCP Servers', property: 'mcpServers' },
					{ title: 'Users', property: 'userIds' }
				]}
				onClickRow={(_d, _isCtrlClick) => {
					// todo: since we don't have a dedicated /device/client/[name] endpoint,
					// have device content show on form from this route
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
					{:else if property === 'userIds'}
						{d.userIds.length}
					{:else}
						{d[property as keyof DeviceClient]}
					{/if}
				{/snippet}
			</Table>
		{/if}
	</div>
</Layout>
