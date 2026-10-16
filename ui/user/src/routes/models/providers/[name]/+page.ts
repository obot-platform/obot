import type { PageLoad } from './$types';
import { redirect } from '@sveltejs/kit';

// LLM gateway and guardrail editors are admin-only.
export const load: PageLoad = async ({ parent }) => {
	const { profile } = await parent();
	if (!profile?.hasAdminAccess?.()) {
		throw redirect(307, '/');
	}
	return {};
};
