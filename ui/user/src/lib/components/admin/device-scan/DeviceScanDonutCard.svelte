<script lang="ts">
	import { resolve } from '$app/paths';
	import {
		DEVICE_SCAN_TOP_N,
		deviceScanBucketsToDonut,
		type DeviceScanTopBucket
	} from '$lib/components/admin/device-scan/deviceScanTopBuckets';
	import DonutGraph from '$lib/components/graph/DonutGraph.svelte';
	import { openUrl } from '$lib/utils';
	import { ChevronRight } from 'lucide-svelte';
	import { twMerge } from 'tailwind-merge';

	type Props = {
		title: string;
		buckets: DeviceScanTopBucket[];
		totalGroups: number;
		emptyMsg: string;
		topN?: number;
		legendOnBottom?: boolean;
		class?: string;
		classes?: {
			graph?: string;
			graphContainer?: string;
		};
	};

	let {
		title,
		buckets,
		totalGroups,
		emptyMsg,
		classes,
		topN = DEVICE_SCAN_TOP_N,
		class: klass,
		legendOnBottom = false
	}: Props = $props();
</script>

<div class={twMerge('paper flex h-full flex-col gap-2', klass)}>
	<div class="flex items-baseline justify-between gap-2">
		<h4 class="font-semibold">{title}</h4>
		{#if totalGroups > topN}
			<span class="text-muted-content text-xs">
				top {topN} of {totalGroups}
			</span>
		{:else if totalGroups > 0}
			<span class="text-muted-content text-xs">
				{totalGroups}
				{totalGroups === 1 ? 'entry' : 'entries'}
			</span>
		{/if}
	</div>

	{#if buckets.length === 0}
		<p class="text-muted-content py-8 text-center text-sm font-light">{emptyMsg}</p>
	{:else}
		<div class={twMerge('flex flex-col items-center gap-4', !legendOnBottom && 'sm:flex-row')}>
			<div class={twMerge('h-56 w-56 shrink-0', classes?.graphContainer)}>
				<DonutGraph
					class={twMerge('h-56 w-56', classes?.graph)}
					data={deviceScanBucketsToDonut(buckets)}
					hideLegend
				/>
			</div>
			<ul class="flex w-full flex-col gap-1 text-sm">
				{#each buckets as bucket (bucket.key)}
					{#if bucket.drilldown === 'mcp' && !bucket.isOther}
						<li>
							<a
								href={resolve(`/admin/device-mcp-servers/${encodeURIComponent(bucket.key)}`)}
								onclick={(e) => {
									e.preventDefault();
									openUrl(
										resolve(`/admin/device-mcp-servers/${encodeURIComponent(bucket.key)}`),
										e.ctrlKey || e.metaKey
									);
								}}
								class="hover:bg-base-200 dark:hover:bg-base-300 group -mx-1 flex items-center gap-2 rounded-md px-1 py-0.5 transition-colors"
							>
								<span class="size-3 shrink-0 rounded-full" style="background-color: {bucket.color}"
								></span>
								<span class="flex-1 truncate">{bucket.label}</span>
								<span class="text-muted-content tabular-nums">{bucket.value}</span>
								<ChevronRight class="text-muted-content size-3 opacity-0 group-hover:opacity-100" />
							</a>
						</li>
					{:else if bucket.drilldown === 'skill' && !bucket.isOther}
						<li>
							<a
								href={resolve(`/admin/device-skills/${encodeURIComponent(bucket.key)}`)}
								onclick={(e) => {
									e.preventDefault();
									openUrl(
										resolve(`/admin/device-skills/${encodeURIComponent(bucket.key)}`),
										e.ctrlKey || e.metaKey
									);
								}}
								class="hover:bg-base-200 dark:hover:bg-base-300 group -mx-1 flex items-center gap-2 rounded-md px-1 py-0.5 transition-colors"
							>
								<span class="size-3 shrink-0 rounded-full" style="background-color: {bucket.color}"
								></span>
								<span class="flex-1 truncate">{bucket.label}</span>
								<span class="text-muted-content tabular-nums">{bucket.value}</span>
								<ChevronRight class="text-muted-content size-3 opacity-0 group-hover:opacity-100" />
							</a>
						</li>
					{:else}
						<li
							class="-mx-1 flex items-center gap-2 rounded-md px-1 py-0.5"
							class:text-muted-content={bucket.isOther}
						>
							<span class="size-3 shrink-0 rounded-full" style="background-color: {bucket.color}"
							></span>
							<span class="flex-1 truncate" class:italic={bucket.isOther}>
								{bucket.label}
								{#if bucket.isOther && bucket.otherCount !== undefined}
									<span class="text-muted-content ml-1 not-italic">({bucket.otherCount} more)</span>
								{/if}
							</span>
							<span class="text-muted-content tabular-nums">{bucket.value}</span>
						</li>
					{/if}
				{/each}
			</ul>
		</div>
	{/if}
</div>
