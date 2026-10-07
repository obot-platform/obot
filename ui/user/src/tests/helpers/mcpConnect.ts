import { http, HttpResponse } from 'msw';

export interface MCPRequest {
	id?: string | number;
	method: string;
	params?: Record<string, unknown>;
}

export type McpConnectRequestHandler = (request: MCPRequest) => unknown;

async function readMcpRequest(request: Request): Promise<MCPRequest | undefined> {
	try {
		const text = await request.text();
		if (!text) {
			return undefined;
		}
		return JSON.parse(text) as MCPRequest;
	} catch {
		return undefined;
	}
}

export function createMcpConnectPostResolver(
	capabilities: Record<string, unknown> = {},
	handleRequest?: McpConnectRequestHandler,
	serverInfo: { name: string; version: string } = { name: 'tester-server', version: '1.0.0' }
) {
	return async ({ request }: { request: Request }) => {
		const body = await readMcpRequest(request);
		if (!body?.method) {
			return new HttpResponse(null, { status: 202 });
		}
		if (body.method === 'initialize') {
			return HttpResponse.json(
				{
					jsonrpc: '2.0',
					id: body.id,
					result: {
						protocolVersion: body.params?.protocolVersion ?? '2025-06-18',
						capabilities,
						serverInfo
					}
				},
				{ headers: { 'Mcp-Session-Id': 'tester-session' } }
			);
		}
		const result = handleRequest?.(body);
		if (result !== undefined) {
			return HttpResponse.json({ jsonrpc: '2.0', id: body.id, result });
		}
		return new HttpResponse(null, { status: 202 });
	};
}

/** Default MSW handlers for Streamable HTTP MCP tester connections. */
export const mcpConnectHandlers = [
	http.get('/mcp-connect/:connectId', () => new HttpResponse(null, { status: 405 })),
	http.post('/mcp-connect/:connectId', createMcpConnectPostResolver())
];
