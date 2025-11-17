import { dev } from '$app/environment';
import { AdminService, type AppPreferences } from '$lib/services';
import { compileAppPreferences } from '$lib/stores/appPreferences.svelte';
import type { LayoutLoad } from './$types';

export const prerender = 'auto';
export const ssr = dev;

export const load: LayoutLoad = async ({ fetch }) => {
	let appPreferences: AppPreferences | undefined;
	try {
		const response = await AdminService.listAppPreferences({ fetch });
		appPreferences = compileAppPreferences(response);
	} catch {
		// If the request fails, use default preferences
		appPreferences = compileAppPreferences();
	}

	return {
		appPreferences
	};
};
