import type { PageLoad } from './$types';
import { redirect } from '@sveltejs/kit';

export const load: PageLoad = ({ params, url }) => {
	throw redirect(
		301,
		`/mcp-servers/c/${encodeURIComponent(params.id)}/instance/${encodeURIComponent(params.ms_id)}${url.search}`
	);
};
