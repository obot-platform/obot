import type { PageLoad } from './$types';
import { redirect } from '@sveltejs/kit';

export const load: PageLoad = ({ url }) => {
	const searchParams = new URLSearchParams(url.searchParams);
	throw redirect(301, `/hosted-agents?view=access-policies&${searchParams}`);
};
