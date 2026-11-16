import { handleRouteError } from '$lib/errors';
import { AgentIdentitiesService, UserService } from '$lib/services';
import type { AgentAPIKey, AgentIdentity } from '$lib/services/agent-identities/types';
import type { PageLoad } from './$types';
import { redirect } from '@sveltejs/kit';

export const load: PageLoad = async ({ params, parent, fetch }) => {
	const { profile } = await parent();
	// Agents are managed by admins; read-only admins and auditors can read them. Everyone else has
	// only their API keys.
	if (!profile.hasAdminAccess?.()) {
		throw redirect(302, '/identity-access?view=api-keys');
	}

	let agent: AgentIdentity | undefined;
	let keys: AgentAPIKey[] = [];
	let creatorName: string | undefined;
	try {
		[agent, keys] = await Promise.all([
			AgentIdentitiesService.getAgent(params.id, { fetch }),
			// Revoked keys are kept so that the page can show them, and so that their history is found.
			AgentIdentitiesService.listAgentAPIKeys(params.id, { fetch, showRevoked: true })
		]);
	} catch (err) {
		handleRouteError(err, `/identity-access/agent-identities/${params.id}`, profile);
	}

	if (agent?.creatorUserID) {
		try {
			const users = await UserService.listUsersIncludeDeleted({ fetch });
			const creator = users.find((user) => user.id === agent.creatorUserID);
			creatorName = creator?.displayName || creator?.email || creator?.username;
		} catch {
			// The creator's name is a nicety; the agent is shown without it.
		}
	}

	return { agent, keys, creatorName, readonly: Boolean(profile.isAdminReadonly?.()) };
};
