import { handleRouteError } from '$lib/errors';
import { ChatService, type ProjectTemplate } from '$lib/services';
import { profile } from '$lib/stores';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ params, fetch }) => {
	let template: ProjectTemplate | undefined;

	try {
		template = await ChatService.getTemplate(params.id, { fetch });
	} catch (e) {
		handleRouteError(e, `/t/${params.id}`, profile.current);
	}

	return {
		template
	};
};
