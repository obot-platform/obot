import type { PageLoad } from './$types';

export const load: PageLoad = ({ params }) => ({ authRequestID: params.oauth_auth_request });
