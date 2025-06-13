<script lang="ts">
	import { FileText, Trash2 } from 'lucide-svelte/icons';
	import { ChatService, EditorService, type Files, type Project } from '$lib/services';
	import { Download, RotateCw } from 'lucide-svelte';
	import { onDestroy } from 'svelte';
	import Confirm from '$lib/components/Confirm.svelte';
	import { getLayout } from '$lib/context/chatLayout.svelte';
	import { tooltip } from '$lib/actions/tooltip.svelte';

	interface Props {
		taskID: string;
		runID: string;
		running?: boolean;
		project: Project;
	}

	let { taskID, runID, running, project }: Props = $props();
	let loading = $state(false);
	let fileToDelete: string | undefined = $state();
	let interval: number;
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
			interval = 0;
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
	<div class="dark:bg-surface1 dark:border-surface3 rounded-3xl bg-white p-5 shadow-md dark:border">
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
			{#each files.items as file}
				<li class="group">
					<div class="flex">
						<button
							class="flex flex-1 items-center"
							onclick={async () => {
								await EditorService.load(layout.items, project, file.name, {
									taskID,
									runID
								});
							}}
						>
							<FileText />
							<span class="ms-3">{file.name}</span>
						</button>
						<button
							class="icon-button ms-2 opacity-0 group-hover:opacity-100"
							onclick={() => {
								EditorService.download(layout.items, project, file.name, {
									taskID,
									runID
								});
							}}
							use:tooltip={'Download File'}
						>
							<Download class="text-gray size-5" />
						</button>
						<button
							class="icon-button ms-2 opacity-0 group-hover:opacity-100"
							onclick={() => {
								fileToDelete = file.name;
							}}
							use:tooltip={'Delete File'}
						>
							<Trash2 class="text-gray size-5" />
						</button>
					</div>
				</li>
			{/each}
		</ul>
	</div>
{/if}

<Confirm
	show={fileToDelete !== undefined}
	msg={`Are you sure you want to delete ${fileToDelete}?`}
	onsuccess={deleteFile}
	oncancel={() => (fileToDelete = undefined)}
/>
