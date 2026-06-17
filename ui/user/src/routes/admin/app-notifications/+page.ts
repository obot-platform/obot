import { handleRouteError } from '$lib/errors';
import { UserService } from '$lib/services';
import type { AppNotifications } from '$lib/services/user/types';
import { defaultAppNotifications } from '$lib/stores/appNotifications.svelte';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ fetch, parent }) => {
	const { profile } = await parent();
	let appNotifications: AppNotifications = defaultAppNotifications;
	try {
		const response = await UserService.getAppNotifications({ fetch });
		appNotifications = {
			...defaultAppNotifications,
			...response
		};
	} catch (err) {
		handleRouteError(err, '/admin/app-notifications', profile);
	}

	return {
		appNotifications
	};
};
