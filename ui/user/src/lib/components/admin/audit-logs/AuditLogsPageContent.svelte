<script lang="ts">
	import type { Snippet } from 'svelte';
	import { fade, slide } from 'svelte/transition';
	import { SvelteMap } from 'svelte/reactivity';
	import { flip } from 'svelte/animate';
	import { X, ChevronLeft, ChevronRight, Funnel, Captions, Plus, Settings } from 'lucide-svelte';
	import { debounce } from 'es-toolkit';
	import { set, endOfDay, isBefore, subDays } from 'date-fns';
	import { page } from '$app/state';
	import { afterNavigate } from '$app/navigation';
	import { goto, replaceState } from '$lib/url';
	import DotDotDot from '$lib/components/DotDotDot.svelte';
	import { type DateRange } from '$lib/components/Calendar.svelte';
	import Search from '$lib/components/Search.svelte';
	import {
		type OrgUser,
		type AuditLogURLFilters,
		AdminService,
		type AuditLog,
		ChatService
	} from '$lib/services';
	import { type PaginatedResponse } from '$lib/services/admin/operations';
	import { clickOutside } from '$lib/actions/clickoutside';
	import { dialogAnimation } from '$lib/actions/dialogAnimation';
	import AuditLogDetails from '$lib/components/admin/audit-logs/AuditLogDetails.svelte';
	import AuditLogsTable from './AuditLogs.svelte';
	import AuditLogsTimeline from './AuditLogsTimeline.svelte';
	import AuditLogCalendar from './AuditLogCalendar.svelte';
	import { localState } from '$lib/runes/localState.svelte';
	import Loading from '$lib/icons/Loading.svelte';
	import FiltersDrawer from '../filters-drawer/FiltersDrawer.svelte';
	import { getUserDisplayName } from '$lib/utils';
	import { setVirtualPageData } from '$lib/components/ui/virtual-page/context';
	import profile from '$lib/stores/profile.svelte';
	import { Group } from '$lib/services';

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

	let auditLogsResponse = $state<PaginatedResponse<AuditLog>>();
	const auditLogsTotalItems = $derived(auditLogsResponse?.total ?? 0);

	let pageIndexLocal = localState('@obot/auditlogs/page-index', 0, {
		parse: (ls) => (typeof ls === 'string' ? parseInt(ls) : (ls ?? 0))
	});
	const pageIndex = $derived(pageIndexLocal.current ?? 0);
	const pageLimit = $state(10000);

	const numberOfPages = $derived(Math.ceil(auditLogsTotalItems / pageLimit));
	const pageOffset = $derived(pageIndex * pageLimit);

	const remoteAuditLogs = $derived(auditLogsResponse?.items ?? []);

	$effect(() => setVirtualPageData(remoteAuditLogs));

	const isReachedMax = $derived(pageIndex >= numberOfPages - 1);
	const isReachedMin = $derived(pageIndex <= 0);

	const users = new SvelteMap<string, OrgUser>();

	let showLoadingSpinner = $state(true);
	let showFilters = $state(false);
	let selectedAuditLog = $state<AuditLog & { user: string }>();
	let rightSidebar = $state<HTMLDialogElement>();
	let showFilterConfirmDialog = $state(false);
	let pendingExportType = $state<'export' | 'scheduled' | null>(null);

	// Supported filters for the audit logs
	// These filters are used to filter the audit logs based on the URL parameters
	// Ignore other params
	type SupportedFilter = keyof AuditLogURLFilters;
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

	const defaultSearchParams: Partial<AuditLogURLFilters> = {
		call_type: ['resources/read', 'tools/call', 'prompts/get'].join(',')
	};

	const searchParamsAsArray: [SupportedFilter, string | undefined | null][] = $derived(
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
							(defaultSearchParams[d]?.toString() ?? null)
			];
		})
	);

	// Extract search supported params from the URL and convert them to AuditLogURLFilters
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
			['mcp_server_catalog_entry_name', mcpServerCatalogEntryName]
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
	const pillsSearchParamFilters = $derived.by(() => {
		const base = searchParamsAsArray
			// exclude start_time and end_time from pills filters
			.filter(([key, value]) => !(key === 'start_time' || key === 'end_time') && isSafe(value))
			.reduce(
				(acc, [key, value]) => {
					acc[key] = value as string | number;
					return acc;
				},
				{} as Record<string, string | number>
			) as Record<keyof AuditLogURLFilters, string>;

		return (
			Object.entries({ ...propsFilters, ...base })
				.filter(([, value]) => !!value)
				// Sort to prioritize props filter keys first, then alphabetically
				.sort((a, b) => {
					// If both keys are in propsFiltersKeys, sort alphabetically
					if (propsFiltersKeys.has(a[0]) && propsFiltersKeys.has(b[0])) {
						return a[0].localeCompare(b[0]);
					}

					// If only a is in propsFiltersKeys, it comes first
					if (propsFiltersKeys.has(a[0])) {
						return -1;
					}

					// If only b is in propsFiltersKeys, it comes first
					if (propsFiltersKeys.has(b[0])) {
						return 1;
					}

					// If neither are in propsFiltersKeys, sort alphabetically
					return a[0].localeCompare(b[0]);
				})
				.reduce(
					(acc, val) => {
						acc[val[0] as keyof AuditLogURLFilters] = val[1] as string;
						return acc;
					},
					{} as Record<string, string | number>
				) as Record<keyof AuditLogURLFilters, string>
		);
	});

	// Filters to be used in the audit logs slideover
	// Exclude filters that are set via props and not undefined
	const auditLogsSlideoverFilters = $derived.by(() => {
		const clone = { ...searchParamFilters };

		for (const key of ['start_time', 'end_time']) {
			delete clone[key as keyof AuditLogURLFilters];
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
			return set(subDays(endTime, 7), { seconds: 0, milliseconds: 0 });
		};

		const startTime = getStartTime(start_time);

		return {
			startTime,
			endTime
		};
	});

	let query = $state(page.url.searchParams.get('query') ?? '');

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
		AdminService.listUsersIncludeDeleted().then((userData) => {
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

	function isSafe<T = unknown>(value: T) {
		return value !== undefined && value !== null;
	}

	async function nextPage() {
		if (isReachedMax) return;

		pageIndexLocal.current = Math.min(numberOfPages, pageIndex + 1);

		fetchAuditLogs({ ...allFilters });
	}

	async function prevPage() {
		if (isReachedMin) return;

		pageIndexLocal.current = Math.max(0, pageIndex - 1);

		fetchAuditLogs({ ...allFilters });
	}

	async function fetchAuditLogs(filters: typeof searchParamFilters) {
		return (auditLogsResponse = await AdminService.listAuditLogs(filters));
	}

	function getFilterDisplayLabel(key: string) {
		const _key = key as keyof AuditLogURLFilters;

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

		if (label === 'user_id') {
			return getUserDisplayName(users, value + '');
		}

		return value + '';
	}

	function handleRightSidebarClose() {
		rightSidebar?.close();
		setTimeout(() => {
			showFilters = false;
			selectedAuditLog = undefined;
		}, 300);
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

{#if showLoadingSpinner}
	<div
		class="absolute inset-0 z-20 flex items-center justify-center"
		in:fade={{ duration: 100 }}
		out:fade|global={{ duration: 300, delay: 500 }}
	>
		<div
			class="bg-surface3/50 border-surface3 text-primary dark:text-primary flex flex-col items-center gap-4 rounded-2xl border px-16 py-8 shadow-md backdrop-blur-[1px]"
		>
			<Loading class="size-32 stroke-1" />
			<div class="text-2xl font-semibold">Loading logs...</div>
		</div>
	</div>
{/if}

<div class="flex flex-col justify-end gap-2">
	<div class="flex flex-col gap-4 md:flex-row">
		<Search
			class="dark:bg-surface1 dark:border-surface3 bg-background border border-transparent shadow-sm"
			onChange={handleQueryChange}
			placeholder="Search..."
			value={query}
		/>

		<div class="flex flex-col gap-2 self-start md:self-end">
			<div class="flex gap-4">
				<AuditLogCalendar
					start={timeRangeFilters.startTime}
					end={timeRangeFilters.endTime}
					onChange={handleDateChange}
				/>

				<button
					class="hover:bg-surface1 dark:bg-surface1 dark:hover:bg-surface3 dark:border-surface3 button bg-background flex w-fit items-center justify-center gap-1 rounded-lg border border-transparent shadow-sm"
					onclick={() => {
						showFilters = true;
						selectedAuditLog = undefined;
						rightSidebar?.show();
					}}
				>
					<Funnel class="size-4" />
					Filters
				</button>
			</div>
		</div>
	</div>
	{#if profile.current.groups.includes(Group.ADMIN) || profile.current.groups.includes(Group.OWNER)}
		<div class="mt-4 flex justify-end gap-2">
			<DotDotDot class="button-primary w-fit text-sm" placement="bottom">
				{#snippet icon()}
					<span class="flex items-center justify-center gap-1">
						<Plus class="size-4" /> Create Export
					</span>
				{/snippet}
				<div class="default-dialog flex min-w-max flex-col p-2">
					<button class="menu-button" onclick={() => handleExportRequest('export')}>
						Create One-time Export
					</button>
					<button class="menu-button" onclick={() => handleExportRequest('scheduled')}>
						Create Export Schedule
					</button>
				</div>
			</DotDotDot>

			<button
				class="hover:bg-surface1 dark:bg-surface1 dark:hover:bg-surface3 dark:border-surface3 button bg-background flex w-fit items-center justify-center gap-1 rounded-lg border border-transparent shadow-sm"
				onclick={() => {
					goto('/admin/audit-logs/exports');
				}}
			>
				<Settings class="size-4" />
				Manage Exports
			</button>
		</div>
	{/if}
</div>

{@render filters()}

{#if auditLogsTotalItems > 0}
	<!-- Timeline Graph (Placeholder) -->
	<div
		class="dark:bg-surface2 dark:border-surface3 bg-background text-on-background rounded-lg border border-transparent shadow-sm"
	>
		<h3 class="mb-2 px-4 pt-4 text-lg font-medium">Timeline</h3>
		<div class="px-4">
			<div class="text-on-surface1 flex h-40 items-center justify-center rounded-md">
				<AuditLogsTimeline
					data={remoteAuditLogs}
					start={timeRangeFilters.startTime}
					end={timeRangeFilters.endTime}
				/>
			</div>
		</div>
		<hr class="dark:border-surface3 my-4 border" />
		<div class="flex items-center justify-between gap-2 px-4 pb-4 text-xs text-gray-600">
			<div class="flex gap-4">
				<div>{Intl.NumberFormat().format(remoteAuditLogs.length)} results</div>

				<div class="flex items-center">
					{#if numberOfPages > 1}
						<sapn>{Intl.NumberFormat().format(pageIndex + 1)}</sapn>/
						<span>{Intl.NumberFormat().format(numberOfPages)}</span>
						<span class="ml-1">pages</span>
					{:else}
						<span>1 page</span>
					{/if}
				</div>
			</div>

			<div class="flex gap-4">
				<button
					class="hover:text-on-surface1/80 active:text-on-surface1/100 flex items-center text-xs transition-colors duration-100 disabled:pointer-events-none disabled:opacity-50"
					disabled={isReachedMin}
					onclick={prevPage}
				>
					<ChevronLeft class="size-[1.4em]" />
					<div>Previous Page</div>
				</button>

				<button
					class="hover:text-on-surface1/80 active:text-on-surface1/100 flex items-center text-xs transition-colors duration-100 disabled:pointer-events-none disabled:opacity-50"
					disabled={isReachedMax}
					onclick={nextPage}
				>
					<div>Next Page</div>
					<ChevronRight class="size-[1.4em]" />
				</button>
			</div>
		</div>
	</div>

	<AuditLogsTable
		data={remoteAuditLogs}
		onSelectRow={async (d: AuditLog & { user: string }) => {
			showFilters = false;
			rightSidebar?.show();
			// Fetch full audit log details with request/response bodies
			try {
				const fullDetails = await AdminService.getAuditLog(d.id);
				selectedAuditLog = { ...fullDetails, user: d.user };
			} catch (error) {
				console.error('Failed to fetch audit log details:', error);
				// Fallback to the cached data if fetch fails
				selectedAuditLog = d;
			}
		}}
		getUserDisplayName={(userId: string, hasConflict?: () => boolean) =>
			getUserDisplayName(users, userId, hasConflict)}
		{emptyContent}
	></AuditLogsTable>
{:else if !showLoadingSpinner}
	<div class="mt-12 flex w-md max-w-full flex-col items-center gap-4 self-center text-center">
		<Captions class="text-on-surface1 size-24 opacity-50" />
		<h4 class="text-on-surface1 text-lg font-semibold">No audit logs</h4>
		<p class="text-on-surface text-sm font-light">
			Currently, there are no audit logs for selected range or filters. Try modifying your search
			criteria or try again later.
		</p>
	</div>
{/if}

<dialog
	bind:this={rightSidebar}
	use:clickOutside={[handleRightSidebarClose, true]}
	use:dialogAnimation={{ type: 'drawer' }}
	class="dark:border-surface1 dark:bg-surface1 bg-background fixed! top-0! right-0! bottom-0! left-auto! z-40 h-dvh w-auto max-w-none rounded-none border-0 shadow-lg outline-none!"
>
	{#if selectedAuditLog}
		<AuditLogDetails onClose={handleRightSidebarClose} auditLog={selectedAuditLog} />
	{/if}

	{#if showFilters}
		<FiltersDrawer
			onClose={handleRightSidebarClose}
			filters={{ ...auditLogsSlideoverFilters }}
			isFilterDisabled={(filterId) => propsFiltersKeys.has(filterId)}
			isFilterClearable={(filterId) => !propsFiltersKeys.has(filterId)}
			getUserDisplayName={(...args) => getUserDisplayName(users, ...args)}
			{getFilterDisplayLabel}
			getDefaultValue={(filter) => defaultSearchParams[filter]}
			endpoint={async (filterId: string, ...args) => {
				if (filterId !== 'mcp_id') {
					return await AdminService.listAuditLogFilterOptions(filterId, ...args);
				}

				if (mcpId) {
					const response = await AdminService.listAuditLogFilterOptions(filterId, ...args);

					return { options: response?.options.filter((option) => option.endsWith(mcpId)) ?? [] };
				}

				if (!id || !mcpServerCatalogEntryName) {
					return await AdminService.listAuditLogFilterOptions(filterId, ...args);
				}

				const items =
					entity === 'catalog'
						? await AdminService.listMCPServersForEntry(id, mcpServerCatalogEntryName)
						: await ChatService.listWorkspaceMCPServersForEntry(id, mcpServerCatalogEntryName);

				const options = items?.map?.((item) => item.id) ?? [];

				return { options };
			}}
		/>
	{/if}
</dialog>

{#snippet filters()}
	{@const entries = Object.entries(pillsSearchParamFilters) as [keyof AuditLogURLFilters, string][]}
	{@const hasFilters = !!entries.length}

	{#if hasFilters}
		<div
			class="flex flex-wrap items-center gap-2"
			in:slide={{ duration: 100 }}
			out:slide={{ duration: 50 }}
		>
			{#each entries as [filterKey, filterValues] (filterKey)}
				{@const displayLabel = getFilterDisplayLabel(filterKey)}
				{@const values = filterValues?.toString().split(',').filter(Boolean) ?? []}
				{@const isClearable = !propsFiltersKeys.has(filterKey)}

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

<!-- Filter Confirmation Dialog -->
{#if showFilterConfirmDialog}
	<div class="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4">
		<div class="dark:bg-surface2 bg-background w-full max-w-2xl rounded-lg p-6 shadow-xl">
			<h3 class="mb-4 text-lg font-semibold">Apply Current Filters to Export?</h3>
			<p class="text-on-surface1 mb-4 text-sm">
				You have active filters applied to the audit logs. Would you like to include these filters
				in the export?
			</p>

			<!-- Show current filters -->
			{#if Object.entries(pillsSearchParamFilters).length > 0 || query}
				{@const entries = Object.entries(pillsSearchParamFilters) as [
					keyof AuditLogURLFilters,
					string
				][]}
				<div class="mb-4 rounded-md bg-gray-50 p-3 dark:bg-gray-800">
					<h4 class="mb-2 text-xs font-medium text-gray-700 dark:text-gray-300">Active Filters:</h4>
					<div class="text-on-surface1 space-y-1 text-xs">
						{#if query}
							<div class="break-words"><strong>Search:</strong> {query}</div>
						{/if}
						{#each entries as [key, value] (key)}
							<div class="break-words">
								<strong>{getFilterDisplayLabel(key)}:</strong>
								{getFilterDisplayValue(key, value)}
							</div>
						{/each}
					</div>
				</div>
			{/if}

			<div class="flex justify-end gap-3">
				<button class="button" onclick={() => handleFilterConfirmation(false)}> No </button>
				<button class="button-primary" onclick={() => handleFilterConfirmation(true)}>
					Yes, Include Filters
				</button>
			</div>
		</div>
	</div>
{/if}
