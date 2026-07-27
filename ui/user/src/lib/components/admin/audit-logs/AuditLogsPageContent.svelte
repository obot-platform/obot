<script lang="ts">
	import { afterNavigate } from '$app/navigation';
	import { page } from '$app/state';
	import { columnResize } from '$lib/actions/resize';
	import { buildPillSearchParamFilters, buildSearchParamFiltersArray } from '$lib/auditlogs';
	import { type DateRange } from '$lib/components/Calendar.svelte';
	import DotDotDot from '$lib/components/DotDotDot.svelte';
	import Search from '$lib/components/Search.svelte';
	import AuditLogEventDetails from '$lib/components/admin/audit-logs/AuditLogEventDetails.svelte';
	import StackedTimeline from '$lib/components/graph/StackedTimeline.svelte';
	import { setVirtualPageData } from '$lib/components/ui/virtual-page/context';
	import Loading from '$lib/icons/Loading.svelte';
	import { localState } from '$lib/runes/localState.svelte';
	import {
		type OrgUser,
		type AuditLogURLFilters,
		AdminService,
		type AuditLogEvent,
		Group,
		UserService
	} from '$lib/services';
	import type { PaginatedResponse } from '$lib/services/http';
	import { responsive } from '$lib/stores';
	import profile from '$lib/stores/profile.svelte';
	import { goto, replaceState } from '$lib/url';
	import { getUserDisplayName, isBasicUser } from '$lib/utils';
	import FiltersDrawer from '../filters-drawer/FiltersDrawer.svelte';
	import AuditLogCalendar from './AuditLogCalendar.svelte';
	import AuditLogFilterPills from './AuditLogFilterPills.svelte';
	import AuditLogTableSkeleton from './AuditLogTableSkeleton.svelte';
	import AuditLogsTable from './AuditLogsTable.svelte';
	import {
		aggregateAuditLogsByBucket,
		toAuditLogTimelineChartRow,
		type AuditLogTimelineChartRow
	} from './timelineUtils';
	import { ChevronLeft, ChevronRight, Funnel, Captions, Plus, Settings } from '@lucide/svelte';
	import { set, endOfDay, isBefore, subDays } from 'date-fns';
	import { debounce } from 'es-toolkit';
	import type { Snippet } from 'svelte';
	import { SvelteMap } from 'svelte/reactivity';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		mcpId?: string | null;
		mcpServerDisplayName?: string | null;
		mcpServerCatalogEntryName?: string | null;
		emptyContent?: Snippet;
		entity?: 'workspace' | 'catalog';
	}

	let { mcpServerDisplayName, mcpServerCatalogEntryName, mcpId, emptyContent }: Props = $props();

	let auditLogsResponse = $state<PaginatedResponse<AuditLogEvent>>();
	const auditLogsTotalItems = $derived(auditLogsResponse?.total ?? 0);

	let pageIndexLocal = localState('@obot/auditlogs/page-index', 0, {
		parse: (ls) => (typeof ls === 'string' ? parseInt(ls) : (ls ?? 0))
	});
	const pageIndex = $derived(pageIndexLocal.current ?? 0);
	const pageLimit = $state(10000);

	const numberOfPages = $derived(Math.ceil(auditLogsTotalItems / pageLimit));
	const pageOffset = $derived(pageIndex * pageLimit);

	const remoteAuditLogs = $derived(auditLogsResponse?.items ?? []);

	/** When there are more than this many results, defer timeline aggregation and table data to keep UI responsive. */
	const DEFER_THRESHOLD = 500;
	let displayTimelineData = $state<AuditLogTimelineChartRow[]>([]);
	let displayTableData = $state<AuditLogEvent[]>([]);

	$effect(() => {
		const items = remoteAuditLogs;
		const start = timeRangeFilters.startTime;
		const end = timeRangeFilters.endTime;
		const threshold = DEFER_THRESHOLD;

		if (items.length <= threshold) {
			displayTableData = items;
			displayTimelineData = items.map(toAuditLogTimelineChartRow);
			return;
		}

		displayTableData = [];
		displayTimelineData = [];
		if (typeof requestIdleCallback !== 'undefined') {
			const id = requestIdleCallback(
				() => {
					displayTableData = items;
					displayTimelineData = aggregateAuditLogsByBucket(items, start, end);
				},
				{ timeout: 200 }
			);
			return () => cancelIdleCallback(id);
		}
		const id = setTimeout(() => {
			displayTableData = items;
			displayTimelineData = aggregateAuditLogsByBucket(items, start, end);
		}, 0);
		return () => clearTimeout(id);
	});

	$effect(() => setVirtualPageData(displayTableData));

	const isTimelineAggregated = $derived(
		displayTimelineData.length > 0 && 'count' in (displayTimelineData[0] as Record<string, unknown>)
	);

	const isReachedMax = $derived(pageIndex >= numberOfPages - 1);
	const isReachedMin = $derived(pageIndex <= 0);

	const users = new SvelteMap<string, OrgUser>();

	let showLoadingSpinner = $state(true);
	let showFilters = $state(false);
	let selectedAuditLog = $state<AuditLogEvent & { user: string }>();
	let rightSidebar = $state<HTMLDivElement>();
	let showFilterConfirmDialog = $state(false);
	let pendingExportType = $state<'export' | 'scheduled' | null>(null);

	// Enforced filters for Basic users - they can only see their own audit logs. The unified `actor`
	// filter matches user_id (or device_id), so pinning it to the user's own id restricts them to
	// their own activity.
	const enforcedFilters = $derived.by(() => {
		if (isBasicUser(profile.current.groups) && profile.current?.id) {
			return { actor: profile.current.id };
		}
		return {};
	});

	const enforcedFiltersKeys = $derived(new Set(Object.keys(enforcedFilters)));

	type SupportedFilter = keyof AuditLogURLFilters;
	const unifiedFilters: SupportedFilter[] = [
		'event_type',
		'actor',
		'operation',
		'mcp_server',
		'tool',
		'outcome',
		'client',
		'duration'
	];
	// When no Source (event_type) is selected, the backend returns every source the caller is
	// authorized to read — both MCP and local-agent for admins/auditors, MCP only otherwise. The UI
	// intentionally does not force a default so "no Source filter" means "all sources I can see".
	const selectedEventTypes = $derived(page.url.searchParams.get('event_type') ?? '');

	// In a server-scoped embedded view the MCP server is fixed, so the MCP Server filter is redundant.
	const isServerScoped = $derived(
		Boolean(mcpId || mcpServerDisplayName || mcpServerCatalogEntryName)
	);

	const forcedEventType = $derived(isServerScoped ? 'mcp_call' : '');
	const visibleFilterKeys = $derived(
		unifiedFilters.filter(
			(key) => !(isServerScoped && (key === 'mcp_server' || key === 'event_type'))
		)
	);

	// supportedFilters also carries the time-range params so they are read from the URL; the drawer
	// itself only renders unifiedFilters (time is handled by the calendar).
	const supportedFilters: SupportedFilter[] = [...unifiedFilters, 'start_time', 'end_time'];

	// Duration filter presets (client-side). Each value encodes a millisecond range "min-max"; an
	// empty bound is unbounded. Translated to processing_time_min/max before querying the backend.
	const DURATION_BUCKETS: { value: string; label: string }[] = [
		{ value: '0-1000', label: '< 1s' },
		{ value: '1000-5000', label: '1–5s' },
		{ value: '5000-30000', label: '5–30s' },
		{ value: '30000-', label: '> 30s' }
	];
	function durationBucketLabel(value: string): string {
		return DURATION_BUCKETS.find((bucket) => bucket.value === value)?.label ?? value;
	}
	function durationToProcessingParams(value: string): Partial<AuditLogURLFilters> {
		const [minRaw, maxRaw] = String(value).split('-');
		const params: Partial<AuditLogURLFilters> = {};
		const min = Number(minRaw);
		const max = Number(maxRaw);
		if (minRaw && !Number.isNaN(min) && min > 0) params.processing_time_min = min;
		if (maxRaw && !Number.isNaN(max) && max > 0) params.processing_time_max = max;
		return params;
	}

	// Resolve an actor id to a display name when it is a known Obot user; device ids (and other
	// non-user actors) are shown verbatim rather than as "Unknown User".
	function actorDisplay(actorId: string): string {
		return users.has(actorId) ? getUserDisplayName(users, actorId) : actorId;
	}

	function formatSingleFilterValue(key: string, value: string): string {
		if (key === 'actor') return actorDisplay(value);
		if (key === 'duration') return durationBucketLabel(value);
		if (key === 'outcome' && value) return value.charAt(0).toUpperCase() + value.slice(1);
		if (key === 'event_type') {
			if (value === 'mcp_call') return 'Obot Gateway';
			if (value === 'local_agent_tool_call') return 'Local Agent Hook';
		}
		return value;
	}

	// Default Operation filter applied on first load: focus on the operations that carry a meaningful
	// payload (actual calls/reads/gets) rather than the list/discovery operations. This is only used
	// when the `operation` param is absent from the URL. Clearing the filter sets the param to an
	// empty string (isSafe('') is true), so the default is not re-applied once the user clears it.
	const DEFAULT_OPERATION_FILTER = 'tools/call,resources/read,prompts/get';

	const searchParamsAsArray: [SupportedFilter, string | undefined | null][] = $derived(
		buildSearchParamFiltersArray<AuditLogURLFilters>(supportedFilters, {
			operation: DEFAULT_OPERATION_FILTER
		})
	);

	// Extract supported search params from the URL and convert them to AuditLogURLFilters.
	// This is used to filter the audit logs based on the URL parameters
	const searchParamFilters = $derived.by<AuditLogURLFilters>(() => {
		return searchParamsAsArray.reduce(
			(acc, [key, value]) => {
				acc[key!] = value;
				return acc;
			},
			{} as Record<string, unknown>
		);
	});

	const propsFilters = $derived.by(() => {
		const entries: [key: SupportedFilter, value: string | null | undefined][] = [
			['mcp_server_display_name', mcpServerDisplayName],
			['mcp_server_catalog_entry_name', mcpServerCatalogEntryName],
			['mcp_id', mcpId ?? undefined]
		];

		return (
			entries
				// Filter out undefined values, null values should be kept as they mean the value is specified
				.filter(([, value]) => value !== undefined)
				.reduce((acc, [key, value]) => ((acc[key] = value!), acc), {} as Record<string, unknown>)
		);
	});

	const propsFiltersKeys = $derived(new Set(Object.keys(propsFilters)));

	// Keep only filters with defined values
	const pillsSearchParamFilters = $derived(
		buildPillSearchParamFilters<AuditLogURLFilters>(
			searchParamsAsArray,
			{ ...propsFilters, ...enforcedFilters },
			propsFiltersKeys
		)
	);

	const hasFilterPills = $derived(Object.keys(pillsSearchParamFilters).length > 0);

	const showAuditExportActions = $derived(
		!isServerScoped &&
			(profile.current.groups.includes(Group.ADMIN) || profile.current.groups.includes(Group.OWNER))
	);

	// Filters to be used in the audit logs slideover
	// Exclude filters that are set via props and not undefined
	const auditLogsSlideoverFilters = $derived.by<Partial<AuditLogURLFilters>>(() => {
		const clone = { ...searchParamFilters };

		for (const key of ['start_time', 'end_time']) {
			delete clone[key as keyof AuditLogURLFilters];
		}

		return { ...clone, ...propsFilters, ...enforcedFilters } as Partial<AuditLogURLFilters>;
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
			return set(subDays(endTime, 7), { seconds: 0, milliseconds: 0 });
		};

		const startTime = getStartTime(start_time);

		return {
			startTime,
			endTime
		};
	});

	let query = $derived(page.url.searchParams.get('query') ?? '');

	// Base filters with time filters and query and pagination
	const allFilters = $derived.by(() => {
		// `duration` is a UI-only preset; translate it to the processing_time_min/max params the
		// backend understands and drop the synthetic key before sending the request.
		const { duration, ...rest } = pillsSearchParamFilters as Partial<AuditLogURLFilters>;
		return {
			...rest,
			...propsFilters,
			...(forcedEventType ? { event_type: forcedEventType } : {}),
			...(duration ? durationToProcessingParams(String(duration)) : {}),
			start_time: timeRangeFilters.startTime.toISOString(),
			end_time: timeRangeFilters.endTime?.toISOString(),
			limit: pageLimit,
			offset: pageOffset,
			query: query
		};
	});

	afterNavigate(() => {
		UserService.listUsersIncludeDeleted().then((userData) => {
			for (const user of userData) {
				users.set(user.id, user);
			}
		});
	});

	$effect(() => {
		if (!allFilters) return;
		if (!pageIndexLocal.isReady) return;

		showLoadingSpinner = true;
		fetchAuditLogs({ ...allFilters }).then((res) => {
			// Reset page and page fragment indexes when the total results are less than the current page offset
			if (!res || pageOffset > (res?.total ?? 0)) {
				pageIndexLocal.current = 0;
			}
			showLoadingSpinner = false;
		});
	});

	// Throttle query update
	const handleQueryChange = debounce((value: string) => {
		query = value;

		if (value) {
			page.url.searchParams.set('query', value);
		} else {
			page.url.searchParams.delete('query');
		}

		// Update the query search param without cause app to react
		// Prevent losing focus from the input
		replaceState(page.url, { query: value });
	}, 100);

	// The fetch $effect below reacts to pageIndex (via allFilters.offset), so updating the page
	// index is enough to reload the current page — no direct fetch call is needed here.
	function nextPage() {
		if (isReachedMax) return;
		pageIndexLocal.current = Math.max(0, Math.min(numberOfPages - 1, pageIndex + 1));
	}

	function prevPage() {
		if (isReachedMin) return;
		pageIndexLocal.current = Math.max(0, pageIndex - 1);
	}

	async function fetchAuditLogs(filters: typeof searchParamFilters) {
		return (auditLogsResponse = await UserService.listAuditLogs(filters));
	}

	function getFilterDisplayLabel(key: string) {
		const _key = key as keyof AuditLogURLFilters;

		if (_key === 'event_type') return 'Source';
		if (_key === 'actor') return 'Actor';
		if (_key === 'operation') return 'Operation';
		if (_key === 'mcp_server') return 'Identifier – MCP Server';
		if (_key === 'tool') return 'Identifier – Tool';
		if (_key === 'outcome') return 'Status';
		if (_key === 'client') return 'Client';
		if (_key === 'duration') return 'Duration';
		if (_key === 'start_time') return 'Start Time';
		if (_key === 'end_time') return 'End Time';
		return key.replace(/_(\w)/g, ' $1');
	}

	// getFilterDisplayValue renders a full (possibly comma-joined) filter value; used by the export
	// confirmation dialog. Pills call getFilterValue per single value instead.
	function getFilterDisplayValue(key: string, value: string | number) {
		return String(value)
			.split(',')
			.map((part) => part.trim())
			.filter(Boolean)
			.map((part) => formatSingleFilterValue(key, part))
			.join(', ');
	}

	function getFilterValue(label: keyof AuditLogURLFilters, value: string | number) {
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

		return formatSingleFilterValue(label, String(value));
	}

	function handleRightSidebarClose() {
		rightSidebar?.hidePopover();
		showFilters = false;
		selectedAuditLog = undefined;
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
		pageIndexLocal.current = 0;
	}

	async function handleExportRequest(formType: 'export' | 'scheduled') {
		// Check if there are any active filters. Intentionally skip carrying event_type over.
		const hasActiveFilters =
			Object.keys(pillsSearchParamFilters).some((key) => key !== 'event_type') || Boolean(query);
		if (hasActiveFilters) {
			// Show confirmation dialog
			pendingExportType = formType;
			showFilterConfirmDialog = true;
			return;
		}

		// No filters, proceed directly
		await proceedWithExport(formType, false);
	}

	async function proceedWithExport(formType: 'export' | 'scheduled', includeFilters: boolean) {
		try {
			// Check if storage credentials are configured
			const response = await AdminService.getStorageCredentials();

			// Prepare URL with current filters and time range
			const url = new URL(window.location.origin + `/admin/audit-logs/exports`);
			url.searchParams.set('form', formType);

			if (includeFilters) {
				// Add current time range
				url.searchParams.set('startTime', timeRangeFilters.startTime.toISOString());
				url.searchParams.set('endTime', timeRangeFilters.endTime.toISOString());

				// Add current filters (excluding time filters as they're handled separately, and
				// event_type which is not carried over to the export view)
				Object.entries(pillsSearchParamFilters).forEach(([key, value]) => {
					if (key !== 'start_time' && key !== 'end_time' && key !== 'event_type' && value) {
						url.searchParams.set(key, value.toString());
					}
				});

				// Add query if present
				if (query) {
					url.searchParams.set('query', query);
				}
			}

			if (response.provider) {
				goto(url.pathname + url.search);
			} else {
				url.searchParams.set('form', 'storage');
				url.searchParams.set('next', formType);
				goto(url.pathname + url.search);
			}
		} catch (error) {
			console.error('Failed to get storage credentials:', error);
			const url = new URL(window.location.origin + `/admin/audit-logs/exports`);
			url.searchParams.set('form', 'storage');
			url.searchParams.set('next', formType);

			if (includeFilters) {
				// Still add filters for when storage config is completed (excluding event_type, which
				// is not carried over to the export view)
				url.searchParams.set('startTime', timeRangeFilters.startTime.toISOString());
				url.searchParams.set('endTime', timeRangeFilters.endTime.toISOString());
				Object.entries(pillsSearchParamFilters).forEach(([key, value]) => {
					if (key !== 'start_time' && key !== 'end_time' && key !== 'event_type' && value) {
						url.searchParams.set(key, value.toString());
					}
				});
				if (query) {
					url.searchParams.set('query', query);
				}
			}

			goto(url.pathname + url.search);
		}
	}

	function handleFilterConfirmation(includeFilters: boolean) {
		showFilterConfirmDialog = false;
		if (pendingExportType) {
			proceedWithExport(pendingExportType, includeFilters);
			pendingExportType = null;
		}
	}
</script>

<div class="flex flex-col justify-end gap-2 @container">
	<div class="flex flex-col gap-4 @min-[768px]:flex-row">
		<Search
			class="dark:bg-base-200 dark:border-base-400 bg-base-100 border border-transparent shadow-sm"
			onChange={handleQueryChange}
			placeholder="Search..."
			value={query}
		/>

		<div class="flex flex-col gap-2 self-start @min-[768px]:self-end">
			<div class="flex gap-4">
				<AuditLogCalendar
					start={timeRangeFilters.startTime}
					end={timeRangeFilters.endTime}
					onChange={handleDateChange}
				/>

				<button
					class="btn btn-neutral h-12.5"
					onclick={() => {
						showFilters = true;
						selectedAuditLog = undefined;
						rightSidebar?.showPopover();
					}}
				>
					<Funnel class="size-4" />
					Filters
				</button>
			</div>
		</div>
	</div>
	{#if hasFilterPills || showAuditExportActions}
		<div
			class="{showAuditExportActions
				? 'mt-4'
				: ''} flex flex-col flex-nowrap gap-4 @min-[768px]:flex-row"
		>
			<div class="min-w-0 grow hidden @min-[768px]:block">
				{@render filters()}
			</div>
			{#if showAuditExportActions}
				<div class="@min-[768px]:ml-auto flex shrink-0 gap-4">
					<DotDotDot class="btn btn-block btn-primary w-fit text-sm" placement="bottom">
						{#snippet icon()}
							<span class="flex items-center justify-center gap-1">
								<Plus class="size-4" /> Create Export
							</span>
						{/snippet}
						<button class="menu-button" onclick={() => handleExportRequest('export')}>
							Create One-time Export
						</button>
						<button class="menu-button" onclick={() => handleExportRequest('scheduled')}>
							Create Export Schedule
						</button>
					</DotDotDot>

					<button
						class="btn btn-neutral rounded-4xl"
						onclick={() => {
							goto('/admin/audit-logs/exports');
						}}
					>
						<Settings class="size-4" />
						Manage Exports
					</button>
				</div>
			{/if}
			<div class="min-w-0 grow block @min-[768px]:hidden">
				{@render filters()}
			</div>
		</div>
	{/if}
</div>

{#if showLoadingSpinner}
	<div class="skeleton rounded-md h-71 mb-4"></div>
	<AuditLogTableSkeleton />
{:else if auditLogsTotalItems > 0}
	<div
		class="dark:bg-base-300 dark:border-base-400 bg-base-100 text-muted-content rounded-lg border border-transparent shadow-sm"
	>
		<h3 class="mb-6 px-4 pt-4 text-xs uppercase font-medium">Timeline</h3>
		<div class="px-4">
			{#if displayTimelineData.length > 0}
				<div
					id="mcp-audit-logs-timeline-chart"
					class="text-muted-content flex h-40 items-center justify-center rounded-md"
				>
					<StackedTimeline
						start={timeRangeFilters.startTime}
						end={timeRangeFilters.endTime}
						data={displayTimelineData}
						categoryKey="eventType"
						dateKey="timestamp"
						primaryValueKey={isTimelineAggregated ? 'count' : undefined}
						secondaryValueKey={isTimelineAggregated ? '_secondary' : undefined}
					/>
				</div>
			{:else}
				<div
					class="text-muted-content flex h-40 items-center justify-center gap-2 rounded-md text-sm"
				>
					<Loading class="size-5 animate-spin" />
					<span>Preparing timeline…</span>
				</div>
			{/if}
		</div>
		<hr class="dark:border-base-400 my-4 border" />
		<div class="flex items-center justify-between gap-2 px-4 pb-4 text-xs text-gray-600">
			<div class="flex gap-4">
				<div>{Intl.NumberFormat().format(remoteAuditLogs.length)} results</div>

				<div class="flex items-center">
					{#if numberOfPages > 1}
						<span>{Intl.NumberFormat().format(pageIndex + 1)}</span>/
						<span>{Intl.NumberFormat().format(numberOfPages)}</span>
						<span class="ml-1">pages</span>
					{:else}
						<span>1 page</span>
					{/if}
				</div>
			</div>

			<div class="flex gap-4">
				<button
					class="hover:text-muted-content active:text-base-content/80 flex items-center text-xs transition-colors duration-100 disabled:pointer-events-none disabled:opacity-50"
					disabled={isReachedMin}
					onclick={prevPage}
				>
					<ChevronLeft class="size-[1.4em]" />
					<div>Previous Page</div>
				</button>

				<button
					class="hover:text-muted-content active:text-base-content/80 flex items-center text-xs transition-colors duration-100 disabled:pointer-events-none disabled:opacity-50"
					disabled={isReachedMax}
					onclick={nextPage}
				>
					<div>Next Page</div>
					<ChevronRight class="size-[1.4em]" />
				</button>
			</div>
		</div>
	</div>
	{#if displayTableData.length > 0}
		<AuditLogsTable
			data={displayTableData}
			onSelectRow={async (d: AuditLogEvent) => {
				showFilters = false;
				rightSidebar?.showPopover();
				const user =
					d.actor.actorType === 'user' && d.actor.id ? getUserDisplayName(users, d.actor.id) : '';
				// Fetch full audit log details with request/response bodies
				try {
					const fullDetails = await UserService.getAuditLog(d.id);
					selectedAuditLog = { ...fullDetails, user };
				} catch (error) {
					console.error('Failed to fetch audit log details:', error);
					// Fallback to the cached data if fetch fails
					selectedAuditLog = { ...d, user };
				}
			}}
			getUserDisplayName={(userId: string, hasConflict?: () => boolean) =>
				getUserDisplayName(users, userId, hasConflict)}
			{emptyContent}
		/>
	{:else if remoteAuditLogs.length > 0}
		<div class="text-muted-content flex items-center justify-center gap-2 py-12 text-sm font-light">
			<Loading class="size-5 animate-spin" />
			<span>Preparing results…</span>
		</div>
	{/if}
{:else if !showLoadingSpinner}
	<div class="mt-12 flex w-md max-w-full flex-col items-center gap-4 self-center text-center">
		<Captions class="text-muted-content size-24 opacity-50" />
		<h4 class="text-muted-content text-lg font-semibold">No audit logs</h4>
		<p class="text-muted-content text-sm font-light">
			Currently, there are no audit logs for selected range or filters. Try modifying your search
			criteria or try again later.
		</p>
	</div>
{/if}

<div
	bind:this={rightSidebar}
	popover
	class={twMerge('drawer-legacy', selectedAuditLog ? 'max-w-[85vw] min-w-lg' : 'md:w-lg lg:w-xl')}
	style={selectedAuditLog ? 'width: 32rem' : ''}
	id="mcp-audit-logs-details-sidebar"
>
	{#if selectedAuditLog}
		{#if !responsive.isMobile && rightSidebar}
			<div
				role="none"
				class="absolute top-0 left-0 z-30 h-full w-3 cursor-col-resize"
				use:columnResize={{ column: rightSidebar, direction: 'right' }}
			></div>
		{/if}
		<AuditLogEventDetails onClose={handleRightSidebarClose} auditLog={selectedAuditLog} />
	{/if}

	{#if showFilters}
		<FiltersDrawer
			onClose={handleRightSidebarClose}
			filters={auditLogsSlideoverFilters}
			getVisibleFilterKeys={() => visibleFilterKeys}
			isFilterMultiSelect={(filterId) => filterId !== 'duration'}
			isFilterDisabled={(filterId) =>
				propsFiltersKeys.has(filterId) || enforcedFiltersKeys.has(filterId)}
			isFilterClearable={(filterId) =>
				!propsFiltersKeys.has(filterId) && !enforcedFiltersKeys.has(filterId)}
			getUserDisplayName={(...args) => getUserDisplayName(users, ...args)}
			{getFilterDisplayLabel}
			getFilterOptionLabel={(key, value) => formatSingleFilterValue(key, value)}
			endpoint={async (filterId: string, opts = {}) => {
				// Duration is a fixed set of client-side presets, not a distinct-value column.
				if (filterId === 'duration') {
					return { options: DURATION_BUCKETS.map((bucket) => bucket.value) };
				}

				// A `duration` selection among the other active filters must also narrow the option
				// list, so translate it to the processing_time_min/max params the backend understands.
				const { duration, ...rest } = opts as Partial<AuditLogURLFilters>;
				return await UserService.listAuditLogFilterOptions(filterId, {
					...rest,
					...(duration ? durationToProcessingParams(String(duration)) : {}),
					start_time: timeRangeFilters.startTime.toISOString(),
					end_time: timeRangeFilters.endTime?.toISOString(),
					// In a server-scoped view the source is pinned to MCP; otherwise prefer the event
					// type(s) currently selected in the drawer (passed via opts) so option lists update
					// live as the user switches Source, falling back to the URL.
					event_type: forcedEventType || String(opts.event_type ?? '') || selectedEventTypes
				});
			}}
		/>
	{/if}
</div>

{#snippet filters()}
	<AuditLogFilterPills
		{pillsSearchParamFilters}
		{getFilterDisplayLabel}
		{getFilterValue}
		isFilterClearable={(filterKey) =>
			!propsFiltersKeys.has(filterKey) && !enforcedFiltersKeys.has(filterKey)}
	/>
{/snippet}

<!-- Filter Confirmation Dialog -->
{#if showFilterConfirmDialog}
	<div class="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4">
		<div class="dark:bg-base-300 bg-base-100 w-full max-w-2xl rounded-lg p-6 shadow-xl">
			<h3 class="mb-4 text-lg font-semibold">Apply Current Filters to Export?</h3>
			<p class="text-muted-content mb-4 text-sm">
				You have active filters applied to the audit logs. Would you like to include these filters
				in the export?
			</p>

			<!-- Show current filters. `event_type` (Source) is excluded since it is not carried over to
			the export view. -->
			{#if Object.keys(pillsSearchParamFilters).some((key) => key !== 'event_type') || query}
				{@const entries = (
					Object.entries(pillsSearchParamFilters) as [keyof AuditLogURLFilters, string][]
				).filter(([key]) => key !== 'event_type')}
				<div class="mb-4 rounded-md bg-gray-50 p-3 dark:bg-gray-800">
					<h4 class="mb-2 text-xs font-medium text-muted-content">Active Filters:</h4>
					<div class="text-muted-content space-y-1 text-xs">
						{#if query}
							<div class="wrap-break-word"><strong>Search:</strong> {query}</div>
						{/if}
						{#each entries as [key, value] (key)}
							<div class="wrap-break-word">
								<strong>{getFilterDisplayLabel(key)}:</strong>
								{getFilterDisplayValue(key, value)}
							</div>
						{/each}
					</div>
				</div>
			{/if}

			<div class="flex justify-end gap-3">
				<button class="btn btn-secondary" onclick={() => handleFilterConfirmation(false)}>
					No
				</button>
				<button class="btn btn-primary" onclick={() => handleFilterConfirmation(true)}>
					Yes, Include Filters
				</button>
			</div>
		</div>
	</div>
{/if}
