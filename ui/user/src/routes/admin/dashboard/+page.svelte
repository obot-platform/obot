<script lang="ts">
	import { resolve } from '$app/paths';
	import DotDotDot from '$lib/components/DotDotDot.svelte';
	import Layout from '$lib/components/Layout.svelte';
	import TweenedMetric from '$lib/components/TweenedMetric.svelte';
	import {
		transformTopToolCalls,
		transformTopServerUsage,
		transformAvgToolCallResponseTime
	} from '$lib/components/admin/usage/utils';
	import DonutGraph from '$lib/components/graph/DonutGraph.svelte';
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
	import { errors, mcpServersAndEntries } from '$lib/stores';
	import { goto } from '$lib/url';
	import { isWithinInterval, set, subMonths } from 'date-fns';
	import { Activity, ChevronRight, Coins, Plus, Server, Users, Wrench } from 'lucide-svelte';
	import { onMount } from 'svelte';
	import { fade } from 'svelte/transition';

	const TOP_TOOLS_LIMIT = 5;
	const TOP_SERVERS_LIMIT = 12;

	let loading = $state(true);
	let loadingToolUsage = $state(true);

	let usersData = $state<OrgUser[]>([]);
	let totalTokensData = $state<TotalTokenUsage>();

	let selectServerTypeDialog = $state<ReturnType<typeof SelectServerType>>();

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
		return transformTopToolCalls(stats)
			.map((t) => ({
				compositeKey: t.toolName,
				toolLabel: t.toolName,
				count: t.count,
				serverDisplayName: t.serverDisplayName
			}))
			.filter(
				(t) => !t.serverDisplayName.startsWith('nba1') && !t.serverDisplayName.startsWith('Obot ')
			);
	}

	function topServersFromStats(stats: AuditLogUsageStats | undefined): TopServerUsageRow[] {
		return transformTopServerUsage(stats).filter(
			(s) => !s.serverName.startsWith('nba1') && !s.serverName.startsWith('Obot ')
		);
	}

	function avgToolCallResponseTimeFromStats(stats: AuditLogUsageStats | undefined) {
		return transformAvgToolCallResponseTime(stats).filter(
			(t) => !t.serverDisplayName.startsWith('nba1') && !t.serverDisplayName.startsWith('Obot ')
		);
	}

	let monthlyActiveUsers = $derived(
		usersData.filter(
			(user) => user.lastActiveDay && isWithinInterval(user.lastActiveDay, { start, end })
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
					count: server.mcpServerInstanceUserCount ?? 0,
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

		const entrieTypes = data.reduce(
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

		return {
			graphData: [
				{
					label: 'Multi-User',
					value: entrieTypes.multi
				},
				{
					label: 'Single-User',
					value: entrieTypes.single
				},
				{
					label: 'Remote',
					value: entrieTypes.remote
				},
				{
					label: 'Composite',
					value: entrieTypes.composite
				}
			],
			popularServers: sortByCountDescending.filter((s) => s.count > 0).slice(0, 5),
			totalServers: serversData.length
		};
	}

	const serverAndEntries = $derived(mcpServersAndEntries.current);
	const { graphData, popularServers, totalServers } = $derived(
		compileServerAndEntries(serversData, serverAndEntries.entries)
	);

	onMount(async () => {
		usersData = await AdminService.listUsersIncludeDeleted();
		totalTokensData = await AdminService.listTotalTokenUsage();
		deployedCatalogEntryServers =
			await AdminService.listAllCatalogDeployedSingleRemoteServers(DEFAULT_MCP_CATALOG_ID);
		deployedWorkspaceCatalogEntryServers =
			await AdminService.listAllWorkspaceDeployedSingleRemoteServers();
		loading = false;

		const endToolStats = set(new Date(), { milliseconds: 0, seconds: 59 });
		const startToolStats = subMonths(endToolStats, 1);
		AdminService.listAuditLogUsageStats({
			start_time: startToolStats.toISOString(),
			end_time: endToolStats.toISOString()
		})
			.then((stats) => {
				topToolCalls = topToolCallsFromStats(stats).slice(0, TOP_TOOLS_LIMIT);
				topServerUsage = topServersFromStats(stats).slice(0, TOP_SERVERS_LIMIT);
				avgToolCallResponseTime = avgToolCallResponseTimeFromStats(stats).slice(0, TOP_TOOLS_LIMIT);
			})
			.catch((error) => {
				if (error?.name === 'AbortError') return;
				errors.append(error);
			})
			.finally(() => {
				loadingToolUsage = false;
			});
	});

	function handleSelectServerType(type: LaunchServerType) {
		selectServerTypeDialog?.close();
		goto(resolve(`/admin/mcp-servers?new=${type}`));
	}

	function getServerUrl(server: MCPCatalogServer) {
		if (server.powerUserWorkspaceID) {
			return `/admin/mcp-servers/w/${server.powerUserWorkspaceID}/s/${server.id}?view=audit-logs`;
		}
		return `/admin/mcp-servers/s/${server.id}?view=audit-logs`;
	}

	function getEntryUrl(entry: MCPCatalogEntry) {
		if (entry.powerUserWorkspaceID) {
			return `/admin/mcp-servers/w/${entry.powerUserWorkspaceID}/c/${entry.id}?view=audit-logs`;
		}
		return `/admin/mcp-servers/c/${entry.id}?view=audit-logs`;
	}
</script>

<Layout title="Dashboard" classes={{ childrenContainer: 'max-w-none', container: '' }}>
	<div class="@container grid grid-cols-12 gap-4">
		<div class="flex flex-col col-span-12 @min-[768px]:col-span-8 gap-4">
			<!-- this token usage graph-->
			<div class="grid grid-cols-12 gap-4">
				<div class="col-span-12 @min-[768px]:col-span-4">
					<div class="paper gap-2">
						<div class="text-xs text-on-surface1 flex items-center gap-1">
							Total Users
							{#if loading}
								<Loading class="size-3" />
							{/if}
						</div>
						<div class="flex w-full justify-between">
							<div class="text-3xl font-semibold">
								<TweenedMetric holdAtZero={loading} target={usersData.length} />
							</div>
							<Users class="size-8 text-primary" />
						</div>
						<a
							href={resolve('/admin/users')}
							class="text-[11px] translate-x-2 self-end bg-surface3/50 transition-colors duration-200 hover:bg-surface3 rounded-md py-0.5 w-fit px-2 flex items-center gap-1"
						>
							See More <ChevronRight class="size-3" />
						</a>
					</div>
				</div>
				<div class="col-span-12 @min-[768px]:col-span-4">
					<div class="paper gap-2">
						<div class="text-xs text-on-surface1 flex items-center gap-1">
							Monthly Active Users
							{#if loading}
								<Loading class="size-3" />
							{/if}
						</div>
						<div class="flex w-full justify-between">
							<div class="text-3xl font-semibold">
								<TweenedMetric holdAtZero={loading} target={monthlyActiveUsers} />
							</div>
							<Activity class="size-8 text-primary" />
						</div>
						<div class="text-xs text-on-surface1">Last 30 Days</div>
					</div>
				</div>
				<div class="col-span-12 @min-[768px]:col-span-4">
					<div class="paper gap-2">
						<div class="text-xs text-on-surface1 flex items-center gap-1">
							Total Tokens
							{#if loading}
								<Loading class="size-3" />
							{/if}
						</div>
						<div class="flex w-full justify-between">
							<div class="text-3xl font-semibold">
								<TweenedMetric
									holdAtZero={loading}
									target={totalTokensData?.totalTokens ?? 0}
									format={(n) => formatNumber(Math.max(0, Math.round(n)))}
								/>
							</div>
							<Coins class="size-8 text-primary" />
						</div>
						<a
							href={resolve('/admin/token-usage')}
							class="text-[11px] translate-x-2 self-end bg-surface3/50 transition-colors duration-200 hover:bg-surface3 rounded-md py-0.5 w-fit px-2 flex items-center gap-1"
						>
							See More <ChevronRight class="size-3" />
						</a>
					</div>
				</div>
			</div>
			{#if loading}
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
					{#if topToolCalls.length > 0}
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
									<div class="size-8 rounded-md bg-surface3 shrink-0"></div>
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
					{#if avgToolCallResponseTime.length > 0}
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
			{#if serverAndEntries.loading}
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
					{#if deployedCatalogEntryServers.length > 0 || deployedWorkspaceCatalogEntryServers.length > 0}
						<h4 class="font-semibold">Active Servers</h4>
						<div class="mb-2 flex flex-col justify-center items-center gap-2">
							<div class="flex w-full gap-2 items-center justify-center">
								<div class="text-3xl font-semibold">
									<TweenedMetric holdAtZero={serverAndEntries.loading} target={totalServers} />
								</div>
								<Server class="size-8 text-primary" />
							</div>
							<div class="text-xs">Total Currently Active</div>
						</div>

						<div class="h-px w-full bg-surface2 mb-4"></div>

						<div class="h-72 flex flex-col items-center justify-center">
							{#if graphData.some((g) => g.value > 0)}
								<DonutGraph class="h-72" donutRatio={0.65} data={graphData} />
							{:else}
								<p class="font-light text-xs text-on-surface1 pt-2 text-center">
									No servers have been deployed yet.
								</p>
							{/if}
						</div>

						<div class="flex justify-end">
							<a
								href={resolve('/admin/mcp-servers?view=deployments')}
								class="text-[11px] transition-colors self-end translate-x-2 duration-200 bg-surface3/50 hover:bg-surface3 rounded-md py-0.5 w-fit px-2 flex items-center gap-1"
							>
								See All <ChevronRight class="size-3" />
							</a>
						</div>
					{:else}
						<div class="grow flex flex-col items-center justify-center gap-4">
							<div>
								<p class="text-sm text-center mb-1">
									Looks like you don't have any servers created yet.
								</p>
								<p class="text-sm text-center">Click below to get started!</p>
							</div>
							<DotDotDot
								class="button-primary w-full text-sm md:w-fit self-center"
								placement="bottom"
							>
								{#snippet icon()}
									<span class="flex items-center justify-center gap-1">
										<Plus class="size-4" /> Add MCP Server
									</span>
								{/snippet}
								<button
									class="menu-button"
									onclick={() => {
										selectServerTypeDialog?.open();
									}}
								>
									Add server
								</button>
								<button
									class="menu-button"
									onclick={() => {
										// editingSource = {
										// 	index: -1,
										// 	value: ''
										// };
										// sourceDialog?.showModal();
									}}
								>
									Add server(s) from Git
								</button>
							</DotDotDot>
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
												{@html stripMarkdownToText(description ?? '')}
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
					{#if popularServers.length > 0}
						<a
							href={resolve('/admin/mcp-servers')}
							class="justify-end self-end text-[11px] translate-x-2 transition-colors duration-200 bg-surface3/50 hover:bg-surface3 rounded-md py-0.5 w-fit px-2 flex items-center gap-1"
						>
							See All <ChevronRight class="size-3" />
						</a>
					{/if}
				</div>
			{/if}
		</div>
	</div>
</Layout>

<SelectServerType bind:this={selectServerTypeDialog} onSelectServerType={handleSelectServerType} />

<svelte:head>
	<title>Obot | Dashboard</title>
</svelte:head>
