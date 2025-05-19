import {
	ChatService,
	getProfile,
	type AuthProvider,
	type Project,
	type ProjectTemplate
} from '$lib/services';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ fetch }) => {
	let editorProjects: Project[] = [];
	let authProviders: AuthProvider[] = [];
	let templates: ProjectTemplate[] = [];
	let profile;

	try {
		profile = await getProfile({ fetch });
		editorProjects = (await ChatService.listProjects({ fetch })).items;
	} catch (_err) {
		// unauthorized, no need to do anything with error
		authProviders = await ChatService.listAuthProviders({ fetch });
		templates = (await ChatService.listTemplates({ fetch })).items.filter(
			(template) => template.featured
		);
	}

	return {
		loggedIn: profile?.loaded ?? false,
		editorProjects,
		authProviders,
		templates
	};
};
