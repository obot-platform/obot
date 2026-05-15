<script lang="ts">
	import { afterNavigate } from '$app/navigation';
	import { page } from '$app/state';
	import type { DateRange } from '$lib/components/Calendar.svelte';
	import IconButton from '$lib/components/primitives/IconButton.svelte';
	import Loading from '$lib/icons/Loading.svelte';
	import {
		AdminService,
		type AuditLogURLFilters,
		type AuditLogUsageStats,
		type OrgUser,
		type UsageStatsFilters
	} from '$lib/services';
	import profile from '$lib/stores/profile.svelte';
	import { goto } from '$lib/url';
	import { getUserDisplayName, isBasicUser } from '$lib/utils';
	import HorizontalBarGraph from '../../graph/HorizontalBarGraph.svelte';
	import StatBar from '../StatBar.svelte';
	import AuditLogCalendar from '../audit-logs/AuditLogCalendar.svelte';
	import FiltersDrawer from '../filters-drawer/FiltersDrawer.svelte';
	import type { SupportedStateFilter } from './types';
	import {
		transformAvgToolCallResponseTime,
		transformTopServerUsage,
		transformTopToolCalls
	} from './utils';
	import { endOfDay, isBefore, set, subDays } from 'date-fns';
	import { ChevronsLeft, ChevronsRight, Funnel, ChartBarDecreasing, X } from 'lucide-svelte';
	import { flip } from 'svelte/animate';
	import { SvelteMap } from 'svelte/reactivity';
	import { fade, slide } from 'svelte/transition';

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

	// Enforced filters for Basic users - they can only see their own usage stats
	const enforcedFilters = $derived.by(() => {
		if (isBasicUser(profile.current.groups) && profile.current?.id) {
			return { user_ids: profile.current.id };
		}
		return {};
	});

	const enforcedFiltersKeys = $derived(new Set(Object.keys(enforcedFilters)));

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
			...propsFilters,
			...enforcedFilters
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

		return { ...clone, ...propsFilters, ...enforcedFilters };
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
		...enforcedFilters,
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
	let rightSidebar = $state<HTMLDivElement>();

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
			transform: transformTopToolCalls
		},
		{
			id: 'most-frequently-used-servers',
			label: 'Most Frequently Used Servers',
			xKey: 'serverName',
			yKey: 'count',
			tooltip: 'calls',
			transform: transformTopServerUsage
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
			transform: transformAvgToolCallResponseTime
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
				// eslint-disable-next-line svelte/prefer-svelte-reactivity
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
				// eslint-disable-next-line svelte/prefer-svelte-reactivity
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
				// eslint-disable-next-line svelte/prefer-svelte-reactivity
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
		rightSidebar?.hidePopover();
		showFilters = false;
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

		goto(url, { noScroll: true });
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
			class="bg-base-400/50 border-base-400 text-primary flex flex-col items-center gap-4 rounded-2xl border px-16 py-8 shadow-md backdrop-blur-[1px]"
		>
			<Loading class="size-32 stroke-1" />
			<div class="text-2xl font-semibold">Loading stats...</div>
		</div>
	</div>
{/if}

<div class="flex flex-col gap-8">
	<div class="flex gap-4 justify-between">
		<!-- Summary with filter button -->
		<div class="flex shrink-0 items-center justify-between gap-4">
			<div class="flex-1">
				<StatBar startTime={filters?.start_time ?? ''} endTime={filters?.end_time ?? ''} />
			</div>
		</div>

		<div class="flex w-full justify-end gap-4">
			<AuditLogCalendar
				start={timeRangeFilters.startTime}
				end={timeRangeFilters.endTime}
				onChange={handleDateChange}
			/>

			{#if !mcpId}
				<button
					class="btn btn-neutral h-12.5"
					onclick={() => {
						showFilters = true;
						rightSidebar?.showPopover();
					}}
				>
					<Funnel class="size-4" />
					Filters
				</button>
			{/if}
		</div>
	</div>

	{@render filtersPill()}

	{#if !showLoadingSpinner && !hasData(filteredGraphConfigs)}
		<div class="mt-12 flex w-md flex-col items-center gap-4 self-center text-center">
			<ChartBarDecreasing class="text-muted-content size-24 opacity-50" />
			<h4 class="text-muted-content text-lg font-semibold">No usage stats</h4>
			<p class="text-muted-content w-sm text-sm font-light">
				Currently, there are no usage stats for the range or selected filters. Try modifying your
				search criteria or try again later.
			</p>
		</div>
	{:else if !showLoadingSpinner}
		<div class="mb-8 grid grid-cols-1 gap-8 lg:grid-cols-2">
			{#each filteredGraphConfigs as cfg (cfg.id)}
				{@const full = graphData[cfg.id] ?? []}
				{@const total = graphTotals[cfg.id] ?? 0}
				{@const page = graphPages[cfg.id] ?? 0}
				{@const maxPage = Math.max(0, Math.ceil(total / graphPageSize) - 1)}
				{@const paginated = full.slice(page * graphPageSize, (page + 1) * graphPageSize)}

				<div class="paper p-0">
					<h3
						class="text-xs uppercase font-medium tracking-wider border-b dark:border-base-400 border-base-300 px-4 py-2 rounded-t-md"
					>
						{cfg.label}
					</h3>

					<div class="p-4">
						<div class="text-muted-content h-[300px] min-h-[300px]">
							{#if paginated.length > 0}
								<HorizontalBarGraph
									data={paginated}
									labelKey={cfg.xKey}
									valueKey={cfg.yKey}
									formatLabel={cfg.formatXLabel}
									formatValue={(value) => Math.round(value).toString()}
								>
									{#snippet tooltipContent(item)}
										<div class="flex flex-col gap-0 text-xs">
											<div class="text-muted-content text-xs">{item.label}</div>
										</div>
										<div class="text-base-content font-semibold">
											{cfg.formatTooltipText
												? cfg.formatTooltipText(item.row as Record<string, string | number>)
												: `${item.value} ${cfg.tooltip}`}
										</div>
									{/snippet}
								</HorizontalBarGraph>
							{:else if !showLoadingSpinner}
								<div
									class="text-muted-content flex h-[300px] items-center justify-center text-sm font-light"
								>
									No data available
								</div>
							{/if}
						</div>

						{#if maxPage > 0}
							<div
								class="mt-4 flex items-center justify-center gap-4 border-t border-base-400 p-4 pb-0"
							>
								<IconButton
									onclick={() => setGraphPage(cfg.id, Math.max(0, page - 1))}
									disabled={page === 0}
									tooltip={{ text: 'Previous Page' }}
								>
									<ChevronsLeft class="size-5" />
								</IconButton>
								<span class="text-sm">
									Page {page + 1} of {maxPage + 1}
									(showing {Math.min(graphPageSize, total - page * graphPageSize)} of {total} items)
								</span>
								<IconButton
									onclick={() => setGraphPage(cfg.id, Math.min(maxPage, page + 1))}
									disabled={page >= maxPage}
									tooltip={{ text: 'Next Page' }}
								>
									<ChevronsRight class="size-5" />
								</IconButton>
							</div>
						{/if}
					</div>
				</div>
			{/each}
		</div>
	{/if}
</div>

<div bind:this={rightSidebar} popover class="drawer-legacy md:w-lg lg:w-xl">
	{#if showFilters}
		<FiltersDrawer
			onClose={handleRightSidebarClose}
			filters={auditLogsSlideoverFilters}
			{getFilterDisplayLabel}
			getUserDisplayName={(...args) => getUserDisplayName(usersMap, ...args)}
			isFilterDisabled={(filterId) =>
				propsFiltersKeys.has(filterId) || enforcedFiltersKeys.has(filterId)}
			isFilterClearable={(filterId) =>
				!propsFiltersKeys.has(filterId) && !enforcedFiltersKeys.has(filterId)}
			endpoint={async (filterId: string, ...args) => {
				const proxyFilterId = proxy.get(filterId as SupportedStateFilter) ?? filterId;
				return AdminService.listAuditLogFilterOptions(proxyFilterId, ...args);
			}}
		/>
	{/if}
</div>

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
				{@const isClearable =
					!propsFiltersKeys.has(filterKey) && !enforcedFiltersKeys.has(filterKey)}

				<div class="filter-primary" animate:flip={{ duration: 100 }}>
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
