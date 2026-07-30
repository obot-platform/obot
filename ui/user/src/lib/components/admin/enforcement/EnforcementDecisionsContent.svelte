<script lang="ts">
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import { columnResize } from '$lib/actions/resize';
	import { buildPillSearchParamFilters, buildSearchParamFiltersArray } from '$lib/auditlogs';
	import Search from '$lib/components/Search.svelte';
	import AuditLogCalendar from '$lib/components/admin/audit-logs/AuditLogCalendar.svelte';
	import AuditLogFilterPills from '$lib/components/admin/audit-logs/AuditLogFilterPills.svelte';
	import AuditLogTableSkeleton from '$lib/components/admin/audit-logs/AuditLogTableSkeleton.svelte';
	import FiltersDrawer from '$lib/components/admin/filters-drawer/FiltersDrawer.svelte';
	import { setVirtualPageData } from '$lib/components/ui/virtual-page/context';
	import { agentLabel, kindLabel } from '$lib/enforcement';
	import { isAbortError } from '$lib/errors';
	import {
		AdminService,
		type EnforcementDecisionEvent,
		type EnforcementDecisionURLFilters,
		type MDMConfiguration
	} from '$lib/services';
	import type { PaginatedResponse } from '$lib/services/http';
	import { profile, responsive } from '$lib/stores';
	import { goto, replaceState } from '$lib/url';
	import EnforcementDecisionDetails from './EnforcementDecisionDetails.svelte';
	import EnforcementDecisionsTable from './EnforcementDecisionsTable.svelte';
	import {
		ChevronLeft,
		ChevronRight,
		CircleAlert,
		Funnel,
		ShieldCheck,
		TriangleAlert
	} from '@lucide/svelte';
	import { endOfDay, set, subDays } from 'date-fns';
	import { debounce } from 'es-toolkit';
	import { onMount } from 'svelte';
	import { SvelteMap } from 'svelte/reactivity';
	import { twMerge } from 'tailwind-merge';

	type SupportedFilter = keyof EnforcementDecisionURLFilters;
	// mdm_configuration_id is deliberately absent: the Devices page surfaces exactly
	// one configuration, so a filter for it would be a control with one option. It
	// stays in the type so a deep link keeps working if multi-fleet support lands.
	const supportedFilters: SupportedFilter[] = [
		'decision',
		'agent',
		'kind',
		'server',
		'tool',
		'actor'
	];
	const pageLimit = 1000;

	let loading = $state(true);
	let fetchError = $state<string | null>(null);
	let response = $state<PaginatedResponse<EnforcementDecisionEvent>>();
	let allowedTotal = $state<number>();
	let blockedTotal = $state<number>();
	let pageIndex = $state(0);
	let showFilters = $state(false);
	let rightSidebar = $state<HTMLDivElement>();
	let selectedDecision = $state<EnforcementDecisionEvent>();
	let configuration = $state<MDMConfiguration>();
	let refreshToken = $state(0);
	const deviceNames = new SvelteMap<string, string>();

	let isAdminReadonly = $derived(profile.current.isAdminReadonly?.());

	const total = $derived(response?.total ?? 0);
	const numberOfPages = $derived(Math.ceil(total / pageLimit));
	const isReachedMin = $derived(pageIndex <= 0);
	const isReachedMax = $derived(pageIndex >= numberOfPages - 1);

	let query = $derived(page.url.searchParams.get('query') ?? '');

	const DEFER_THRESHOLD = 500;
	let displayTableData = $state<EnforcementDecisionEvent[]>([]);
	$effect(() => {
		const items = response?.items ?? [];

		if (items.length <= DEFER_THRESHOLD) {
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
		buildSearchParamFiltersArray<EnforcementDecisionURLFilters>(supportedFilters)
	);
	const searchParamFilters = $derived.by<EnforcementDecisionURLFilters>(() => {
		return searchParamFiltersAsArray.reduce(
			(acc, [key, value]) => {
				acc[key!] = value;
				return acc;
			},
			{} as Record<string, unknown>
		);
	});

	const pillsSearchParamFilters = $derived(
		buildPillSearchParamFilters<EnforcementDecisionURLFilters>(searchParamFiltersAsArray)
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

	const filters = $derived<EnforcementDecisionURLFilters>({
		...pillsSearchParamFilters,
		start_time: timeRangeFilters.startTime.toISOString(),
		end_time: timeRangeFilters.endTime.toISOString(),
		limit: pageLimit,
		offset: pageOffset,
		query
	});

	const filterPaginationKey = $derived(
		JSON.stringify({
			...pillsSearchParamFilters,
			query,
			start_time: timeRangeFilters.startTime.toISOString(),
			end_time: timeRangeFilters.endTime.toISOString()
		})
	);

	// The filters that scope the tile counts, enumerated rather than spread so that
	// three keys structurally cannot leak in. Paging is excluded because a count is
	// the same whichever page is showing, and inheriting offset would refetch both
	// counts on every page turn. The decision filter is excluded because with
	// decision=deny selected, a tile that inherited it would read "Allowed 0".
	// A new filter has to be added here to scope the counts.
	const countFilters = $derived.by<EnforcementDecisionURLFilters>(() => ({
		actor: pillsSearchParamFilters.actor,
		agent: pillsSearchParamFilters.agent,
		kind: pillsSearchParamFilters.kind,
		server: pillsSearchParamFilters.server,
		tool: pillsSearchParamFilters.tool,
		start_time: timeRangeFilters.startTime.toISOString(),
		end_time: timeRangeFilters.endTime.toISOString(),
		query
	}));

	$effect(() => {
		void filterPaginationKey;
		pageIndex = 0;
	});

	$effect(() => {
		const controller = new AbortController();
		const currentFilters = filters;
		void refreshToken;

		loading = true;
		fetchError = null;

		AdminService.listEnforcementDecisions(currentFilters, { signal: controller.signal })
			.then((res) => {
				if (controller.signal.aborted) return;
				response = res;
				if (pageOffset > (res?.total ?? 0)) {
					pageIndex = 0;
				}
			})
			.catch((err) => {
				if (isAbortError(err) || controller.signal.aborted) return;
				console.error('Failed to fetch enforcement decisions:', err);
				fetchError = err instanceof Error ? err.message : 'Failed to load enforcement decisions';
			})
			.finally(() => {
				if (controller.signal.aborted) return;
				loading = false;
			});

		return () => controller.abort();
	});

	// Both tiles stay meaningful regardless of the decision filter, and each doubles
	// as a one-click filter for its own verdict.
	$effect(() => {
		const controller = new AbortController();
		void refreshToken;
		const currentCountFilters = countFilters;

		allowedTotal = undefined;
		blockedTotal = undefined;

		for (const [verdict, assign] of [
			['allow', (value: number) => (allowedTotal = value)],
			['deny', (value: number) => (blockedTotal = value)]
		] as const) {
			AdminService.listEnforcementDecisions(
				{ ...currentCountFilters, decision: verdict, limit: 1, offset: 0 },
				{ signal: controller.signal }
			)
				.then((res) => {
					if (controller.signal.aborted) return;
					assign(res.total ?? 0);
				})
				.catch(() => {
					// A failed count leaves its tile blank; the list request owns error
					// reporting.
				});
		}

		return () => controller.abort();
	});

	// Device IDs are opaque, so resolve them to hostnames once. This is not on the
	// critical path: without it every device simply renders as its ID.
	onMount(() => {
		void (async () => {
			try {
				const configurations = await AdminService.listMDMConfigurations();
				const current =
					configurations.find((candidate) => candidate.isDefault) ?? configurations[0];
				if (!current) return;
				configuration = current;
				const devices = await AdminService.listMDMDevices(current.id);
				for (const device of devices) {
					if (device.hostname) deviceNames.set(device.deviceID, device.hostname);
				}
			} catch {
				// Leave the map empty and fall back to raw device IDs.
			}
		})();
	});

	function getDeviceDisplayName(deviceID?: string): string {
		if (!deviceID) return '';
		return deviceNames.get(deviceID) ?? deviceID;
	}

	const handleQueryChange = debounce((value: string) => {
		query = value;

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

	function toggleDecisionFilter(verdict: 'allow' | 'deny') {
		const url = page.url;
		if (url.searchParams.get('decision') === verdict) {
			url.searchParams.delete('decision');
		} else {
			url.searchParams.set('decision', verdict);
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
		const labels: Record<string, string> = {
			actor: 'Device',
			agent: 'Agent',
			decision: 'Result',
			kind: 'Tool Type',
			server: 'MCP Server',
			tool: 'Tool'
		};
		return labels[key] ?? key.replace(/_(\w)/g, ' $1');
	}

	function getFilterOptionLabel(key: string, value: string) {
		if (key === 'decision') return value === 'allow' ? 'Allowed' : 'Blocked';
		if (key === 'agent') return agentLabel(value);
		if (key === 'kind') return kindLabel(value);
		if (key === 'actor') return getDeviceDisplayName(value);
		return value;
	}

	function handleRightSidebarClose() {
		rightSidebar?.hidePopover();
		selectedDecision = undefined;
		showFilters = false;
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
		<div class="flex gap-4 self-start @min-[768px]:self-end">
			<AuditLogCalendar
				start={timeRangeFilters.startTime}
				end={timeRangeFilters.endTime}
				onChange={handleDateChange}
			/>
			<button
				class="btn btn-neutral h-12.5"
				onclick={() => {
					showFilters = true;
					selectedDecision = undefined;
					rightSidebar?.showPopover();
				}}
			>
				<Funnel class="size-4" />
				Filters
			</button>
		</div>
	</div>

	<div class="flex flex-wrap gap-4">
		{#each [{ verdict: 'allow', label: 'Allowed', value: allowedTotal }, { verdict: 'deny', label: 'Blocked', value: blockedTotal }] as const as tile (tile.verdict)}
			<button
				type="button"
				aria-pressed={pillsSearchParamFilters.decision === tile.verdict}
				class={twMerge(
					'dark:bg-base-200 bg-base-100 flex min-w-40 flex-col gap-1 rounded-lg border p-4 text-left shadow-sm transition-colors',
					pillsSearchParamFilters.decision === tile.verdict
						? 'border-primary'
						: 'dark:border-base-400 border-transparent hover:border-primary/40'
				)}
				onclick={() => toggleDecisionFilter(tile.verdict)}
			>
				<span class="text-muted-content text-xs font-medium tracking-wider uppercase">
					{tile.label}
				</span>
				{#if tile.value === undefined}
					<span class="bg-base-300 dark:bg-base-400 h-7 w-16 animate-pulse rounded"></span>
				{:else}
					<span
						class={twMerge(
							'text-2xl font-semibold',
							tile.verdict === 'deny' && tile.value > 0 && 'text-error'
						)}
					>
						{Intl.NumberFormat().format(tile.value)}
					</span>
				{/if}
			</button>
		{/each}
	</div>

	{#if configuration && !configuration.enforcementEnabled}
		<div class="notification-alert flex items-start gap-2.5 p-2.5">
			<TriangleAlert class="size-4 shrink-0" />
			<span class="text-xs">
				Enforcement is currently disabled, so no new decisions are being recorded.
				<a class="text-link" href={resolve('/admin/devices')}>Enable it on the Devices page.</a>
			</span>
		</div>
	{/if}

	{#if hasFilterPills}
		<AuditLogFilterPills
			{pillsSearchParamFilters}
			{getFilterDisplayLabel}
			getFilterValue={(key, value) => getFilterOptionLabel(key.toString(), value.toString())}
		/>
	{/if}
</div>

{#if loading}
	<AuditLogTableSkeleton />
{:else if fetchError}
	<div class="notification-error flex w-full items-center gap-3 p-4">
		<CircleAlert class="size-5 shrink-0" />
		<div class="flex flex-col gap-1">
			<p class="text-sm font-semibold">Unable to load enforcement decisions</p>
			<p class="text-sm font-light">{fetchError}</p>
		</div>
	</div>
{:else if displayTableData.length > 0}
	<EnforcementDecisionsTable
		{getDeviceDisplayName}
		onSelectRow={(decision) => {
			showFilters = false;
			selectedDecision = decision;
			rightSidebar?.showPopover();
		}}
	/>
{:else}
	<div class="flex w-full flex-col items-center justify-center gap-4 px-6 py-16 text-center">
		<ShieldCheck class="text-muted-content size-20 opacity-50" />
		<h4 class="text-muted-content text-lg font-semibold">No enforcement decisions</h4>
		<p class="text-muted-content max-w-md text-sm font-light">
			Nothing has been recorded for this range. Decisions are only logged while enforcement is
			enabled for the fleet.
		</p>
	</div>
{/if}

<div class="flex grow"></div>

{#if !loading && total > 0 && numberOfPages > 1}
	<div class="bg-base-200 dark:bg-base-100 sticky bottom-0 left-0 z-50 w-full py-4">
		<div class="text-muted-content flex items-center justify-between gap-4 px-1 text-sm">
			<button
				class="hover:text-base-content flex items-center gap-1 disabled:opacity-50"
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
				class="hover:text-base-content flex items-center gap-1 disabled:opacity-50"
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
	class={twMerge('drawer-legacy', selectedDecision ? 'max-w-[85vw] min-w-lg' : 'md:w-lg lg:w-xl')}
	style={selectedDecision ? 'width: 32rem' : ''}
>
	{#if selectedDecision && !responsive.isMobile && rightSidebar}
		<div
			role="none"
			class="absolute top-0 left-0 z-30 h-full w-3 cursor-col-resize"
			use:columnResize={{ column: rightSidebar, direction: 'right' }}
		></div>
	{/if}
	{#if selectedDecision}
		<EnforcementDecisionDetails
			decision={selectedDecision}
			deviceName={getDeviceDisplayName(selectedDecision.deviceID)}
			readOnly={isAdminReadonly}
			onClose={handleRightSidebarClose}
			onAllowlistUpdated={() => (refreshToken += 1)}
		/>
	{:else if showFilters}
		<FiltersDrawer
			onClose={handleRightSidebarClose}
			filters={{ ...searchParamFilters }}
			isFilterDisabled={() => false}
			isFilterMultiSelect={() => true}
			getUserDisplayName={(id: string) => id}
			{getFilterDisplayLabel}
			{getFilterOptionLabel}
			endpoint={async (filterId, opts) => {
				const response = await AdminService.listEnforcementDecisionFilterOptions(filterId, {
					...opts,
					start_time: timeRangeFilters.startTime.toISOString(),
					end_time: timeRangeFilters.endTime.toISOString()
				});
				return { options: response?.options ?? [] };
			}}
		/>
	{/if}
</div>
