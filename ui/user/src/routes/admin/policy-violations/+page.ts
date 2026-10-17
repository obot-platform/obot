import type { PageLoad } from './$types';
import { redirect } from '@sveltejs/kit';

// Guardrail findings, which replace message policy violations, are on the LLM audit log.
export const load: PageLoad = () => {
	throw redirect(307, '/audit-logs?view=llm');
};
