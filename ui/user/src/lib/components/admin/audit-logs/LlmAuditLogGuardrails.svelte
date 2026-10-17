<script lang="ts" module>
	import { m } from '$lib/i18n';
	import type { LLMAuditGuardrailFinding } from '$lib/services';

	type Outcome = LLMAuditGuardrailFinding['outcome'];

	// Most severe first, so a request's indicator shows the strongest action taken.
	const outcomeOrder: Outcome[] = ['blocked', 'redacted', 'error', 'logged'];

	export function guardrailOutcomeLabel(outcome: Outcome) {
		switch (outcome) {
			case 'blocked':
				return m.audit_usage_audit_logs_model_guardrail_outcome_blocked();
			case 'redacted':
				return m.audit_usage_audit_logs_model_guardrail_outcome_redacted();
			case 'logged':
				return m.audit_usage_audit_logs_model_guardrail_outcome_logged();
			case 'error':
				return m.audit_usage_audit_logs_model_guardrail_outcome_error();
			default:
				return outcome;
		}
	}

	export function guardrailOutcomeClass(outcome: Outcome) {
		switch (outcome) {
			case 'blocked':
			case 'error':
				return 'bg-error/15 text-error';
			case 'redacted':
				return 'bg-warning/15 text-warning';
			default:
				return 'bg-base-300 text-base-content';
		}
	}

	// guardrailOutcomes returns the distinct outcomes of a request's findings, most severe first.
	export function guardrailOutcomes(findings: LLMAuditGuardrailFinding[] | undefined) {
		return outcomeOrder.filter((o) => findings?.some((f) => f.outcome === o));
	}
</script>

<script lang="ts">
	import { twMerge } from 'tailwind-merge';

	interface Props {
		findings: LLMAuditGuardrailFinding[];
	}

	let { findings }: Props = $props();

	function stageLabel(stage: LLMAuditGuardrailFinding['stage']) {
		return stage === 'output'
			? m.audit_usage_audit_logs_model_guardrail_stage_output()
			: m.audit_usage_audit_logs_model_guardrail_stage_input();
	}

	function guardrailLabel(finding: LLMAuditGuardrailFinding) {
		if (
			finding.guardrailName &&
			finding.guardrailType &&
			finding.guardrailName !== finding.guardrailType
		) {
			return `${finding.guardrailName} (${finding.guardrailType})`;
		}
		return finding.guardrailName || finding.guardrailType || '';
	}
</script>

<section class="flex flex-col gap-0.5" aria-label={m.audit_usage_audit_logs_model_guardrails()}>
	<p class="text-base font-semibold">{m.audit_usage_audit_logs_model_guardrails()}</p>
	<p class="text-muted-content mb-2 text-xs font-light">
		{m.audit_usage_audit_logs_model_guardrails_description()}
	</p>
	<ul class="flex flex-col gap-2">
		{#each findings as finding, i (i)}
			{@const guardrail = guardrailLabel(finding)}
			<li class="bg-base-100 dark:bg-base-300 flex flex-col gap-1 rounded-md p-3 text-sm">
				<div class="flex flex-wrap items-center gap-2">
					<span
						class={twMerge(
							'rounded-full px-2 py-0.5 text-xs font-medium',
							guardrailOutcomeClass(finding.outcome)
						)}
					>
						{guardrailOutcomeLabel(finding.outcome)}
					</span>
					<span class="font-medium wrap-break-word">{finding.policyName || finding.policyID}</span>
					<span class="text-muted-content text-xs">{stageLabel(finding.stage)}</span>
				</div>
				{#if guardrail}
					<p class="text-xs font-light wrap-break-word">
						<span class="font-medium">{m.audit_usage_audit_logs_model_guardrail_label()}</span>
						{guardrail}
					</p>
				{/if}
				{#if finding.categories?.length}
					<div class="flex flex-wrap items-center gap-1 text-xs font-light">
						<span class="font-medium">
							{m.audit_usage_audit_logs_model_guardrail_categories_label()}
						</span>
						{#each finding.categories as category (category)}
							<span class="bg-base-400 rounded-full px-2 py-0.5 text-[11px]">{category}</span>
						{/each}
					</div>
				{/if}
				{#if finding.error}
					<p class="text-error text-xs font-light wrap-break-word">{finding.error}</p>
				{/if}
			</li>
		{/each}
	</ul>
</section>
