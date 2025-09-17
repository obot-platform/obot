<script lang="ts">
	import { columnResize } from '$lib/actions/resize';
	import CopyButton from '$lib/components/CopyButton.svelte';
	import type { AuditLog } from '$lib/services/admin/types';
	import { responsive } from '$lib/stores';
	import { X } from 'lucide-svelte';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		auditLog: AuditLog & {
			user: string;
		};
		onClose: () => void;
	}

	let { auditLog, onClose }: Props = $props();
	let container = $state<HTMLDivElement>();
</script>

{#if !responsive.isMobile && container}
	<div
		role="none"
		class="absolute z-30 h-full w-3 cursor-col-resize"
		use:columnResize={{ column: container, direction: 'right' }}
	></div>
{/if}
<div
	class="dark:bg-gray-990 h-full w-screen max-w-full bg-gray-50 md:w-lg md:max-w-[85vw] md:min-w-lg"
	bind:this={container}
>
	<div class="dark:bg-surface1 relative flex w-full flex-col bg-white p-4 pl-5 shadow-xs">
		<div
			class={twMerge(
				'absolute top-0 left-0 h-full w-1',
				auditLog.responseStatus >= 400 ? 'bg-red-500' : 'bg-blue-500'
			)}
		></div>
		<h3 class="text-lg font-semibold">
			{new Date(auditLog.createdAt)
				.toLocaleString(undefined, {
					year: 'numeric',
					month: 'short',
					day: 'numeric',
					hour: '2-digit',
					minute: '2-digit',
					second: '2-digit',
					hour12: true,
					timeZoneName: 'short'
				})
				.replace(/,/g, '')}
		</h3>
		<p class="text-xs font-light text-gray-400">
			{auditLog.requestID}
		</p>
		<button onclick={onClose} class="icon-button absolute top-1/2 right-4 -translate-y-1/2">
			<X class="size-5" />
		</button>
	</div>
	<div class="default-scrollbar-thin relative h-[calc(100%-60px)] overflow-y-auto pb-4">
		<div class="bg-surface2 absolute top-0 left-0 h-full w-1"></div>

		<div class="flex flex-wrap gap-2 p-4 pl-5">
			{#if auditLog.callType}
				<div class="dark:bg-surface3 bg-surface2 rounded-full px-3 py-1 text-[11px] font-light">
					<span class="font-medium">Call Type:</span>
					{auditLog.callType}
				</div>
			{/if}
			{#if auditLog.sessionID}
				<div class="dark:bg-surface3 bg-surface2 rounded-full px-3 py-1 text-[11px] font-light">
					<span class="font-medium">Session ID:</span>
					{auditLog.sessionID}
				</div>
			{/if}
			{#if auditLog.mcpID}
				<div class="dark:bg-surface3 bg-surface2 rounded-full px-3 py-1 text-[11px] font-light">
					<span class="font-medium">Server:</span>
					{auditLog.mcpServerDisplayName} ({auditLog.mcpID})
				</div>
			{/if}
			{#if auditLog.mcpServerCatalogEntryName}
				<div class="dark:bg-surface3 bg-surface2 rounded-full px-3 py-1 text-[11px] font-light">
					<span class="font-medium">Parent Entry ID:</span>
					{auditLog.mcpServerCatalogEntryName}
				</div>
			{/if}
		</div>

		<div class="p-4 pl-5">
			<h4 class="text-lg font-semibold">HTTP Request</h4>
			<div class="flex flex-col gap-1 px-4 py-2 text-sm font-light">
				{#if auditLog.user}
					<p><span class="font-medium">User</span>: {auditLog.user}</p>
				{/if}
				{#if auditLog.userAgent}
					<p><span class="font-medium">User Agent</span>: {auditLog.userAgent}</p>
				{/if}
				{#if auditLog.client}
					<p>
						<span class="font-medium">Client</span>: {auditLog.client.name}/{auditLog.client
							.version}
					</p>
				{/if}
				{#if auditLog.clientIP}
					<p><span class="font-medium">Client IP</span>: {auditLog.clientIP}</p>
				{/if}
			</div>

			{#if auditLog.requestHeaders}
				<p class="my-2 text-base font-semibold">Request Headers</p>

				<div
					class="dark:bg-surface2 relative flex flex-col gap-2 overflow-hidden rounded-md bg-white p-4 pl-5"
				>
					<div class="absolute top-0 left-0 h-full w-1 bg-blue-800"></div>
					<div class="flex flex-col gap-1">
						{#each Object.entries(auditLog.requestHeaders ?? {}) as [key, value] (key)}
							<p>
								<span class="font-medium">{key}</span>: {value}
							</p>
						{/each}
					</div>
				</div>
			{/if}

			{#if Object.keys(auditLog.requestBody ?? {}).length > 0}
				{@const body = JSON.stringify(auditLog.requestBody, null, 2)}

				<p class="translate-y-2 pt-4 text-base font-semibold">Request Body</p>
				<div class="relative text-white">
					<pre class="default-scrollbar-thin max-h-96 overflow-y-auto p-4">
						<code class="language-json">{body}</code>
					</pre>

					<CopyButton
						classes={{ button: 'absolute right-4 top-4 flex flex-col items-end' }}
						text={body}
					/>
				</div>
			{/if}
		</div>

		<div class="p-4 pl-5">
			<div class="flex items-center gap-2">
				<h4 class="text-lg font-semibold">HTTP Response</h4>
				{#if auditLog.responseStatus}
					<p
						class={twMerge(
							'w-fit rounded-full px-3 py-1 text-xs font-semibold text-white',
							auditLog.responseStatus >= 400 ? 'bg-red-500' : 'bg-blue-500'
						)}
					>
						{auditLog.responseStatus}
					</p>
				{/if}
			</div>

			{#if auditLog.responseHeaders}
				<p class="mt-4 mb-2 text-base font-semibold">Response Headers</p>
				<div
					class="dark:bg-surface2 relative flex flex-col gap-2 overflow-hidden rounded-md bg-white p-4 pl-5"
				>
					<div class="absolute top-0 left-0 h-full w-1 bg-blue-800"></div>
					<div class="flex flex-col gap-1">
						{#each Object.entries(auditLog.responseHeaders ?? {}) as [key, value] (key)}
							<p>
								<span class="font-medium">{key}</span>: {value}
							</p>
						{/each}
					</div>
				</div>
			{/if}

			{#if auditLog.error}
				<div class="mt-4 flex flex-col">
					<div class="mb-2 text-base font-semibold">Response Error</div>
					<p class="text-red-500">{auditLog.error}</p>
				</div>
			{/if}

			{#if Object.keys(auditLog.responseBody ?? {}).length > 0}
				{@const body = JSON.stringify(auditLog.responseBody, null, 2)}

				<p class="translate-y-2 pt-4 text-base font-semibold">Response Body</p>
				<div class="relative text-white">
					<pre class="default-scrollbar-thin max-h-96 overflow-y-auto p-4">
						<code class="language-json">{body}</code>
					</pre>

					<CopyButton
						classes={{ button: 'absolute right-4 top-4 flex flex-col items-end text-current' }}
						text={body}
					/>
				</div>
			{/if}
		</div>
	</div>
</div>
