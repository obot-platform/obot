<script lang="ts">
	import { FileText, X } from '$lib/icons';
	import Milkdown from '$lib/components/editor/Milkdown.svelte';
	import Table from '$lib/components/editor/Table.svelte';
	import Codemirror from '$lib/components/editor/Codemirror.svelte';
	import { ChatService, EditorService, type InvokeInput } from '$lib/services';
	import Task from '$lib/components/tasks/Task.svelte';
	import Controls from '$lib/components/editor/Controls.svelte';
	import { currentAssistant } from '$lib/stores';
	import Image from '$lib/components/editor/Image.svelte';
	import { CheckSquare, Table as TableIcon, Image as ImageIcon, Wrench } from 'lucide-svelte';
	import { isImage } from '$lib/image';
	import Terminal from '$lib/components/terminal/Terminal.svelte';
	import { term } from '$lib/stores';
	import Tool from '$lib/components/tool/Tool.svelte';

	const editorVisible = EditorService.visible;

	function onFileChanged(name: string, contents: string) {
		for (const item of EditorService.items) {
			if (item.name === name) {
				item.buffer = contents;
				item.modified = true;
			}
		}
	}

	async function onInvoke(invoke: InvokeInput) {
		if ($currentAssistant.id) {
			await ChatService.invoke($currentAssistant.id, invoke);
		}
	}
</script>

<div class="flex h-full flex-col">
	{#if $editorVisible}
		{#if EditorService.items.length > 1 || (!EditorService.items[0].task && !EditorService.items[0].table && !EditorService.items[0].generic)}
			<div class="flex rounded-3xl border-gray-100 pt-2">
				<ul class="flex flex-1 flex-wrap text-center text-sm">
					{#each EditorService.items as item}
						<li class="pb-2 pl-2">
							<div
								role="none"
								class:selected={item.selected}
								onclick={() => {
									EditorService.select(item.id);
								}}
								class="active group flex rounded-3xl bg-gray-70 px-4 py-3 text-black dark:bg-gray-950 dark:text-gray-50"
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
										EditorService.remove(item.id);
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
				<Controls navBar />
			</div>
		{/if}

		{#each EditorService.items as file}
			<div class:hidden={!file.selected} class="flex-1 overflow-auto">
				{#if file.name.toLowerCase().endsWith('.md')}
					<Milkdown {file} {onFileChanged} {onInvoke} />
				{:else if file.table}
					<Table tableName={file.table} />
				{:else if file.task}
					<Task
						id={file.id}
						onChanged={(task) => {
							file.task = task;
							file.name = task.name || file.name;
						}}
					/>
				{:else if isImage(file.name)}
					<Image {file} />
				{:else if file.id.startsWith('tl1')}
					<Tool id={file.id} />
				{:else}
					<Codemirror {file} {onFileChanged} {onInvoke} />
				{/if}
			</div>
		{/each}
	{/if}
	{#if term.open}
		{#if !$editorVisible}
			<div class="self-end">
				<Controls />
			</div>
		{/if}
		<div class="p-5 {$editorVisible ? 'h-1/2' : 'h-full'}">
			<Terminal />
		</div>
	{/if}
</div>

<style lang="postcss">
	.selected {
		@apply bg-blue text-white shadow-md;
	}
</style>
