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
		transformToolCallErrorsByServer,
		mcpUsageCounts
	} from '$lib/components/admin/usage/utils';
	import IconButton from '$lib/components/primitives/IconButton.svelte';
	import { DEFAULT_MCP_CATALOG_ID } from '$lib/constants';
	import { m } from '$lib/i18n';
	import {
		ownedLayoutStore,
		trackProbe,
		trackRangeDataset
	} from '$lib/runes/dashboard/dashboardData.svelte';
	import { createGridDrag } from '$lib/runes/dashboard/gridDrag.svelte';
	import {
		AdminService,
		UserService,
		type DeviceClientStat,
		type DeviceMCPServerStat,
		type DeviceSkillStat,
		type MCPCatalogServer,
		type OrgUser
	} from '$lib/services';
	import {
		availableWidgets,
		buildGridCells,
		colSpanClass,
		commitLayout,
		factoryConfig,
		type WidgetId,
		projectConfig,
		hasGridContent,
		headerVisibilities,
		hasLlmUsage,
		neededDatasets,
		placedWidgetIds,
		widgetStatuses,
		type DashboardConfig,
		type DeviceStatId,
		type GridCell,
		type ModelStatId,
		type PlatformStatId
	} from '$lib/services/dashboard/layout';
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
	import DashboardWidget from './widgets/DashboardWidget.svelte';
	import WidgetFrame from './widgets/WidgetFrame.svelte';
	import { deviceStatLabel, platformStatLabel } from './widgets/labels';
	import type { DashboardStatTile, PopularServer } from './widgets/types';
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
	import { onMount } from 'svelte';
	import { twMerge } from 'tailwind-merge';

	let { data } = $props();
	let hasDeviceScans = $derived(data?.hasDeviceScans ?? false);
	let loading = $state(true);

	let configureDashboardDialog = $state<ReturnType<typeof ConfigureDashboard>>();

	let usersData = $state<OrgUser[]>([]);

	const range = () => ({ start, end });
	const mcpUsage = trackRangeDataset({
		enabled: () => layoutReady && neededData.has('mcp-usage'),
		range,
		fetch: async (rangeStart, rangeEnd, signal) => {
			const stats = await UserService.listMcpAuditLogUsageStats(
				{ start_time: rangeStart.toISOString(), end_time: rangeEnd.toISOString() },
				{ signal }
			);
			const items = (stats.items ?? []).filter(
				(s) =>
					!s.mcpID.startsWith('sms1') &&
					!s.mcpServerDisplayName.startsWith('nba1') &&
					!s.mcpServerDisplayName.startsWith('Obot ')
			);
			return { ...stats, items };
		}
	});
	const deviceScans = trackRangeDataset({
		enabled: () => layoutReady && isAdmin && neededData.has('device-scans'),
		range,
		fetch: (rangeStart, rangeEnd, signal) =>
			AdminService.getDeviceScanStats(
				{ start: rangeStart.toISOString(), end: rangeEnd.toISOString() },
				{ signal }
			)
	});
	const tokenTotals = trackRangeDataset({
		enabled: () => layoutReady && isAdmin && neededData.has('token-totals'),
		range,
		fetch: (rangeStart, rangeEnd, signal) =>
			AdminService.listTotalTokenUsage({ start: rangeStart, end: rangeEnd }, { signal })
	});
	const tokenStats = trackRangeDataset({
		enabled: () => layoutReady && isAdmin && neededData.has('token-stats'),
		range,
		fetch: (rangeStart, rangeEnd, signal) =>
			AdminService.getTokenUsageStats({ start: rangeStart, end: rangeEnd }, {}, { signal })
	});
	const mcpUsageStats = $derived(mcpUsage.data);
	const deviceScanStats = $derived(deviceScans.data);
	const totalTokensData = $derived(tokenTotals.data);
	const tokenUsageStats = $derived(tokenStats.data);
	const loadingToolUsage = $derived(mcpUsage.loading);
	const loadingDeviceScanStats = $derived(deviceScans.loading);
	const loadingTokens = $derived(tokenTotals.loading);
	const loadingTokenUsageStats = $derived(tokenStats.loading);

	const mcpProbe = trackProbe({
		enabled: () => layoutReady && usedVisibilities.has('mcp-stats'),
		retryKey: range,
		check: async (signal) =>
			(
				await UserService.listAuditLogs(
					{ event_type: 'mcp_call', limit: 1 },
					{ signal, dontLogErrors: true }
				)
			).total > 0
	});
	const llmProbe = trackProbe({
		enabled: () => layoutReady && isAdmin && usedVisibilities.has('llm-stats'),
		retryKey: range,
		check: async (signal) =>
			hasLlmUsage(
				await AdminService.listTotalTokenUsage(
					{ start: new Date(0), end: new Date() },
					{ signal, dontLogErrors: true }
				)
			)
	});
	const headerAvailability = $derived({
		deviceScans: hasDeviceScans,
		mcpStats: mcpProbe.value,
		llmStats: llmProbe.value
	});

	const doesSupportK8sUpdates = $derived(version.current.engine === 'kubernetes');
	const isAdmin = $derived(Boolean(profile.current.hasAdminAccess?.()));
	const viewer = $derived({ isAdmin });

	const storedLayout = ownedLayoutStore(() => profile.current.id);
	const layoutReady = $derived(storedLayout.isReady);
	const dashboardLayout = $derived.by(() => {
		if (!storedLayout.isReady || storedLayout.current == null) return factoryConfig(viewer);
		return projectConfig(storedLayout.current, viewer);
	});

	const topToolCalls = $derived(topToolCallsFromStats(mcpUsageStats));
	const topServerUsage = $derived(topServersFromStats(mcpUsageStats));
	const avgToolCallResponseTime = $derived(avgToolCallResponseTimeFromStats(mcpUsageStats));
	const mcpCounts = $derived(mcpUsageCounts(mcpUsageStats));
	let maxToolsToShow = $derived(hasDeviceScans ? 3 : 5);
	let maxServersToShow = $derived(hasDeviceScans ? 10 : 12);

	const { startTime: initialStart, endTime: initialEnd } = getTimeRangeFilter({ days: 30 });
	let end = $state(initialEnd);
	let start = $state(initialStart);
	const rangeBusy = $derived(
		mcpUsage.busy || tokenTotals.busy || tokenStats.busy || deviceScans.busy
	);

	let activeUsers = $derived(
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

	const gridDrag = createGridDrag({
		layout: () => dashboardLayout,
		commit: applyLayout
	});
	const editingLayout = $derived(gridDrag.draft ?? dashboardLayout);
	const placedWidgets = $derived(placedWidgetIds(editingLayout.rows));
	const individualToolResponses = $derived(
		placedWidgets.has('tool-individual-response-time')
			? transformIndividualToolResponseTimes(mcpUsageStats)
			: []
	);
	const toolCallErrors = $derived(
		placedWidgets.has('tool-call-errors') ? transformToolCallErrors(mcpUsageStats) : []
	);
	const toolCallErrorsByServer = $derived(
		placedWidgets.has('tool-call-errors-by-server')
			? transformToolCallErrorsByServer(mcpUsageStats)
			: []
	);
	const mostActiveUsers = $derived(
		placedWidgets.has('most-active-users') ? transformMostActiveUsers(mcpUsageStats) : []
	);

	function updateLayout(next: DashboardConfig) {
		applyLayout(next);
		gridDrag.follow(next);
	}

	const neededData = $derived(neededDatasets(dashboardLayout));
	const usedVisibilities = $derived(headerVisibilities(dashboardLayout.rows));
	const probesPending = $derived(
		(usedVisibilities.has('mcp-stats') && !mcpProbe.settled) ||
			(isAdmin && usedVisibilities.has('llm-stats') && !llmProbe.settled)
	);

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
			id: 'active-users',
			label: platformStatLabel('active-users'),
			loading,
			value: activeUsers,
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

	const visibleModelStats = $derived(dashboardLayout.modelStats);
	const modelStatValue = $derived<Record<ModelStatId, number>>({
		total: totalTokensData?.totalTokens ?? 0,
		input: totalTokensData?.inputTokens ?? 0,
		output: totalTokensData?.outputTokens ?? 0,
		'cached-input':
			(totalTokensData?.cacheReadTokens ?? 0) + (totalTokensData?.cacheWriteTokens ?? 0),
		spend: totalTokensData?.totalSpend ?? 0
	});
	const visiblePlatformTiles = $derived(
		platformStatTiles.filter((tile) => dashboardLayout.platformStats.includes(tile.id))
	);
	const visibleDeviceTiles = $derived(
		deviceScanTiles.filter((tile) => dashboardLayout.deviceStats.includes(tile.id))
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
			individualResponseCount: mcpCounts.individualResponses,
			toolErrorCount: mcpCounts.toolErrors,
			serverErrorCount: mcpCounts.serverErrors,
			activeUserCount: mcpCounts.activeUsers
		})
	);
	// Built once per change of layout, statuses, or header availability (not per render pass).
	const gridCells = $derived(buildGridCells(editingLayout.rows, statuses, headerAvailability));
	const hasContent = $derived(hasGridContent(gridCells));
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

{#snippet dashboardRows(cells: GridCell[], parent: number[])}
	{#each cells as cell (cell.key)}
		{#if cell.kind === 'box'}
			<div
				class="@container border-base-300 bg-base-100/35 dark:bg-base-300/35 col-span-12 rounded-xl border px-3 py-3"
				data-dashboard-box
				data-box-path={[...parent, cell.rowIndex].join('-')}
				data-child-count={cell.row.rows.length}
			>
				<div class="grid grid-cols-12 gap-4">
					{@render dashboardRows(cell.children, [...parent, cell.rowIndex])}
				</div>
			</div>
		{:else if cell.kind === 'header'}
			<HeaderBar
				row={cell.row}
				parent={parent.join('-')}
				rowIndex={cell.rowIndex}
				config={editingLayout}
				dragging={gridDrag.dragging?.kind === 'header' && gridDrag.dragging.id === cell.row.id}
				onChange={updateLayout}
				onDragStart={(event) => gridDrag.begin(event, { kind: 'header', id: cell.row.id })}
			/>
		{:else}
			<div
				class={twMerge('h-full min-w-0', colSpanClass(cell.rowSize))}
				data-dashboard-card
				data-widget-id={cell.placement.id}
				data-parent={parent.join('-')}
				data-row={cell.rowIndex}
				data-index={cell.index}
			>
				<WidgetFrame
					placement={cell.placement}
					config={editingLayout}
					loading={statuses[cell.placement.id]?.loading ?? false}
					dragging={gridDrag.dragging?.kind === 'widget' &&
						gridDrag.dragging.id === cell.placement.id}
					onChange={updateLayout}
					onDragStart={(event) => gridDrag.begin(event, { kind: 'widget', id: cell.placement.id })}
				>
					<DashboardWidget id={cell.placement.id} model={widgetModel} />
				</WidgetFrame>
			</div>
		{/if}
	{/each}
{/snippet}

<Layout title={m.nav_dashboard()} classes={{ childrenContainer: 'max-w-none', container: '' }}>
	{#snippet rightNavActions()}
		<AuditLogCalendar
			{start}
			{end}
			disabled={rangeBusy}
			disableCustomRange
			disableSubDayRanges
			onChange={handleDateRangeChange}
			classes={{
				input: 'bg-base-300 hover:bg-base-300 shadow-inner',
				calendar: 'bg-base-300 shadow-none! border not-dark:border-base-300!'
			}}
			tooltipPlacement="bottom"
		/>
		<IconButton
			tooltip={{ text: m.dashboard_configure(), placement: 'left' }}
			onclick={() => configureDashboardDialog?.open()}
			class="dark:hover:bg-base-400"
		>
			<Columns3Cog class="size-6" />
		</IconButton>
	{/snippet}
	<div class="@container grid min-w-0 w-full max-w-full grid-cols-12 gap-4">
		{#if !hasContent && probesPending}
			<!-- Header checks still running: wait instead of flashing the empty state. -->
		{:else if !hasContent}
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
			{@render dashboardRows(gridCells, [])}
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
