import * as data from './data';
import { http, HttpResponse } from 'msw';

export const handlers = [
	http.get('/api/all-mcps/entries', () => HttpResponse.json({ items: data.listMCPsResponse })),
	http.get('/api/all-mcps/servers', () =>
		HttpResponse.json({ items: data.listMCPCatalogServersResponse })
	),
	http.get('/api/app-notification', () => HttpResponse.json(data.getAppNotificationResponse)),
	http.get('/api/app-preferences', () => HttpResponse.json(data.listAppPreferencesResponse)),
	http.get('/api/default-model-aliases', () =>
		HttpResponse.json({ items: data.listDefaultModelAliasesResponse })
	),
	http.get('/api/license', () => HttpResponse.json(data.getLicenseResponse)),
	http.delete('/api/license', () => HttpResponse.json(data.getLicenseResponse)),
	http.get('/api/mcp-server-instances', () =>
		HttpResponse.json({ items: data.listMcpServerInstancesResponse })
	),
	http.get('/api/mcp-servers', () =>
		HttpResponse.json({ items: data.listSingleOrRemoteMcpServersResponse })
	),
	http.get('/api/me', () => HttpResponse.json(data.getProfileResponse)),
	http.get('/api/models', () => HttpResponse.json({ items: data.listModelsResponse })),
	http.get('/api/users', () => HttpResponse.json({ items: data.listUsersResponse })),
	http.get('/api/version', () => HttpResponse.json(data.getVersionResponse))
];
