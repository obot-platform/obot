import { handleRouteError } from '$lib/errors';
import { AdminService } from '$lib/services';
import type {
	AgentCatalog,
	Harness,
	HostedAgent,
	HostedAgentPool,
	HostedAgentPoolAssignment,
	HostedAgentPoolDefaults
} from '$lib/services/admin/types';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ fetch, parent }) => {
	const { profile } = await parent();
	let hostedAgents: HostedAgent[] = [];
	let agentCatalogs: AgentCatalog[] = [];
	let harnesses: Harness[] = [];
	let pools: HostedAgentPool[] = [];
	let assignments: HostedAgentPoolAssignment[] = [];
	let poolDefaults: HostedAgentPoolDefaults | undefined;

	try {
		// Admins get the unfiltered list; the default view is access-rule filtered.
		[hostedAgents, agentCatalogs, harnesses, pools, assignments] = await Promise.all([
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
	} catch (err) {
		handleRouteError(err, '/admin/hosted-agents', profile);
	}

	return {
		hostedAgents,
		agentCatalogs,
		harnesses,
		pools,
		assignments,
		poolDefaults
	};
};
