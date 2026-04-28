<script lang="ts">
	import { resolve } from '$app/paths';
	import Layout from '$lib/components/Layout.svelte';
	import TweenedMetric from '$lib/components/TweenedMetric.svelte';
	import McpServerGitSync from '$lib/components/admin/McpServerGitSync.svelte';
	import {
		transformTopToolCalls,
		transformTopServerUsage,
		transformAvgToolCallResponseTime
	} from '$lib/components/admin/usage/utils';
	import DonutGraph, {
		type DonutDatum,
		type DonutLegendItem
	} from '$lib/components/graph/DonutGraph.svelte';
	import HorizontalBarGraph from '$lib/components/graph/HorizontalBarGraph.svelte';
	import SelectServerType from '$lib/components/mcp/SelectServerType.svelte';
	import { DEFAULT_MCP_CATALOG_ID } from '$lib/constants';
	import { formatNumber } from '$lib/format';
	import Loading from '$lib/icons/Loading.svelte';
	import { stripMarkdownToText } from '$lib/markdown';
	import {
		AdminService,
		type AuditLogUsageStats,
		type LaunchServerType,
		type MCPCatalogEntry,
		type MCPCatalogServer,
		type OrgUser,
		type TotalTokenUsage
	} from '$lib/services';
	import { errors, mcpServersAndEntries, profile, version } from '$lib/stores';
	import { goto } from '$lib/url';
	import { isWithinInterval, set, subMonths } from 'date-fns';
	import { Activity, ChevronRight, Coins, Server, Siren, Users, Wrench } from 'lucide-svelte';
	import { onMount } from 'svelte';
	import { fade } from 'svelte/transition';
	import { twMerge } from 'tailwind-merge';

	const TOP_TOOLS_LIMIT = 5;
	const TOP_SERVERS_LIMIT = 12;

	const DEPLOYMENT_STATUS_ORDER = [
		'Available',
		'Progressing',
		'Unavailable',
		'Needs Attention',
		'Shutdown',
		'Unknown'
	] as const;

	const ENTRY_TYPE_GRAPH_META: {
		key: 'multi' | 'single' | 'remote' | 'composite';
		label: string;
		baseColor: string;
	}[] = [
		{ key: 'multi', label: 'Multi-User', baseColor: '#fee090' },
		{ key: 'single', label: 'Single-User', baseColor: '#f46d43' },
		{ key: 'remote', label: 'Remote', baseColor: '#4575b4' },
		{ key: 'composite', label: 'Composite', baseColor: '#fdae61' }
	];

	const entryTypeDonutLegend: DonutLegendItem[] = ENTRY_TYPE_GRAPH_META.map(
		({ label, baseColor }) => ({ label, color: baseColor })
	);

	function mixHex(base: string, toward: string, t: number): string {
		const parse = (hex: string) => {
			const s = hex.replace('#', '');
			const full = s.length === 3 ? [...s].map((c) => c + c).join('') : s;
			return [0, 2, 4].map((i) => parseInt(full.slice(i, i + 2), 16));
		};
		const [r1, g1, b1] = parse(base);
		const [r2, g2, b2] = parse(toward);
		const blend = (a: number, b: number) => Math.round(a + (b - a) * t);
		const r = blend(r1, r2);
		const g = blend(g1, g2);
		const b = blend(b1, b2);
		return `#${[r, g, b].map((x) => x.toString(16).padStart(2, '0')).join('')}`;
	}

	function deploymentStatusSortKey(status: string): number {
		const i = DEPLOYMENT_STATUS_ORDER.indexOf(status as (typeof DEPLOYMENT_STATUS_ORDER)[number]);
		return i >= 0 ? i : DEPLOYMENT_STATUS_ORDER.length;
	}

	function catalogServerEntryKind(
		server: MCPCatalogServer
	): 'multi' | 'single' | 'remote' | 'composite' {
		if (!server.catalogEntryID) return 'multi';
		if (server.manifest.runtime === 'composite') return 'composite';
		if (server.manifest.runtime === 'remote') return 'remote';
		return 'single';
	}

	function normalizeServerDeploymentStatus(server: MCPCatalogServer): string {
		const raw = server.deploymentStatus?.trim();
		if (raw && DEPLOYMENT_STATUS_ORDER.includes(raw as (typeof DEPLOYMENT_STATUS_ORDER)[number]))
			return raw;
		if (raw) return raw;
		return 'Unknown';
	}

	/** 12-column grid: 3× col-span-4 per full row; last row fills width (6+6 or 12). */
	function deploymentStatusRowLayout(total: number): {
		itemsInLastRow: number;
		lastRowStart: number;
	} {
		const rem = total % 3;
		const itemsInLastRow = rem === 0 ? 3 : rem;
		const lastRowStart = total - itemsInLastRow;
		return { itemsInLastRow, lastRowStart };
	}

	function deploymentStatusGridColClass(i: number, total: number): string {
		const { itemsInLastRow, lastRowStart } = deploymentStatusRowLayout(total);
		if (i < lastRowStart) return 'col-span-4';
		if (itemsInLastRow === 1) return 'col-span-12';
		if (itemsInLastRow === 2) return 'col-span-6';
		return 'col-span-4';
	}

	function deploymentStatusGridShowBorderRight(i: number, total: number): boolean {
		const { itemsInLastRow, lastRowStart } = deploymentStatusRowLayout(total);
		if (i >= lastRowStart) {
			if (itemsInLastRow === 1) return false;
			if (itemsInLastRow === 2) return i === lastRowStart;
			return i < lastRowStart + 2;
		}
		return i % 3 !== 2;
	}

	let loading = $state(true);
	let loadingToolUsage = $state(true);

	let usersData = $state<OrgUser[]>([]);
	let totalTokensData = $state<TotalTokenUsage>();

	let selectServerTypeDialog = $state<ReturnType<typeof SelectServerType>>();
	let sourceDialog = $state<ReturnType<typeof McpServerGitSync>>();

	const doesSupportK8sUpdates = $derived(version.current.engine === 'kubernetes');

	type TopToolCallRow = {
		compositeKey: string;
		toolLabel: string;
		count: number;
		serverDisplayName: string;
	};

	type TopServerUsageRow = { serverName: string; count: number };

	type AvgToolCallResponseTimeRow = {
		toolName: string;
		averageResponseTimeMs: number;
		serverDisplayName: string;
	};

	let topToolCalls = $state<TopToolCallRow[]>([]);
	let topServerUsage = $state<TopServerUsageRow[]>([]);
	let avgToolCallResponseTime = $state<AvgToolCallResponseTimeRow[]>([]);

	const end = new Date();
	const start = subMonths(end, 1);

	function topToolCallsFromStats(stats: AuditLogUsageStats | undefined): TopToolCallRow[] {
		return transformTopToolCalls(stats).map((t) => ({
			compositeKey: t.toolName,
			toolLabel: t.toolName,
			count: t.count,
			serverDisplayName: t.serverDisplayName
		}));
	}

	function topServersFromStats(stats: AuditLogUsageStats | undefined): TopServerUsageRow[] {
		return transformTopServerUsage(stats);
	}

	function avgToolCallResponseTimeFromStats(stats: AuditLogUsageStats | undefined) {
		return transformAvgToolCallResponseTime(stats);
	}

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
						color: mixHex(baseColor, '#ffffff', t * maxTint)
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
		const endToolStats = set(new Date(), { milliseconds: 0, seconds: 59 });
		const startToolStats = subMonths(endToolStats, 1);

		AdminService.listAuditLogUsageStats({
			start_time: startToolStats.toISOString(),
			end_time: endToolStats.toISOString()
		})
			.then((stats) => {
				const statsToUse = (stats.items ?? []).filter(
					(s) =>
						!s.mcpID.startsWith('sms1') &&
						!s.mcpID.startsWith('nba1') &&
						!s.mcpServerDisplayName.startsWith('Obot ')
				);
				const adjustedStats = {
					...stats,
					items: statsToUse
				};
				topToolCalls = topToolCallsFromStats(adjustedStats).slice(0, TOP_TOOLS_LIMIT);
				topServerUsage = topServersFromStats(adjustedStats).slice(0, TOP_SERVERS_LIMIT);
				avgToolCallResponseTime = avgToolCallResponseTimeFromStats(adjustedStats).slice(
					0,
					TOP_TOOLS_LIMIT
				);
			})
			.catch((error) => {
				if (error?.name === 'AbortError') return;
				errors.append(error);
			})
			.finally(() => {
				loadingToolUsage = false;
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

	function handleSelectServerType(type: LaunchServerType) {
		selectServerTypeDialog?.close();
		goto(resolve(`/admin/mcp-servers?new=${type}`));
	}

	function getServerUrl(server: MCPCatalogServer) {
		if (server.powerUserWorkspaceID) {
			return `/admin/mcp-servers/w/${server.powerUserWorkspaceID}/s/${server.id}`;
		}
		return `/admin/mcp-servers/s/${server.id}`;
	}

	function getEntryUrl(entry: MCPCatalogEntry) {
		if (entry.powerUserWorkspaceID) {
			return `/admin/mcp-servers/w/${entry.powerUserWorkspaceID}/c/${entry.id}`;
		}
		return `/admin/mcp-servers/c/${entry.id}`;
	}
</script>

<Layout title="Dashboard" classes={{ childrenContainer: 'max-w-none', container: '' }}>
	<div class="@container grid grid-cols-12 gap-4">
		<div class="flex flex-col col-span-12 @min-[768px]:col-span-8 gap-4">
			<!-- this token usage graph-->
			<div class="grid grid-cols-12 gap-4">
				<div class="col-span-12 @min-[768px]:col-span-4">
					<div class="paper gap-2 h-full">
						<div class="text-xs text-on-surface1 flex items-center gap-1">Total Users</div>
						<div class="flex w-full justify-between">
							{#if loading}
								<Loading class="size-8" />
							{:else}
								<div class="text-3xl font-semibold">
									<TweenedMetric holdAtZero={loading} target={usersData.length} />
								</div>
								<Users class="size-8 text-primary" />
							{/if}
						</div>
						{#if !isBootStrapUser}
							<a
								href={resolve('/admin/users')}
								class="text-[11px] translate-x-2 self-end bg-surface3/50 transition-colors duration-200 hover:bg-surface3 rounded-md py-0.5 w-fit px-2 flex items-center gap-1"
							>
								See More <ChevronRight class="size-3" />
							</a>
						{/if}
					</div>
				</div>
				<div class="col-span-12 @min-[768px]:col-span-4">
					<div class="paper gap-2 h-full">
						<div class="text-xs text-on-surface1 flex items-center gap-1">Monthly Active Users</div>
						<div class="flex w-full justify-between">
							{#if loading}
								<Loading class="size-8" />
							{:else}
								<div class="text-3xl font-semibold">
									<TweenedMetric holdAtZero={loading} target={monthlyActiveUsers} />
								</div>
								<Activity class="size-8 text-primary" />
							{/if}
						</div>
						<div class="text-xs text-on-surface1">Last 30 Days</div>
					</div>
				</div>
				<div class="col-span-12 @min-[768px]:col-span-4">
					<div class="paper gap-2 h-full">
						<div class="text-xs text-on-surface1 flex items-center gap-1">Total Tokens</div>
						<div class="flex w-full justify-between">
							{#if loading}
								<Loading class="size-8" />
							{:else}
								<div class="text-3xl font-semibold">
									<TweenedMetric
										holdAtZero={loading}
										target={totalTokensData?.totalTokens ?? 0}
										format={(n) => formatNumber(Math.max(0, Math.round(n)))}
									/>
								</div>
								<Coins class="size-8 text-primary" />
							{/if}
						</div>
						{#if !isBootStrapUser}
							<a
								href={resolve('/admin/token-usage')}
								class="text-[11px] translate-x-2 self-end bg-surface3/50 transition-colors duration-200 hover:bg-surface3 rounded-md py-0.5 w-fit px-2 flex items-center gap-1"
							>
								See More <ChevronRight class="size-3" />
							</a>
						{/if}
					</div>
				</div>
			</div>
			{#if loadingToolUsage}
				<div class="bg-surface3 h-[400px] animate-pulse rounded-md"></div>
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
						data={topServerUsage}
						labelKey="serverName"
						valueKey="count"
						formatValue={(value) => Math.round(value).toString()}
						class="h-[400px]"
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

			<div class="grid grid-cols-12 gap-4 grow">
				<div class="paper h-full gap-1 col-span-12 @min-[768px]:col-span-6 flex flex-col min-h-72">
					<h4 class="flex items-center gap-2 font-semibold mb-1">
						Recently Popular Tools
						<span class="text-on-surface1 text-xs font-light">(Last 30 Days)</span>
					</h4>
					{#if loadingToolUsage}
						<div class="pt-2 flex flex-col gap-4 w-full">
							{#each Array.from({ length: TOP_TOOLS_LIMIT }) as _, i (i)}
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
							{#each topToolCalls as row (row.compositeKey)}
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
					<div class="flex grow min-h-0"></div>
					{#if topToolCalls.length > 0 && !isBootStrapUser}
						<a
							href={resolve('/admin/usage')}
							class="text-[11px] translate-x-2 self-end bg-surface3/50 transition-colors duration-200 hover:bg-surface3 rounded-md py-0.5 w-fit px-2 flex items-center gap-1 mt-2"
						>
							See More <ChevronRight class="size-3" />
						</a>
					{/if}
				</div>
				<div class="paper h-full gap-1 col-span-12 @min-[768px]:col-span-6 flex flex-col min-h-72">
					<h4 class="flex items-center gap-2 font-semibold mb-1">
						Tool Call Average Response Time
						<span class="text-on-surface1 text-xs font-light">(Last 30 Days)</span>
					</h4>
					{#if loadingToolUsage}
						<div class="pt-2 flex flex-col gap-4 w-full">
							{#each Array.from({ length: TOP_TOOLS_LIMIT }) as _, i (i)}
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
								{#each avgToolCallResponseTime as row (row.toolName)}
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
											{row.averageResponseTimeMs}ms
										</div>
									</li>
								{/each}
							</ul>
						</div>
					{/if}
					<div class="flex grow min-h-0"></div>
					{#if avgToolCallResponseTime.length > 0 && !isBootStrapUser}
						<a
							href={resolve('/admin/usage')}
							class="text-[11px] translate-x-2 self-end bg-surface3/50 transition-colors duration-200 hover:bg-surface3 rounded-md py-0.5 w-fit px-2 flex items-center gap-1 mt-2"
						>
							See More <ChevronRight class="size-3" />
						</a>
					{/if}
				</div>
			</div>
		</div>
		<div class="col-span-12 @min-[768px]:col-span-4 flex flex-col gap-4">
			{#if serverAndEntries.loading || loading}
				<div class="bg-surface3 h-[530px] animate-pulse rounded-md"></div>
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
				<div in:fade={{ duration: 150 }} class="paper min-h-96">
					<h4 class="font-semibold">Total Servers</h4>
					{#if doesSupportK8sUpdates}
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
										<div class="text-3xl font-semibold">
											<TweenedMetric holdAtZero={serverAndEntries.loading} target={row.count} />
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
								<div class="text-3xl font-semibold">
									<TweenedMetric holdAtZero={serverAndEntries.loading} target={totalServers} />
								</div>
								<Server class="size-6 text-primary" />
							</div>
							<div class="text-xs">Total Currently Active</div>
						</div>
					{/if}

					<div class="h-72 flex flex-col items-center justify-center">
						{#if graphData.some((g) => g.value > 0)}
							<DonutGraph
								class="h-72"
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
								{@const icon =
									'server' in info ? info.server?.manifest.icon : info.entry?.manifest.icon}
								{@const displayName =
									'server' in info
										? (info.server?.alias ?? info.server?.manifest.name)
										: info.entry?.manifest.name}
								{@const description =
									'server' in info
										? info.server?.manifest.description
										: info.entry?.manifest.description}
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
			{/if}
		</div>
	</div>
</Layout>

<McpServerGitSync
	bind:this={sourceDialog}
	onSync={async () => {
		await AdminService.refreshMCPCatalog(DEFAULT_MCP_CATALOG_ID);
		goto('/admin/mcp-servers');
	}}
	defaultCatalogId={DEFAULT_MCP_CATALOG_ID}
/>
<SelectServerType bind:this={selectServerTypeDialog} onSelectServerType={handleSelectServerType} />

<svelte:head>
	<title>Obot | Dashboard</title>
</svelte:head>
