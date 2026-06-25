import { UserService } from '$lib/services';
import type { AppNotification } from '$lib/services/user/types';

export const defaultAppNotification: AppNotification = {
	banner: {
		enabled: false,
		text: '',
		dismissible: false,
		type: 'info',
		resetDismissed: false
	}
};

const store = $state<{
	current?: AppNotification;
	loading: boolean;
	initialize: (appNotification?: AppNotification) => Promise<void>;
}>({
	current: undefined,
	loading: false,
	initialize
});

async function initialize(appNotification?: AppNotification) {
	if (appNotification) {
		store.current = appNotification;
		return;
	}

	store.loading = true;
	try {
		store.current = await UserService.getAppNotification();
	} catch (_err) {
		store.current = undefined;
	} finally {
		store.loading = false;
	}
}

export default store;
