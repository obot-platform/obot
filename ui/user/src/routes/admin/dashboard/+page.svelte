<script lang="ts">
	import { resolve } from '$app/paths';
	import Layout from '$lib/components/Layout.svelte';
	import TweenedMetric from '$lib/components/TweenedMetric.svelte';
	import DeviceScanDonutCard from '$lib/components/admin/device-scan/DeviceScanDonutCard.svelte';
	import DeviceScanTimelineCard from '$lib/components/admin/device-scan/DeviceScanTimelineCard.svelte';
	import { buildDeviceScanTopBuckets } from '$lib/components/admin/device-scan/deviceScanTopBuckets';
	import DonutGraph, { type DonutDatum } from '$lib/components/graph/DonutGraph.svelte';
	import HorizontalBarGraph from '$lib/components/graph/HorizontalBarGraph.svelte';
	import { DEFAULT_MCP_CATALOG_ID } from '$lib/constants';
	import { formatNumber } from '$lib/format';
	import Loading from '$lib/icons/Loading.svelte';
	import { stripMarkdownToText } from '$lib/markdown';
	import {
		AdminService,
		type DeviceClientStat,
		type DeviceMCPServerStat,
		type DeviceScanStats,
		type DeviceSkillStat,
		type MCPCatalogEntry,
		type MCPCatalogServer,
		type OrgUser,
		type TotalTokenUsage
	} from '$lib/services';
	import { errors, mcpServersAndEntries, profile, version } from '$lib/stores';
	import { ENTRY_TYPE_GRAPH_META, entryTypeDonutLegend } from './constants';
	import type { AvgToolCallResponseTimeRow, TopServerUsageRow, TopToolCallRow } from './types';
	import {
		avgToolCallResponseTimeFromStats,
		catalogServerEntryKind,
		deploymentStatusGridColClass,
		deploymentStatusGridShowBorderRight,
		deploymentStatusSortKey,
		mixHex,
		normalizeServerDeploymentStatus,
		topServersFromStats,
		topToolCallsFromStats
	} from './utils';
	import { isWithinInterval, subMonths } from 'date-fns';
	import {
		Activity,
		ChevronRight,
		Coins,
		Laptop,
		MonitorCheck,
		PencilRuler,
		Server,
		Siren,
		Users,
		Wrench
	} from 'lucide-svelte';
	import { onMount } from 'svelte';
	import { fade, fly } from 'svelte/transition';
	import { twMerge } from 'tailwind-merge';

	let { data } = $props();
	let hasDeviceScans = $derived(data?.hasDeviceScans ?? false);
	let loading = $state(true);
	let loadingToolUsage = $state(true);
	let loadingDeviceScanStats = $state(true);

	let usersData = $state<OrgUser[]>([]);
	let totalTokensData = $state<TotalTokenUsage>();

	const doesSupportK8sUpdates = $derived(version.current.engine === 'kubernetes');

	let topToolCalls = $state<TopToolCallRow[]>([]);
	let topServerUsage = $state<TopServerUsageRow[]>([]);
	let avgToolCallResponseTime = $state<AvgToolCallResponseTimeRow[]>([]);
	let deviceScanStats = $state<DeviceScanStats | null>(null);
	let maxToolsToShow = $derived(hasDeviceScans ? 3 : 5);
	let maxServersToShow = $derived(hasDeviceScans ? 10 : 12);

	const end = new Date();
	const start = subMonths(end, 1);

	let monthlyActiveUsers = $derived(
		usersData.filter(
			(user) => user.lastActiveDay && isWithinInterval(new Date(user.lastActiveDay), { start, end })
		).length
	);

	let deployedCatalogEntryServers = $state<MCPCatalogServer[]>([]);
	let deployedWorkspaceCatalogEntryServers = $state<MCPCatalogServer[]>([]);
	let serversData = $derived(
		mcpServersAndEntries.current.loading || loading
			? []
			: [
					...deployedCatalogEntryServers.filter((server) => !server.deleted),
					...deployedWorkspaceCatalogEntryServers.filter((server) => !server.deleted),
					...mcpServersAndEntries.current.servers.filter((server) => !server.deleted)
				]
	);

	function compileServerAndEntries(data: MCPCatalogServer[], entries: MCPCatalogEntry[]) {
		const entriesMap = new Map(entries.map((e) => [e.id, e]));
		const catalogEntriesCount = data.reduce<
			Record<
				string,
				{
					entry?: MCPCatalogEntry | undefined;
					server?: MCPCatalogServer | undefined;
					count: number;
					id: string;
				}
			>
		>((acc, server) => {
			if (!server.catalogEntryID) {
				acc[server.id] = {
					server,
					count: 1,
					id: server.id
				};
				return acc;
			}
			if (!acc[server.catalogEntryID]) {
				const entry = entriesMap.get(server.catalogEntryID);
				if (!entry) return acc;
				acc[server.catalogEntryID] = {
					entry,
					count: 0,
					id: entry.id
				};
			}
			acc[server.catalogEntryID].count++;
			return acc;
		}, {});
		const sortByCountDescending = Object.values(catalogEntriesCount).sort(
			(a, b) => b.count - a.count
		);

		const entryTypes = data.reduce(
			(acc, server) => {
				if (!server.catalogEntryID) acc.multi++;
				else if (server.manifest.runtime === 'composite') acc.composite++;
				else if (server.manifest.runtime === 'remote') acc.remote++;
				else acc.single++;
				return acc;
			},
			{
				multi: 0,
				single: 0,
				remote: 0,
				composite: 0
			}
		);

		let graphData: DonutDatum[] = [];
		let deploymentStatusBreakdown: { status: string; count: number }[] = [];
		if (doesSupportK8sUpdates) {
			const overallByStatus: Record<string, number> = {};
			for (const server of data) {
				const s = normalizeServerDeploymentStatus(server);
				overallByStatus[s] = (overallByStatus[s] ?? 0) + 1;
			}
			deploymentStatusBreakdown = Object.entries(overallByStatus)
				.filter(([, count]) => count > 0)
				.sort(([a], [b]) => {
					const d = deploymentStatusSortKey(a) - deploymentStatusSortKey(b);
					return d !== 0 ? d : a.localeCompare(b);
				})
				.map(([status, count]) => ({ status, count }));

			const countsByKindAndStatus: Record<string, number> = {};
			for (const server of data) {
				const kind = catalogServerEntryKind(server);
				const status = normalizeServerDeploymentStatus(server);
				const key = `${kind}\0${status}`;
				countsByKindAndStatus[key] = (countsByKindAndStatus[key] ?? 0) + 1;
			}

			for (const { key: kind, label: typeLabel, baseColor } of ENTRY_TYPE_GRAPH_META) {
				const prefix = `${kind}\0`;
				const statusEntries = Object.entries(countsByKindAndStatus)
					.filter(([k]) => k.startsWith(prefix))
					.map(([k, value]) => [k.slice(prefix.length), value] as [string, number])
					.filter(([, value]) => value > 0)
					.sort(([a], [b]) => {
						const d = deploymentStatusSortKey(a) - deploymentStatusSortKey(b);
						return d !== 0 ? d : a.localeCompare(b);
					});

				const n = statusEntries.length;
				const maxTint = 0.25;
				statusEntries.forEach(([status, value], i) => {
					const t = n <= 1 ? 0 : i / (n - 1);
					graphData.push({
						label: `${typeLabel} · ${status}`,
						value,
						color: mixHex(baseColor, '#ffffff', t * maxTint),
						groupKey: kind
					});
				});
			}
		} else {
			graphData = ENTRY_TYPE_GRAPH_META.map(({ key, label, baseColor }) => ({
				label,
				value: entryTypes[key],
				color: baseColor
			}));
		}

		return {
			graphData,
			popularServers: sortByCountDescending.filter((s) => s.count > 0).slice(0, 5),
			totalServers: data.length,
			deploymentStatusBreakdown
		};
	}

	const serverAndEntries = $derived(mcpServersAndEntries.current);
	const { graphData, popularServers, totalServers, deploymentStatusBreakdown } = $derived(
		compileServerAndEntries(serversData, serverAndEntries.entries)
	);

	let isBootStrapUser = $derived(profile.current.isBootstrapUser?.() ?? false);

	onMount(async () => {
		AdminService.listAuditLogUsageStats({
			start_time: start.toISOString(),
			end_time: end.toISOString()
		})
			.then((stats) => {
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
				topToolCalls = topToolCallsFromStats(adjustedStats);
				topServerUsage = topServersFromStats(adjustedStats);
				avgToolCallResponseTime = avgToolCallResponseTimeFromStats(adjustedStats);
			})
			.catch((error) => {
				if (error?.name === 'AbortError') return;
				errors.append(error);
			})
			.finally(() => {
				loadingToolUsage = false;
			});

		AdminService.getDeviceScanStats({ start: start.toISOString(), end: end.toISOString() })
			.then((stats) => {
				deviceScanStats = stats;
			})
			.catch((error) => {
				if (error?.name === 'AbortError') return;
				errors.append(error);
			})
			.finally(() => {
				loadingDeviceScanStats = false;
			});

		const [users, tokens, catalogServers, workspaceServers] = await Promise.all([
			AdminService.listUsersIncludeDeleted(),
			AdminService.listTotalTokenUsage(),
			AdminService.listAllCatalogDeployedSingleRemoteServers(DEFAULT_MCP_CATALOG_ID),
			AdminService.listAllWorkspaceDeployedSingleRemoteServers()
		]);

		usersData = users;
		totalTokensData = tokens;
		deployedCatalogEntryServers = catalogServers;
		deployedWorkspaceCatalogEntryServers = workspaceServers;
		loading = false;
	});

	function getServerUrl(server: MCPCatalogServer) {
		if (server.powerUserWorkspaceID) {
			return `/admin/mcp-servers/w/${server.powerUserWorkspaceID}/s/${server.id}?view=server-instances`;
		}
		return `/admin/mcp-servers/s/${server.id}?view=server-instances`;
	}

	function getEntryUrl(entry: MCPCatalogEntry) {
		if (entry.powerUserWorkspaceID) {
			return `/admin/mcp-servers/w/${entry.powerUserWorkspaceID}/c/${entry.id}?view=server-instances`;
		}
		return `/admin/mcp-servers/c/${entry.id}?view=server-instances`;
	}

	const platformStatTiles = $derived([
		{
			id: 'total-users',
			label: 'Total Users',
			loading,
			value: usersData.length,
			icon: Users,
			seeMore: '/admin/users'
		},
		{
			id: 'monthly-active-users',
			label: 'Monthly Active Users',
			loading,
			value: monthlyActiveUsers,
			icon: Activity,
			seeMore: '/admin/users'
		},
		{
			id: 'total-tokens',
			label: 'Total Tokens',
			loading,
			value: totalTokensData?.totalTokens ?? 0,
			icon: Coins,
			seeMore: '/admin/token-usage'
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
			(m) => m.configHash,
			(m) => m.name?.trim() || '(unnamed)',
			(m) => m.deviceCount,
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

	let deviceScanTiles = $derived([
		{
			id: 'device-overview',
			label: 'Unique Devices',
			loading: loadingDeviceScanStats,
			value: deviceScanStats?.deviceCount ?? 0,
			icon: Laptop,
			seeMore: '/admin/devices'
		},
		{
			id: 'device-users',
			label: 'Unique Users',
			loading: loadingDeviceScanStats,
			value: deviceScanStats?.userCount ?? 0,
			icon: Users
		},
		{
			id: 'device-clients',
			label: 'Unique Clients',
			loading: loadingDeviceScanStats,
			value: deviceScanStats?.clients?.length ?? 0,
			icon: MonitorCheck,
			seeMore: '/admin/device-clients'
		},
		{
			id: 'device-mcps',
			label: 'Unique MCPs',
			loading: loadingDeviceScanStats,
			value: deviceScanStats?.mcpServers?.length ?? 0,
			icon: Server,
			seeMore: '/admin/device-mcp-servers'
		},
		{
			id: 'device-skills',
			label: 'Unique Skills',
			loading: loadingDeviceScanStats,
			value: deviceScanStats?.skills?.length ?? 0,
			icon: PencilRuler,
			seeMore: '/admin/device-skills'
		}
	]);
</script>

<Layout title="Dashboard" classes={{ childrenContainer: 'max-w-none', container: '' }}>
	<div class="@container grid min-w-0 w-full max-w-full grid-cols-12 gap-4">
		<div class="col-span-12 grid grid-cols-12 gap-4">
			<div
				class={twMerge(
					'paper flex min-w-0 flex-col gap-0 p-0',
					hasDeviceScans ? ' col-span-12 @3xl:col-span-5' : 'col-span-12'
				)}
			>
				{#if hasDeviceScans}
					<div class="shrink-0 border-b border-surface2 px-4 py-2">
						<h4 class="flex items-center font-light text-xs uppercase">On Platform</h4>
					</div>
				{/if}
				<div class="@container min-w-0 w-full max-w-full">
					<div class="grid w-full grid-cols-2 gap-0 @md:grid-cols-12">
						{#each platformStatTiles as platformStat (platformStat.id)}
							{@render platformStatCell(platformStat)}
						{/each}
					</div>
				</div>
			</div>
			{#if hasDeviceScans}
				<div
					class="gap-0 paper min-w-0 p-0 col-span-12 @3xl:col-span-7"
					in:fly={{ x: 100, duration: 150 }}
				>
					<div class="col-span-12 border-b border-surface2 px-4 py-2">
						<h4 class="flex items-center font-light text-xs uppercase">Device Scans</h4>
					</div>
					<div class="@container min-w-0 w-full max-w-full">
						<div class="grid grid-cols-2 gap-0 @md:flex @md:items-center">
							{#each deviceScanTiles as deviceScanStat (deviceScanStat.id)}
								{@render deviceScanStatCell(deviceScanStat)}
							{/each}
						</div>
					</div>
				</div>
			{/if}
		</div>

		<div
			class={twMerge(
				'flex flex-col gap-4 col-span-12',
				hasDeviceScans ? '@3xl:col-span-5' : '@3xl:col-span-8'
			)}
		>
			{#if hasDeviceScans}
				{@render serverActivityGraph()}
				{@render topServerDeploymentList()}
			{/if}

			{#if loadingToolUsage}
				<div
					class={twMerge(
						'bg-surface3 animate-pulse rounded-md',
						hasDeviceScans ? 'h-81' : 'h-[400px]'
					)}
				></div>
			{:else}
				<div in:fade={{ duration: 150 }} class="paper gap-1 w-full min-h-72">
					<div class="flex flex-wrap items-center justify-between gap-4">
						<h4 class="flex items-center gap-1 font-semibold">
							Top Servers Used <span class="text-on-surface1 text-xs font-light"
								>(Last 30 Days)</span
							>
						</h4>
					</div>
					<HorizontalBarGraph
						data={topServerUsage.slice(0, maxServersToShow)}
						labelKey="serverName"
						valueKey="count"
						formatValue={(value) => Math.round(value).toString()}
						class={hasDeviceScans ? 'h-67.5' : 'h-[400px]'}
					>
						{#snippet tooltipContent(item)}
							<div class="flex flex-col gap-0 text-xs">
								<div class="text-on-surface1 text-xs">{item.label}</div>
							</div>
							<div class="text-on-background font-semibold">
								{item.value} calls
							</div>
						{/snippet}
					</HorizontalBarGraph>
				</div>
			{/if}

			{#if !hasDeviceScans}
				<div class={twMerge('grid grid-cols-12 gap-4 grow')}>
					{@render popularTools()}
					{@render toolAverageResponseTime()}
				</div>
			{/if}
		</div>
		{#if hasDeviceScans}
			<div
				class="col-span-12 @3xl:col-span-7 flex flex-col gap-4"
				in:fly={{ x: 100, duration: 150 }}
			>
				{#if loadingDeviceScanStats}
					<div class="grid grid-cols-1 gap-4 lg:grid-cols-2">
						{#each Array.from({ length: 4 }) as _, i (i)}
							<div class="paper min-h-72 animate-pulse bg-surface3/40"></div>
						{/each}
					</div>
				{:else}
					<DeviceScanDonutCard
						title="Top Device Skills"
						buckets={deviceScanSkillBuckets}
						totalGroups={totalDeviceScanSkillGroups}
						emptyMsg="No skills observed yet."
						class="h-fit"
						classes={{ graphContainer: '@md:w-1/2', graph: 'h-56 w-full' }}
					/>
					<div class="grid grid-cols-1 gap-4 lg:grid-cols-2">
						<DeviceScanDonutCard
							legendOnBottom
							title="Device Clients"
							buckets={deviceScanClientBuckets}
							totalGroups={totalDeviceScanClientGroups}
							emptyMsg="No clients observed yet."
						/>
						<DeviceScanDonutCard
							legendOnBottom
							title="Top Device MCP Servers"
							buckets={deviceScanMcpBuckets}
							totalGroups={totalDeviceScanMcpGroups}
							emptyMsg="No MCP servers observed yet."
						/>
					</div>
					<div class="h-80">
						<DeviceScanTimelineCard
							rangeStart={start}
							rangeEnd={end}
							timelineRows={deviceScanTimelineRows}
							totalSubmissions={totalDeviceScanSubmissions}
						/>
					</div>
				{/if}
			</div>
		{:else}
			<div
				class="col-span-12 @3xl:col-span-4 flex flex-col gap-4"
				in:fly={{ x: 100, duration: 150 }}
			>
				{@render serverActivityGraph()}
				{@render topServerDeploymentList()}
			</div>
		{/if}
		{#if hasDeviceScans}
			<div class="col-span-12 grid grid-cols-12 gap-4">
				{@render popularTools()}
				{@render toolAverageResponseTime()}
			</div>
		{/if}
	</div>
</Layout>

{#snippet serverActivityGraph()}
	{#if serverAndEntries.loading || loading}
		<div class={twMerge('bg-surface3 animate-pulse rounded-md', 'h-[530px]')}></div>
		<div class="paper gap-1 flex grow">
			<h4 class="flex items-center gap-2 font-semibold">Most Popular Servers</h4>
			<div class="pt-2 flex flex-col gap-4">
				{#each Array.from({ length: 5 }) as _, i (i)}
					<div class="flex gap-2 items-center animate-pulse w-full">
						<div class="size-8 rounded-md bg-surface3 shrink-0"></div>
						<div class="flex flex-col gap-2 flex-1">
							<div class="h-4 w-full rounded bg-surface3"></div>
							<div class="h-3 w-full rounded bg-surface3"></div>
						</div>
					</div>
				{/each}
			</div>
		</div>
	{:else}
		<div
			in:fade={{ duration: 150 }}
			class={twMerge('paper', hasDeviceScans ? 'min-h-64' : 'min-h-96')}
		>
			<h4 class="font-semibold">Server Activity</h4>
			{#if doesSupportK8sUpdates && deploymentStatusBreakdown.length > 0}
				<div class="mb-2 grid grid-cols-12 gap-x-2 gap-y-5">
					{#each deploymentStatusBreakdown as row, i (row.status)}
						<div
							class={twMerge(
								'flex flex-col items-center justify-center px-1 text-center',
								deploymentStatusGridColClass(i, deploymentStatusBreakdown.length),
								deploymentStatusGridShowBorderRight(i, deploymentStatusBreakdown.length) &&
									'border-r border-surface2'
							)}
						>
							<div class="flex items-center gap-1">
								<div class={twMerge('font-semibold', hasDeviceScans ? 'text-xl' : 'text-3xl')}>
									<TweenedMetric target={row.count} />
								</div>
								{#if row.status === 'Available'}
									<Server class="size-6 text-primary" />
								{:else if row.status === 'Needs Attention'}
									<Siren class="size-6 text-yellow-500" />
								{:else}
									<Server class="size-6 text-on-surface1/75" />
								{/if}
							</div>
							<div class="text-xs">{row.status}</div>
						</div>
					{/each}
				</div>
			{:else}
				<div class="mb-2 flex flex-col justify-center items-center">
					<div class="flex w-full gap-2 items-center justify-center">
						<div class={twMerge('font-semibold', hasDeviceScans ? 'text-xl' : 'text-3xl')}>
							<TweenedMetric target={totalServers} />
						</div>
						<Server class="size-6 text-primary" />
					</div>
					<div class="text-xs">Total Currently Active</div>
				</div>
			{/if}

			<div
				class={twMerge(
					'flex flex-col items-center justify-center',
					hasDeviceScans ? 'h-64' : 'h-80'
				)}
			>
				{#if graphData.some((g) => g.value > 0)}
					<DonutGraph
						class={twMerge('h-80', hasDeviceScans ? 'h-64' : '')}
						donutRatio={0.65}
						data={graphData}
						legend={doesSupportK8sUpdates ? entryTypeDonutLegend : undefined}
					/>
				{:else}
					<p class="font-light text-xs text-on-surface1 pt-2 text-center">
						No servers have been deployed yet.
					</p>
				{/if}
			</div>

			{#if !isBootStrapUser && totalServers > 0}
				<div class="flex justify-end">
					<a
						href={resolve('/admin/mcp-servers?view=deployments')}
						class="text-[11px] transition-colors self-end translate-x-2 duration-200 bg-surface3/50 hover:bg-surface3 rounded-md py-0.5 w-fit px-2 flex items-center gap-1"
					>
						See More <ChevronRight class="size-3" />
					</a>
				</div>
			{/if}
		</div>
	{/if}
{/snippet}

{#snippet topServerDeploymentList()}
	<div in:fade={{ duration: 150 }} class="paper gap-1 flex grow">
		<h4 class="flex items-center gap-2 font-semibold">Most Deployed Servers</h4>
		{#if mcpServersAndEntries.current.loading || loading}
			<div class="pt-2 flex flex-col gap-4">
				{#each Array.from({ length: 5 }) as _, i (i)}
					<div class="flex gap-2 items-center animate-pulse w-full">
						<div class="size-8 rounded-md bg-surface3 shrink-0"></div>
						<div class="flex flex-col gap-2 flex-1">
							<div class="h-4 w-full rounded bg-surface3"></div>
							<div class="h-3 w-full rounded bg-surface3"></div>
						</div>
					</div>
				{/each}
			</div>
		{:else if popularServers.length > 0}
			<div class="pt-2 flex flex-col gap-2">
				{#each popularServers as info (info.id)}
					{@const icon = 'server' in info ? info.server?.manifest.icon : info.entry?.manifest.icon}
					{@const displayName =
						'server' in info
							? (info.server?.alias ?? info.server?.manifest.name)
							: info.entry?.manifest.name}
					{@const description =
						'server' in info ? info.server?.manifest.description : info.entry?.manifest.description}
					{@const url = info.server
						? getServerUrl(info.server)
						: info.entry
							? getEntryUrl(info.entry)
							: undefined}
					<a
						class="flex gap-2 items-center dark:hover:bg-surface2 hover:bg-surface1 transition-colors duration-150 -mx-2 px-2 py-1 rounded-md"
						href={url ? resolve(url as `/${string}`) : undefined}
					>
						{#if icon}
							<img
								src={icon}
								alt={info.id}
								class="size-9 bg-surface1 dark:bg-surface2 rounded-md p-1"
							/>
						{:else}
							<Server class="size-9 opacity-65 bg-surface1 rounded-md p-1" />
						{/if}
						<div class="flex flex-col gap-0.5 max-w-[calc(100%-4.5rem)]">
							<p class="text-sm font-medium">{displayName}</p>
							{#if description}
								<p class="text-xs truncate line-clamp-1 break-all font-light">
									{stripMarkdownToText(description ?? '')}
								</p>
							{/if}
							<p class="text-xs text-on-surface1 italic">Deployed {info.count} times</p>
						</div>
						<ChevronRight class="size-5 shrink-0" />
					</a>
				{/each}
			</div>
		{:else}
			<p
				class="text-xs text-on-surface1 pt-2 font-light text-center h-full flex items-center justify-center"
			>
				No servers have been deployed yet.
			</p>
		{/if}
		<div class="flex grow"></div>
		{#if popularServers.length > 0 && !isBootStrapUser}
			<a
				href={resolve('/admin/mcp-servers')}
				class="justify-end self-end text-[11px] translate-x-2 transition-colors duration-200 bg-surface3/50 hover:bg-surface3 rounded-md py-0.5 w-fit px-2 flex items-center gap-1"
			>
				See More <ChevronRight class="size-3" />
			</a>
		{/if}
	</div>
{/snippet}

{#snippet platformStatCell(platformStat: (typeof platformStatTiles)[number])}
	{@const defaultClasses = 'col-span-4 p-2 flex gap-4 items-center justify-between w-full'}
	<div
		class="col-span-1 min-w-0 border-r-0 px-2 my-2 flex [&:last-child:nth-child(odd)]:col-span-2 @md:col-span-4 @md:[&:last-child:nth-child(odd)]:col-span-4 @md:not-last:border-r @md:not-last:border-surface2"
	>
		{#if platformStat.seeMore && !isBootStrapUser}
			<a
				class={twMerge(
					defaultClasses,
					'group w-full hover:bg-surface2/50 transition-colors duration-200 rounded-md'
				)}
				href={resolve(platformStat.seeMore as `/${string}`)}
			>
				{@render statContent(platformStat)}
			</a>
		{:else}
			<div class={defaultClasses}>
				{@render statContent(platformStat)}
			</div>
		{/if}
	</div>
{/snippet}

{#snippet deviceScanStatCell(deviceScanStat: (typeof deviceScanTiles)[number])}
	{@const defaultClasses = 'p-2 flex gap-4 items-center justify-between w-full'}
	<div
		class="col-span-1 min-w-0 flex border-r-0 px-2 my-2 [&:last-child:nth-child(odd)]:col-span-2 @md:flex-1 @md:col-span-auto @md:[&:last-child:nth-child(odd)]:col-span-auto @md:not-last:border-r @md:not-last:border-surface2"
	>
		{#if deviceScanStat.seeMore}
			<a
				href={resolve(deviceScanStat.seeMore as `/${string}`)}
				class={twMerge(
					defaultClasses,
					'group w-full hover:bg-surface2/50 transition-colors duration-200 rounded-md'
				)}
			>
				{@render statContent(deviceScanStat)}
			</a>
		{:else}
			<div class={defaultClasses}>
				{@render statContent(deviceScanStat)}
			</div>
		{/if}
	</div>
{/snippet}

{#snippet statContent(platformStat: (typeof platformStatTiles | typeof deviceScanTiles)[number])}
	<div class="w-full">
		<div class="text-xs text-on-surface1 flex items-center gap-1 shrink-0 mb-0.5">
			{platformStat.label}
		</div>

		<div class="flex items-center gap-1 justify-between">
			{#if platformStat.loading}
				<Loading class="size-6" />
			{:else}
				<div class="text-xl font-semibold">
					<TweenedMetric holdAtZero={platformStat.loading} target={platformStat.value} />
				</div>
			{/if}
			<div class="relative size-4 shrink-0">
				<platformStat.icon
					class="size-4 text-primary transition-opacity duration-200 group-hover:opacity-0"
				/>
				<ChevronRight
					class="pointer-events-none text-on-surface1 absolute inset-0 size-4 opacity-0 transition-opacity duration-200 group-hover:opacity-100"
				/>
			</div>
		</div>
	</div>
{/snippet}

{#snippet popularTools()}
	<div
		class={twMerge(
			'paper gap-1 col-span-12 flex flex-col @3xl:col-span-6 ',
			!hasDeviceScans && 'h-full min-h-72'
		)}
	>
		<h4 class="flex items-center gap-2 font-semibold mb-1">
			Recently Popular Tools
			<span class="text-on-surface1 text-xs font-light">(Last 30 Days)</span>
		</h4>
		{#if loadingToolUsage}
			<div class="pt-2 flex flex-col gap-4 w-full">
				{#each Array.from({ length: maxToolsToShow }) as _, i (i)}
					<div class="flex gap-2 items-center animate-pulse w-full">
						<div class="size-8 rounded-md bg-surface3 shrink-0"></div>
						<div class="flex flex-col gap-2 flex-1">
							<div class="h-4 w-full rounded bg-surface3"></div>
							<div class="h-3 w-full rounded bg-surface3"></div>
						</div>
					</div>
				{/each}
			</div>
		{:else if topToolCalls.length === 0}
			<p
				class="text-xs text-on-surface1 pt-2 font-light grow flex items-center justify-center h-full text-center"
			>
				No recent tool calls.
			</p>
		{:else}
			<ul class="pt-2 flex flex-col gap-2">
				{#each topToolCalls.slice(0, maxToolsToShow) as row (row.compositeKey)}
					<li class="flex gap-2 items-center">
						<div
							class="size-8 items-center justify-center shrink-0 bg-surface1 dark:bg-surface2 rounded-md p-1"
						>
							<Wrench class="size-6 opacity-65 shrink-0" />
						</div>
						<div class="flex flex-col gap-1 min-w-0">
							<p class="text-sm font-medium truncate">
								{row.toolLabel.split('.').slice(1).join('.') || row.compositeKey}
							</p>
							<p class="text-xs text-on-surface1">
								{formatNumber(row.count)} calls · {row.serverDisplayName}
							</p>
						</div>
					</li>
				{/each}
			</ul>
		{/if}
		{#if !hasDeviceScans}
			<div class="flex grow min-h-0"></div>
		{/if}
		{#if topToolCalls.length > 0 && !isBootStrapUser}
			<a
				href={resolve('/admin/usage')}
				class="text-[11px] translate-x-2 self-end bg-surface3/50 transition-colors duration-200 hover:bg-surface3 rounded-md py-0.5 w-fit px-2 flex items-center gap-1 mt-2"
			>
				See More <ChevronRight class="size-3" />
			</a>
		{/if}
	</div>
{/snippet}

{#snippet toolAverageResponseTime()}
	<div
		class={twMerge(
			'paper gap-1 col-span-12 flex flex-col @3xl:col-span-6',
			!hasDeviceScans && 'h-full min-h-72'
		)}
	>
		<h4 class="flex items-center gap-2 font-semibold mb-1">
			Tool Call Average Response Time
			<span class="text-on-surface1 text-xs font-light">(Last 30 Days)</span>
		</h4>
		{#if loadingToolUsage}
			<div class="pt-2 flex flex-col gap-4 w-full">
				{#each Array.from({ length: maxToolsToShow }) as _, i (i)}
					<div class="flex gap-2 items-center animate-pulse w-full">
						<div class="flex flex-col gap-2 flex-1">
							<div class="h-4 w-full rounded bg-surface3"></div>
							<div class="h-3 w-full rounded bg-surface3"></div>
						</div>
					</div>
				{/each}
			</div>
		{:else if avgToolCallResponseTime.length === 0}
			<p
				class="text-xs text-on-surface1 pt-2 font-light grow flex items-center justify-center h-full text-center"
			>
				No recent tool calls.
			</p>
		{:else}
			<div class="pt-2 flex flex-col gap-4 w-full">
				<ul class="flex flex-col gap-2">
					{#each avgToolCallResponseTime.slice(0, maxToolsToShow) as row (row.toolName)}
						<li class="flex gap-2 items-center">
							<div class="flex flex-col gap-1 min-w-0 grow pr-4">
								<p class="text-sm font-medium truncate">
									{row.toolName.split('.').slice(1).join('.')}
								</p>
								<p class="text-xs text-on-surface1">
									{row.serverDisplayName}
								</p>
							</div>
							<div class="text-sm">
								{row.averageResponseTimeMs.toFixed(2)}ms
							</div>
						</li>
					{/each}
				</ul>
			</div>
		{/if}
		{#if !hasDeviceScans}
			<div class="flex grow min-h-0"></div>
		{/if}
		{#if avgToolCallResponseTime.length > 0 && !isBootStrapUser}
			<a
				href={resolve('/admin/usage')}
				class="text-[11px] translate-x-2 self-end bg-surface3/50 transition-colors duration-200 hover:bg-surface3 rounded-md py-0.5 w-fit px-2 flex items-center gap-1 mt-2"
			>
				See More <ChevronRight class="size-3" />
			</a>
		{/if}
	</div>
{/snippet}

<svelte:head>
	<title>Obot | Dashboard</title>
</svelte:head>
