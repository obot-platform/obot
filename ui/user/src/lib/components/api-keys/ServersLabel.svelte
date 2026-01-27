<script lang="ts">
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import type { MCPCatalogServer } from '$lib/services/chat/types';
	import { TriangleAlert } from 'lucide-svelte';

	interface Props {
		mcpServerIds: string[];
		mcpServers: MCPCatalogServer[];
	}

	let { mcpServerIds, mcpServers }: Props = $props();

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
</script>

<div class="">
	{#if !isAllServers}
		{#if resolvedServers.length === 1}
			{resolvedServers[0]}
		{:else if resolvedServers.length === 2}
			{resolvedServers[0]} & {resolvedServers[1]}
		{:else if resolvedServers.length === 3}
			{resolvedServers[0]}, {resolvedServers[1]}, & {resolvedServers[2]}
		{:else if resolvedServers.length > 3}
			{resolvedServers[0]}, {resolvedServers[1]}, & {resolvedServers.length - 2} more
		{/if}
	{/if}
	{#if deletedServersCount > 0}
		<span
			class="inline-block"
			use:tooltip={`Includes ${deletedServersCount} deleted server${deletedServersCount === 1 ? '' : 's'}.`}
		>
			<TriangleAlert class="size-3 text-yellow-500" />
		</span>
	{/if}
</div>
