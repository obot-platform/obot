<script lang="ts">
	import { popover } from '$lib/actions';
	import Controls from '$lib/components/editor/Controls.svelte';
	import FileEditors from '$lib/components/editor/FileEditors.svelte';
	import Terminal from '$lib/components/terminal/Terminal.svelte';
	import { getLayout } from '$lib/context/layout.svelte';
	import { ChatService, EditorService, type InvokeInput, type Project } from '$lib/services';
	import { term } from '$lib/stores';
	import { Download } from 'lucide-svelte';
	import { X } from 'lucide-svelte/icons';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		project: Project;
		currentThreadID?: string;
	}

	let { project, currentThreadID }: Props = $props();
	const layout = getLayout();

	let downloadable = $derived.by(() => {
		const selected = layout.items.find((item) => item.selected);
		return !!selected?.file;
	});

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

<div class="relative flex h-full flex-col">
	{#if layout.fileEditorOpen}
		{#if layout.items.length > 1 || (!layout.items[0]?.table && !layout.items[0]?.generic)}
			<div class="relative flex items-center border-b-2 border-surface2">
				<ul class="relative flex flex-1 items-center gap-1 pb-2 text-center text-sm">
					{#each layout.items as item (item.id)}
						{@const tt = popover({ hover: true, placement: 'top' })}
						<p use:tt.tooltip class="rounded-full bg-surface2 p-2">
							{item.name}
						</p>

						<li class="flex-1">
							<!-- TODO: div with onclick is not accessible, we'll need to update this in the future -->
							<div
								use:tt.ref
								role="none"
								onclick={() => {
									EditorService.select(layout.items, item.id);
								}}
								class={twMerge(
									'group relative flex cursor-pointer rounded-lg border-transparent bg-surface1 p-1 hover:bg-surface3',
									item.selected && 'bg-surface3'
								)}
							>
								<div
									class="relative flex w-full items-center justify-between gap-1 [&_svg]:size-4 [&_svg]:min-w-fit"
								>
									<span class="line-clamp-1 break-all p-1">{item.name}</span>

									<button
										class={twMerge(
											'right-0 hidden rounded-lg p-1 group-hover:block',
											item.selected
												? 'bg-surface3 hover:bg-surface2'
												: 'bg-surface1 hover:bg-surface3'
										)}
										onclick={() => {
											EditorService.remove(layout.items, item.id);
											if (layout.items.length === 0) {
												layout.fileEditorOpen = false;
											}
										}}
									>
										<X />
									</button>
								</div>
							</div>
						</li>
					{/each}
				</ul>

				<Controls navBar {project} class="bg-background px-2" {currentThreadID} />
			</div>
		{/if}

		<div class="relative flex h-full flex-col overflow-hidden">
			<div class="default-scrollbar-thin relative flex-1">
				<FileEditors
					{project}
					{currentThreadID}
					{onFileChanged}
					{onInvoke}
					bind:items={layout.items}
				/>
			</div>

			{#if downloadable}
				<button
					class="icon-button absolute right-5 top-5"
					onclick={() => {
						const selected = layout.items.find((item) => item.selected);
						if (selected) {
							EditorService.download(layout.items, project, selected.name, {
								taskID: selected.file?.taskID,
								runID: selected.file?.runID,
								threadID: selected.file?.threadID
							});
						}
					}}
				>
					<Download class="h-5 w-5" />
				</button>
			{/if}
		</div>
	{/if}
	{#if term.open}
		<div
			class={layout.fileEditorOpen
				? '-mb-3 -ml-5 h-1/2 border-t-4 border-surface1 px-2 pt-2'
				: 'h-full'}
		>
			<Terminal {project} />
		</div>
	{/if}
</div>
