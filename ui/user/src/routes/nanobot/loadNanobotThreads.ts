import type { ChatAPI } from '$lib/services/nanobot/chat/index.svelte';
import { nanobotChat } from '$lib/stores/nanobotChat.svelte';
import { get } from 'svelte/store';

/**
 * Ensures nanobot threads are loaded for the given project and chat API.
 * Idempotent: if store already has threads and is not loading, does nothing.
 */
export async function loadNanobotThreads(
	chatApi: ChatAPI,
	projectId: string,
	threadId?: string
): Promise<void> {
	const storedChat = get(nanobotChat);
	if (storedChat && !storedChat.isThreadsLoading) {
		return;
	}

	if (!storedChat) {
		nanobotChat.set({
			isThreadsLoading: true,
			projectId,
			threadId,
			threads: [],
			resources: []
		});
	}

	const threads = await chatApi.getThreads();

	nanobotChat.update((data) => {
		if (data) {
			data.threads = threads ?? [];
			data.isThreadsLoading = false;
		}
		return data;
	});
}
