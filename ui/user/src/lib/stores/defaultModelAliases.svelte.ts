import { UserService } from '$lib/services';
import type { DefaultModelAlias } from '$lib/services/user/types';

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
		try {
			const defaultModelAliases = await UserService.listDefaultModelAliases();
			store.current = defaultModelAliases;
		} finally {
			store.loading = false;
		}
	}
}

export default store;
