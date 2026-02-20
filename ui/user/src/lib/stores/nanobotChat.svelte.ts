import type { ChatService } from '$lib/services/nanobot/chat/index.svelte';
import type { Chat, Resource } from '$lib/services/nanobot/types';
import { writable } from 'svelte/store';

export interface NanobotChat {
	projectId: string;
	threadId?: string;
	chat?: ChatService;
	threads: Chat[];
	isThreadsLoading: boolean;
	resources: Resource[];
}

/**
 * Storing nanobot chat data in a store so it can be accessed from anywhere in the app.
 */
export const nanobotChat = writable<NanobotChat | null>(null);
