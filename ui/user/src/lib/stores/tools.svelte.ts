import { listTools } from '$lib/services/chat/operations';
import { type AssistantTool } from '$lib/services/chat/types';
import context from '$lib/stores/context';

const store = $state({
	items: [] as AssistantTool[]
});

context.init(async () => {
	store.items = (await listTools()).items;
});

export default store;
