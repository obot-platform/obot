import type { PageLoad } from './$types';
import { redirect } from '@sveltejs/kit';

export const load: PageLoad = ({ url }) => {
	// Model access policies are LLM gateway profiles now.
	const searchParams = new URLSearchParams(url.searchParams);
	searchParams.set('view', 'profiles');
	throw redirect(301, `/models?${searchParams}`);
};
