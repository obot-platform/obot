import { listProjects } from '$lib/services/chat/operations';
import { type Project } from '$lib/services/chat/types';

const store = $state({
	items: [] as Project[],
	reload: async () => {
		store.items = (await listProjects()).items;
	}
});

export default store;
