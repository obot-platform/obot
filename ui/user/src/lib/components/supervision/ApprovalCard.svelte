<script lang="ts">
	import { parseErrorContent } from '$lib/errors';
	import { m } from '$lib/i18n';
	import Loading from '$lib/icons/Loading.svelte';
	import {
		approvalStatusLabel,
		approveApproval,
		denyApproval,
		describeAction,
		type Approval
	} from '$lib/services/provenance';
	import { profile } from '$lib/stores';
	import { success } from '$lib/stores/success';
	import { formatTimeAgo, formatTimeUntil } from '$lib/time';
	import { resolve } from '$lib/url';
	import LabelBadges from './LabelBadges.svelte';
	import RiskPill from './RiskPill.svelte';
	import SupervisorAvatar from './SupervisorAvatar.svelte';
	import SurfaceBadge from './SurfaceBadge.svelte';
	import { Check, X } from '@lucide/svelte';
	import { twMerge } from 'tailwind-merge';

	// One approval request, recommendation first: the Supervisor's call on what a policy held,
	// then what was held and why, then accept-recommendation, approve and deny for pending ones.

	interface Props {
		approval: Approval;
		onResolved?: (approval: Approval) => void;
		highlighted?: boolean;
		/** A condensed card for the Supervisor home's attention queue. */
		compact?: boolean;
	}

	let { approval, onResolved, highlighted = false, compact = false }: Props = $props();

	let note = $state('');
	let busy = $state<'approve' | 'deny' | 'accept'>();
	let error = $state('');
	let readonly = $derived(profile.current.isAdminReadonly?.());
	let pending = $derived(approval.status === 'pending');

	// The Supervisor's recommendation in its own voice. Only approve and deny can be accepted;
	// anything else is the Supervisor saying a person should decide.
	let recommendation = $derived.by(() => {
		const triage = approval.triage;
		if (!triage) return undefined;
		switch (triage.recommendation) {
			case 'approve':
				return {
					kind: 'approve' as const,
					text: m.supervision_recommend_approve(),
					tone: 'border-success bg-success/5',
					button: 'btn-success'
				};
			case 'deny':
				return {
					kind: 'deny' as const,
					text: m.supervision_recommend_deny(),
					tone: 'border-error bg-error/5',
					button: 'btn-error'
				};
			default:
				return {
					kind: undefined,
					text: m.supervision_recommend_escalate(),
					tone: 'border-warning bg-warning/5',
					button: ''
				};
		}
	});

	async function resolveWith(kind: 'approve' | 'deny', accepted = false) {
		busy = accepted ? 'accept' : kind;
		error = '';
		const text = note.trim() || (accepted ? m.supervision_recommend_accept_note() : '');
		try {
			const resolved =
				kind === 'approve'
					? await approveApproval(approval.id, text, { dontLogErrors: true })
					: await denyApproval(approval.id, text, { dontLogErrors: true });
			success.add(
				kind === 'approve'
					? m.supervision_approvals_approved_toast()
					: m.supervision_approvals_denied_toast()
			);
			onResolved?.(resolved);
		} catch (e) {
			error = parseErrorContent(e).message;
		} finally {
			busy = undefined;
		}
	}
</script>

<article
	class={twMerge('paper gap-3', highlighted && 'border-primary')}
	aria-label={approval.policyName}
	id={`approval-${approval.id}`}
>
	{#if recommendation && approval.triage}
		<section
			class={twMerge('flex items-start gap-3 rounded-md border-l-2 p-3', recommendation.tone)}
			aria-label={m.supervision_recommend_title()}
		>
			<SupervisorAvatar size="sm" />
			<div class="flex min-w-0 flex-col gap-1">
				<p class="text-sm">
					<span class="font-semibold">{recommendation.text}</span>
					<span class="text-muted-content ml-1 text-xs">
						{m.supervision_approvals_triage_confidence({
							percent: Math.round((approval.triage.confidence ?? 0) * 100)
						})}
					</span>
				</p>
				<p
					class={twMerge('text-sm', compact && 'line-clamp-3')}
					title={compact ? approval.triage.rationale : undefined}
				>
					{approval.triage.rationale}
				</p>
				<span class="text-muted-content text-xs" title={formatTimeAgo(approval.triage.at).fullDate}>
					{m.supervision_recommend_by({
						model: approval.triage.model || m.supervision_presence_name(),
						time: formatTimeAgo(approval.triage.at).relativeTime
					})}
				</span>
			</div>
		</section>
	{:else if pending}
		<div class="text-muted-content flex items-center gap-3 rounded-md p-1 text-sm">
			<SupervisorAvatar size="sm" active={false} />
			{m.supervision_recommend_none()}
		</div>
	{/if}

	<div class="flex flex-wrap items-start justify-between gap-2">
		<div class="flex min-w-0 flex-col gap-1">
			<span class="text-muted-content text-xs uppercase">{m.supervision_approvals_policy()}</span>
			<h3 class="text-base font-semibold">
				<a href={resolve(`/policies/${approval.policyID}`)} class="hover:underline">
					{approval.policyName}
				</a>
			</h3>
		</div>
		<div class="flex items-center gap-2">
			<RiskPill risk={approval.risk} labelled />
			<span
				class={twMerge(
					'badge badge-sm badge-soft',
					approval.status === 'approved'
						? 'badge-success'
						: approval.status === 'denied'
							? 'badge-error'
							: approval.status === 'expired'
								? 'badge-ghost'
								: 'badge-warning'
				)}
			>
				{approvalStatusLabel(approval.status)}
			</span>
		</div>
	</div>

	<p class={twMerge('text-sm whitespace-pre-wrap', compact && 'text-muted-content line-clamp-3')}>
		{approval.reason}
	</p>

	<dl class="grid grid-cols-1 gap-x-6 gap-y-1 text-sm md:grid-cols-2">
		<div class="flex gap-2">
			<dt class="text-muted-content w-20 shrink-0">{m.supervision_approvals_action()}</dt>
			<dd class="flex min-w-0 flex-wrap items-center gap-1">
				<SurfaceBadge surface={approval.action.surface} short />
				<code class="truncate text-xs">{describeAction(approval.action)}</code>
			</dd>
		</div>
		{#if !compact}
			<div class="flex gap-2">
				<dt class="text-muted-content w-20 shrink-0">{m.core_col_user()}</dt>
				<dd class="truncate">{approval.userID || m.core_unknown_user()}</dd>
			</div>
			<div class="flex gap-2">
				<dt class="text-muted-content w-20 shrink-0">{m.supervision_labels()}</dt>
				<dd>
					{#if approval.labels?.length}
						<LabelBadges labels={approval.labels} />
					{:else}
						<span class="text-muted-content">{m.supervision_none()}</span>
					{/if}
				</dd>
			</div>
		{/if}
		<div class="flex gap-2">
			<dt class="text-muted-content w-20 shrink-0">{m.supervision_col_session()}</dt>
			<dd>
				<a href={resolve(`/sessions/${approval.sessionID}`)} class="text-link">
					{m.supervision_approvals_view_session()}
				</a>
			</dd>
		</div>
	</dl>

	<div class="text-muted-content flex flex-wrap gap-x-4 gap-y-1 text-xs">
		<span title={formatTimeAgo(approval.createdAt).fullDate}>
			{m.supervision_approvals_created({ time: formatTimeAgo(approval.createdAt).relativeTime })}
		</span>
		{#if pending}
			<span title={formatTimeUntil(approval.expiresAt).fullDate}>
				{m.supervision_approvals_expires({
					time: formatTimeUntil(approval.expiresAt).relativeTime
				})}
			</span>
		{:else if approval.resolvedAt}
			<span>
				{m.supervision_approvals_resolved({
					status: approvalStatusLabel(approval.status),
					user: approval.resolvedBy || '',
					time: formatTimeAgo(approval.resolvedAt).relativeTime
				})}
			</span>
		{/if}
		{#if approval.approvers?.length}
			<span>{m.supervision_approvals_approvers({ names: approval.approvers.join(', ') })}</span>
		{/if}
	</div>

	{#if approval.note && !pending}
		<p class="text-muted-content text-xs italic">{approval.note}</p>
	{/if}

	{#if error}
		<div class="notification-error rounded-md p-3 text-sm" role="alert">{error}</div>
	{/if}

	{#if pending && !readonly}
		<div class="flex flex-col gap-2">
			{#if !compact}
				<label for={`approval-note-${approval.id}`} class="text-sm font-light">
					{m.supervision_approvals_note()}
				</label>
				<input
					id={`approval-note-${approval.id}`}
					class="text-input-filled"
					placeholder={m.supervision_approvals_note_placeholder()}
					bind:value={note}
				/>
			{/if}
			<div class="flex flex-wrap items-center justify-end gap-2">
				{#if recommendation?.kind}
					{@const kind = recommendation.kind}
					<button
						type="button"
						class={twMerge('btn mr-auto text-sm', recommendation.button)}
						disabled={!!busy}
						onclick={() => resolveWith(kind, true)}
					>
						{#if busy === 'accept'}
							<Loading class="size-4" />
						{:else if kind === 'approve'}
							<Check class="size-4" />
						{:else}
							<X class="size-4" />
						{/if}
						{m.supervision_recommend_accept()}
					</button>
				{/if}
				<button
					type="button"
					class="btn btn-error btn-soft text-sm"
					disabled={!!busy}
					onclick={() => resolveWith('deny')}
				>
					{#if busy === 'deny'}<Loading
							class="size-4"
						/>{:else}{m.supervision_approvals_deny()}{/if}
				</button>
				<button
					type="button"
					class={twMerge('btn text-sm', recommendation?.kind ? 'btn-secondary' : 'btn-primary')}
					disabled={!!busy}
					onclick={() => resolveWith('approve')}
				>
					{#if busy === 'approve'}<Loading
							class="size-4"
						/>{:else}{m.supervision_approvals_approve()}{/if}
				</button>
			</div>
		</div>
	{/if}
</article>
