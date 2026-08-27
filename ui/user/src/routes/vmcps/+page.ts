import type { PageLoad } from './$types';
import { redirect } from '@sveltejs/kit';

export const load: PageLoad = async ({ parent }) => {
	// temporarily only for admins due to using composite catalog entries
	const { profile } = await parent();
	if (!profile.isAdmin?.()) {
		throw redirect(307, '/');
	}
};
