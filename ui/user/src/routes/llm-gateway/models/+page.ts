import type { PageLoad } from './$types';
import { redirect } from '@sveltejs/kit';

export const load: PageLoad = ({ url }) => {
	const searchParams = new URLSearchParams(url.searchParams);
	searchParams.set('view', 'models');
	throw redirect(301, `/models?${searchParams}`);
};
