import type { PageLoad } from './$types';

export const load: PageLoad = async ({ params }) => {
	return {
		project: params.project,
		thread: params.thread
	};
};
