import { listTools } from '$lib/services/chat/operations';
import { type AssistantTool } from '$lib/services/chat/types';
import { onInit } from '$lib/stores/context.svelte';

const store = $state({
	items: [] as AssistantTool[],
	hasTool
});

function hasTool(tool: string) {
	for (const t of store.items) {
		if (t.id === tool) {
			return t.enabled || t.builtin;
		}
	}
	return false;
}

onInit(async () => {
	store.items = (await listTools()).items;
});

export default store;
