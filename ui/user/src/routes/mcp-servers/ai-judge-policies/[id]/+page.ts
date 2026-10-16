import type { PageLoad } from './$types';
import { redirect } from '@sveltejs/kit';

// AI judge (message) policies are now guardrails: each became an LLM Judge guardrail and a blocking guardrail policy.
export const load: PageLoad = () => {
	throw redirect(307, '/models?view=profiles');
};
