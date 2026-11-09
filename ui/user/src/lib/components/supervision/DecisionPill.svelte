<script lang="ts">
	import { m } from '$lib/i18n';
	import { effectLabel, type ApprovalStatus, type Decision } from '$lib/services/provenance';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		decision?: Decision | null;
		/** The current status of the approval the decision asked for, when known. The decision
		 * itself is fixed when the action was held, so without it the pill says pending forever. */
		approvalStatus?: ApprovalStatus;
		class?: string;
	}

	let { decision, approvalStatus, class: klass }: Props = $props();

	// What the pill says, and its color: enforced outcome first, then a shadow would-have-been.
	let view = $derived.by(() => {
		if (!decision) return undefined;
		if (decision.killed) {
			return { text: m.supervision_decision_killed(), color: 'badge-error' };
		}
		if (decision.approvalID && decision.effect === 'require_approval' && !decision.shadow) {
			switch (approvalStatus) {
				case 'approved':
					return { text: m.supervision_decision_approval_approved(), color: 'badge-success' };
				case 'denied':
					return { text: m.supervision_decision_approval_denied(), color: 'badge-error' };
				case 'expired':
					return { text: m.supervision_decision_approval_expired(), color: 'badge-neutral' };
			}
			return { text: m.supervision_decision_approval_pending(), color: 'badge-warning' };
		}
		if (decision.shadow && decision.effect !== 'allow') {
			return {
				text: m.supervision_decision_shadow({ effect: effectLabel(decision.effect) }),
				color: 'badge-warning'
			};
		}
		if (!decision.allow) {
			return { text: m.supervision_decision_denied(), color: 'badge-error' };
		}
		if (decision.effect !== 'allow') {
			return { text: effectLabel(decision.effect), color: 'badge-primary' };
		}
		return { text: m.supervision_decision_allowed(), color: 'badge-success' };
	});
</script>

{#if view}
	<span class={twMerge('badge badge-sm badge-soft whitespace-nowrap', view.color, klass)}>
		{view.text}
	</span>
{/if}
