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
	import SurfaceBadge from './SurfaceBadge.svelte';
	import { Sparkles } from '@lucide/svelte';
	import { twMerge } from 'tailwind-merge';

	// One approval request: what a policy held, why, the Supervisor's recommendation when it
	// reviewed the request, and approve/deny for pending ones.

	interface Props {
		approval: Approval;
		onResolved?: (approval: Approval) => void;
		highlighted?: boolean;
	}

	let { approval, onResolved, highlighted = false }: Props = $props();

	let note = $state('');
	let busy = $state<'approve' | 'deny'>();
	let error = $state('');
	let readonly = $derived(profile.current.isAdminReadonly?.());
	let pending = $derived(approval.status === 'pending');

	function triageView(rec: string) {
		switch (rec) {
			case 'approve':
				return { text: m.supervision_approvals_triage_approve(), color: 'badge-success' };
			case 'deny':
				return { text: m.supervision_approvals_triage_deny(), color: 'badge-error' };
			default:
				return { text: m.supervision_approvals_triage_escalate(), color: 'badge-warning' };
		}
	}

	async function resolveWith(kind: 'approve' | 'deny') {
		busy = kind;
		error = '';
		try {
			const resolved =
				kind === 'approve'
					? await approveApproval(approval.id, note.trim(), { dontLogErrors: true })
					: await denyApproval(approval.id, note.trim(), { dontLogErrors: true });
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

	<p class="text-sm whitespace-pre-wrap">{approval.reason}</p>

	<dl class="grid grid-cols-1 gap-x-6 gap-y-1 text-sm md:grid-cols-2">
		<div class="flex gap-2">
			<dt class="text-muted-content w-20 shrink-0">{m.supervision_approvals_action()}</dt>
			<dd class="flex min-w-0 flex-wrap items-center gap-1">
				<SurfaceBadge surface={approval.action.surface} short />
				<code class="truncate text-xs">{describeAction(approval.action)}</code>
			</dd>
		</div>
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

	{#if approval.triage}
		{@const view = triageView(approval.triage.recommendation)}
		<div class="bg-primary/5 flex flex-col gap-1 rounded-md p-3 text-sm">
			<div class="flex flex-wrap items-center gap-2">
				<Sparkles class="text-primary size-4" />
				<span class="font-medium">{m.supervision_approvals_triage()}</span>
				<span class={twMerge('badge badge-sm badge-soft', view.color)}>{view.text}</span>
				<span class="text-muted-content text-xs">
					{m.supervision_approvals_triage_confidence({
						percent: Math.round((approval.triage.confidence ?? 0) * 100)
					})}
				</span>
			</div>
			<p class="text-sm">{approval.triage.rationale}</p>
		</div>
	{/if}

	{#if approval.note && !pending}
		<p class="text-muted-content text-xs italic">{approval.note}</p>
	{/if}

	{#if error}
		<div class="notification-error rounded-md p-3 text-sm" role="alert">{error}</div>
	{/if}

	{#if pending && !readonly}
		<div class="flex flex-col gap-2">
			<label for={`approval-note-${approval.id}`} class="text-sm font-light">
				{m.supervision_approvals_note()}
			</label>
			<input
				id={`approval-note-${approval.id}`}
				class="text-input-filled"
				placeholder={m.supervision_approvals_note_placeholder()}
				bind:value={note}
			/>
			<div class="flex justify-end gap-2">
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
					class="btn btn-primary text-sm"
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
