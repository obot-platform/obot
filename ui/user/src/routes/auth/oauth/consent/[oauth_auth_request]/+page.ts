import { handleRouteError } from '$lib/errors';
import { UserService } from '$lib/services';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ params, fetch, parent }) => {
	const { profile } = await parent();
	let consent;

	try {
		consent = await UserService.getOAuthConsent(params.oauth_auth_request, { fetch });
	} catch (err) {
		handleRouteError(err, `/auth/oauth/consent/${params.oauth_auth_request}`, profile);
	}

	return {
		consent
	};
};
