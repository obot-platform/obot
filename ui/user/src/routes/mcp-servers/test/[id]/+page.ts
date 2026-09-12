import { handleRouteError, HttpError } from '$lib/errors';
import { UserService, type MCPCatalogServer, type VMCPInstance } from '$lib/services';
import { vmcpTesterServer } from '$lib/services/vmcps/tester';
import type { PageLoad } from './$types';

const VMCP_PREFIX = 'vmcp1';
const VMCP_INSTANCE_PREFIX = 'vmcpi1';

function safeBackTarget(server: MCPCatalogServer): string {
	if (server.catalogEntryID && server.serverUserType === 'singleUser') {
		return `/mcp-servers/c/${encodeURIComponent(server.catalogEntryID)}/instance/${encodeURIComponent(server.id)}`;
	}
	return `/mcp-servers/s/${encodeURIComponent(server.id)}`;
}

async function loadVMCPTesterTarget(
	id: string,
	profileId: string,
	fetcher: typeof fetch
): Promise<{ server: MCPCatalogServer; backTarget: string }> {
	let instance: VMCPInstance | undefined;
	let vmcpID = id;

	if (id.startsWith(VMCP_INSTANCE_PREFIX)) {
		instance = await UserService.getVMCPInstance(id, { fetch: fetcher });
		vmcpID = instance.vmcpID;
	}

	const vmcp = await UserService.getVMCP(vmcpID, { fetch: fetcher });
	if (vmcp.components.length === 0) {
		throw new HttpError(404, `404 /mcp-servers/test/${id}: vMCP is not connectable`);
	}
	if (!instance) {
		const instances = await UserService.listVMCPInstances({ fetch: fetcher });
		instance = instances
			.filter((candidate) => candidate.vmcpID === vmcp.id && candidate.userID === profileId)
			.sort((a, b) => a.created.localeCompare(b.created))[0];
	}

	return {
		server: vmcpTesterServer(vmcp, id, instance),
		backTarget: `/vmcps/${vmcp.id}`
	};
}

export const load: PageLoad = async ({ params, fetch, parent }) => {
	const { profile } = await parent();
	const path = `/mcp-servers/test/${params.id}`;
	try {
		if (params.id.startsWith(VMCP_PREFIX) || params.id.startsWith(VMCP_INSTANCE_PREFIX)) {
			return await loadVMCPTesterTarget(params.id, profile.id, fetch);
		}
		const server = await UserService.getMCPTesterServer(params.id, { fetch });
		return {
			server,
			backTarget: safeBackTarget(server)
		};
	} catch (error) {
		handleRouteError(error, path, profile);
	}
};
