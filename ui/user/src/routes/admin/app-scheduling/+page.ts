import { handleRouteError } from '$lib/errors';
import { AdminService, type AppK8sSettings } from '$lib/services';
import type { PageLoad } from './$types';
import { redirect } from '@sveltejs/kit';

export const load: PageLoad = async ({ fetch, parent }) => {
	const { profile, version } = await parent();

	if (version?.engine !== 'kubernetes') {
		throw redirect(302, '/');
	}

	let k8sSettings: AppK8sSettings | undefined;
	try {
		k8sSettings = await AdminService.getAppK8sSettings({ fetch });
	} catch (err) {
		handleRouteError(err, '/admin/app-scheduling', profile);
	}

	if (!k8sSettings?.available) {
		throw redirect(302, '/');
	}

	return {
		k8sSettings
	};
};
