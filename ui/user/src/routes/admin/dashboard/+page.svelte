<script lang="ts">
	import { resolve } from '$app/paths';
	import Layout from '$lib/components/Layout.svelte';
	import TweenedMetric from '$lib/components/TweenedMetric.svelte';
	import DonutGraph from '$lib/components/graph/DonutGraph.svelte';
	import StackedTimeline from '$lib/components/graph/StackedTimeline.svelte';
	import { DEFAULT_MCP_CATALOG_ID } from '$lib/constants';
	import { formatNumber } from '$lib/format';
	import Loading from '$lib/icons/Loading.svelte';
	import { stripMarkdownToText } from '$lib/markdown';
	import {
		AdminService,
		type MCPCatalogEntry,
		type MCPCatalogServer,
		type OrgUser,
		type TokenUsage,
		type TokenUsageWithCategory,
		type TotalTokenUsage
	} from '$lib/services';
	import { errors, mcpServersAndEntries } from '$lib/stores';
	import { aggregateTimelineDataByBucket, getUserLabels } from '../token-usage/utils';
	import { isWithinInterval, subMonths } from 'date-fns';
	import { Activity, ChevronRight, Coins, PencilRuler, Server, Users, Wrench } from 'lucide-svelte';
	import { onMount } from 'svelte';
	import { fade } from 'svelte/transition';
	import { twMerge } from 'tailwind-merge';

	let loading = $state(true);
	let loadingTokensUsage = $state(true);

	let usersData = $state<OrgUser[]>([]);
	let selectedTokenType = $derived<'input' | 'output'>('input');
	let tokenUsageData = $state<TokenUsage[]>([]);
	let totalTokensData = $state<TotalTokenUsage>();
	const end = new Date();
	const start = subMonths(end, 1);

	let mainChartData = $state<TokenUsageWithCategory[]>([]);
	let usersMap = $derived(new Map(usersData.map((user) => [user.id, user])));

	const DEFER_DATA_THRESHOLD = 400;
	const TIMELINE_AGGREGATE_THRESHOLD = 500;

	let monthlyActiveUsers = $derived(
		usersData.filter(
			(user) => user.lastActiveDay && isWithinInterval(user.lastActiveDay, { start, end })
		).length
	);

	let deployedCatalogEntryServers = $state<MCPCatalogServer[]>([]);
	let deployedWorkspaceCatalogEntryServers = $state<MCPCatalogServer[]>([]);
	let serversData = $derived([
		...deployedCatalogEntryServers.filter((server) => !server.deleted),
		...deployedWorkspaceCatalogEntryServers.filter((server) => !server.deleted),
		...mcpServersAndEntries.current.servers.filter((server) => !server.deleted)
	]);

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

		const entrieTypes = Object.values(catalogEntriesCount).reduce(
			(acc, info) => {
				if (info.server) acc.multi++;
				if (!info.entry) return acc;
				if (info.entry.manifest.runtime === 'composite') acc.composite++;
				else if (info.entry.manifest.runtime === 'remote') acc.remote++;
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

		const timeRange = { start, end };
		AdminService.listTokenUsage(timeRange)
			.then((tokenUsage) => {
				if (tokenUsage.length <= DEFER_DATA_THRESHOLD) {
					tokenUsageData = tokenUsage;
					return;
				}
				// Defer so the UI can paint (200, loading off) before heavy derivation. Safari lacks requestIdleCallback.
				const schedule =
					typeof requestIdleCallback !== 'undefined'
						? (fn: () => void) => requestIdleCallback(fn, { timeout: 120 })
						: (fn: () => void) => setTimeout(fn, 0);
				schedule(() => {
					tokenUsageData = tokenUsage;
				});
			})
			.finally(() => {
				loadingTokensUsage = false;
			})
			.catch((error) => {
				if (error?.name === 'AbortError') return;
				errors.append(error);
			});
	});

	function computeMainTimelineData(
		filtered: TokenUsage[],
		group: string,
		users: Map<string, OrgUser>
	): TokenUsageWithCategory[] {
		const userKeys = [...new Set(filtered.map((r) => r.userID ?? r.runName ?? 'Unknown'))].sort();
		const userKeyToLabel = getUserLabels(users, userKeys);
		return filtered.map((r) => ({
			...r,
			date: r.date,
			promptTokens: r.promptTokens ?? 0,
			completionTokens: r.completionTokens ?? 0,
			totalTokens: r.totalTokens ?? (r.promptTokens ?? 0) + (r.completionTokens ?? 0),
			category:
				userKeyToLabel.get(r.userID ?? r.runName ?? 'Unknown') ?? r.userID ?? r.runName ?? 'Unknown'
		}));
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

	$effect(() => {
		if (tokenUsageData.length <= TIMELINE_AGGREGATE_THRESHOLD) {
			const timeline = computeMainTimelineData(tokenUsageData, 'group_by_users', usersMap);
			mainChartData = timeline;
			return;
		}

		const schedule =
			typeof requestIdleCallback !== 'undefined'
				? (fn: () => void) => requestIdleCallback(fn, { timeout: 150 })
				: (fn: () => void) => setTimeout(fn, 0);
		schedule(() => {
			const timeline = computeMainTimelineData(tokenUsageData, 'group_by_users', usersMap);
			mainChartData =
				timeline.length <= TIMELINE_AGGREGATE_THRESHOLD
					? timeline
					: (aggregateTimelineDataByBucket(timeline, start, end) as TokenUsageWithCategory[]);
		});
	});

</script>

<Layout title="Dashboard" classes={{ childrenContainer: 'max-w-none', container: '' }}>
	<div class="grid grid-cols-12 gap-4">
		<div class="flex flex-col md:col-span-8 col-span-12 gap-4">
			<!-- this token usage graph-->
			<div class="grid grid-cols-12 gap-4">
				<div class="md:col-span-4 col-span-12">
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
							See All <ChevronRight class="size-3" />
						</a>
					</div>
				</div>
				<div class="md:col-span-4 col-span-12">
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
				<div class="md:col-span-4 col-span-12">
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
							See All <ChevronRight class="size-3" />
						</a>
					</div>
				</div>
			</div>
			{#if loadingTokensUsage}
				<div class="bg-surface3 h-[400px] animate-pulse rounded-md"></div>
			{:else}
				<div in:fade={{ duration: 150 }} class="paper w-full min-h-72">
					<div class="flex flex-wrap items-center justify-between gap-4">
						<h4 class="flex items-center gap-1 font-semibold">
							Token Usage <span class="text-on-surface1 text-xs font-light">(Last 30 Days)</span>
						</h4>
						<div class="flex shrink-0">
							<button
								class={twMerge(
									'button-secondary rounded-r-none border border-r-0 text-xs',
									selectedTokenType === 'input' && 'bg-surface2 border-surface2'
								)}
								onclick={() => (selectedTokenType = 'input')}
							>
								Input Tokens
							</button>
							<button
								class={twMerge(
									'button-secondary rounded-l-none border text-xs',
									selectedTokenType === 'output' && 'bg-surface2 border-surface2'
								)}
								onclick={() => (selectedTokenType = 'output')}
							>
								Output Tokens
							</button>
						</div>
					</div>
					<StackedTimeline
						{start}
						{end}
						data={mainChartData}
						dateKey="date"
						primaryValueKey={selectedTokenType === 'input' ? 'promptTokens' : 'completionTokens'}
						categoryKey="category"
						class="h-72"
						legend={{
							showSecondaryLabel: false
						}}
					>
						{#snippet tooltipContent(item)}
							{@const value = item.primaryTotal ?? 0}
							<div class="flex flex-col gap-0 text-xs">
								<div class="text-sm font-light">{item.key}</div>
								<div class="text-on-surface1">{item.date}</div>
								<div class="tooltip-divider"></div>
							</div>
							<div class="flex flex-col gap-1">
								<div class="text-on-background flex flex-col">
									<div class="text-xl font-bold">{value.toLocaleString()}</div>
								</div>
							</div>
						{/snippet}
					</StackedTimeline>
				</div>
			{/if}

			<div class="grid grid-cols-12 gap-4">
				<div class="paper gap-1 md:col-span-6 col-span-12">
					<h4 class="flex items-center gap-2 font-semibold">Most Popular Tools</h4>
					<ul class="pt-2 flex flex-col gap-2">
						<li class="flex gap-2 items-center">
							<Wrench class="size-8 opacity-65" />
							<div class="flex flex-col gap-1">
								<p class="text-sm font-medium">Tool Name</p>
								<p class="text-xs text-on-surface1">100 calls</p>
							</div>
						</li>
						<li class="flex gap-2 items-center">
							<Wrench class="size-8 opacity-65" />
							<div class="flex flex-col gap-1">
								<p class="text-sm font-medium">Tool Name</p>
								<p class="text-xs text-on-surface1">100 calls</p>
							</div>
						</li>
						<li class="flex gap-2 items-center">
							<Wrench class="size-8 opacity-65" />
							<div class="flex flex-col gap-1">
								<p class="text-sm font-medium">Tool Name</p>
								<p class="text-xs text-on-surface1">100 calls</p>
							</div>
						</li>
						<li class="flex gap-2 items-center">
							<Wrench class="size-8 opacity-65" />
							<div class="flex flex-col gap-1">
								<p class="text-sm font-medium">Tool Name</p>
								<p class="text-xs text-on-surface1">100 calls</p>
							</div>
						</li>
						<li class="flex gap-2 items-center">
							<Wrench class="size-8 opacity-65" />
							<div class="flex flex-col gap-1">
								<p class="text-sm font-medium">Tool Name</p>
								<p class="text-xs text-on-surface1">100 calls</p>
							</div>
						</li>
					</ul>
				</div>
				<div class="paper gap-1 md:col-span-6 col-span-12">
					<h4 class="flex items-center gap-2 font-semibold">Most Popular Skills</h4>
					<ul class="pt-2 flex flex-col gap-2">
						<li class="flex gap-2 items-center">
							<PencilRuler class="size-8 opacity-65" />
							<div class="flex flex-col gap-1">
								<p class="text-sm font-medium">Skill Name</p>
								<p class="text-xs text-on-surface1">100 calls</p>
							</div>
						</li>
						<li class="flex gap-2 items-center">
							<PencilRuler class="size-8 opacity-65" />
							<div class="flex flex-col gap-1">
								<p class="text-sm font-medium">Skill Name</p>
								<p class="text-xs text-on-surface1">100 calls</p>
							</div>
						</li>
						<li class="flex gap-2 items-center">
							<PencilRuler class="size-8 opacity-65" />
							<div class="flex flex-col gap-1">
								<p class="text-sm font-medium">Skill Name</p>
								<p class="text-xs text-on-surface1">100 calls</p>
							</div>
						</li>
						<li class="flex gap-2 items-center">
							<PencilRuler class="size-8 opacity-65" />
							<div class="flex flex-col gap-1">
								<p class="text-sm font-medium">Skill Name</p>
								<p class="text-xs text-on-surface1">100 calls</p>
							</div>
						</li>
						<li class="flex gap-2 items-center">
							<Server class="size-8 opacity-65" />
							<div class="flex flex-col gap-1">
								<p class="text-sm font-medium">Server Name</p>
								<p class="text-xs text-on-surface1">100 calls</p>
							</div>
						</li>
					</ul>
				</div>
			</div>
		</div>
		<div class="md:col-span-4 col-span-12 flex flex-col gap-4">
			{#if serverAndEntries.loading}
				<div class="bg-surface3 h-[380px] animate-pulse rounded-md"></div>
				<div class="bg-surface3 h-[380px] animate-pulse rounded-md"></div>
			{:else}
				<div in:fade={{ duration: 150 }} class="paper">
					<h4 class="font-semibold">Active Servers</h4>

					<div class="mb-2 flex flex-col justify-center items-center gap-2">
						<div class="text-xs">Total Currently Active</div>
						<div class="flex w-full gap-2 items-center justify-center">
							<div class="text-3xl font-semibold">
								<TweenedMetric
									holdAtZero={serverAndEntries.loading}
									target={totalServers}
								/>
							</div>
							<Server class="size-8 text-primary" />
						</div>
					</div>

					<div class="h-px w-full bg-surface2 mb-4"></div>

					<DonutGraph class="h-72" donutRatio={0.65} data={graphData} />

					<div class="flex justify-end">
						<a
							href={resolve('/admin/mcp-servers?view=deployments')}
							class="text-[11px] transition-colors self-end translate-x-2 duration-200 bg-surface3/50 hover:bg-surface3 rounded-md py-0.5 w-fit px-2 flex items-center gap-1"
						>
							See All <ChevronRight class="size-3" />
						</a>
					</div>
				</div>
				<div in:fade={{ duration: 150 }} class="paper gap-1 flex grow">
					<h4 class="flex items-center gap-2 font-semibold">Most Popular Servers</h4>
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
								class="flex gap-2 items-center hover:bg-surface1 -mx-2 px-2 py-1 rounded-md"
								href={url ? resolve(url as `/${string}`) : undefined}
							>
								{#if icon}
									<img src={icon} alt={info.id} class="size-9 bg-surface1 rounded-md p-1" />
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
					<div class="flex grow"></div>
					<a
						href={resolve('/admin/mcp-servers')}
						class="justify-end self-end text-[11px] translate-x-2 transition-colors duration-200 bg-surface3/50 hover:bg-surface3 rounded-md py-0.5 w-fit px-2 flex items-center gap-1"
					>
						See All <ChevronRight class="size-3" />
					</a>
				</div>
			{/if}
		</div>
	</div>
</Layout>

<svelte:head>
	<title>Obot | Dashboard</title>
</svelte:head>
