<script lang="ts" module>
	import type { LLMAuditLog } from '$lib/services';

	type Routing = Pick<
		LLMAuditLog,
		| 'targetModel'
		| 'servedProvider'
		| 'servedModel'
		| 'providerKey'
		| 'fallback'
		| 'claudeSubscription'
	>;

	// servedBy names the provider and model that served a request, as "provider/model".
	export function servedBy(log: Routing) {
		if (!log.servedModel) return log.servedProvider ?? '';
		return log.servedProvider ? `${log.servedProvider}/${log.servedModel}` : log.servedModel;
	}

	// wasRouted reports whether a request was served by another model than the requested one.
	export function wasRouted(log: Routing) {
		const served = servedBy(log);
		return Boolean(served && log.targetModel && served !== log.targetModel);
	}
</script>

<script lang="ts">
	import { m } from '$lib/i18n';

	interface Props {
		auditLog: Routing & Pick<LLMAuditLog, 'duration'>;
	}

	let { auditLog }: Props = $props();

	// Values are names such as OPENAI_API_KEY_team_rotation or provider/model, so a long one
	// wraps after a _, /, - or ., not in the middle of a word.
	function breakableParts(value: string) {
		return value.split(/(?<=[_/.-])/);
	}

	const rows = $derived(
		[
			{
				label: m.audit_usage_audit_logs_model_routing_requested_model(),
				value: auditLog.targetModel
			},
			{ label: m.audit_usage_audit_logs_model_routing_served_by(), value: servedBy(auditLog) },
			{
				label: m.audit_usage_audit_logs_model_routing_provider_key(),
				value: auditLog.providerKey
			},
			{
				// The user's own Claude subscription paid for the request, not a provider key.
				label: m.audit_usage_audit_logs_model_routing_paid_by(),
				value: auditLog.claudeSubscription
					? m.audit_usage_audit_logs_model_routing_claude_subscription()
					: ''
			},
			{
				label: m.audit_usage_audit_logs_model_routing_fallback(),
				value: auditLog.fallback
					? m.audit_usage_audit_logs_model_routing_yes()
					: m.audit_usage_audit_logs_model_routing_no()
			},
			{
				label: m.audit_usage_audit_logs_model_routing_duration(),
				value: auditLog.duration
					? m.audit_usage_usage_latency_ms({ ms: Intl.NumberFormat().format(auditLog.duration) })
					: ''
			}
		].filter((row) => row.value)
	);
</script>

<section class="flex flex-col gap-0.5" aria-label={m.audit_usage_audit_logs_model_routing()}>
	<p class="text-base font-semibold">{m.audit_usage_audit_logs_model_routing()}</p>
	<p class="text-muted-content mb-2 text-xs font-light">
		{m.audit_usage_audit_logs_model_routing_description()}
	</p>
	<dl
		class="bg-base-100 dark:bg-base-300 grid grid-cols-[auto_minmax(0,1fr)] gap-x-4 gap-y-1 rounded-md p-3 text-sm"
	>
		{#each rows as row (row.label)}
			<dt class="font-medium">{row.label}</dt>
			<dd class="font-light wrap-break-word">
				{#each breakableParts(row.value ?? '') as part, i (i)}{#if i > 0}<wbr />{/if}{part}{/each}
			</dd>
		{/each}
	</dl>
</section>
