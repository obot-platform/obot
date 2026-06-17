import { UserService } from '$lib/services';
import type { AppNotifications } from '$lib/services/user/types';

const store = $state<{
	current?: AppNotifications;
	loading: boolean;
	initialize: (appNotifications?: AppNotifications) => Promise<void>;
}>({
	current: undefined,
	loading: false,
	initialize
});

async function initialize(appNotifications?: AppNotifications) {
	if (appNotifications) {
		store.current = appNotifications;
		return;
	}

	store.loading = true;
	try {
		store.current = await UserService.getAppNotifications();
	} catch(_err) {
		store.current = undefined;
	} finally {
		store.loading = false;
	}
}

export default store;
