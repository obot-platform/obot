<script lang="ts">
	import { columnResize } from '$lib/actions/resize';
	import Layout from '$lib/components/Layout.svelte';
	import Select from '$lib/components/Select.svelte';
	import AuditLogCalendar from '$lib/components/admin/audit-logs/AuditLogCalendar.svelte';
	import StackedTimeline from '$lib/components/graph/StackedTimeline.svelte';
	import IconButton from '$lib/components/primitives/IconButton.svelte';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants';
	import Loading from '$lib/icons/Loading.svelte';
	import { AdminService } from '$lib/services';
	import type {
		OrgUser,
		MessagePolicyViolation,
		MessagePolicyViolationFilters,
		MessagePolicyViolationStats
	} from '$lib/services/admin/types';
	import { PolicyDirectionLabels } from '$lib/services/admin/types';
	import type { PolicyDirection } from '$lib/services/admin/types';
	import { responsive, timePreference } from '$lib/stores';
	import { formatLogTimestamp } from '$lib/time';
	import { getUserDisplayName } from '$lib/utils';
	import { subDays, set } from 'date-fns';
	import { ShieldAlert } from 'lucide-svelte';
	import { X } from 'lucide-svelte';
	import { onMount } from 'svelte';
	import { SvelteMap } from 'svelte/reactivity';
	import { fade } from 'svelte/transition';
	import { slide } from 'svelte/transition';

	const duration = PAGE_TRANSITION_DURATION;

	let violations = $state<MessagePolicyViolation[]>([]);
	let stats = $state<MessagePolicyViolationStats | null>(null);
	let total = $state(0);
	let selectedViolation = $state<MessagePolicyViolation | null>(null);
	let detailedViolation = $state<MessagePolicyViolation | null>(null);
	let loading = $state(true);

	let startTime = $state(subDays(new Date(), 30));
	let endTime = $state(set(new Date(), { milliseconds: 0, seconds: 59 }));

	let filterDirection = $state('all_directions');
	let filterUserID = $state('all_users');
	let filterPolicyID = $state('all_policies');

	let userFilterOptions = $state<string[]>([]);
	let policyFilterOptions = $state<{ id: string; name: string }[]>([]);

	let groupBy = $state('group_by_policy');

	let pageOffset = $state(0);
	const pageLimit = 100;

	let rightSidebar = $state<HTMLDivElement>();

	const users = new SvelteMap<string, OrgUser>();
	function displayName(id: string) {
		return getUserDisplayName(users, id);
	}

	function buildFilters(): MessagePolicyViolationFilters {
		const filters: MessagePolicyViolationFilters = {
			start_time: startTime.toISOString(),
			end_time: endTime.toISOString(),
			limit: pageLimit,
			offset: pageOffset
		};
		if (filterDirection && filterDirection !== 'all_directions')
			filters.direction = filterDirection;
		if (filterUserID && filterUserID !== 'all_users') filters.user_id = filterUserID;
		if (filterPolicyID && filterPolicyID !== 'all_policies') filters.policy_id = filterPolicyID;
		return filters;
	}

	async function fetchData() {
		loading = true;
		try {
			const filters = buildFilters();
			const statsFilters = {
				...filters,
				time_group_by: groupBy === 'group_by_user' ? 'user' : 'policy'
			};
			const [violationsResp, statsResp] = await Promise.all([
				AdminService.listMessagePolicyViolations(filters),
				AdminService.getMessagePolicyViolationStats(statsFilters)
			]);
			violations = violationsResp.items ?? [];
			total = violationsResp.total;
			stats = statsResp;
		} finally {
			loading = false;
		}
	}

	async function viewDetail(v: MessagePolicyViolation) {
		selectedViolation = v;
		detailedViolation = null;
		rightSidebar?.showPopover();
		try {
			detailedViolation = await AdminService.getMessagePolicyViolation(v.id);
		} catch {
			detailedViolation = v;
		}
	}

	function closeSidebar() {
		rightSidebar?.hidePopover();
		selectedViolation = null;
		detailedViolation = null;
	}

	function applyFilter() {
		pageOffset = 0;
		fetchData();
	}

	async function fetchFilterOptions() {
		const [usersResp, policiesResp] = await Promise.all([
			AdminService.listMessagePolicyViolationFilterOptions('user_id'),
			AdminService.listMessagePolicyViolationFilterOptions('policy_name')
		]);
		userFilterOptions = usersResp ?? [];
		const policyMap = new Map(stats?.byPolicy.map((p) => [p.policyName, p.policyID]) ?? []);
		policyFilterOptions = (policiesResp ?? []).map((name: string) => ({
			id: policyMap.get(name) ?? name,
			name
		}));
	}

	onMount(() => {
		fetchData().then(() => fetchFilterOptions());
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

	let directionLabel = (d: string) => PolicyDirectionLabels[d as PolicyDirection] ?? d;

	let totalPages = $derived(Math.ceil(total / pageLimit));
	let currentPage = $derived(Math.floor(pageOffset / pageLimit) + 1);

	const directionSelectOptions = [
		{ id: 'all_directions', label: 'All Directions' },
		{ id: 'user-message', label: 'User Messages' },
		{ id: 'tool-calls', label: 'Tool Calls' }
	];
	let userSelectOptions = $derived([
		{ id: 'all_users', label: 'All Users' },
		...userFilterOptions.map((uid) => ({ id: uid, label: displayName(uid) }))
	]);
	let policySelectOptions = $derived([
		{ id: 'all_policies', label: 'All Policies' },
		...policyFilterOptions.map((p) => ({ id: p.id, label: p.name }))
	]);

	function handleFilterSelect(
		kind: 'direction' | 'user' | 'policy',
		option: { id: string | number }
	) {
		const id = String(option.id);
		const allKey =
			kind === 'direction' ? 'all_directions' : kind === 'user' ? 'all_users' : 'all_policies';

		if (id === allKey) {
			if (kind === 'direction') filterDirection = allKey;
			else if (kind === 'user') filterUserID = allKey;
			else filterPolicyID = allKey;
		} else {
			// Remove the "all" sentinel when a specific value is selected
			if (kind === 'direction') {
				filterDirection =
					filterDirection
						.split(',')
						.filter((v) => v !== allKey)
						.join(',') || id;
			} else if (kind === 'user') {
				filterUserID =
					filterUserID
						.split(',')
						.filter((v) => v !== allKey)
						.join(',') || id;
			} else {
				filterPolicyID =
					filterPolicyID
						.split(',')
						.filter((v) => v !== allKey)
						.join(',') || id;
			}
		}
		applyFilter();
	}

	function handleFilterClear(
		kind: 'direction' | 'user' | 'policy',
		option?: { id: string | number }
	) {
		if (!option) return;
		const allKey =
			kind === 'direction' ? 'all_directions' : kind === 'user' ? 'all_users' : 'all_policies';

		const removeId = String(option.id);
		if (kind === 'direction') {
			const parts = filterDirection
				.split(',')
				.map((v) => v.trim())
				.filter((v) => v && v !== allKey && v !== removeId);
			filterDirection = parts.length === 0 ? allKey : parts.join(',');
		} else if (kind === 'user') {
			const parts = filterUserID
				.split(',')
				.map((v) => v.trim())
				.filter((v) => v && v !== allKey && v !== removeId);
			filterUserID = parts.length === 0 ? allKey : parts.join(',');
		} else {
			const parts = filterPolicyID
				.split(',')
				.map((v) => v.trim())
				.filter((v) => v && v !== allKey && v !== removeId);
			filterPolicyID = parts.length === 0 ? allKey : parts.join(',');
		}
		applyFilter();
	}

	function handleFilterClearAll(kind: 'direction' | 'user' | 'policy') {
		if (kind === 'direction') filterDirection = 'all_directions';
		else if (kind === 'user') filterUserID = 'all_users';
		else filterPolicyID = 'all_policies';
		applyFilter();
	}

	const groupByOptions = [
		{ id: 'group_by_policy', label: 'Group by Policy' },
		{ id: 'group_by_user', label: 'Group by User' }
	];

	let chartData = $derived(
		(stats?.byTime ?? [])
			.filter((b) => b.count > 0)
			.map((b) => ({
				createdAt: b.time,
				category: groupBy === 'group_by_user' ? displayName(b.category) : b.category,
				count: b.count,
				_secondary: 0 as const
			}))
	);

	function handleGroupByChange(option: { id: string | number }) {
		groupBy = String(option.id);
		fetchData();
	}
</script>

<svelte:head>
	<title>Obot | Message Policy Violations</title>
</svelte:head>

<Layout
	title="Message Policy Violations"
	classes={{
		container: 'md:px-0 px-0 pt-0',
		childrenContainer: 'max-w-none',
		noSidebarTitle: 'pl-4 md:pl-8 mx-auto md:max-w-(--breakpoint-xl) pt-4'
	}}
>
	<div class="flex-1" in:fade={{ duration }} out:fade={{ duration }}>
		{#if loading}
			<div
				class="absolute inset-0 z-20 flex items-center justify-center"
				in:fade={{ duration: 100 }}
				out:fade|global={{ duration: 300, delay: 500 }}
			>
				<div
					class="bg-base-400/50 border-base-400 text-primary dark:text-primary flex flex-col items-center gap-4 rounded-2xl border px-16 py-8 shadow-md backdrop-blur-[1px]"
				>
					<Loading class="size-32 stroke-1" />
					<div class="text-2xl font-semibold">Loading violations...</div>
				</div>
			</div>
		{/if}

		<div class="mb-4 flex flex-col gap-4">
			<!-- Overall Stats -->
			<div class="bg-base-300 dark:bg-base-200 w-full">
				<div class="m-auto w-full px-4 py-4 md:max-w-(--breakpoint-xl) md:px-8">
					<h4 class="font-semibold">Overall Stats</h4>
					<div class="flex flex-col flex-wrap items-stretch gap-4 md:flex-row">
						<div class="flex min-w-0 flex-1 flex-col gap-1 py-2">
							<div class="text-base-content text-xs font-light">Total Violations</div>
							<div class="text-primary flex items-center gap-1 text-xl font-semibold">
								{#if loading}
									<Loading class="size-4 animate-spin" />
								{:else}
									{total.toLocaleString()}
								{/if}
							</div>
						</div>
						<div class="divider-horizontal hidden md:block"></div>
						<div class="flex min-w-0 flex-1 flex-col gap-1 py-2">
							<div class="text-base-content text-xs font-light">User Message Violations</div>
							<div class="text-primary flex items-center gap-1 text-xl font-semibold">
								{#if loading}
									<Loading class="size-4 animate-spin" />
								{:else}
									{(stats?.byDirection.userMessage ?? 0).toLocaleString()}
								{/if}
							</div>
						</div>
						<div class="divider-horizontal hidden md:block"></div>
						<div class="flex min-w-0 flex-1 flex-col gap-1 py-2">
							<div class="text-base-content text-xs font-light">Tool Call Violations</div>
							<div class="text-primary flex items-center gap-1 text-xl font-semibold">
								{#if loading}
									<Loading class="size-4 animate-spin" />
								{:else}
									{(stats?.byDirection.toolCalls ?? 0).toLocaleString()}
								{/if}
							</div>
						</div>
					</div>
				</div>
			</div>

			<!-- Filter bar -->
			<div
				class="m-auto flex w-full max-w-full flex-col gap-4 px-4 md:max-w-(--breakpoint-xl) md:px-8"
			>
				<div class="flex w-full flex-wrap items-center justify-end gap-4">
					<p class="text-muted-content w-full text-sm md:w-fit">Filter by:</p>
					<Select
						class="dark:border-base-400 border border-transparent"
						classes={{ root: 'w-full md:flex-1 dark:border-base-400' }}
						options={directionSelectOptions}
						bind:selected={filterDirection}
						multiple
						searchInDropdown
						id="filter-direction"
						onSelect={(option) => handleFilterSelect('direction', option)}
						onClear={(option) => handleFilterClear('direction', option)}
						onClearAll={filterDirection !== 'all_directions'
							? () => handleFilterClearAll('direction')
							: undefined}
						placeholder="Filter by direction..."
						buttonReadOnly
						buttonTitle="Directions"
						displayCount={!!filterDirection && filterDirection !== 'all_directions'}
					/>
					<Select
						class="dark:border-base-400 border border-transparent"
						classes={{ root: 'w-full md:flex-1 dark:border-base-400' }}
						options={userSelectOptions}
						bind:selected={filterUserID}
						multiple
						searchInDropdown
						id="filter-user"
						onSelect={(option) => handleFilterSelect('user', option)}
						onClear={(option) => handleFilterClear('user', option)}
						onClearAll={filterUserID !== 'all_users'
							? () => handleFilterClearAll('user')
							: undefined}
						placeholder="Filter by user..."
						buttonReadOnly
						buttonTitle="Users"
						displayCount={!!filterUserID && filterUserID !== 'all_users'}
					/>
					<Select
						class="dark:border-base-400 border border-transparent"
						classes={{ root: 'w-full md:flex-1 dark:border-base-400' }}
						options={policySelectOptions}
						bind:selected={filterPolicyID}
						multiple
						searchInDropdown
						id="filter-policy"
						onSelect={(option) => handleFilterSelect('policy', option)}
						onClear={(option) => handleFilterClear('policy', option)}
						onClearAll={filterPolicyID !== 'all_policies'
							? () => handleFilterClearAll('policy')
							: undefined}
						placeholder="Filter by policy..."
						buttonReadOnly
						buttonTitle="Policies"
						displayCount={!!filterPolicyID && filterPolicyID !== 'all_policies'}
					/>
					<div class="bg-base-400 hidden h-8 w-0.5 md:block"></div>
					<AuditLogCalendar start={startTime} end={endTime} onChange={handleTimeRangeChange} />
				</div>

				{#if filterDirection !== 'all_directions' || filterUserID !== 'all_users' || filterPolicyID !== 'all_policies'}
					<div class="flex flex-wrap items-center gap-2" in:slide={{ axis: 'y', duration: 100 }}>
						{#if filterDirection !== 'all_directions'}
							{#each filterDirection.split(',') as direction (direction)}
								<div class="filter-primary">
									<span class="font-semibold">Direction:</span>{directionLabel(direction)}
									<button onclick={() => handleFilterClear('direction', { id: direction })}>
										<X class="size-3" />
									</button>
								</div>
							{/each}
						{/if}
						{#if filterUserID !== 'all_users'}
							{#each filterUserID.split(',') as userID (userID)}
								<div class="filter-primary">
									<span class="font-semibold">User:</span>{displayName(userID)}
									<button onclick={() => handleFilterClear('user', { id: userID })}>
										<X class="size-3" />
									</button>
								</div>
							{/each}
						{/if}
						{#if filterPolicyID !== 'all_policies'}
							{#each filterPolicyID.split(',') as policyID (policyID)}
								<div class="filter-primary">
									<span class="font-semibold">Policy:</span>{policyFilterOptions.find(
										(p) => p.id === policyID
									)?.name}
									<button onclick={() => handleFilterClear('policy', { id: policyID })}>
										<X class="size-3" />
									</button>
								</div>
							{/each}
						{/if}
					</div>
				{/if}

				<!-- Chart -->
				<div class="paper w-full gap-0 pt-4">
					<div class="mb-1 flex flex-wrap items-center justify-between gap-2">
						<h4 class="flex items-center gap-2 font-semibold">
							Policy Violations
							{#if loading}
								<Loading class="size-4 animate-spin" />
							{/if}
						</h4>
						<Select
							class="bg-base-300 dark:bg-base-100 dark:border-base-400 w-[50dvw] border border-transparent shadow-inner md:w-64"
							options={groupByOptions}
							selected={groupBy}
							onSelect={handleGroupByChange}
						/>
					</div>
					<div class="w-full pt-2">
						{#key groupBy}
							<StackedTimeline
								start={startTime}
								end={endTime}
								data={chartData}
								categoryKey="category"
								dateKey="createdAt"
								primaryValueKey="count"
								secondaryValueKey="_secondary"
								class="h-96"
								legend={{
									showSecondaryLabel: false,
									primaryLabel: '',
									hideCategoryLabel: false
								}}
							>
								{#snippet tooltipContent(item)}
									<div class="flex flex-col gap-0 text-xs">
										<div class="text-sm font-light">{item.key}</div>
										<div class="text-muted-content">{item.date}</div>
										<div class="divider"></div>
									</div>
									<div class="flex flex-col gap-1">
										<div class="text-base-content text-xl font-bold">
											{(item.primaryTotal ?? 0).toLocaleString()}
										</div>
									</div>
								{/snippet}
							</StackedTimeline>
						{/key}
					</div>
				</div>

				<!-- Table -->
				{#if !loading && violations.length === 0}
					<div
						class="mt-12 flex w-md max-w-full flex-col items-center gap-4 self-center text-center"
					>
						<ShieldAlert class="text-muted-content size-24 opacity-50" />
						<h4 class="text-muted-content text-lg font-semibold">No policy violations</h4>
						<p class="text-muted-content text-sm font-light">
							Currently, there are no policy violations for the selected range or filters. Try
							modifying your search criteria or try again later.
						</p>
					</div>
				{:else if violations.length > 0}
					<div
						class="dark:bg-base-300 bg-base-100 flex w-full min-w-full flex-1 divide-y divide-gray-200 overflow-x-auto overflow-y-visible rounded-lg border border-transparent shadow-sm"
					>
						<table class="w-full flex-1 table-fixed border-collapse border-spacing-0">
							<thead>
								<tr>
									<th
										class="dark:bg-base-200 bg-base-300 text-muted-content sticky top-0 box-content w-[4ch] px-6 py-3 text-left text-xs font-medium tracking-wider uppercase"
									>
										#
									</th>
									<th
										class="dark:bg-base-200 bg-base-300 text-muted-content sticky top-0 box-content w-[34ch] px-6 py-3 text-left text-xs font-medium tracking-wider uppercase"
									>
										Timestamp
									</th>
									<th
										class="dark:bg-base-200 bg-base-300 text-muted-content sticky top-0 box-content w-[24ch] px-6 py-3 text-left text-xs font-medium tracking-wider uppercase"
									>
										User
									</th>
									<th
										class="dark:bg-base-200 bg-base-300 text-muted-content sticky top-0 box-content w-[24ch] px-6 py-3 text-left text-xs font-medium tracking-wider uppercase"
									>
										Policy
									</th>
									<th
										class="dark:bg-base-200 bg-base-300 text-muted-content sticky top-0 box-content w-[24ch] px-6 py-3 text-left text-xs font-medium tracking-wider uppercase"
									>
										Applies To
									</th>
								</tr>
							</thead>
							<tbody>
								{#each violations as v, i (v.id)}
									<tr
										class="hover:bg-base-400 dark:hover:bg-base-400 group h-14 cursor-pointer text-sm transition-colors duration-300"
										onclick={() => viewDetail(v)}
									>
										<td class="px-6 py-3">{pageOffset + i + 1}</td>
										<td class="whitespace-nowrap">
											<div class="truncate px-6 py-4">
												{formatLogTimestamp(v.createdAt, timePreference.timeFormat)}
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
							class="dark:bg-base-300 bg-base-100 flex items-center justify-between gap-2 rounded-lg border border-transparent px-4 py-3 text-xs text-gray-600 shadow-sm"
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
									class="hover:text-base-content/80 active:text-base-content flex items-center text-xs transition-colors duration-100 disabled:pointer-events-none disabled:opacity-50"
									disabled={pageOffset === 0}
									onclick={() => {
										pageOffset = Math.max(0, pageOffset - pageLimit);
										fetchData();
									}}
								>
									Previous
								</button>
								<button
									class="hover:text-base-content/80 active:text-base-content flex items-center text-xs transition-colors duration-100 disabled:pointer-events-none disabled:opacity-50"
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
	</div>

	<!-- Detail drawer -->
	<div
		bind:this={rightSidebar}
		popover
		class="drawer-legacy max-w-[85vw] min-w-lg"
		style="width: 32rem"
	>
		{#if !responsive.isMobile && rightSidebar}
			<div
				role="none"
				class="absolute top-0 left-0 z-30 h-full w-3 cursor-col-resize"
				use:columnResize={{ column: rightSidebar, direction: 'right' }}
			></div>
		{/if}

		{#if selectedViolation}
			<div class="bg-base-200 text-base-content flex h-full w-[inherit] min-w-[inherit] flex-col">
				<div
					class="dark:bg-base-200 bg-base-100 relative flex w-full items-center justify-between p-4 pl-5 shadow-xs"
				>
					<div class="bg-primary absolute top-0 left-0 h-full w-1"></div>
					<h3 class="text-lg font-semibold">Violation Detail</h3>
					<IconButton onclick={closeSidebar}>
						<X class="size-5" />
					</IconButton>
				</div>

				<div class="default-scrollbar-thin relative flex-1 overflow-y-auto pb-4">
					<div class="bg-base-300 absolute top-0 left-0 h-full w-1"></div>

					{#if detailedViolation}
						<div class="flex flex-wrap gap-2 p-4 pl-5">
							<div
								class="dark:bg-base-400 bg-base-300 rounded-full px-3 py-1 text-[11px] font-light"
							>
								<span class="font-medium">Policy:</span>
								{detailedViolation.policyName}
							</div>
							<div
								class="dark:bg-base-400 bg-base-300 rounded-full px-3 py-1 text-[11px] font-light"
							>
								<span class="font-medium">Applies To:</span>
								{directionLabel(detailedViolation.direction)}
							</div>
							{#if detailedViolation.projectID}
								<div
									class="dark:bg-base-400 bg-base-300 rounded-full px-3 py-1 text-[11px] font-light"
								>
									<span class="font-medium">Project:</span>
									{detailedViolation.projectID}
								</div>
							{/if}
							{#if detailedViolation.threadID}
								<div
									class="dark:bg-base-400 bg-base-300 rounded-full px-3 py-1 text-[11px] font-light"
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
									{formatLogTimestamp(detailedViolation.createdAt, timePreference.timeFormat)}
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

							<p class="mt-6 mb-2 text-base font-semibold">Blocked Content</p>
							{#if detailedViolation.blockedContent}
								<div
									class="dark:bg-base-300 bg-base-100 relative overflow-hidden rounded-md p-4 pl-5"
								>
									<div class="bg-primary/50 absolute top-0 left-0 h-full w-1"></div>
									<pre
										class="default-scrollbar-thin max-h-96 overflow-y-auto text-sm wrap-break-word whitespace-pre-wrap">{JSON.stringify(
											detailedViolation.blockedContent,
											null,
											2
										)}</pre>
								</div>
							{:else}
								<p class="text-sm font-light text-muted-content">
									Blocked content is only visible to auditors.
								</p>
							{/if}
						</div>
					{:else}
						<div
							class="text-muted-content flex items-center justify-center gap-2 py-12 text-sm font-light"
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

<style lang="postcss">
	.divider {
		height: 1px;
		width: 100%;
		background-color: var(--color-base-400);
		margin-top: 0.5rem;
		margin-bottom: 0.5rem;
	}
</style>
