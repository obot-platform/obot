<script lang="ts">
	import type { Chat } from '$lib/services/nanobot/types';
	import { onMount } from 'svelte';
	import Threads from '$lib/components/nanobot/Threads.svelte';
	import { ChatAPI } from '$lib/services/nanobot/chat/index.svelte';

	interface Props {
		chatApi: ChatAPI;
		projectId: string;
	}

	let { chatApi, projectId }: Props = $props();

	let threads = $state<Chat[]>([]);
	let isLoading = $state(true);

	async function refreshThreads() {
		threads = await chatApi.getThreads();
	}

	// Expose refreshThreads for parent components
	export { refreshThreads };

	onMount(async () => {
		try {
			await refreshThreads();
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
		} catch (error) {
			console.error('Failed to rename thread:', error);
		}
	}

	async function handleDeleteThread(threadId: string) {
		try {
			await chatApi.deleteThread(threadId);
			threads = threads.filter((t) => t.id !== threadId);
		} catch (error) {
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
				{projectId}
			/>
		</div>
	</div>
</div>
