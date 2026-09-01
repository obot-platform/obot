import type { PageLoad } from './$types';
import { redirect } from '@sveltejs/kit';

export const load: PageLoad = async ({ parent }) => {
	const { profile } = await parent();
	if (!profile?.loaded) {
		throw redirect(303, '/');
	}
	if (!profile.requirePasswordChange) {
		throw redirect(303, '/');
	}
};
