<script lang="ts">
	import { afterNavigate } from '$app/navigation';
	import { page } from '$app/state';
	import { columnResize } from '$lib/actions/resize';
	import { buildPillSearchParamFilters, buildSearchParamFiltersArray } from '$lib/auditlogs';
	import { type DateRange } from '$lib/components/Calendar.svelte';
	import DotDotDot from '$lib/components/DotDotDot.svelte';
	import Search from '$lib/components/Search.svelte';
	import McpAuditLogDetails from '$lib/components/admin/audit-logs/McpAuditLogDetails.svelte';
	import StackedTimeline from '$lib/components/graph/StackedTimeline.svelte';
	import { setVirtualPageData } from '$lib/components/ui/virtual-page/context';
	import Loading from '$lib/icons/Loading.svelte';
	import { localState } from '$lib/runes/localState.svelte';
	import {
		type OrgUser,
		type McpAuditLogURLFilters,
		AdminService,
		type McpAuditLog,
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
	import McpAuditLogsTable from './McpAuditLogsTable.svelte';
	import {
		aggregateMcpAuditLogsByBucket,
		toMcpAuditLogTimelineChartRow,
		type McpAuditLogTimelineChartRow
	} from './timelineUtils';
	import { ChevronLeft, ChevronRight, Funnel, Captions, Plus, Settings } from '@lucide/svelte';
	import { set, endOfDay, isBefore, subDays } from 'date-fns';
	import { debounce } from 'es-toolkit';
	import type { Snippet } from 'svelte';
	import { SvelteMap } from 'svelte/reactivity';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		mcpId?: string | null;
		id?: string | null;
		mcpServerDisplayName?: string | null;
		mcpServerCatalogEntryName?: string | null;
		emptyContent?: Snippet;
		entity?: 'workspace' | 'catalog';
	}

	let {
		mcpServerDisplayName,
		mcpServerCatalogEntryName,
		mcpId,
		id,
		emptyContent,
		entity = 'catalog'
	}: Props = $props();

	let auditLogsResponse = $state<PaginatedResponse<McpAuditLog>>();
	const auditLogsTotalItems = $derived(auditLogsResponse?.total ?? 0);

	let pageIndexLocal = localState('@obot/auditlogs/page-index', 0, {
		parse: (ls) => (typeof ls === 'string' ? parseInt(ls) : (ls ?? 0))
	});
	const pageIndex = $derived(pageIndexLocal.current ?? 0);
	const pageLimit = $state(10000);

	const numberOfPages = $derived(Math.ceil(auditLogsTotalItems / pageLimit));
	const pageOffset = $derived(pageIndex * pageLimit);

	const remoteMcpAuditLogs = $derived(auditLogsResponse?.items ?? []);

	/** When there are more than this many results, defer timeline aggregation and table data to keep UI responsive. */
	const DEFER_THRESHOLD = 500;
	let displayTimelineData = $state<McpAuditLogTimelineChartRow[]>([]);
	let displayTableData = $state<McpAuditLog[]>([]);

	$effect(() => {
		const items = remoteMcpAuditLogs;
		const start = timeRangeFilters.startTime;
		const end = timeRangeFilters.endTime;
		const threshold = DEFER_THRESHOLD;

		if (items.length <= threshold) {
			displayTableData = items;
			displayTimelineData = items.map(toMcpAuditLogTimelineChartRow);
			return;
		}

		displayTableData = [];
		displayTimelineData = [];
		if (typeof requestIdleCallback !== 'undefined') {
			const id = requestIdleCallback(
				() => {
					displayTableData = items;
					displayTimelineData = aggregateMcpAuditLogsByBucket(items, start, end);
				},
				{ timeout: 200 }
			);
			return () => cancelIdleCallback(id);
		}
		const id = setTimeout(() => {
			displayTableData = items;
			displayTimelineData = aggregateMcpAuditLogsByBucket(items, start, end);
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
	let selectedMcpAuditLog = $state<McpAuditLog & { user: string }>();
	let rightSidebar = $state<HTMLDivElement>();
	let showFilterConfirmDialog = $state(false);
	let pendingExportType = $state<'export' | 'scheduled' | null>(null);

	// Enforced filters for Basic users - they can only see their own audit logs
	const enforcedFilters = $derived.by(() => {
		if (isBasicUser(profile.current.groups) && profile.current?.id) {
			return { user_id: profile.current.id };
		}
		return {};
	});

	const enforcedFiltersKeys = $derived(new Set(Object.keys(enforcedFilters)));

	// Supported filters for the audit logs
	// These filters are used to filter the audit logs based on the URL parameters
	// Ignore other params
	type SupportedFilter = keyof McpAuditLogURLFilters;
	const supportedFilters: SupportedFilter[] = [
		'user_id',
		'mcp_id',
		'mcp_server_display_name',
		'mcp_server_catalog_entry_name',
		'call_type',
		'call_identifier',
		'client_name',
		'client_version',
		'client_ip',
		'response_status',
		'session_id',
		'start_time',
		'end_time'
	];

	const defaultSearchParams: Partial<McpAuditLogURLFilters> = {
		call_type: ['resources/read', 'tools/call', 'prompts/get'].join(',')
	};

	const searchParamsAsArray: [SupportedFilter, string | undefined | null][] = $derived(
		buildSearchParamFiltersArray<McpAuditLogURLFilters>(supportedFilters, defaultSearchParams)
	);

	// Extract search supported params from the URL and convert them to McpAuditLogURLFilters
	// This is used to filter the audit logs based on the URL parameters
	const searchParamFilters = $derived.by<McpAuditLogURLFilters>(() => {
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
		buildPillSearchParamFilters<McpAuditLogURLFilters>(
			searchParamsAsArray,
			{ ...propsFilters, ...enforcedFilters },
			propsFiltersKeys
		)
	);

	const hasFilterPills = $derived(Object.keys(pillsSearchParamFilters).length > 0);

	const showAuditExportActions = $derived(
		profile.current.groups.includes(Group.ADMIN) || profile.current.groups.includes(Group.OWNER)
	);

	// Filters to be used in the audit logs slideover
	// Exclude filters that are set via props and not undefined
	const auditLogsSlideoverFilters = $derived.by(() => {
		const clone = { ...searchParamFilters };

		for (const key of ['start_time', 'end_time']) {
			delete clone[key as keyof McpAuditLogURLFilters];
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
	const allFilters = $derived({
		...pillsSearchParamFilters,
		...propsFilters,
		start_time: timeRangeFilters.startTime.toISOString(),
		end_time: timeRangeFilters.endTime?.toISOString(),
		limit: pageLimit,
		offset: pageOffset,
		query: query
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
		fetchMcpAuditLogs({ ...allFilters }).then((res) => {
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

	async function nextPage() {
		if (isReachedMax) return;

		pageIndexLocal.current = Math.min(numberOfPages, pageIndex + 1);

		fetchMcpAuditLogs({ ...allFilters });
	}

	async function prevPage() {
		if (isReachedMin) return;

		pageIndexLocal.current = Math.max(0, pageIndex - 1);

		fetchMcpAuditLogs({ ...allFilters });
	}

	async function fetchMcpAuditLogs(filters: typeof searchParamFilters) {
		return (auditLogsResponse = await UserService.listMcpAuditLogs(filters));
	}

	function getFilterDisplayLabel(key: string) {
		const _key = key as keyof McpAuditLogURLFilters;

		if (_key === 'mcp_server_display_name') return 'Server';
		if (_key === 'mcp_server_catalog_entry_name') return 'Server Catalog Entry Name';
		if (_key === 'mcp_id') return 'Server ID';
		if (_key === 'start_time') return 'Start Time';
		if (_key === 'end_time') return 'End Time';
		if (_key === 'user_id') return 'User';
		if (_key === 'client_name') return 'Client Name';
		if (_key === 'client_version') return 'Client Version';
		if (_key === 'call_type') return 'Call Type';
		if (_key === 'session_id') return 'Session ID';
		if (_key === 'response_status') return 'Response Status';
		if (_key === 'client_ip') return 'Client IP';

		return key.replace(/_(\w)/g, ' $1');
	}

	function getFilterDisplayValue(key: string, value: string | number) {
		if (key === 'user_id') {
			if (typeof value === 'string') {
				const array = value.split(',').map((v) => v.trim());

				return array.map((v) => getUserDisplayName(users, v)).join(', ');
			}

			return getUserDisplayName(users, value + '');
		}

		return value + '';
	}

	function getFilterValue(label: keyof McpAuditLogURLFilters, value: string | number) {
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

		if (label === 'user_id') {
			return getUserDisplayName(users, value + '');
		}

		return value + '';
	}

	function handleRightSidebarClose() {
		rightSidebar?.hidePopover();
		showFilters = false;
		selectedMcpAuditLog = undefined;
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
		// Check if there are any active filters
		const hasActiveFilters = Object.keys(pillsSearchParamFilters).length > 0 || query;
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

				// Add current filters (excluding time filters as they're handled separately)
				Object.entries(pillsSearchParamFilters).forEach(([key, value]) => {
					if (key !== 'start_time' && key !== 'end_time' && value) {
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
				// Still add filters for when storage config is completed
				url.searchParams.set('startTime', timeRangeFilters.startTime.toISOString());
				url.searchParams.set('endTime', timeRangeFilters.endTime.toISOString());
				Object.entries(pillsSearchParamFilters).forEach(([key, value]) => {
					if (key !== 'start_time' && key !== 'end_time' && value) {
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
						selectedMcpAuditLog = undefined;
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
	<!-- Timeline Graph (Placeholder) -->
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
						categoryKey="callType"
						dateKey="createdAt"
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
				<div>{Intl.NumberFormat().format(remoteMcpAuditLogs.length)} results</div>

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
		<McpAuditLogsTable
			data={displayTableData}
			onSelectRow={async (d: McpAuditLog & { user: string }) => {
				showFilters = false;
				rightSidebar?.showPopover();
				// Fetch full audit log details with request/response bodies
				try {
					const fullDetails = await UserService.getMcpAuditLog(d.id);
					selectedMcpAuditLog = { ...fullDetails, user: d.user };
				} catch (error) {
					console.error('Failed to fetch audit log details:', error);
					// Fallback to the cached data if fetch fails
					selectedMcpAuditLog = d;
				}
			}}
			getUserDisplayName={(userId: string, hasConflict?: () => boolean) =>
				getUserDisplayName(users, userId, hasConflict)}
			{emptyContent}
		/>
	{:else if remoteMcpAuditLogs.length > 0}
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
	class={twMerge(
		'drawer-legacy',
		selectedMcpAuditLog ? 'max-w-[85vw] min-w-lg' : 'md:w-lg lg:w-xl'
	)}
	style={selectedMcpAuditLog ? 'width: 32rem' : ''}
	id="mcp-audit-logs-details-sidebar"
>
	{#if selectedMcpAuditLog}
		{#if !responsive.isMobile && rightSidebar}
			<div
				role="none"
				class="absolute top-0 left-0 z-30 h-full w-3 cursor-col-resize"
				use:columnResize={{ column: rightSidebar, direction: 'right' }}
			></div>
		{/if}
		<McpAuditLogDetails onClose={handleRightSidebarClose} auditLog={selectedMcpAuditLog} />
	{/if}

	{#if showFilters}
		<FiltersDrawer
			onClose={handleRightSidebarClose}
			filters={{ ...auditLogsSlideoverFilters }}
			isFilterDisabled={(filterId) =>
				propsFiltersKeys.has(filterId) || enforcedFiltersKeys.has(filterId)}
			isFilterClearable={(filterId) =>
				!propsFiltersKeys.has(filterId) && !enforcedFiltersKeys.has(filterId)}
			getUserDisplayName={(...args) => getUserDisplayName(users, ...args)}
			{getFilterDisplayLabel}
			getDefaultValue={(filter) => defaultSearchParams[filter]}
			endpoint={async (filterId: string, opts = {}) => {
				const timeFilters = {
					start_time: timeRangeFilters.startTime.toISOString(),
					end_time: timeRangeFilters.endTime?.toISOString()
				};
				if (filterId !== 'mcp_id') {
					return await UserService.listMcpAuditLogFilterOptions(filterId, {
						...opts,
						...timeFilters
					});
				}

				if (mcpId) {
					const response = await UserService.listMcpAuditLogFilterOptions(filterId, {
						...opts,
						...timeFilters
					});

					return { options: response?.options.filter((option) => option.endsWith(mcpId)) ?? [] };
				}

				if (!id || !mcpServerCatalogEntryName) {
					return await UserService.listMcpAuditLogFilterOptions(filterId, {
						...opts,
						...timeFilters
					});
				}

				const items =
					entity === 'catalog'
						? await AdminService.listMCPServersForEntry(id, mcpServerCatalogEntryName)
						: await UserService.listWorkspaceMCPServersForEntry(id, mcpServerCatalogEntryName);

				const options = items?.map?.((item) => item.id) ?? [];

				return { options };
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

			<!-- Show current filters -->
			{#if Object.entries(pillsSearchParamFilters).length > 0 || query}
				{@const entries = Object.entries(pillsSearchParamFilters) as [
					keyof McpAuditLogURLFilters,
					string
				][]}
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
