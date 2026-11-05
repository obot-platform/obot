<script lang="ts" generics="T extends object">
	import CopyButton from '$lib/components/CopyButton.svelte';
	import JsonPreview from '$lib/components/JsonPreview.svelte';
	import IconButton from '$lib/components/primitives/IconButton.svelte';
	import { m } from '$lib/i18n';
	import Loading from '$lib/icons/Loading.svelte';
	import { Group } from '$lib/services';
	import { profile, userDeviceSettings } from '$lib/stores';
	import { formatLogTimestamp } from '$lib/time';
	import { X } from '@lucide/svelte';
	import type { Snippet } from 'svelte';
	import { twMerge } from 'tailwind-merge';

	type AuditLogHeaders = Record<string, string | string[]> | string;

	interface Props<T> {
		auditLog: T & {
			userID: string;
			createdAt: string;
			responseStatus?: number;
			requestID?: string;
			requestHeaders?: AuditLogHeaders;
			requestBody?: unknown;
			policyModifiedRequestBody?: unknown;
			mutatedRequestBody?: unknown;
			originalResponseBody?: unknown;
			responseBody?: unknown;
			responseHeaders?: AuditLogHeaders;
			error?: string;
			webhookStatuses?: Record<string, string>[];
			user?: string;
		};
		onClose: () => void;
		preRequestContent?: Snippet<[T]>;
		additRequestContent?: Snippet<[T]>;
		loading?: {
			request?: boolean;
			response?: boolean;
		};
		shouldShowPayload?: boolean;
	}

	let {
		auditLog,
		onClose,
		preRequestContent,
		additRequestContent,
		loading,
		shouldShowPayload
	}: Props<T> = $props();
	let hasAuditorAccess = $derived(profile.current.groups.includes(Group.AUDITOR));

	function hasBody(body: unknown) {
		if (body == null) return false;
		if (typeof body === 'object' && !Array.isArray(body)) {
			return Object.keys(body).length > 0;
		}
		return true;
	}

	function hasHeaders(headers: AuditLogHeaders | undefined) {
		if (!headers) return false;
		if (typeof headers === 'string') return headers.length > 0;
		return Object.keys(headers).length > 0;
	}
</script>

<div
	id="mcp-audit-log-details"
	class="bg-base-200 text-base-content flex h-full w-[inherit] min-w-[inherit] flex-col"
>
	<div class="dark:bg-base-300 bg-base-100 relative flex w-full flex-col p-4 pl-5 shadow-xs">
		<div
			class={twMerge(
				'absolute top-0 left-0 h-full w-1',
				(auditLog.responseStatus ?? 0) >= 400 ? 'bg-error' : 'bg-primary'
			)}
		></div>
		<h3 class="text-lg font-semibold">
			{formatLogTimestamp(auditLog.createdAt, userDeviceSettings.timeFormat)}
		</h3>
		<p class="text-muted-content text-xs font-light">
			{auditLog.requestID}
		</p>
		<IconButton
			id="mcp-audit-log-details-close-btn"
			onclick={onClose}
			class="absolute top-1/2 right-4 -translate-y-1/2"
		>
			<X class="size-5" />
		</IconButton>
	</div>
	<div class="default-scrollbar-thin relative h-[calc(100%-60px)] overflow-y-auto pb-4">
		<div class="bg-base-300 absolute top-0 left-0 h-full w-1"></div>

		{#if preRequestContent}
			{@render preRequestContent(auditLog)}
		{/if}

		<div class="px-5 flex flex-col gap-4">
			{#if shouldShowPayload}
				{#if loading?.request}
					<Loading />
				{:else}
					{#if hasBody(auditLog?.requestBody)}
						{@render jsonBody(m.audit_usage_audit_logs_request_body(), auditLog?.requestBody)}
					{:else if !hasAuditorAccess}
						{@render noAuditorAccessInfo()}
					{/if}

					{#if hasBody(auditLog?.policyModifiedRequestBody)}
						{@render jsonBody(
							m.audit_usage_audit_logs_policy_modified_request_body(),
							auditLog?.policyModifiedRequestBody
						)}
					{/if}

					{#if hasBody(auditLog?.mutatedRequestBody)}
						{@render jsonBody(
							m.audit_usage_audit_logs_mutated_request_body(),
							auditLog?.mutatedRequestBody
						)}
					{/if}
				{/if}
			{/if}

			{#if shouldShowPayload}
				{#if loading?.response}
					<Loading />
				{:else}
					{#if hasBody(auditLog?.originalResponseBody)}
						{@render jsonBody(
							m.audit_usage_audit_logs_original_response_body(),
							auditLog?.originalResponseBody
						)}
					{/if}

					{#if hasBody(auditLog?.responseBody)}
						{@render jsonBody(m.audit_usage_audit_logs_response_body(), auditLog?.responseBody)}
					{:else if !hasAuditorAccess}
						{@render noAuditorAccessInfo()}
					{/if}
				{/if}
			{/if}

			<div class="divider text-xs uppercase my-0">
				{m.audit_usage_audit_logs_additional_information()}
			</div>
			{#if hasHeaders(auditLog.requestHeaders)}
				{@render jsonBody(m.audit_usage_audit_logs_request_headers(), auditLog.requestHeaders)}
			{/if}

			{#if hasHeaders(auditLog.responseHeaders)}
				{@render jsonBody(m.audit_usage_audit_logs_response_headers(), auditLog.responseHeaders)}
			{/if}

			{#if !hasAuditorAccess}
				{@render noAuditorAccessInfo()}
			{/if}

			{#if shouldShowPayload}
				<div class="divider my-0"></div>
				<div class="flex flex-col gap-0.5">
					{@render title(m.audit_usage_audit_logs_http_request())}
					{#if auditLog.user || additRequestContent}
						<div class="flex flex-col gap-1 px-4 py-2 text-sm font-light">
							{#if auditLog.user}
								<p class="grid grid-cols-2 gap-2 break-all">
									<span class="font-medium">{m.audit_usage_audit_logs_user_label()}</span>
									{auditLog.user}
								</p>
							{/if}
							{#if additRequestContent}
								{@render additRequestContent(auditLog)}
							{/if}
						</div>
					{/if}
				</div>

				<div class="divider my-0"></div>

				<div class="flex items-center gap-2">
					<p class="text-base font-semibold">{m.audit_usage_audit_logs_http_response()}</p>
					{#if loading?.response}
						<div class="skeleton h-4 w-8 rounded-full"></div>
					{:else if auditLog?.responseStatus}
						<p
							class={twMerge(
								'w-fit rounded-full px-3 py-1 text-xs font-semibold text-white',
								auditLog.responseStatus >= 400 ? 'bg-error' : 'bg-primary'
							)}
						>
							{auditLog.responseStatus}
						</p>
					{/if}
				</div>
			{/if}

			{#if auditLog?.error}
				<div class="divider my-0"></div>
				<div class="flex flex-col gap-0.5">
					{@render title(m.audit_usage_audit_logs_response_error())}
					<p class="text-error text-sm">{auditLog.error}</p>
				</div>
			{/if}

			{#if shouldShowPayload}
				{#if auditLog?.webhookStatuses && auditLog.webhookStatuses.length > 0}
					<div class="divider my-0"></div>
					{@render jsonBody(m.audit_usage_audit_logs_webhook_statuses(), auditLog.webhookStatuses)}
				{/if}
			{/if}
		</div>
	</div>
</div>

{#snippet title(label: string)}
	<p class="text-base font-semibold mb-2">{label}</p>
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

{#snippet noAuditorAccessInfo()}
	<div class="bg-base-300 text-muted-content rounded-md p-3 text-xs italic">
		{m.audit_usage_audit_logs_hidden_for_access_level()}
	</div>
{/snippet}
