<script lang="ts">
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import type { MCPCatalogServer } from '$lib/services/chat/types';

	interface Props {
		mcpServerIds: string[];
		mcpServers: MCPCatalogServer[];
	}

	let { mcpServerIds, mcpServers }: Props = $props();

	let isAllServers = $derived(mcpServerIds.includes('*'));

	let serverMap = $derived(new Map(mcpServers.map((s) => [s.id, s])));

	let resolvedServers = $derived.by(() => {
		if (isAllServers) return [];
		return mcpServerIds.map((id) => {
			const server = serverMap.get(id);
			return server?.alias || server?.manifest.name || 'Deleted Server';
		});
	});

	let tooltipContent = $derived.by(() => {
		if (isAllServers) return '';
		const firstThree = resolvedServers.slice(0, 3);
		const remaining = resolvedServers.length - 3;
		if (remaining > 0) {
			return [...firstThree, `...and ${remaining} more`].join('\n');
		}
		return firstThree.join('\n');
	});

	let displayText = $derived.by(() => {
		if (isAllServers) return 'All My Servers';
		const count = mcpServerIds.length;
		return `${count} server${count === 1 ? '' : 's'}`;
	});
</script>

<span class="pill-rounded bg-surface3" use:tooltip={tooltipContent}>
	{displayText}
</span>
