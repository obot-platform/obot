import type { PageLoad } from './$types';
import { redirect } from '@sveltejs/kit';

export const load: PageLoad = ({ params, url }) => {
	throw redirect(
		301,
		`/mcp-servers/access-policies/w/${encodeURIComponent(params.wid)}/r/${encodeURIComponent(params.id)}${url.search}`
	);
};
