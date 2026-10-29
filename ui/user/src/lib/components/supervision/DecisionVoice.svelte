<script lang="ts">
	import { m } from '$lib/i18n';
	import { supervisorLine, type ProvenanceEvent, type VoiceTone } from '$lib/services/provenance';
	import { formatTimeAgo } from '$lib/time';
	import { resolve } from '$lib/url';
	import SupervisorAvatar from './SupervisorAvatar.svelte';
	import SurfaceBadge from './SurfaceBadge.svelte';
	import { twMerge } from 'tailwind-merge';

	// One decision told by the Supervisor in the first person. Judged decisions (the Supervisor
	// decided them itself) carry its rationale and stand out; routine allows stay quiet.

	interface Props {
		event: ProvenanceEvent;
		/** Show time, surface and session links (the activity feed); the timeline has its own. */
		meta?: boolean;
		/** Show the engine's reason when the Supervisor did not judge the action itself. */
		reason?: boolean;
		class?: string;
	}

	let { event, meta = true, reason = true, class: klass }: Props = $props();

	let line = $derived(supervisorLine(event));
	let quiet = $derived(line?.tone === 'allowed');
	let detail = $derived(
		line?.judgement.rationale || (reason && line && !quiet ? event.decision?.reason : undefined)
	);

	const toneClass: Record<VoiceTone, string> = {
		judged: 'border-primary bg-primary/5',
		held: 'border-warning bg-warning/5',
		blocked: 'border-error bg-error/5',
		shadow: 'border-base-300 dark:border-base-400',
		acted: 'border-base-300 dark:border-base-400',
		allowed: 'border-transparent'
	};
</script>

{#if line}
	<div
		class={twMerge(
			'flex items-start gap-2 rounded-md border-l-2 px-2 py-1.5',
			toneClass[line.tone],
			klass
		)}
		data-tone={line.tone}
	>
		<SupervisorAvatar size="sm" active={!quiet} />
		<div class="flex min-w-0 flex-1 flex-col gap-0.5">
			<p class={twMerge('text-sm', quiet ? 'text-muted-content' : 'font-medium')}>
				{line.text}
				{#if line.judgement.judged}
					<span class="badge badge-xs badge-primary ml-1 align-middle">
						{m.supervision_feed_my_call()}
					</span>
					{#if line.judgement.confidence !== undefined}
						<span class="text-muted-content ml-1 text-xs font-normal">
							{m.supervision_feed_confidence({ percent: line.judgement.confidence })}
						</span>
					{/if}
				{/if}
			</p>
			{#if detail}
				<p
					class={twMerge(
						'text-muted-content line-clamp-2 text-xs',
						line.judgement.rationale && 'text-base-content italic'
					)}
					title={detail}
				>
					{detail}
				</p>
			{/if}
			{#if meta}
				<div class="text-muted-content flex flex-wrap items-center gap-x-2 gap-y-1 text-xs">
					<span title={formatTimeAgo(event.at).fullDate}
						>{formatTimeAgo(event.at).relativeTime}</span
					>
					<SurfaceBadge surface={event.surface} short />
					<a href={resolve(`/sessions/${event.sessionID}`)} class="text-link">
						{m.supervision_approvals_view_session()}
					</a>
					{#if event.decision?.approvalID && line.tone !== 'allowed'}
						<a href={resolve(`/approvals?approval=${event.decision.approvalID}`)} class="text-link">
							{m.supervision_feed_decide()}
						</a>
					{/if}
				</div>
			{/if}
		</div>
	</div>
{/if}
