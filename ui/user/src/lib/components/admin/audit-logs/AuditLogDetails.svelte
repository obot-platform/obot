<script lang="ts" generics="T extends object">
	import CopyButton from '$lib/components/CopyButton.svelte';
	import IconButton from '$lib/components/primitives/IconButton.svelte';
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

	function formatHeaderValue(value: string | string[]) {
		return Array.isArray(value) ? value.join(', ') : value;
	}
</script>

<div class="bg-base-200 text-base-content flex h-full w-[inherit] min-w-[inherit] flex-col">
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
		<IconButton onclick={onClose} class="absolute top-1/2 right-4 -translate-y-1/2">
			<X class="size-5" />
		</IconButton>
	</div>
	<div class="default-scrollbar-thin relative h-[calc(100%-60px)] overflow-y-auto pb-4">
		<div class="bg-base-300 absolute top-0 left-0 h-full w-1"></div>

		{#if preRequestContent}
			{@render preRequestContent(auditLog)}
		{/if}

		<div class="p-4 pl-5">
			<h4 class="text-lg font-semibold">HTTP Request</h4>
			{#if auditLog.user || additRequestContent}
				<div class="flex flex-col gap-1 px-4 py-2 text-sm font-light">
					{#if auditLog.user}
						<p><span class="font-medium">User</span>: {auditLog.user}</p>
					{/if}
					{#if additRequestContent}
						{@render additRequestContent(auditLog)}
					{/if}
				</div>
			{/if}

			{#if shouldShowPayload}
				{#if hasHeaders(auditLog.requestHeaders)}
					<p class="my-2 text-base font-semibold">Request Headers</p>

					<div
						class="dark:bg-base-300 bg-base-100 relative flex flex-col gap-2 overflow-hidden rounded-md p-4 pl-5"
					>
						<div class="bg-primary/50 absolute top-0 left-0 h-full w-1"></div>
						{#if typeof auditLog.requestHeaders === 'string'}
							<pre class="whitespace-pre-wrap break-all">{auditLog.requestHeaders}</pre>
						{:else}
							<div class="flex flex-col gap-1">
								{#each Object.entries(auditLog.requestHeaders ?? {}) as [key, value] (key)}
									<p>
										<span class="font-medium">{key}</span>: {formatHeaderValue(value)}
									</p>
								{/each}
							</div>
						{/if}
					</div>
				{:else if !hasAuditorAccess}
					{@render noAuditorAccessInfo('Request Headers')}
				{/if}
			{/if}

			{#if shouldShowPayload}
				{#if loading?.request}
					<Loading />
				{:else}
					{#if hasBody(auditLog?.requestBody)}
						{@render jsonBody('Request Body', auditLog?.requestBody)}
					{:else if !hasAuditorAccess}
						{@render noAuditorAccessInfo('Request Body')}
					{/if}

					{#if hasBody(auditLog?.mutatedRequestBody)}
						{@render jsonBody('Mutated Request Body', auditLog?.mutatedRequestBody)}
					{/if}
				{/if}
			{/if}
		</div>

		<div class="p-4 pl-5">
			<div class="flex items-center gap-2">
				<h4 class="text-lg font-semibold">HTTP Response</h4>
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

			{#if shouldShowPayload}
				{#if hasHeaders(auditLog.responseHeaders)}
					<p class="mt-4 mb-2 text-base font-semibold">Response Headers</p>
					<div
						class="dark:bg-base-300 bg-base-100 relative flex flex-col gap-2 overflow-hidden rounded-md p-4 pl-5"
					>
						<div class="bg-primary/50 absolute top-0 left-0 h-full w-1"></div>
						{#if typeof auditLog.responseHeaders === 'string'}
							<pre class="whitespace-pre-wrap break-all">{auditLog.responseHeaders}</pre>
						{:else}
							<div class="flex flex-col gap-1">
								{#each Object.entries(auditLog.responseHeaders ?? {}) as [key, value] (key)}
									<p>
										<span class="font-medium">{key}</span>: {formatHeaderValue(value)}
									</p>
								{/each}
							</div>
						{/if}
					</div>
				{:else if !hasAuditorAccess}
					{@render noAuditorAccessInfo('Response Headers')}
				{/if}
			{/if}

			{#if auditLog?.error}
				<div class="mt-4 flex flex-col">
					<div class="mb-2 text-base font-semibold">Response Error</div>
					<p class="text-error">{auditLog.error}</p>
				</div>
			{/if}

			{#if shouldShowPayload}
				{#if loading?.response}
					<Loading />
				{:else}
					{#if hasBody(auditLog?.originalResponseBody)}
						{@render jsonBody('Original Response Body', auditLog?.originalResponseBody)}
					{/if}

					{#if hasBody(auditLog?.responseBody)}
						{@render jsonBody('Response Body', auditLog?.responseBody)}
					{:else if !hasAuditorAccess}
						{@render noAuditorAccessInfo('Response Body')}
					{/if}
				{/if}
			{/if}

			{#if shouldShowPayload}
				{#if auditLog?.webhookStatuses && auditLog.webhookStatuses.length > 0}
					{@const statuses = JSON.stringify(auditLog.webhookStatuses, null, 2)}

					<p class="translate-y-2 pt-4 text-base font-semibold">Webhook Statuses</p>
					<div class="relative text-white">
						<pre class="default-scrollbar-thin max-h-96 overflow-y-auto p-4"><code
								class="language-json">{statuses}</code
							></pre>

						<CopyButton
							classes={{ button: 'absolute right-4 top-4 flex flex-col items-end text-current' }}
							text={statuses}
						/>
					</div>
				{/if}
			{/if}
		</div>
	</div>
</div>

{#snippet jsonBody(name: string, value: unknown)}
	{@const body = JSON.stringify(value, null, 2)}

	<p class="translate-y-2 pt-4 text-base font-semibold">{name}</p>
	<div class="relative text-white">
		<pre class="default-scrollbar-thin max-h-96 overflow-y-auto p-4"><code class="language-json"
				>{body}</code
			></pre>

		<CopyButton
			classes={{ button: 'absolute right-4 top-4 flex flex-col items-end text-current' }}
			text={body}
		/>
	</div>
{/snippet}

{#snippet noAuditorAccessInfo(name: string)}
	<p class="mt-4 mb-2 text-base font-semibold">{name}</p>
	<div class="text-muted-content text-xs">
		<i>Details are hidden; auditor role is required to access this information.</i>
	</div>
{/snippet}
