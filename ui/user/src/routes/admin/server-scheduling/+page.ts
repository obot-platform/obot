import { handleRouteError } from '$lib/errors';
import { AdminService, type ObotK8sSettings } from '$lib/services';
import type { PageLoad } from '../$types';

export const load: PageLoad = async ({ fetch, parent }) => {
	const { profile } = await parent();
	let k8sSettings: ObotK8sSettings | undefined;
	try {
		k8sSettings = await AdminService.listObotK8sSettings({ fetch });
	} catch (err) {
		handleRouteError(err, '/admin/server-scheduling', profile);
	}

	return {
		k8sSettings
	};
};
