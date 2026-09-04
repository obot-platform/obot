import type { PageLoad } from './$types';

export const load: PageLoad = async ({ parent }) => {
	await parent();
};
