import { mcpConnectHandlers } from '../helpers/mcpConnect';
import { mcpServerActionHandlers } from '../helpers/mcpServerActions';
import * as data from './data';
import { http, HttpResponse } from 'msw';

const providerIcon = '<svg xmlns="http://www.w3.org/2000/svg" width="1" height="1" />';

export const handlers = [
	http.get(
		/\/(?:admin\/assets\/(?:okta|local)_icon_small\.png|okta\.svg)$/,
		() => new HttpResponse(providerIcon, { headers: { 'Content-Type': 'image/svg+xml' } })
	),
	http.get('/api/all-mcps/entries', () =>
		HttpResponse.json({ items: data.listMCPCatalogEntriesResponse })
	),
	http.get('/api/all-mcps/servers', () =>
		HttpResponse.json({ items: data.listMCPCatalogServersResponse })
	),
	http.get('/api/app-notification', () => HttpResponse.json(data.getAppNotificationResponse)),
	http.get('/api/app-preferences', () => HttpResponse.json(data.listAppPreferencesResponse)),
	http.get('/api/auth-providers', () =>
		HttpResponse.json({ items: data.listAuthProvidersResponse })
	),
	http.post('/api/auth-providers/:providerID/reveal', () =>
		HttpResponse.json(null, { status: 404 })
	),
	http.get('/api/bootstrap', () => HttpResponse.json(data.getBootstrapStatusResponse)),
	http.get('/api/local-auth/users', () => HttpResponse.json({ items: [] })),
	http.get('/api/default-model-aliases', () =>
		HttpResponse.json({ items: data.listDefaultModelAliasesResponse })
	),
	http.get('/api/image-pull-secrets/capability', () => HttpResponse.json({ available: false })),
	http.get('/api/image-pull-secrets', () => HttpResponse.json({ items: [] })),
	http.get('/api/eula', () => HttpResponse.json({ accepted: true })),
	http.get('/api/license', () => HttpResponse.json(data.getLicenseResponse)),
	http.delete('/api/license', () => HttpResponse.json(data.getLicenseResponse)),
	http.get('/api/mcp-capacity', () => HttpResponse.json(data.getMCPCapacityResponse)),
	http.get('/api/setup/explicit-role-emails', () =>
		HttpResponse.json(data.listExplicitRoleEmailsResponse)
	),
	http.post('/api/setup/cancel-temp-login', () => new HttpResponse(null, { status: 204 })),
	http.post('/api/setup/initiate-temp-login', () =>
		HttpResponse.json(data.initiateTempLoginResponse)
	),
	http.get('/api/mcp-catalogs/default/entries', () =>
		HttpResponse.json({ items: data.listMCPCatalogEntriesResponse })
	),
	http.get('/api/mcp-catalogs/default/entries/all-servers', () =>
		HttpResponse.json({ items: data.listAllCatalogDeployedSingleRemoteServersResponse })
	),
	// Catalog entry and vMCP forms lazily load these on mount, so every spec that renders one needs
	// them even when they are not what the spec is asserting.
	http.get('/api/mcp-catalogs/:catalogID/access-control-rules', () =>
		HttpResponse.json({ items: [] })
	),
	http.get('/api/mcp-catalogs/:catalogID/entries/:entryID/servers', () =>
		HttpResponse.json({ items: [] })
	),
	http.get('/api/mcp-catalogs/default/servers', () =>
		HttpResponse.json({ items: data.listMCPCatalogServersResponse })
	),
	...mcpConnectHandlers,
	...mcpServerActionHandlers,
	http.get('/api/mcp-server-instances', () =>
		HttpResponse.json({ items: data.listMcpServerInstancesResponse })
	),
	http.get('/api/mcp-servers/:id/logs', () => HttpResponse.json({ items: [] })),
	http.get('/api/workspaces/:workspaceID/entries/:entryID/servers/:serverID/logs', () =>
		HttpResponse.json({ items: [] })
	),
	http.get('/api/workspaces/:workspaceID/servers/:serverID/logs', () =>
		HttpResponse.json({ items: [] })
	),
	http.get('/api/mcp-servers', () =>
		HttpResponse.json({ items: data.listSingleOrRemoteMcpServersResponse })
	),
	http.get('/api/message-policy-violations/filter-options/:filter', () =>
		HttpResponse.json({ options: [] })
	),
	http.get('/api/me', () => HttpResponse.json(data.getProfileResponse)),
	http.get('/api/model-providers', () => HttpResponse.json({ items: [] })),
	http.get('/api/models', () => HttpResponse.json({ items: data.listModelsResponse })),
	http.get('/api/product-telemetry-consent', () => HttpResponse.json({})),
	http.get('/api/users', () => HttpResponse.json({ items: data.listUsersResponse })),
	http.get('/api/vmcps', () => HttpResponse.json({ items: [] })),
	http.post('/api/vmcps/:vmcpID/reveal', () => HttpResponse.json({ components: {} })),
	http.get('/api/vmcps/:vmcpID', () => new HttpResponse(null, { status: 404 })),
	http.get('/api/vmcp-instances', () => HttpResponse.json({ items: [] })),
	http.post('/api/vmcp-instances/:instanceID/reveal', () => HttpResponse.json({ components: {} })),
	http.get('/api/vmcp-instances/:instanceID', () => new HttpResponse(null, { status: 404 })),
	http.post(
		'/api/vmcps/:vmcpID/components/:componentID/generate-tool-previews',
		() => new HttpResponse(null, { status: 404 })
	),
	http.get('/api/groups', () => HttpResponse.json({ items: [] })),
	http.get('/api/version', () => HttpResponse.json(data.getVersionResponse)),
	http.get('/api/workspaces/all-entries', () =>
		HttpResponse.json({ items: data.listAllUserWorkspaceCatalogEntriesResponse })
	),
	http.get('/api/workspaces/all-entries/all-servers', () =>
		HttpResponse.json({ items: data.listAllWorkspaceDeployedSingleRemoteServersResponse })
	),
	http.get('/api/workspaces/all-servers', () =>
		HttpResponse.json({ items: data.listAllUserWorkspaceMCPServersResponse })
	)
];
