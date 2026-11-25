<script lang="ts">
	import { isAbortError } from '$lib/errors';
	import { AdminService, type TokenUsageStats, type TokenUsageStatsFilters } from '$lib/services';
	import { errors } from '$lib/stores';
	import TokenUsageRequestTiles from './TokenUsageRequestTiles.svelte';
	import TokenUsageRequestsChart from './TokenUsageRequestsChart.svelte';
	import TokenUsageSpendByProvider from './TokenUsageSpendByProvider.svelte';
	import TokenUsageTopModels from './TokenUsageTopModels.svelte';

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
</script>

<div class="flex flex-col gap-4">
	<TokenUsageRequestTiles {loading} {stats} />

	<div class="grid grid-cols-1 gap-4 xl:grid-cols-3">
		<TokenUsageRequestsChart {loading} {stats} {startDate} {endDate} />
		<TokenUsageSpendByProvider {loading} {stats} />
	</div>

	<!-- No overflow-hidden here: it would make this card the table's scroll container, and the
	table's sticky header, offset for the page's own sticky bar, would cover its first rows. -->
	<TokenUsageTopModels {loading} {stats} {modelLabel} />
</div>
