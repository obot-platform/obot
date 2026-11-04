// Isolated local-auth E2E: OBOT_TEST_OWNER_STATE=/tmp/owner.json OBOT_TEST_ENTRY_ID=entry node tools/test-admin-readonly.mjs
// Use tools/mcp-role-fixture.mjs and grant all test users the seeded entry through an ACR.
// The entry needs an optional, sensitive, userAllowed header named X-ROLE-TEST.
import assert from 'node:assert/strict';
import fs from 'node:fs/promises';
import { chromium } from 'playwright';

const baseURL = process.env.OBOT_TEST_URL || 'http://localhost:8080';
assert(
	process.env.OBOT_TEST_OWNER_STATE && process.env.OBOT_TEST_ENTRY_ID,
	'Set OBOT_TEST_OWNER_STATE and OBOT_TEST_ENTRY_ID for an isolated test instance'
);
const browser = await chromium.launch({
	...(process.env.OBOT_TEST_CHROMIUM ? { executablePath: process.env.OBOT_TEST_CHROMIUM } : {})
});
const owner = await browser.newContext({
	baseURL,
	storageState: process.env.OBOT_TEST_OWNER_STATE
});
const sessions = new Map();
const probe = await browser.newContext({ baseURL });
const prefix = `roles-${Date.now()}`;
const password = 'Local-test-role-password-123!';
const report = [];
async function json(r) {
	const t = await r.text();
	assert(r.ok(), `${r.url()}: ${r.status()} ${t.slice(0, 200)}`);
	return t ? JSON.parse(t) : null;
}
async function poll(fn, label) {
	for (let i = 0; i < 80; i++) {
		if (await fn()) return;
		await new Promise((r) => setTimeout(r, 250));
	}
	assert.fail(label);
}
async function rpc(page, instance, key, id, method, params) {
	const r = await page.evaluate(
		async (x) => {
			const r = await fetch(`/mcp-connect/${x.instance}`, {
				method: 'POST',
				headers: {
					...(x.session ? { 'Mcp-Session-Id': x.session } : {}),
					Authorization: `Bearer ${x.key}`,
					'Content-Type': 'application/json',
					Accept: 'application/json, text/event-stream'
				},
				body: JSON.stringify({ jsonrpc: '2.0', id: x.id, method: x.method, params: x.params })
			});
			return { status: r.status, session: r.headers.get('Mcp-Session-Id'), text: await r.text() };
		},
		{ instance, key, id, method, params, session: sessions.get(instance) }
	);
	assert.equal(r.status, 200, `${method}: ${r.text}`);
	if (r.session) sessions.set(instance, r.session);
	const d = r.text.startsWith('event:')
		? r.text
				.split('\n')
				.filter((x) => x.startsWith('data:'))
				.map((x) => JSON.parse(x.slice(5)))
				.find((x) => x.id === id)
		: JSON.parse(r.text);
	assert(!d.error, JSON.stringify(d.error));
	return d.result;
}
try {
	for (const [roleName, baseRole] of [
		['standard', 4],
		['power', 128],
		['powerplus', 64],
		['readonly', 256],
		['admin', 16],
		['owner', 8]
	])
		for (const auditor of [false, true]) {
			const name = roleName + (auditor ? '_auditor' : '');
			if (
				process.env.OBOT_TEST_ROLE_FILTER &&
				!process.env.OBOT_TEST_ROLE_FILTER.split(',').includes(name)
			)
				continue;
			const email = `${prefix}-${name}@roles.test`;
			await json(
				await owner.request.post('/api/local-auth/users', {
					data: { email, password, requirePasswordChange: false }
				})
			);
			const c = await browser.newContext({
				baseURL,
				viewport: { width: 1440, height: 1100 },
				reducedMotion: 'reduce'
			});
			try {
				c.setDefaultNavigationTimeout(90_000);
				const p = await c.newPage();
				const errors = [];
				p.on('pageerror', (e) => errors.push(e.message));
				console.log(`START ${name}`);
				await p.goto('/login/local');
				await p.waitForLoadState('networkidle');
				await p.getByLabel('Email', { exact: true }).fill(email);
				await p.locator('input[name="password"]').fill(password);
				await p.getByRole('button', { name: 'Sign in', exact: true }).click();
				await p.waitForURL(
					(u) => !u.pathname.startsWith('/login') && !u.pathname.startsWith('/oauth2')
				);
				console.log(`LOGIN ${name}`);
				let me = await json(await c.request.get('/api/me'));
				const role = baseRole | (auditor ? 32 : 0);
				await json(await owner.request.patch(`/api/users/${me.id}`, { data: { role } }));
				me = await json(await c.request.get('/api/me'));
				assert.equal(me.effectiveRole, role);
				const admin = baseRole === 16 || baseRole === 8;
				const viewAdmin = admin || baseRole === 256 || auditor;
				for (const [method, path, data] of [
					['PUT', '/api/registry-settings', { anonymousRegistryAccessEnabled: false }],
					['POST', '/api/user-default-role-settings', { role: 4 }]
				])
					assert.equal(
						(await c.request.fetch(path, { method, data })).status(),
						admin ? 200 : 403,
						`${name} ${path}`
					);
				if (baseRole === 256)
					assert(!me.groups.includes('power-user') && !me.groups.includes('power-user-plus'));
				assert.equal(
					(await c.request.get('/api/mcp-tunnels')).status(),
					admin || baseRole === 256 ? 200 : 403
				);
				if (!admin && !auditor)
					assert.equal(
						(await c.request.post('/api/auth-providers/local/reveal', { data: {} })).status(),
						403
					);
				const v = await json(
					await c.request.post('/api/vmcps?scope=personal', {
						data: {
							displayName: `${prefix}-${name}`,
							components: [
								{
									name: 'echo',
									mcpServerCatalogEntryID: process.env.OBOT_TEST_ENTRY_ID,
									forceSingleUser: true,
									configuration: [{ key: 'X-ROLE-TEST', policy: 'userAllowed' }]
								}
							]
						}
					})
				);
				const instance = await json(
					await c.request.post('/api/vmcp-instances', { data: { vmcpID: v.id } })
				);
				await json(
					await c.request.post(`/api/vmcp-instances/${instance.id}/configure`, {
						data: { components: { [v.components[0].id]: { 'X-ROLE-TEST': name } } }
					})
				);
				const personalConfig = await json(
					await c.request.post(`/api/vmcp-instances/${instance.id}/reveal`, { data: {} })
				);
				assert.equal(personalConfig.components[v.components[0].id]['X-ROLE-TEST'], name);

				const key = await json(
					await c.request.post('/api/api-keys', {
						data: { name: `${prefix}-${name}`, mcpServerIds: [v.id], canAccessSkills: true }
					})
				);
				console.log(`CONNECT ${name}`);
				await p.goto(`/vmcps/${v.id}`);
				await p.getByRole('heading', { name: v.displayName, exact: true }).waitFor();
				await rpc(p, instance.id, key.key, 1, 'initialize', {
					protocolVersion: '2025-03-26',
					capabilities: {},
					clientInfo: { name: 'role-e2e', version: '1' }
				});
				await p.evaluate(
					async ({ instance, key, session }) => {
						const r = await fetch(`/mcp-connect/${instance}`, {
							method: 'POST',
							headers: {
								Authorization: `Bearer ${key}`,
								'Content-Type': 'application/json',
								Accept: 'application/json, text/event-stream',
								...(session ? { 'Mcp-Session-Id': session } : {})
							},
							body: JSON.stringify({ jsonrpc: '2.0', method: 'notifications/initialized' })
						});
						if (!r.ok) throw Error(await r.text());
					},
					{ instance: instance.id, key: key.key, session: sessions.get(instance.id) }
				);

				const tools = await rpc(p, instance.id, key.key, 2, 'tools/list', {});
				const echo = tools.tools.find((t) => t.name.includes('echo'));
				assert(echo);
				const result = await rpc(p, instance.id, key.key, 3, 'tools/call', {
					name: echo.name,
					arguments: { message: name }
				});
				assert(JSON.stringify(result).includes(`roles-echo:${name}`));
				assert(JSON.stringify(result).includes(`credential:${name}`));
				for (const path of [
					'/identity-access?view=agents',
					'/models',
					'/skills',
					'/mcp-servers',
					'/audit-logs',
					'/admin/platform?view=settings'
				]) {
					if (path.startsWith('/admin/') && !viewAdmin) {
						assert.equal((await c.request.get(path, { maxRedirects: 0 })).status(), 403);
						continue;
					}
					await p.goto(path);
					await p.getByRole('link', { name: 'Identity & Access', exact: true }).waitFor();
					assert(
						!/Internal Error|500 Internal|Something went wrong/.test(
							await p.locator('body').innerText()
						),
						`${name} ${path}`
					);
					if (path.startsWith('/identity-access')) {
						await p.waitForLoadState('networkidle');
						const splash = p.getByRole('dialog').filter({ hasText: 'Welcome to Obot!' });
						if (await splash.isVisible()) {
							const analytics = splash.getByRole('checkbox', { name: /Share product usage data/ });
							if (await analytics.count()) await analytics.uncheck();
							await splash.getByRole('button', { name: 'Continue', exact: true }).click();
							await splash.waitFor({ state: 'hidden' });
						}
						await p.getByRole('button', { name: 'Create Agent Identity', exact: true }).click();
						await p
							.getByRole('textbox', { name: 'Name', exact: true })
							.fill(`${prefix}-${name}-ui`);
						await p.getByRole('checkbox', { name: /Skill access/ }).check();
						const created = p.waitForResponse(
							(r) => r.request().method() === 'POST' && r.url().endsWith('/api/api-keys')
						);
						await p.getByRole('button', { name: 'Save', exact: true }).click();
						const uiKey = await json(await created);
						await p.getByRole('button', { name: /saved my key/i }).click();
						await json(await c.request.delete(`/api/api-keys/${uiKey.id}`));
					}
				}
				await p.goto(`/identity-access/agents/${key.id}`);
				await p.getByRole('button', { name: `Delete ${key.name}`, exact: true }).waitFor();
				await p.getByRole('button', { name: `Delete ${key.name}`, exact: true }).click();
				await p
					.getByRole('dialog')
					.getByRole('button', { name: "Yes, I'm sure", exact: true })
					.click({ trial: true });
				if (process.env.OBOT_TEST_ARTIFACT_DIR) {
					await fs.mkdir(process.env.OBOT_TEST_ARTIFACT_DIR, { recursive: true });
					await p.screenshot({
						path: `${process.env.OBOT_TEST_ARTIFACT_DIR}/${name}.png`,
						fullPage: true
					});
				}
				const deleted = p.waitForResponse(
					(r) => r.request().method() === 'DELETE' && r.url().includes('api-keys/')
				);
				await p
					.getByRole('dialog')
					.getByRole('button', { name: "Yes, I'm sure", exact: true })
					.click();
				assert((await deleted).ok());
				assert((await json(await c.request.get(`/api/api-keys/${key.id}`))).revokedAt);
				assert.equal(
					(
						await probe.request.get('/api/skills', {
							headers: { Authorization: `Bearer ${key.key}` }
						})
					).status(),
					401
				);
				for (const invalid of [320, 384, 352, 416])
					assert.equal(
						(
							await owner.request.patch(`/api/users/${me.id}`, { data: { role: invalid } })
						).status(),
						400
					);
				await json(await c.request.delete(`/api/vmcp-instances/${instance.id}`));
				await json(await c.request.delete(`/api/vmcps/${v.id}`));
				if (baseRole === 128 || baseRole === 64) {
					const w = (await json(await c.request.get('/api/workspaces'))).items.find(
						(w) => w.userID === me.id
					);
					assert(w);
					const e = await json(
						await c.request.post(`/api/workspaces/${w.id}/entries`, {
							data: {
								name: 'Downgrade test',
								runtime: 'remote',
								remoteConfig: { fixedURL: 'http://localhost:8099' }
							}
						})
					);
					await json(
						await owner.request.patch(`/api/users/${me.id}`, {
							data: { role: 256 | (auditor ? 32 : 0) }
						})
					);
					assert.equal(
						(await json(await c.request.get('/api/me'))).effectiveRole,
						256 | (auditor ? 32 : 0)
					);
					await poll(
						async () =>
							!(await json(await owner.request.get('/api/workspaces'))).items.some(
								(x) => x.id === w.id
							),
						`${name} workspace cleanup`
					);
					await poll(
						async () =>
							(await owner.request.get(`/api/workspaces/${w.id}/entries/${e.id}`)).status() === 404,
						`${name} entry cleanup`
					);
				}
				assert.deepEqual(errors, [], `${name} browser exceptions`);
				report.push({
					name,
					role,
					passed: true,
					checks: [
						'local auth login',
						'admin read/write boundaries',
						'personal credentials configure/reveal',
						'MCP initialize/tools list/tool call',
						'UI identity create/revoke',
						'revoked key rejected without cookies',
						'invalid combined roles rejected',
						'personal disconnect',
						...([128, 64].includes(baseRole) ? ['workspace and entry cleanup on downgrade'] : [])
					]
				});
				console.log(`PASS ${name}`);
			} finally {
				await c.close();
			}
		}
	await fs.writeFile(
		process.env.OBOT_TEST_REPORT || '/tmp/admin-readonly-e2e.json',
		JSON.stringify(report, null, 2)
	);
	console.log(`All ${report.length} roles passed`);
} finally {
	await probe.close();
	await owner.close();
	await browser.close();
}
