import type { PageLoad } from './$types';
import { redirect } from '@sveltejs/kit';

export const load: PageLoad = ({ url }) => {
	const searchParams = new URLSearchParams(url.searchParams);
	searchParams.set('view', 'device-mcp-servers');
	throw redirect(301, `/admin/devices?${searchParams}`);
};
