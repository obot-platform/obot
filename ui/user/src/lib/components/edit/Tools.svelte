<script lang="ts">
	import { popover } from '$lib/actions';
	import CollapsePane from '$lib/components/edit/CollapsePane.svelte';
	import {
		type Assistant,
		type AssistantTool,
		type AssistantToolType,
		ChatService,
		type Project
	} from '$lib/services';
	import { Plus, X } from 'lucide-svelte/icons';
	import ToolCatalog from './ToolCatalog.svelte';
	import { responsive, version } from '$lib/stores';
	import CustomTool from '$lib/components/edit/CustomTool.svelte';
	import { SquarePen } from 'lucide-svelte';

	interface Props {
		tools: AssistantTool[];
		onNewTools: (tools: AssistantTool[]) => Promise<void>;
		project: Project;
		assistant?: Assistant;
	}

	let { tools = $bindable(), onNewTools, project, assistant }: Props = $props();
	let enabledList = $derived(tools.filter((t) => !t.builtin && t.enabled && t.id));
	let typeSelectionTT = popover({
		fixed: responsive.isMobile,
		slide: responsive.isMobile ? 'up' : undefined
	});
	let toolCatalog: ReturnType<typeof ToolCatalog>;
	let customToolDialog = $state<HTMLDialogElement>();
	let toEdit = $state<AssistantTool>();

	async function remove(tool: AssistantTool) {
		if (tool.toolType) {
			tools = tools.filter((t) => t.id !== tool.id);
		} else {
			await onNewTools(tools.filter((t) => t.id !== tool.id));
		}
	}

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

		tools.push(newTool);
		toEdit = newTool;
		customToolDialog?.showModal();
		typeSelectionTT.toggle(false);
	}
</script>

{#snippet toolList(tools: AssistantTool[])}
	<ul class="flex flex-col">
		{#each tools as tool (tool.id)}
			{@const tt = popover({ hover: true, placement: 'top', delay: 300 })}

			<div class="flex w-full cursor-pointer items-start justify-between gap-1 p-2" use:tt.ref>
				<div class="flex w-full flex-col gap-1">
					<div class="flex w-full items-center justify-between gap-1 text-sm font-medium">
						<span class="flex items-center gap-2">
							{#if tool.icon}
								<img
									src={tool.icon}
									class="size-6 rounded-full bg-white p-1"
									alt="tool {tool.name} icon"
								/>
							{/if}
							<p class="line-clamp-1">{tool.name || 'Untitled'}</p>
						</span>
						{#if tool.toolType}
							<button class="icon-button-small" onclick={() => edit(tool)}>
								<SquarePen class="size-5" />
							</button>
						{:else}
							<button class="icon-button-small" onclick={() => remove(tool)}>
								<X class="size-5" />
							</button>
						{/if}
					</div>
					<span class="line-clamp-2 text-xs text-gray-500">{tool.description}</span>
				</div>

				<p use:tt.tooltip class="tooltip max-w-64">{tool.description}</p>
			</div>
		{/each}
	</ul>
{/snippet}

<CollapsePane header="Tools">
	<div class="flex flex-col gap-2">
		<ul class="flex flex-col gap-2">
			{@render toolList(enabledList)}
		</ul>

		{#if version.current.dockerSupported}
			<button
				class="button flex items-center gap-1 self-end text-sm"
				use:typeSelectionTT.ref
				onclick={() => typeSelectionTT.toggle(true)}
			>
				<Plus class="size-4" />
				Tools
			</button>

			<div
				class="default-dialog bottom-0 left-0 w-full p-2 md:bottom-auto md:left-auto md:w-fit"
				use:typeSelectionTT.tooltip
			>
				<div class="flex flex-col gap-2">
					<button
						class="button flex items-center gap-2"
						onclick={() => {
							typeSelectionTT.toggle(false);
							toolCatalog.open();
						}}
					>
						From Catalog
					</button>
					<button
						class="button flex items-center gap-2"
						onclick={() => {
							newTool('python');
						}}
					>
						Python Code
					</button>
					<button
						class="button flex items-center gap-2"
						onclick={() => {
							newTool('javascript');
						}}
					>
						JavaScript Code
					</button>
					<button
						class="button flex items-center gap-2"
						onclick={() => {
							newTool('script');
						}}
					>
						Shell Script
					</button>
					<button
						class="button flex items-center gap-2"
						onclick={() => {
							newTool('container');
						}}
					>
						Docker Container
					</button>
				</div>
			</div>

			<dialog
				class="default-dialog relative size-full md:w-4/5"
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
							tools = tools.map((t) => (t.id === tool.id ? tool : t));
						}}
						onDelete={async (tool) => {
							tools = tools.filter((t) => t.id !== tool.id);
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
		{/if}

		<div class="self-end">
			<ToolCatalog
				bind:this={toolCatalog}
				{tools}
				onSelectTools={onNewTools}
				maxTools={assistant?.maxTools}
				dialogOnly={version.current.dockerSupported}
			/>
		</div>
	</div>
</CollapsePane>
