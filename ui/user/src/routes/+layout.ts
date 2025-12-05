import { dev } from '$app/environment';
import {
	AdminService,
	ChatService,
	type AppPreferences,
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

	try {
		version = await ChatService.getVersion({ fetch });
	} catch {
		version = undefined;
	}

	try {
		const response = await AdminService.listAppPreferences({ fetch });
		const response2 = await ChatService.getProfile({ fetch });
		appPreferences = compileAppPreferences(response);
		profile = response2;
	} catch {
		// If the request fails, use default preferences
		appPreferences = compileAppPreferences();
	}

	try {
		profile = await ChatService.getProfile({ fetch });
	} catch {
		profile = {
			id: '',
			email: '',
			iconURL: '',
			role: 0,
			groups: [],
			unauthorized: true,
			username: ''
		};
	}

	return {
		appPreferences,
		profile,
		version
	};
};
