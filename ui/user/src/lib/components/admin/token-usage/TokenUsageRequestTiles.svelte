<script lang="ts">
	import TweenedMetric from '$lib/components/TweenedMetric.svelte';
	import { m } from '$lib/i18n';
	import type { TokenUsageStats } from '$lib/services';
	import { formatCount, formatLatency, successRate } from './requestStats';
	import { twMerge } from 'tailwind-merge';

	type Props = {
		loading: boolean;
		stats?: TokenUsageStats;
		framed?: boolean;
		class?: string;
	};

	let { loading, stats, framed = true, class: className }: Props = $props();

	const totals = $derived(stats?.totals);
	const statTiles = $derived([
		{
			id: 'requests',
			label: m.audit_usage_usage_requests(),
			value: totals?.requests ?? 0,
			format: formatCount
		},
		{
			id: 'success-rate',
			label: m.audit_usage_usage_success_rate(),
			value: totals ? successRate(totals) : 0,
			format: (n: number) => `${n.toFixed(1)}%`
		},
		{
			id: 'latency',
			label: m.audit_usage_usage_avg_latency(),
			value: totals?.avgDurationMs ?? 0,
			format: formatLatency
		},
		{
			id: 'fallbacks',
			label: m.audit_usage_usage_fallbacks(),
			value: totals?.fallbacks ?? 0,
			format: formatCount
		}
	]);
</script>

<div class={twMerge(framed ? '@container paper min-w-0 gap-0 p-0' : undefined, className)}>
	<div class="grid w-full grid-cols-2 gap-0 @md:grid-cols-4">
		{#each statTiles as tile (tile.id)}
			<div
				class="border-base-300 col-span-1 my-2 flex min-w-0 px-2 not-last:border-r @max-md:nth-[2n]:border-r-0"
			>
				<div class="w-full min-w-0 p-2 leading-none">
					<div class="text-muted-content mb-1 text-[11px] tracking-wide @md:text-xs">
						{tile.label}
					</div>
					{#if loading && !stats}
						<div class="skeleton h-6 w-16 rounded-md"></div>
					{:else}
						<div class="text-lg font-semibold tracking-tight tabular-nums @md:text-xl">
							<TweenedMetric target={tile.value} format={tile.format} />
						</div>
					{/if}
				</div>
			</div>
		{/each}
	</div>
</div>
