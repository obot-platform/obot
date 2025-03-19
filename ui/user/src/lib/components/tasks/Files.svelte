<script lang="ts">
	import { FileText, Trash } from 'lucide-svelte/icons';
	import { ChatService, EditorService, type Files, type Project } from '$lib/services';
	import { Download, RotateCw } from 'lucide-svelte';
	import { onDestroy } from 'svelte';
	import Confirm from '$lib/components/Confirm.svelte';
	import { getLayout } from '$lib/context/layout.svelte';

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
	<div class="rounded-3xl bg-gray-50 p-5 dark:bg-gray-950">
		<div class="flex justify-between">
			<h4 class="mb-3 text-xl font-semibold">Files</h4>
			<button onclick={loadFiles}>
				<RotateCw class="h-4 w-4 {loading ? 'animate-spin' : ''}" />
			</button>
		</div>
		<p class="text-gray">
			Files are private to the task execution. On start of the task a copy of the global workspace
			files is made, but no changes are persisted back to the global workspace.
		</p>
		<ul class="space-y-4 px-3 py-6 text-sm">
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
							class="ms-2 hidden group-hover:block"
							onclick={() => {
								EditorService.download(layout.items, project, file.name, {
									taskID,
									runID
								});
							}}
						>
							<Download class="text-gray h-5 w-5" />
						</button>
						<button
							class="ms-2 hidden group-hover:block"
							onclick={() => {
								fileToDelete = file.name;
							}}
						>
							<Trash class="text-gray h-5 w-5" />
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
