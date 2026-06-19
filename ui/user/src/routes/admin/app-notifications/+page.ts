import { handleRouteError } from '$lib/errors';
import { UserService } from '$lib/services';
import type { AppNotifications } from '$lib/services/user/types';
import { defaultAppNotifications } from '$lib/stores/appNotifications.svelte';
import type { PageLoad } from './$types';

// Distinguishes the first client run (SSR hydration) from later client-side navigations.
let hasHydrated = false;

export const load: PageLoad = async ({ fetch, parent }) => {
	const { profile, appNotifications: initialAppNotifications } = await parent();
	let appNotifications: AppNotifications = defaultAppNotifications;

	try {
		let response: AppNotifications | undefined;

		if (import.meta.env.SSR && initialAppNotifications) {
			response = initialAppNotifications;
		} else if (!hasHydrated && initialAppNotifications) {
			hasHydrated = true;
			response = initialAppNotifications;
		} else {
			hasHydrated = true;
			response = await UserService.getAppNotifications({ fetch });
		}

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
