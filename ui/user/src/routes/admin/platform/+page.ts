import { handleRouteError } from '$lib/errors';
import {
	AdminService,
	UserService,
	type AppNotification,
	type GitCredential,
	type ImagePullSecret,
	type ImagePullSecretCapability,
	type K8sSettings,
	type License
} from '$lib/services';
import { defaultAppNotification } from '$lib/stores/appNotification.svelte';
import type { PageLoad } from './$types';

const views = new Set([
	'license',
	'branding',
	'notifications',
	'mcp-config',
	'registry-connections',
	'git-credentials'
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
	let capability: ImagePullSecretCapability = { available: false };
	let imagePullSecrets: ImagePullSecret[] = [];
	let gitCredentials: GitCredential[] = [];

	switch (view) {
		case 'git-credentials':
			gitCredentials = await AdminService.listGitCredentials({
				fetch,
				dontLogErrors: true
			}).catch(() => []);
			break;
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
				[capability, imagePullSecrets] = await Promise.all([
					AdminService.getImagePullSecretCapability({ fetch }),
					AdminService.listImagePullSecrets({ fetch })
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
		capability,
		imagePullSecrets,
		gitCredentials
	};
};
