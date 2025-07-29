<script lang="ts">
	import { browser } from '$app/environment';
	import { afterNavigate, goto } from '$app/navigation';
	import AuditDetails from '$lib/components/admin/audit-logs/AuditDetails.svelte';
	import Calendar, { type DateRange } from '$lib/components/Calendar.svelte';
	import Layout from '$lib/components/Layout.svelte';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants';
	import { type OrgUser, type AuditLogFilters, AdminService } from '$lib/services';
	import { formatTimeRange, getTimeRangeShorthand } from '$lib/time';
	import { Captions, X } from 'lucide-svelte';
	import { fade } from 'svelte/transition';

	const duration = PAGE_TRANSITION_DURATION;

	let users = $state<OrgUser[]>([]);
	let currentFilters = $state<AuditLogFilters & { mcpId?: string | null }>({});

	let timeRange = $derived(
		currentFilters.startTime || currentFilters.endTime
			? {
					startTime: currentFilters.startTime ?? '',
					endTime: currentFilters.endTime ?? ''
				}
			: {
					startTime: new Date(Date.now() - 7 * 24 * 60 * 60 * 1000).toISOString(),
					endTime: new Date().toISOString()
				}
	);

	let sort = $derived(
		currentFilters.sortBy && currentFilters.sortOrder
			? {
					sortBy: currentFilters.sortBy,
					sortOrder: currentFilters.sortOrder as 'asc' | 'desc'
				}
			: undefined
	);

	afterNavigate(() => {
		currentFilters = compileSortAndFilters();

		AdminService.listUsers().then((userData) => {
			users = userData;
		});
	});

	function compileSortAndFilters(): AuditLogFilters & {
		mcpId?: string | null;
		sortBy?: string | null;
		sortOrder?: string | null;
	} {
		if (!browser) return {};

		const url = new URL(window.location.href);
		const mcpId = url.searchParams.get('mcpId');
		const startTime = url.searchParams.get('startTime')
			? decodeURIComponent(url.searchParams.get('startTime')!)
			: null;
		const endTime = url.searchParams.get('endTime')
			? decodeURIComponent(url.searchParams.get('endTime')!)
			: null;
		const userId = url.searchParams.get('userId');
		const client = url.searchParams.get('client')
			? decodeURIComponent(url.searchParams.get('client')!)
			: null;
		const callType = url.searchParams.get('callType');
		const sessionId = url.searchParams.get('sessionId');
		const mcpServerDisplayName = url.searchParams.get('name')
			? decodeURIComponent(url.searchParams.get('name')!)
			: null;
		const mcpServerCatalogEntryName = url.searchParams.get('entryId')
			? decodeURIComponent(url.searchParams.get('entryId')!)
			: null;
		const sortBy = url.searchParams.get('sortBy');
		const sortOrder = url.searchParams.get('sortOrder');

		return {
			mcpId,
			startTime,
			endTime,
			userId,
			client,
			callType,
			sessionId,
			mcpServerDisplayName,
			mcpServerCatalogEntryName,
			sortBy,
			sortOrder
		};
	}

	function convertFilterDisplayLabel(key: string) {
		if (key === 'mcpServerDisplayName') return 'Server';
		if (key === 'mcpServerCatalogEntryName') return 'Server ID';
		if (key === 'mcpId') return 'Server ID';
		if (key === 'startTime') return 'Start Time';
		if (key === 'endTime') return 'End Time';
		if (key === 'userId') return 'User ID';
		if (key === 'client') return 'Client';
		if (key === 'callType') return 'Call Type';
		if (key === 'sessionId') return 'Session ID';
		return key;
	}

	function handleDateChange(value: DateRange) {
		const url = new URL(window.location.href);

		// make sure to preserve existing filters
		Object.entries(currentFilters).forEach(([key, filterValue]) => {
			if (filterValue && key !== 'startTime' && key !== 'endTime') {
				let urlKey = key;
				if (key === 'mcpServerDisplayName') {
					urlKey = 'name';
				} else if (key === 'mcpServerCatalogEntryName') {
					urlKey = 'entryId';
				}
				url.searchParams.set(urlKey, String(filterValue));
			}
		});

		if (value.start && value.end) {
			url.searchParams.set('startTime', value.start.toISOString());
			url.searchParams.set('endTime', value.end.toISOString());
		} else if (value.start) {
			// If no end time, assume full day of startTime (end at 23:59:59)
			const endOfDay = new Date(value.start);
			endOfDay.setHours(23, 59, 59, 999);
			url.searchParams.set('startTime', value.start.toISOString());
			url.searchParams.set('endTime', endOfDay.toISOString());
		}

		goto(url.toString());
	}
</script>

<Layout>
	<div class="my-4 h-screen" in:fade={{ duration }} out:fade={{ duration }}>
		<div class="flex min-h-full flex-col gap-8 pb-8">
			<div class="flex items-center justify-between gap-4">
				<h1 class="text-2xl font-semibold">Audit Logs</h1>
				{@render datetimeRangeSelector()}
			</div>
			{@render filters()}
			{@render logsContent()}
		</div>
	</div>
</Layout>

{#snippet filters()}
	{@const keys = Object.keys(currentFilters)}
	{@const hasFilters = Object.entries(currentFilters).some(([_, value]) => value)}
	{#if hasFilters}
		<div class="flex flex-wrap items-center gap-2">
			{#each keys.filter((key) => key !== 'startTime' && key !== 'endTime' && key !== 'sortBy' && key !== 'sortOrder') as key (key)}
				{@const value = currentFilters[key as keyof typeof currentFilters]}
				{#if value}
					<div
						class="flex items-center gap-1 rounded-full border border-blue-500 bg-blue-500/33 px-4 py-2"
					>
						<p class="text-xs font-semibold">
							{convertFilterDisplayLabel(key)}: <span class="font-light">{value}</span>
						</p>

						<button
							class="rounded-full p-1 transition-colors duration-200 hover:bg-blue-500/50"
							onclick={() => {
								const url = new URL(window.location.href);

								let urlKey = key;
								if (key === 'mcpServerDisplayName') {
									urlKey = 'name';
								} else if (key === 'mcpServerCatalogEntryName') {
									urlKey = 'entryId';
								}
								url.searchParams.delete(urlKey);
								goto(url.toString());
							}}
						>
							<X class="size-3" />
						</button>
					</div>
				{/if}
			{/each}
		</div>
	{/if}
{/snippet}

{#snippet logsContent()}
	{@const { mcpId, mcpServerCatalogEntryName, mcpServerDisplayName } = currentFilters}
	<div class="flex flex-col gap-8" in:fade={{ duration }}>
		<AuditDetails
			allowPagination
			mcpId={mcpId ?? undefined}
			mcpCatalogEntryId={mcpServerCatalogEntryName ?? undefined}
			mcpServerDisplayName={mcpServerDisplayName ?? undefined}
			{users}
			filters={{
				...currentFilters,
				startTime: timeRange.startTime,
				endTime: timeRange.endTime
			}}
			{sort}
		>
			{#snippet emptyContent()}
				<div class="mt-12 flex w-md flex-col items-center gap-4 self-center text-center">
					<Captions class="size-24 text-gray-200 dark:text-gray-900" />
					<h4 class="text-lg font-semibold text-gray-400 dark:text-gray-600">No audit logs</h4>
					<p class="text-sm font-light text-gray-400 dark:text-gray-600">
						Currently, there are no audit logs.
					</p>
				</div>
			{/snippet}
		</AuditDetails>
	</div>
{/snippet}

{#snippet datetimeRangeSelector()}
	<div class="flex items-center">
		<div
			class="dark:border-surface3 dark:bg-surface1 flex min-h-12.5 flex-shrink-0 items-center gap-2 truncate rounded-l-lg border border-r-0 border-transparent bg-white px-2 text-sm shadow-sm"
		>
			<span class="bg-surface3 rounded-md px-3 py-1 text-xs">
				{getTimeRangeShorthand(timeRange.startTime, timeRange.endTime)}
			</span>
			{formatTimeRange(timeRange.startTime, timeRange.endTime)}
		</div>
		<Calendar
			compact
			class="dark:border-surface3 hover:bg-surface1 dark:hover:bg-surface3 dark:bg-surface1 flex min-h-12.5 flex-shrink-0 items-center gap-2 truncate rounded-none rounded-r-lg border border-transparent bg-white px-4 text-sm shadow-sm"
			initialValue={{
				start: new Date(timeRange.startTime),
				end: timeRange.endTime ? new Date(timeRange.endTime) : null
			}}
			onChange={handleDateChange}
		/>
	</div>
{/snippet}

<svelte:head>
	<title>Obot | Audit Logs</title>
</svelte:head>
