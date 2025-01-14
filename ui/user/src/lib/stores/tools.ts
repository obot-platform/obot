import { listTools } from '$lib/services/chat/operations';
import { type AssistantToolList } from '$lib/services/chat/types';
import currentAssistant from '$lib/stores/currentassistant';
import { writable } from 'svelte/store';

const store = writable<AssistantToolList>({
	readonly: true,
	items: []
});

let initialized = false;

if (typeof window !== 'undefined') {
	currentAssistant.subscribe(async (assistant) => {
		if (initialized) {
			return;
		}
		if (assistant.id) {
			store.set(await listTools(assistant.id));
			initialized = true;
		}
	});
}

export default store;
