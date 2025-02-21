<script lang="ts">
	import { FileText, X } from 'lucide-svelte/icons';
	import Milkdown from '$lib/components/editor/Milkdown.svelte';
	import Table from '$lib/components/editor/Table.svelte';
	import Codemirror from '$lib/components/editor/Codemirror.svelte';
	import { ChatService, EditorService, type InvokeInput, type Project } from '$lib/services';
	import Task from '$lib/components/tasks/Task.svelte';
	import Controls from '$lib/components/editor/Controls.svelte';
	import Image from '$lib/components/editor/Image.svelte';
	import { CheckSquare, Table as TableIcon, Image as ImageIcon, Wrench } from 'lucide-svelte';
	import { isImage } from '$lib/image';
	import Terminal from '$lib/components/terminal/Terminal.svelte';
	import { term } from '$lib/stores';
	import Tool from '$lib/components/tool/Tool.svelte';
	import Pdf from './editor/Pdf.svelte';
	import { getLayout } from '$lib/context/layout.svelte';
	import type { EditorItem } from '$lib/services/editor/index.svelte';

	interface Props {
		project: Project;
		currentThreadID?: string;
		items: EditorItem[];
	}

	let { project, currentThreadID, items = $bindable() }: Props = $props();
	const layout = getLayout();

	let height = $state<number>();

	function onFileChanged(name: string, contents: string) {
		for (const item of items) {
			if (item.name === name && item.file) {
				item.file.buffer = contents;
				item.file.modified = true;
			}
		}
	}

	async function onInvoke(invoke: InvokeInput) {
		if (currentThreadID) {
			await ChatService.invoke(project.assistantID, project.id, currentThreadID, invoke);
		}
	}
</script>

<div class="flex h-full flex-col">
	{#if layout.fileEditorOpen}
		{#if items.length > 1 || (!items[0].task && !items[0].table && !items[0].generic)}
			<div class="-mx-5 -mt-3 flex border-b-2 border-surface2 px-2 pb-2">
				<ul class="flex flex-1 flex-wrap gap-2 text-center text-sm">
					{#each items as item}
						<li>
							<div
								role="none"
								class:selected={item.selected}
								onclick={() => {
									EditorService.select(items, item.id);
								}}
								class="colors-surface1 group flex rounded-3xl px-4 py-3"
							>
								<div class="flex flex-1 items-center gap-2 ps-2">
									{#if item.table}
										<TableIcon class="h-5 w-5" />
									{:else if item.task}
										<CheckSquare class="h-5 w-5" />
									{:else if isImage(item.name)}
										<ImageIcon class="h-5 w-5" />
									{:else if item.id.startsWith('tl1')}
										<Wrench class="h-5 w-5" />
									{:else}
										<FileText class="h-5 w-5" />
									{/if}
									<span>{item.name}</span>
								</div>
								<button
									class="ml-2"
									onclick={() => {
										EditorService.remove(items, item.id);
									}}
								>
									<X
										class="h-5 w-5 {item.selected
											? 'text-white'
											: 'text-gray'} opacity-0 transition-all group-hover:opacity-100"
									/>
								</button>
							</div>
						</li>
					{/each}
				</ul>
				<Controls navBar {project} {items} />
			</div>
		{/if}

		{#each items as file}
			<div class:hidden={!file.selected} class="flex-1 overflow-auto" bind:clientHeight={height}>
				{#if file.name.toLowerCase().endsWith('.md')}
					<Milkdown {file} {onFileChanged} {onInvoke} {items} />
				{:else if file.name.toLowerCase().endsWith('.pdf')}
					<Pdf {file} {height} />
				{:else if file.table?.name}
					<Table tableName={file.table?.name} {project} {currentThreadID} {items} />
				{:else if file.task}
					<Task
						{project}
						{items}
						id={file.id}
						onChanged={(task) => {
							file.task = task;
							file.name = task.name || file.name;
						}}
					/>
				{:else if isImage(file.name)}
					<Image {file} />
				{:else if file.id.startsWith('tl1')}
					<Tool id={file.id} {project} {items} />
				{:else}
					<Codemirror {file} {onFileChanged} {onInvoke} {items} />
				{/if}
			</div>
		{/each}
	{/if}
	{#if term.open}
		<div
			class={layout.fileEditorOpen
				? '-mx-5 -mb-3 h-1/2 border-t-4 border-surface1 px-2 pt-2'
				: 'h-full'}
		>
			<Terminal {project} />
		</div>
	{/if}
</div>

<style lang="postcss">
	.selected {
		@apply bg-blue text-white;
	}
</style>
