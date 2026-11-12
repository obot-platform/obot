<script lang="ts">
	import GraphTitle from '$lib/components/graph/GraphTitle.svelte';
	import { m } from '$lib/i18n';
	import type { TokenUsageStats } from '$lib/services';
	import { providerLabel } from '$lib/services/llm-gateway/providers';
	import { formatTokenUsageUSD as formatUSD } from './tokenUsageTimeline';
	import { twMerge } from 'tailwind-merge';

	type Props = {
		loading: boolean;
		stats?: TokenUsageStats;
		class?: string;
	};

	let { loading, stats, class: className }: Props = $props();

	const providerSpend = $derived(
		(stats?.providers ?? [])
			.filter((provider) => provider.totalSpend > 0)
			.map((provider) => ({
				provider: provider.provider
					? providerLabel(provider.provider)
					: m.audit_usage_usage_unknown_provider(),
				spend: provider.totalSpend
			}))
	);
	const maxProviderSpend = $derived(
		Math.max(0, ...providerSpend.map((provider) => provider.spend))
	);
</script>

<div class={twMerge('paper flex h-full min-h-0 flex-col gap-0 overflow-hidden p-0', className)}>
	<GraphTitle title={m.audit_usage_usage_spend_by_provider()} element="h5" />
	<div class="w-full shrink-0 p-4">
		{#if loading && !stats}
			<div class="skeleton h-48 w-full rounded-md"></div>
		{:else if providerSpend.length}
			<ul class="flex flex-col gap-3">
				{#each providerSpend as provider (provider.provider)}
					<li class="flex flex-col gap-1">
						<div class="flex justify-between text-xs">
							<span>{provider.provider}</span>
							<span class="font-medium">{formatUSD(provider.spend)}</span>
						</div>
						<div class="bg-base-200 dark:bg-base-300 h-2 w-full rounded-full">
							<div
								class="bg-primary h-2 rounded-full"
								style={`width: ${maxProviderSpend ? Math.max(2, (provider.spend / maxProviderSpend) * 100) : 0}%`}
							></div>
						</div>
					</li>
				{/each}
			</ul>
		{:else}
			<div class="text-muted-content flex h-48 items-center justify-center text-sm font-light">
				{m.audit_usage_usage_no_requests()}
			</div>
		{/if}
	</div>
</div>
