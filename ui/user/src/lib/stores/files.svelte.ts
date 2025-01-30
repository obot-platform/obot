import { listFiles } from '$lib/services/chat/operations';
import { type File } from '$lib/services/chat/types';
import context from '$lib/stores/context';

const store = $state({
	items: [] as File[]
});

context.init(async () => {
	store.items = (await listFiles()).items;
});

export default store;
