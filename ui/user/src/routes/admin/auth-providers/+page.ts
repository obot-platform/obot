import type { PageLoad } from './$types';
import { redirect } from '@sveltejs/kit';

export const load: PageLoad = ({ url }) => {
	const searchParams = new URLSearchParams(url.searchParams);
	searchParams.set('view', 'auth-providers');
	throw redirect(301, `/identity-access?${searchParams}`);
};
