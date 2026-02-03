<script lang="ts">
	import { ExternalLink } from 'lucide-svelte';
	import type { ChatMessageItemResourceLink } from '$lib/services/nanobot/types';

	interface Props {
		item: ChatMessageItemResourceLink;
	}

	let { item }: Props = $props();
</script>

<div class="bg-base-200 mb-3 rounded-lg p-3">
	<div class="flex items-center gap-2 text-sm">
		<ExternalLink class="text-primary h-4 w-4" />
		<span>{item.name || item.uri}</span>
	</div>
	{#if item.description}
		<p class="mt-1 text-xs opacity-70">{item.description}</p>
	{/if}
</div>
