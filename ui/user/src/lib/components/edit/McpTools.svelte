<script lang="ts">
	import { getToolBundleMap } from '$lib/context/toolReferences.svelte';
	import type { Project, ProjectMCP, ToolReference } from '$lib/services/chat/types';
	import CollapsePane from '$lib/components/edit/CollapsePane.svelte';
	import { responsive } from '$lib/stores';
	import { ChevronRight, Wrench, X } from 'lucide-svelte';
	import { twMerge } from 'tailwind-merge';
	import { ChatService } from '$lib/services';

	interface Props {
		project: Project;
		onClose: () => void;
	}

	type ToolCatalog = {
		tool: ToolReference;
		bundleTools: ToolReference[] | undefined;
		total?: number;
	}[];

	let { onClose, project }: Props = $props();
	let mcps = $state<Map<string, ProjectMCP>>(new Map());

	export function load() {
		ChatService.listProjectMCPs(project.assistantID, project.id).then((response) => {
			mcps = new Map(response.map((mcp) => [mcp.catalogID, mcp]));
		});
	}

	$effect(() => {
		if (project) {
			load();
		}
	});

	const bundles: ToolCatalog = $derived.by(() => {
		return Array.from(getToolBundleMap().values()).reduce<ToolCatalog>(
			(acc, { tool, bundleTools }) => {
				console.log(tool);
				if (mcps.has(tool.id)) {
					acc.push({
						tool,
						bundleTools
					});
				}
				return acc;
			},
			[]
		);
	});
</script>

<div class="flex h-full w-full flex-col overflow-hidden md:h-[75vh]">
	<h4
		class="border-surface3 relative mx-4 flex items-center justify-center border-b py-4 text-lg font-semibold md:justify-start"
	>
		MCP Server & Tools
		{#if onClose}
			<button class="icon-button absolute top-2 right-0" onclick={() => onClose()}>
				{#if responsive.isMobile}
					<ChevronRight class="size-6" />
				{:else}
					<X class="size-6" />
				{/if}
			</button>
		{/if}
	</h4>
	<div
		class="default-scrollbar-thin flex min-h-0 w-full grow flex-col items-stretch overflow-x-hidden overflow-y-auto py-2"
	>
		{#each bundles as bundle (bundle.tool.id)}
			<div>
				{@render catalogItem(bundle)}
			</div>
		{/each}
		{#if bundles.length === 0}
			<p class="p-4 text-sm text-gray-500">No tools available.</p>
		{/if}
	</div>
</div>

{#snippet toolInfo(tool: ToolReference, headerLabel?: string, headerLabelClass?: string)}
	{#if tool.metadata?.icon}
		<img
			class="size-8 flex-shrink-0 rounded-md bg-white p-1 dark:bg-gray-600"
			src={tool.metadata?.icon}
			alt="message icon"
		/>
	{:else}
		<Wrench class="size-8 flex-shrink-0 rounded-md bg-gray-100 p-1 text-black" />
	{/if}
	<span class="flex grow flex-col px-2 text-left">
		<span class="flex items-center gap-1">
			{tool.name}
			{#if headerLabel}
				<span class={twMerge('text-xs text-gray-500', headerLabelClass)}>{headerLabel}</span>
			{/if}
		</span>
		<span class="text-gray text-xs font-normal dark:text-gray-300">
			{tool.description}
		</span>
	</span>
{/snippet}

{#snippet catalogItem(item: ToolCatalog[0])}
	{@const { tool, bundleTools } = item}
	<CollapsePane
		showDropdown={bundleTools ? bundleTools.length > 0 : false}
		classes={{
			header: twMerge('group py-0 pl-0 pr-6.5'),
			content: 'border-none p-0 bg-surface1'
		}}
	>
		{#if bundleTools && bundleTools.length > 0}
			{#each bundleTools as subTool (subTool.id)}
				{@render subToolItem(subTool)}
			{/each}
		{/if}

		{#snippet header()}
			<div class="flex grow items-center justify-between gap-2 rounded-lg p-2 px-4">
				<div class="flex grow items-center">
					{@render toolInfo(tool)}
				</div>
			</div>
		{/snippet}
	</CollapsePane>
{/snippet}

{#snippet subToolItem(toolReference: ToolReference)}
	<div
		class={twMerge('group flex grow items-center gap-2 p-2 px-4 transition-opacity duration-200')}
	>
		{@render toolInfo(toolReference)}
	</div>
{/snippet}
