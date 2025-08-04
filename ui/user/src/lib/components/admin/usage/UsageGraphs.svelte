<script lang="ts">
	import {
		ChevronsLeft,
		ChevronsRight,
		LoaderCircle,
		Funnel,
		ChartBarDecreasing
	} from 'lucide-svelte';
	import {
		AdminService,
		type AuditLogUsageStats,
		type OrgUser,
		type UsageStatsFilters
	} from '$lib/services';
	import StatBar from '../StatBar.svelte';
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import HorizontalBarGraph from '../../graph/HorizontalBarGraph.svelte';
	import UsageFilters from './UsageFilters.svelte';
	import { clickOutside } from '$lib/actions/clickoutside';
	import { dialogAnimation } from '$lib/actions/dialogAnimation';

	interface Props {
		mcpId?: string;
		mcpCatalogEntryId?: string;
		mcpServerDisplayName?: string;
		users: OrgUser[];
		filters?: UsageStatsFilters;
		sort?: { sortBy: string; sortOrder: 'asc' | 'desc' };
	}

	let {
		mcpId,
		mcpCatalogEntryId,
		mcpServerDisplayName,
		filters,
		sort = { sortBy: 'created_at', sortOrder: 'desc' },
		users
	}: Props = $props();

	let listUsageStats = $state<Promise<AuditLogUsageStats>>();
	let graphPageSize = $state(10);
	let graphPages = $state<Record<string, number>>({});
	// eslint-disable-next-line @typescript-eslint/no-explicit-any
	let graphData = $derived<Record<string, Array<any>>>({});
	let graphTotals = $derived<Record<string, number>>({});
	let showFilters = $state(false);
	let rightSidebar = $state<HTMLDialogElement>();

	type GraphConfig = {
		id: string;
		label: string;
		xKey: string;
		yKey: string;
		tooltip: string;
		formatXLabel?: (x: string | number) => string;
		// eslint-disable-next-line @typescript-eslint/no-explicit-any
		formatTooltipText?: (data: Record<string, any>) => string;
		// eslint-disable-next-line @typescript-eslint/no-explicit-any
		transform: (stats?: AuditLogUsageStats) => Array<Record<string, any>>;
	};

	const graphConfigs: GraphConfig[] = [
		{
			id: 'most-frequent-tool-calls',
			label: 'Most Frequent Tool Calls',
			xKey: 'toolName',
			yKey: 'count',
			tooltip: 'calls',
			formatXLabel: (d) => String(d).split('.').slice(1).join('.'),
			formatTooltipText: (data) => `${data.count} calls • ${data.serverDisplayName}`,
			transform: (stats) => {
				const counts = new Map<string, { count: number; serverDisplayName: string }>();
				for (const s of stats?.items ?? []) {
					for (const call of s.toolCalls ?? []) {
						const key = `${s.mcpServerDisplayName}.${call.toolName}`;
						const existing = counts.get(key) ?? {
							count: 0,
							serverDisplayName: s.mcpServerDisplayName
						};
						existing.count += call.callCount;
						counts.set(key, existing);
					}
				}
				return Array.from(counts.entries())
					.map(([toolName, { count, serverDisplayName }]) => ({
						toolName,
						count,
						serverDisplayName
					}))
					.sort((a, b) => b.count - a.count);
			}
		},
		{
			id: 'most-frequently-used-servers',
			label: 'Most Frequently Used Servers',
			xKey: 'serverName',
			yKey: 'count',
			tooltip: 'calls',
			transform: (stats) => {
				const counts = new Map<string, number>();
				for (const s of stats?.items ?? []) {
					const total = (s.toolCalls ?? []).reduce((sum, t) => sum + t.callCount, 0);
					if (total > 0) {
						counts.set(s.mcpServerDisplayName, (counts.get(s.mcpServerDisplayName) ?? 0) + total);
					}
				}
				return Array.from(counts.entries())
					.map(([serverName, count]) => ({ serverName, count }))
					.sort((a, b) => b.count - a.count);
			}
		},
		{
			id: 'tool-call-average-response-time',
			label: 'Tool Call Average Response Time',
			xKey: 'toolName',
			yKey: 'averageResponseTimeMs',
			tooltip: 'ms',
			formatXLabel: (d) => String(d).split('.').slice(1).join('.'),
			formatTooltipText: (data) =>
				`${data.averageResponseTimeMs.toFixed(2)}ms avg • ${data.serverDisplayName}`,
			transform: (stats) => {
				const responseTimes = new Map<
					string,
					{ total: number; count: number; serverDisplayName: string }
				>();

				for (const s of stats?.items ?? []) {
					for (const call of s.toolCalls ?? []) {
						const key = `${s.mcpServerDisplayName}.${call.toolName}`;
						for (const item of call.items ?? []) {
							const entry = responseTimes.get(key) ?? {
								total: 0,
								count: 0,
								serverDisplayName: s.mcpServerDisplayName
							};
							entry.total += item.processingTimeMs;
							entry.count += 1;
							responseTimes.set(key, entry);
						}
					}
				}

				return Array.from(responseTimes.entries())
					.map(([toolName, { total, count, serverDisplayName }]) => ({
						toolName,
						averageResponseTimeMs: count > 0 ? total / count : 0,
						serverDisplayName
					}))
					.sort((a, b) => b.averageResponseTimeMs - a.averageResponseTimeMs);
			}
		},
		{
			id: 'tool-call-individual-response-time',
			label: 'Tool Call Individual Response Time',
			xKey: 'toolName',
			yKey: 'processingTimeMs',
			tooltip: 'ms',
			formatXLabel: (d) => {
				const parts = String(d).split('.');
				return parts[parts.length - 1];
			},
			formatTooltipText: (data) =>
				`${data.processingTimeMs.toFixed(2)}ms • ${data.serverDisplayName}`,
			transform: (stats) => {
				const rows = [];
				for (const s of stats?.items ?? []) {
					for (const call of s.toolCalls ?? []) {
						for (const [itemIndex, item] of (call.items ?? []).entries()) {
							rows.push({
								toolName: `${s.mcpServerDisplayName}.${s.mcpID}.${itemIndex}.${call.toolName}`,
								processingTimeMs: item.processingTimeMs,
								serverDisplayName: s.mcpServerDisplayName
							});
						}
					}
				}
				return rows.sort((a, b) => b.processingTimeMs - a.processingTimeMs);
			}
		},
		{
			id: 'tool-call-errors',
			label: 'Tool Call Errors',
			xKey: 'toolName',
			yKey: 'errorCount',
			tooltip: 'errors',
			formatXLabel: (d) => {
				// Just grab the tool name
				const parts = String(d).split('.');
				return parts[parts.length - 1];
			},
			formatTooltipText: (data) => `${data.errorCount} errors • ${data.serverDisplayName}`,
			transform: (stats) => {
				const errorCounts = new Map<string, { errorCount: number; serverDisplayName: string }>();
				for (const s of stats?.items ?? []) {
					for (const call of s.toolCalls ?? []) {
						const key = `${s.mcpServerDisplayName}.${call.toolName}`;
						let count = 0;
						for (const item of call.items ?? []) {
							if (item.error || item.responseStatus >= 400) count++;
						}
						if (count > 0) {
							const existing = errorCounts.get(key) ?? {
								errorCount: 0,
								serverDisplayName: s.mcpServerDisplayName
							};
							existing.errorCount += count;
							errorCounts.set(key, existing);
						}
					}
				}
				return Array.from(errorCounts.entries())
					.filter(([_, { errorCount }]) => errorCount > 0)
					.map(([toolName, { errorCount, serverDisplayName }]) => ({
						toolName,
						errorCount,
						serverDisplayName
					}))
					.sort((a, b) => b.errorCount - a.errorCount);
			}
		},
		{
			id: 'tool-call-errors-by-server',
			label: 'Tool Call Errors by Server',
			xKey: 'serverName',
			yKey: 'errorCount',
			tooltip: 'errors',
			formatXLabel: (d) => String(d),
			transform: (stats) => {
				const errorCounts = new Map<string, number>();
				for (const s of stats?.items ?? []) {
					let count = 0;
					for (const call of s.toolCalls ?? []) {
						for (const item of call.items ?? []) {
							if (item.error || item.responseStatus >= 400) count++;
						}
					}
					if (count > 0) {
						errorCounts.set(
							s.mcpServerDisplayName,
							(errorCounts.get(s.mcpServerDisplayName) ?? 0) + count
						);
					}
				}
				return Array.from(errorCounts.entries())
					.map(([serverName, errorCount]) => ({ serverName, errorCount }))
					.sort((a, b) => b.errorCount - a.errorCount);
			}
		},
		{
			id: 'most-active-users',
			label: 'Most Active Users',
			xKey: 'userId',
			yKey: 'callCount',
			tooltip: 'calls',
			formatXLabel: (userId) => {
				const user = users.find((u) => u.id === userId);
				return user ? user.email : String(userId);
			},
			transform: (stats) => {
				const userCounts = new Map<string, number>();
				for (const s of stats?.items ?? []) {
					for (const call of s.toolCalls ?? []) {
						for (const item of call.items ?? []) {
							const count = userCounts.get(item.userID) ?? 0;
							userCounts.set(item.userID, count + 1);
						}
					}
				}
				return Array.from(userCounts.entries())
					.map(([userId, callCount]) => ({ userId, callCount }))
					.sort((a, b) => b.callCount - a.callCount);
			}
		}
	];

	// Filter out server-related graphs when viewing a specific server
	const filteredGraphConfigs = $derived.by(() => {
		const isSpecificServer = mcpId || mcpCatalogEntryId;
		if (isSpecificServer) {
			// Remove server comparison graphs when viewing a specific server
			return graphConfigs.filter(
				(cfg) =>
					cfg.id !== 'most-frequently-used-servers' && cfg.id !== 'tool-call-errors-by-server'
			);
		}
		return graphConfigs;
	});

	$effect(() => {
		if (mcpId || mcpCatalogEntryId || mcpServerDisplayName || filters || sort) reload();
	});

	async function reload() {
		const statsPromise = mcpId
			? AdminService.listServerOrInstanceAuditLogStats(mcpId, {
					startTime: filters?.startTime ?? '',
					endTime: filters?.endTime ?? ''
				})
			: AdminService.listAuditLogUsageStats({
					startTime: filters?.startTime ?? '',
					endTime: filters?.endTime ?? '',
					mcpServerCatalogEntryName: mcpCatalogEntryId,
					mcpServerDisplayName
				});

		// Apply client-side user filtering if userId filter is set
		listUsageStats = statsPromise.then((stats) => {
			if (!filters?.userId || !stats?.items) {
				return stats;
			}

			// Filter the items to only include data for the specified user
			const filteredItems = stats.items
				.map((item) => ({
					...item,
					toolCalls:
						item.toolCalls
							?.map((toolCall) => ({
								...toolCall,
								items:
									toolCall.items?.filter((callItem) => callItem.userID === filters.userId) ?? []
							}))
							?.filter((toolCall) => (toolCall.items?.length ?? 0) > 0) ?? []
				}))
				.filter((item) => (item.toolCalls?.length ?? 0) > 0);

			return {
				...stats,
				items: filteredItems
			};
		});
	}

	$effect(() => {
		if (!listUsageStats) return;
		updateGraphs();
	});

	async function updateGraphs() {
		const stats = await listUsageStats;
		// eslint-disable-next-line @typescript-eslint/no-explicit-any
		const data: Record<string, any[]> = {};
		const totals: Record<string, number> = {};

		for (const cfg of filteredGraphConfigs) {
			const rows = cfg.transform(stats);
			data[cfg.id] = rows;
			totals[cfg.id] = rows.length;
		}

		graphData = data;
		graphTotals = totals;
	}

	function setGraphPage(id: string, p: number) {
		graphPages[id] = p;
	}

	function handleRightSidebarClose() {
		rightSidebar?.close();
		showFilters = false;
	}

	function hasData(graphConfigs: GraphConfig[]) {
		return graphConfigs.some((cfg) => graphTotals[cfg.id] ?? 0 > 0);
	}
</script>

{#await listUsageStats}
	<div class="flex w-full justify-center">
		<LoaderCircle class="size-6 animate-spin" />
	</div>
{:then stats}
	{#if !hasData(filteredGraphConfigs)}
		<div class="mt-12 flex w-md flex-col items-center gap-4 self-center text-center">
			<ChartBarDecreasing class="size-24 text-gray-200 dark:text-gray-900" />
			<h4 class="text-lg font-semibold text-gray-400 dark:text-gray-600">No usage stats</h4>
			<p class="w-sm text-sm font-light text-gray-400 dark:text-gray-600">
				Currently, there are no usage stats for the range or selected filters. Try modifying your
				search criteria or try again later.
			</p>
		</div>
	{:else}
		<div class="flex flex-col gap-8">
			<!-- Summary with filter button -->
			<div class="flex items-center justify-between gap-4">
				<div class="flex-1">
					<StatBar startTime={filters?.startTime ?? ''} endTime={filters?.endTime ?? ''} />
				</div>
				{#if !(mcpId || mcpCatalogEntryId)}
					<button
						class="hover:bg-surface1 dark:bg-surface1 dark:hover:bg-surface3 dark:border-surface3 button flex h-12 w-fit items-center justify-center gap-1 rounded-lg border border-transparent bg-white shadow-sm"
						onclick={() => {
							showFilters = true;
							rightSidebar?.show();
						}}
					>
						<Funnel class="size-4" />
						Filters
					</button>
				{/if}
			</div>

			<div class="grid grid-cols-1 gap-8 lg:grid-cols-2">
				{#each filteredGraphConfigs as cfg (cfg.id)}
					{@const full = graphData[cfg.id] ?? []}
					{@const total = graphTotals[cfg.id] ?? 0}
					{@const page = graphPages[cfg.id] ?? 0}
					{@const maxPage = Math.max(0, Math.ceil(total / graphPageSize) - 1)}
					{@const paginated = full.slice(page * graphPageSize, (page + 1) * graphPageSize)}

					<div
						class="dark:bg-surface1 dark:border-surface3 rounded-md border border-transparent bg-white p-6 shadow-sm"
					>
						<h3 class="mb-4 text-lg font-semibold">{cfg.label}</h3>

						{#if paginated.length > 0}
							<HorizontalBarGraph
								data={paginated}
								x={cfg.xKey}
								y={cfg.yKey}
								padding={10}
								formatTooltipText={cfg.formatTooltipText ||
									((d) => `${d[cfg.yKey]} ${cfg.tooltip}`)}
								formatXLabel={cfg.formatXLabel}
							/>
						{:else}
							<div
								class="flex h-[300px] items-center justify-center text-sm font-light text-gray-400 dark:text-gray-600"
							>
								No data available
							</div>
						{/if}

						{#if maxPage > 0}
							<div
								class="mt-4 flex items-center justify-center gap-4 border-t border-gray-200 p-4 dark:border-gray-700"
							>
								<button
									class="icon-button disabled:opacity-50"
									onclick={() => setGraphPage(cfg.id, Math.max(0, page - 1))}
									disabled={page === 0}
									use:tooltip={'Previous Page'}
								>
									<ChevronsLeft class="size-5" />
								</button>
								<span class="text-sm">
									Page {page + 1} of {maxPage + 1}
									(showing {Math.min(graphPageSize, total - page * graphPageSize)} of {total} items)
								</span>
								<button
									class="icon-button disabled:opacity-50"
									onclick={() => setGraphPage(cfg.id, Math.min(maxPage, page + 1))}
									disabled={page >= maxPage}
									use:tooltip={'Next Page'}
								>
									<ChevronsRight class="size-5" />
								</button>
							</div>
						{/if}
					</div>
				{/each}
			</div>

			<dialog
				bind:this={rightSidebar}
				use:clickOutside={[handleRightSidebarClose, true]}
				use:dialogAnimation={{ type: 'drawer' }}
				class="dark:border-surface1 dark:bg-surface1 fixed! top-0! right-0! bottom-0! left-auto! z-40 h-screen w-auto max-w-none rounded-none border-0 bg-white shadow-lg outline-none!"
			>
				{#if showFilters}
					<UsageFilters usageStats={stats} {users} onClose={handleRightSidebarClose} {filters} />
				{/if}
			</dialog>
		</div>
	{/if}
{/await}
