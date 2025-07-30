<script lang="ts">
	import { browser } from '$app/environment';
	import { afterNavigate, goto } from '$app/navigation';
	import Calendar, { type DateRange } from '$lib/components/Calendar.svelte';
	import Layout from '$lib/components/Layout.svelte';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants';
	import { type OrgUser, type UsageStatsFilters, AdminService } from '$lib/services';
	import UsageGraphs from '$lib/components/admin/usage/UsageGraphs.svelte';

	import { formatTimeRange, getTimeRangeShorthand } from '$lib/time';
	import { X } from 'lucide-svelte';
	import { fade } from 'svelte/transition';

	const duration = PAGE_TRANSITION_DURATION;

	let users = $state<OrgUser[]>([]);
	let currentFilters = $state<UsageStatsFilters>({});

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

	afterNavigate(() => {
		currentFilters = compileSortAndFilters();

		AdminService.listUsers().then((userData) => {
			users = userData;
		});
	});

	function compileSortAndFilters(): UsageStatsFilters & {
		mcpId?: string | null;
		sortBy?: string | null;
		sortOrder?: string | null;
	} {
		if (!browser) return {};

		const url = new URL(window.location.href);
		const startTime = url.searchParams.get('startTime')
			? decodeURIComponent(url.searchParams.get('startTime')!)
			: null;
		const endTime = url.searchParams.get('endTime')
			? decodeURIComponent(url.searchParams.get('endTime')!)
			: null;
		const userId = url.searchParams.get('userId');
		const mcpServerDisplayName = url.searchParams.get('name')
			? decodeURIComponent(url.searchParams.get('name')!)
			: null;

		return {
			startTime,
			endTime,
			userId,
			mcpServerDisplayName
		};
	}

	function convertFilterDisplayLabel(key: string) {
		if (key === 'mcpServerDisplayName') return 'Server';
		if (key === 'startTime') return 'Start Time';
		if (key === 'endTime') return 'End Time';
		if (key === 'userId') return 'User ID';
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
				<h1 class="text-2xl font-semibold">Usage</h1>
				{@render datetimeRangeSelector()}
			</div>
			{@render filters()}
			{@render usageContent()}
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

{#snippet usageContent()}
	{@const { mcpServerDisplayName } = currentFilters}
	<div class="flex flex-col gap-8" in:fade={{ duration }}>
		<UsageGraphs
			mcpServerDisplayName={mcpServerDisplayName ?? undefined}
			{users}
			filters={{
				...currentFilters,
				startTime: timeRange.startTime,
				endTime: timeRange.endTime
			}}
		/>
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
			start={new Date(timeRange.startTime)}
			end={timeRange.endTime ? new Date(timeRange.endTime) : null}
			initialValue={{
				start: new Date(timeRange.startTime),
				end: timeRange.endTime ? new Date(timeRange.endTime) : null
			}}
			onChange={handleDateChange}
		/>
	</div>
{/snippet}

<svelte:head>
	<title>Obot | Usage</title>
</svelte:head>
