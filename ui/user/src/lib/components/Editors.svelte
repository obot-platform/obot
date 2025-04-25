<script lang="ts">
	import { overflowToolTip } from '$lib/actions/overflow';
	import Controls from '$lib/components/editor/Controls.svelte';
	import FileEditors from '$lib/components/editor/FileEditors.svelte';
	import { getLayout } from '$lib/context/layout.svelte';
	import { ChatService, EditorService, type InvokeInput, type Project } from '$lib/services';
	import type { EditorItem } from '$lib/services/editor/index.svelte';
	import { Download } from 'lucide-svelte';
	import { X } from 'lucide-svelte/icons';
	import { slide } from 'svelte/transition';
	import { twMerge } from 'tailwind-merge';
	import Files from '$lib/components/edit/Files.svelte';

	interface Props {
		project: Project;
		currentThreadID?: string;
	}

	let { project, currentThreadID }: Props = $props();
	let saveTimeout: ReturnType<typeof setTimeout>;
	const layout = getLayout();

	let downloadable = $derived.by(() => {
		const selected = layout.items.find((item) => item.selected);

		// embedded pdf viewer has it's own download button
		if (selected?.name.toLowerCase().endsWith('.pdf')) {
			return false;
		}

		return !!selected?.file;
	});

	const debouncedSave = (item: EditorItem) => {
		// Clear previous timeout
		if (saveTimeout) clearTimeout(saveTimeout);

		// Set new timeout for debounced save
		saveTimeout = setTimeout(() => {
			EditorService.save(item, project, {
				taskID: item.file?.taskID,
				runID: item.file?.runID,
				threadID: item.file?.threadID
			});
		}, 300);
	};

	function onFileChanged(name: string, contents: string) {
		const item = layout.items.find((item) => item.name === name);
		if (item && item.file) {
			item.file.buffer = contents;
			item.file.modified = true;
			debouncedSave(item);
		}
	}

	async function onInvoke(invoke: InvokeInput) {
		if (currentThreadID) {
			await ChatService.invoke(project.assistantID, project.id, currentThreadID, invoke);
		}
	}
</script>

<div class="relative flex h-full w-full flex-col">
	{#if layout.items.length > 1 || (!layout.items[0]?.table && !layout.items[0]?.generic)}
		<div class="file-tabs relative flex items-center pt-1">
			{#if currentThreadID}
				<div class="pb-1 pl-1">
					<Files {project} thread {currentThreadID} primary={false} helperText={'Browse Files'} />
				</div>
			{/if}
			<ul
				class="default-scrollbar-thin relative mt-auto flex grow items-center gap-1 overflow-x-auto pl-1 text-center text-sm"
			>
				{#each layout.items as item (item.id)}
					<li class="flex max-w-[200px] min-w-[100px] flex-1" data-item-id={item.id}>
						<div
							role="none"
							onclick={() => {
								EditorService.select(layout.items, item.id);
							}}
							class={twMerge(
								'group bg-surface2 border-surface2 relative flex w-full cursor-pointer rounded-t-lg border-2 border-b-0 p-1 transition-colors duration-300',
								item.selected && 'border-surface2 bg-white dark:bg-black',
								!item.selected && 'hover:bg-surface3 hover:border-surface3'
							)}
							transition:slide={{ axis: 'x', duration: 200 }}
						>
							<div
								class="group/file relative flex w-full items-center justify-between gap-1 [&_svg]:size-4 [&_svg]:min-w-fit"
							>
								<span use:overflowToolTip class="truncate p-1">{item.name}</span>
								<button
									class="hover:bg-surface2 flex h-6 w-0 flex-shrink-0 items-center justify-center overflow-hidden rounded-full text-gray-500 transition-all duration-300 group-hover/file:w-6"
									class:w-6={item.selected}
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

			<Controls navBar {project} class="flex-shrink-0 px-2 pb-1" {currentThreadID} />
		</div>
	{/if}

	<div class="default-scrollbar-thin relative flex grow flex-col overflow-y-auto">
		<FileEditors {project} {currentThreadID} {onFileChanged} {onInvoke} bind:items={layout.items} />

		{#if downloadable}
			<button
				class="icon-button absolute top-2 right-2"
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
</div>

<style lang="postcss">
	.file-tabs {
		&:after {
			position: absolute;
			content: '';
			bottom: 0;
			left: 0;
			width: 100%;
			height: 2px;
			background-color: var(--surface2);
			z-index: -10;
		}
	}
</style>
