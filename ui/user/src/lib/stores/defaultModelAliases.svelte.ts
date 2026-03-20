import { ChatService } from '$lib/services';
import type { DefaultModelAlias } from '$lib/services/chat/types';

const store = $state<{
	current: DefaultModelAlias[];
	loading: boolean;
	initialize: (defaultModelAliases?: DefaultModelAlias[]) => Promise<void>;
}>({
	current: [],
	loading: false,
	initialize
});

async function initialize(defaultModelAliases?: DefaultModelAlias[]) {
	if (defaultModelAliases) {
		store.current = defaultModelAliases;
	} else {
		store.loading = true;
		const defaultModelAliases = await ChatService.listDefaultModelAliases();
		store.current = defaultModelAliases;
		store.loading = false;
	}
}

export default store;
