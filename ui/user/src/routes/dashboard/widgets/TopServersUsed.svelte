<script lang="ts">
	import Skeleton from '$lib/components/Skeleton.svelte';
	import GraphTitle from '$lib/components/graph/GraphTitle.svelte';
	import HorizontalBarList from '$lib/components/graph/HorizontalBarList.svelte';
	import { m } from '$lib/i18n';
	import type { TopServerUsageRow } from '$lib/services/dashboard/types';
	import { fade } from 'svelte/transition';

	type Props = {
		loading: boolean;
		rows: TopServerUsageRow[];
		maxServersToShow: number;
	};

	let { loading, rows, maxServersToShow }: Props = $props();
</script>

{#if loading}
	<Skeleton type="card" class="h-full min-h-72 w-full" />
{:else}
	<div in:fade={{ duration: 150 }} class="paper h-full min-h-72 w-full gap-0 p-0">
		<GraphTitle title={m.dashboard_top_servers_used()} element="h4" />
		{#if rows.length > 0}
			<div class="p-4">
				<HorizontalBarList
					data={rows.slice(0, maxServersToShow)}
					labelKey="serverName"
					valueKey="count"
					formatValue={(value) => Math.round(value).toString()}
				>
					{#snippet tooltipContent(item)}
						<div class="flex flex-col gap-0 text-xs">
							<div class="text-muted-content text-xs">{item.label}</div>
						</div>
						<div class="text-base-content font-semibold">
							{m.dashboard_calls_count({ count: item.value })}
						</div>
					{/snippet}
				</HorizontalBarList>
			</div>
		{:else}
			<p
				class="text-muted-content flex h-full grow items-center justify-center pt-2 text-center text-xs font-light"
			>
				{m.dashboard_no_top_servers_data()}
			</p>
		{/if}
	</div>
{/if}
