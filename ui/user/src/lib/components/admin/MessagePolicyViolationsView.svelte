<script lang="ts">
	import { columnResize } from '$lib/actions/resize';
	import Select from '$lib/components/Select.svelte';
	import AuditLogCalendar from '$lib/components/admin/audit-logs/AuditLogCalendar.svelte';
	import StackedTimeline from '$lib/components/graph/StackedTimeline.svelte';
	import IconButton from '$lib/components/primitives/IconButton.svelte';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants';
	import { m } from '$lib/i18n';
	import Loading from '$lib/icons/Loading.svelte';
	import {
		AdminService,
		UserService,
		type OrgUser,
		type MessagePolicyViolation,
		type MessagePolicyViolationFilters,
		type MessagePolicyViolationStats,
		type PolicyDirection,
		PolicyDirectionLabels
	} from '$lib/services';
	import { responsive, userDeviceSettings } from '$lib/stores';
	import { formatLogTimestamp } from '$lib/time';
	import { getUserDisplayName, withDisabledMarker } from '$lib/utils';
	import { ShieldAlert, X } from '@lucide/svelte';
	import { subDays, set } from 'date-fns';
	import { onMount } from 'svelte';
	import { SvelteMap } from 'svelte/reactivity';
	import { fade, slide } from 'svelte/transition';

	interface Props {
		policyDirection?: Extract<PolicyDirection, 'user-message' | 'tool-calls'>;
	}

	let { policyDirection }: Props = $props();

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
		if (policyDirection) filters.direction = policyDirection;
		else if (filterDirection && filterDirection !== 'all_directions')
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
		const scope = policyDirection ? { direction: policyDirection } : undefined;
		const [usersResp, policiesResp] = await Promise.all([
			AdminService.listMessagePolicyViolationFilterOptions('user_id', scope),
			AdminService.listMessagePolicyViolationFilterOptions('policy_name', scope)
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
		UserService.listUsersIncludeDeleted().then((userData) => {
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
	let visibleViolations = $derived(
		policyDirection
			? violations.filter(
					(violation) =>
						directionLabel(violation.direction) === PolicyDirectionLabels[policyDirection]
				)
			: violations
	);

	let totalPages = $derived(Math.ceil(total / pageLimit));
	let currentPage = $derived(Math.floor(pageOffset / pageLimit) + 1);

	const directionSelectOptions = [
		{ id: 'all_directions', label: m.ai_judge_all_directions() },
		{ id: 'user-message', label: m.ai_judge_user_messages() },
		{ id: 'tool-calls', label: m.ai_judge_tool_calls() }
	];
	let userSelectOptions = $derived([
		{ id: 'all_users', label: m.ai_judge_all_users() },
		...userFilterOptions.map((uid) => ({
			id: uid,
			label: withDisabledMarker(displayName(uid), users.get(uid))
		}))
	]);
	let policySelectOptions = $derived([
		{ id: 'all_policies', label: m.ai_judge_all_policies() },
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
		{ id: 'group_by_policy', label: m.ai_judge_group_by_policy() },
		{ id: 'group_by_user', label: m.ai_judge_group_by_user() }
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

<div class="flex-1 mt-1" in:fade={{ duration }}>
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
				<div class="text-2xl font-semibold">{m.ai_judge_loading_violations()}</div>
			</div>
		</div>
	{/if}

	<div class="m-auto flex w-full max-w-full flex-col gap-4 md:max-w-(--breakpoint-xl)">
		<div class="flex flex-row min-w-0 shrink-0 gap-1 py-2 paper md:w-fit">
			<div class="text-base-content text-sm font-light">{m.ai_judge_total_violations()}</div>
			<div class="flex items-center gap-1 text-sm font-semibold">
				{#if loading}
					<Loading class="size-4 animate-spin" />
				{:else if policyDirection === 'user-message'}
					{(stats?.byDirection.userMessage ?? 0).toLocaleString()}
				{:else if policyDirection === 'tool-calls'}
					{(stats?.byDirection.toolCalls ?? 0).toLocaleString()}
				{/if}
			</div>
		</div>
		<!-- Filter bar -->
		<div class="flex w-full flex-wrap items-center justify-end gap-4">
			<p class="text-muted-content w-full text-sm md:w-fit">{m.ai_judge_filter_by()}</p>
			{#if !policyDirection}
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
					placeholder={m.ai_judge_filter_by_direction()}
					buttonReadOnly
					buttonTitle={m.ai_judge_directions()}
					displayCount={!!filterDirection && filterDirection !== 'all_directions'}
				/>
			{/if}
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
				onClearAll={filterUserID !== 'all_users' ? () => handleFilterClearAll('user') : undefined}
				placeholder={m.ai_judge_filter_by_user()}
				buttonReadOnly
				buttonTitle={m.ai_judge_users()}
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
				placeholder={m.ai_judge_filter_by_policy()}
				buttonReadOnly
				buttonTitle={m.ai_judge_policies()}
				displayCount={!!filterPolicyID && filterPolicyID !== 'all_policies'}
			/>
			<div class="bg-base-400 hidden h-8 w-0.5 md:block"></div>
			<AuditLogCalendar start={startTime} end={endTime} onChange={handleTimeRangeChange} />
		</div>

		{#if (!policyDirection && filterDirection !== 'all_directions') || filterUserID !== 'all_users' || filterPolicyID !== 'all_policies'}
			<div class="flex flex-wrap items-center gap-2" in:slide={{ axis: 'y', duration: 100 }}>
				{#if !policyDirection && filterDirection !== 'all_directions'}
					{#each filterDirection.split(',') as direction (direction)}
						<div class="filter-primary">
							<span class="font-semibold">{m.ai_judge_direction_label()}</span>{directionLabel(
								direction
							)}
							<button onclick={() => handleFilterClear('direction', { id: direction })}>
								<X class="size-3" />
							</button>
						</div>
					{/each}
				{/if}
				{#if filterUserID !== 'all_users'}
					{#each filterUserID.split(',') as userID (userID)}
						<div class="filter-primary">
							<span class="font-semibold">{m.ai_judge_user_label()}</span>{withDisabledMarker(
								displayName(userID),
								users.get(userID)
							)}
							<button onclick={() => handleFilterClear('user', { id: userID })}>
								<X class="size-3" />
							</button>
						</div>
					{/each}
				{/if}
				{#if filterPolicyID !== 'all_policies'}
					{#each filterPolicyID.split(',') as policyID (policyID)}
						<div class="filter-primary">
							<span class="font-semibold">{m.ai_judge_policy_label()}</span
							>{policyFilterOptions.find((p) => p.id === policyID)?.name}
							<button onclick={() => handleFilterClear('policy', { id: policyID })}>
								<X class="size-3" />
							</button>
						</div>
					{/each}
				{/if}
			</div>
		{/if}

		<!-- Chart -->
		{#if !loading && visibleViolations.length > 0}
			<div class="paper w-full gap-0 pt-4">
				<div class="mb-1 flex flex-wrap items-center justify-between gap-2">
					<h4 class="flex items-center gap-2 font-semibold">
						{m.ai_judge_policy_violations()}
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
		{/if}

		<!-- Table -->
		{#if !loading && visibleViolations.length === 0}
			<div class="mt-12 flex w-md max-w-full flex-col items-center gap-4 self-center text-center">
				<ShieldAlert class="text-muted-content size-24 opacity-50" />
				<h4 class="text-muted-content text-lg font-semibold">
					{m.ai_judge_no_policy_violations()}
				</h4>
				<p class="text-muted-content text-sm font-light">
					{m.ai_judge_no_policy_violations_description()}
				</p>
			</div>
		{:else if visibleViolations.length > 0}
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
								{m.core_col_timestamp()}
							</th>
							<th
								class="dark:bg-base-200 bg-base-300 text-muted-content sticky top-0 box-content w-[24ch] px-6 py-3 text-left text-xs font-medium tracking-wider uppercase"
							>
								{m.core_col_user()}
							</th>
							<th
								class="dark:bg-base-200 bg-base-300 text-muted-content sticky top-0 box-content w-[24ch] px-6 py-3 text-left text-xs font-medium tracking-wider uppercase"
							>
								{m.ai_judge_col_policy()}
							</th>
							<th
								class="dark:bg-base-200 bg-base-300 text-muted-content sticky top-0 box-content w-[24ch] px-6 py-3 text-left text-xs font-medium tracking-wider uppercase"
							>
								{m.ai_judge_applies_to()}
							</th>
						</tr>
					</thead>
					<tbody>
						{#each visibleViolations as v, i (v.id)}
							<tr
								class="hover:bg-base-400 dark:hover:bg-base-400 group h-14 cursor-pointer text-sm transition-colors duration-300"
								onclick={() => viewDetail(v)}
							>
								<td class="px-6 py-3">{pageOffset + i + 1}</td>
								<td class="whitespace-nowrap">
									<div class="truncate px-6 py-4">
										{formatLogTimestamp(v.createdAt, userDeviceSettings.timeFormat)}
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
							{m.ai_judge_showing_range({
								start: pageOffset + 1,
								end: Math.min(pageOffset + pageLimit, total),
								total
							})}
						</div>
						<div class="flex items-center">
							<span>{currentPage}</span>/<span>{totalPages}</span>
							<span class="ml-1">{m.ai_judge_pages()}</span>
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
							{m.core_previous()}
						</button>
						<button
							class="hover:text-base-content/80 active:text-base-content flex items-center text-xs transition-colors duration-100 disabled:pointer-events-none disabled:opacity-50"
							disabled={currentPage >= totalPages}
							onclick={() => {
								pageOffset += pageLimit;
								fetchData();
							}}
						>
							{m.ai_judge_next()}
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
	class="drawer-legacy md:max-w-[85vw] md:min-w-lg min-w-full max-w-full"
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
				<h3 class="text-lg font-semibold">{m.ai_judge_violation_detail()}</h3>
				<IconButton onclick={closeSidebar}>
					<X class="size-5" />
				</IconButton>
			</div>

			<div class="default-scrollbar-thin relative flex-1 overflow-y-auto pb-4">
				<div class="bg-base-300 absolute top-0 left-0 h-full w-1"></div>

				{#if detailedViolation}
					<div class="flex flex-wrap gap-2 p-4 pl-5">
						<div class="dark:bg-base-400 bg-base-300 rounded-full px-3 py-1 text-[11px] font-light">
							<span class="font-medium">{m.ai_judge_policy_label()}</span>
							{detailedViolation.policyName}
						</div>
						<div class="dark:bg-base-400 bg-base-300 rounded-full px-3 py-1 text-[11px] font-light">
							<span class="font-medium">{m.ai_judge_applies_to_label()}</span>
							{directionLabel(detailedViolation.direction)}
						</div>
						{#if detailedViolation.projectID}
							<div
								class="dark:bg-base-400 bg-base-300 rounded-full px-3 py-1 text-[11px] font-light"
							>
								<span class="font-medium">{m.ai_judge_project_label()}</span>
								{detailedViolation.projectID}
							</div>
						{/if}
						{#if detailedViolation.threadID}
							<div
								class="dark:bg-base-400 bg-base-300 rounded-full px-3 py-1 text-[11px] font-light"
							>
								<span class="font-medium">{m.ai_judge_thread_label()}</span>
								{detailedViolation.threadID}
							</div>
						{/if}
					</div>

					<div class="p-4 pl-5">
						<div class="flex flex-col gap-1 text-sm font-light">
							<p>
								<span class="font-medium">{m.core_col_timestamp()}</span>:
								{formatLogTimestamp(detailedViolation.createdAt, userDeviceSettings.timeFormat)}
							</p>
							<p>
								<span class="font-medium">{m.core_col_user()}</span>:
								{displayName(detailedViolation.userID)}
							</p>
						</div>

						<p class="mt-6 mb-2 text-base font-semibold">{m.ai_judge_policy_definition()}</p>
						<p class="text-sm font-light">{detailedViolation.policyDefinition}</p>

						<p class="mt-6 mb-2 text-base font-semibold">{m.ai_judge_explanation()}</p>
						<p class="text-sm font-light">{detailedViolation.violationExplanation}</p>

						<p class="mt-6 mb-2 text-base font-semibold">{m.ai_judge_blocked_content()}</p>
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
								{m.ai_judge_blocked_content_auditors_only()}
							</p>
						{/if}
					</div>
				{:else}
					<div
						class="text-muted-content flex items-center justify-center gap-2 py-12 text-sm font-light"
					>
						<Loading class="size-5 animate-spin" />
						<span>{m.ai_judge_loading_details()}</span>
					</div>
				{/if}
			</div>
		</div>
	{/if}
</div>

<style lang="postcss">
	.divider {
		height: 1px;
		width: 100%;
		background-color: var(--color-base-400);
		margin-top: 0.5rem;
		margin-bottom: 0.5rem;
	}
</style>
