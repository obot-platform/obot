<script lang="ts">
	import GraphTitle from '$lib/components/graph/GraphTitle.svelte';
	import StackedTimeline from '$lib/components/graph/StackedTimeline.svelte';
	import { m } from '$lib/i18n';
	import type { TokenUsageStats } from '$lib/services';
	import { twMerge } from 'tailwind-merge';

	type Props = {
		loading: boolean;
		stats?: TokenUsageStats;
		startDate: Date;
		endDate: Date;
		framed?: boolean;
		class?: string;
	};

	let { loading, stats, startDate, endDate, framed = true, class: className }: Props = $props();

	const successLabel = m.audit_usage_usage_successful();
	const failedLabel = m.audit_usage_usage_failed();
	const requestsTimeline = $derived(
		(stats?.buckets ?? []).flatMap((bucket) => [
			{ category: successLabel, date: bucket.time, count: bucket.successes },
			{ category: failedLabel, date: bucket.time, count: bucket.requests - bucket.successes }
		])
	);
	const hasRequests = $derived((stats?.totals?.requests ?? 0) > 0);
</script>

{#if framed}
	<div
		class={twMerge(
			'paper flex min-h-0 flex-col gap-0 overflow-hidden p-0 xl:col-span-2',
			className
		)}
	>
		<GraphTitle title={m.audit_usage_usage_requests_over_time()} element="h5" />
		<div class="w-full shrink-0 p-4">
			{@render body()}
		</div>
	</div>
{:else}
	{@render body()}
{/if}

{#snippet body()}
	{#if loading && !stats}
		<div class="skeleton h-48 w-full rounded-md"></div>
	{:else if hasRequests}
		<StackedTimeline
			start={startDate}
			end={endDate}
			data={requestsTimeline}
			categoryKey="category"
			dateKey="date"
			primaryValueKey="count"
			class="h-48"
		/>
	{:else}
		<div class="text-muted-content flex h-48 items-center justify-center text-sm font-light">
			{m.audit_usage_usage_no_requests()}
		</div>
	{/if}
{/snippet}
