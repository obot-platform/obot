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
	{#if EditorService.items.length > 1 || (!EditorService.items[0].task && !EditorService.items[0].table)}
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
								<FileText class="h-5 w-5" />
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
			{:else if file.name.toLowerCase().endsWith('.png')}
				<Image {file} />
			{:else}
				<Codemirror {file} {onFileChanged} {onInvoke} />
			{/if}
		</div>
	{/each}
</div>

<style lang="postcss">
	.selected {
		@apply bg-blue text-white shadow-md;
	}
</style>
