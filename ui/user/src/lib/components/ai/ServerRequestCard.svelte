<script lang="ts">
	import RequestStateBadge from '$lib/components/mcp-server-requests/RequestStateBadge.svelte';
	import { parseErrorContent } from '$lib/errors';
	import { m } from '$lib/i18n';
	import type { VMCP } from '$lib/services';
	import {
		approveMCPServerRequest,
		denyMCPServerRequest,
		requestTarget,
		type MCPServerRequest
	} from '$lib/services/mcpserverrequests';
	import { profile } from '$lib/stores';
	import { formatTimeAgo } from '$lib/time';
	import { resolve } from '$lib/url';
	import { Check, ExternalLink, X } from '@lucide/svelte';

	// An MCP server request: what was asked for, by whom, and where it stands. Administrators can
	// decide a pending one here, into one of the shared vMCPs; anything more goes to the review page.

	interface Props {
		request: MCPServerRequest;
		/** Shared vMCPs an approval can add the server to. Without them the card can only deny. */
		vmcps?: VMCP[];
	}

	let { request: initial, vmcps = [] }: Props = $props();

	// Follows the prop, and is replaced by the decided request once the viewer decides.
	let request = $derived(initial);
	let vmcpID = $derived(
		vmcps.find((v) => v.id === initial.suggestedVMCPID)?.id ?? vmcps[0]?.id ?? ''
	);
	let busy = $state<'approve' | 'deny'>();
	let error = $state('');

	let canDecide = $derived(
		request.state === 'pending' &&
			!!profile.current.isAdmin?.() &&
			!profile.current.isAdminReadonly?.()
	);
	let vmcpName = $derived(vmcps.find((v) => v.id === request.vmcpID)?.displayName);

	async function decide(kind: 'approve' | 'deny') {
		busy = kind;
		error = '';
		try {
			request =
				kind === 'approve'
					? await approveMCPServerRequest(
							request.id,
							{ vmcpID, grantRequesterAccess: true },
							{ dontLogErrors: true }
						)
					: await denyMCPServerRequest(request.id, '', { dontLogErrors: true });
		} catch (e) {
			error = parseErrorContent(e).message;
		} finally {
			busy = undefined;
		}
	}
</script>

<div class="flex flex-col gap-2" aria-label={request.name}>
	<div class="flex flex-wrap items-start justify-between gap-2">
		<div class="flex min-w-0 flex-col gap-1">
			<div class="flex flex-wrap items-center gap-2">
				<span class="font-medium">{request.name}</span>
				<RequestStateBadge state={request.state} />
			</div>
			<span class="text-muted-content text-xs break-all">{requestTarget(request)}</span>
			<span class="text-muted-content text-xs">
				{m.ai_widget_requested_by({
					name: request.requesterEmail || request.requesterName || request.requesterID
				})} · {formatTimeAgo(request.created).relativeTime}
			</span>
			{#if request.reason}
				<p class="text-sm">{request.reason}</p>
			{/if}
			{#if request.state === 'approved' && vmcpName}
				<p class="text-xs text-success">
					{m.ai_widget_request_added({ vmcp: vmcpName })}
				</p>
			{/if}
			{#if request.decisionReason}
				<p class="text-muted-content text-xs italic">{request.decisionReason}</p>
			{/if}
		</div>
		<a
			href={resolve(`/mcp-server-requests/${request.id}`)}
			class="btn btn-secondary flex items-center gap-1 rounded-4xl px-3 py-1 text-xs"
		>
			{m.ai_widget_open_request()}
			<ExternalLink class="size-3" />
		</a>
	</div>

	{#if canDecide}
		<div class="flex flex-wrap items-center gap-2">
			{#if vmcps.length}
				<label class="flex items-center gap-2 text-xs">
					{m.ai_widget_approve_into()}
					<select
						class="bg-base-100 dark:bg-base-200 rounded-md border px-2 py-1 text-xs"
						bind:value={vmcpID}
						disabled={!!busy}
					>
						{#each vmcps as vmcp (vmcp.id)}
							<option value={vmcp.id}>{vmcp.displayName || vmcp.id}</option>
						{/each}
					</select>
				</label>
				<button
					type="button"
					class="btn btn-primary flex items-center gap-1 rounded-4xl px-3 py-1 text-xs"
					disabled={!!busy || !vmcpID}
					onclick={() => decide('approve')}
				>
					<Check class="size-3" />
					{m.ai_widget_approve()}
				</button>
			{/if}
			<button
				type="button"
				class="btn btn-secondary flex items-center gap-1 rounded-4xl px-3 py-1 text-xs"
				disabled={!!busy}
				onclick={() => decide('deny')}
			>
				<X class="size-3" />
				{m.ai_widget_deny()}
			</button>
		</div>
	{/if}
	{#if error}
		<p class="text-xs text-red-600" role="alert">{error}</p>
	{/if}
</div>
