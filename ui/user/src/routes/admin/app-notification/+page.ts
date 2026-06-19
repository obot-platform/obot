import { handleRouteError } from '$lib/errors';
import { UserService } from '$lib/services';
import type { AppNotification } from '$lib/services/user/types';
import { defaultAppNotification } from '$lib/stores/appNotification.svelte';
import type { PageLoad } from './$types';

// Distinguishes the first client run (SSR hydration) from later client-side navigations.
let hasHydrated = false;

export const load: PageLoad = async ({ fetch, parent }) => {
	const { profile, appNotification: initialAppNotification } = await parent();
	let appNotification: AppNotification = defaultAppNotification;

	try {
		let response: AppNotification | undefined;

		if (import.meta.env.SSR && initialAppNotification) {
			response = initialAppNotification;
		} else if (!hasHydrated && initialAppNotification) {
			hasHydrated = true;
			response = initialAppNotification;
		} else {
			hasHydrated = true;
			response = await UserService.getAppNotification({ fetch });
		}

		appNotification = {
			...defaultAppNotification,
			...(response ?? {})
		};
	} catch (err) {
		handleRouteError(err, '/admin/app-notification', profile);
	}

	return {
		appNotification
	};
};
