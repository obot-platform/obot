import type { PageLoad } from './$types';

// Every signed-in user can follow their own requests; administrators see everyone's.
export const load: PageLoad = async () => {
	return {};
};
