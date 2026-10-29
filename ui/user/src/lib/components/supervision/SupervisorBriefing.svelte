<script lang="ts">
	import { m } from '$lib/i18n';
	import Loading from '$lib/icons/Loading.svelte';
	import type { SupervisorInsights } from '$lib/services/provenance';
	import { formatTimeAgo } from '$lib/time';
	import SupervisorAvatar from './SupervisorAvatar.svelte';
	import { RefreshCw } from '@lucide/svelte';

	// The Supervisor's first-person briefing on the last day, with a refresh that asks it to
	// write a new one.

	interface Props {
		insights?: SupervisorInsights;
		loading?: boolean;
		refreshing?: boolean;
		unavailable?: boolean;
		onRefresh?: () => void;
	}

	let {
		insights,
		loading = false,
		refreshing = false,
		unavailable = false,
		onRefresh
	}: Props = $props();

	let paragraphs = $derived(
		(insights?.briefing ?? '')
			.split(/\n\s*\n/)
			.map((p) => p.trim())
			.filter(Boolean)
	);
</script>

<section class="paper border-primary/30 gap-3 border" aria-label={m.supervision_briefing_title()}>
	<div class="flex items-center justify-between gap-2">
		<h2 class="flex items-center gap-2 text-base font-semibold">
			<SupervisorAvatar size="sm" />
			{m.supervision_briefing_title()}
		</h2>
		{#if onRefresh}
			<button
				type="button"
				class="btn btn-ghost btn-sm"
				aria-label={m.supervision_briefing_refresh()}
				title={m.supervision_briefing_refresh()}
				disabled={refreshing || loading}
				onclick={onRefresh}
			>
				<RefreshCw class="size-4 {refreshing ? 'animate-spin' : ''}" />
			</button>
		{/if}
	</div>

	{#if loading && !insights}
		<div class="flex justify-center py-4"><Loading class="size-6" /></div>
	{:else if paragraphs.length}
		<div class="flex flex-col gap-2 text-sm leading-relaxed">
			{#each paragraphs as p, i (i)}
				<p>{p}</p>
			{/each}
		</div>
		<span class="text-muted-content text-xs" title={formatTimeAgo(insights?.generatedAt).fullDate}>
			{#if insights?.briefingModel}
				{m.supervision_briefing_by({
					model: insights.briefingModel,
					time: formatTimeAgo(insights.generatedAt).relativeTime
				})}
			{:else}
				{m.supervision_briefing_deterministic({
					time: formatTimeAgo(insights?.generatedAt).relativeTime
				})}
			{/if}
		</span>
	{:else}
		<p class="text-muted-content text-sm">
			{unavailable ? m.supervision_briefing_unavailable() : m.supervision_briefing_none()}
		</p>
	{/if}
</section>
