import type { PageLoad } from './$types';

// Obot AI is for every signed-in user; its tools follow the user's own permissions.
export const load: PageLoad = async () => {
	return {};
};
