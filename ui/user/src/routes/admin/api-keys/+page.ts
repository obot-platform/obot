import { redirect } from '@sveltejs/kit';

export const load = async ({ url }) => {
	throw redirect(301, `/admin/agent-auth-scopes${url.search}`);
};
