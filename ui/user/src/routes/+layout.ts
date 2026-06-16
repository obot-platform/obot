import { dev } from '$app/environment';
import {
	UserService,
	type AppPreferences,
	type DefaultModelAlias,
	type License,
	type Model,
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
	let license: License | undefined;
	let defaultModelAliases: DefaultModelAlias[] | undefined;
	let models: Model[] | undefined;
	try {
		version = await UserService.getVersion({ fetch });
	} catch {
		version = undefined;
	}

	try {
		license = await UserService.getLicense({ fetch });
	} catch {
		license = undefined;
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

		try {
			models = await UserService.listModels({ fetch });
		} catch {
			models = undefined;
		}
	}

	return {
		appPreferences,
		profile,
		version,
		license,
		defaultModelAliases,
		models
	};
};
