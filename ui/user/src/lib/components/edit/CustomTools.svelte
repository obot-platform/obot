<script lang="ts">
	import { popover } from '$lib/actions';
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import { ChatService, type AssistantToolType, type Project } from '$lib/services';
	import { responsive } from '$lib/stores';
	import { Plus, SquarePen } from 'lucide-svelte';
	import { getLayout, openCustomTool } from '$lib/context/layout.svelte';
	import { getProjectTools } from '$lib/context/projectTools.svelte';
	import CollapsePane from '$lib/components/edit/CollapsePane.svelte';

	interface Props {
		project: Project;
	}

	let { project }: Props = $props();
	let typeSelectionTT = popover();
	let layout = getLayout();

	const projectTools = getProjectTools();
	let enabledCustomTools = $derived(
		projectTools.tools.filter((t) => !t.builtin && t.enabled && t.id && t.toolType)
	);

	async function newTool(type: AssistantToolType) {
		const newTool = await ChatService.createTool(project.assistantID, project.id, {
			id: '',
			toolType: type
		});

		projectTools.tools.push(newTool);
		openCustomTool(layout, newTool.id);
		typeSelectionTT.toggle(false);
	}
</script>

<CollapsePane classes={{ header: 'pl-3 text-md', content: 'p-2' }} iconSize={5}>
	{#snippet header()}
		<span class="flex grow items-center gap-2 text-start text-sm font-extralight">
			Custom Tools
		</span>
	{/snippet}
	<div class="text-md flex flex-col gap-2">
		{#if enabledCustomTools.length > 0}
			<div class="flex flex-col">
				{#each enabledCustomTools as customTool}
					<button
						class="group hover:bg-surface3 flex min-h-9 w-full items-center gap-3 rounded-md transition-colors duration-300"
						onclick={() => openCustomTool(layout, customTool.id)}
					>
						<div class="flex w-full min-w-0 items-center gap-3 py-2 pl-2">
							{#if customTool.icon}
								<div class="bg-surface1 flex-shrink-0 rounded-md p-1 dark:bg-gray-200">
									<img src={customTool.icon} class="size-6" alt="tool {customTool.name} icon" />
								</div>
							{/if}
							<div class="flex min-w-0 flex-col text-left">
								<p class="truncate text-left text-xs">
									{customTool.name || 'Untitled'}
								</p>
								<span class="truncate text-xs text-gray-500">{customTool.description}</span>
							</div>
						</div>
						<div
							class="py-2 pr-3 transition-opacity duration-200 group-hover:opacity-100 md:opacity-0"
						>
							<SquarePen class="size-4" />
						</div>
					</button>
				{/each}
			</div>
		{/if}

		<div class="flex justify-end">
			<button
				class="button flex items-center gap-1 text-xs"
				onclick={() => typeSelectionTT.toggle(true)}
				use:tooltip={'Add Custom Tool'}
				use:typeSelectionTT.ref
			>
				<Plus class="size-4" /> Add Custom Tool
			</button>
		</div>
	</div>

	<div
		class="default-dialog bottom-0 left-0 w-full p-2 md:bottom-auto md:left-auto md:w-fit"
		use:typeSelectionTT.tooltip={{
			fixed: responsive.isMobile,
			slide: responsive.isMobile ? 'up' : undefined
		}}
	>
		<div class="flex flex-col gap-2">
			<button
				class="menu-button"
				onclick={() => {
					newTool('python');
				}}
			>
				Python Code
			</button>
			<button
				class="menu-button"
				onclick={() => {
					newTool('javascript');
				}}
			>
				JavaScript Code
			</button>
			<button
				class="menu-button"
				onclick={() => {
					newTool('script');
				}}
			>
				Shell Script
			</button>
			<button
				class="menu-button"
				onclick={() => {
					newTool('container');
				}}
			>
				Docker Container
			</button>
		</div>
	</div>
</CollapsePane>
