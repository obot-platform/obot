import type { PageLoad } from './$types';
import { redirect } from '@sveltejs/kit';

export const load: PageLoad = ({ params }) => {
	const searchParams = new URLSearchParams({ view: 'providers', provider: params.name });
	throw redirect(307, `/models?${searchParams}`);
};
