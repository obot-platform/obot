import { profile } from '$lib/stores';
import type { PageLoad } from './$types';
import { redirect } from '@sveltejs/kit';

export const load: PageLoad = async () => {
	if (profile.current?.isAdmin?.()) {
		throw redirect(302, '/admin/usage');
	}
};
