import { listTools } from '$lib/services/chat/operations';
import { type AssistantToolList } from '$lib/services/chat/types';
import assistants from '$lib/stores/assistants';
import { writable } from 'svelte/store';

const store = writable<AssistantToolList>({
	readonly: true,
	items: []
});

let initialized = false;

if (typeof window !== 'undefined') {
	assistants.subscribe(async (assistants) => {
		if (initialized) {
			return;
		}
		for (const assistant of assistants) {
			if (assistant.current && assistant.id) {
				store.set(await listTools(assistant.id));
				initialized = true;
				break;
			}
		}
	});
}

export default store;
