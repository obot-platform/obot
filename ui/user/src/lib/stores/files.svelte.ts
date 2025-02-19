import { listFiles } from '$lib/services/chat/operations';
import { type File } from '$lib/services/chat/types';
import { onInit } from '$lib/stores/context.svelte';

const store = $state({
	items: [] as File[]
});

onInit(async () => {
	store.items = (await listFiles()).items;
});

export default store;
