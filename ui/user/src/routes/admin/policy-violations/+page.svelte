<script lang="ts">
	import Layout from '$lib/components/Layout.svelte';
	import { AdminService } from '$lib/services';
	import type {
		PolicyViolation,
		PolicyViolationFilters,
		PolicyViolationStats
	} from '$lib/services/admin/types';
	import { PolicyDirectionLabels } from '$lib/services/admin/types';
	import type { PolicyDirection } from '$lib/services/admin/types';
	import { onMount } from 'svelte';
	import { subDays, set } from 'date-fns';
	import AuditLogCalendar from '$lib/components/admin/audit-logs/AuditLogCalendar.svelte';
	import Search from '$lib/components/Search.svelte';
	import HorizontalBarGraph from '$lib/components/graph/HorizontalBarGraph.svelte';
	import { fade } from 'svelte/transition';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants';

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

	let filterUserId = $state('');
	let filterPolicyId = $state('');
	let filterDirection = $state('');

	let pageOffset = $state(0);
	const pageLimit = 100;

	function buildFilters(): PolicyViolationFilters {
		const filters: PolicyViolationFilters = {
			start_time: startTime.toISOString(),
			end_time: endTime.toISOString(),
			limit: pageLimit,
			offset: pageOffset
		};
		if (query) filters.query = query;
		if (filterUserId) filters.user_id = filterUserId;
		if (filterPolicyId) filters.policy_id = filterPolicyId;
		if (filterDirection) filters.direction = filterDirection;
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
		selectedViolation = v;
		try {
			detailedViolation = await AdminService.getPolicyViolation(v.id);
		} catch {
			detailedViolation = v;
		}
	}

	function closeDetail() {
		selectedViolation = null;
		detailedViolation = null;
	}

	onMount(() => {
		fetchData();
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
</script>

<svelte:head>
	<title>Obot | Policy Violations</title>
</svelte:head>

<Layout title="Policy Violations">
	<div class="flex flex-col gap-6 pb-8" in:fade={{ duration }}>
		<!-- Filters -->
		<div class="flex flex-wrap items-center gap-3">
			<AuditLogCalendar start={startTime} end={endTime} onChange={handleTimeRangeChange} />

			<Search value={query} placeholder="Search violations..." onChange={handleSearch} />

			<select
				class="input h-9 w-auto min-w-[140px] text-sm"
				bind:value={filterDirection}
				onchange={() => {
					pageOffset = 0;
					fetchData();
				}}
			>
				<option value="">All Directions</option>
				<option value="user-message">User Messages</option>
				<option value="tool-calls">Tool Calls</option>
			</select>
		</div>

		<!-- Stats -->
		{#if stats && !loading}
			<div class="grid grid-cols-1 gap-4 md:grid-cols-3">
				<!-- Total violations -->
				<div class="bg-surface1 flex flex-col gap-1 rounded-lg p-4">
					<span class="text-on-surface2 text-xs font-medium uppercase">Total Violations</span>
					<span class="text-on-surface1 text-2xl font-bold">{total}</span>
				</div>

				<!-- By direction -->
				<div class="bg-surface1 flex flex-col gap-1 rounded-lg p-4">
					<span class="text-on-surface2 text-xs font-medium uppercase">User Messages</span>
					<span class="text-on-surface1 text-2xl font-bold">{stats.byDirection.userMessage}</span>
				</div>
				<div class="bg-surface1 flex flex-col gap-1 rounded-lg p-4">
					<span class="text-on-surface2 text-xs font-medium uppercase">Tool Calls</span>
					<span class="text-on-surface1 text-2xl font-bold">{stats.byDirection.toolCalls}</span>
				</div>
			</div>

			<!-- Charts row -->
			{#if stats.byPolicy.length > 0 || stats.byUser.length > 0}
				<div class="grid grid-cols-1 gap-4 md:grid-cols-2">
					{#if stats.byPolicy.length > 0}
						<div class="bg-surface1 flex flex-col gap-2 rounded-lg p-4">
							<span class="text-on-surface2 text-xs font-medium uppercase">By Policy</span>
							<div class="h-[200px]">
								<HorizontalBarGraph data={stats.byPolicy} labelKey="policyName" valueKey="count" />
							</div>
						</div>
					{/if}
					{#if stats.byUser.length > 0}
						<div class="bg-surface1 flex flex-col gap-2 rounded-lg p-4">
							<span class="text-on-surface2 text-xs font-medium uppercase">Top Users</span>
							<div class="h-[200px]">
								<HorizontalBarGraph
									data={stats.byUser.slice(0, 10)}
									labelKey="userID"
									valueKey="count"
								/>
							</div>
						</div>
					{/if}
				</div>
			{/if}

			<!-- Timeline -->
			{#if stats.byTime.length > 0}
				<div class="bg-surface1 flex flex-col gap-2 rounded-lg p-4">
					<span class="text-on-surface2 text-xs font-medium uppercase">Violations Over Time</span>
					<div class="flex h-[120px] items-end gap-[2px]">
						{@const maxCount = Math.max(1, ...stats.byTime.map((b) => b.count))}
						{#each stats.byTime as bucket}
							<div
								class="flex-1 rounded-t-sm bg-blue-500 transition-all"
								style="height: {(bucket.count / maxCount) * 100}%"
								title="{new Date(bucket.time).toLocaleString()}: {bucket.count}"
							></div>
						{/each}
					</div>
					<div class="text-on-surface3 flex justify-between text-[10px]">
						<span>{startTime.toLocaleDateString()}</span>
						<span>{endTime.toLocaleDateString()}</span>
					</div>
				</div>
			{/if}
		{/if}

		<!-- Table -->
		<div class="bg-surface1 overflow-hidden rounded-lg">
			{#if loading}
				<div class="text-on-surface2 flex items-center justify-center p-12 text-sm">Loading...</div>
			{:else if violations.length === 0}
				<div class="text-on-surface2 flex items-center justify-center p-12 text-sm">
					No violations found for the selected time range.
				</div>
			{:else}
				<table class="w-full text-sm">
					<thead>
						<tr
							class="border-border text-on-surface2 border-b text-left text-xs font-medium uppercase"
						>
							<th class="p-3">Timestamp</th>
							<th class="p-3">User</th>
							<th class="p-3">Policy</th>
							<th class="p-3">Direction</th>
							<th class="p-3">Explanation</th>
						</tr>
					</thead>
					<tbody>
						{#each violations as v (v.id)}
							<tr
								class="border-border hover:bg-surface2 cursor-pointer border-b transition-colors"
								onclick={() => viewDetail(v)}
							>
								<td class="text-on-surface2 p-3 text-xs whitespace-nowrap">
									{new Date(v.createdAt).toLocaleString()}
								</td>
								<td class="p-3 font-mono text-xs">{v.userID}</td>
								<td class="p-3">{v.policyName}</td>
								<td class="p-3">{directionLabel(v.direction)}</td>
								<td class="text-on-surface2 max-w-[300px] truncate p-3 text-xs">
									{v.violationExplanation}
								</td>
							</tr>
						{/each}
					</tbody>
				</table>

				<!-- Pagination -->
				{#if totalPages > 1}
					<div class="border-border flex items-center justify-between border-t p-3">
						<span class="text-on-surface2 text-xs">
							Showing {pageOffset + 1}-{Math.min(pageOffset + pageLimit, total)} of {total}
						</span>
						<div class="flex gap-2">
							<button
								class="button-secondary text-xs"
								disabled={pageOffset === 0}
								onclick={() => {
									pageOffset = Math.max(0, pageOffset - pageLimit);
									fetchData();
								}}
							>
								Previous
							</button>
							<button
								class="button-secondary text-xs"
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

	<!-- Detail sidebar -->
	{#if selectedViolation}
		<!-- svelte-ignore a11y_no_static_element_interactions -->
		<!-- svelte-ignore a11y_click_events_have_key_events -->
		<div
			class="fixed inset-0 z-40 bg-black/30"
			onclick={closeDetail}
			in:fade={{ duration: 150 }}
			out:fade={{ duration: 150 }}
		></div>
		<div
			class="bg-background border-border fixed top-0 right-0 z-50 flex h-full w-full max-w-lg flex-col border-l shadow-xl"
			in:fade={{ duration: 150 }}
			out:fade={{ duration: 150 }}
		>
			<div class="border-border flex items-center justify-between border-b p-4">
				<h3 class="text-on-surface1 text-lg font-semibold">Violation Detail</h3>
				<button class="icon-button" onclick={closeDetail}>
					<svg
						class="size-5"
						viewBox="0 0 24 24"
						fill="none"
						stroke="currentColor"
						stroke-width="2"
					>
						<path d="M6 18L18 6M6 6l12 12" />
					</svg>
				</button>
			</div>

			<div class="flex-1 overflow-y-auto p-4">
				{#if detailedViolation}
					<div class="flex flex-col gap-4">
						<div>
							<span class="text-on-surface2 text-xs font-medium uppercase">Timestamp</span>
							<p class="text-on-surface1 text-sm">
								{new Date(detailedViolation.createdAt).toLocaleString()}
							</p>
						</div>
						<div>
							<span class="text-on-surface2 text-xs font-medium uppercase">User ID</span>
							<p class="text-on-surface1 font-mono text-sm">{detailedViolation.userID}</p>
						</div>
						<div>
							<span class="text-on-surface2 text-xs font-medium uppercase">Policy</span>
							<p class="text-on-surface1 text-sm">{detailedViolation.policyName}</p>
						</div>
						<div>
							<span class="text-on-surface2 text-xs font-medium uppercase">Policy Definition</span>
							<p class="text-on-surface2 text-sm">{detailedViolation.policyDefinition}</p>
						</div>
						<div>
							<span class="text-on-surface2 text-xs font-medium uppercase">Direction</span>
							<p class="text-on-surface1 text-sm">{directionLabel(detailedViolation.direction)}</p>
						</div>
						<div>
							<span class="text-on-surface2 text-xs font-medium uppercase">Explanation</span>
							<p class="text-on-surface1 text-sm">{detailedViolation.violationExplanation}</p>
						</div>
						{#if detailedViolation.projectID}
							<div>
								<span class="text-on-surface2 text-xs font-medium uppercase">Project ID</span>
								<p class="text-on-surface1 font-mono text-sm">{detailedViolation.projectID}</p>
							</div>
						{/if}
						{#if detailedViolation.threadID}
							<div>
								<span class="text-on-surface2 text-xs font-medium uppercase">Thread ID</span>
								<p class="text-on-surface1 font-mono text-sm">{detailedViolation.threadID}</p>
							</div>
						{/if}
						{#if detailedViolation.blockedContent}
							<div>
								<span class="text-on-surface2 text-xs font-medium uppercase">Blocked Content</span>
								<pre
									class="bg-surface2 text-on-surface1 mt-1 max-h-[300px] overflow-auto rounded p-3 text-xs">{JSON.stringify(
										detailedViolation.blockedContent,
										null,
										2
									)}</pre>
							</div>
						{/if}
					</div>
				{:else}
					<div class="text-on-surface2 flex items-center justify-center p-8 text-sm">
						Loading details...
					</div>
				{/if}
			</div>
		</div>
	{/if}
</Layout>
