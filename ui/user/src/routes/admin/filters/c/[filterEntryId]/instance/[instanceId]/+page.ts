import type { PageLoad } from './$types';
import { redirect } from '@sveltejs/kit';

export const load: PageLoad = ({ params, url }) => {
	throw redirect(
		301,
		`/mcp-servers/filters/c/${encodeURIComponent(params.filterEntryId)}/instance/${encodeURIComponent(params.instanceId)}${url.search}`
	);
};
