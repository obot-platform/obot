import { listKnowledgeFiles } from '$lib/services/chat/operations';
import type { KnowledgeFile } from '$lib/services/chat/types';
import context from '$lib/stores/context';

const store = $state({
	items: [] as KnowledgeFile[]
});

context.init(async () => {
	store.items = (await listKnowledgeFiles()).items;
});

export default store;
