import { handleRouteError } from '$lib/errors';
import { UserService } from '$lib/services';
import type { AppNotifications } from '$lib/services/user/types';
import { defaultAppNotifications } from '$lib/stores/appNotifications.svelte';
import type { PageLoad } from './$types';

let isInitialLoad = true;
export const load: PageLoad = async ({ fetch, parent }) => {
	const { profile, appNotifications: initialAppNotifications } = await parent();
	let appNotifications: AppNotifications = defaultAppNotifications;
	try {
		const response =
			isInitialLoad && initialAppNotifications
				? initialAppNotifications
				: await UserService.getAppNotifications({ fetch });
		isInitialLoad = false;
		appNotifications = {
			...defaultAppNotifications,
			...(response ?? {})
		};
	} catch (err) {
		handleRouteError(err, '/admin/app-notifications', profile);
	}

	return {
		appNotifications
	};
};
