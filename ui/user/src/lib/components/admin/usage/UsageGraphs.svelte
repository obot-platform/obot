<script lang="ts">
	import { Funnel, ChartBarDecreasing, X } from 'lucide-svelte';
	import {
		AdminService,
		type AuditLogURLFilters,
		type AuditLogUsageStats,
		type OrgUser,
		type UsageStatsFilters
	} from '$lib/services';
	import StatBar from '../StatBar.svelte';
	import { SvelteMap } from 'svelte/reactivity';
	import { afterNavigate } from '$app/navigation';
	import { goto } from '$lib/url';
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
	import StackedBarsChart, {
		type StackTooltipArg,
		type TooltipArg
	} from '$lib/components/charts/StackedBarsChart.svelte';
	import type { Snippet } from 'svelte';

	type Props = {
		mcpId?: string | null;
		mcpServerDisplayName?: string | null;
		mcpServerCatalogEntryName?: string | null;
	};

	type GraphDataItem = {
		createdAt: Date;
		mcpID: string;
		mcpServerDisplayName: string;
		mcpServerCatalogEntryName?: string;
		toolName: string;
		userID: string;
		processingTimeMs: number;
		responseStatus: number;
		error: string;
	};

	type GraphConfig = {
		id: string;
		label: string;
		dateAccessor: (d: GraphDataItem) => Date;
		categoryAccessor: (d: GraphDataItem) => string;
		groupAccessor?: (d: GraphDataItem[]) => number;
		formatYLabel?: (d: GraphDataItem) => string;
		segmentTooltip?: Snippet<[TooltipArg]>;
		stackTooltip?: Snippet<[StackTooltipArg]>;
	};

	type GraphConfigWithRenderer = GraphConfig & {
		renderer: Snippet<[{ data: GraphDataItem[]; config: GraphConfig }]>;
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
	let showFilters = $state(false);
	let rightSidebar = $state<HTMLDivElement>();

	const usersMap = new SvelteMap<string, OrgUser>([]);
	const usersAsArray = $derived(usersMap.values().toArray());

	let stats = $state<AuditLogUsageStats>();

	const graphData = $derived.by(() => {
		const array: GraphDataItem[] = [];

		for (const s of stats?.items ?? []) {
			for (const call of s.toolCalls ?? []) {
				for (const item of call.items ?? []) {
					if (item.userID) {
						array.push({
							createdAt: new Date(item.createdAt),
							mcpID: s.mcpID,
							mcpServerDisplayName: s.mcpServerDisplayName,
							mcpServerCatalogEntryName:
								'mcpServerCatalogEntryName' in s
									? (s['mcpServerCatalogEntryName'] as string)
									: undefined,
							toolName: call.toolName,
							userID: item.userID,
							processingTimeMs: item.processingTimeMs,
							responseStatus: item.responseStatus,
							error: item.error
						});
					}
				}
			}
		}

		return array.sort((a, b) => (a.createdAt as Date).getTime() - (b.createdAt as Date).getTime());
	});

	let responseTimeView = $state<'average' | 'individual'>('average');

	const graphConfigs: GraphConfigWithRenderer[] = [
		{
			id: 'most-frequent-tool-calls',
			label: 'Most Frequent Tool Calls',
			dateAccessor: (d) => d.createdAt,
			categoryAccessor: (d) => d.toolName,
			groupAccessor: (d) => {
				return d.length;
			},
			segmentTooltip: unifiedTooltip,
			renderer: defaultChartSnippet
		},
		{
			id: 'most-frequently-used-servers',
			label: 'Most Frequently Used Servers',
			dateAccessor: (d) => d.createdAt,
			categoryAccessor: (d) => d.mcpServerDisplayName,
			groupAccessor: (d) => {
				return d.length;
			},
			segmentTooltip: unifiedTooltip,
			renderer: defaultChartSnippet
		},
		{
			id: 'tool-call-average-response-time',
			label: 'Tool Call Response Time',
			dateAccessor: (d) => d.createdAt,
			categoryAccessor: (d) => d.toolName,
			groupAccessor: (d) => {
				if (responseTimeView === 'individual') {
					return d.reduce((acc, item) => acc + (item.processingTimeMs as number), 0);
				}

				return d.reduce((acc, item) => acc + (item.processingTimeMs as number), 0) / d.length;
			},
			segmentTooltip: toolCallResponseTimeTooltip,
			renderer: toolCallResponseTimeGraph
		},
		{
			id: 'tool-call-errors',
			label: 'Tool Call Errors',
			dateAccessor: (d) => d.createdAt,
			categoryAccessor: (d) => d.toolName,
			groupAccessor: (d) => d.filter((d) => d.responseStatus >= 400).length,
			segmentTooltip: unifiedTooltip,
			renderer: defaultChartSnippet
		},
		{
			id: 'tool-call-errors-by-server',
			label: 'Tool Call Errors by Server',
			dateAccessor: (d) => d.createdAt,
			categoryAccessor: (d) => d.mcpServerDisplayName,
			groupAccessor: (d) => d.filter((d) => d.responseStatus >= 400).length,
			segmentTooltip: unifiedTooltip,
			renderer: defaultChartSnippet
		},
		{
			id: 'most-active-users',
			label: 'Most Active Users',
			dateAccessor: (d) => d.createdAt,
			categoryAccessor: (d) => d.userID,
			groupAccessor: (d) => d.length,
			segmentTooltip: usersTooltip,
			stackTooltip: usersStackTooltip,
			renderer: defaultChartSnippet
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

	afterNavigate(() => {
		AdminService.listUsersIncludeDeleted().then((userData) => {
			for (const user of userData) {
				usersMap.set(user.id, user);
			}
		});
	});

	async function updateGraphs() {
		stats = await listUsageStats;
	}

	function handleRightSidebarClose() {
		rightSidebar?.hidePopover();
		showFilters = false;
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

{#snippet unifiedTooltip(arg: TooltipArg, unit: string | undefined = undefined)}
	{@const typeOfValue = typeof arg.value}

	<div class="flex flex-col gap-2">
		<div class="flex flex-col gap-0">
			{#if arg?.date}
				{@const formattedDate = new Date(arg.date).toLocaleString(undefined, {
					year: 'numeric',
					month: 'short',
					day: 'numeric',
					hour: '2-digit',
					minute: '2-digit'
				})}
				<div class="text-xs opacity-70">
					{formattedDate}
				</div>
			{/if}
			{#if arg?.category}
				<div class="text-sm font-semibold">
					{arg.category}
				</div>
			{/if}
		</div>
		{#if arg?.value || typeof arg.value === 'number'}
			<div class="text-sm font-medium">
				<span>
					{#if typeOfValue === 'number'}
						{arg.value.toLocaleString()}
					{:else}
						{arg.value}
					{/if}
				</span>

				{#if unit}
					<span>{unit}</span>
				{/if}
			</div>
		{/if}
	</div>
{/snippet}

{#snippet toolCallResponseTimeTooltip(arg: TooltipArg)}
	{@render unifiedTooltip(arg, 'ms')}
{/snippet}

{#snippet usersTooltip(arg: TooltipArg)}
	{@const displayName = getUserDisplayName(usersMap, String(arg.category))}

	<div class="flex flex-col gap-2">
		<div class="flex flex-col gap-0">
			{#if arg?.date}
				{@const formattedDate = new Date(arg.date).toLocaleString(undefined, {
					year: 'numeric',
					month: 'short',
					day: 'numeric',
					hour: '2-digit',
					minute: '2-digit'
				})}
				<div class="text-xs opacity-70">
					{formattedDate}
				</div>
			{/if}
			{#if arg?.category}
				<div class="text-sm font-semibold">
					{displayName}
				</div>
			{/if}
		</div>
		{#if arg?.value || typeof arg.value === 'number'}
			<div class="text-sm font-medium">
				<span>
					{arg.value.toLocaleString()}
				</span>
			</div>
		{/if}
	</div>
{/snippet}

{#snippet usersStackTooltip(arg: StackTooltipArg)}
	<div class="flex flex-col gap-2">
		{#if arg?.date}
			<div class="text-xs">
				{arg.date.toLocaleDateString(undefined, {
					year: 'numeric',
					month: 'short',
					day: 'numeric',
					hour: '2-digit',
					minute: '2-digit'
				})}
			</div>
		{/if}
		<div class="text-base-content/50 flex flex-col gap-1">
			{#each arg.segments as segment (segment.category)}
				{@const userDisplayName = getUserDisplayName(usersMap, segment.category)}
				{@const items = (segment.group ?? []) as GraphDataItem[]}
				{@const errorCount = items.filter((item) => Boolean(item.error)).length}
				{@const avgResponseTime =
					items.length > 0
						? items.reduce((s, i) => s + (i.processingTimeMs ?? 0), 0) / items.length
						: 0}

				<div class="flex flex-col gap-1">
					<div class="flex items-center gap-2">
						<div class="h-3 w-3 rounded-sm" style="background-color: {segment.color}"></div>
						<div class="text-base-content text-sm font-semibold">{userDisplayName}</div>
						<div class="ml-auto font-semibold">
							<span class="text-base-content">{segment.value.toLocaleString()}</span> calls
						</div>
					</div>
					<div class="ml-5 text-xs">
						Avg Response: <span class="text-base-content"
							>{Math.round(avgResponseTime).toLocaleString()}ms</span
						>
						{#if errorCount > 0}
							| Errors: <span class="text-error">{errorCount.toLocaleString()}</span>
						{/if}
					</div>
				</div>
			{/each}
			<div class="mt-1 flex items-center gap-2 border-t pt-1">
				<div class="text-sm font-semibold">Total</div>
				<div class="ml-auto text-lg font-bold">
					<span class="text-base-content">{arg.total.toLocaleString()}</span> calls
				</div>
			</div>
		</div>
	</div>
{/snippet}

{#snippet defaultChartSnippet({
	data = [],
	config,
	views
}: {
	data: GraphDataItem[];
	config: GraphConfig;
	views?: Snippet;
})}
	<div
		class="dark:bg-surface1 dark:border-surface3 bg-background rounded-md border border-transparent p-6 shadow-sm"
	>
		<div class="flex justify-between gap-4">
			<h3 class="mb-4 text-lg font-semibold">{config.label}</h3>
			{#if views}
				{@render views()}
			{/if}
		</div>

		<div class="flex h-[300px] flex-col">
			{#if data.length > 0}
				<StackedBarsChart
					start={new Date(filters.start_time)}
					end={new Date(filters.end_time)}
					{data}
					padding={{ top: 16, right: 16, bottom: 32, left: 32 }}
					dateAccessor={config.dateAccessor}
					categoryAccessor={config.categoryAccessor}
					groupAccessor={config.groupAccessor ?? ((d) => d.length)}
					segmentTooltip={config.segmentTooltip}
					stackTooltip={config.stackTooltip}
				/>
			{:else if !showLoadingSpinner}
				<div class="text-on-surface1 flex h-[300px] items-center justify-center text-sm font-light">
					No data available
				</div>
			{/if}
		</div>
	</div>
{/snippet}

{#snippet toolCallReponseTimeViews()}
	<div class="flex gap-2">
		<button
			class="button h-8 px-3 py-1 text-sm"
			class:button-primary={responseTimeView === 'average'}
			onclick={() => (responseTimeView = 'average')}
		>
			Average
		</button>
		<button
			class="button h-8 px-3 py-1 text-sm"
			class:button-primary={responseTimeView === 'individual'}
			onclick={() => (responseTimeView = 'individual')}
		>
			Individual
		</button>
	</div>
{/snippet}

{#snippet toolCallResponseTimeGraph({
	data,
	config
}: {
	data: GraphDataItem[];
	config: GraphConfig;
})}
	{@render defaultChartSnippet({
		data,
		config,
		views: toolCallReponseTimeViews
	})}
{/snippet}

{#if showLoadingSpinner}
	<div
		class="absolute inset-0 z-10 flex items-center justify-center"
		in:fade={{ duration: 100 }}
		out:fade|global={{ duration: 300, delay: 500 }}
	>
		<div
			class="bg-surface3/50 border-surface3 text-primary flex flex-col items-center gap-4 rounded-2xl border px-16 py-8 shadow-md backdrop-blur-[1px]"
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
					class="hover:bg-surface1 dark:bg-surface1 dark:hover:bg-surface3 dark:border-surface3 button bg-background flex h-12 w-fit items-center justify-center gap-1 rounded-lg border border-transparent shadow-sm"
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

	<!-- Summary with filter button -->
	<div class="flex items-center justify-between gap-4">
		<div class="flex-1">
			<StatBar startTime={filters?.start_time ?? ''} endTime={filters?.end_time ?? ''} />
		</div>
	</div>

	{#if !showLoadingSpinner && !graphData.length}
		<div class="mt-12 flex w-md flex-col items-center gap-4 self-center text-center">
			<ChartBarDecreasing class="text-on-surface1 size-24 opacity-50" />
			<h4 class="text-on-surface1 text-lg font-semibold">No usage stats</h4>
			<p class="text-on-surface1 w-sm text-sm font-light">
				Currently, there are no usage stats for the range or selected filters. Try modifying your
				search criteria or try again later.
			</p>
		</div>
	{:else if !showLoadingSpinner}
		<div class="grid grid-cols-1 gap-8 lg:grid-cols-2">
			{#each filteredGraphConfigs as cfg (cfg.id)}
				{@render cfg.renderer({
					data: $state.snapshot(graphData),
					config: {
						id: cfg.id,
						label: cfg.label,
						dateAccessor: cfg.dateAccessor,
						categoryAccessor: cfg.categoryAccessor,
						groupAccessor: cfg.groupAccessor,
						segmentTooltip: cfg.segmentTooltip,
						stackTooltip: cfg.stackTooltip
					}
				})}
			{/each}
		</div>
	{/if}
</div>

<div bind:this={rightSidebar} popover class="drawer md:w-lg lg:w-xl">
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
				{@const isClearable = Object.keys(propsFilters).every((d) => d !== filterKey)}

				<div
					class="border-primary/50 bg-primary/10 text-primary flex items-center gap-1 rounded-lg border px-4 py-2"
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
							class="hover:bg-primary/25 rounded-full p-1 transition-colors duration-200"
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
