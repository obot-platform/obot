import type { PageLoad } from './$types';
import { redirect } from '@sveltejs/kit';

// The user's API keys were once the "Agents" tab, at /identity-access/agents. Agents are now their
// own resource, so bookmarks to the old paths go to the API Keys pages.
export const load: PageLoad = ({ params, url }) => {
	throw redirect(301, `/identity-access/api-keys/${encodeURIComponent(params.id)}${url.search}`);
};
