<script lang="ts">
	import Layout from '$lib/components/Layout.svelte';
	import { AdminService } from '$lib/services';
	import type {
		OrgUser,
		PolicyViolation,
		PolicyViolationFilters,
		PolicyViolationStats
	} from '$lib/services/admin/types';
	import { PolicyDirectionLabels } from '$lib/services/admin/types';
	import type { PolicyDirection } from '$lib/services/admin/types';
	import { onMount } from 'svelte';
	import { SvelteMap } from 'svelte/reactivity';
	import { subDays, set } from 'date-fns';
	import AuditLogCalendar from '$lib/components/admin/audit-logs/AuditLogCalendar.svelte';
	import Search from '$lib/components/Search.svelte';
	import Select from '$lib/components/Select.svelte';
	import HorizontalBarGraph from '$lib/components/graph/HorizontalBarGraph.svelte';
	import StackedTimeline from '$lib/components/graph/StackedTimeline.svelte';
	import { fade } from 'svelte/transition';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants';
	import Loading from '$lib/icons/Loading.svelte';
	import { ShieldAlert, Funnel } from 'lucide-svelte';
	import { columnResize } from '$lib/actions/resize';
	import { responsive } from '$lib/stores';
	import { X } from 'lucide-svelte';
	import { getUserDisplayName } from '$lib/utils';

	const duration = PAGE_TRANSITION_DURATION;

	let violations = $state<PolicyViolation[]>([]);
	let stats = $state<PolicyViolationStats | null>(null);
	let total = $state(0);
	let selectedViolation = $state<PolicyViolation | null>(null);
	let detailedViolation = $state<PolicyViolation | null>(null);
	let loading = $state(true);
	let query = $state('');

	let startTime = $state(subDays(new Date(), 30));
	let endTime = $state(set(new Date(), { milliseconds: 0, seconds: 59 }));

	let filterDirection = $state('');
	let filterUserID = $state('');
	let filterPolicyID = $state('');

	let showFilters = $state(false);
	let userFilterOptions = $state<string[]>([]);
	let policyFilterOptions = $state<{ id: string; name: string }[]>([]);

	let pageOffset = $state(0);
	const pageLimit = 100;

	let rightSidebar = $state<HTMLDivElement>();

	let activeFilterCount = $derived(
		(filterUserID ? 1 : 0) + (filterPolicyID ? 1 : 0) + (filterDirection ? 1 : 0)
	);

	const users = new SvelteMap<string, OrgUser>();
	function displayName(id: string) {
		return getUserDisplayName(users, id);
	}

	function buildFilters(): PolicyViolationFilters {
		const filters: PolicyViolationFilters = {
			start_time: startTime.toISOString(),
			end_time: endTime.toISOString(),
			limit: pageLimit,
			offset: pageOffset
		};
		if (query) filters.query = query;
		if (filterDirection) filters.direction = filterDirection;
		if (filterUserID) filters.user_id = filterUserID;
		if (filterPolicyID) filters.policy_id = filterPolicyID;
		return filters;
	}

	async function fetchData() {
		loading = true;
		try {
			const filters = buildFilters();
			const [violationsResp, statsResp] = await Promise.all([
				AdminService.listPolicyViolations(filters),
				AdminService.getPolicyViolationStats(filters)
			]);
			violations = violationsResp.items ?? [];
			total = violationsResp.total;
			stats = statsResp;
		} finally {
			loading = false;
		}
	}

	async function viewDetail(v: PolicyViolation) {
		showFilters = false;
		selectedViolation = v;
		detailedViolation = null;
		rightSidebar?.showPopover();
		try {
			detailedViolation = await AdminService.getPolicyViolation(v.id);
		} catch {
			detailedViolation = v;
		}
	}

	function closeSidebar() {
		rightSidebar?.hidePopover();
		showFilters = false;
		selectedViolation = null;
		detailedViolation = null;
	}

	async function openFilters() {
		selectedViolation = null;
		detailedViolation = null;
		showFilters = true;
		rightSidebar?.showPopover();
		const [usersResp, policiesResp] = await Promise.all([
			AdminService.listPolicyViolationFilterOptions('user_id'),
			AdminService.listPolicyViolationFilterOptions('policy_name')
		]);
		userFilterOptions = usersResp ?? [];
		// Build policy name-to-ID mapping from stats if available
		const policyMap = new Map(stats?.byPolicy.map((p) => [p.policyName, p.policyID]) ?? []);
		policyFilterOptions = (policiesResp ?? []).map((name: string) => ({
			id: policyMap.get(name) ?? name,
			name
		}));
	}

	function applyFilter() {
		pageOffset = 0;
		fetchData();
	}

	function clearFilters() {
		filterUserID = '';
		filterPolicyID = '';
		filterDirection = '';
		pageOffset = 0;
		fetchData();
	}

	onMount(() => {
		fetchData();
		AdminService.listUsersIncludeDeleted().then((userData) => {
			for (const user of userData) {
				users.set(user.id, user);
			}
		});
	});

	function handleTimeRangeChange({ start, end }: { start: Date; end: Date }) {
		startTime = start;
		endTime = end;
		pageOffset = 0;
		fetchData();
	}

	function handleSearch(value: string) {
		query = value;
		pageOffset = 0;
		fetchData();
	}

	let directionLabel = (d: string) => PolicyDirectionLabels[d as PolicyDirection] ?? d;

	let totalPages = $derived(Math.ceil(total / pageLimit));
	let currentPage = $derived(Math.floor(pageOffset / pageLimit) + 1);

	const directionSelectOptions = [
		{ id: 'user-message', label: 'User Messages' },
		{ id: 'tool-calls', label: 'Tool Calls' }
	];
	let userSelectOptions = $derived(
		userFilterOptions.map((uid) => ({ id: uid, label: displayName(uid) }))
	);
	let policySelectOptions = $derived(
		policyFilterOptions.map((p) => ({ id: p.id, label: p.name }))
	);
</script>

<svelte:head>
	<title>Obot | Policy Violations</title>
</svelte:head>

<Layout
	classes={{ childrenContainer: 'max-w-none', container: '' }}
	title="Policy Violations"
>
	<div class="flex-1" in:fade={{ duration }} out:fade={{ duration }}>
		{#if loading}
			<div
				class="absolute inset-0 z-20 flex items-center justify-center"
				in:fade={{ duration: 100 }}
				out:fade|global={{ duration: 300, delay: 500 }}
			>
				<div
					class="bg-surface3/50 border-surface3 text-primary dark:text-primary flex flex-col items-center gap-4 rounded-2xl border px-16 py-8 shadow-md backdrop-blur-[1px]"
				>
					<Loading class="size-32 stroke-1" />
					<div class="text-2xl font-semibold">Loading violations...</div>
				</div>
			</div>
		{/if}

		<div class="flex min-h-full flex-col gap-8 pb-8">
			<!-- Filters -->
			<div class="flex flex-col gap-4 md:flex-row">
				<Search
					class="dark:bg-surface1 dark:border-surface3 bg-background border border-transparent shadow-sm"
					value={query}
					placeholder="Search violations..."
					onChange={handleSearch}
				/>

				<div class="flex gap-4 self-start md:self-end">
					<AuditLogCalendar start={startTime} end={endTime} onChange={handleTimeRangeChange} />

					<button
						class="hover:bg-surface1 dark:bg-surface1 dark:hover:bg-surface3 dark:border-surface3 button bg-background relative flex w-fit items-center justify-center gap-1 rounded-lg border border-transparent text-sm shadow-sm"
						onclick={openFilters}
					>
						<Funnel class="size-4" />
						Filters
						{#if activeFilterCount > 0}
							<span
								class="bg-primary absolute -top-1.5 -right-1.5 flex size-4 items-center justify-center rounded-full text-[10px] font-bold text-white"
							>
								{activeFilterCount}
							</span>
						{/if}
					</button>
				</div>
			</div>

			<!-- Stats -->
			{#if stats && !loading}
				<div class="grid grid-cols-1 gap-4 md:grid-cols-3">
					<div
						class="dark:bg-surface2 dark:border-surface3 bg-background flex flex-col gap-1 rounded-lg border border-transparent p-4 shadow-sm"
					>
						<span class="text-on-surface2 text-xs font-medium uppercase">Total Violations</span>
						<span class="text-on-surface1 text-2xl font-bold">{total}</span>
					</div>

					<div
						class="dark:bg-surface2 dark:border-surface3 bg-background flex flex-col gap-1 rounded-lg border border-transparent p-4 shadow-sm"
					>
						<span class="text-on-surface2 text-xs font-medium uppercase">User Message Violations</span>
						<span class="text-on-surface1 text-2xl font-bold"
							>{stats.byDirection.userMessage}</span
						>
					</div>
					<div
						class="dark:bg-surface2 dark:border-surface3 bg-background flex flex-col gap-1 rounded-lg border border-transparent p-4 shadow-sm"
					>
						<span class="text-on-surface2 text-xs font-medium uppercase">Tool Call Violations</span>
						<span class="text-on-surface1 text-2xl font-bold">{stats.byDirection.toolCalls}</span>
					</div>
				</div>

				<!-- Charts row -->
				{#if stats.byPolicy.length > 0 || stats.byUser.length > 0}
					<div class="grid grid-cols-1 gap-4 md:grid-cols-2">
						{#if stats.byPolicy.length > 0}
							<div
								class="dark:bg-surface2 dark:border-surface3 bg-background flex flex-col gap-2 rounded-lg border border-transparent p-4 shadow-sm"
							>
								<span class="text-on-surface2 text-xs font-medium uppercase">By Policy</span>
								<div class="h-[200px]">
									<HorizontalBarGraph data={stats.byPolicy} labelKey="policyName" valueKey="count" />
								</div>
							</div>
						{/if}
						{#if stats.byUser.length > 0}
							<div
								class="dark:bg-surface2 dark:border-surface3 bg-background flex flex-col gap-2 rounded-lg border border-transparent p-4 shadow-sm"
							>
								<span class="text-on-surface2 text-xs font-medium uppercase">Top Users</span>
								<div class="h-[200px]">
									<HorizontalBarGraph
										data={stats.byUser.slice(0, 10).map((u) => ({
											...u,
											displayName: displayName(u.userID)
										}))}
										labelKey="displayName"
										valueKey="count"
									/>
								</div>
							</div>
						{/if}
					</div>
				{/if}

				<!-- Timeline -->
				<div
					class="dark:bg-surface2 dark:border-surface3 bg-background text-on-background rounded-lg border border-transparent shadow-sm"
				>
					<h3 class="mb-2 px-4 pt-4 text-lg font-medium">Timeline</h3>
					<div class="px-4 pb-4">
						<div class="text-on-surface1 flex h-40 items-center justify-center rounded-md">
							<StackedTimeline
								start={startTime}
								end={endTime}
								data={stats.byTime
									.filter((b) => b.count > 0)
									.map((b) => ({
										createdAt: b.time,
										policyName: b.policyName,
										count: b.count,
										_secondary: 0 as const
									}))}
								categoryKey="policyName"
								dateKey="createdAt"
								primaryValueKey="count"
								secondaryValueKey="_secondary"
							/>
						</div>
					</div>
				</div>
			{/if}

			<!-- Table -->
			{#if !loading && violations.length === 0}
				<div
					class="mt-12 flex w-md max-w-full flex-col items-center gap-4 self-center text-center"
				>
					<ShieldAlert class="text-on-surface1 size-24 opacity-50" />
					<h4 class="text-on-surface1 text-lg font-semibold">No policy violations</h4>
					<p class="text-on-surface text-sm font-light">
						Currently, there are no policy violations for the selected range or filters. Try
						modifying your search criteria or try again later.
					</p>
				</div>
			{:else if violations.length > 0}
				<div
					class="dark:bg-surface2 bg-background flex w-full min-w-full flex-1 divide-y divide-gray-200 overflow-x-auto overflow-y-visible rounded-lg border border-transparent shadow-sm"
				>
					<table class="w-full flex-1 table-fixed border-collapse border-spacing-0">
						<thead>
							<tr>
								<th
									class="dark:bg-surface1 bg-surface2 text-on-surface1 sticky top-0 box-content w-[4ch] px-6 py-3 text-left text-xs font-medium tracking-wider uppercase"
								>
									#
								</th>
								<th
									class="dark:bg-surface1 bg-surface2 text-on-surface1 sticky top-0 box-content w-[34ch] px-6 py-3 text-left text-xs font-medium tracking-wider uppercase"
								>
									Timestamp
								</th>
								<th
									class="dark:bg-surface1 bg-surface2 text-on-surface1 sticky top-0 box-content w-[24ch] px-6 py-3 text-left text-xs font-medium tracking-wider uppercase"
								>
									User
								</th>
								<th
									class="dark:bg-surface1 bg-surface2 text-on-surface1 sticky top-0 box-content w-[24ch] px-6 py-3 text-left text-xs font-medium tracking-wider uppercase"
								>
									Policy
								</th>
								<th
									class="dark:bg-surface1 bg-surface2 text-on-surface1 sticky top-0 box-content w-[24ch] px-6 py-3 text-left text-xs font-medium tracking-wider uppercase"
								>
									Direction
								</th>
							</tr>
						</thead>
						<tbody>
							{#each violations as v, i (v.id)}
								<tr
									class="hover:bg-surface1 dark:hover:bg-surface3 group h-14 cursor-pointer text-sm transition-colors duration-300"
									onclick={() => viewDetail(v)}
								>
									<td class="px-6 py-3">{pageOffset + i + 1}</td>
									<td class="whitespace-nowrap">
										<div class="truncate px-6 py-4">
											{new Date(v.createdAt)
												.toLocaleString(undefined, {
													year: 'numeric',
													month: 'short',
													day: 'numeric',
													hour: '2-digit',
													minute: '2-digit',
													second: '2-digit',
													hour12: true,
													timeZoneName: 'short'
												})
												.replace(/,/g, '')}
										</div>
									</td>
									<td class="whitespace-nowrap">
										<div class="truncate px-6 py-4">{displayName(v.userID)}</div>
									</td>
									<td class="whitespace-nowrap">
										<div class="truncate px-6 py-4">{v.policyName}</div>
									</td>
									<td class="whitespace-nowrap">
										<div class="truncate px-6 py-4">{directionLabel(v.direction)}</div>
									</td>
								</tr>
							{/each}
						</tbody>
					</table>
				</div>

				<!-- Pagination -->
				{#if totalPages > 1}
					<div
						class="dark:bg-surface2 bg-background flex items-center justify-between gap-2 rounded-lg border border-transparent px-4 py-3 text-xs text-gray-600 shadow-sm"
					>
						<div class="flex gap-4">
							<div>
								Showing {pageOffset + 1}-{Math.min(pageOffset + pageLimit, total)} of {total}
							</div>
							<div class="flex items-center">
								<span>{currentPage}</span>/<span>{totalPages}</span>
								<span class="ml-1">pages</span>
							</div>
						</div>
						<div class="flex gap-4">
							<button
								class="hover:text-on-surface1/80 active:text-on-surface1/100 flex items-center text-xs transition-colors duration-100 disabled:pointer-events-none disabled:opacity-50"
								disabled={pageOffset === 0}
								onclick={() => {
									pageOffset = Math.max(0, pageOffset - pageLimit);
									fetchData();
								}}
							>
								Previous
							</button>
							<button
								class="hover:text-on-surface1/80 active:text-on-surface1/100 flex items-center text-xs transition-colors duration-100 disabled:pointer-events-none disabled:opacity-50"
								disabled={currentPage >= totalPages}
								onclick={() => {
									pageOffset += pageLimit;
									fetchData();
								}}
							>
								Next
							</button>
						</div>
					</div>
					{/if}
			{/if}
		</div>
	</div>

	<!-- Detail drawer -->
	<div
		bind:this={rightSidebar}
		popover
		class="drawer max-w-[85vw] min-w-lg"
		style="width: 32rem"
	>
		{#if !responsive.isMobile && rightSidebar}
			<div
				role="none"
				class="absolute top-0 left-0 z-30 h-full w-3 cursor-col-resize"
				use:columnResize={{ column: rightSidebar, direction: 'right' }}
			></div>
		{/if}

		{#if showFilters}
			<div
				class="dark:bg-gray-990 text-on-background flex h-full w-[inherit] min-w-[inherit] flex-col bg-gray-50"
			>
				<div
					class="dark:bg-surface1 bg-background relative flex w-full items-center justify-between p-4 pl-5 shadow-xs"
				>
					<div class="bg-primary absolute top-0 left-0 h-full w-1"></div>
					<h3 class="text-lg font-semibold">Filters</h3>
					<button onclick={closeSidebar} class="icon-button">
						<X class="size-5" />
					</button>
				</div>

				<div class="default-scrollbar-thin relative flex-1 overflow-y-auto">
					<div class="bg-surface2 absolute top-0 left-0 h-full w-1"></div>

					<div class="flex flex-col gap-6 p-4 pl-5">
						<div class="flex flex-col gap-1">
							<div class="flex items-center justify-between">
								<span class="text-md font-light">By Direction</span>
								{#if filterDirection}
									<button
										class="text-xs opacity-50 transition-opacity duration-200 hover:opacity-80 active:opacity-100"
										onclick={() => {
											filterDirection = '';
											applyFilter();
										}}
									>
										{filterDirection.includes(',') ? 'Clear All' : 'Clear'}
									</button>
								{/if}
							</div>
							<Select
								id="filter-direction"
								class="dark:border-surface3 bg-surface1 dark:bg-background border border-transparent shadow-inner"
								classes={{ root: 'w-full', clear: 'hover:bg-surface3 bg-transparent' }}
								options={directionSelectOptions}
								bind:selected={filterDirection}
								multiple
								onSelect={() => applyFilter()}
								onClear={() => applyFilter()}
							/>
						</div>

						<div class="flex flex-col gap-1">
							<div class="flex items-center justify-between">
								<span class="text-md font-light">By User</span>
								{#if filterUserID}
									<button
										class="text-xs opacity-50 transition-opacity duration-200 hover:opacity-80 active:opacity-100"
										onclick={() => {
											filterUserID = '';
											applyFilter();
										}}
									>
										{filterUserID.includes(',') ? 'Clear All' : 'Clear'}
									</button>
								{/if}
							</div>
							<Select
								id="filter-user"
								class="dark:border-surface3 bg-surface1 dark:bg-background border border-transparent shadow-inner"
								classes={{ root: 'w-full', clear: 'hover:bg-surface3 bg-transparent' }}
								options={userSelectOptions}
								bind:selected={filterUserID}
								multiple
								searchable
								onSelect={() => applyFilter()}
								onClear={() => applyFilter()}
							/>
						</div>

						<div class="flex flex-col gap-1">
							<div class="flex items-center justify-between">
								<span class="text-md font-light">By Policy</span>
								{#if filterPolicyID}
									<button
										class="text-xs opacity-50 transition-opacity duration-200 hover:opacity-80 active:opacity-100"
										onclick={() => {
											filterPolicyID = '';
											applyFilter();
										}}
									>
										{filterPolicyID.includes(',') ? 'Clear All' : 'Clear'}
									</button>
								{/if}
							</div>
							<Select
								id="filter-policy"
								class="dark:border-surface3 bg-surface1 dark:bg-background border border-transparent shadow-inner"
								classes={{ root: 'w-full', clear: 'hover:bg-surface3 bg-transparent' }}
								options={policySelectOptions}
								bind:selected={filterPolicyID}
								multiple
								searchable
								onSelect={() => applyFilter()}
								onClear={() => applyFilter()}
							/>
						</div>

						{#if activeFilterCount > 0}
							<button
								class="text-primary hover:text-primary/80 self-start text-sm font-medium"
								onclick={clearFilters}
							>
								Clear all filters
							</button>
						{/if}
					</div>
				</div>
			</div>
		{:else if selectedViolation}
			<div
				class="dark:bg-gray-990 text-on-background flex h-full w-[inherit] min-w-[inherit] flex-col bg-gray-50"
			>
				<div
					class="dark:bg-surface1 bg-background relative flex w-full items-center justify-between p-4 pl-5 shadow-xs"
				>
					<div class="bg-primary absolute top-0 left-0 h-full w-1"></div>
					<h3 class="text-lg font-semibold">Violation Detail</h3>
					<button onclick={closeSidebar} class="icon-button">
						<X class="size-5" />
					</button>
				</div>

				<div class="default-scrollbar-thin relative flex-1 overflow-y-auto pb-4">
					<div class="bg-surface2 absolute top-0 left-0 h-full w-1"></div>

					{#if detailedViolation}
						<div class="flex flex-wrap gap-2 p-4 pl-5">
							<div
								class="dark:bg-surface3 bg-surface2 rounded-full px-3 py-1 text-[11px] font-light"
							>
								<span class="font-medium">Policy:</span>
								{detailedViolation.policyName}
							</div>
							<div
								class="dark:bg-surface3 bg-surface2 rounded-full px-3 py-1 text-[11px] font-light"
							>
								<span class="font-medium">Direction:</span>
								{directionLabel(detailedViolation.direction)}
							</div>
							{#if detailedViolation.projectID}
								<div
									class="dark:bg-surface3 bg-surface2 rounded-full px-3 py-1 text-[11px] font-light"
								>
									<span class="font-medium">Project:</span>
									{detailedViolation.projectID}
								</div>
							{/if}
							{#if detailedViolation.threadID}
								<div
									class="dark:bg-surface3 bg-surface2 rounded-full px-3 py-1 text-[11px] font-light"
								>
									<span class="font-medium">Thread:</span>
									{detailedViolation.threadID}
								</div>
							{/if}
						</div>

						<div class="p-4 pl-5">
							<div class="flex flex-col gap-1 text-sm font-light">
								<p>
									<span class="font-medium">Timestamp</span>:
									{new Date(detailedViolation.createdAt).toLocaleString()}
								</p>
								<p>
									<span class="font-medium">User</span>:
									{displayName(detailedViolation.userID)}
								</p>
							</div>

							<p class="mt-6 mb-2 text-base font-semibold">Policy Definition</p>
							<p class="text-sm font-light">{detailedViolation.policyDefinition}</p>

							<p class="mt-6 mb-2 text-base font-semibold">Explanation</p>
							<p class="text-sm font-light">{detailedViolation.violationExplanation}</p>

							{#if detailedViolation.blockedContent}
								<p class="mt-6 mb-2 text-base font-semibold">Blocked Content</p>
								<div
									class="dark:bg-surface2 bg-background relative overflow-hidden rounded-md p-4 pl-5"
								>
									<div class="bg-primary/50 absolute top-0 left-0 h-full w-1"></div>
									<pre
										class="default-scrollbar-thin max-h-96 overflow-y-auto text-sm break-words whitespace-pre-wrap">{JSON.stringify(
											detailedViolation.blockedContent,
											null,
											2
										)}</pre>
								</div>
							{/if}
						</div>
					{:else}
						<div
							class="text-on-surface1 flex items-center justify-center gap-2 py-12 text-sm font-light"
						>
							<Loading class="size-5 animate-spin" />
							<span>Loading details...</span>
						</div>
					{/if}
				</div>
			</div>
		{/if}

	</div>
</Layout>
