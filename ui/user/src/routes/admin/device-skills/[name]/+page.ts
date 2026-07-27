import type { PageLoad } from './$types';
import { redirect } from '@sveltejs/kit';

export const load: PageLoad = ({ params, url }) => {
	throw redirect(301, `/admin/devices/skills/${encodeURIComponent(params.name)}${url.search}`);
};
