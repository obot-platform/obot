import type { OrgUser } from '$lib/services';
import {
	agentDisplayName,
	parseAgentActor,
	type AgentLookup
} from '$lib/services/agent-identities';
import { getUserDisplayName, isDisabledUser } from '$lib/utils';

// The owner of an instance or a connection is a principal: the plain ID of a user, or "agent:<id>"
// for an agent, which is never a user ID. Everything here splits owners by that kind, so that
// an agent's ID is never looked up as a user, and only users are.

/** An owner shown in a list of instances. */
export interface InstanceOwner {
	/** The owner as the API gives it: a user ID, or "agent:<id>". Audit logs are filtered by it. */
	id: string;
	name: string;
	agent: boolean;
	disabled: boolean;
}

/** Whether any of the owners is an agent, which is then named from the agents. */
export function hasAgentOwner(ownerIDs: (string | undefined)[]): boolean {
	return ownerIDs.some((owner) => parseAgentActor(owner));
}

/**
 * Names an owner: an agent by its record, marked once it is deleted, and by its ID when the agents
 * cannot be listed (that is for admins and auditors); a user by the directory.
 */
export function instanceOwner(
	ownerID: string,
	users: Map<string, OrgUser>,
	agents?: Map<string, AgentLookup>
): InstanceOwner {
	const agentId = parseAgentActor(ownerID);
	if (agentId) {
		return { id: ownerID, name: agentDisplayName(agents, agentId), agent: true, disabled: false };
	}
	return {
		id: ownerID,
		name: getUserDisplayName(users, ownerID),
		agent: false,
		disabled: isDisabledUser(users.get(ownerID))
	};
}
