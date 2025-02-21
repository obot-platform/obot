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
	import { isCapabilityTool } from '$lib/model/tools';
	import type { EditorItem } from '$lib/services/editor/index.svelte';
	import { getLayout } from '$lib/context/layout.svelte';

	interface Prop {
		project: Project;
		items: EditorItem[];
		tools: AssistantTool[];
		version: Version;
	}

	let menu = $state<ReturnType<typeof Menu>>();
	let { project, items = $bindable(), tools, version }: Prop = $props();
	const layout = getLayout();

	async function addTool() {
		const tool = await ChatService.createTool(project.assistantID, project.id, newTool);
		await EditorService.load(items, project, tool.id);
		layout.fileEditorOpen = true;
		menu?.toggle(false);
	}

	async function editTool(id: string) {
		await EditorService.load(items, project, id);
		layout.fileEditorOpen = true;
		menu?.toggle(false);
	}

	async function onLoad() {
		tools = (await ChatService.listTools(project.assistantID, project.id)).items;
	}
</script>

<Menu bind:this={menu} title="Tools" description="Available tools" {onLoad}>
	{#snippet icon()}
		<Wrench class="h-5 w-5" />
	{/snippet}
	{#snippet body()}
		<ul class="space-y-4 py-6 text-sm">
			{#each tools as tool, i}
				{#if !isCapabilityTool(tool.id)}
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
									<p class="text-xs font-normal text-gray dark:text-gray-300">
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
					class="-mb-3 -mr-3 mt-3 flex items-center justify-end gap-2 rounded-3xl p-3 px-4 hover:bg-gray-500 hover:text-white"
				>
					Add Custom Tool
					<Plus class="ms-1 h-5 w-5" />
				</button>
			</div>
		{/if}
	{/snippet}
</Menu>
