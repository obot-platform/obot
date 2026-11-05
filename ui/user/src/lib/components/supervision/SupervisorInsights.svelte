<script lang="ts">
	import Confirm from '$lib/components/Confirm.svelte';
	import { parseErrorContent } from '$lib/errors';
	import { m } from '$lib/i18n';
	import Loading from '$lib/icons/Loading.svelte';
	import { performInsightAction, type Insight } from '$lib/services/provenance';
	import { profile } from '$lib/stores';
	import { success } from '$lib/stores/success';
	import { formatTimeAgo } from '$lib/time';
	import { resolve } from '$lib/url';
	import { Info, Lightbulb, RefreshCw, Siren, TriangleAlert } from '@lucide/svelte';
	import { twMerge } from 'tailwind-merge';

	// What the Supervisor noticed on its own, most severe first, each with an optional one-click
	// follow-up. Kill actions ask for confirmation; everything else runs on click.

	interface Props {
		items: Insight[];
		title?: string;
		description?: string;
		emptyText?: string;
		error?: string;
		loading?: boolean;
		/** When the findings were computed; shown as "Updated … ago" when set. */
		generatedAt?: string;
		/** How many to show before "Show all"; the most severe come first. */
		limit?: number;
		onRefresh?: () => void;
		refreshing?: boolean;
		onActed?: (insight: Insight) => void;
	}

	let {
		items,
		title = m.supervision_insights_title(),
		description,
		emptyText = m.supervision_insights_empty(),
		error = '',
		loading = false,
		generatedAt = '',
		limit = 5,
		onRefresh,
		refreshing = false,
		onActed
	}: Props = $props();

	let showAll = $state(false);

	let readonly = $derived(profile.current.isAdminReadonly?.());
	let busy = $state<string>();
	let actionError = $state('');
	let confirming = $state<Insight>();

	const severityOrder = { critical: 0, warning: 1, info: 2 };
	let sorted = $derived(
		[...items].sort((a, b) => (severityOrder[a.severity] ?? 3) - (severityOrder[b.severity] ?? 3))
	);
	let shown = $derived(showAll ? sorted : sorted.slice(0, limit));

	const severityView = {
		critical: { icon: Siren, color: 'text-error', border: 'border-error bg-error/5' },
		warning: { icon: TriangleAlert, color: 'text-warning', border: 'border-warning bg-warning/5' },
		info: { icon: Info, color: 'text-primary', border: 'border-primary/40' }
	};

	function severityLabel(severity: string) {
		switch (severity) {
			case 'critical':
				return m.supervision_insight_severity_critical();
			case 'warning':
				return m.supervision_insight_severity_warning();
			default:
				return m.supervision_insight_severity_info();
		}
	}

	function needsConfirm(insight: Insight) {
		return !!insight.action && /\/kills\b/.test(insight.action.path);
	}

	async function act(insight: Insight) {
		if (!insight.action) return;
		confirming = undefined;
		busy = insight.id;
		actionError = '';
		try {
			await performInsightAction(insight.action, { dontLogErrors: true });
			success.add(m.supervision_insight_done({ label: insight.action.label }));
			onActed?.(insight);
		} catch (e) {
			actionError = parseErrorContent(e).message;
		} finally {
			busy = undefined;
		}
	}
</script>

<section class="paper gap-3" aria-label={title}>
	<div class="flex flex-col gap-1">
		<div class="flex items-center justify-between gap-2">
			<h2 class="flex items-center gap-2 text-base font-semibold">
				<Lightbulb class="text-primary size-4" />
				{title}
			</h2>
			{#if onRefresh}
				<button
					type="button"
					class="btn btn-ghost btn-sm"
					aria-label={m.supervision_insights_refresh()}
					disabled={loading || refreshing}
					onclick={onRefresh}
				>
					<RefreshCw class="size-4 {refreshing ? 'animate-spin' : ''}" />
				</button>
			{/if}
		</div>
		{#if description}
			<p class="text-muted-content text-xs">{description}</p>
		{/if}
		{#if generatedAt}
			<span class="text-muted-content text-xs" title={formatTimeAgo(generatedAt).fullDate}>
				{m.supervision_insights_updated({ time: formatTimeAgo(generatedAt).relativeTime })}
			</span>
		{/if}
	</div>

	{#if actionError}
		<div class="notification-error rounded-md p-3 text-sm" role="alert">{actionError}</div>
	{/if}

	{#if loading && items.length === 0}
		<div class="flex justify-center py-4"><Loading class="size-6" /></div>
	{:else if error && items.length === 0}
		<p class="text-muted-content text-sm">{error}</p>
	{:else if sorted.length === 0}
		<p class="text-muted-content text-sm">{emptyText}</p>
	{:else}
		<ul class="flex flex-col gap-2">
			{#each shown as insight (insight.id)}
				{@const view = severityView[insight.severity] ?? severityView.info}
				<li
					class={twMerge('flex items-start gap-3 rounded-md border-l-2 p-3', view.border)}
					aria-label={insight.title}
				>
					<view.icon class={twMerge('mt-0.5 size-4 shrink-0', view.color)} />
					<div class="flex min-w-0 flex-1 flex-col gap-1">
						<p class="text-sm font-medium">
							<span class="sr-only">{severityLabel(insight.severity)}:</span>
							{insight.title}
						</p>
						{#if insight.detail}
							<p class="text-muted-content line-clamp-3 text-xs" title={insight.detail}>
								{insight.detail}
							</p>
						{/if}
						<div class="flex flex-wrap items-center gap-3 text-xs">
							{#if insight.action && !readonly}
								<button
									type="button"
									class={twMerge(
										'btn btn-xs',
										insight.severity === 'critical' ? 'btn-error' : 'btn-primary'
									)}
									disabled={!!busy}
									onclick={() => (needsConfirm(insight) ? (confirming = insight) : act(insight))}
								>
									{#if busy === insight.id}<Loading class="size-3" />{/if}
									{insight.action.label}
								</button>
							{/if}
							{#if insight.sessionID}
								<a href={resolve(`/sessions/${insight.sessionID}`)} class="text-link">
									{m.supervision_insight_open_session()}
								</a>
							{/if}
							{#if insight.policyID}
								<a href={resolve(`/policies/${insight.policyID}`)} class="text-link">
									{m.supervision_insight_open_policy()}
								</a>
							{/if}
						</div>
					</div>
				</li>
			{/each}
		</ul>
		{#if sorted.length > limit}
			<button
				type="button"
				class="btn btn-ghost btn-sm self-center text-xs"
				onclick={() => (showAll = !showAll)}
			>
				{showAll
					? m.supervision_insights_show_fewer()
					: m.supervision_insights_show_all({ count: sorted.length })}
			</button>
		{/if}
	{/if}
</section>

<Confirm
	show={!!confirming}
	type="info"
	msg={m.supervision_insight_confirm_kill({ label: confirming?.action?.label ?? '' })}
	submitText={confirming?.action?.label}
	onsuccess={() => confirming && act(confirming)}
	oncancel={() => (confirming = undefined)}
/>
