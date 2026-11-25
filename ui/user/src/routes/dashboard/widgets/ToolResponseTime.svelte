<script lang="ts">
	import Skeleton from '$lib/components/Skeleton.svelte';
	import GraphTitle from '$lib/components/graph/GraphTitle.svelte';
	import { m } from '$lib/i18n';
	import type { AvgToolCallResponseTimeRow } from '$lib/services/dashboard/types';
	import { resolve } from '$lib/url';
	import { ChevronRight } from '@lucide/svelte';

	type Props = {
		loading: boolean;
		rows: AvgToolCallResponseTimeRow[];
		maxToolsToShow: number;
		isBootStrapUser: boolean;
	};

	let { loading, rows, maxToolsToShow, isBootStrapUser }: Props = $props();

	const graphContainerClasses = 'flex h-full flex-col gap-4 px-6 py-4';
</script>

<div class="paper flex h-full min-h-72 flex-col gap-0 p-0">
	<GraphTitle title={m.dashboard_tool_call_avg_response_time()} element="h4" />
	<div class={graphContainerClasses}>
		{#if loading}
			<Skeleton type="list" class="w-full" count={maxToolsToShow} />
		{:else if rows.length === 0}
			<p
				class="text-muted-content flex h-full grow items-center justify-center pt-2 text-center text-xs font-light"
			>
				{m.dashboard_no_recent_tool_calls()}
			</p>
		{:else}
			<div class="flex w-full flex-col gap-4">
				<ul class="flex flex-col gap-2">
					{#each rows.slice(0, maxToolsToShow) as row (row.toolName)}
						<li class="flex items-center gap-2">
							<div class="flex min-w-0 grow flex-col gap-1 pr-4">
								<p class="truncate text-sm font-medium">
									{row.toolName.split('.').slice(1).join('.')}
								</p>
								<p class="text-muted-content text-xs">
									{row.serverDisplayName}
								</p>
							</div>
							<div class="text-sm">
								{row.averageResponseTimeMs.toFixed(2)}ms
							</div>
						</li>
					{/each}
				</ul>
			</div>
		{/if}
		<div class="flex min-h-0 grow"></div>
		{#if rows.length > 0 && !isBootStrapUser}
			<a
				href={resolve('/usage')}
				class="bg-base-400/50 hover:bg-base-400 mt-2 flex w-fit translate-x-2 items-center gap-1 self-end rounded-md px-2 py-0.5 text-[11px] transition-colors duration-200"
			>
				{m.dashboard_see_more()}
				<ChevronRight class="size-3" />
			</a>
		{/if}
	</div>
</div>
