<script lang="ts">
	import type { DateRange } from '$lib/components/Calendar.svelte';
	import Layout from '$lib/components/Layout.svelte';
	import AuditLogCalendar from '$lib/components/admin/audit-logs/AuditLogCalendar.svelte';
	import { buildDeviceScanTopBuckets } from '$lib/components/admin/device-scan/deviceScanTopBuckets';
	import { formatTokenUsageUSD } from '$lib/components/admin/token-usage/tokenUsageTimeline';
	import {
		transformIndividualToolResponseTimes,
		transformMostActiveUsers,
		transformToolCallErrors,
		transformToolCallErrorsByServer
	} from '$lib/components/admin/usage/utils';
	import IconButton from '$lib/components/primitives/IconButton.svelte';
	import { DEFAULT_MCP_CATALOG_ID } from '$lib/constants';
	import { m } from '$lib/i18n';
	import { localState } from '$lib/runes/localState.svelte';
	import {
		AdminService,
		UserService,
		type DeviceClientStat,
		type DeviceMCPServerStat,
		type DeviceScanStats,
		type DeviceSkillStat,
		type MCPCatalogServer,
		type McpAuditLogUsageStats,
		type OrgUser,
		type TokenUsageStats,
		type TotalTokenUsage
	} from '$lib/services';
	import {
		avgToolCallResponseTimeFromStats,
		compileServerAndEntries,
		topServersFromStats,
		topToolCallsFromStats
	} from '$lib/services/dashboard/utils';
	import { errors, mcpServersAndEntries, profile, version } from '$lib/stores';
	import { getTimeRangeFilter } from '$lib/time';
	import ConfigureDashboard from './ConfigureDashboard.svelte';
	import HeaderBar from './HeaderBar.svelte';
	import {
		DASHBOARD_LAYOUT_KEY,
		DEVICE_STAT_IDS,
		MODEL_STAT_IDS,
		PLATFORM_STAT_IDS,
		availableWidgets,
		colSpanClass,
		commitLayout,
		factoryConfig,
		type WidgetId,
		headerVisible,
		isHiddenBecauseEmpty,
		moveHeader,
		parseDashboardConfig,
		placeWidget,
		projectConfig,
		rowsForRender,
		visibleStatIds,
		widgetStatuses,
		type DashboardConfig,
		type DashboardRow,
		type DeviceStatId,
		type DropTarget,
		type ModelStatId,
		type PlatformStatId,
		type WidgetPlacement
	} from './dashboardLayout';
	import { dragScrollDelta } from './dragScroll';
	import DashboardWidget from './widgets/DashboardWidget.svelte';
	import WidgetFrame from './widgets/WidgetFrame.svelte';
	import { deviceStatLabel, platformStatLabel } from './widgets/labels';
	import type { DashboardStatTile, PopularServer } from './widgets/model';
	import {
		Activity,
		CircleDollarSign,
		Coins,
		Columns3Cog,
		Laptop,
		LayoutDashboard,
		MonitorCheck,
		PencilRuler,
		Server,
		Users
	} from '@lucide/svelte';
	import { isWithinInterval } from 'date-fns';
	import { onDestroy, onMount } from 'svelte';
	import { twMerge } from 'tailwind-merge';

	let { data } = $props();
	let hasDeviceScans = $derived(data?.hasDeviceScans ?? false);
	let hasMcpStats = $derived(data?.hasMcpStats ?? false);
	let hasLlmStats = $derived(data?.hasLlmStats ?? false);
	const headerAvailability = $derived({
		deviceScans: hasDeviceScans,
		mcpStats: hasMcpStats,
		llmStats: hasLlmStats
	});
	let loading = $state(true);
	let loadingToolUsage = $state(true);
	let loadingDeviceScanStats = $state(true);
	let loadingTokens = $state(true);
	let loadingTokenUsageStats = $state(true);

	let configureDashboardDialog = $state<ReturnType<typeof ConfigureDashboard>>();

	let usersData = $state<OrgUser[]>([]);
	let totalTokensData = $state<TotalTokenUsage>();
	let tokenUsageStats = $state<TokenUsageStats>();

	const doesSupportK8sUpdates = $derived(version.current.engine === 'kubernetes');
	const isAdmin = $derived(Boolean(profile.current.hasAdminAccess?.()));
	const viewer = $derived({ isAdmin });

	const storedLayout = localState<DashboardConfig | null>(DASHBOARD_LAYOUT_KEY, null, {
		parse: parseDashboardConfig
	});
	const dashboardLayout = $derived.by(() => {
		if (!storedLayout.isReady || storedLayout.current == null) return factoryConfig(viewer);
		return projectConfig(storedLayout.current, viewer);
	});

	let mcpUsageStats = $state<McpAuditLogUsageStats>();
	const topToolCalls = $derived(topToolCallsFromStats(mcpUsageStats));
	const topServerUsage = $derived(topServersFromStats(mcpUsageStats));
	const avgToolCallResponseTime = $derived(avgToolCallResponseTimeFromStats(mcpUsageStats));
	const individualToolResponses = $derived(transformIndividualToolResponseTimes(mcpUsageStats));
	const toolCallErrors = $derived(transformToolCallErrors(mcpUsageStats));
	const toolCallErrorsByServer = $derived(transformToolCallErrorsByServer(mcpUsageStats));
	const mostActiveUsers = $derived(transformMostActiveUsers(mcpUsageStats));
	let deviceScanStats = $state<DeviceScanStats | null>(null);
	let maxToolsToShow = $derived(hasDeviceScans ? 3 : 5);
	let maxServersToShow = $derived(hasDeviceScans ? 10 : 12);

	const { startTime: initialStart, endTime: initialEnd } = getTimeRangeFilter({ days: 30 });
	let end = $state(initialEnd);
	let start = $state(initialStart);
	const rangeBusy = $derived(
		loadingToolUsage || loadingTokens || loadingTokenUsageStats || loadingDeviceScanStats
	);
	let rangeRequest = 0;

	let monthlyActiveUsers = $derived(
		usersData.filter(
			(user) => user.lastActiveDay && isWithinInterval(new Date(user.lastActiveDay), { start, end })
		).length
	);

	let deployedCatalogEntryServers = $state<MCPCatalogServer[]>([]);
	let deployedWorkspaceCatalogEntryServers = $state<MCPCatalogServer[]>([]);
	let serversData = $derived.by(() => {
		if (mcpServersAndEntries.current.loading || loading) return [];
		// eslint-disable-next-line svelte/prefer-svelte-reactivity
		const seen = new Set<string>();
		const result: MCPCatalogServer[] = [];
		for (const list of [
			deployedCatalogEntryServers,
			deployedWorkspaceCatalogEntryServers,
			mcpServersAndEntries.current.servers
		]) {
			for (const server of list) {
				if (server.deleted || seen.has(server.id)) continue;
				seen.add(server.id);
				result.push(server);
			}
		}
		return result;
	});

	const serverAndEntries = $derived(mcpServersAndEntries.current);
	const { graphData, popularServers, totalServers, deploymentStatusBreakdown } = $derived(
		compileServerAndEntries(serversData, serverAndEntries.entries, doesSupportK8sUpdates)
	);

	let isBootStrapUser = $derived(profile.current.isBootstrapUser?.() ?? false);

	function handleDateRangeChange(range: DateRange) {
		if (!range.start || !range.end) return;
		start = range.start;
		end = range.end;
	}

	function applyLayout(next: DashboardConfig) {
		const previous = storedLayout.current ?? factoryConfig(viewer);
		storedLayout.current = commitLayout(previous, next, viewer);
	}

	function resetLayout() {
		storedLayout.current = null;
	}

	type DragSubject = { kind: 'widget'; id: WidgetId } | { kind: 'header'; id: string };

	let dragging = $state<DragSubject | null>(null);
	let draft = $state<DashboardConfig | null>(null);
	let dragOrigin = $state<string | null>(null);
	let dragClone: HTMLElement | null = null;
	let dragOffsetX = 0;
	let dragOffsetY = 0;
	let dragPointerX = 0;
	let dragPointerY = 0;
	let dragScrollFrame = 0;
	const editingLayout = $derived(draft ?? dashboardLayout);

	function updateLayout(next: DashboardConfig) {
		applyLayout(next);
		if (dragging) draft = next;
	}

	function showRow(row: DashboardRow): boolean {
		if (row.kind === 'header') return headerVisible(row, headerAvailability);
		if (row.kind === 'box') {
			if (row.rows.length === 0) return true;
			return row.rows.some(showRow);
		}
		return row.items.some((item) => !isHiddenBecauseEmpty(item, statuses[item.id]));
	}

	function visiblePlacements(items: WidgetPlacement[]) {
		return items.flatMap((item, index) =>
			isHiddenBecauseEmpty(item, statuses[item.id]) ? [] : [{ item, index }]
		);
	}

	function parsePath(value: string | undefined): number[] {
		if (!value) return [];
		return value.split('-').map((part) => Number(part));
	}

	function draggedNode(): HTMLElement | null {
		if (!dragging || typeof document === 'undefined') return null;
		const selector =
			dragging.kind === 'widget'
				? `[data-dashboard-card][data-widget-id="${CSS.escape(dragging.id)}"]`
				: `[data-dashboard-header][data-header-id="${CSS.escape(dragging.id)}"]`;
		for (const match of document.querySelectorAll(selector)) {
			if (match instanceof HTMLElement && !match.closest('[data-drag-clone]')) return match;
		}
		return null;
	}

	function pointInside(node: HTMLElement, x: number, y: number): boolean {
		const rect = node.getBoundingClientRect();
		return x >= rect.left && x <= rect.right && y >= rect.top && y <= rect.bottom;
	}

	function nodeArea(node: HTMLElement): number {
		const rect = node.getBoundingClientRect();
		return rect.width * rect.height;
	}

	function actionFor(
		node: HTMLElement,
		x: number,
		y: number,
		kind: DragSubject['kind']
	): DropTarget {
		const rect = node.getBoundingClientRect();
		if (node.hasAttribute('data-dashboard-card')) {
			const parent = parsePath(node.dataset.parent);
			const row = Number(node.dataset.row);
			const index = Number(node.dataset.index);
			const edgeBand = Math.min(16, rect.height * 0.12);
			const edge = y < rect.top + edgeBand || y > rect.bottom - edgeBand;
			if (kind === 'header' || edge) {
				return {
					kind: 'new-row',
					path: parent,
					index: y < rect.top + rect.height / 2 ? row : row + 1
				};
			}
			return {
				kind: 'slot',
				path: [...parent, row],
				index: x < rect.left + rect.width / 2 ? index : index + 1
			};
		}
		if (node.hasAttribute('data-dashboard-header')) {
			const parent = parsePath(node.dataset.parent);
			const row = Number(node.dataset.row);
			return {
				kind: 'new-row',
				path: parent,
				index: y < rect.top + rect.height / 2 ? row : row + 1
			};
		}
		const path = parsePath(node.dataset.boxPath);
		const count = Number(node.dataset.childCount ?? '0');
		return { kind: 'new-row', path, index: y < rect.top + rect.height * 0.35 ? 0 : count };
	}

	function actionAt(x: number, y: number, subject: DragSubject): DropTarget | null {
		const dragged = draggedNode();
		const candidates = [
			...document.querySelectorAll(
				'[data-dashboard-card], [data-dashboard-header], [data-dashboard-box]'
			)
		].filter(
			(node): node is HTMLElement =>
				node instanceof HTMLElement && node !== dragged && !node.closest('[data-drag-clone]')
		);
		const hit = candidates
			.filter((node) => !node.hasAttribute('data-dashboard-box') && pointInside(node, x, y))
			.sort((a, b) => nodeArea(a) - nodeArea(b))[0];
		if (hit) return actionFor(hit, x, y, subject.kind);
		if (dragged && pointInside(dragged, x, y)) return null;
		const box = candidates
			.filter((node) => node.hasAttribute('data-dashboard-box') && pointInside(node, x, y))
			.sort((a, b) => nodeArea(a) - nodeArea(b))[0];
		return box ? actionFor(box, x, y, subject.kind) : null;
	}

	function clearDragClone() {
		dragClone?.remove();
		dragClone = null;
	}

	function mountDragClone(event: PointerEvent) {
		if (typeof document === 'undefined') return;
		const current = event.currentTarget;
		const source =
			current instanceof Element
				? current.closest('[data-dashboard-card], [data-dashboard-header]')
				: null;
		if (!(source instanceof HTMLElement)) return;
		const heading = source.querySelector('h2, h3, h4, h5, h6');
		const anchor = heading instanceof HTMLElement ? heading : source;
		const rect = anchor.getBoundingClientRect();
		const clone = document.createElement('div');

		clearDragClone();
		clone.textContent = anchor.textContent?.trim() ?? '';
		clone.setAttribute('aria-hidden', 'true');
		clone.setAttribute('data-drag-clone', '');
		clone.className = source.hasAttribute('data-dashboard-header')
			? 'bg-base-100 dark:bg-base-300 border-base-300 rounded-md border px-3 py-2 text-lg font-semibold shadow-xl'
			: 'bg-base-100 dark:bg-base-300 border-base-300 rounded-md border px-3 py-2 text-xs font-medium tracking-wider uppercase shadow-xl';
		clone.style.position = 'fixed';
		clone.style.margin = '0';
		clone.style.width = 'max-content';
		clone.style.maxWidth = '420px';
		clone.style.pointerEvents = 'none';
		clone.style.zIndex = '70';
		clone.style.opacity = '0.95';
		document.body.appendChild(clone);
		const cloneRect = clone.getBoundingClientRect();
		dragOffsetX = Math.min(
			Math.max(event.clientX - rect.left, 0),
			Math.max(cloneRect.width - 8, 0)
		);
		dragOffsetY = Math.min(
			Math.max(event.clientY - rect.top, 0),
			Math.max(cloneRect.height - 8, 0)
		);
		clone.style.left = `${event.clientX - dragOffsetX}px`;
		clone.style.top = `${event.clientY - dragOffsetY}px`;
		dragClone = clone;
	}

	function moveDragClone(x: number, y: number) {
		if (!dragClone) return;
		dragClone.style.left = `${x - dragOffsetX}px`;
		dragClone.style.top = `${y - dragOffsetY}px`;
	}

	function dashboardScroller(): HTMLElement | null {
		if (typeof document === 'undefined') return null;
		const anchor = document.querySelector(
			'[data-dashboard-card], [data-dashboard-header], [data-dashboard-box]'
		);
		let current = anchor?.parentElement ?? null;
		while (current) {
			const overflowY = getComputedStyle(current).overflowY;
			if (
				(overflowY === 'auto' || overflowY === 'scroll') &&
				current.scrollHeight > current.clientHeight + 1
			) {
				return current;
			}
			current = current.parentElement;
		}
		return null;
	}

	function placeDraggedAtPointer() {
		if (!dragging) return;
		moveDragClone(dragPointerX, dragPointerY);
		const action = actionAt(dragPointerX, dragPointerY, dragging);
		if (!action) return;
		const layout = draft ?? dashboardLayout;
		const next = moveSubject(layout, dragging, action);
		if (JSON.stringify(next.rows) !== JSON.stringify(layout.rows)) draft = next;
	}

	function stopDragAutoScroll() {
		if (!dragScrollFrame || typeof cancelAnimationFrame === 'undefined') return;
		cancelAnimationFrame(dragScrollFrame);
		dragScrollFrame = 0;
	}

	function tickDragAutoScroll() {
		dragScrollFrame = 0;
		if (!dragging) return;
		const scroller = dashboardScroller();
		const delta = dragScrollDelta(scroller, dragPointerY);
		if (!delta || !scroller) return;
		scroller.scrollTop += delta;
		placeDraggedAtPointer();
		dragScrollFrame = requestAnimationFrame(tickDragAutoScroll);
	}

	function syncDragAutoScroll() {
		if (!dragScrollDelta(dashboardScroller(), dragPointerY)) {
			stopDragAutoScroll();
			return;
		}
		if (!dragScrollFrame) dragScrollFrame = requestAnimationFrame(tickDragAutoScroll);
	}

	function moveSubject(layout: DashboardConfig, subject: DragSubject, action: DropTarget) {
		if (subject.kind === 'widget') return placeWidget(layout, subject.id, action);
		if (action.kind !== 'new-row') return layout;
		return moveHeader(layout, subject.id, action.index, action.path);
	}

	function onPointerMove(event: PointerEvent) {
		if (!dragging) return;
		dragPointerX = event.clientX;
		dragPointerY = event.clientY;
		placeDraggedAtPointer();
		syncDragAutoScroll();
	}

	function detachDragListeners() {
		stopDragAutoScroll();
		if (typeof window === 'undefined') return;
		window.removeEventListener('pointermove', onPointerMove);
		window.removeEventListener('pointerup', onPointerUp);
		window.removeEventListener('pointercancel', onPointerCancel);
		window.removeEventListener('keydown', onKeyDown);
		clearDragClone();
	}

	function finishDrag(commit: boolean) {
		if (!dragging && !draft) {
			detachDragListeners();
			return;
		}
		const next = draft;
		const origin = dragOrigin;
		dragging = null;
		draft = null;
		dragOrigin = null;
		detachDragListeners();
		if (commit && next && origin && JSON.stringify(next.rows) !== origin) applyLayout(next);
	}

	function onPointerUp() {
		finishDrag(true);
	}

	function onPointerCancel() {
		finishDrag(false);
	}

	function onKeyDown(event: KeyboardEvent) {
		if (event.key !== 'Escape') return;
		event.preventDefault();
		finishDrag(false);
	}

	function beginDrag(event: PointerEvent, subject: DragSubject) {
		if (event.button !== 0) return;
		event.preventDefault();
		event.stopPropagation();
		mountDragClone(event);
		dragging = subject;
		dragOrigin = JSON.stringify((draft ?? dashboardLayout).rows);
		draft = null;
		window.addEventListener('pointermove', onPointerMove);
		window.addEventListener('pointerup', onPointerUp);
		window.addEventListener('pointercancel', onPointerCancel);
		window.addEventListener('keydown', onKeyDown);
	}

	onDestroy(detachDragListeners);

	function loadRangeDatasets(rangeStart: Date, rangeEnd: Date) {
		const request = ++rangeRequest;
		loadingToolUsage = true;

		UserService.listMcpAuditLogUsageStats({
			start_time: rangeStart.toISOString(),
			end_time: rangeEnd.toISOString()
		})
			.then((stats) => {
				if (request !== rangeRequest) return;
				const statsToUse = (stats.items ?? []).filter(
					(s) =>
						!s.mcpID.startsWith('sms1') &&
						!s.mcpServerDisplayName.startsWith('nba1') &&
						!s.mcpServerDisplayName.startsWith('Obot ')
				);
				const adjustedStats = {
					...stats,
					items: statsToUse
				};
				mcpUsageStats = adjustedStats;
			})
			.catch((error) => {
				if (request !== rangeRequest || error?.name === 'AbortError') return;
				errors.append(error);
			})
			.finally(() => {
				if (request === rangeRequest) loadingToolUsage = false;
			});

		if (!profile.current.hasAdminAccess?.()) {
			loadingTokens = false;
			loadingTokenUsageStats = false;
			loadingDeviceScanStats = false;
			return;
		}

		loadingDeviceScanStats = true;
		loadingTokens = true;
		loadingTokenUsageStats = true;

		AdminService.getDeviceScanStats({
			start: rangeStart.toISOString(),
			end: rangeEnd.toISOString()
		})
			.then((stats) => {
				if (request !== rangeRequest) return;
				deviceScanStats = stats;
			})
			.catch((error) => {
				if (request !== rangeRequest || error?.name === 'AbortError') return;
				errors.append(error);
			})
			.finally(() => {
				if (request === rangeRequest) loadingDeviceScanStats = false;
			});

		AdminService.listTotalTokenUsage({ start: rangeStart, end: rangeEnd })
			.then((tokens) => {
				if (request !== rangeRequest) return;
				totalTokensData = tokens;
			})
			.catch((error) => {
				if (request !== rangeRequest || error?.name === 'AbortError') return;
				errors.append(error);
			})
			.finally(() => {
				if (request === rangeRequest) loadingTokens = false;
			});

		AdminService.getTokenUsageStats({ start: rangeStart, end: rangeEnd })
			.then((stats) => {
				if (request !== rangeRequest) return;
				tokenUsageStats = stats;
			})
			.catch((error) => {
				if (request !== rangeRequest || error?.name === 'AbortError') return;
				errors.append(error);
			})
			.finally(() => {
				if (request === rangeRequest) loadingTokenUsageStats = false;
			});
	}

	$effect(() => {
		loadRangeDatasets(start, end);
	});

	onMount(() => {
		if (!profile.current.hasAdminAccess?.()) {
			loading = false;
			return;
		}

		Promise.all([
			UserService.listUsersIncludeDeleted(),
			AdminService.listAllCatalogDeployedSingleRemoteServers(DEFAULT_MCP_CATALOG_ID),
			AdminService.listAllWorkspaceDeployedSingleRemoteServers()
		])
			.then(([users, catalogServers, workspaceServers]) => {
				usersData = users;
				deployedCatalogEntryServers = catalogServers;
				deployedWorkspaceCatalogEntryServers = workspaceServers;
			})
			.catch((error) => {
				if (error?.name === 'AbortError') return;
				errors.append(error);
			})
			.finally(() => {
				loading = false;
			});
	});

	const platformStatTiles = $derived<DashboardStatTile<PlatformStatId>[]>([
		{
			id: 'total-users',
			label: platformStatLabel('total-users'),
			loading,
			value: usersData.length,
			icon: Users,
			seeMore: '/identity-access?view=users'
		},
		{
			id: 'monthly-active-users',
			label: platformStatLabel('monthly-active-users'),
			loading,
			value: monthlyActiveUsers,
			icon: Activity,
			seeMore: '/identity-access?view=users'
		},
		{
			id: 'total-tokens',
			label: platformStatLabel('total-tokens'),
			loading: loadingTokens,
			value: totalTokensData?.totalTokens ?? 0,
			icon: Coins,
			seeMore: '/usage?view=llm'
		},
		{
			id: 'total-spend',
			label: platformStatLabel('total-spend'),
			loading: loadingTokens,
			value: totalTokensData?.totalSpend ?? 0,
			icon: CircleDollarSign,
			seeMore: '/usage?view=llm',
			format: formatTokenUsageUSD
		}
	]);

	let deviceScanClientBuckets = $derived(
		buildDeviceScanTopBuckets<DeviceClientStat>(
			deviceScanStats?.clients,
			(c) => c.name,
			(c) => c.name,
			(c) => c.deviceCount
		)
	);
	let deviceScanMcpBuckets = $derived(
		buildDeviceScanTopBuckets<DeviceMCPServerStat>(
			deviceScanStats?.mcpServers,
			(s) => s.configHash,
			(s) => s.name?.trim() || m.dashboard_unnamed(),
			(s) => s.deviceCount,
			'mcp'
		)
	);
	let deviceScanSkillBuckets = $derived(
		buildDeviceScanTopBuckets<DeviceSkillStat>(
			deviceScanStats?.skills,
			(s) => s.name,
			(s) => s.name,
			(s) => s.deviceCount,
			'skill'
		)
	);
	let totalDeviceScanClientGroups = $derived(deviceScanStats?.clients?.length ?? 0);
	let totalDeviceScanMcpGroups = $derived(deviceScanStats?.mcpServers?.length ?? 0);
	let totalDeviceScanSkillGroups = $derived(deviceScanStats?.skills?.length ?? 0);

	type DeviceScanTimelineRow = { scanned_at: string; category: 'scans' };
	let deviceScanTimelineRows = $derived<DeviceScanTimelineRow[]>(
		(deviceScanStats?.scanTimestamps ?? []).map((ts) => ({
			scanned_at: ts,
			category: 'scans' as const
		}))
	);
	let totalDeviceScanSubmissions = $derived(deviceScanStats?.scanTimestamps?.length ?? 0);

	const deviceScanTiles = $derived<DashboardStatTile<DeviceStatId>[]>([
		{
			id: 'device-overview',
			label: deviceStatLabel('device-overview'),
			loading: loadingDeviceScanStats,
			value: deviceScanStats?.deviceCount ?? 0,
			icon: Laptop,
			seeMore: '/inventory?view=devices'
		},
		{
			id: 'device-clients',
			label: deviceStatLabel('device-clients'),
			loading: loadingDeviceScanStats,
			value: deviceScanStats?.clients?.length ?? 0,
			icon: MonitorCheck,
			seeMore: '/inventory?view=device-clients'
		},
		{
			id: 'device-mcps',
			label: deviceStatLabel('device-mcps'),
			loading: loadingDeviceScanStats,
			value: deviceScanStats?.mcpServers?.length ?? 0,
			icon: Server,
			seeMore: '/inventory?view=device-mcp-servers'
		},
		{
			id: 'device-skills',
			label: deviceStatLabel('device-skills'),
			loading: loadingDeviceScanStats,
			value: deviceScanStats?.skills?.length ?? 0,
			icon: PencilRuler,
			seeMore: '/inventory?view=device-skills'
		}
	]);

	const visibleModelStats = $derived(visibleStatIds(dashboardLayout.modelStats, MODEL_STAT_IDS));
	const modelStatValue = $derived<Record<ModelStatId, number>>({
		total: totalTokensData?.totalTokens ?? 0,
		input: totalTokensData?.inputTokens ?? 0,
		output: totalTokensData?.outputTokens ?? 0,
		'cached-input':
			(totalTokensData?.cacheReadTokens ?? 0) + (totalTokensData?.cacheWriteTokens ?? 0),
		spend: totalTokensData?.totalSpend ?? 0
	});
	const visiblePlatformTiles = $derived(
		platformStatTiles.filter((tile) =>
			visibleStatIds(dashboardLayout.platformStats, PLATFORM_STAT_IDS).includes(tile.id)
		)
	);
	const visibleDeviceTiles = $derived(
		deviceScanTiles.filter((tile) =>
			visibleStatIds(dashboardLayout.deviceStats, DEVICE_STAT_IDS).includes(tile.id)
		)
	);
	const serverLoading = $derived(serverAndEntries.loading || loading);
	const statuses = $derived(
		widgetStatuses({
			platformTiles: visiblePlatformTiles,
			deviceTiles: visibleDeviceTiles,
			serverLoading,
			totalServers,
			toolUsageLoading: loadingToolUsage,
			topServerCount: topServerUsage.length,
			popularToolCount: topToolCalls.length,
			responseTimeCount: avgToolCallResponseTime.length,
			serversLoading: serverLoading,
			deployedCount: popularServers.length,
			deviceScanLoading: loadingDeviceScanStats,
			skillCount: deviceScanSkillBuckets.length,
			clientCount: deviceScanClientBuckets.length,
			mcpCount: deviceScanMcpBuckets.length,
			scanCount: deviceScanTimelineRows.length,
			tokenLoading: loadingTokens,
			totalTokens: totalTokensData?.totalTokens ?? 0,
			totalSpend: totalTokensData?.totalSpend ?? 0,
			modelTiles: visibleModelStats.map((id) => ({
				loading: loadingTokens,
				value: modelStatValue[id]
			})),
			requestStatsLoading: loadingTokenUsageStats,
			requestCount: tokenUsageStats?.totals.requests ?? 0,
			providerSpendCount: (tokenUsageStats?.providers ?? []).filter(
				(provider) => provider.totalSpend > 0
			).length,
			topModelCount:
				(tokenUsageStats?.topModelsByRequests?.length ?? 0) +
				(tokenUsageStats?.topModelsBySpend?.length ?? 0),
			individualResponseCount: individualToolResponses.length,
			toolErrorCount: toolCallErrors.length,
			serverErrorCount: toolCallErrorsByServer.length,
			activeUserCount: mostActiveUsers.length
		})
	);
	const renderedRows = $derived(rowsForRender(editingLayout, statuses, headerAvailability));
	const emptyWidgets = $derived(
		(
			Object.entries(statuses) as [WidgetId, { loading: boolean; empty: boolean } | undefined][]
		).flatMap(([id, status]) => (status && !status.loading && status.empty ? [id] : []))
	);
	const widgetModel = $derived({
		isBootStrapUser,
		hasDeviceScans,
		start,
		end,
		platformTiles: visiblePlatformTiles,
		deviceTiles: visibleDeviceTiles,
		serverLoading,
		doesSupportK8sUpdates,
		deploymentStatusBreakdown,
		totalServers,
		graphData,
		loadingToolUsage,
		topServerUsage,
		maxServersToShow,
		popularServers: popularServers as PopularServer[],
		serversLoading: serverLoading,
		topToolCalls,
		maxToolsToShow,
		avgToolCallResponseTime,
		individualToolResponses,
		toolCallErrors,
		toolCallErrorsByServer,
		mostActiveUsers,
		users: usersData,
		loadingTokens,
		totalTokenUsage: totalTokensData,
		modelStats: visibleModelStats,
		loadingTokenUsageStats,
		tokenUsageStats,
		loadingDeviceScanStats,
		deviceScanSkillBuckets,
		totalDeviceScanSkillGroups,
		deviceScanClientBuckets,
		totalDeviceScanClientGroups,
		deviceScanMcpBuckets,
		totalDeviceScanMcpGroups,
		deviceScanTimelineRows,
		totalDeviceScanSubmissions
	});
</script>

{#snippet dashboardRows(rows: DashboardRow[], parent: number[])}
	{#each rows as row, rowIndex (row.kind === 'widgets' ? row.items
				.map((placement) => placement.id)
				.join('|') : row.id)}
		{#if row.kind === 'box'}
			{#if showRow(row)}
				<div
					class="@container border-base-300 bg-base-100/35 dark:bg-base-300/35 col-span-12 rounded-xl border px-3 py-3"
					data-dashboard-box
					data-box-path={[...parent, rowIndex].join('-')}
					data-child-count={row.rows.length}
				>
					<div class="grid grid-cols-12 gap-4">
						{@render dashboardRows(row.rows, [...parent, rowIndex])}
					</div>
				</div>
			{/if}
		{:else if row.kind === 'header'}
			{#if row.text.trim() && showRow(row)}
				<HeaderBar
					{row}
					parent={parent.join('-')}
					{rowIndex}
					config={editingLayout}
					dragging={dragging?.kind === 'header' && dragging.id === row.id}
					onChange={updateLayout}
					onDragStart={(event) => beginDrag(event, { kind: 'header', id: row.id })}
				/>
			{/if}
		{:else}
			{@const visible = visiblePlacements(row.items)}
			{#if visible.length > 0}
				{#each visible as placement (placement.item.id)}
					<div
						class={twMerge('h-full min-w-0', colSpanClass(visible.length))}
						data-dashboard-card
						data-widget-id={placement.item.id}
						data-parent={parent.join('-')}
						data-row={rowIndex}
						data-index={placement.index}
					>
						<WidgetFrame
							placement={placement.item}
							config={editingLayout}
							dragging={dragging?.kind === 'widget' && dragging.id === placement.item.id}
							onChange={updateLayout}
							onDragStart={(event) => beginDrag(event, { kind: 'widget', id: placement.item.id })}
						>
							<DashboardWidget id={placement.item.id} model={widgetModel} />
						</WidgetFrame>
					</div>
				{/each}
			{/if}
		{/if}
	{/each}
{/snippet}

<Layout title={m.nav_dashboard()} classes={{ childrenContainer: 'max-w-none', container: '' }}>
	{#snippet rightNavActions()}
		<IconButton
			tooltip={{ text: m.dashboard_configure(), placement: 'left' }}
			onclick={() => configureDashboardDialog?.open()}
			class="dark:hover:bg-base-400"
		>
			<Columns3Cog class="size-6" />
		</IconButton>
		<AuditLogCalendar
			{start}
			{end}
			disabled={rangeBusy}
			onChange={handleDateRangeChange}
			classes={{
				input: 'bg-base-300 hover:bg-base-300 shadow-inner',
				calendar: 'bg-base-300 shadow-none! border not-dark:border-base-300!'
			}}
		/>
	{/snippet}
	<div class="@container grid min-w-0 w-full max-w-full grid-cols-12 gap-4">
		{#if renderedRows.length === 0}
			<div class="col-span-12 flex flex-col items-center gap-3 py-16 text-center">
				<LayoutDashboard class="text-muted-content size-24 opacity-50" />
				<p class="text-muted-content text-sm font-light">{m.dashboard_configure_empty()}</p>
				<button
					type="button"
					class="btn btn-primary"
					onclick={() => configureDashboardDialog?.open()}
				>
					{m.dashboard_configure()}
				</button>
			</div>
		{:else}
			{@render dashboardRows(editingLayout.rows, [])}
		{/if}
	</div>
</Layout>

<ConfigureDashboard
	bind:this={configureDashboardDialog}
	config={dashboardLayout}
	defaults={factoryConfig(viewer)}
	available={availableWidgets(viewer)}
	{emptyWidgets}
	onChange={applyLayout}
	onReset={resetLayout}
/>

<svelte:head>
	<title>{m.dashboard_page_title()}</title>
</svelte:head>
