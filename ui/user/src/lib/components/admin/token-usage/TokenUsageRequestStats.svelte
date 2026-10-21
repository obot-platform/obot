<script lang="ts">
	import Skeleton from '$lib/components/Skeleton.svelte';
	import TweenedMetric from '$lib/components/TweenedMetric.svelte';
	import StackedTimeline from '$lib/components/graph/StackedTimeline.svelte';
	import Table from '$lib/components/table/Table.svelte';
	import { isAbortError } from '$lib/errors';
	import { m } from '$lib/i18n';
	import {
		AdminService,
		type TokenUsageStats,
		type TokenUsageStatsFilters,
		type TokenUsageStatsModel
	} from '$lib/services';
	import { providerLabel } from '$lib/services/llm-gateway/providers';
	import { errors } from '$lib/stores';
	import { formatTokenUsageUSD as formatUSD } from './tokenUsageTimeline';
	import { twMerge } from 'tailwind-merge';

	// TokenUsageRequestStats shows how LLM gateway requests went over a time range: request
	// counts, success rate and latency, requests over time, spend by provider and the top
	// models. It is computed from Obot's token usage records, which every request has.
	type Props = {
		startDate: Date;
		endDate: Date;
		filters?: TokenUsageStatsFilters;
		// modelLabel names a model ("provider/model") for display.
		modelLabel?: (model: string) => string;
	};

	let { startDate, endDate, filters = {}, modelLabel = (model) => model }: Props = $props();

	let stats = $state<TokenUsageStats>();
	let loading = $state(true);
	let topModelsBy = $state<'requests' | 'spend'>('requests');

	const filtersKey = $derived(
		JSON.stringify([
			startDate.toISOString(),
			endDate.toISOString(),
			filters.userIDs ?? [],
			filters.models ?? [],
			filters.apiKeyIDs ?? []
		])
	);

	$effect(() => {
		void filtersKey;
		const controller = new AbortController();
		loading = true;
		AdminService.getTokenUsageStats({ start: startDate, end: endDate }, filters, {
			signal: controller.signal
		})
			.then((response) => {
				if (!controller.signal.aborted) stats = response;
			})
			.catch((err) => {
				if (isAbortError(err) || controller.signal.aborted) return;
				errors.append(err);
			})
			.finally(() => {
				if (!controller.signal.aborted) loading = false;
			});
		return () => controller.abort();
	});

	function formatCount(value: number) {
		return Intl.NumberFormat().format(Math.round(value));
	}

	function formatLatency(ms: number) {
		return m.audit_usage_usage_latency_ms({ ms: formatCount(ms) });
	}

	function successRate(t: { requests: number; successes: number }) {
		return t.requests ? (t.successes / t.requests) * 100 : 0;
	}

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

	const successLabel = m.audit_usage_usage_successful();
	const failedLabel = m.audit_usage_usage_failed();
	const requestsTimeline = $derived(
		(stats?.buckets ?? []).flatMap((b) => [
			{ category: successLabel, date: b.time, count: b.successes },
			{ category: failedLabel, date: b.time, count: b.requests - b.successes }
		])
	);
	const hasRequests = $derived((totals?.requests ?? 0) > 0);

	const providerSpend = $derived(
		(stats?.providers ?? [])
			.filter((p) => p.totalSpend > 0)
			.map((p) => ({
				provider: p.provider ? providerLabel(p.provider) : m.audit_usage_usage_unknown_provider(),
				spend: p.totalSpend
			}))
	);
	const maxProviderSpend = $derived(Math.max(0, ...providerSpend.map((p) => p.spend)));

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

<div class="flex flex-col gap-4">
	<div class="@container paper min-w-0 gap-0 p-0">
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

	<div class="grid grid-cols-1 gap-4 xl:grid-cols-3">
		<div class="paper flex min-h-0 flex-col gap-0 overflow-hidden p-0 xl:col-span-2">
			{@render cardHeader(m.audit_usage_usage_requests_over_time())}
			<div class="w-full shrink-0 p-4">
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
					{@render empty()}
				{/if}
			</div>
		</div>
		<div class="paper flex h-full min-h-0 flex-col gap-0 overflow-hidden p-0">
			{@render cardHeader(m.audit_usage_usage_spend_by_provider())}
			<div class="w-full shrink-0 p-4">
				{#if loading && !stats}
					<div class="skeleton h-48 w-full rounded-md"></div>
				{:else if providerSpend.length}
					<ul class="flex flex-col gap-3">
						{#each providerSpend as p (p.provider)}
							<li class="flex flex-col gap-1">
								<div class="flex justify-between text-xs">
									<span>{p.provider}</span>
									<span class="font-medium">{formatUSD(p.spend)}</span>
								</div>
								<div class="bg-base-200 dark:bg-base-300 h-2 w-full rounded-full">
									<div
										class="bg-primary h-2 rounded-full"
										style={`width: ${maxProviderSpend ? Math.max(2, (p.spend / maxProviderSpend) * 100) : 0}%`}
									></div>
								</div>
							</li>
						{/each}
					</ul>
				{:else}
					{@render empty()}
				{/if}
			</div>
		</div>
	</div>

	<!-- No overflow-hidden here: it would make this card the table's scroll container, and the
	table's sticky header, offset for the page's own sticky bar, would cover its first rows. -->
	<div class="paper flex min-h-0 flex-col gap-0 p-0">
		<div class="bg-base-300 flex shrink-0 items-center rounded-t-md">
			{#each [{ id: 'requests', label: m.audit_usage_usage_top_models_by_requests() }, { id: 'spend', label: m.audit_usage_usage_top_models_by_spend() }] as const as tab (tab.id)}
				<button
					class={twMerge(
						'relative py-2 text-xs font-medium uppercase transition-colors px-4 first:rounded-tl-md',
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
							class="absolute bottom-1 left-1/2 -translate-x-1/2 h-0.5 w-[calc(100%-2rem)] bg-primary"
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
</div>

{#snippet cardHeader(label: string)}
	<h5
		class="dark:border-base-400 border-base-300 shrink-0 rounded-t-md border-b px-4 py-2 text-xs font-medium uppercase"
	>
		{label}
	</h5>
{/snippet}

{#snippet empty()}
	<div class="text-muted-content flex h-48 items-center justify-center text-sm font-light">
		{m.audit_usage_usage_no_requests()}
	</div>
{/snippet}
