// Local, read-only MCP fixture for the documented Docker verification.
import http from 'node:http';

http.createServer(async (req, res) => {
  if (req.method === 'GET') {
    res.writeHead(405).end();
    return;
  }
  let body = '';
  for await (const chunk of req) body += chunk;
  let message;
  try { message = JSON.parse(body); }
  catch { res.writeHead(400).end(); return; }
  if (message.id === undefined) {
    res.writeHead(202).end();
    return;
  }
  let result = {};
  if (message.method === 'initialize') {
    result = {
      protocolVersion: message.params.protocolVersion,
      capabilities: { tools: {} },
      serverInfo: { name: 'docs-verification', version: '1.0.0' },
    };
  }
  if (message.method === 'tools/list') {
    result = { tools: ['echo', 'private_echo'].map(name => ({
      name,
      description: 'Return supplied text for documentation verification.',
      inputSchema: { type: 'object', properties: { text: { type: 'string' } }, required: ['text'] },
    })) };
  }
  if (message.method === 'tools/call') {
    result = { content: [{ type: 'text', text: message.params.arguments.text }] };
  }
  res.writeHead(200, { 'Content-Type': 'application/json' }).end(JSON.stringify({ jsonrpc: '2.0', id: message.id, result }));
}).listen(8090, '0.0.0.0', () => console.log('Documentation MCP fixture listening on 8090'));
