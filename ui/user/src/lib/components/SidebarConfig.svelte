<script lang="ts">
	import { closeSidebarConfig, getLayout } from '$lib/context/layout.svelte';
	import type { Assistant, AssistantTool, Project } from '$lib/services';
	import { fade } from 'svelte/transition';
	import Slack from './slack/Slack.svelte';
	import CustomTool from './edit/CustomTool.svelte';
	import { getProjectTools } from '$lib/context/projectTools.svelte';

	interface Props {
		project: Project;
		currentThreadID?: string;
		assistant?: Assistant;
	}

	let { project = $bindable(), currentThreadID = $bindable() }: Props = $props();
	const layout = getLayout();

	const projectTools = getProjectTools();
	let toEdit = $state<AssistantTool>();

	$effect(() => {
		if (layout.customToolId) {
			toEdit = projectTools.tools.find((t) => t.id === layout.customToolId);
		}
	});
</script>

<div class="default-scrollbar-thin relative flex w-full justify-center overflow-y-auto" in:fade>
	{#if layout.sidebarConfig === 'slack'}
		<Slack {project} inline />
	{:else if layout.sidebarConfig === 'custom-tool' && layout.customToolId && toEdit}
		{#key layout.customToolId}
			<CustomTool
				bind:tool={toEdit}
				{project}
				onSave={async (tool) => {
					projectTools.tools = projectTools.tools.map((t) => (t.id === tool.id ? tool : t));
				}}
				onDelete={async (tool) => {
					projectTools.tools = projectTools.tools.filter((t) => t.id !== tool.id);
					closeSidebarConfig(layout);
				}}
			/>
		{/key}
	{:else}
		<div class="p-8">
			{@render underConstruction()}
		</div>
	{/if}
</div>

{#snippet underConstruction()}
	<div class="flex w-full flex-col items-center justify-center font-light">
		<img src="/user/images/under-construction.webp" alt="under construction" class="size-32" />
		<p class="text-sm font-light text-gray-500">
			This section is under construction. Please check back later.
		</p>
	</div>
{/snippet}
