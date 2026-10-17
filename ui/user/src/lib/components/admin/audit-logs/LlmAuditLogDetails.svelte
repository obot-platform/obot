<script lang="ts">
	import { formatAuditLogAPIKeyName, getAuditLogAPIKeyMaskedKey } from '$lib/auditlogs';
	import { isAbortError } from '$lib/errors';
	import { m } from '$lib/i18n';
	import { AdminService, type LLMAuditLog } from '$lib/services';
	import { formatTokenUsageUSD } from '../token-usage/tokenUsageTimeline';
	import AuditLogDetails from './AuditLogDetails.svelte';
	import LlmAuditLogGuardrails from './LlmAuditLogGuardrails.svelte';
	import LlmAuditLogRouting, { servedBy } from './LlmAuditLogRouting.svelte';
	import { CircleAlert } from '@lucide/svelte';

	export type LlmAuditLogDetail = LLMAuditLog & { user: string };
	interface Props {
		id: string;
		auditLog: LlmAuditLogDetail;
		onClose: () => void;
	}
	let { id, auditLog, onClose }: Props = $props();
	let loading = $state(true);
	let fetchError = $state<string | null>(null);
	let additData = $state<LLMAuditLog>();

	$effect(() => {
		if (!id) return;

		const controller = new AbortController();
		loading = true;
		fetchError = null;

		AdminService.getLLMAuditLog(id, { signal: controller.signal })
			.then((response) => {
				if (controller.signal.aborted) return;
				additData = response;
			})
			.catch((err) => {
				if (isAbortError(err) || controller.signal.aborted) return;
				console.error('Failed to fetch LLM audit log details:', err);
				fetchError =
					err instanceof Error ? err.message : m.audit_usage_audit_logs_model_load_details_failed();
			})
			.finally(() => {
				if (controller.signal.aborted) return;
				loading = false;
			});

		return () => controller.abort();
	});

	const titles = {
		modelProvider: m.audit_usage_audit_logs_model_model_provider(),
		modelID: m.audit_usage_audit_logs_model_model_id()
	};

	const properties = ['modelProvider', 'modelID'] as const;

	// tokenSummary describes a request's tokens, with the cached input and reasoning output
	// they include, e.g. "3,132 input (3,072 read from cache) · 40 output (12 reasoning)".
	function tokenSummary(log: LLMAuditLog) {
		if (!log.inputTokens && !log.outputTokens) return '';
		const n = (value: number) => Intl.NumberFormat().format(value);
		const withDetail = (main: string, detail: string[]) =>
			detail.length ? `${main} (${detail.join(', ')})` : main;
		const cached: string[] = [];
		if (log.cacheReadTokens) {
			cached.push(
				m.audit_usage_audit_logs_model_tokens_cache_read({ count: n(log.cacheReadTokens) })
			);
		}
		if (log.cacheWriteTokens) {
			cached.push(
				m.audit_usage_audit_logs_model_tokens_cache_write({ count: n(log.cacheWriteTokens) })
			);
		}
		const input = withDetail(
			m.audit_usage_audit_logs_model_tokens_input({ count: n(log.inputTokens) }),
			cached
		);
		const output = withDetail(
			m.audit_usage_audit_logs_model_tokens_output({ count: n(log.outputTokens) }),
			log.reasoningTokens
				? [m.audit_usage_audit_logs_model_tokens_reasoning({ count: n(log.reasoningTokens) })]
				: []
		);
		return `${input} · ${output}`;
	}
</script>

{#if fetchError}
	<div class="notification-error m-4 flex items-center gap-3 p-3">
		<CircleAlert class="size-4 shrink-0" />
		<div class="flex flex-col gap-1">
			<p class="text-sm font-semibold">{m.audit_usage_audit_logs_model_unable_to_load_details()}</p>
			<p class="text-sm font-light">{fetchError}</p>
		</div>
	</div>
{/if}

<AuditLogDetails
	auditLog={{
		...auditLog,
		...additData
	}}
	{onClose}
	loading={{
		request: loading,
		response: loading
	}}
	shouldShowPayload
>
	{#snippet preRequestContent(data)}
		<div class="flex flex-wrap gap-2 p-4 pl-5">
			{#each properties as property (property)}
				{#if data[property as keyof typeof data]}
					<div class="bg-base-400 rounded-full px-3 py-1 text-[11px] font-light truncate">
						<span class="font-medium">{titles[property as keyof typeof titles]}:</span>
						{data[property as keyof typeof data]}
					</div>
				{/if}
			{/each}
		</div>
		{#if servedBy(data) || data.providerKey}
			<div class="px-5 pb-4">
				<LlmAuditLogRouting auditLog={data} />
			</div>
		{/if}
		{#if data.guardrailFindings?.length}
			<div class="px-5 pb-4">
				<LlmAuditLogGuardrails findings={data.guardrailFindings} />
			</div>
		{/if}
	{/snippet}

	{#snippet additRequestContent(data)}
		{@const requestURL = [data.requestMethod, data.requestPath].filter(Boolean).join(' ')}
		{@const apiKey = formatAuditLogAPIKeyName(
			data.apiKeyName ?? '',
			getAuditLogAPIKeyMaskedKey(data.userID, data.apiKeyID)
		)}
		{#if requestURL}
			<p class="grid grid-cols-2 gap-2 wrap-break-word">
				<span class="font-medium">{m.audit_usage_audit_logs_model_request_url_label()}</span>
				{requestURL}
			</p>
		{/if}
		{#if data.userAgent}
			<p class="grid grid-cols-2 gap-2 wrap-break-word">
				<span class="font-medium">{m.audit_usage_audit_logs_model_user_agent_label()}</span>
				{data.userAgent}
			</p>
		{/if}
		{#if apiKey}
			<p class="grid grid-cols-2 gap-2 wrap-break-word">
				<span class="font-medium">{m.audit_usage_audit_logs_model_api_key_label()}</span>
				{apiKey}
			</p>
		{/if}
		{#if tokenSummary(data)}
			<p class="grid grid-cols-2 gap-2">
				<span class="font-medium">{m.audit_usage_audit_logs_model_tokens_label()}</span>
				{tokenSummary(data)}
			</p>
		{/if}
		{#if data.spend}
			<p class="grid grid-cols-2 gap-2">
				<span class="font-medium">{m.audit_usage_audit_logs_model_spend_label()}</span>
				{formatTokenUsageUSD(data.spend)}
			</p>
		{/if}
	{/snippet}
</AuditLogDetails>
