<script lang="ts">
	import type { Chat } from '$lib/services/nanobot/types';
	import { onMount } from 'svelte';
	import Threads from '$lib/components/nanobot/Threads.svelte';
	import { getLayout } from '$lib/context/nanobotLayout.svelte';
	import { ChatAPI } from '$lib/services/nanobot/chat/index.svelte';

	interface Props {
		chatApi: ChatAPI;
	}

	let { chatApi }: Props = $props();

	let threads = $state<Chat[]>([]);
	let isLoading = $state(true);
	// const sidebar = getLayout();

	onMount(async () => {
		try {
			threads = await chatApi.getThreads();
		} finally {
			isLoading = false;
		}
	});

	async function handleRenameThread(threadId: string, newTitle: string) {
		try {
			await chatApi.renameThread(threadId, newTitle);
			const threadIndex = threads.findIndex((t) => t.id === threadId);
			if (threadIndex !== -1) {
				threads[threadIndex].title = newTitle;
			}
			// notifications.success('Thread Renamed', `Successfully renamed to "${newTitle}"`);
		} catch (error) {
			// notifications.error('Rename Failed', 'Unable to rename the thread. Please try again.');
			console.error('Failed to rename thread:', error);
		}
	}

	async function handleDeleteThread(threadId: string) {
		try {
			await chatApi.deleteThread(threadId);
			const threadToDelete = threads.find((t) => t.id === threadId);
			threads = threads.filter((t) => t.id !== threadId);
			// notifications.success('Thread Deleted', `Deleted "${threadToDelete?.title || 'thread'}"`);
		} catch (error) {
			// notifications.error('Delete Failed', 'Unable to delete the thread. Please try again.');
			console.error('Failed to delete thread:', error);
		}
	}
</script>

<div class="flex-1">
	<div class="flex h-full flex-col">
		<!-- Threads section (takes up ~40% of available space) -->
		<div class="flex-shrink-0">
			<Threads
				{threads}
				onRename={handleRenameThread}
				onDelete={handleDeleteThread}
				{isLoading}
				onThreadClick={() => {}}
			/>
		</div>
	</div>
</div>
