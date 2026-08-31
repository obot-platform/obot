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

let hasHydratedAppNotification = false;

export const load: PageLoad = async ({ fetch, parent }) => {
	const {
		profile,
		version,
		appPreferences,
		appNotification: initialAppNotification
	} = await parent();

	let license: License | undefined;
	try {
		license = await UserService.getLicense({ fetch });
	} catch (err) {
		handleRouteError(err, '/admin/license', profile);
	}

	let appNotification: AppNotification = defaultAppNotification;
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
		handleRouteError(err, '/admin/app-notification', profile);
	}

	let k8sSettings: K8sSettings | undefined;
	if (version?.engine === 'kubernetes' && !version?.hideK8sDetails) {
		try {
			k8sSettings = await AdminService.listK8sSettings({ fetch });
		} catch (err) {
			handleRouteError(err, '/admin/server-scheduling', profile);
		}
	}

	let apiKeys: APIKey[] = [];
	let users: OrgUser[] = [];
	try {
		[apiKeys, users] = await Promise.all([
			ApiKeysService.listAllApiKeys({ fetch }),
			UserService.listUsers({ fetch })
		]);
	} catch (err) {
		handleRouteError(err, '/admin/agent-auth-scopes', profile);
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
