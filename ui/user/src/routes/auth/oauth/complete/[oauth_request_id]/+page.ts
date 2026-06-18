import type { PageLoad } from './$types';

export const load: PageLoad = ({ params }) => {
	return {
		redirectURL: `/oauth/complete/${encodeURIComponent(params.oauth_request_id)}`
	};
};
