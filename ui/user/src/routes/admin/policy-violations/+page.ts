import { UserService } from '$lib/services';
import type { PageLoad } from './$types';
import { redirect } from '@sveltejs/kit';

export const load: PageLoad = async ({ fetch }) => {
	const version = await UserService.getVersion({ fetch });
	if (!version.messagePoliciesEnabled) {
		throw redirect(302, '/admin');
	}

	return {};
};
