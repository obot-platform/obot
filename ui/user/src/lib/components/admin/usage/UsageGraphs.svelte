<script lang="ts">
	import { ChevronsLeft, ChevronsRight, Funnel, ChartBarDecreasing, X } from 'lucide-svelte';
	import {
		AdminService,
		type AuditLogURLFilters,
		type AuditLogUsageStats,
		type OrgUser,
		type UsageStatsFilters
	} from '$lib/services';
	import StatBar from '../StatBar.svelte';
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import HorizontalBarGraph from '../../graph/HorizontalBarGraph.svelte';
	import { clickOutside } from '$lib/actions/clickoutside';
	import { dialogAnimation } from '$lib/actions/dialogAnimation';
	import { SvelteMap } from 'svelte/reactivity';
	import { afterNavigate, goto } from '$app/navigation';
	import FiltersDrawer from '../filters-drawer/FiltersDrawer.svelte';
	import { getUserDisplayName } from '$lib/utils';
	import type { SupportedStateFilter } from './types';
	import { fade, slide } from 'svelte/transition';
	import { flip } from 'svelte/animate';
	import { endOfDay, isBefore, set, subDays } from 'date-fns';
	import { page } from '$app/state';
	import type { DateRange } from '$lib/components/Calendar.svelte';
	import AuditLogCalendar from '../audit-logs/AuditLogCalendar.svelte';
	import Loading from '$lib/icons/Loading.svelte';

	type Props = {
		mcpId?: string | null;
		mcpServerDisplayName?: string | null;
		mcpServerCatalogEntryName?: string | null;
	};

	type GraphConfig = {
		id: string;
		label: string;
		xKey: string;
		yKey: string;
		tooltip: string;
		formatXLabel?: (x: string | number) => string;
		formatTooltipText?: (data: Record<string, string | number>) => string;
		transform: (stats?: AuditLogUsageStats) => Array<Record<string, string | number>>;
	};

	let { mcpId, mcpServerCatalogEntryName, mcpServerDisplayName }: Props = $props();

	const supportedFilters: SupportedStateFilter[] = [
		'user_ids',
		'mcp_id',
		'mcp_server_display_names',
		'mcp_server_catalog_entry_names',
		'start_time',
		'end_time'
	];

	const proxy = new Map<SupportedStateFilter, keyof AuditLogURLFilters>([
		['user_ids', 'user_id'],
		['mcp_id', 'mcp_id'],
		['mcp_server_display_names', 'mcp_server_display_name'],
		['mcp_server_catalog_entry_names', 'mcp_server_catalog_entry_name'],
		['end_time', 'end_time'],
		['start_time', 'start_time']
	]);

	const searchParamsAsArray: [keyof UsageStatsFilters, string | undefined | null][] = $derived(
		supportedFilters.map((d) => {
			const hasSearchParam = page.url.searchParams.has(d);

			const value = page.url.searchParams.get(d);
			const isValueDefined = isSafe(value);

			return [
				d,
				isValueDefined
					? // Value is defined then decode and use it
						decodeURIComponent(value)
					: hasSearchParam
						? // Value is not defined but has a search param then override with empty string
							''
						: // No search params return default value if exist otherwise return undefined
							null
			];
		})
	);

	// Extract search supported params from the URL and convert them to UsageStatsFilters
	// This is used to filter the audit logs based on the URL parameters
	const searchParamFilters = $derived.by<UsageStatsFilters>(() => {
		return searchParamsAsArray.reduce(
			(acc, [key, value]) => {
				acc[key!] = value;
				return acc;
			},
			{} as Record<string, string | number | undefined | null>
		);
	});

	const propsFilters = $derived.by(() => {
		const entries: [key: string, value: string | null | undefined][] = [
			['mcp_id', mcpId],
			['mcp_server_display_names', mcpServerDisplayName],
			['mcp_server_catalog_entry_names', mcpServerCatalogEntryName]
		];

		return (
			entries
				// Filter out undefined values, null values should be kept as they mean the value is specified
				.filter(([, value]) => value !== undefined)
				.reduce(
					(acc, [key, value]) => ((acc[key] = value!), acc),
					{} as Record<string, string | null>
				)
		);
	});

	const propsFiltersKeys = $derived(new Set(Object.keys(propsFilters)));

	// Keep only filters with defined values
	const pillsSearchParamFilters = $derived.by(() => {
		const filters = searchParamsAsArray
			// exclude start_time and end_time from pills filters
			.filter(([key, value]) => !(key === 'start_time' || key === 'end_time') && isSafe(value))
			.reduce(
				(acc, [key, value]) => {
					acc[key!] = value as string | number;
					return acc;
				},
				{} as Record<string, string | number>
			);

		// Sort pills; those from props goes first
		return Object.entries({
			...filters,
			...propsFilters
		})
			.sort((a, b) => {
				if (propsFiltersKeys.has(a[0])) {
					return -1;
				}

				return a[0].localeCompare(b[0]);
			})
			.reduce(
				(acc, val) => {
					acc[val[0]] = val[1] as string | number;
					return acc;
				},
				{} as Record<string, string | number>
			);
	});

	// Filters to be used in the audit logs slideover
	// Exclude filters that are set via props and not undefined
	const auditLogsSlideoverFilters = $derived.by(() => {
		const clone = { ...searchParamFilters };

		for (const key of ['start_time', 'end_time']) {
			delete clone[key as SupportedStateFilter];
		}

		return { ...clone, ...propsFilters };
	});

	let timeRangeFilters = $derived.by(() => {
		const { start_time, end_time } = searchParamFilters;

		const endTime = set(new Date(end_time || new Date()), { milliseconds: 0, seconds: 59 });

		const getStartTime = (date: typeof start_time) => {
			const parsedStartTime = set(new Date(date ? date : Date.now()), {
				milliseconds: 0,
				seconds: 0
			});

			if (date) {
				// Ensure start time is not after end time
				if (isBefore(parsedStartTime, endTime)) {
					return parsedStartTime;
				}
			}

			// Return 7 days before end time
			return subDays(endTime, 7);
		};

		const startTime = getStartTime(start_time);

		return {
			startTime,
			endTime
		};
	});

	let filters = $derived({
		...searchParamFilters,
		...propsFilters,
		start_time: timeRangeFilters.startTime.toISOString(),
		end_time: timeRangeFilters.endTime.toISOString()
	});

	let showLoadingSpinner = $state(true);
	let listUsageStats = $state<Promise<AuditLogUsageStats>>();
	let graphPageSize = $state(10);
	let graphPages = $state<Record<string, number>>({});
	let graphData = $derived<Record<string, Record<string, string | number>[]>>({});
	let graphTotals = $derived<Record<string, number>>({});
	let showFilters = $state(false);
	let rightSidebar = $state<HTMLDialogElement>();

	const usersMap = new SvelteMap<string, OrgUser>([]);
	const usersAsArray = $derived(usersMap.values().toArray());

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
				`${(data.averageResponseTimeMs as number).toFixed(2)}ms avg • ${data.serverDisplayName}`,
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
				`${(data.processingTimeMs as number).toFixed(2)}ms • ${data.serverDisplayName}`,
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
			formatTooltipText: (data) => {
				const user = usersAsArray.find((u) => u.id === data.userId);
				return `${data.callCount} calls • ${userDisplayName(user)}`;
			},
			formatXLabel: (userId) => {
				const user = usersAsArray.find((u) => u.id === userId);
				return userDisplayName(user);
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
		const isSpecificServer = mcpId;
		if (isSpecificServer) {
			// Remove server comparison graphs when viewing a specific server
			return graphConfigs.filter(
				(cfg) =>
					cfg.id !== 'most-frequently-used-servers' && cfg.id !== 'tool-call-errors-by-server'
			);
		}
		return graphConfigs;
	});

	afterNavigate(() => {
		AdminService.listUsersIncludeDeleted().then((userData) => {
			for (const user of userData) {
				usersMap.set(user.id, user);
			}
		});
	});

	$effect(() => {
		if (mcpId || filters) reload();
	});

	$effect(() => {
		if (!listUsageStats) return;
		showLoadingSpinner = true;

		updateGraphs().then(() => {
			showLoadingSpinner = false;
		});
	});

	async function reload() {
		listUsageStats = mcpId
			? AdminService.listServerOrInstanceAuditLogStats(mcpId, {
					start_time: filters.start_time,
					end_time: filters.end_time
				})
			: AdminService.listAuditLogUsageStats({
					...filters
				});
	}

	function userDisplayName(user?: OrgUser): string {
		if (!user) {
			return 'Unknown';
		}

		let display = user.originalEmail || user.email || user.id || 'Unknown';
		if (user.deletedAt) {
			display += ' (Deleted)';
		}
		return display;
	}

	afterNavigate(() => {
		AdminService.listUsersIncludeDeleted().then((userData) => {
			for (const user of userData) {
				usersMap.set(user.id, user);
			}
		});
	});

	async function updateGraphs() {
		const stats = await listUsageStats;
		const data: Record<string, Record<string, string | number>[]> = {};
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
		setTimeout(() => {
			showFilters = false;
		}, 300);
	}

	function hasData(graphConfigs: GraphConfig[]) {
		return graphConfigs.some((cfg) => graphTotals[cfg.id] ?? 0 > 0);
	}

	function getFilterDisplayLabel(key: string) {
		const _key = key as SupportedStateFilter;

		if (_key === 'mcp_server_display_names') return 'Server';
		if (_key === 'mcp_server_catalog_entry_names') return 'Server Catalog Entry Name';
		if (_key === 'mcp_id') return 'Server ID';
		if (_key === 'start_time') return 'Start Time';
		if (_key === 'end_time') return 'End Time';
		if (_key === 'user_ids') return 'User';

		return key.replace(/_(\w)/g, ' $1');
	}

	function getFilterValue(label: SupportedStateFilter, value: string | number) {
		if (label === 'start_time' || label === 'end_time') {
			return new Date(value).toLocaleString(undefined, {
				year: 'numeric',
				month: 'short',
				day: 'numeric',
				hour: '2-digit',
				minute: '2-digit',
				second: '2-digit',
				hour12: true,
				timeZoneName: 'short'
			});
		}

		if (label === 'user_ids') {
			const hasConflict = (display?: string) => {
				const isConflicted = usersAsArray.some(
					(user) =>
						user.id !== value && display && getUserDisplayName(usersMap, user.id) === display
				);

				return isConflicted;
			};

			return getUserDisplayName(usersMap, value + '', hasConflict);
		}

		return value + '';
	}

	function handleDateChange({ start, end }: DateRange) {
		const url = page.url;

		if (start) {
			url.searchParams.set('start_time', start.toISOString());

			if (end) {
				url.searchParams.set('end_time', end.toISOString());
			} else {
				const end = endOfDay(start);
				url.searchParams.set('end_time', end.toISOString());
			}
		}

		goto(url.toString(), { noScroll: true });
	}

	function isSafe<T = unknown>(value: T) {
		return value !== undefined && value !== null;
	}
</script>

{#if showLoadingSpinner}
	<div
		class="absolute inset-0 z-10 flex items-center justify-center"
		in:fade={{ duration: 100 }}
		out:fade|global={{ duration: 300, delay: 500 }}
	>
		<div
			class="bg-surface3/50 border-surface3 flex flex-col items-center gap-4 rounded-2xl border px-16 py-8 text-blue-500 shadow-md backdrop-blur-[1px] dark:text-blue-500"
		>
			<Loading class="size-32 stroke-1" />
			<div class="text-2xl font-semibold">Loading stats...</div>
		</div>
	</div>
{/if}

<div class="flex flex-col gap-8">
	<div class="flex flex-col">
		<div class="flex w-full justify-end gap-4">
			<AuditLogCalendar
				start={timeRangeFilters.startTime}
				end={timeRangeFilters.endTime}
				onChange={handleDateChange}
			/>

			{#if !mcpId}
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
	</div>

	{@render filtersPill()}

	<!-- Summary with filter button -->
	<div class="flex items-center justify-between gap-4">
		<div class="flex-1">
			<StatBar startTime={filters?.start_time ?? ''} endTime={filters?.end_time ?? ''} />
		</div>
	</div>

	{#if !showLoadingSpinner && !hasData(filteredGraphConfigs)}
		<div class="mt-12 flex w-md flex-col items-center gap-4 self-center text-center">
			<ChartBarDecreasing class="size-24 text-gray-200 dark:text-gray-900" />
			<h4 class="text-lg font-semibold text-gray-400 dark:text-gray-600">No usage stats</h4>
			<p class="w-sm text-sm font-light text-gray-400 dark:text-gray-600">
				Currently, there are no usage stats for the range or selected filters. Try modifying your
				search criteria or try again later.
			</p>
		</div>
	{:else if !showLoadingSpinner}
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

					<div class="h-[300px] min-h-[300h]">
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
						{:else if !showLoadingSpinner}
							<div
								class="flex h-[300px] items-center justify-center text-sm font-light text-gray-400 dark:text-gray-600"
							>
								No data available
							</div>
						{/if}
					</div>

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
	{/if}
</div>

<dialog
	bind:this={rightSidebar}
	use:clickOutside={[handleRightSidebarClose, true]}
	use:dialogAnimation={{ type: 'drawer' }}
	class="dark:border-surface1 dark:bg-surface1 fixed! top-0! right-0! bottom-0! left-auto! z-40 h-screen w-auto max-w-none rounded-none border-0 bg-white shadow-lg outline-none!"
>
	{#if showFilters}
		<FiltersDrawer
			onClose={handleRightSidebarClose}
			filters={auditLogsSlideoverFilters}
			{getFilterDisplayLabel}
			getUserDisplayName={(...args) => getUserDisplayName(usersMap, ...args)}
			isFilterDisabled={(filterId) => propsFiltersKeys.has(filterId)}
			isFilterClearable={(filterId) => !propsFiltersKeys.has(filterId)}
			endpoint={async (filterId: string, ...args) => {
				const proxyFilterId = proxy.get(filterId as SupportedStateFilter) ?? filterId;
				return AdminService.listAuditLogFilterOptions(proxyFilterId, ...args);
			}}
		/>
	{/if}
</dialog>

{#snippet filtersPill()}
	{@const entries = Object.entries(pillsSearchParamFilters)}
	{@const filterEntries = entries.filter(([, value]) => !!value) as [
		SupportedStateFilter,
		string | number | null
	][]}
	{@const hasFilters = !!filterEntries.length}

	{#if hasFilters}
		<div
			class="flex flex-wrap items-center gap-2"
			in:slide={{ duration: 100 }}
			out:slide={{ duration: 50 }}
		>
			{#each filterEntries as [filterKey, filterValues] (filterKey)}
				{@const displayLabel = getFilterDisplayLabel(filterKey)}
				{@const values = filterValues?.toString().split(',').filter(Boolean) ?? []}
				{@const isClearable = Object.keys(propsFilters).every((d) => d !== filterKey)}

				<div
					class="flex items-center gap-1 rounded-lg border border-blue-500/50 bg-blue-500/10 px-4 py-2 text-blue-600 dark:text-blue-300"
					animate:flip={{ duration: 100 }}
				>
					<div class="text-xs font-semibold">
						<span>{displayLabel}</span>
						<span>:</span>
						{#each values as value (value)}
							{@const isMultiple = values.length > 1}

							{#if isMultiple}
								<span class="font-light">
									<span>{getFilterValue(filterKey, value)}</span>
								</span>

								<span class="mx-1 font-bold last:hidden">OR</span>
							{:else}
								<span class="font-light">{getFilterValue(filterKey, value)}</span>
							{/if}
						{/each}
					</div>

					{#if isClearable}
						<button
							class="rounded-full p-1 transition-colors duration-200 hover:bg-blue-500/25"
							onclick={() => {
								const url = page.url;
								url.searchParams.set(filterKey, '');

								goto(url, { noScroll: true });
							}}
						>
							<X class="size-3" />
						</button>
					{/if}
				</div>
			{/each}
		</div>
	{/if}
{/snippet}
