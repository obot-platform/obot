import type { PageLoad } from './$types';
import { redirect } from '@sveltejs/kit';

export const load: PageLoad = ({ url }) => {
	const searchParams = new URLSearchParams(url.searchParams);
	searchParams.delete('view');
	throw redirect(301, `/admin/platform?view=registry-connections&${searchParams}`);
};
