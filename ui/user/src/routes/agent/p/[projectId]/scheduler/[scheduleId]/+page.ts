import type { PageLoad } from './$types';

export const ssr = false;

export const load: PageLoad = async ({ params }) => {
	return {
		projectId: params.projectId,
		scheduleId: params.scheduleId
	};
};
