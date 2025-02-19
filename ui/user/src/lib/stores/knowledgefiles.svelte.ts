import { listKnowledgeFiles } from '$lib/services/chat/operations';
import type { KnowledgeFile } from '$lib/services/chat/types';
import { onInit } from '$lib/stores/context.svelte';

const store = $state({
	items: [] as KnowledgeFile[]
});

onInit(async () => {
	store.items = (await listKnowledgeFiles()).items;
});

export default store;
