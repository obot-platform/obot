<script lang="ts">
	import Layout from '$lib/components/Layout.svelte';
	import Skeleton from '$lib/components/Skeleton.svelte';
	import RequestStateBadge from '$lib/components/mcp-server-requests/RequestStateBadge.svelte';
	import { parseErrorContent } from '$lib/errors';
	import { m } from '$lib/i18n';
	import { getVMCP, listVMCPs, type VMCP } from '$lib/services';
	import {
		approveMCPServerRequest,
		denyMCPServerRequest,
		getMCPServerRequest,
		requestTarget,
		type MCPServerRequest
	} from '$lib/services/mcpserverrequests';
	import { profile } from '$lib/stores';
	import { formatTimeAgo } from '$lib/time';
	import { goto } from '$lib/url';
	import type { PageProps } from './$types';
	import { untrack } from 'svelte';

	// One request. An administrator, often arriving from the Slack alert, approves it into an
	// existing or a new vMCP, or denies it.

	let { data }: PageProps = $props();

	let isAdmin = $derived(!!profile.current.isAdmin?.());
	let request = $state<MCPServerRequest>();
	let sharedVMCPs = $state<VMCP[]>([]);
	let loading = $state(true);
	let error = $state('');
	let busy = $state(false);

	let destination = $state<'existing' | 'new'>('existing');
	let vmcpID = $state('');
	let newName = $state('');
	let newDescription = $state('');
	let note = $state('');
	let denyReason = $state('');
	let grantRequester = $state(true);
	let vmcpName = $state('');

	async function loadVMCPName(id?: string) {
		vmcpName = '';
		if (!id) return;
		try {
			vmcpName = (await getVMCP(id, { dontLogErrors: true })).displayName ?? '';
		} catch {
			// The requester may not be able to read the vMCP; its ID stands in.
		}
	}

	async function load(id: string) {
		loading = true;
		error = '';
		try {
			const loaded = await getMCPServerRequest(id, { dontLogErrors: true });
			request = loaded;
			newName = loaded.name;
			loadVMCPName(loaded.vmcpID);
			if (untrack(() => isAdmin) && loaded.state === 'pending') {
				const vmcps = await listVMCPs({ dontLogErrors: true });
				sharedVMCPs = vmcps.filter((v) => !v.userID && !v.sourceURL);
				const suggested = sharedVMCPs.find((v) => v.id === loaded.suggestedVMCPID);
				vmcpID = suggested?.id ?? sharedVMCPs[0]?.id ?? '';
				destination = sharedVMCPs.length > 0 ? 'existing' : 'new';
			}
		} catch (e) {
			error = parseErrorContent(e).message;
		} finally {
			loading = false;
		}
	}

	$effect(() => {
		load(data.id);
	});

	let canApprove = $derived(destination === 'existing' ? vmcpID !== '' : newName.trim() !== '');

	async function approve() {
		if (!request || !canApprove) return;
		busy = true;
		error = '';
		try {
			const decided = await approveMCPServerRequest(request.id, {
				...(destination === 'existing'
					? { vmcpID, grantRequesterAccess: grantRequester }
					: { newVMCP: { displayName: newName.trim(), description: newDescription.trim() } }),
				reason: note.trim() || undefined
			});
			vmcpName =
				destination === 'new'
					? newName.trim()
					: (sharedVMCPs.find((v) => v.id === decided.vmcpID)?.displayName ?? '');
			request = decided;
		} catch (e) {
			error = parseErrorContent(e).message;
		} finally {
			busy = false;
		}
	}

	async function deny() {
		if (!request) return;
		busy = true;
		error = '';
		try {
			request = await denyMCPServerRequest(request.id, denyReason.trim());
		} catch (e) {
			error = parseErrorContent(e).message;
		} finally {
			busy = false;
		}
	}

	function stateLabel(r: MCPServerRequest) {
		return r.state === 'approved'
			? m.server_requests_state_approved()
			: m.server_requests_state_denied();
	}
</script>

<svelte:head>
	<title>Obot | {request?.name ?? m.server_requests_title()}</title>
</svelte:head>

<Layout
	title={request?.name ?? m.server_requests_title()}
	showBackButton
	onBackButtonClick={() => goto('/mcp-server-requests')}
>
	<div class="flex flex-col gap-6 pb-8">
		{#if error}
			<div class="notification-error rounded-md p-3 text-sm" role="alert">{error}</div>
		{/if}

		{#if loading && !request}
			<Skeleton type="card" class="min-h-48 w-full" />
		{:else if request}
			<section
				class="dark:bg-base-200 bg-base-100 flex flex-col gap-4 rounded-lg p-4 shadow-sm"
				aria-label={request.name}
			>
				<div class="flex flex-wrap items-center justify-between gap-2">
					<p class="text-muted-content text-sm">
						{m.server_requests_requested_by({
							name: request.requesterName || request.requesterEmail || request.requesterID
						})} ·
						{m.server_requests_requested_at({ time: formatTimeAgo(request.created).relativeTime })}
					</p>
					<RequestStateBadge state={request.state} />
				</div>
				{#if request.description}
					<p class="text-sm">{request.description}</p>
				{/if}
				<dl class="grid gap-x-4 gap-y-3 text-sm sm:grid-cols-[max-content_1fr]">
					{#if request.existingCatalogEntryID}
						<dt class="font-medium">{m.server_requests_catalog_entry()}</dt>
						<dd><code class="break-all">{request.existingCatalogEntryID}</code></dd>
					{:else}
						<dt class="font-medium">{m.server_requests_runs_as()}</dt>
						<dd><code class="break-all">{requestTarget(request)}</code></dd>
					{/if}
					{#if request.reason}
						<dt class="font-medium">{m.server_requests_reason()}</dt>
						<dd class="whitespace-pre-wrap">{request.reason}</dd>
					{/if}
					{#if request.sourceURL}
						<dt class="font-medium">{m.server_requests_source()}</dt>
						<dd>
							<a
								class="text-link break-all"
								href={request.sourceURL}
								target="_blank"
								rel="noopener noreferrer">{request.sourceURL}</a
							>
						</dd>
					{/if}
					{#if request.config?.length}
						<dt class="font-medium">{m.server_requests_config()}</dt>
						<dd>
							<ul class="flex flex-col gap-1">
								{#each request.config as c (c.key)}
									<li>
										<code>{c.key}</code>
										{#if c.sensitive}<span class="text-muted-content text-xs">
												({m.server_requests_sensitive()})</span
											>{/if}
										{#if c.description}<span class="text-muted-content">
												— {c.description}</span
											>{/if}
									</li>
								{/each}
							</ul>
						</dd>
					{/if}
				</dl>
			</section>

			{#if request.state === 'approved'}
				<section class="notification-info flex flex-col gap-2 rounded-md p-4 text-sm">
					<p>
						{m.server_requests_approved_result({ vmcp: vmcpName || request.vmcpID || '' })}
						{m.server_requests_decided({
							state: stateLabel(request),
							time: formatTimeAgo(request.decidedAt).relativeTime
						})}
					</p>
					{#if request.decisionReason}<p class="whitespace-pre-wrap">
							{request.decisionReason}
						</p>{/if}
					{#if request.vmcpID}
						<a class="text-link self-start" href="/vmcps/{request.vmcpID}"
							>{m.server_requests_open_vmcp()}</a
						>
					{/if}
				</section>
				{#if isAdmin && request.requesterCanConnect === false}
					<p class="notification-alert rounded-md p-3 text-sm" role="status">
						{m.server_requests_requester_no_access()}
					</p>
				{/if}
			{:else if request.state === 'denied'}
				<section class="notification-error flex flex-col gap-1 rounded-md p-4 text-sm">
					<p>
						{m.server_requests_decided({
							state: stateLabel(request),
							time: formatTimeAgo(request.decidedAt).relativeTime
						})}
					</p>
					{#if request.decisionReason}<p class="whitespace-pre-wrap">
							{request.decisionReason}
						</p>{/if}
				</section>
			{:else if isAdmin}
				<section
					class="dark:bg-base-200 bg-base-100 flex flex-col gap-4 rounded-lg p-4 shadow-sm"
					aria-labelledby="decide-title"
				>
					<h2 id="decide-title" class="font-semibold">{m.server_requests_decide_title()}</h2>

					<fieldset class="flex flex-col gap-3">
						<legend class="mb-2 text-sm font-medium">{m.server_requests_destination()}</legend>
						<label class="flex items-center gap-2 text-sm">
							<input
								type="radio"
								value="existing"
								bind:group={destination}
								disabled={sharedVMCPs.length === 0}
							/>
							{m.server_requests_existing_vmcp()}
						</label>
						{#if destination === 'existing'}
							<label class="ml-6 flex flex-col gap-1 text-sm">
								{m.server_requests_choose_vmcp()}
								<select class="text-input-filled" bind:value={vmcpID}>
									{#each sharedVMCPs as vmcp (vmcp.id)}
										<option value={vmcp.id}>
											{vmcp.displayName || vmcp.id}{vmcp.id === request.suggestedVMCPID
												? ` (${m.server_requests_suggested_vmcp()})`
												: ''}
										</option>
									{/each}
								</select>
							</label>
							<label class="ml-6 flex items-center gap-2 text-sm">
								<input type="checkbox" bind:checked={grantRequester} />
								{m.server_requests_grant_requester()}
							</label>
						{/if}
						{#if sharedVMCPs.length === 0}
							<p class="text-muted-content ml-6 text-xs">{m.server_requests_no_shared_vmcps()}</p>
						{/if}
						<label class="flex items-center gap-2 text-sm">
							<input type="radio" value="new" bind:group={destination} />
							{m.server_requests_new_vmcp()}
						</label>
						{#if destination === 'new'}
							<div class="ml-6 flex flex-col gap-3">
								<label class="flex flex-col gap-1 text-sm">
									{m.server_requests_new_vmcp_name()}
									<input class="text-input-filled" bind:value={newName} />
								</label>
								<label class="flex flex-col gap-1 text-sm">
									{m.server_requests_new_vmcp_description()}
									<input class="text-input-filled" bind:value={newDescription} />
								</label>
								<p class="text-muted-content text-xs">{m.server_requests_new_vmcp_access()}</p>
							</div>
						{/if}
					</fieldset>

					<label class="flex flex-col gap-1 text-sm">
						{m.server_requests_note()}
						<textarea class="text-input-filled min-h-16" bind:value={note}></textarea>
					</label>

					<div class="flex justify-end">
						<button
							type="button"
							class="btn btn-primary"
							disabled={busy || !canApprove}
							onclick={approve}
						>
							{m.server_requests_approve()}
						</button>
					</div>

					<hr class="border-base-300" />

					<label class="flex flex-col gap-1 text-sm">
						{m.server_requests_deny_reason()}
						<textarea class="text-input-filled min-h-16" bind:value={denyReason}></textarea>
					</label>
					<div class="flex justify-end">
						<button type="button" class="btn btn-secondary" disabled={busy} onclick={deny}>
							{m.server_requests_deny()}
						</button>
					</div>
				</section>
			{/if}
		{/if}
	</div>
</Layout>
