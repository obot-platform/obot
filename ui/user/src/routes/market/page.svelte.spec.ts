import { page as appPage } from '$app/state';
import { saveBlob } from '$lib/download';
import { m } from '$lib/i18n';
import { Group } from '$lib/services';
import type { ConnectorAccessRequest, MCPCatalogEntry, Skill } from '$lib/services';
import { mcpServersAndEntries, vmcpInstances } from '$lib/stores';
import { formatTimeAgo } from '$lib/time';
import { createMCPCatalogEntry, createVMCP } from '../../tests/helpers/mcp';
import { applyTestGoto } from '../../tests/helpers/navigation.svelte';
import { createMockProfile, preparePageData } from '../../tests/helpers/pageData';
import { worker } from '../../tests/mocks/worker';
import type { PageData } from './$types';
import MarketPage from './+page.svelte';
import { http, HttpResponse } from 'msw';
import { describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

vi.mock('$app/state', async (importOriginal) => {
	const actual = await importOriginal<typeof import('$app/state')>();
	const { createAppState } = await import('../../tests/helpers/navigation.svelte');
	return createAppState(actual);
});

vi.mock('$lib/download', async (importOriginal) => {
	const actual = await importOriginal<typeof import('$lib/download')>();
	return { ...actual, saveBlob: vi.fn() };
});

vi.mock('$app/navigation', async (importOriginal) => {
	const actual = await importOriginal<typeof import('$app/navigation')>();
	const { applyTestGoto } = await import('../../tests/helpers/navigation.svelte');
	return {
		...actual,
		goto: (url: string | URL) => applyTestGoto(url)
	};
});

vi.mock('$lib/url', async (importOriginal) => {
	const actual = await importOriginal<typeof import('$lib/url')>();
	const { applyTestGoto } = await import('../../tests/helpers/navigation.svelte');
	return {
		...actual,
		goto: (url: string | URL) => applyTestGoto(url),
		clearUrlParams: () => undefined
	};
});

const connectorEntry = createMCPCatalogEntry({
	id: 'mce1-market',
	name: 'Marketplace Connector'
});
connectorEntry.manifest = {
	...connectorEntry.manifest,
	name: 'Marketplace Connector',
	shortDescription: '查询企业信息与工商数据',
	description: '企业信息查询连接器。\n\n示例：「帮我查询杭州的 AI 公司」「查询公司股东结构」',
	metadata: {
		displayNameZh: '企业信息查询',
		categories: '数据,企业'
	}
};

const skill: Skill = {
	id: 'sk1-report',
	type: 'skill',
	name: 'weekly-report-en',
	displayName: 'English Weekly Skill',
	description:
		'根据本周工作记录生成周报。卡片只保留两到三行摘要，完整说明、数据来源和输出格式要求放在详情弹窗中展示。',
	metadata: { displayNameZh: '中文周报技能' },
	repoURL: 'https://github.com/example/weekly-report'
};

function requestFixture(
	status: ConnectorAccessRequest['manifest']['status']
): ConnectorAccessRequest {
	return {
		id: 'car1-1',
		created: '2026-10-26T08:00:00.000Z',
		manifest: {
			requesterUserID: 'user-1',
			requesterUsername: 'enduser',
			connectorID: connectorEntry.id,
			connectorName: '企业信息查询',
			mcpCatalogID: 'default',
			status,
			reason: '需要查数据'
		}
	};
}

function mockMarketApis(entries: MCPCatalogEntry[] = [connectorEntry]) {
	worker.use(
		http.get('/api/all-mcps/entries', () => HttpResponse.json({ items: entries })),
		http.get('/api/all-mcps/servers', () => HttpResponse.json({ items: [] })),
		http.get('/api/mcp-servers', () => HttpResponse.json({ items: [] })),
		http.get('/api/mcp-server-instances', () => HttpResponse.json({ items: [] }))
	);
}

function resetStore() {
	mcpServersAndEntries.current = {
		entries: [],
		servers: [],
		userInstances: [],
		userConfiguredServers: [],
		loading: false,
		lastFetched: null,
		isInitialized: false
	};
}

async function renderMarketPage({
	requests = [],
	vmcps = [],
	catalogEntries = [connectorEntry],
	profile = createMockProfile([Group.USER])
}: {
	requests?: ConnectorAccessRequest[];
	vmcps?: PageData['vmcps'];
	catalogEntries?: MCPCatalogEntry[];
	profile?: ReturnType<typeof createMockProfile>;
} = {}) {
	await applyTestGoto('/market');
	resetStore();
	vmcpInstances.current = { items: [], loading: false };
	mockMarketApis(catalogEntries);
	const data = await preparePageData<PageData>({
		profile,
		skills: [skill],
		vmcps,
		requests
	});
	return render(MarketPage, { data });
}

async function openConnectorsTab() {
	await page.getByRole('tab', { name: m.market_tab_connectors() }).click();
}

async function openMineTab() {
	await page.getByRole('tab', { name: m.market_tab_mine() }).click();
}

async function openMyLinksTab() {
	await page.getByRole('tab', { name: m.market_tab_my_links() }).click();
}

describe('market page', () => {
	it('defaults to Skills and renders Skills, Connectors, Mine, and My links tabs', async () => {
		await renderMarketPage({ profile: createMockProfile([Group.POWERUSER]) });

		for (const href of ['/dashboard', '/market', '/vmcps', '/mcp-servers']) {
			await expect.element(page.getByCSS(`a.sidebar-link[href="${href}"]`)).toBeInTheDocument();
		}
		await expect.element(page.getByRole('tab', { name: m.market_tab_skills() })).toBeVisible();
		await expect.element(page.getByRole('tab', { name: m.market_tab_connectors() })).toBeVisible();
		await expect.element(page.getByRole('tab', { name: m.market_tab_mine() })).toBeVisible();
		await expect.element(page.getByRole('tab', { name: m.market_tab_my_links() })).toBeVisible();
		await expect
			.element(page.getByRole('tab', { name: m.market_tab_skills() }))
			.toHaveAttribute('aria-selected', 'true');
		await expect
			.element(page.getByRole('tab', { name: m.market_tab_my_links() }))
			.toHaveAttribute('aria-selected', 'false');

		// Skills is the default; expert/shared-vMCP cards are no longer marketplace entries.
		const skillCard = page.getByTestId('market-skill-sk1-report');
		await expect.element(skillCard).toBeVisible();
		await expect.element(skillCard.getByText('中文周报技能')).toBeVisible();
		await expect.element(skillCard.getByText('github.com')).toBeVisible();
		await expect
			.element(skillCard.getByText(/根据本周工作记录生成周报。/))
			.toHaveClass(/\btruncate\b/);
		await expect.element(page.getByRole('tab', { name: /Experts/ })).not.toBeInTheDocument();

		// Connectors tab lists the entry with a 未连接 pill.
		await openConnectorsTab();
		const card = page.getByTestId(`market-connector-${connectorEntry.id}`);
		await expect.element(card).toBeVisible();
		await expect.element(page.getByText(m.market_connectors_requires_approval())).toBeVisible();
		await expect.element(card.getByText('数据, 企业')).toBeVisible();
		await expect.element(card.getByText(m.market_not_connected())).toBeVisible();
	});

	it('shows markdown detail with examples and submits a request', async () => {
		const created = requestFixture('pending');
		const submit = vi.fn();
		worker.use(
			http.post('/api/connector-access-requests', () => {
				submit();
				return HttpResponse.json(created, { status: 201 });
			})
		);

		await renderMarketPage();
		await openConnectorsTab();
		await page.getByTestId(`market-connector-${connectorEntry.id}`).click();

		// Markdown description renders as paragraphs, not raw markdown source.
		await expect.element(page.getByText('企业信息查询连接器。')).toBeVisible();

		// 试试这样用 derives quoted prompt lines from the description.
		await expect.element(page.getByText(m.market_try_this())).toBeVisible();
		await expect.element(page.getByText('帮我查询杭州的 AI 公司')).toBeVisible();

		// Filing a request flips the CTA to 审批中.
		await page.getByRole('button', { name: m.market_request_connect() }).click();
		await expect.poll(() => submit).toHaveBeenCalledTimes(1);
		await expect
			.element(page.getByRole('button', { name: m.market_request_pending() }))
			.toBeDisabled();
	});

	it('shows the current user request status overview', async () => {
		const pending = await renderMarketPage({ requests: [requestFixture('pending')] });
		await openMineTab();

		const item = page.getByTestId('market-request-car1-1');
		await expect.element(page.getByTestId('market-my-requests')).toBeVisible();
		await expect.element(item.getByText('企业信息查询')).toBeVisible();
		await expect.element(item.getByText('需要查数据')).toBeVisible();
		await expect.element(item.getByText(m.market_request_pending())).toBeVisible();
		await expect
			.element(item.getByRole('time'))
			.toHaveAttribute('title', formatTimeAgo(requestFixture('pending').created).fullDate);
		await pending.unmount();

		await renderMarketPage({ requests: [requestFixture('approved')] });
		await openMineTab();
		await expect
			.element(page.getByTestId('market-request-car1-1').getByText(m.market_request_approved()))
			.toBeVisible();
	});

	it('shows an empty state when the user has no connector requests', async () => {
		await renderMarketPage();
		await openMineTab();

		await expect.element(page.getByTestId('market-my-requests')).toBeVisible();
		await expect.element(page.getByText(m.market_my_requests_empty())).toBeVisible();
	});

	it('groups approved and connected connectors under Available', async () => {
		const profile = createMockProfile([Group.USER]);
		const personal = createVMCP(
			{ id: 'vmcp2-personal', displayName: '我的连接', userID: profile.id },
			[connectorEntry]
		);
		await renderMarketPage({ profile, vmcps: [personal] });
		await openConnectorsTab();

		await expect.element(page.getByText(m.market_connectors_available())).toBeVisible();
		await expect.element(page.getByTestId(`market-connector-${connectorEntry.id}`)).toBeVisible();
	});

	it('connects an authorized connector without filing an access request', async () => {
		const profile = createMockProfile([Group.USER]);
		const authorizedEntry = {
			...structuredClone(connectorEntry),
			authorized: true
		};
		const created = createVMCP(
			{ id: 'vmcp1authorized', displayName: 'Authorized Connector', userID: profile.id },
			[authorizedEntry]
		);
		const requestAccess = vi.fn();
		const createVMCPRequest = vi.fn();
		worker.use(
			http.get('/api/all-mcps/entries', () => HttpResponse.json({ items: [authorizedEntry] })),
			http.post('/api/connector-access-requests', () => {
				requestAccess();
				return HttpResponse.json({ status: 400 });
			}),
			http.post('/api/vmcps', async ({ request }) => {
				createVMCPRequest(await request.json());
				return HttpResponse.json(created, { status: 201 });
			}),
			http.get('/api/vmcps/vmcp1authorized', () => HttpResponse.json(created))
		);

		await renderMarketPage({ profile, vmcps: [], catalogEntries: [authorizedEntry] });
		await openConnectorsTab();

		const card = page.getByTestId(`market-connector-${connectorEntry.id}`);
		await expect.element(page.getByText(m.market_connectors_available())).toBeVisible();
		await expect.element(card.getByText(m.market_authorized())).toBeVisible();
		await expect
			.element(page.getByText(m.market_connectors_requires_approval()))
			.not.toBeInTheDocument();

		await card.click();
		await page.getByRole('button', { name: m.market_connect() }).click();

		await expect.element(page.getByCSS('#connect-to-vmcp-dialog')).toBeVisible();
		await expect.element(page.getByText(m.vmcps_connection_url())).toBeVisible();
		expect(requestAccess).not.toHaveBeenCalled();
		expect(createVMCPRequest).toHaveBeenCalledWith(
			expect.objectContaining({
				components: [expect.objectContaining({ mcpServerCatalogEntryID: connectorEntry.id })]
			})
		);
	});

	it('shows authorized enterprise vMCPs in the connectors tab and opens the connect dialog', async () => {
		const profile = createMockProfile([Group.USER]);
		const enterprise = createVMCP(
			{
				id: 'vmcp-enterprise',
				displayName: '企业数据服务',
				description: '由企业统一配置的企业级数据连接方案',
				userID: '',
				creatorUserID: 'admin-user'
			},
			[connectorEntry, createMCPCatalogEntry({ id: 'mce2-enterprise', name: 'Second Service' })]
		);
		const personal = createVMCP(
			{ id: 'vmcp-personal-only', displayName: '我的私有链接', userID: profile.id },
			[connectorEntry]
		);

		await renderMarketPage({ profile, vmcps: [enterprise, personal] });
		await openConnectorsTab();

		const section = page.getByText(m.market_enterprise_solutions());
		await expect.element(section).toBeVisible();
		await expect.element(page.getByText(m.market_enterprise_solutions_desc())).toBeVisible();

		const card = page.getByTestId(`market-enterprise-${enterprise.id}`);
		await expect.element(card).toBeVisible();
		await expect.element(card.getByText('企业数据服务')).toBeVisible();
		await expect.element(card.getByText('由企业统一配置的企业级数据连接方案')).toBeVisible();
		await expect.element(card.getByText(m.market_enterprise_badge())).toBeVisible();
		await expect
			.element(page.getByTestId(`market-enterprise-setup-${enterprise.id}`))
			.not.toBeInTheDocument();
		await expect
			.element(card.getByText(m.market_enterprise_services_count({ count: 2 })))
			.toBeVisible();

		await expect
			.element(page.getByTestId(`market-enterprise-${personal.id}`))
			.not.toBeInTheDocument();
		await expect.element(page.getByText('我的私有链接', { exact: true })).not.toBeInTheDocument();

		// Marketplace search narrows both connector entries and enterprise solutions.
		await page.getByPlaceholder(m.market_search_connectors()).fill('统一配置');
		await expect.element(card).toBeVisible();
		await expect
			.element(page.getByTestId(`market-connector-${connectorEntry.id}`))
			.not.toBeInTheDocument();

		await card.click();

		const dialog = page.getByCSS('#connect-to-vmcp-dialog');
		await expect.element(dialog).toBeVisible();
		await expect.element(page.getByText(m.vmcps_connection_url())).toBeVisible();
		await expect.element(dialog.getByRole('heading', { name: '企业数据服务' })).toBeVisible();
	});

	it('flags enterprise vMCPs that need setup and strips markdown from card descriptions', async () => {
		const profile = createMockProfile([Group.USER]);
		const needsSetup = createVMCP(
			{
				id: 'vmcp-enterprise-setup',
				displayName: '企业数据中台',
				description:
					'需要配置 Metabase Key。请参考 [查看该 MCP 的 Key](/data-permissions?view=api-keys) 完成授权。',
				userID: '',
				creatorUserID: 'admin-user'
			},
			[connectorEntry]
		);
		needsSetup.components![0].configuration = [{ key: 'METABASE_API_KEY', policy: 'userAllowed' }];
		const instanceNeedsSetup = createVMCP(
			{
				id: 'vmcp-enterprise-instance-setup',
				displayName: '企业报表服务',
				userID: '',
				creatorUserID: 'admin-user'
			},
			[connectorEntry]
		);

		await renderMarketPage({ profile, vmcps: [needsSetup, instanceNeedsSetup] });
		await openConnectorsTab();

		const card = page.getByTestId(`market-enterprise-${needsSetup.id}`);
		await expect.element(card).toBeVisible();

		const setupBadge = page.getByTestId(`market-enterprise-setup-${needsSetup.id}`);
		await expect.element(setupBadge).toBeVisible();
		await expect.element(setupBadge).toHaveTextContent(m.market_enterprise_needs_setup());
		await expect.element(card.getByText(m.market_enterprise_needs_setup_hint())).toBeVisible();

		await expect
			.element(card.getByText('需要配置 Metabase Key。请参考 查看该 MCP 的 Key 完成授权。'))
			.toBeVisible();
		await expect.element(card.getByText(/\]\(/)).not.toBeInTheDocument();

		// The badge also follows the current user's instance status, not only the definition.
		await expect
			.element(page.getByTestId(`market-enterprise-setup-${instanceNeedsSetup.id}`))
			.not.toBeInTheDocument();
		vmcpInstances.current = {
			items: [
				{
					id: 'vmcpi-instance-setup',
					vmcpID: instanceNeedsSetup.id,
					userID: profile.id,
					created: '2026-01-01T00:00:00Z',
					status: {
						configured: false,
						missingRequiredConfiguration: ['component-1.API_TOKEN']
					}
				}
			],
			loading: false
		};
		await expect
			.element(page.getByTestId(`market-enterprise-setup-${instanceNeedsSetup.id}`))
			.toBeVisible();
	});

	it('shows rejected review comments and allows re-requesting', async () => {
		const rejected = {
			...requestFixture('rejected'),
			manifest: {
				...requestFixture('rejected').manifest,
				reviewComment: '暂时不开放'
			}
		};
		await renderMarketPage({ requests: [rejected] });
		await openConnectorsTab();

		await page.getByTestId(`market-connector-${connectorEntry.id}`).click();
		await expect.element(page.getByText(`${m.market_rejected_reason()}：暂时不开放`)).toBeVisible();
		await expect
			.element(page.getByRole('button', { name: m.market_request_connect() }))
			.toBeVisible();
	});

	it('marks connectors composed into a personal vMCP as connected', async () => {
		const profile = createMockProfile([Group.USER]);
		const personal = createVMCP(
			{ id: 'vmcp2-personal', displayName: '我的连接', userID: profile.id },
			[connectorEntry]
		);
		await renderMarketPage({ profile, vmcps: [personal] });

		await openConnectorsTab();
		const card = page.getByTestId(`market-connector-${connectorEntry.id}`);
		await expect.element(card).toBeVisible();
		await expect.element(card.getByText(m.market_connected())).toBeVisible();
	});

	it('opens the existing connection in place for a basic user', async () => {
		const profile = createMockProfile([Group.USER]);
		const personal = createVMCP(
			{ id: 'vmcp1connected', displayName: 'Connected Connector', userID: profile.id },
			[connectorEntry]
		);

		await renderMarketPage({ profile, vmcps: [personal] });
		await openConnectorsTab();
		await page.getByTestId(`market-connector-${connectorEntry.id}`).click();
		await page.getByRole('button', { name: m.market_connect() }).click();

		await expect.element(page.getByCSS('#connect-to-vmcp-dialog')).toBeVisible();
		await expect.element(page.getByText(m.vmcps_connection_url())).toBeVisible();
		expect(appPage.url.pathname).toBe('/market');
	});

	it('lists the current user personal vMCPs under Mine', async () => {
		const profile = createMockProfile([Group.USER]);
		const personal = createVMCP(
			{ id: 'vmcp2-personal', displayName: '我的连接', userID: profile.id },
			[connectorEntry]
		);
		const anotherUsers = createVMCP(
			{ id: 'vmcp3-other', displayName: '别人的连接', userID: 'another-user' },
			[connectorEntry]
		);
		await renderMarketPage({ profile, vmcps: [personal, anotherUsers] });
		await openMineTab();

		const connection = page.getByTestId('market-my-vmcp-vmcp2-personal');
		await expect.element(page.getByTestId('market-my-connections')).toBeVisible();
		await expect.element(connection).toBeVisible();
		await expect.element(connection.getByText('我的连接')).toBeVisible();
		await expect
			.element(page.getByTestId('market-my-vmcp-components-vmcp2-personal'))
			.toHaveTextContent(m.market_components_count({ count: 1 }));
		await expect.element(page.getByTestId('market-my-vmcp-vmcp3-other')).not.toBeInTheDocument();
	});

	it('shows operational status for my vMCPs under Mine', async () => {
		const profile = createMockProfile([Group.USER]);
		const ready = createVMCP({ id: 'vmcp2-ready', displayName: '就绪连接', userID: profile.id }, [
			connectorEntry
		]);
		const pending = createVMCP(
			{
				id: 'vmcp3-pending',
				displayName: '待配置连接',
				creatorUserID: profile.id,
				userID: '',
				status: { ready: false }
			},
			[connectorEntry, createMCPCatalogEntry({ id: 'mce2-extra', name: 'Extra' })]
		);

		await renderMarketPage({ profile, vmcps: [ready, pending] });
		vmcpInstances.current = {
			items: [
				{
					id: 'vmcpi-ready',
					vmcpID: ready.id,
					userID: profile.id,
					created: '2026-01-01T00:00:00Z',
					status: { configured: true }
				}
			],
			loading: false
		};
		await openMineTab();

		const readyRow = page.getByTestId('market-my-vmcp-vmcp2-ready');
		await expect.element(readyRow).toBeVisible();
		await expect.element(readyRow.getByText(m.core_status_ready())).toBeVisible();
		await expect.element(readyRow.getByText(m.core_status_configured())).toBeVisible();
		await expect
			.element(page.getByTestId('market-my-vmcp-instances-vmcp2-ready'))
			.toHaveTextContent(m.market_instances_count({ count: 1 }));

		const pendingRow = page.getByTestId('market-my-vmcp-vmcp3-pending');
		await expect.element(pendingRow).toBeVisible();
		await expect.element(pendingRow.getByText(m.market_not_ready())).toBeVisible();
		await expect.element(pendingRow.getByText(m.market_not_configured())).toBeVisible();
		await expect
			.element(page.getByTestId('market-my-vmcp-components-vmcp3-pending'))
			.toHaveTextContent(m.market_components_count({ count: 2 }));
		await expect
			.element(page.getByTestId('market-my-vmcp-instances-vmcp3-pending'))
			.toHaveTextContent(m.market_instances_count({ count: 0 }));
	});

	it('shows only the current user vMCPs under My links', async () => {
		const profile = createMockProfile([Group.USER]);
		const owned = createVMCP({ id: 'vmcp2-owned', displayName: '我的链接', userID: profile.id }, [
			connectorEntry
		]);
		const created = createVMCP(
			{ id: 'vmcp3-created', displayName: '我创建的链接', creatorUserID: profile.id },
			[connectorEntry]
		);
		const anotherUsers = createVMCP(
			{
				id: 'vmcp4-other',
				displayName: '别人的链接',
				userID: 'another-user',
				creatorUserID: 'another-user'
			},
			[connectorEntry]
		);
		await renderMarketPage({ profile, vmcps: [owned, created, anotherUsers] });
		await openMyLinksTab();

		await expect.element(page.getByText('我的链接', { exact: true }).last()).toBeVisible();
		await expect.element(page.getByText('我创建的链接', { exact: true })).toBeVisible();
		await expect.element(page.getByText('别人的链接', { exact: true })).not.toBeInTheDocument();
		await expect
			.element(page.getByRole('button', { name: m.vmcps_connect() }).first())
			.toBeVisible();
		await expect
			.element(page.getByRole('button', { name: m.vmcps_copy_connect_url() }).first())
			.toBeVisible();
		await expect
			.element(page.getByRole('button', { name: m.vmcps_test_vmcp() }).first())
			.toBeVisible();
	});

	it('creates an approved connector as a personal vMCP and opens the harness dialog in place', async () => {
		const profile = createMockProfile([Group.ADMIN]);
		const created = createVMCP(
			{
				id: 'vmcp1market',
				displayName: 'Marketplace Connector',
				userID: profile.id
			},
			[connectorEntry]
		);
		const create = vi.fn();
		const requestURL = vi.fn();
		worker.use(
			http.post('/api/vmcps', async ({ request }) => {
				requestURL(request.url);
				create(await request.json());
				return HttpResponse.json(created, { status: 201 });
			}),
			http.get('/api/vmcps/vmcp1market', () => HttpResponse.json(created))
		);

		await renderMarketPage({ profile, requests: [requestFixture('approved')] });
		await openConnectorsTab();
		await page.getByTestId(`market-connector-${connectorEntry.id}`).click();
		await page.getByRole('button', { name: m.market_connect() }).click();

		await expect.poll(() => create).toHaveBeenCalledTimes(1);
		expect(requestURL).toHaveBeenCalledWith(expect.stringMatching(/\/api\/vmcps\?scope=personal$/));
		expect(create).toHaveBeenCalledWith(
			expect.objectContaining({
				components: [expect.objectContaining({ mcpServerCatalogEntryID: connectorEntry.id })]
			})
		);
		await expect.element(page.getByCSS('#connect-to-vmcp-dialog')).toBeVisible();
		await expect.element(page.getByText(m.vmcps_connection_url())).toBeVisible();
	});

	it('reuses an existing personal vMCP with the same connector without creating another', async () => {
		const profile = createMockProfile([Group.USER]);
		const existing = createVMCP(
			{
				id: 'vmcp1existing',
				displayName: 'Existing Connector',
				creatorUserID: profile.id,
				userID: ''
			},
			[connectorEntry]
		);
		existing.components![0].mcpServerCatalogEntryID = '';
		existing.components![0].catalogEntry = {
			...existing.components![0].catalogEntry,
			id: connectorEntry.id
		} as (typeof existing.components)[number]['catalogEntry'];
		const create = vi.fn();

		await renderMarketPage({ profile, requests: [requestFixture('approved')], vmcps: [] });
		worker.use(
			http.get('/api/vmcps', () => HttpResponse.json({ items: [existing] })),
			http.post('/api/vmcps', async ({ request }) => {
				create(await request.json());
				return HttpResponse.json(existing, { status: 201 });
			}),
			http.get(`/api/vmcps/${existing.id}`, () => HttpResponse.json(existing))
		);

		await openConnectorsTab();
		await page.getByTestId(`market-connector-${connectorEntry.id}`).click();
		await page.getByRole('button', { name: m.market_connect() }).click();

		await expect.element(page.getByCSS('#connect-to-vmcp-dialog')).toBeVisible();
		expect(create).not.toHaveBeenCalled();
	});

	it('shows one-line install commands and call instructions when a skill is clicked', async () => {
		worker.use(
			http.get(`/api/skills/${skill.id}/download-url`, () =>
				HttpResponse.json({ path: `/api/skills/${skill.id}/download?expires=1&token=t` })
			)
		);

		await renderMarketPage();
		const skillCard = page.getByTestId('market-skill-sk1-report');
		await skillCard.click();

		// Modal displays the skill usage section, one-line commands, and call hint
		const modal = page.getByRole('dialog', { name: '中文周报技能' });
		await expect.element(modal).toBeVisible();
		await expect.element(modal.getByText(m.market_skill_usage_title())).toBeVisible();
		await expect.element(modal.getByText(m.skills_install_step_command())).toBeVisible();
		await expect.element(modal.getByText(m.market_skill_step_call())).toBeVisible();

		// One-line commands for clients (Cursor, Claude Code, Codex) are rendered in modal
		await expect.element(modal.getByText('Cursor')).toBeVisible();
		await expect.element(modal.getByText('Claude Code')).toBeVisible();
		await expect.element(modal.getByText('Codex')).toBeVisible();

		// The command embeds the signed download URL
		const expectedCommand = `tmp="$(mktemp -d)" && curl -fsSL "${window.location.origin}/api/skills/sk1-report/download?expires=1&token=t" -o "$tmp/skill.zip" && mkdir -p "$HOME/.claude/skills/weekly-report-en" && unzip -oq "$tmp/skill.zip" -d "$HOME/.claude/skills/weekly-report-en" && rm -rf "$tmp"`;
		await expect.element(modal.getByCSS('#command-claudeCode')).toHaveValue(expectedCommand);
	});

	it('downloads the skill zip locally when the download button is clicked', async () => {
		vi.mocked(saveBlob).mockClear();
		worker.use(
			http.get(`/api/skills/${skill.id}/download-url`, () =>
				HttpResponse.json({ path: `/api/skills/${skill.id}/download?expires=1&token=t` })
			),
			http.get(`/api/skills/${skill.id}/download`, () =>
				HttpResponse.arrayBuffer(new Uint8Array([0x50, 0x4b, 0x03, 0x04]).buffer)
			)
		);

		await renderMarketPage();
		await page.getByTestId('market-skill-sk1-report').click();

		const modal = page.getByRole('dialog', { name: '中文周报技能' });
		await modal.getByRole('button', { name: m.skills_download() }).click();

		await expect.poll(() => vi.mocked(saveBlob).mock.calls).toHaveLength(1);
		const [blob, filename] = vi.mocked(saveBlob).mock.calls[0];
		expect(blob).toBeInstanceOf(Blob);
		expect(filename).toBe('weekly-report-en.zip');
	});
	it('shows unpublished hint when an approved request connector is not published in market', async () => {
		const approvedUnpublished: ConnectorAccessRequest = {
			...requestFixture('approved'),
			connectorPublished: false
		};
		await renderMarketPage({ requests: [approvedUnpublished] });
		await openMineTab();

		const item = page.getByTestId('market-request-car1-1');
		await expect.element(item.getByText(m.market_request_approved())).toBeVisible();
		await expect
			.element(item.getByText(m.market_request_approved_unpublished_hint()))
			.toBeVisible();
	});
});
