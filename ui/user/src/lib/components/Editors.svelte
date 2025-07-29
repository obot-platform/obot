<script lang="ts">
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import FileEditors from '$lib/components/editor/FileEditors.svelte';
	import { getLayout } from '$lib/context/chatLayout.svelte';
	import { ChatService, EditorService, type InvokeInput, type Project } from '$lib/services';
	import type { EditorItem } from '$lib/services/editor/index.svelte';
	import { Download } from 'lucide-svelte';
	import { X } from 'lucide-svelte/icons';

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
	{#if layout.items.length > 1 || !layout.items[0]?.generic}
		<div class="file-tabs relative flex items-center justify-between gap-2 p-2">
			<h4 class="px-2 text-base font-semibold text-gray-400 dark:text-gray-600">
				{layout.items.find((item) => item.selected)?.name}
			</h4>
			<div class="flex items-center gap-2">
				{#if downloadable}
					<button
						class="icon-button"
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
						use:tooltip={'Download File'}
					>
						<Download class="h-5 w-5" />
					</button>
				{/if}
				<button
					class="icon-button"
					onclick={() => {
						layout.fileEditorOpen = false;
					}}
					use:tooltip={'Close Editor'}
				>
					<X class="h-5 w-5" />
				</button>
			</div>
		</div>
	{/if}

	<div class="default-scrollbar-thin relative flex grow flex-col overflow-y-auto">
		<FileEditors {onFileChanged} {onInvoke} bind:items={layout.items} />
	</div>
</div>

<style lang="postcss">
</style>
