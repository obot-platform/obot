import * as data from './data';
import { http, HttpResponse } from 'msw';

export const handlers = [
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
	http.get('/api/bootstrap', () => HttpResponse.json(data.getBootstrapStatusResponse)),
	http.get('/api/default-model-aliases', () =>
		HttpResponse.json({ items: data.listDefaultModelAliasesResponse })
	),
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
	http.get('/api/mcp-catalogs/default/servers', () =>
		HttpResponse.json({ items: data.listMCPCatalogServersResponse })
	),
	http.get('/api/mcp-server-instances', () =>
		HttpResponse.json({ items: data.listMcpServerInstancesResponse })
	),
	http.get('/api/mcp-servers/:id/logs', () => HttpResponse.json({ items: [] })),
	http.get('/api/mcp-servers', () =>
		HttpResponse.json({ items: data.listSingleOrRemoteMcpServersResponse })
	),
	http.get('/api/me', () => HttpResponse.json(data.getProfileResponse)),
	http.get('/api/model-providers', () => HttpResponse.json({ items: [] })),
	http.get('/api/models', () => HttpResponse.json({ items: data.listModelsResponse })),
	http.get('/api/users', () => HttpResponse.json({ items: data.listUsersResponse })),
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
