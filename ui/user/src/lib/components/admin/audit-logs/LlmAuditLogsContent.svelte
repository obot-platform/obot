<script lang="ts">
	import { afterNavigate } from '$app/navigation';
	import { page } from '$app/state';
	import { columnResize } from '$lib/actions/resize';
	import Search from '$lib/components/Search.svelte';
	import AuditLogCalendar from '$lib/components/admin/audit-logs/AuditLogCalendar.svelte';
	import LlmAuditLogsTable from '$lib/components/admin/audit-logs/LlmAuditLogsTable.svelte';
	import { setVirtualPageData } from '$lib/components/ui/virtual-page/context';
	import {
		AdminService,
		UserService,
		type LLMAuditLog,
		type LLMAuditLogURLFilters,
		type OrgUser
	} from '$lib/services';
	import type { PaginatedResponse } from '$lib/services/http';
	import { responsive } from '$lib/stores';
	import { goto, replaceState } from '$lib/url';
	import { getUserDisplayName } from '$lib/utils';
	import AuditLogTableSkeleton from './AuditLogTableSkeleton.svelte';
	import LlmAuditLogDetails, { type LlmAuditLogDetail } from './LlmAuditLogDetails.svelte';
	import { Captions } from '@lucide/svelte';
	import { endOfDay, set, subDays } from 'date-fns';
	import { debounce } from 'es-toolkit';

	const pageLimit = 10000;

	let loading = $state(true);
	let response = $state<PaginatedResponse<LLMAuditLog>>();
	let pageIndex = $state(0);
	let query = $state(page.url.searchParams.get('query') ?? '');
	let users = $state<OrgUser[]>([]);

	let usersMap = $derived(new Map(users.map((user) => [user.id, user])));

	let rightSidebar = $state<HTMLDivElement>();
	let selectedAuditLog = $state<LlmAuditLogDetail>();

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

	const timeRangeFilters = $derived.by(() => {
		const startParam = page.url.searchParams.get('start_time');
		const endParam = page.url.searchParams.get('end_time');
		const endTime = set(new Date(endParam || new Date()), { milliseconds: 0, seconds: 59 });
		const startTime = startParam
			? set(new Date(startParam), { milliseconds: 0, seconds: 0 })
			: set(subDays(endTime, 30), { milliseconds: 0, seconds: 0 });

		return { startTime, endTime };
	});

	const filters = $derived<LLMAuditLogURLFilters>({
		start_time: timeRangeFilters.startTime.toISOString(),
		end_time: timeRangeFilters.endTime.toISOString(),
		limit: pageLimit,
		offset: pageOffset,
		query
	});

	$effect(() => {
		fetchLogs(filters);
	});

	afterNavigate(() => {
		UserService.listUsersIncludeDeleted().then((userData) => {
			users = userData;
		});
	});

	const handleQueryChange = debounce((value: string) => {
		query = value;
		pageIndex = 0;
		if (value) {
			page.url.searchParams.set('query', value);
		} else {
			page.url.searchParams.delete('query');
		}
		replaceState(page.url, {});
	}, 100);

	async function fetchLogs(currentFilters: LLMAuditLogURLFilters) {
		loading = true;
		try {
			response = await AdminService.listLLMAuditLogs(currentFilters);
			console.log('response', response);
			if (pageOffset > (response?.total ?? 0)) {
				pageIndex = 0;
			}
		} finally {
			loading = false;
		}
	}

	function handleDateChange({ start, end }: { start?: Date; end?: Date }) {
		const url = page.url;
		if (start) {
			url.searchParams.set('start_time', start.toISOString());
			url.searchParams.set('end_time', (end ?? endOfDay(start)).toISOString());
		}
		pageIndex = 0;
		goto(url, { noScroll: true });
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
		<div class="self-start @min-[768px]:self-end">
			<AuditLogCalendar
				start={timeRangeFilters.startTime}
				end={timeRangeFilters.endTime}
				onChange={handleDateChange}
			/>
		</div>
	</div>
</div>

{#if loading}
	<AuditLogTableSkeleton />
{:else if displayTableData.length > 0}
	<LlmAuditLogsTable
		onSelectRow={(d: LLMAuditLog) => {
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

<div
	bind:this={rightSidebar}
	popover
	class="drawer-legacy max-w-[85vw] min-w-lg"
	style="width: 32rem"
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
			onClose={() => {
				rightSidebar?.hidePopover();
				selectedAuditLog = undefined;
			}}
		/>
	{/if}
</div>
