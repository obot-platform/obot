import { dev } from '$app/environment';
import {
	UserService,
	type AppPreferences,
	type DefaultModelAlias,
	type Profile,
	type Version
} from '$lib/services';
import { compileAppPreferences } from '$lib/stores/appPreferences.svelte';
import type { LayoutLoad } from './$types';

export const prerender = 'auto';
export const ssr = dev;

export const load: LayoutLoad = async ({ fetch }) => {
	let appPreferences: AppPreferences | undefined;
	let profile: Profile | undefined;
	let version: Version | undefined;
	let defaultModelAliases: DefaultModelAlias[] | undefined;

	try {
		version = await UserService.getVersion({ fetch });
	} catch {
		version = undefined;
	}

	try {
		const response = await UserService.listAppPreferences({ fetch });
		const response2 = await UserService.getProfile({ fetch });
		appPreferences = compileAppPreferences(response);
		profile = response2;
	} catch {
		// If the request fails, use default preferences
		appPreferences = compileAppPreferences();
	}

	try {
		profile = await UserService.getProfile({ fetch });
	} catch {
		profile = {
			id: '',
			email: '',
			iconURL: '',
			role: 0,
			effectiveRole: 0,
			groups: [],
			unauthorized: true,
			username: ''
		};
	}

	if (!profile.unauthorized) {
		try {
			defaultModelAliases = await UserService.listDefaultModelAliases({ fetch });
		} catch {
			defaultModelAliases = undefined;
		}
	}

	return {
		appPreferences,
		profile,
		version,
		defaultModelAliases
	};
};
