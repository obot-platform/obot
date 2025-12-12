import { ChatService } from '$lib/services';
import type { PageServerLoad } from './$types';
import { redirect } from '@sveltejs/kit';

export const prerender = false;
export const load: PageServerLoad = async ({ fetch }) => {
	const profile = await ChatService.getProfile({ fetch });
	if (profile.hasAdminAccess?.()) {
		redirect(302, '/admin/usage');
	}
};
