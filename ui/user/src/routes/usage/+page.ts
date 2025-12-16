import type { PageLoad } from '../$types';
import { redirect } from '@sveltejs/kit';

export const load: PageLoad = async ({ parent }) => {
	const { profile } = await parent();
	if (profile?.hasAdminAccess?.()) {
		throw redirect(302, '/admin/usage');
	}
};
