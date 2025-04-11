<script lang="ts">
	import popover from '$lib/actions/popover.svelte';
	import type { ToolReference } from '$lib/services';
	import { WrenchIcon } from 'lucide-svelte';

	interface Props {
		tool?: ToolReference;
		tools?: ToolReference[];
	}

	let { tool, tools }: Props = $props();
	let { ref, tooltip } = popover({
		placement: 'top-start',
		offset: 4
	});
</script>

<div
	use:ref
	class="bg-surface2 flex size-8 cursor-help items-center gap-1 rounded-full p-2 dark:bg-gray-500 dark:text-black"
>
	{#if tool}
		{#if tool?.metadata?.icon}
			<img alt={tool.name || 'Unknown'} src={tool.metadata.icon} class="h-4 w-4" />
		{:else}
			<WrenchIcon class="h-4 w-4" />
		{/if}
	{/if}
	{#if tools}
		<p class="text-xs">
			{#if tools.length > 9}
				<span><b>9</b>+</span>
			{:else}
				<span>+<b>{tools.length}</b></span>
			{/if}
		</p>
	{/if}
</div>

<div use:tooltip={{ hover: true }} class="tooltip hidden">
	{#if tool}
		<p>{tool.name}</p>
	{/if}
	{#if tools}
		<ul class="flex flex-col gap-4 p-2">
			{#each tools as tool}
				<li class="flex items-center gap-2">
					{#if tool?.metadata?.icon}
						<div class="bg-surface2 rounded-full p-2 dark:bg-gray-100">
							<img alt={tool.name || 'Unknown'} src={tool.metadata.icon} class="h-4 w-4" />
						</div>
					{:else}
						<WrenchIcon class="h-4 w-4" />
					{/if}
					<p>{tool.name}</p>
				</li>
			{/each}
		</ul>
	{/if}
</div>
