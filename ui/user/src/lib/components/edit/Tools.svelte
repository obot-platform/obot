<script lang="ts">
	import { popover } from '$lib/actions';
	import CollapsePane from '$lib/components/edit/CollapsePane.svelte';
	import {
		type AssistantTool,
		type AssistantToolType,
		ChatService,
		type Project
	} from '$lib/services';
	import ToolCatalog from './ToolCatalog.svelte';
	import CustomTool from '$lib/components/edit/CustomTool.svelte';
	import { Plus, X, SquarePen } from 'lucide-svelte/icons';
	import { responsive, tools as toolsStore, version } from '$lib/stores';

	interface Props {
		onNewTools: (tools: AssistantTool[], oauthApps: Set<string>) => Promise<void>;
		project: Project;
	}

	let enabledList = $derived(
		toolsStore.current.tools.filter((t) => !t.builtin && t.enabled && t.id)
	);

	let { onNewTools, project }: Props = $props();
	let typeSelectionTT = popover({
		fixed: responsive.isMobile,
		slide: responsive.isMobile ? 'up' : undefined
	});
	let customToolDialog = $state<HTMLDialogElement>();
	let toEdit = $state<AssistantTool>();

	async function remove(tool: AssistantTool) {
		if (tool.toolType) {
			toolsStore.setTools(toolsStore.current.tools.filter((t) => t.id !== tool.id));
		} else {
			await onNewTools(
				toolsStore.current.tools.filter((t) => t.id !== tool.id),
				new Set(project.oauthApps)
			);
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

		toolsStore.setTools([...toolsStore.current.tools, newTool]);
		toEdit = newTool;
		customToolDialog?.showModal();
		typeSelectionTT.toggle(false);
	}

	let toolCatalog = popover({ fixed: true, slide: responsive.isMobile ? 'left' : undefined });
</script>

{#snippet toolList(tools: AssistantTool[])}
	<ul class="flex flex-col gap-2">
		{#each tools as tool (tool.id)}
			{@const tt = popover({ hover: true, placement: 'top', delay: 300 })}

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
				</div>

				<p use:tt.tooltip class="tooltip max-w-64">{tool.description}</p>
			</div>
		{/each}
	</ul>
{/snippet}

<CollapsePane header="Tools">
	<div class="flex flex-col gap-2">
		{@render toolList(enabledList)}

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
							toolCatalog.toggle(true);
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
							toolsStore.setTools(
								toolsStore.current.tools.map((t) => (t.id === tool.id ? tool : t))
							);
						}}
						onDelete={async (tool) => {
							toolsStore.setTools(toolsStore.current.tools.filter((t) => t.id !== tool.id));
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
			{#if !version.current.dockerSupported}
				<button
					class="button flex items-center gap-1 text-sm"
					onclick={() => toolCatalog.toggle(true)}
					use:toolCatalog.ref><Plus class="size-4" /> Tools</button
				>
			{:else}
				<button class="hidden" aria-label="Tools" use:toolCatalog.ref></button>
			{/if}
			<div
				use:toolCatalog.tooltip
				class="default-dialog bottom-0 left-0 h-screen w-full rounded-none p-2 md:bottom-1/2 md:left-1/2 md:h-fit md:w-auto md:-translate-x-1/2 md:translate-y-1/2 md:rounded-xl"
			>
				<ToolCatalog
					onSelectTools={onNewTools}
					onSubmit={() => toolCatalog.toggle(false)}
					oauthApps={new Set(project.oauthApps)}
				/>
			</div>
		</div>
	</div>
</CollapsePane>
