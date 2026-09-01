import type { PageLoad } from './$types';
import { redirect } from '@sveltejs/kit';

export const load: PageLoad = ({ params, url }) => {
	throw redirect(301, `/mcp-servers/s/${encodeURIComponent(params.id)}/details${url.search}`);
};
