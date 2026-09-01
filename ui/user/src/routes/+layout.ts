import { dev } from '$app/environment';
import {
	UserService,
	type AppNotification,
	type AppPreferences,
	type DefaultModelAlias,
	type License,
	type Model,
	type Profile,
	type Version
} from '$lib/services';
import { compileAppPreferences } from '$lib/stores/appPreferences.svelte';
import type { LayoutLoad } from './$types';
import { redirect } from '@sveltejs/kit';

export const prerender = 'auto';
export const ssr = dev;

export const load: LayoutLoad = async ({ fetch, url }) => {
	const [versionResult, licenseResult, appPreferencesResult, profileResult] =
		await Promise.allSettled([
			UserService.getVersion({ fetch }),
			UserService.getLicense({ fetch }),
			UserService.listAppPreferences({ fetch }),
			UserService.getProfile({ fetch })
		]);

	const version: Version | undefined =
		versionResult.status === 'fulfilled' ? versionResult.value : undefined;
	const license: License | undefined =
		licenseResult.status === 'fulfilled' ? licenseResult.value : undefined;
	const appPreferences: AppPreferences =
		appPreferencesResult.status === 'fulfilled'
			? compileAppPreferences(appPreferencesResult.value)
			: compileAppPreferences();
	const profile: Profile =
		profileResult.status === 'fulfilled'
			? profileResult.value
			: {
					id: '',
					email: '',
					iconURL: '',
					role: 0,
					effectiveRole: 0,
					groups: [],
					unauthorized: true,
					username: ''
				};

	if (profile.requirePasswordChange && url.pathname !== '/change-password') {
		throw redirect(303, `/change-password?rd=${encodeURIComponent(url.pathname + url.search)}`);
	}

	let defaultModelAliases: DefaultModelAlias[] | undefined;
	let models: Model[] | undefined;
	let appNotification: AppNotification | undefined;

	// A restricted session is refused all three of these, and the password page renders without
	// them, so asking is three guaranteed 403s on every load.
	if (!profile.unauthorized && !profile.requirePasswordChange) {
		const [defaultModelAliasesResult, modelsResult, appNotificationResult] =
			await Promise.allSettled([
				UserService.listDefaultModelAliases({ fetch }),
				UserService.listModels({ fetch }),
				UserService.getAppNotification({ fetch })
			]);
		defaultModelAliases =
			defaultModelAliasesResult.status === 'fulfilled'
				? defaultModelAliasesResult.value
				: undefined;
		models = modelsResult.status === 'fulfilled' ? modelsResult.value : undefined;
		appNotification =
			appNotificationResult.status === 'fulfilled' ? appNotificationResult.value : undefined;
	}

	return {
		appPreferences,
		profile,
		version,
		license,
		defaultModelAliases,
		models,
		appNotification
	};
};
