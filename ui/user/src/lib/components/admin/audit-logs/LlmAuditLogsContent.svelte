<script lang="ts">
	import { afterNavigate } from '$app/navigation';
	import { page } from '$app/state';
	import { columnResize } from '$lib/actions/resize';
	import { buildPillSearchParamFilters, buildSearchParamFiltersArray } from '$lib/auditlogs';
	import DotDotDot from '$lib/components/DotDotDot.svelte';
	import Search from '$lib/components/Search.svelte';
	import AuditLogCalendar from '$lib/components/admin/audit-logs/AuditLogCalendar.svelte';
	import LlmAuditLogsTable from '$lib/components/admin/audit-logs/LlmAuditLogsTable.svelte';
	import { setVirtualPageData } from '$lib/components/ui/virtual-page/context';
	import { isAbortError } from '$lib/errors';
	import {
		AdminService,
		UserService,
		type LLMAuditLog,
		type LLMAuditLogURLFilters,
		type OrgUser
	} from '$lib/services';
	import type { PaginatedResponse } from '$lib/services/http';
	import { profile, responsive } from '$lib/stores';
	import { goto, replaceState } from '$lib/url';
	import { getUserDisplayName } from '$lib/utils';
	import FiltersDrawer from '../filters-drawer/FiltersDrawer.svelte';
	import AuditLogFilterPills from './AuditLogFilterPills.svelte';
	import AuditLogTableSkeleton from './AuditLogTableSkeleton.svelte';
	import LlmAuditLogDetails, { type LlmAuditLogDetail } from './LlmAuditLogDetails.svelte';
	import {
		Captions,
		ChevronLeft,
		ChevronRight,
		CircleAlert,
		Funnel,
		Plus,
		Settings
	} from '@lucide/svelte';
	import { endOfDay, set, subDays } from 'date-fns';
	import { debounce } from 'es-toolkit';
	import { twMerge } from 'tailwind-merge';

	type SupportedFilter = keyof LLMAuditLogURLFilters;
	const supportedFilters: SupportedFilter[] = [
		'user_id',
		'client',
		'client_session_id',
		'message_policy_triggered',
		'model_provider',
		'outcome',
		'request_path',
		'response_status',
		'target_model'
	];
	const pageLimit = 1000;

	let loading = $state(true);
	let fetchError = $state<string | null>(null);
	let response = $state<PaginatedResponse<LLMAuditLog>>();
	let pageIndex = $state(0);
	let users = $state<OrgUser[]>([]);
	let showFilters = $state(false);
	let rightSidebar = $state<HTMLDivElement>();
	let selectedAuditLog = $state<LlmAuditLogDetail>();
	let isAdminReadonly = $derived(profile.current.isAdminReadonly?.());

	const total = $derived(response?.total ?? 0);
	const numberOfPages = $derived(Math.ceil(total / pageLimit));
	const isReachedMin = $derived(pageIndex <= 0);
	const isReachedMax = $derived(pageIndex >= numberOfPages - 1);

	let query = $derived(page.url.searchParams.get('query') ?? '');
	let includeModelsRequests = $derived(
		page.url.searchParams.get('include_models_requests') === 'true'
	);
	let includeModelsRequestsDraft = $state(false);
	$effect(() => {
		includeModelsRequestsDraft = includeModelsRequests;
	});
	let usersMap = $derived(new Map(users.map((user) => [user.id, user])));

	const DEFER_THRESHOLD = 500;
	let displayTableData = $state<LLMAuditLog[]>([]);
	$effect(() => {
		const items = response?.items ?? [];
		const threshold = DEFER_THRESHOLD;

		if (items.length <= threshold) {
			displayTableData = items;
			return;
		}

		displayTableData = [];
		if (typeof requestIdleCallback !== 'undefined') {
			const id = requestIdleCallback(
				() => {
					displayTableData = items;
				},
				{ timeout: 200 }
			);
			return () => cancelIdleCallback(id);
		}
		const id = setTimeout(() => {
			displayTableData = items;
		}, 0);
		return () => clearTimeout(id);
	});

	$effect(() => {
		setVirtualPageData(displayTableData);
	});

	const pageOffset = $derived(pageIndex * pageLimit);

	const searchParamFiltersAsArray = $derived(
		buildSearchParamFiltersArray<LLMAuditLogURLFilters>(supportedFilters)
	);
	const searchParamFilters = $derived.by<LLMAuditLogURLFilters>(() => {
		return searchParamFiltersAsArray.reduce(
			(acc, [key, value]) => {
				acc[key!] = value;
				return acc;
			},
			{} as Record<string, unknown>
		);
	});

	const pillsSearchParamFilters = $derived(
		buildPillSearchParamFilters<LLMAuditLogURLFilters>(searchParamFiltersAsArray)
	);
	const hasFilterPills = $derived(Object.keys(pillsSearchParamFilters).length > 0);

	const timeRangeFilters = $derived.by(() => {
		const startParam = page.url.searchParams.get('start_time');
		const endParam = page.url.searchParams.get('end_time');
		const endTime = set(new Date(endParam || new Date()), { milliseconds: 0, seconds: 59 });
		const startTime = startParam
			? set(new Date(startParam), { milliseconds: 0, seconds: 0 })
			: set(subDays(endTime, 7), { milliseconds: 0, seconds: 0 });

		return { startTime, endTime };
	});

	const filters = $derived<LLMAuditLogURLFilters>({
		...pillsSearchParamFilters,
		start_time: timeRangeFilters.startTime.toISOString(),
		end_time: timeRangeFilters.endTime.toISOString(),
		limit: pageLimit,
		offset: pageOffset,
		include_models_requests: includeModelsRequests.toString(),
		query
	});

	const filterPaginationKey = $derived(
		JSON.stringify({
			...pillsSearchParamFilters,
			query,
			include_models_requests: includeModelsRequests,
			start_time: timeRangeFilters.startTime.toISOString(),
			end_time: timeRangeFilters.endTime.toISOString()
		})
	);

	$effect(() => {
		void filterPaginationKey;
		pageIndex = 0;
	});

	$effect(() => {
		const controller = new AbortController();
		const currentFilters = filters;

		loading = true;
		fetchError = null;

		AdminService.listLLMAuditLogs(currentFilters, { signal: controller.signal })
			.then((res) => {
				if (controller.signal.aborted) return;
				response = res;
				if (pageOffset > (res?.total ?? 0)) {
					pageIndex = 0;
				}
			})
			.catch((err) => {
				if (isAbortError(err) || controller.signal.aborted) return;
				console.error('Failed to fetch LLM audit logs:', err);
				fetchError = err instanceof Error ? err.message : 'Failed to load LLM audit logs';
			})
			.finally(() => {
				if (controller.signal.aborted) return;
				loading = false;
			});

		return () => controller.abort();
	});

	afterNavigate(() => {
		UserService.listUsersIncludeDeleted().then((userData) => {
			users = userData;
		});
	});

	const handleQueryChange = debounce((value: string) => {
		if (value) {
			page.url.searchParams.set('query', value);
		} else {
			page.url.searchParams.delete('query');
		}
		replaceState(page.url, {});
	}, 100);

	function handleDateChange({ start, end }: { start?: Date; end?: Date }) {
		const url = page.url;
		if (start) {
			url.searchParams.set('start_time', start.toISOString());
			url.searchParams.set('end_time', (end ?? endOfDay(start)).toISOString());
		}
		goto(url, { noScroll: true });
	}

	function nextPage() {
		if (!isReachedMax) pageIndex += 1;
	}

	function prevPage() {
		if (!isReachedMin) pageIndex -= 1;
	}

	function getFilterDisplayLabel(key: string) {
		const _key = key as keyof LLMAuditLogURLFilters;
		if (_key === 'outcome') return 'Outcome';
		if (_key === 'request_path') return 'Path';
		if (_key === 'response_status') return 'Status';
		if (_key === 'user_id') return 'User';
		if (_key === 'client_session_id') return 'Client Session ID';
		if (_key === 'message_policy_triggered') return 'Message Policy Action';

		return key.replace(/_(\w)/g, ' $1');
	}

	function getFilterValue(label: keyof LLMAuditLogURLFilters, value: string | number) {
		if (label === 'user_id') {
			return getUserDisplayName(usersMap, value + '');
		}
		if (label === 'message_policy_triggered') {
			return value === 'true' ? 'Triggered' : 'Not triggered';
		}

		return value + '';
	}

	function handleRightSidebarClose() {
		rightSidebar?.hidePopover();
		selectedAuditLog = undefined;
		showFilters = false;
	}

	function buildExportURL(
		formType: 'export' | 'scheduled' | 'storage',
		next?: 'export' | 'scheduled'
	) {
		const url = new URL('/admin/llm-audit-logs/exports', page.url.origin);
		page.url.searchParams.forEach((value, key) => {
			url.searchParams.set(key, value);
		});
		url.searchParams.set('form', formType);
		if (next) {
			url.searchParams.set('next', next);
		}
		return url;
	}

	async function openExportForm(formType: 'export' | 'scheduled') {
		try {
			const response = await AdminService.getStorageCredentials();
			if (response.provider) {
				goto(buildExportURL(formType), { replaceState: false });
			} else {
				goto(buildExportURL('storage', formType), { replaceState: false });
			}
		} catch (error) {
			console.error('Failed to get storage credentials:', error);
			goto(buildExportURL('storage', formType), { replaceState: false });
		}
	}
</script>

<div class="flex flex-col gap-4 @container">
	<div class="flex flex-col gap-4 @min-[768px]:flex-row">
		<Search
			class="dark:bg-base-200 dark:border-base-400 bg-base-100 border border-transparent shadow-sm"
			onChange={handleQueryChange}
			placeholder="Search..."
			value={query}
		/>
		<div class="self-start @min-[768px]:self-end flex gap-4">
			<AuditLogCalendar
				start={timeRangeFilters.startTime}
				end={timeRangeFilters.endTime}
				onChange={handleDateChange}
			/>
			<button
				class="btn btn-neutral h-12.5"
				onclick={() => {
					includeModelsRequestsDraft = includeModelsRequests;
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

	{#if hasFilterPills || !isAdminReadonly}
		<div class="flex flex-col flex-nowrap gap-4 @min-[768px]:flex-row">
			<div class="min-w-0 grow hidden @min-[768px]:block">
				{#if hasFilterPills}
					<AuditLogFilterPills {pillsSearchParamFilters} {getFilterDisplayLabel} {getFilterValue} />
				{/if}
			</div>
			{#if !isAdminReadonly}
				<div class="@min-[768px]:ml-auto flex shrink-0 gap-4">
					<DotDotDot class="btn btn-block btn-primary w-fit text-sm" placement="bottom">
						{#snippet icon()}
							<span class="flex items-center justify-center gap-1">
								<Plus class="size-4" /> Create Export
							</span>
						{/snippet}
						<button class="menu-button" onclick={() => openExportForm('export')}>
							Create One-time Export
						</button>
						<button class="menu-button" onclick={() => openExportForm('scheduled')}>
							Create Export Schedule
						</button>
					</DotDotDot>

					<button
						class="btn btn-neutral rounded-4xl"
						onclick={() => {
							goto('/admin/llm-audit-logs/exports');
						}}
					>
						<Settings class="size-4" />
						Manage Exports
					</button>
				</div>
			{/if}
			<div class="min-w-0 grow block @min-[768px]:hidden">
				{#if hasFilterPills}
					<AuditLogFilterPills {pillsSearchParamFilters} {getFilterDisplayLabel} {getFilterValue} />
				{/if}
			</div>
		</div>
	{/if}
</div>

{#if loading}
	<AuditLogTableSkeleton />
{:else if fetchError}
	<div class="notification-error flex w-full items-center gap-3 p-4">
		<CircleAlert class="size-5 shrink-0" />
		<div class="flex flex-col gap-1">
			<p class="text-sm font-semibold">Unable to load LLM audit logs</p>
			<p class="text-sm font-light">{fetchError}</p>
		</div>
	</div>
{:else if displayTableData.length > 0}
	<LlmAuditLogsTable
		onSelectRow={(d: LLMAuditLog) => {
			showFilters = false;
			selectedAuditLog = {
				...d,
				user: getUserDisplayName(usersMap, d.userID)
			};
			rightSidebar?.showPopover();
		}}
		getUserDisplayName={(id: string) => getUserDisplayName(usersMap, id)}
	/>
{:else}
	<div class="flex flex-col items-center justify-center gap-4 px-6 py-16 text-center w-full">
		<Captions class="text-muted-content size-20 opacity-50" />
		<h4 class="text-muted-content text-lg font-semibold">No LLM audit logs</h4>
		<p class="text-muted-content max-w-md text-sm font-light">
			There are no LLM audit logs for the selected range or search criteria.
		</p>
	</div>
{/if}

<div class="flex grow"></div>

{#if !loading && total > 0 && numberOfPages > 1}
	<div class="sticky left-0 w-full bottom-0 bg-base-200 dark:bg-base-100 z-50 py-4">
		<div class="text-muted-content flex items-center justify-between gap-4 px-1 text-sm">
			<button
				class="hover:text-base-content flex gap-1 items-center disabled:opacity-50"
				disabled={isReachedMin}
				onclick={prevPage}
			>
				<ChevronLeft class="size-4" /> Previous Page
			</button>
			<div class="flex gap-4">
				<div>
					{Intl.NumberFormat().format(pageIndex + 1)} of {Intl.NumberFormat().format(
						numberOfPages || 1
					)} pages
				</div>
			</div>
			<button
				class="hover:text-base-content flex gap-1 items-center disabled:opacity-50"
				disabled={isReachedMax}
				onclick={nextPage}
			>
				Next Page <ChevronRight class="size-4" />
			</button>
		</div>
	</div>
{/if}

<div
	bind:this={rightSidebar}
	popover
	class={twMerge('drawer-legacy', selectedAuditLog ? 'max-w-[85vw] min-w-lg' : 'md:w-lg lg:w-xl')}
	style={selectedAuditLog ? 'width: 32rem' : ''}
>
	{#if selectedAuditLog}
		{#if !responsive.isMobile && rightSidebar}
			<div
				role="none"
				class="absolute top-0 left-0 z-30 h-full w-3 cursor-col-resize"
				use:columnResize={{ column: rightSidebar, direction: 'right' }}
			></div>
		{/if}
	{/if}
	{#if selectedAuditLog}
		<LlmAuditLogDetails
			id={selectedAuditLog.id}
			auditLog={selectedAuditLog}
			onClose={handleRightSidebarClose}
		/>
	{:else if showFilters}
		<FiltersDrawer
			onClose={handleRightSidebarClose}
			filters={{ ...searchParamFilters }}
			isFilterDisabled={() => false}
			isFilterClearable={() => true}
			booleanFilters={[
				{
					property: 'include_models_requests',
					label: 'Show model discovery requests',
					selected: includeModelsRequestsDraft,
					onChange: (selected) => (includeModelsRequestsDraft = selected)
				}
			]}
			getUserDisplayName={(...args) => getUserDisplayName(usersMap, ...args)}
			{getFilterDisplayLabel}
			getFilterOptionLabel={(key, value) =>
				key === 'message_policy_triggered'
					? value === 'true'
						? 'Triggered'
						: 'Not triggered'
					: value}
			endpoint={async (filterId, opts) => {
				const response = await AdminService.listLLMAuditLogFilterOptions(filterId, {
					...opts,
					include_models_requests: includeModelsRequests.toString(),
					start_time: timeRangeFilters.startTime.toISOString(),
					end_time: timeRangeFilters.endTime.toISOString()
				});
				return { options: response?.options ?? [] };
			}}
		/>
	{/if}
</div>
