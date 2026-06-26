import { handleRouteError } from '$lib/errors';
import { AdminService, type K8sSettings } from '$lib/services';
import { profile } from '$lib/stores';
import type { PageLoad } from '../$types';
import { redirect } from '@sveltejs/kit';

export const load: PageLoad = async ({ fetch, parent }) => {
	const { version } = await parent();
	if (version?.engine !== 'kubernetes' || version?.hideMcpK8sDetails) {
		throw redirect(302, '/');
	}

	let k8sSettings: K8sSettings | undefined;
	try {
		k8sSettings = await AdminService.listMcpServersK8sSettings({ fetch });
	} catch (err) {
		handleRouteError(err, '/admin/chat-configuration', profile.current);
	}

	return {
		k8sSettings
	};
};
