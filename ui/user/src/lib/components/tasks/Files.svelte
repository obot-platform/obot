<script lang="ts">
	import { FileText } from '$lib/icons';
	import { currentAssistant } from '$lib/stores';
	import { ChatService, EditorService, type Files } from '$lib/services';
	import { RotateCw } from 'lucide-svelte';
	import { onDestroy } from 'svelte';

	interface Props {
		taskID: string;
		runID: string;
		running?: boolean;
	}

	let { taskID, runID, running }: Props = $props();
	let loading = $state(false);
	let interval: number;

	async function loadFiles() {
		try {
			loading = true;
			files = await ChatService.listFiles($currentAssistant.id, {
				taskID,
				runID
			});
		} finally {
			loading = false;
		}
	}

	$effect(() => {
		if (running && !interval) {
			interval = setInterval(loadFiles, 5000);
		} else if (!running && interval) {
			clearInterval(interval);
			interval = 0;
		}
	});

	$effect(() => {
		if (!files && $currentAssistant.id) {
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
	<div class="mt-8 rounded-3xl bg-gray-50 p-5 dark:bg-gray-950">
		<div class="flex justify-between">
			<h4 class="mb-3 text-xl font-semibold">Files</h4>
			<button onclick={loadFiles}>
				<RotateCw class="h-4 w-4 {loading ? 'animate-spin' : ''}" />
			</button>
		</div>
		<ul class="space-y-4 px-3 py-6 text-sm">
			{#each files.items as file}
				<li class="group">
					<div class="flex">
						<button
							class="flex flex-1 items-center"
							onclick={async () => {
								await EditorService.load($currentAssistant.id, file.name, {
									taskID,
									runID
								});
							}}
						>
							<FileText />
							<span class="ms-3">{file.name}</span>
						</button>
					</div>
				</li>
			{/each}
		</ul>
	</div>
{/if}
