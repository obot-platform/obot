<script lang="ts">
	import CopyButton from '$lib/components/CopyButton.svelte';
	import JsonPreview from '$lib/components/JsonPreview.svelte';
	import IconButton from '$lib/components/primitives/IconButton.svelte';
	import { m } from '$lib/i18n';
	import type { AuditLogEvent } from '$lib/services';
	import { userDeviceSettings } from '$lib/stores';
	import { formatLogTimestamp } from '$lib/time';
	import { X } from '@lucide/svelte';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		auditLog: AuditLogEvent & { user: string };
		onClose: () => void;
	}

	let { auditLog, onClose }: Props = $props();
	const details = $derived(auditLog.details);

	function hasBody(body: unknown) {
		if (body == null) return false;
		if (typeof body === 'object' && !Array.isArray(body)) return Object.keys(body).length > 0;
		return true;
	}

	function formatHeaderValue(value: string | string[]) {
		const values = Array.isArray(value) ? value : [value];
		return values.map((v) => `"${v}"`).join(', ');
	}
</script>

<div class="bg-base-200 text-base-content flex h-full w-[inherit] min-w-[inherit] flex-col">
	<div class="dark:bg-base-300 bg-base-100 relative flex w-full flex-col p-4 pl-5 shadow-xs">
		<div
			class={twMerge(
				'absolute top-0 left-0 h-full w-1',
				auditLog.outcome.status === 'success' && 'bg-primary',
				auditLog.outcome.status === 'unknown' && 'bg-base-400',
				['failure', 'denied', 'timeout'].includes(auditLog.outcome.status) && 'bg-error'
			)}
		></div>
		<h3 class="text-lg font-semibold">
			{formatLogTimestamp(auditLog.timestamp.occurredAt, userDeviceSettings.timeFormat)}
		</h3>
		<p class="text-muted-content text-xs font-light">
			{auditLog.action.name || auditLog.action.operation}
		</p>
		<IconButton onclick={onClose} class="absolute top-1/2 right-4 -translate-y-1/2">
			<X class="size-5" />
		</IconButton>
	</div>

	<div class="default-scrollbar-thin relative h-[calc(100%-60px)] overflow-y-auto pb-4">
		<div class="bg-base-300 absolute top-0 left-0 h-full w-1"></div>

		<div class="flex flex-wrap gap-2 py-4 px-5">
			{@render chip(m.admin_sub_log_event(), auditLog.eventType)}
			{@render chip(m.admin_sub_log_outcome(), auditLog.outcome.status)}
			{@render chip(m.admin_sub_log_operation(), auditLog.action.operation)}
			{@render chip(m.admin_sub_log_target(), auditLog.target.targetType)}
		</div>

		<div class="px-5 flex flex-col gap-4">
			{#if details?.payloadRedacted}
				<div class="bg-base-300 text-muted-content rounded-md p-3 text-xs italic">
					{m.admin_sub_log_payload_hidden_for_access_level()}
				</div>
			{:else if details}
				{#if hasBody(details.request?.body)}
					{@render jsonBody(m.admin_sub_log_request_tool_input(), details.request?.body)}
				{/if}
				{#if hasBody(details.request?.mutatedBody)}
					{@render jsonBody(m.admin_sub_log_mutated_request_body(), details.request?.mutatedBody)}
				{/if}

				{#if hasBody(details.response?.originalBody)}
					{@render jsonBody(
						m.admin_sub_log_original_response_body(),
						details.response?.originalBody
					)}
				{/if}
				{#if hasBody(details.response?.body)}
					{@render jsonBody(m.admin_sub_log_response_tool_output(), details.response?.body)}
				{/if}
			{/if}

			<div class="divider my-0 text-xs uppercase">{m.admin_sub_log_additional_information()}</div>

			{#if details}
				{#if details.payloadRedacted}
					<div class="bg-base-300 text-muted-content rounded-md p-3 text-xs italic">
						{m.admin_sub_log_hidden_for_access_level()}
					</div>
					<div class="divider my-0"></div>
				{:else}
					{#if details.environment}
						<div class="divider my-0"></div>
						<div class="flex flex-col gap-0.5">
							{@render title(m.admin_sub_log_environment())}
							<div class="flex flex-col gap-1 px-4 text-sm font-light">
								{@render field(m.admin_sub_log_working_directory(), details.environment.cwd)}
								{@render field(m.admin_sub_log_git_root(), details.environment.gitRoot)}
								{@render field(m.admin_sub_log_git_branch(), details.environment.gitBranch)}
								{@render field(m.admin_sub_log_git_commit(), details.environment.gitCommit)}
								{@render field(
									m.admin_sub_log_git_remotes(),
									details.environment.gitRemotes?.join(', ')
								)}
								{@render field(m.admin_sub_log_hostname(), details.device?.hostname)}
								{@render field(m.admin_sub_log_local_username(), details.device?.localUsername)}
								{@render field(
									m.admin_sub_log_reported_email(),
									details.environment.reportedUserEmail
								)}
								{@render field(
									m.admin_sub_log_transcript_path(),
									details.environment.transcriptPath
								)}
							</div>
						</div>
					{/if}

					{#if hasBody(details.request?.headers)}
						{@render headersBody(m.admin_sub_log_request_headers(), details.request?.headers)}
					{/if}
					{#if hasBody(details.response?.headers)}
						{@render headersBody(m.admin_sub_log_response_headers(), details.response?.headers)}
					{/if}
					{#if hasBody(details.rawEvent)}
						{@render jsonBody(m.admin_sub_log_raw_event(), details.rawEvent)}
					{/if}
					{#if details.environment || details.request?.headers || details.response?.headers || details.rawEvent}
						<div class="divider my-0"></div>
					{/if}
				{/if}
			{/if}

			<div class="flex flex-col gap-0.5">
				{@render title(m.admin_sub_log_event())}
				<div class="flex flex-col gap-1 px-4 text-sm font-light">
					{@render field(
						m.admin_sub_log_actor(),
						auditLog.user || auditLog.actor.id || m.admin_sub_log_unknown()
					)}
					{@render field(m.admin_sub_log_actor_type(), auditLog.actor.actorType)}
					{@render field(m.admin_sub_log_credential(), auditLog.actor.credentialID)}
					{@render field(m.admin_sub_log_action(), auditLog.action.name)}
					{@render field(m.admin_sub_log_action_kind(), auditLog.action.kind)}
					{@render field(m.admin_sub_log_target(), auditLog.target.name || auditLog.target.id)}
					{@render field(
						m.admin_sub_log_parent_target(),
						auditLog.target.parent?.name || auditLog.target.parent?.id
					)}
					{@render field(m.admin_sub_log_http_status(), auditLog.outcome.httpStatus)}
					{@render field(m.admin_sub_log_reason(), auditLog.outcome.reason)}
					{@render field(m.admin_sub_log_duration_ms(), auditLog.outcome.durationMs)}
					{@render field(
						m.admin_sub_log_recorded_at(),
						formatLogTimestamp(auditLog.timestamp.recordedAt, userDeviceSettings.timeFormat)
					)}
					{@render field(m.admin_sub_log_timestamp_source(), auditLog.timestamp.source)}
				</div>
			</div>

			{#if auditLog.outcome.error}
				<div class="divider my-0"></div>
				<div class="flex flex-col gap-0.5">
					<div class="text-base font-semibold">{m.admin_sub_log_error()}</div>
					<p class="text-error text-sm">{auditLog.outcome.error}</p>
				</div>
			{/if}

			{#if details}
				{#if details.trace || details.network}
					<div class="divider my-0"></div>
					<div class="flex flex-col gap-0.5">
						{@render title(m.admin_sub_log_trace_network())}
						<div class="flex flex-col gap-1 px-4 text-sm font-light">
							{@render field(m.admin_sub_log_session_id(), details.trace?.sessionID)}
							{@render field(m.admin_sub_log_request_id(), details.trace?.requestID)}
							{@render field(m.admin_sub_log_idempotency_key(), details.trace?.idempotencyKey)}
							{@render field(m.admin_sub_log_tool_use_id(), details.trace?.toolUseID)}
							{@render field(m.admin_sub_log_turn_id(), details.trace?.turnID)}
							{@render field(m.admin_sub_log_client_ip(), details.network?.clientIP)}
							{@render field(
								m.admin_sub_log_started_at(),
								details.startedAt
									? formatLogTimestamp(details.startedAt, userDeviceSettings.timeFormat)
									: undefined
							)}
						</div>
					</div>
				{/if}

				{#if details.client || details.scope}
					<div class="divider my-0"></div>
					<div class="flex flex-col gap-0.5">
						{@render title(m.admin_sub_log_mcp_context())}
						<div class="flex flex-col gap-1 px-4 text-sm font-light">
							{@render field(
								m.admin_sub_log_client(),
								[details.client?.name, details.client?.version].filter(Boolean).join(' / ')
							)}
							{@render field(m.admin_sub_log_user_agent(), details.client?.userAgent)}
							{@render field(m.admin_sub_log_workspace(), details.scope?.powerUserWorkspaceID)}
							{@render field(
								m.admin_sub_log_catalog_entry(),
								details.scope?.mcpServerCatalogEntryName
							)}
						</div>
					</div>
				{/if}

				{#if details.agent || details.device}
					<div class="divider my-0"></div>
					<div class="flex flex-col gap-0.5">
						{@render title(m.admin_sub_log_agent_device())}
						<div class="flex flex-col gap-1 px-4 text-sm font-light">
							{@render field(
								m.admin_sub_log_agent(),
								[details.agent?.provider, details.agent?.version].filter(Boolean).join(' / ')
							)}
							{@render field(
								'CLI',
								[details.agent?.cliName, details.agent?.cliVersion].filter(Boolean).join(' / ')
							)}
							{@render field(
								m.admin_sub_log_model(),
								[details.agent?.model, details.agent?.modelID].filter(Boolean).join(' / ')
							)}
							{@render field(m.admin_sub_log_permission_mode(), details.agent?.permissionMode)}
							{@render field(m.admin_sub_log_device(), details.device?.id)}
							{@render field(m.admin_sub_log_deployment_id(), details.device?.deploymentID)}
							{@render field(
								m.admin_sub_log_os_architecture(),
								[details.device?.os, details.device?.architecture].filter(Boolean).join(' / ')
							)}
						</div>
					</div>
				{/if}

				{#if details.webhookStatuses?.length}
					{@render jsonBody(m.admin_sub_log_webhook_statuses(), details.webhookStatuses)}
				{/if}
			{/if}
		</div>
	</div>
</div>

{#snippet title(label: string)}
	<p class="text-base font-semibold mb-2">{label}</p>
{/snippet}

{#snippet chip(label: string, value: string | undefined | null)}
	{#if value}
		<div class="bg-base-400 rounded-full px-3 py-1 text-[11px] font-light">
			<span class="font-medium">{label}:</span>
			{value}
		</div>
	{/if}
{/snippet}

{#snippet field(label: string, value: string | number | undefined | null)}
	{#if value !== undefined && value !== null && value !== ''}
		<p class="grid grid-cols-2 gap-2 break-all">
			<span class="font-medium">{label}:</span>
			{value}
		</p>
	{/if}
{/snippet}

{#snippet headersBody(
	name: string,
	headers: Record<string, string | string[]> | string | undefined
)}
	{@const text =
		typeof headers === 'string'
			? headers
			: Object.entries(headers ?? {})
					.map(([key, value]) => `${key}: ${formatHeaderValue(value)}`)
					.join('\n')}
	<div class="flex flex-col gap-0.5">
		<p class="text-base font-semibold flex items-center gap-2">
			{name}
			<CopyButton classes={{ button: 'text-xs font-normal flex items-center gap-1' }} {text} />
		</p>
		<div class="relative mt-2">
			<JsonPreview value={text} ariaLabel={`${name} JSON`} maximizable />
		</div>
	</div>
{/snippet}

{#snippet jsonBody(name: string, value: unknown)}
	<div class="flex flex-col gap-0.5">
		<p class="text-base font-semibold flex items-center gap-2">
			{name}
			<CopyButton
				text={typeof value === 'string' ? value : JSON.stringify(value, null, 2)}
				classes={{ button: 'text-xs font-normal flex items-center gap-1' }}
			/>
		</p>
		<div class="relative mt-2">
			<JsonPreview {value} ariaLabel={`${name} JSON`} maximizable />
		</div>
	</div>
{/snippet}
