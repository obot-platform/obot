import type { ChatService } from '$lib/services/nanobot/chat/index.svelte';
import { writable } from 'svelte/store';

export interface NanobotChat {
	projectId: string;
	threadId: string;
	chat: ChatService;
}

/**
 * When the user creates a thread on the nanobot index page, we store the
 * existing ChatService here and navigate to /nanobot/p/{projectId}?tid={threadId}.
 * The project page claims this chat so the UI doesn't refetch or flash.
 */
export const nanobotChat = writable<NanobotChat | null>(null);
