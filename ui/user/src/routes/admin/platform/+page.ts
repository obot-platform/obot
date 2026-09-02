import { handleRouteError } from '$lib/errors';
import {
	AdminService,
	ApiKeysService,
	UserService,
	type AppNotification,
	type K8sSettings,
	type License,
	type OrgUser
} from '$lib/services';
import type { APIKey } from '$lib/services/api-keys/types';
import { defaultAppNotification } from '$lib/stores/appNotification.svelte';
import type { PageLoad } from './$types';

const views = new Set([
	'license',
	'branding',
	'notifications',
	'mcp-config',
	'registry-connections'
]);

let hasHydratedAppNotification = false;

export const load: PageLoad = async ({ fetch, parent, url }) => {
	const {
		profile,
		version,
		appPreferences,
		appNotification: initialAppNotification
	} = await parent();
	const requestedView = url.searchParams.get('view');
	const view = requestedView && views.has(requestedView) ? requestedView : 'license';

	let license: License | undefined;
	let appNotification: AppNotification = defaultAppNotification;
	let k8sSettings: K8sSettings | undefined;
	let apiKeys: APIKey[] = [];
	let users: OrgUser[] = [];

	switch (view) {
		case 'license':
			try {
				license = await UserService.getLicense({ fetch });
			} catch (err) {
				handleRouteError(err, '/admin/platform?view=license', profile);
			}
			break;
		case 'notifications':
			try {
				let response: AppNotification | undefined;

				if (import.meta.env.SSR && initialAppNotification) {
					response = initialAppNotification;
				} else if (!hasHydratedAppNotification && initialAppNotification) {
					hasHydratedAppNotification = true;
					response = initialAppNotification;
				} else {
					hasHydratedAppNotification = true;
					response = await UserService.getAppNotification({ fetch });
				}

				appNotification = {
					...defaultAppNotification,
					...(response ?? {})
				};
			} catch (err) {
				handleRouteError(err, '/admin/platform?view=notifications', profile);
			}
			break;
		case 'mcp-config':
			if (version?.engine === 'kubernetes' && !version?.hideK8sDetails) {
				try {
					k8sSettings = await AdminService.listK8sSettings({ fetch });
				} catch (err) {
					handleRouteError(err, '/admin/platform?view=mcp-config', profile);
				}
			}
			break;
		case 'registry-connections':
			try {
				[apiKeys, users] = await Promise.all([
					ApiKeysService.listAllApiKeys({ fetch }),
					UserService.listUsers({ fetch })
				]);
			} catch (err) {
				handleRouteError(err, '/admin/platform?view=registry-connections', profile);
			}
			break;
	}

	return {
		license,
		appPreferences,
		appNotification,
		k8sSettings,
		apiKeys,
		users
	};
};
