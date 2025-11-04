import { handleRouteError } from '$lib/errors';
import { AdminService, type K8sSettings } from '$lib/services';
import { profile } from '$lib/stores';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ fetch }) => {
	let k8sSettings: K8sSettings | undefined;
	try {
		k8sSettings = await AdminService.listK8sSettings({ fetch });
	} catch (err) {
		handleRouteError(err, '/admin/chat-configuration', profile.current);
	}

	return {
		k8sSettings
	};
};
