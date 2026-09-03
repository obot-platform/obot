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
import type { PageLoad } from './$types';

const views = new Set([
	'agents',
	'templates',
	'harnesses',
	'pools',
	'config-sources',
	'access-policies'
]);

export const load: PageLoad = async ({ fetch, parent, url }) => {
	const { profile } = await parent();
	const hasAdminAccess = profile.hasAdminAccess?.() ?? false;
	const requestedView = url.searchParams.get('view');
	const view =
		requestedView && views.has(requestedView) && (hasAdminAccess || requestedView === 'agents')
			? requestedView
			: 'agents';

	let hostedAgents: HostedAgent[] = [];
	let instances: HostedAgentInstance[] = [];
	let pools: HostedAgentPool[] = [];
	let templates: HostedAgent[] = [];
	let agentCatalogs: AgentCatalog[] = [];
	let harnesses: Harness[] = [];
	let adminPools: HostedAgentPool[] = [];
	let adminAssignments: HostedAgentPoolAssignment[] = [];
	let poolDefaults: HostedAgentPoolDefaults | undefined;
	let hostedAgentAccessPolicies: HostedAgentAccessPolicy[] = [];

	if (view === 'agents') {
		try {
			[hostedAgents, instances, pools] = await Promise.all([
				AdminService.listHostedAgents({ fetch, all: hasAdminAccess }),
				AdminService.listHostedAgentInstances(undefined, { fetch }),
				AdminService.listHostedAgentPools({ fetch })
			]);
		} catch (err) {
			handleRouteError(err, '/hosted-agents', profile);
		}
	} else if (hasAdminAccess) {
		try {
			switch (view) {
				case 'templates':
					[templates, harnesses] = await Promise.all([
						AdminService.listHostedAgents({ fetch, all: true }),
						AdminService.listHarnesses({ fetch })
					]);
					break;
				case 'harnesses':
					harnesses = await AdminService.listHarnesses({ fetch });
					break;
				case 'pools':
					[adminPools, adminAssignments] = await Promise.all([
						AdminService.listHostedAgentPools({ fetch }),
						AdminService.listHostedAgentPoolAssignments({ fetch })
					]);
					try {
						poolDefaults = await AdminService.getHostedAgentPoolDefaults({ fetch });
					} catch {
						// Defaults do not exist until an administrator configures them.
					}
					break;
				case 'config-sources':
					agentCatalogs = await AdminService.listAgentCatalogs({ fetch });
					break;
				case 'access-policies':
					hostedAgentAccessPolicies = await AdminService.listHostedAgentAccessPolicies({
						fetch
					});
					break;
			}
		} catch (err) {
			handleRouteError(err, '/hosted-agents', profile);
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
