import { handleRouteError } from '$lib/errors';
import { AdminService } from '$lib/services';
import type {
	AgentCatalog,
	Harness,
	HostedAgent,
	HostedAgentAccessPolicy,
	HostedAgentInstance,
	HostedAgentPool,
	HostedAgentPoolAssignment,
	HostedAgentPoolDefaults
} from '$lib/services/admin/types';
import type { PageLoad } from '../v2/hosted-agents/$types';

export const load: PageLoad = async ({ fetch, parent }) => {
	const { profile } = await parent();
	const hasAdminAccess = profile.hasAdminAccess?.() ?? false;

	let hostedAgents: HostedAgent[] = [];
	let instances: HostedAgentInstance[] = [];
	let pools: HostedAgentPool[] = [];

	try {
		[hostedAgents, instances, pools] = await Promise.all([
			AdminService.listHostedAgents({ fetch }),
			AdminService.listHostedAgentInstances(undefined, { fetch }),
			AdminService.listHostedAgentPools({ fetch })
		]);
	} catch (err) {
		handleRouteError(err, '/v2/hosted-agents', profile);
	}

	let templates: HostedAgent[] = [];
	let agentCatalogs: AgentCatalog[] = [];
	let harnesses: Harness[] = [];
	let adminPools: HostedAgentPool[] = [];
	let adminAssignments: HostedAgentPoolAssignment[] = [];
	let poolDefaults: HostedAgentPoolDefaults | undefined;
	let hostedAgentAccessPolicies: HostedAgentAccessPolicy[] = [];

	if (hasAdminAccess) {
		try {
			[templates, agentCatalogs, harnesses, adminPools, adminAssignments] = await Promise.all([
				AdminService.listHostedAgents({ fetch, all: true }),
				AdminService.listAgentCatalogs({ fetch }),
				AdminService.listHarnesses({ fetch }),
				AdminService.listHostedAgentPools({ fetch }),
				AdminService.listHostedAgentPoolAssignments({ fetch })
			]);
			try {
				poolDefaults = await AdminService.getHostedAgentPoolDefaults({ fetch });
			} catch {
				// Defaults do not exist until an administrator configures them.
			}
			hostedAgentAccessPolicies = await AdminService.listHostedAgentAccessPolicies({ fetch });
		} catch (err) {
			handleRouteError(err, '/v2/hosted-agents', profile);
		}
	}

	return {
		hasAdminAccess,
		hostedAgents,
		instances,
		pools,
		templates,
		agentCatalogs,
		harnesses,
		adminPools,
		adminAssignments,
		poolDefaults,
		hostedAgentAccessPolicies
	};
};
