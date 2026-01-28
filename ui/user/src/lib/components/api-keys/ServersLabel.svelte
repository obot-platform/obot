<script lang="ts">
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import { TriangleAlert } from 'lucide-svelte';
	import { mcpServersAndEntries } from '$lib/stores';
	import { compileAvailableMcpServers } from '$lib/services/chat/mcp';

	interface Props {
		mcpServerIds: string[];
	}

	let { mcpServerIds }: Props = $props();
	let mcpServers = $derived(
		compileAvailableMcpServers(
			mcpServersAndEntries.current.servers,
			mcpServersAndEntries.current.userConfiguredServers
		)
	);

	let isAllServers = $derived(mcpServerIds.includes('*'));

	let serverMap = $derived(new Map(mcpServers.map((s) => [s.id, s])));

	let deletedServersCount = $derived(
		isAllServers ? 0 : mcpServerIds.filter((id) => !serverMap.has(id)).length
	);

	let resolvedServers = $derived.by(() => {
		if (isAllServers) return [];
		return mcpServerIds
			.map((id) => {
				const server = serverMap.get(id);
				return server?.alias || server?.manifest.name || null;
			})
			.filter((name): name is string => name !== null);
	});

	type DisplayItem = { name: string; deleted: boolean };
	let displayItems = $derived.by((): DisplayItem[] => {
		if (isAllServers) return [];
		const items: DisplayItem[] = resolvedServers.map((name) => ({ name, deleted: false }));
		for (let i = 0; i < deletedServersCount; i++) {
			items.push({ name: 'Deleted', deleted: true });
		}
		return items;
	});
</script>

{#snippet serverName(item: DisplayItem)}
	{#if item.deleted}<i class="text-on-surface1 font-light italic">({item.name})</i
		>{:else}{item.name}{/if}
{/snippet}

<div class="">
	{#if isAllServers}
		All MCP Servers
	{:else if displayItems.length === 1}
		{@render serverName(displayItems[0])}
	{:else if displayItems.length === 2}
		{@render serverName(displayItems[0])} & {@render serverName(displayItems[1])}
	{:else if displayItems.length === 3}
		{@render serverName(displayItems[0])}, {@render serverName(displayItems[1])}, & {@render serverName(
			displayItems[2]
		)}
	{:else if displayItems.length > 3}
		{@render serverName(displayItems[0])}, {@render serverName(displayItems[1])}, & {displayItems.length -
			2} more
	{/if}
	{#if displayItems.length > 3 && deletedServersCount > 0}
		<span
			class="inline-block"
			use:tooltip={`Includes ${deletedServersCount} deleted server${deletedServersCount === 1 ? '' : 's'}.`}
		>
			<TriangleAlert class="size-3 text-yellow-500" />
		</span>
	{/if}
</div>
