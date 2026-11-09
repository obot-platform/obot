<script lang="ts">
	import ApprovalCard from '$lib/components/supervision/ApprovalCard.svelte';
	import { parseErrorContent } from '$lib/errors';
	import { m } from '$lib/i18n';
	import { listVMCPs, type VMCP } from '$lib/services';
	import { listMCPServerRequests, type MCPServerRequest } from '$lib/services/mcpserverrequests';
	import { listApprovals, type Approval } from '$lib/services/provenance';
	import { profile } from '$lib/stores';
	import { resolve } from '$lib/url';
	import ServerRequestCard from './ServerRequestCard.svelte';
	import WidgetFrame from './WidgetFrame.svelte';
	import { CircleCheck } from '@lucide/svelte';
	import { onMount } from 'svelte';

	// Everything waiting on the viewer's decision, decided right here: actions the Supervisor is
	// holding for approval and users' requests for MCP servers. The viewer clicks; Obot AI never
	// decides these on its own.

	const MAX = 10;

	let actions = $state<Approval[]>([]);
	let requests = $state<MCPServerRequest[]>([]);
	let vmcps = $state<VMCP[]>([]);
	let error = $state('');
	let loading = $state(true);

	onMount(async () => {
		if (!profile.current.isAdmin?.()) {
			error = m.ai_widget_not_found();
			loading = false;
			return;
		}
		const [held, asked, shared] = await Promise.allSettled([
			listApprovals({ status: 'pending', limit: MAX }, { dontLogErrors: true }),
			listMCPServerRequests({ state: 'pending' }, { dontLogErrors: true }),
			listVMCPs({ dontLogErrors: true })
		]);
		if (held.status === 'fulfilled') actions = held.value.items ?? [];
		if (asked.status === 'fulfilled') requests = asked.value.slice(0, MAX);
		if (shared.status === 'fulfilled')
			vmcps = shared.value.filter((v) => !v.userID && !v.sourceURL);
		if (held.status === 'rejected' && asked.status === 'rejected')
			error = parseErrorContent(held.reason).message;
		loading = false;
	});

	let empty = $derived(actions.length === 0 && requests.length === 0);
</script>

<WidgetFrame {loading} {error}>
	{#if empty}
		<p class="text-muted-content flex items-center gap-2 text-sm">
			<CircleCheck class="size-4 text-green-600" />
			{m.ai_widget_approvals_none()}
		</p>
	{/if}

	{#if actions.length}
		<section class="flex flex-col gap-2" aria-label={m.ai_widget_approvals_actions()}>
			<h4 class="text-muted-content text-xs font-semibold uppercase">
				{m.ai_widget_approvals_actions()} ({actions.length})
			</h4>
			{#each actions as approval, i (approval.id)}
				<ApprovalCard {approval} compact onResolved={(a) => (actions[i] = a)} />
			{/each}
			<a href={resolve('/approvals')} class="text-link self-start text-xs">
				{m.ai_widget_approvals_all()}
			</a>
		</section>
	{/if}

	{#if requests.length}
		<section class="flex flex-col gap-2" aria-label={m.ai_widget_approvals_requests()}>
			<h4 class="text-muted-content text-xs font-semibold uppercase">
				{m.ai_widget_approvals_requests()} ({requests.length})
			</h4>
			<ul class="divide-base-300 dark:divide-base-400 flex flex-col divide-y">
				{#each requests as request (request.id)}
					<li class="py-2"><ServerRequestCard {request} {vmcps} /></li>
				{/each}
			</ul>
		</section>
	{/if}
</WidgetFrame>
