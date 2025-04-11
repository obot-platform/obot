<script lang="ts">
	import { popover } from '$lib/actions';
	import CollapsePane from '$lib/components/edit/CollapsePane.svelte';
	import {
		type AssistantTool,
		type AssistantToolType,
		ChatService,
		type Project
	} from '$lib/services';
	import CustomTool from '$lib/components/edit/CustomTool.svelte';
	import { Plus, SquarePen } from 'lucide-svelte/icons';
	import { responsive } from '$lib/stores';
	import { twMerge } from 'tailwind-merge';
	import { getProjectTools } from '$lib/context/projectTools.svelte';

	interface Props {
		project: Project;
	}

	const projectTools = getProjectTools();
	let { project }: Props = $props();
	let enabledList = $derived(
		projectTools.tools.filter((t) => !t.builtin && t.enabled && t.id && t.toolType)
	);
	let typeSelectionTT = popover();
	let customToolDialog = $state<HTMLDialogElement>();
	let toEdit = $state<AssistantTool>();

	function edit(tool: AssistantTool) {
		toEdit = tool;
		customToolDialog?.showModal();
		typeSelectionTT.toggle(false);
	}

	async function newTool(type: AssistantToolType) {
		const newTool = await ChatService.createTool(project.assistantID, project.id, {
			id: '',
			toolType: type
		});

		projectTools.tools.push(newTool);
		toEdit = newTool;
		customToolDialog?.showModal();
		typeSelectionTT.toggle(false);
	}
</script>

{#snippet toolList(tools: AssistantTool[])}
	<ul class="flex flex-col gap-2">
		{#each tools as tool (tool.id)}
			{@const tt = popover({ placement: 'top', delay: 300 })}

			<div
				class="bg-surface1 flex w-full cursor-pointer items-start justify-between gap-1 rounded-md p-2"
				use:tt.ref
			>
				<div class="flex w-full flex-col gap-1">
					<div class="flex w-full items-center justify-between gap-1 text-sm font-medium">
						<div class="flex items-center gap-2">
							{#if tool.icon}
								<div class="bg-surface1 flex-shrink-0 rounded-md p-1 dark:bg-gray-200">
									<img src={tool.icon} class="size-6" alt="tool {tool.name} icon" />
								</div>
							{/if}
							<div class="flex flex-col">
								<p class="line-clamp-1">{tool.name || 'Untitled'}</p>
								<span class="line-clamp-2 text-xs font-light text-gray-500">{tool.description}</span
								>
							</div>
						</div>
						<button class="icon-button-small" onclick={() => edit(tool)}>
							<SquarePen class="size-5" />
						</button>
					</div>
				</div>

				<p use:tt.tooltip={{ hover: true }} class="tooltip max-w-64">{tool.description}</p>
			</div>
		{/each}
	</ul>
{/snippet}

<CollapsePane header="Custom Tools">
	<p class="pb-4 text-sm text-gray-500">Custom tools added here are available to all threads.</p>
	<div class="flex flex-col gap-2">
		{@render toolList(enabledList)}

		<button
			class="button flex items-center gap-1 self-end text-sm"
			use:typeSelectionTT.ref
			onclick={() => typeSelectionTT.toggle(true)}
		>
			<Plus class="size-4" />
			Custom Tool
		</button>

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

		<dialog
			class={twMerge(
				'default-dialog relative size-full md:w-4/5',
				responsive.isMobile && 'mobile-screen-dialog p-0'
			)}
			bind:this={customToolDialog}
			onclose={() => {
				toEdit = undefined;
			}}
		>
			{#if toEdit}
				<CustomTool
					bind:tool={toEdit}
					{project}
					onSave={async (tool) => {
						projectTools.tools = projectTools.tools.map((t) => (t.id === tool.id ? tool : t));
					}}
					onDelete={async (tool) => {
						projectTools.tools = projectTools.tools.filter((t) => t.id !== tool.id);
						toEdit = undefined;
						customToolDialog?.close();
					}}
					onClose={() => {
						toEdit = undefined;
						customToolDialog?.close();
					}}
				></CustomTool>
			{/if}
		</dialog>
	</div>
</CollapsePane>
