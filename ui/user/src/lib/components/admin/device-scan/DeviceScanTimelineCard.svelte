<script lang="ts">
	import GraphTitle from '$lib/components/graph/GraphTitle.svelte';
	import StackedTimeline from '$lib/components/graph/StackedTimeline.svelte';
	import { m } from '$lib/i18n';

	type Props = {
		rangeStart: Date | string;
		rangeEnd: Date | string;
		timelineRows: { scanned_at: string; category: string }[];
		totalSubmissions: number;
		emptyMsg?: string;
	};

	let {
		rangeStart,
		rangeEnd,
		timelineRows,
		totalSubmissions,
		emptyMsg = m.inventory_enforcement_devices_scan_no_submissions()
	}: Props = $props();
</script>

<div class="paper flex h-full flex-col p-0 gap-0">
	<GraphTitle title={m.inventory_enforcement_devices_scan_timeline()} element="h4">
		{#snippet rightContent()}
			{#if totalSubmissions > 0}
				<span class="text-muted-content text-xs">
					{totalSubmissions === 1
						? m.inventory_enforcement_devices_scan_submissions_one({ count: totalSubmissions })
						: m.inventory_enforcement_devices_scan_submissions_other({ count: totalSubmissions })}
				</span>
			{/if}
		{/snippet}
	</GraphTitle>

	<div class="flex flex-col p-4 h-full">
		{#if timelineRows.length === 0}
			<p
				class="text-muted-content flex min-h-56 flex-1 items-center justify-center text-sm font-light"
			>
				{emptyMsg}
			</p>
		{:else}
			<div class="text-muted-content flex min-h-56 flex-1 items-center justify-center">
				<StackedTimeline
					start={new Date(rangeStart)}
					end={new Date(rangeEnd)}
					data={timelineRows}
					categoryKey="category"
					dateKey="scanned_at"
				/>
			</div>
		{/if}
	</div>
</div>
