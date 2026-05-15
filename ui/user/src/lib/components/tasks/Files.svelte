<script lang="ts">
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import Confirm from '$lib/components/Confirm.svelte';
	import { getLayout } from '$lib/context/chatLayout.svelte';
	import { ChatService, EditorService, type Files, type Project } from '$lib/services';
	import IconButton from '../primitives/IconButton.svelte';
	import { Download, RotateCw } from 'lucide-svelte';
	import { FileText, Trash2 } from 'lucide-svelte/icons';
	import { onDestroy } from 'svelte';

	interface Props {
		taskID: string;
		runID: string;
		running?: boolean;
		project: Project;
	}

	let { taskID, runID, running, project }: Props = $props();
	let loading = $state(false);
	let fileToDelete: string | undefined = $state();
	let interval: ReturnType<typeof setInterval> | undefined;
	const layout = getLayout();

	async function loadFiles() {
		try {
			loading = true;
			files = await ChatService.listFiles(project.assistantID, project.id, {
				taskID,
				runID
			});
		} finally {
			loading = false;
		}
	}

	async function deleteFile() {
		if (!fileToDelete) {
			return;
		}
		await ChatService.deleteFile(project.assistantID, project.id, fileToDelete, {
			taskID,
			runID
		});
		await loadFiles();
		fileToDelete = undefined;
	}

	$effect(() => {
		if (running && !interval) {
			loadFiles();
			interval = setInterval(loadFiles, 5000);
		} else if (!running && interval) {
			clearInterval(interval);
			interval = undefined;
		}
	});

	$effect(() => {
		if (!files) {
			loadFiles();
		}
	});

	onDestroy(() => {
		if (interval) {
			clearInterval(interval);
		}
	});

	let files: Files | undefined = $state();
</script>

{#if files && files.items.length > 0}
	<div
		class="dark:bg-base-200 dark:border-base-400 bg-base-100 rounded-3xl p-5 shadow-md dark:border"
	>
		<div class="mb-3 flex items-center justify-between">
			<h4 class="text-xl font-semibold">Files</h4>
			<button onclick={loadFiles} use:tooltip={'Refresh Files'}>
				<RotateCw class="size-5 {loading ? 'animate-spin' : ''}" />
			</button>
		</div>
		<p class="text-gray">
			Files are private to the task execution. On start of the task a copy of the global workspace
			files is made, but no changes are persisted back to the global workspace.
		</p>
		<ul class="space-y-4 py-6 text-sm">
			{#each files.items as file (file.name)}
				<li class="group">
					<div class="flex">
						<button
							class="flex flex-1 items-center"
							onclick={async () => {
								await EditorService.load(layout.items, project, file.name, {
									taskID,
									runID
								});
								layout.fileEditorOpen = true;
							}}
						>
							<FileText />
							<span class="ms-3">{file.name}</span>
						</button>
						<IconButton
							class="ms-2 opacity-0 group-hover:opacity-100"
							onclick={() => {
								EditorService.download(layout.items, project, file.name, {
									taskID,
									runID
								});
							}}
							tooltip={{ text: 'Download File' }}
						>
							<Download class="text-muted-content size-5" />
						</IconButton>
						<IconButton
							class="ms-2 opacity-0 group-hover:opacity-100"
							onclick={() => {
								fileToDelete = file.name;
							}}
							tooltip={{ text: 'Delete File' }}
						>
							<Trash2 class="text-muted-content size-5" />
						</IconButton>
					</div>
				</li>
			{/each}
		</ul>
	</div>
{/if}

<Confirm
	show={fileToDelete !== undefined}
	msg={`Delete ${fileToDelete}?`}
	onsuccess={deleteFile}
	oncancel={() => (fileToDelete = undefined)}
/>
