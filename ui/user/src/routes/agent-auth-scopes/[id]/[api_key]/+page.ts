import type { PageLoad } from './$types';
import { redirect } from '@sveltejs/kit';

export const load: PageLoad = ({ params, url }) => {
	throw redirect(
		301,
		`/identity-access/agents/${encodeURIComponent(params.id)}/${encodeURIComponent(params.api_key)}${url.search}`
	);
};
