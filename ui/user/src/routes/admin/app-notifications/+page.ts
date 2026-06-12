import { handleRouteError } from '$lib/errors';
import { AdminService } from '$lib/services';
import type { AppNotifications } from '$lib/services/user/types';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ fetch, parent }) => {
	const { profile } = await parent();
	let appNotifications: AppNotifications = {
		banner: {
			enabled: false,
			text: '',
			dismissable: false,
			type: 'info'
		}
	};
	try {
		appNotifications = await AdminService.getAppNotifications({ fetch });
	} catch (err) {
		handleRouteError(err, '/admin/app-notifications', profile);
	}

	return {
		appNotifications
	};
};
