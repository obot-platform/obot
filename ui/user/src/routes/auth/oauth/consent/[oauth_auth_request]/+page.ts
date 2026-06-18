import { UserService } from '$lib/services';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ params, fetch }) => {
	const consent = await UserService.getOAuthConsent(params.oauth_auth_request, { fetch });
	return {
		consent
	};
};
