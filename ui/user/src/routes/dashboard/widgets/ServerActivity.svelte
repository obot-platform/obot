<script lang="ts">
	import Skeleton from '$lib/components/Skeleton.svelte';
	import TweenedMetric from '$lib/components/TweenedMetric.svelte';
	import DonutGraph, { type DonutDatum } from '$lib/components/graph/DonutGraph.svelte';
	import GraphTitle from '$lib/components/graph/GraphTitle.svelte';
	import { m } from '$lib/i18n';
	import { entryTypeDonutLegend } from '$lib/services/dashboard/constants';
	import {
		deploymentStatusGridColClass,
		deploymentStatusGridShowBorderRight
	} from '$lib/services/dashboard/utils';
	import { resolve } from '$lib/url';
	import { ChevronRight, Server, Siren } from '@lucide/svelte';
	import { fade } from 'svelte/transition';
	import { twMerge } from 'tailwind-merge';

	type Props = {
		loading: boolean;
		hasDeviceScans: boolean;
		doesSupportK8sUpdates: boolean;
		deploymentStatusBreakdown: { status: string; count: number }[];
		totalServers: number;
		graphData: DonutDatum[];
		isBootStrapUser: boolean;
	};

	let {
		loading,
		hasDeviceScans,
		doesSupportK8sUpdates,
		deploymentStatusBreakdown,
		totalServers,
		graphData,
		isBootStrapUser
	}: Props = $props();

	const deploymentStatusLabels: Record<string, () => string> = {
		Available: m.dashboard_status_available,
		Progressing: m.dashboard_status_progressing,
		Unavailable: m.dashboard_status_unavailable,
		'Needs Attention': m.dashboard_status_needs_attention,
		Shutdown: m.dashboard_status_shutdown,
		Unknown: m.core_unknown,
		Pending: m.core_status_pending
	};

	function deploymentStatusLabel(status: string) {
		return deploymentStatusLabels[status]?.() ?? status;
	}

	const minHeight = $derived(hasDeviceScans ? 'min-h-64' : 'min-h-96');
</script>

{#if loading}
	<Skeleton type="card" class={twMerge('h-full w-full', minHeight)} />
{:else}
	<div in:fade={{ duration: 150 }} class={twMerge('paper h-full gap-0 p-0', minHeight)}>
		<GraphTitle title={m.dashboard_server_activity()} element="h4" />
		<div class="flex flex-col gap-4 px-6 py-4">
			{#if deploymentStatusBreakdown.length > 0 || totalServers > 0}
				{#if doesSupportK8sUpdates && deploymentStatusBreakdown.length > 0}
					<div class="mb-2 grid grid-cols-12 gap-x-2 gap-y-5">
						{#each deploymentStatusBreakdown as row, i (row.status)}
							<div
								class={twMerge(
									'flex flex-col items-center justify-center px-1 text-center',
									deploymentStatusGridColClass(i, deploymentStatusBreakdown.length),
									deploymentStatusGridShowBorderRight(i, deploymentStatusBreakdown.length) &&
										'border-base-300 border-r'
								)}
							>
								<div class="flex items-center gap-1">
									<div class={twMerge('font-semibold', hasDeviceScans ? 'text-xl' : 'text-3xl')}>
										<TweenedMetric target={row.count} />
									</div>
									{#if row.status === 'Available'}
										<Server class="text-primary size-6" />
									{:else if row.status === 'Needs Attention'}
										<Siren class="text-warning size-6" />
									{:else}
										<Server class="text-muted-content/75 size-6" />
									{/if}
								</div>
								<div class="text-xs">{deploymentStatusLabel(row.status)}</div>
							</div>
						{/each}
					</div>
				{:else}
					<div class="mb-2 flex flex-col items-center justify-center">
						<div class="flex w-full items-center justify-center gap-2">
							<div class={twMerge('font-semibold', hasDeviceScans ? 'text-xl' : 'text-3xl')}>
								<TweenedMetric target={totalServers} />
							</div>
							<Server class="text-primary size-6" />
						</div>
						<div class="text-xs">{m.dashboard_total_currently_active()}</div>
					</div>
				{/if}
			{/if}

			<div
				class={twMerge(
					'flex grow flex-col items-center justify-center',
					hasDeviceScans ? 'h-64' : 'h-80'
				)}
			>
				{#if graphData.some((item) => item.value > 0)}
					<DonutGraph
						class={twMerge('h-80', hasDeviceScans ? 'h-64' : '')}
						donutRatio={0.65}
						data={graphData}
						legend={doesSupportK8sUpdates ? entryTypeDonutLegend : undefined}
					/>
				{:else}
					<p class="text-muted-content pt-2 text-center text-xs font-light">
						{m.dashboard_no_servers_deployed()}
					</p>
				{/if}
			</div>

			{#if !isBootStrapUser && totalServers > 0}
				<div class="flex justify-end">
					<a
						href={resolve('/mcp-servers?view=deployments')}
						class="bg-base-400/50 hover:bg-base-400 flex w-fit translate-x-2 items-center gap-1 self-end rounded-md px-2 py-0.5 text-[11px] transition-colors duration-200"
					>
						{m.dashboard_see_more()}
						<ChevronRight class="size-3" />
					</a>
				</div>
			{/if}
		</div>
	</div>
{/if}
