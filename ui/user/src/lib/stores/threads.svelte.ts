import { createThread, listThreads } from '$lib/services/chat/operations';
import type { Thread } from '$lib/services/chat/types';

const store = $state({
	items: [] as Thread[],
	createOrGetDefault
});

async function createOrGetDefault() {
	if (store.items.length === 0) {
		store.items = (await listThreads()).items;
	}
	if (store.items.length === 0) {
		store.items.push(await createThread());
	}
	return store.items[0];
}

export default store;
