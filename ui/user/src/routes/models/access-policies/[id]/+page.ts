import { profileUrl } from '$lib/services/llm-gateway/profiles';
import type { PageLoad } from './$types';
import { redirect } from '@sveltejs/kit';

// Model access policies are LLM gateway profiles now.
export const load: PageLoad = ({ params }) => {
	throw redirect(307, profileUrl(params.id));
};
