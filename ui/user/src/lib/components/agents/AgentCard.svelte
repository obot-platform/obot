<script lang="ts">
	import { DEFAULT_PROJECT_NAME } from '$lib/constants';
	import AssistantIcon from '$lib/icons/AssistantIcon.svelte';
	import type { ProjectTemplate, MCP, MCPInfo } from '$lib/services';
	import { twMerge } from 'tailwind-merge';
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import { Star } from 'lucide-svelte';

	interface Props {
		template: ProjectTemplate;
		mcps?: MCP[];
		onclick?: () => void;
	}

	let { template, mcps = [], onclick }: Props = $props();
</script>

<div class="relative h-full w-full">
	<button
		{onclick}
		class={twMerge(
			'card group from-surface2 to-surface1 relative z-20 h-full w-full flex-col overflow-hidden border border-transparent bg-radial-[at_25%_25%] to-75% shadow-sm select-none',
			!onclick && 'cursor-not-allowed opacity-50'
		)}
	>
		<div class="flex h-fit w-full flex-col items-center gap-2 p-4 pb-12 md:h-auto md:grow">
			{#if template.featured}
				<div
					use:tooltip={'Featured'}
					class="pointer-events-none absolute top-2 left-2 z-30 text-blue-500"
				>
					<Star class="size-5" />
				</div>
			{/if}
			<AssistantIcon project={template.projectSnapshot} class="size-22" />
			<div class="flex flex-col items-center text-center">
				<h4 class="text-sm font-semibold">
					{template.projectSnapshot.name || DEFAULT_PROJECT_NAME}
				</h4>
				{#if template.projectSnapshot.description}
					<p class="line-clamp-2 text-xs font-light text-gray-500">
						{template.projectSnapshot.description}
					</p>
				{/if}
			</div>
			{#if mcps.length > 0}
				<div class="absolute bottom-2 flex gap-1">
					{#each mcps.slice(0, 5) as mcp}
						{#if mcp.commandManifest?.server?.icon}
							{@render mcpPill(mcp.commandManifest)}
						{/if}
						{#if mcp.urlManifest?.server?.icon}
							{@render mcpPill(mcp.urlManifest)}
						{/if}
					{/each}
					{#if mcps.length > 5}
						<div
							class="flex size-6 items-center justify-center rounded-full bg-gray-100 text-xs font-medium dark:bg-gray-700"
						>
							+{mcps.length - 5}
						</div>
					{/if}
				</div>
			{/if}
		</div>
	</button>
</div>

{#snippet mcpPill(mcp: MCPInfo)}
	<div class="h-fit w-fit flex-shrink-0 rounded-md bg-gray-50 p-1 dark:bg-gray-600">
		<img
			use:tooltip={mcp.server.name}
			src={mcp.server.icon}
			alt={`${mcp.server.name} logo`}
			class="size-6"
		/>
	</div>
{/snippet}
