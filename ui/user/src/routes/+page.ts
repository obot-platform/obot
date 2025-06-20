import { ChatService, getProfile, type AuthProvider, type ProjectTemplate } from '$lib/services';
import { Role } from '$lib/services/admin/types';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ fetch }) => {
	let authProviders: AuthProvider[] = [];
	let templates: ProjectTemplate[] = [];
	let profile;

	try {
		profile = await getProfile({ fetch });
	} catch (_err) {
		// unauthorized, no need to do anything with error
		authProviders = await ChatService.listAuthProviders({ fetch });
		templates = (await ChatService.listTemplates({ fetch })).items.filter(
			(template) => template.featured
		);
	}

	return {
		loggedIn: profile?.loaded ?? false,
		isAdmin: profile?.role === Role.ADMIN,
		authProviders,
		templates
	};
};
