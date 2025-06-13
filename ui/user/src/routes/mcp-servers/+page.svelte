<script lang="ts">
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import Layout from '$lib/components/Layout.svelte';
	import McpCatalog from '$lib/components/mcp/McpCatalog.svelte';
	import Table from '$lib/components/Table.svelte';
	import { initToolReferences } from '$lib/context/toolReferences.svelte';
	import { formatTimeAgo } from '$lib/time';
	import { Plus, Trash2, Unplug } from 'lucide-svelte';
	import { fade } from 'svelte/transition';

	initToolReferences([]);

	const mockMyServers: {
		id: number;
		name: string;
		type: 'hosted' | 'remote';
		lastConnected: string;
	}[] = [
		{
			id: 1,
			name: 'My Github Server',
			type: 'hosted',
			lastConnected: formatTimeAgo('2025-06-13T12:00:00Z').relativeTime
		},
		{
			id: 2,
			name: 'My-Docs-1',
			type: 'remote',
			lastConnected: formatTimeAgo('2025-06-13T12:00:00Z').relativeTime
		}
	];

	const mockAvailableServers: (typeof mockMyServers)[0][] = [
		{
			id: 3,
			name: 'Engineering Firecrawl',
			type: 'hosted',
			lastConnected: formatTimeAgo('2025-06-13T12:00:00Z').relativeTime
		},
		{
			id: 4,
			name: 'My-Docs-2',
			type: 'remote',
			lastConnected: formatTimeAgo('2025-06-13T12:00:00Z').relativeTime
		}
	];

	let mcpCatalog = $state<ReturnType<typeof McpCatalog>>();
	let selectedMcpIds = $derived(mockMyServers.map((s) => s.id.toString()));
</script>

<Layout>
	<div class="my-8 flex flex-col gap-8" in:fade>
		<h1 class="text-2xl font-semibold">MCP Servers</h1>
		<div class="flex flex-col gap-2">
			<div class="mb-2 flex items-center justify-between">
				<h2 class="text-lg font-semibold">My MCP Servers</h2>
				<button
					class="button-primary flex items-center gap-1 text-sm"
					onclick={() => {
						mcpCatalog?.open();
					}}
				>
					<Plus class="size-6" /> Add New Server
				</button>
			</div>
			<Table data={mockMyServers} fields={['name', 'type', 'lastConnected']}>
				{#snippet actions(d)}
					<button
						class="icon-button hover:text-blue-500"
						onclick={() => {
							console.log(d);
						}}
						use:tooltip={'Connect'}
					>
						<Unplug class="size-4" />
					</button>
					<button class="icon-button hover:text-red-500" onclick={() => {}}>
						<Trash2 class="size-4" />
					</button>
				{/snippet}
			</Table>
		</div>

		<div class="flex flex-col gap-2">
			<h2 class="mb-2 text-lg font-semibold">Available MCP Servers</h2>
			<Table
				data={mockAvailableServers}
				fields={['name', 'type', 'lastConnected']}
				headers={[{ property: 'lastConnected', title: 'Last Connected' }]}
			>
				{#snippet actions(d)}
					<button
						class="icon-button hover:text-blue-500"
						onclick={() => {
							console.log(d);
						}}
						use:tooltip={'Connect'}
					>
						<Unplug class="size-4" />
					</button>
				{/snippet}
			</Table>
		</div>
	</div>
</Layout>

<McpCatalog
	bind:this={mcpCatalog}
	onSetupMcp={(mcp, mcpServerInfo) => {
		// TODO:
	}}
	{selectedMcpIds}
	submitText={'Add Server'}
	title="Launch MCP Server"
/>

<svelte:head>
	<title>Obot | MCP Servers</title>
</svelte:head>
