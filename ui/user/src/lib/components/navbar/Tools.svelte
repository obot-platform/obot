<script lang="ts">
	import { Plus, Wrench } from 'lucide-svelte/icons';
	import {
		type AssistantTool,
		ChatService,
		EditorService,
		type Project,
		type Version
	} from '$lib/services';
	import { newTool } from '$lib/components/tool/Tool.svelte';
	import Menu from '$lib/components/navbar/Menu.svelte';
	import { PenBox } from 'lucide-svelte';
	import { getLayout } from '$lib/context/layout.svelte';
	import { responsive } from '$lib/stores';

	interface Prop {
		project: Project;
		tools: AssistantTool[];
		version: Version;
	}

	let menu = $state<ReturnType<typeof Menu>>();
	let { project, tools, version }: Prop = $props();
	const layout = getLayout();

	async function addTool() {
		const tool = await ChatService.createTool(project.assistantID, project.id, newTool);
		await EditorService.load(layout.items, project, tool.id);
		layout.fileEditorOpen = true;
		menu?.toggle(false);
	}

	async function editTool(id: string) {
		await EditorService.load(layout.items, project, id);
		layout.fileEditorOpen = true;
		menu?.toggle(false);
	}

	async function onLoad() {
		tools = (await ChatService.listTools(project.assistantID, project.id)).items;
	}
</script>

<Menu
	bind:this={menu}
	title="Tools"
	description="AI can use the following tools."
	showRefresh={false}
	{onLoad}
	classes={{
		button: 'button-icon-primary',
		dialog: responsive.isMobile
			? 'rounded-none max-h-[calc(100vh-64px)] left-0 bottom-0 w-full'
			: ''
	}}
	slide={responsive.isMobile ? 'up' : undefined}
	fixed={responsive.isMobile}
>
	{#snippet icon()}
		<Wrench class="h-5 w-5" />
	{/snippet}
	{#snippet body()}
		<ul class="space-y-4 py-6 text-sm">
			{#each tools as tool, i}
				{#if !tool.builtin && tool.enabled}
					<li>
						<div class="flex">
							{#if tool.icon}
								<img
									class="h-8 w-8 rounded-md bg-gray-100 p-1"
									src={tool.icon}
									alt="message icon"
								/>
							{:else}
								<Wrench class="h-8 w-8 rounded-md bg-gray-100 p-1 text-black" />
							{/if}
							<div class="flex flex-1 px-2">
								<div>
									<label for="checkbox-item-{i}" class="text-sm font-medium dark:text-gray-100"
										>{tool.name}</label
									>
									<p class="text-gray text-xs font-normal dark:text-gray-300">
										{tool.description}
									</p>
								</div>
							</div>
							<button
								class="p-1"
								class:invisible={!tool.id.startsWith('tl1')}
								onclick={() => editTool(tool.id)}
							>
								<PenBox class="h-4 w-4" />
							</button>
						</div>
					</li>
				{/if}
			{/each}
		</ul>
		{#if version.dockerSupported}
			<div class="flex justify-end">
				<button
					onclick={addTool}
					class="button mt-3 -mr-3 -mb-3 flex items-center justify-end gap-1 text-sm"
				>
					Add Custom Tool
					<Plus class="ms-1 size-4" />
				</button>
			</div>
		{/if}
	{/snippet}
</Menu>
