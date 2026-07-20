<script lang="ts">
	import CopyButton from '$lib/components/CopyButton.svelte';
	import IconButton from '$lib/components/primitives/IconButton.svelte';
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

		<div class="flex flex-wrap gap-2 p-4 pl-5">
			{@render chip('Event', auditLog.eventType)}
			{@render chip('Outcome', auditLog.outcome.status)}
			{@render chip('Operation', auditLog.action.operation)}
			{@render chip('Target', auditLog.target.targetType)}
		</div>

		<div class="p-4 pl-5">
			<h4 class="text-lg font-semibold">Event</h4>
			<div class="flex flex-col gap-1 px-4 py-2 text-sm font-light">
				{@render field('Actor', auditLog.user || auditLog.actor.id || 'Unknown')}
				{@render field('Actor Type', auditLog.actor.actorType)}
				{@render field('Credential', auditLog.actor.credentialID)}
				{@render field('Action', auditLog.action.name)}
				{@render field('Action Kind', auditLog.action.kind)}
				{@render field('Target', auditLog.target.name || auditLog.target.id)}
				{@render field('Parent Target', auditLog.target.parent?.name || auditLog.target.parent?.id)}
				{@render field('HTTP Status', auditLog.outcome.httpStatus)}
				{@render field('Reason', auditLog.outcome.reason)}
				{@render field('Duration (ms)', auditLog.outcome.durationMs)}
				{@render field(
					'Recorded At',
					formatLogTimestamp(auditLog.timestamp.recordedAt, userDeviceSettings.timeFormat)
				)}
				{@render field('Timestamp Source', auditLog.timestamp.source)}
			</div>

			{#if auditLog.outcome.error}
				<div class="mt-4 flex flex-col">
					<div class="mb-2 text-base font-semibold">Error</div>
					<p class="text-error">{auditLog.outcome.error}</p>
				</div>
			{/if}

			{#if details}
				<h4 class="mt-4 text-lg font-semibold">Trace & Network</h4>
				<div class="flex flex-col gap-1 px-4 py-2 text-sm font-light">
					{@render field('Session ID', details.trace?.sessionID)}
					{@render field('Request ID', details.trace?.requestID)}
					{@render field('Idempotency Key', details.trace?.idempotencyKey)}
					{@render field('Tool Use ID', details.trace?.toolUseID)}
					{@render field('Turn ID', details.trace?.turnID)}
					{@render field('Client IP', details.network?.clientIP)}
					{@render field(
						'Started At',
						details.startedAt
							? formatLogTimestamp(details.startedAt, userDeviceSettings.timeFormat)
							: undefined
					)}
				</div>

				{#if details.client || details.scope}
					<h4 class="mt-4 text-lg font-semibold">MCP Context</h4>
					<div class="flex flex-col gap-1 px-4 py-2 text-sm font-light">
						{@render field(
							'Client',
							[details.client?.name, details.client?.version].filter(Boolean).join(' / ')
						)}
						{@render field('User Agent', details.client?.userAgent)}
						{@render field('Workspace', details.scope?.powerUserWorkspaceID)}
						{@render field('Catalog Entry', details.scope?.mcpServerCatalogEntryName)}
					</div>
				{/if}

				{#if details.agent || details.device}
					<h4 class="mt-4 text-lg font-semibold">Agent & Device</h4>
					<div class="flex flex-col gap-1 px-4 py-2 text-sm font-light">
						{@render field(
							'Agent',
							[details.agent?.provider, details.agent?.version].filter(Boolean).join(' / ')
						)}
						{@render field(
							'CLI',
							[details.agent?.cliName, details.agent?.cliVersion].filter(Boolean).join(' / ')
						)}
						{@render field(
							'Model',
							[details.agent?.model, details.agent?.modelID].filter(Boolean).join(' / ')
						)}
						{@render field('Permission Mode', details.agent?.permissionMode)}
						{@render field('Device', details.device?.id)}
						{@render field('Deployment ID', details.device?.deploymentID)}
						{@render field(
							'OS / Architecture',
							[details.device?.os, details.device?.architecture].filter(Boolean).join(' / ')
						)}
					</div>
				{/if}

				{#if details.payloadRedacted}
					<div class="bg-base-300 text-muted-content mt-4 rounded-md p-3 text-xs italic">
						Payload and sensitive environment details are hidden for your access level.
					</div>
				{:else}
					{#if details.environment}
						<h4 class="mt-4 text-lg font-semibold">Environment</h4>
						<div class="flex flex-col gap-1 px-4 py-2 text-sm font-light">
							{@render field('Working Directory', details.environment.cwd)}
							{@render field('Git Root', details.environment.gitRoot)}
							{@render field('Git Branch', details.environment.gitBranch)}
							{@render field('Git Commit', details.environment.gitCommit)}
							{@render field('Git Remotes', details.environment.gitRemotes?.join(', '))}
							{@render field('Hostname', details.device?.hostname)}
							{@render field('Local Username', details.device?.localUsername)}
							{@render field('Reported Email', details.environment.reportedUserEmail)}
							{@render field('Transcript Path', details.environment.transcriptPath)}
						</div>
					{/if}

					{#if hasBody(details.request?.headers)}
						{@render headersBody('Request Headers', details.request?.headers)}
					{/if}
					{#if hasBody(details.request?.body)}
						{@render jsonBody('Request / Tool Input', details.request?.body)}
					{/if}
					{#if hasBody(details.request?.mutatedBody)}
						{@render jsonBody('Mutated Request Body', details.request?.mutatedBody)}
					{/if}
					{#if hasBody(details.response?.headers)}
						{@render headersBody('Response Headers', details.response?.headers)}
					{/if}
					{#if hasBody(details.response?.originalBody)}
						{@render jsonBody('Original Response Body', details.response?.originalBody)}
					{/if}
					{#if hasBody(details.response?.body)}
						{@render jsonBody('Response / Tool Output', details.response?.body)}
					{/if}
					{#if hasBody(details.rawEvent)}
						{@render jsonBody('Raw Event', details.rawEvent)}
					{/if}
				{/if}

				{#if details.webhookStatuses?.length}
					{@render jsonBody('Webhook Statuses', details.webhookStatuses)}
				{/if}
			{/if}
		</div>
	</div>
</div>

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
		<p><span class="font-medium">{label}</span>: {value}</p>
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
	<p class="translate-y-2 pt-4 text-base font-semibold">{name}</p>
	<div class="relative text-white">
		<pre class="default-scrollbar-thin max-h-96 overflow-y-auto p-4">{text}</pre>
		<CopyButton
			classes={{ button: 'absolute right-4 top-4 flex flex-col items-end text-current' }}
			{text}
		/>
	</div>
{/snippet}

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
