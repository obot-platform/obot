import type { MCPCatalogServer, VMCP, VMCPInstance } from '$lib/services';

export function userAllowedConfigurationKeys(vmcp: VMCP): string[] {
	return (vmcp.components ?? []).flatMap((component) => {
		const componentID = component.id || component.mcpServerCatalogEntryID || '';
		return (component.configuration ?? [])
			.filter((field) => field.policy === 'userAllowed')
			.map((field) => (componentID ? `${componentID}.${field.key}` : field.key));
	});
}

export function vmcpTesterServer(
	vmcp: VMCP,
	connectID: string,
	instance?: VMCPInstance
): MCPCatalogServer {
	const created = instance?.created ?? vmcp.created;
	const pendingUserConfig = instance ? [] : userAllowedConfigurationKeys(vmcp);
	return {
		id: connectID,
		userID: instance?.userID ?? vmcp.userID ?? '',
		configured: instance?.status?.configured ?? pendingUserConfig.length === 0,
		catalogEntryID: '',
		missingRequiredEnvVars: instance?.status?.missingRequiredConfiguration ?? pendingUserConfig,
		mcpCatalogID: '',
		created,
		updated: created,
		type: instance?.type ?? vmcp.type ?? 'vmcp',
		manifest: {
			name: vmcp.displayName,
			description: vmcp.description,
			icon: vmcp.icon,
			runtime: 'vmcp'
		},
		serverUserType: 'multiUser',
		deploymentStatus: vmcp.status?.ready ? 'Available' : 'Unavailable',
		canConnect: true
	};
}
