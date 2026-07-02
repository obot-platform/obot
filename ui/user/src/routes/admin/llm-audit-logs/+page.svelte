<script lang="ts">
	import { goto, replaceState } from '$app/navigation';
	import { page } from '$app/state';
	import Layout from '$lib/components/Layout.svelte';
	import Search from '$lib/components/Search.svelte';
	import AuditLogCalendar from '$lib/components/admin/audit-logs/AuditLogCalendar.svelte';
	import Loading from '$lib/icons/Loading.svelte';
	import { UserService, type LLMAuditLog, type LLMAuditLogURLFilters } from '$lib/services';
	import type { PaginatedResponse } from '$lib/services/http';
	import { userDeviceSettings } from '$lib/stores';
	import { formatLogTimestamp } from '$lib/time';
	import { Captions, ChevronLeft, ChevronRight } from '@lucide/svelte';
	import { endOfDay, set, subDays } from 'date-fns';
	import { debounce } from 'es-toolkit';
	import { fade } from 'svelte/transition';

	const pageLimit = 100;

	let response = $state<PaginatedResponse<LLMAuditLog>>();
	let loading = $state(true);
	let pageIndex = $state(0);
	let query = $state(page.url.searchParams.get('query') ?? '');

	const total = $derived(response?.total ?? 0);
	const logs = $derived(response?.items ?? []);
	const numberOfPages = $derived(Math.ceil(total / pageLimit));
	const pageOffset = $derived(pageIndex * pageLimit);
	const isReachedMin = $derived(pageIndex <= 0);
	const isReachedMax = $derived(pageIndex >= numberOfPages - 1);

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
		void fetchLogs(filters);
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
			response = await UserService.listLLMAuditLogs(currentFilters);
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

	function nextPage() {
		if (!isReachedMax) pageIndex += 1;
	}

	function prevPage() {
		if (!isReachedMin) pageIndex -= 1;
	}

	function formatDuration(ms: number) {
		return ms ? Intl.NumberFormat().format(ms) : '';
	}

	function formatNumber(value: number) {
		return value ? Intl.NumberFormat().format(value) : '';
	}
</script>

<svelte:head>
	<title>Obot | LLM Audit Logs</title>
</svelte:head>

<Layout classes={{ childrenContainer: 'max-w-none', container: '' }} title="LLM Audit Logs">
	<div class="flex-1" in:fade={{ duration: 100 }}>
		<div class="flex min-h-full flex-col gap-6 pb-8">
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

			<div
				class="dark:bg-base-300 bg-base-100 relative overflow-x-auto rounded-lg border border-transparent shadow-sm"
			>
				{#if loading}
					<div
						class="absolute inset-0 z-10 flex items-center justify-center bg-base-100/60 p-8 dark:bg-base-300/60"
					>
						<Loading class="text-primary size-12" />
					</div>
				{/if}

				{#if logs.length}
					<table class="w-full table-fixed border-collapse text-sm">
						<thead>
							<tr
								class="text-muted-content dark:bg-base-200 bg-base-300 text-left text-xs font-medium tracking-wider uppercase"
							>
								<th class="w-[32ch] px-6 py-3">Timestamp</th>
								<th class="w-[24ch] px-6 py-3">User</th>
								<th class="w-[18ch] px-6 py-3">Provider</th>
								<th class="w-[28ch] px-6 py-3">Model</th>
								<th class="w-[28ch] px-6 py-3">Target Model</th>
								<th class="w-[26ch] px-6 py-3">Path</th>
								<th class="w-[16ch] px-6 py-3">Status</th>
								<th class="w-[18ch] px-6 py-3">Outcome</th>
								<th class="w-[18ch] px-6 py-3">Duration (ms)</th>
								<th class="w-[18ch] px-6 py-3">Input</th>
								<th class="w-[18ch] px-6 py-3">Output</th>
								<th class="w-[22ch] px-6 py-3">Client</th>
								<th class="w-[28ch] px-6 py-3">Session</th>
								<th class="w-[22ch] px-6 py-3">IP Address</th>
							</tr>
						</thead>
						<tbody>
							{#each logs as log (log.id)}
								<tr
									class="hover:bg-base-200 dark:hover:bg-base-400 border-base-200 dark:border-base-400 border-t transition-colors"
								>
									<td class="truncate px-6 py-4"
										>{formatLogTimestamp(log.createdAt, userDeviceSettings.timeFormat)}</td
									>
									<td class="truncate px-6 py-4">{log.userID}</td>
									<td class="truncate px-6 py-4">{log.modelProvider}</td>
									<td class="truncate px-6 py-4">{log.modelID}</td>
									<td class="truncate px-6 py-4">{log.targetModel}</td>
									<td class="truncate px-6 py-4">{log.requestPath}</td>
									<td class="truncate px-6 py-4">{log.responseStatus || ''}</td>
									<td class="truncate px-6 py-4">{log.outcome}</td>
									<td class="truncate px-6 py-4">{formatDuration(log.duration)}</td>
									<td class="truncate px-6 py-4">{formatNumber(log.inputTokens)}</td>
									<td class="truncate px-6 py-4">{formatNumber(log.outputTokens)}</td>
									<td class="truncate px-6 py-4">{log.client}</td>
									<td class="truncate px-6 py-4">{log.clientSessionID}</td>
									<td class="truncate px-6 py-4">{log.clientIP}</td>
								</tr>
							{/each}
						</tbody>
					</table>
				{:else if !loading}
					<div class="flex flex-col items-center gap-4 px-6 py-16 text-center">
						<Captions class="text-muted-content size-20 opacity-50" />
						<h4 class="text-muted-content text-lg font-semibold">No LLM audit logs</h4>
						<p class="text-muted-content max-w-md text-sm font-light">
							There are no LLM audit logs for the selected range or search criteria.
						</p>
					</div>
				{/if}
			</div>

			{#if total > 0}
				<div class="text-muted-content flex items-center justify-between gap-4 px-1 text-xs">
					<div class="flex gap-4">
						<div>{Intl.NumberFormat().format(logs.length)} results</div>
						<div>
							{Intl.NumberFormat().format(pageIndex + 1)} / {Intl.NumberFormat().format(
								numberOfPages || 1
							)} pages
						</div>
					</div>
					<div class="flex gap-4">
						<button
							class="hover:text-base-content flex items-center disabled:opacity-50"
							disabled={isReachedMin}
							onclick={prevPage}
						>
							<ChevronLeft class="size-[1.4em]" /> Previous Page
						</button>
						<button
							class="hover:text-base-content flex items-center disabled:opacity-50"
							disabled={isReachedMax}
							onclick={nextPage}
						>
							Next Page <ChevronRight class="size-[1.4em]" />
						</button>
					</div>
				</div>
			{/if}
		</div>
	</div>
</Layout>
