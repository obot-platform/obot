import { handleRouteError } from '$lib/errors';
import { AdminService, type K8sSettings } from '$lib/services';
import type { PageLoad } from './$types';
import { redirect } from '@sveltejs/kit';

export const load: PageLoad = async ({ fetch, parent }) => {
	const { profile, version } = await parent();

	if (version?.engine !== 'kubernetes' || version?.hideK8sDetails) {
		throw redirect(302, '/');
	}

	let k8sSettings: K8sSettings | undefined;
	try {
		k8sSettings = await AdminService.listK8sSettings({ fetch });
	} catch (err) {
		handleRouteError(err, '/admin/chat-configuration', profile);
	}

	return {
		k8sSettings
	};
};
