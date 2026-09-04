import {
	clearMcpServerOAuth,
	getMcpServerOauthURL,
	isMcpServerOauthNeeded,
	listMcpCatalogServerPrompts,
	listMcpCatalogServerResources,
	listMcpCatalogServerTools,
	validateSingleOrRemoteMcpServerLaunched
} from './operations';
import { afterEach, expect, it, vi } from 'vitest';

afterEach(() => vi.unstubAllGlobals());

it.each(['vmcp1test', 'ms1test'])(
	'routes actions for %s to its own resource endpoints',
	async (id) => {
		const fetcher = vi.fn().mockImplementation(() =>
			Promise.resolve(
				new Response(JSON.stringify([]), {
					status: 200,
					headers: { 'content-type': 'application/json' }
				})
			)
		);
		vi.stubGlobal('fetch', fetcher);
		await listMcpCatalogServerTools(id);
		await listMcpCatalogServerPrompts(id);
		await listMcpCatalogServerResources(id);
		await getMcpServerOauthURL(id);
		await isMcpServerOauthNeeded(id);
		await validateSingleOrRemoteMcpServerLaunched(id);
		await clearMcpServerOAuth(id);
		const resource = id.startsWith('vmcp1') ? 'vmcps' : 'mcp-servers';
		const listing = id.startsWith('vmcp1') ? 'vmcps' : 'all-mcps/servers';
		expect(fetcher.mock.calls.map(([url]) => new URL(url, 'http://localhost').pathname)).toEqual([
			`/api/${listing}/${id}/tools`,
			`/api/${listing}/${id}/prompts`,
			`/api/${listing}/${id}/resources`,
			`/api/${resource}/${id}/oauth-url`,
			`/api/${resource}/${id}/check-oauth`,
			`/api/${resource}/${id}/launch`,
			`/api/${resource}/${id}/oauth`
		]);
	}
);
