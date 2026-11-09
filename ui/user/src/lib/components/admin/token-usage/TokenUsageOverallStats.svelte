<script lang="ts">
	import GraphTitle from '$lib/components/graph/GraphTitle.svelte';
	import { m } from '$lib/i18n';
	import Loading from '$lib/icons/Loading.svelte';
	import type { TotalTokenUsage } from '$lib/services';
	import { formatTokenUsageUSD as formatUSD } from './tokenUsageTimeline';
	import { twMerge } from 'tailwind-merge';

	type StatId = 'total' | 'input' | 'output' | 'cached-input' | 'spend';

	type Props = {
		title: string;
		loading: boolean;
		totals?: TotalTokenUsage;
		// When omitted, every stat is shown. The dashboard passes the selected subset.
		visible?: readonly StatId[];
		class?: string;
	};

	let { title, loading, totals, visible, class: className }: Props = $props();

	const items = $derived.by(() => {
		const cachedInput = (totals?.cacheReadTokens ?? 0) + (totals?.cacheWriteTokens ?? 0);
		const all: { id: StatId; label: string; value: number; spend?: boolean }[] = [
			{ id: 'total', label: m.core_total_label(), value: totals?.totalTokens ?? 0 },
			{ id: 'input', label: m.core_col_input(), value: totals?.inputTokens ?? 0 },
			{ id: 'output', label: m.core_col_output(), value: totals?.outputTokens ?? 0 },
			{ id: 'cached-input', label: m.audit_usage_usage_cached_input(), value: cachedInput },
			{ id: 'spend', label: m.core_spend(), value: totals?.totalSpend ?? 0, spend: true }
		];
		if (!visible) return all;
		const chosen = new Set<string>(visible);
		return all.filter((item) => chosen.has(item.id));
	});
</script>

<div class={twMerge('paper m-auto w-full gap-0 p-0', className)}>
	<GraphTitle {title} element="h4" />
	<div class="flex flex-col flex-wrap items-stretch gap-4 p-4 md:flex-row">
		{#each items as item, index (item.id)}
			{#if index > 0}
				<div class="divider-horizontal hidden bg-base-300! md:block dark:bg-base-400!"></div>
			{/if}
			{#if item.spend}
				{@render spend(item.label, item.value)}
			{:else}
				{@render summary(item.label, item.value)}
			{/if}
		{/each}
	</div>
</div>

{#snippet summary(label: string, value: number)}
	<div class="flex min-w-0 flex-1 flex-col gap-1 py-2">
		<div class="text-muted-content text-xs font-light">{label}</div>
		<div class="flex items-center gap-1 text-xl font-semibold">
			{#if loading}
				<div class="py-2">
					<Loading class="size-4 animate-spin" />
				</div>
			{:else}
				{value.toLocaleString()}
			{/if}
		</div>
	</div>
{/snippet}

{#snippet spend(label: string, value: number | undefined)}
	<div class="flex min-w-0 flex-1 flex-col gap-1 py-2">
		<div class="text-muted-content text-xs font-light">{label}</div>
		<div class="flex items-center gap-1 text-xl font-semibold">
			{#if loading}
				<div class="py-2">
					<Loading class="size-4 animate-spin" />
				</div>
			{:else}
				{formatUSD(value ?? 0)}
			{/if}
		</div>
	</div>
{/snippet}

<style lang="postcss">
	.divider-horizontal {
		width: 1px;
		height: auto;
		background-color: var(--color-base-400);
		margin-left: 1rem;
		margin-right: 1rem;
	}
</style>
