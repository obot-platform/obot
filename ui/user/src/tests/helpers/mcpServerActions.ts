import { http, HttpResponse } from 'msw';

/** Mirrors `mcpActionPath` in `src/lib/services/user/operations.ts`. */
export function mcpServerActionCollection(id: string): 'vmcps' | 'mcp-servers' {
	return id.startsWith('vmcp1') ? 'vmcps' : 'mcp-servers';
}

export function mcpServerActionApiBase(id: string): string {
	return `/api/${mcpServerActionCollection(id)}/${id}`;
}

/** Default success stubs for post-configure launch and OAuth checks. */
export const mcpServerActionHandlers = [
	http.post('/api/mcp-servers/:id/launch', () => HttpResponse.json({})),
	http.get('/api/mcp-servers/:id/oauth-url', () => HttpResponse.json({ oauthURL: '' })),
	http.post('/api/vmcps/:id/launch', () => HttpResponse.json({})),
	http.get('/api/vmcps/:id/oauth-url', () => HttpResponse.json({ oauthURL: '' }))
];
