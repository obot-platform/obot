<script lang="ts">
	import Skeleton from '$lib/components/Skeleton.svelte';
	import GraphTitle from '$lib/components/graph/GraphTitle.svelte';
	import Table from '$lib/components/table/Table.svelte';
	import { m } from '$lib/i18n';
	import type { TokenUsageStats, TokenUsageStatsModel } from '$lib/services';
	import { providerLabel } from '$lib/services/llm-gateway/providers';
	import { formatCount, formatLatency, successRate } from './requestStats';
	import { formatTokenUsageUSD as formatUSD } from './tokenUsageTimeline';
	import { twMerge } from 'tailwind-merge';

	type Props = {
		loading: boolean;
		stats?: TokenUsageStats;
		modelLabel?: (model: string) => string;
		title?: string;
		class?: string;
	};

	let { loading, stats, modelLabel = (model) => model, title, class: className }: Props = $props();

	let topModelsBy = $state<'requests' | 'spend'>('requests');

	const topModels = $derived(
		((topModelsBy === 'spend' ? stats?.topModelsBySpend : stats?.topModelsByRequests) ?? []).map(
			(model: TokenUsageStatsModel) => ({
				id: model.model,
				model: modelLabel(model.model),
				provider: model.provider ? providerLabel(model.provider) : '',
				requests: formatCount(model.requests),
				success: `${successRate(model).toFixed(1)}%`,
				tokens: formatCount(model.totalTokens),
				spend: formatUSD(model.totalSpend),
				latency: model.avgDurationMs ? formatLatency(model.avgDurationMs) : ''
			})
		)
	);
</script>

<div class={twMerge('paper flex min-h-0 flex-col gap-0 p-0', className)}>
	{#if title}
		<GraphTitle {title} element="h4" />
	{/if}
	<div class={twMerge('bg-base-300 flex shrink-0 items-center', !title && 'rounded-t-md')}>
		{#each [{ id: 'requests', label: m.audit_usage_usage_top_models_by_requests() }, { id: 'spend', label: m.audit_usage_usage_top_models_by_spend() }] as const as tab (tab.id)}
			<button
				class={twMerge(
					'relative px-4 py-2 text-xs font-medium uppercase transition-colors first:rounded-tl-md',
					topModelsBy === tab.id
						? 'bg-base-100 dark:bg-base-200'
						: 'text-muted-content hover:text-base-content hover:bg-base-100/50 hover:dark:bg-base-200/50'
				)}
				aria-pressed={topModelsBy === tab.id}
				onclick={() => (topModelsBy = tab.id)}
			>
				{tab.label}
				{#if topModelsBy === tab.id}
					<span
						class="bg-primary absolute bottom-1 left-1/2 h-0.5 w-[calc(100%-2rem)] -translate-x-1/2"
					></span>
				{/if}
			</button>
		{/each}
	</div>
	<div class="p-1">
		{#if loading && !stats}
			<Skeleton type="table" count={3} />
		{:else}
			<Table
				data={topModels}
				fields={['model', 'provider', 'requests', 'success', 'tokens', 'spend', 'latency']}
				headers={[
					{ property: 'model', title: m.audit_usage_audit_logs_model() },
					{ property: 'provider', title: m.core_col_provider() },
					{ property: 'requests', title: m.audit_usage_usage_col_requests() },
					{ property: 'success', title: m.audit_usage_usage_col_success() },
					{ property: 'tokens', title: m.audit_usage_usage_col_tokens() },
					{ property: 'spend', title: m.core_spend() },
					{ property: 'latency', title: m.audit_usage_usage_col_latency() }
				]}
				classes={{ root: 'rounded-none shadow-none', thead: 'dark:bg-base-100' }}
				noDataMessage={m.audit_usage_usage_no_requests()}
			/>
		{/if}
	</div>
</div>
