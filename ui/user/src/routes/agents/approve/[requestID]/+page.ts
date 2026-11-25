import { getHttpStatusCode, parseErrorContent } from '$lib/errors';
import { m } from '$lib/i18n';
import { AgentIdentitiesService } from '$lib/services';
import { agentApprovalSignInPath } from '$lib/services/agent-identities';
import type { PageLoad } from './$types';
import { redirect } from '@sveltejs/kit';

// The page at which a user approves an agent's request to act on their behalf (the approvalURL the
// agent got). A signed-out user signs in first and comes back. A request the user can't approve shows
// the server's message on a plain error page, and never redirects anywhere.
export const load: PageLoad = async ({ params, url, parent, fetch }) => {
	const { profile } = await parent();
	if (!profile?.loaded) {
		throw redirect(303, agentApprovalSignInPath(url));
	}

	const base = {
		// The page itself, to come back to after signing in with another account.
		returnPath: url.pathname + url.search,
		oauthError: url.searchParams.get('oauth_error')
	};
	if (profile.isBootstrapUser?.()) {
		return { ...base, error: m.identity_access_approve_bootstrap() };
	}

	try {
		const request = await AgentIdentitiesService.getAgentDelegationRequest(params.requestID, {
			fetch
		});
		return { ...base, request };
	} catch (err) {
		if (getHttpStatusCode(err) === 401) {
			throw redirect(303, agentApprovalSignInPath(url));
		}
		return { ...base, error: parseErrorContent(err).message };
	}
};
