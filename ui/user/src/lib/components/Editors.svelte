<script lang="ts">
	import { FileText, X } from 'lucide-svelte/icons';
	import { ChatService, EditorService, type InvokeInput, type Project } from '$lib/services';
	import Controls from '$lib/components/editor/Controls.svelte';
	import { Table as TableIcon, Image as ImageIcon, Wrench } from 'lucide-svelte';
	import { isImage } from '$lib/image';
	import Terminal from '$lib/components/terminal/Terminal.svelte';
	import { term } from '$lib/stores';
	import { getLayout } from '$lib/context/layout.svelte';
	import FileEditors from '$lib/components/editor/FileEditors.svelte';

	interface Props {
		project: Project;
		currentThreadID?: string;
	}

	let { project, currentThreadID }: Props = $props();
	const layout = getLayout();

	function onFileChanged(name: string, contents: string) {
		for (const item of layout.items) {
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
		{#if layout.items.length > 1 || (!layout.items[0]?.table && !layout.items[0]?.generic)}
			<div class="-mx-5 -mt-3 flex border-b-2 border-surface2 px-2 pb-2">
				<ul class="flex flex-1 flex-wrap gap-2 text-center text-sm">
					{#each layout.items as item}
						<li>
							<div
								role="none"
								class:selected={item.selected}
								onclick={() => {
									EditorService.select(layout.items, item.id);
								}}
								class="colors-surface1 group flex rounded-2xl px-4 py-3"
							>
								<div class="flex flex-1 items-center gap-2 ps-2">
									{#if item.table}
										<TableIcon class="h-5 w-5" />
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
										EditorService.remove(layout.items, item.id);
										if (layout.items.length === 0) {
											layout.fileEditorOpen = false;
										}
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
				<Controls navBar {project} />
			</div>
		{/if}

		<FileEditors {project} {currentThreadID} {onFileChanged} {onInvoke} bind:items={layout.items} />
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
