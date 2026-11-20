// Local deterministic MCP fixture for test-admin-readonly.mjs; run on port 8099.
import { Server } from '@modelcontextprotocol/sdk/server/index.js';
import { StreamableHTTPServerTransport } from '@modelcontextprotocol/sdk/server/streamableHttp.js';
import { ListToolsRequestSchema, CallToolRequestSchema } from '@modelcontextprotocol/sdk/types.js';
import { createServer } from 'node:http';

createServer(async (req, res) => {
	const server = new Server(
		{ name: 'roles-fixture', version: '1' },
		{ capabilities: { tools: {} } }
	);
	server.setRequestHandler(ListToolsRequestSchema, () => ({
		tools: [
			{
				name: 'echo',
				description: 'Echo a test message',
				inputSchema: { type: 'object', properties: { message: { type: 'string' } } }
			}
		]
	}));
	server.setRequestHandler(CallToolRequestSchema, (r) => ({
		content: [
			{
				type: 'text',
				text:
					'roles-echo:' + r.params.arguments?.message + ';credential:' + req.headers['x-role-test']
			}
		]
	}));
	const transport = new StreamableHTTPServerTransport({
		sessionIdGenerator: undefined,
		enableJsonResponse: true
	});
	await server.connect(transport);
	res.on('close', () => {
		transport.close();
		server.close();
	});
	await transport.handleRequest(req, res);
}).listen(8099, '0.0.0.0', () => console.log('Ready 8099'));
