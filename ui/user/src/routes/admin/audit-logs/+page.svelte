<script lang="ts">
	import { fade, slide } from 'svelte/transition';
	import { X, ChevronLeft, ChevronRight } from 'lucide-svelte';
	import { throttle } from 'es-toolkit';
	import { page } from '$app/state';
	import { afterNavigate, goto } from '$app/navigation';
	import { type DateRange } from '$lib/components/Calendar.svelte';
	import Layout from '$lib/components/Layout.svelte';
	import Search from '$lib/components/Search.svelte';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants';
	import {
		type OrgUser,
		type AuditLogURLFilters,
		AdminService,
		type AuditLog
	} from '$lib/services';
	import { getUser, type PaginatedResponse } from '$lib/services/admin/operations';
	import { clickOutside } from '$lib/actions/clickoutside';
	import { dialogAnimation } from '$lib/actions/dialogAnimation';
	import AuditLogDetails from '$lib/components/admin/audit-logs/AuditLogDetails.svelte';
	import AuditFilters from '$lib/components/admin/audit-logs/AuditFilters.svelte';
	import AuditLogsTable from './AuditLogs.svelte';
	import AuditLogsTimeline from './AuditLogsTimeline.svelte';
	import AuditLogCalendar from './AuditLogCalendar.svelte';
	import { endOfDay, set, subDays } from 'date-fns';
	import { localState } from '$lib/runes/localState.svelte';

	const duration = PAGE_TRANSITION_DURATION;

	let auditLogsResponse = $state<PaginatedResponse<AuditLog>>();
	const auditLogsTotalItems = $derived(auditLogsResponse?.total ?? 0);

	let pageIndexLocal = localState('@obot/auditlogs/page-index', {
		value: 0,
		parse: (ls) => (ls ? parseInt(ls) : 0)
	});
	const pageIndex = $derived(pageIndexLocal.current ?? 0);
	const pageLimit = $state(10000);

	const numberOfPages = $derived(Math.ceil(auditLogsTotalItems / pageLimit));
	const pageOffset = $derived(pageIndex * pageLimit);

	const remoteAuditLogs = $derived(auditLogsResponse?.items ?? []);

	const isReachedMax = $derived(pageIndex >= numberOfPages - 1);
	const isReachedMin = $derived(pageIndex <= 0);

	let fragmentIndex = $state(0);
	const fragmentLimit = $state(1000);
	const numberOfFragments = $derived(Math.ceil(remoteAuditLogs.length / fragmentLimit));
	const fragmentSliceStart = $derived(0);
	const fragmentSliceEnd = $derived(
		Math.min(remoteAuditLogs.length, (fragmentIndex + 1) * fragmentLimit)
	);

	const fragmentedAuditLogs = $derived(remoteAuditLogs.slice(fragmentSliceStart, fragmentSliceEnd));

	const users = new Map<string, OrgUser>();

	let showFilters = $state(false);
	let selectedAuditLog = $state<AuditLog & { user: string }>();
	let rightSidebar = $state<HTMLDialogElement>();

	let query = $state(page.url.searchParams.get('query') ?? '');

	const searchParamFilters = $derived.by<AuditLogURLFilters>(() => {
		return page.url.searchParams
			.entries()
			.filter(([key]) => {
				if (['query', 'limit', 'offset'].includes(key)) {
					return false;
				}

				return true;
			})
			.reduce(
				(acc, [key, value]) => {
					acc[key] = decodeURIComponent(value ?? '');
					return acc;
				},
				{} as Record<string, string>
			);
	});

	let timeRangeFilters = $derived.by(() => {
		const { start_time, end_time } = searchParamFilters;

		if (start_time || end_time) {
			const today = set(new Date(), { milliseconds: 0, seconds: 0 });

			return {
				startTime: set(new Date(start_time ?? today), { milliseconds: 0, seconds: 0 }),
				endTime: set(new Date(end_time ?? subDays(today, 7)), { milliseconds: 0, seconds: 0 })
			};
		}

		return {
			startTime: subDays(set(new Date(), { milliseconds: 0, seconds: 0 }), 7),
			endTime: set(new Date(new Date()), { milliseconds: 0, seconds: 0 })
		};
	});

	const allFilters = $derived({
		...searchParamFilters,
		start_time: timeRangeFilters.startTime.toISOString(),
		end_time: timeRangeFilters.endTime?.toISOString() ?? '',
		limit: pageLimit,
		offset: pageOffset,
		query: query
	});

	afterNavigate(() => {
		AdminService.listUsers().then((userData) => {
			for (const user of userData) {
				users.set(user.id, user);
			}
		});
	});

	$effect(() => {
		if (!allFilters) return;
		if (!pageIndexLocal.isReady) return;

		fetchAuditLogs({ ...allFilters }).then((res) => {
			// Reset page and page fragment indexes when the total results are less than the current page offset
			if (!res || pageOffset > (res?.total ?? 0)) {
				pageIndexLocal.current = 0;
				fragmentIndex = 0;
			}
		});
	});

	// Throttle query update
	const handleQueryChange = throttle((value: string) => {
		query = value;

		if (value) {
			page.url.searchParams.set('query', value);
		} else {
			page.url.searchParams.delete('query');
		}

		// Update the query search param without cause app to react
		// Prevent losing focus from the input
		history.replaceState(null, '', page.url);
	}, 100);

	async function nextPage() {
		if (isReachedMax) return;

		//Reset fragment index
		fragmentIndex = 0;
		pageIndexLocal.current = Math.min(numberOfPages, pageIndex + 1);

		fetchAuditLogs({ ...allFilters });
	}

	async function prevPage() {
		if (isReachedMin) return;

		//Reset fragment index
		fragmentIndex = 0;
		pageIndexLocal.current = Math.max(0, pageIndex - 1);

		fetchAuditLogs({ ...allFilters });
	}

	async function fetchAuditLogs(filters: typeof searchParamFilters) {
		const { mcp_id: mcpId } = filters;

		if (mcpId) {
			return (auditLogsResponse = await AdminService.listServerOrInstanceAuditLogs(mcpId, filters));
		} else {
			return (auditLogsResponse = await AdminService.listAuditLogs(filters));
		}
	}

	async function fetchUserById(id: string) {
		const cache = users.get(id);

		if (cache) {
			return cache;
		}

		const remote = await getUser(id);
		users.set(id, remote);

		return remote;
	}

	function getFilterDisplayLabel(key: keyof AuditLogURLFilters) {
		if (key === 'mcp_server_display_name') return 'Server';
		if (key === 'mcp_server_catalog_entry_name') return 'Server ID';
		if (key === 'mcp_id') return 'Server ID';
		if (key === 'start_time') return 'Start Time';
		if (key === 'end_time') return 'End Time';
		if (key === 'user_id') return 'User';
		if (key === 'client_name') return 'Client Name';
		if (key === 'client_version') return 'Client Version';
		if (key === 'call_type') return 'Call Type';
		if (key === 'session_id') return 'Session ID';
		if (key === 'response_status') return 'Response Status';
		if (key === 'client_ip') return 'Client IP';

		return key.replace(/_(\w)/g, ' $1');
	}

	async function getFilterValue(label: keyof AuditLogURLFilters, value: string | number) {
		if (label === 'start_time' || label === 'end_time') {
			return Promise.resolve(
				new Date(value).toLocaleString(undefined, {
					year: 'numeric',
					month: 'short',
					day: 'numeric',
					hour: '2-digit',
					minute: '2-digit',
					second: '2-digit',
					hour12: true,
					timeZoneName: 'short'
				})
			);
		}

		if (label === 'user_id') {
			return (await fetchUserById(value + '')).displayName;
		}

		return Promise.resolve(value + '');
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

		goto(url.toString(), { noScroll: true });
		pageIndexLocal.current = 0;
	}
</script>

<svelte:head>
	<title>Obot | Audit Logs</title>
</svelte:head>

<Layout classes={{ childrenContainer: 'max-w-none' }}>
	<div class="my-4 h-screen" in:fade={{ duration }} out:fade={{ duration }}>
		<div class="flex min-h-full flex-col gap-8 pb-8">
			<div class="flex items-center justify-between gap-4">
				<h1 class="text-2xl font-semibold">Audit Logs</h1>
			</div>

			<div class="flex gap-4">
				<Search
					class="dark:bg-surface1 dark:border-surface3 border border-transparent bg-white shadow-sm"
					onChange={handleQueryChange}
					placeholder="Search..."
					value={query}
				/>

				<AuditLogCalendar
					start={timeRangeFilters.startTime}
					end={timeRangeFilters.endTime}
					onChange={handleDateChange}
				/>

				<button
					class="dark:bg-surface1 dark:hover:bg-surface2/70 dark:active:bg-surface2 dark:border-surface3 flex w-full items-center justify-center gap-1 rounded-lg border border-transparent bg-white px-4 py-2 shadow-sm hover:bg-gray-50 focus:ring-2 focus:ring-blue-500 focus:ring-offset-2 focus:outline-none sm:w-auto"
					onclick={() => {
						showFilters = true;
						selectedAuditLog = undefined;
						rightSidebar?.show();
					}}
				>
					<svg
						class="size-4"
						xmlns="http://www.w3.org/2000/svg"
						viewBox="0 0 24 24"
						fill="none"
						stroke="currentColor"
						stroke-width="2"
						stroke-linecap="round"
						stroke-linejoin="round"
					>
						<polygon points="22 3 2 3 10 12.46 10 19 14 21 14 12.46 22 3"></polygon>
					</svg>

					Filters
				</button>
			</div>

			{@render filters()}

			<!-- Timeline Graph (Placeholder) -->
			<div
				class="dark:bg-surface2 dark:border-surface3 rounded-lg border border-transparent bg-white text-black shadow-sm dark:text-white"
			>
				<h3 class="mb-2 px-4 pt-4 text-lg font-medium">Timeline</h3>
				<div class="px-4">
					<div class="flex h-40 items-center justify-center rounded-md text-gray-500">
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
				data={fragmentedAuditLogs}
				currentFragmentIndex={fragmentIndex}
				getFragmentIndex={(rowIndex: number) => Math.floor(rowIndex / fragmentLimit)}
				getFragmentRowIndex={(rowIndex: number) => {
					const fragIndex = Math.floor(rowIndex / fragmentLimit);

					return rowIndex - fragIndex * fragmentLimit;
				}}
				onLoadNextFragment={(rowFragmentIndex: number) => {
					fragmentIndex = Math.min(numberOfFragments - 1, rowFragmentIndex + 1);
				}}
				onSelectRow={(d: typeof selectedAuditLog) => {
					selectedAuditLog = d;
					showFilters = false;
					rightSidebar?.show();
				}}
				{fetchUserById}
			>
				{#snippet emptyContent()}
					<!-- Just to skip ts checker, have to added later -->
				{/snippet}
			</AuditLogsTable>
		</div>
	</div>
</Layout>

<dialog
	bind:this={rightSidebar}
	use:clickOutside={[handleRightSidebarClose, true]}
	use:dialogAnimation={{ type: 'drawer' }}
	class="dark:border-surface1 dark:bg-surface1 fixed! top-0! right-0! bottom-0! left-auto! z-40 h-screen w-auto max-w-none rounded-none border-0 bg-white shadow-lg outline-none!"
>
	{#if selectedAuditLog}
		<AuditLogDetails onClose={handleRightSidebarClose} auditLog={selectedAuditLog} />
	{/if}
	{#if showFilters}
		<AuditFilters
			onClose={handleRightSidebarClose}
			filters={{ ...searchParamFilters }}
			{fetchUserById}
			{getFilterDisplayLabel}
		/>
	{/if}
</dialog>

{#snippet filters()}
	{@const keys = Object.keys(searchParamFilters) as (keyof AuditLogURLFilters)[]}
	{@const hasFilters = Object.values(searchParamFilters).some((value) => !!value)}

	{#if hasFilters}
		<div
			class="flex flex-wrap items-center gap-2"
			in:slide={{ duration: 200 }}
			out:slide={{ duration: 100 }}
		>
			{#each keys as key (key)}
				{@const displayLabel = getFilterDisplayLabel(key)}
				{@const values =
					searchParamFilters[key as keyof typeof searchParamFilters]
						?.toString()
						.split(',')
						.filter(Boolean) ?? []}

				<div
					class="flex items-center gap-1 rounded-lg border border-blue-500/50 bg-blue-500/10 px-4 py-2 text-blue-600 dark:text-blue-300"
				>
					<div class="text-xs font-semibold">
						<span>{displayLabel}</span>
						<span>:</span>
						{#each values as value (value)}
							{@const isMultiple = values.length > 1}

							{#await getFilterValue(key, value) then response}
								{#if isMultiple}
									<span class="font-light">
										<span>{response}</span>
									</span>

									<span class="mx-1 font-bold last:hidden">OR</span>
								{:else}
									<span class="font-light">{response}</span>
								{/if}
							{/await}
						{/each}
					</div>

					<button
						class="rounded-full p-1 transition-colors duration-200 hover:bg-blue-500/25"
						onclick={() => {
							const url = page.url;
							url.searchParams.delete(key);

							goto(url, { noScroll: true });
						}}
					>
						<X class="size-3" />
					</button>
				</div>
			{/each}
		</div>
	{/if}
{/snippet}
