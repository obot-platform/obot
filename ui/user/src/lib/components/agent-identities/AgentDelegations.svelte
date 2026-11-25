<script lang="ts">
	import Confirm from '$lib/components/Confirm.svelte';
	import Table from '$lib/components/table/Table.svelte';
	import { m } from '$lib/i18n';
	import Loading from '$lib/icons/Loading.svelte';
	import { AgentIdentitiesService } from '$lib/services';
	import { delegationTimes } from '$lib/services/agent-identities/helpers';
	import type { AgentDelegation } from '$lib/services/agent-identities/types';
	import { untrack } from 'svelte';

	interface Props {
		agentId: string;
		agentName: string;
		readonly?: boolean;
	}

	let { agentId, agentName, readonly }: Props = $props();

	let delegations = $state<AgentDelegation[]>([]);
	let loading = $state(true);
	let failed = $state(false);
	let revoking = $state<AgentDelegation>();
	let busy = $state(false);

	const userName = (d: AgentDelegation) => d.user.displayName || d.user.email || d.user.id;

	const rows = $derived(
		delegations.map((d) => ({
			id: d.id,
			user: userName(d),
			...delegationTimes(d)
		}))
	);

	async function load() {
		try {
			delegations = await AgentIdentitiesService.listAgentDelegations(agentId);
			failed = false;
		} catch {
			failed = true;
		} finally {
			loading = false;
		}
	}

	$effect(() => {
		void agentId;
		untrack(load);
	});

	async function revoke() {
		const target = revoking;
		if (!target) return;
		busy = true;
		try {
			await AgentIdentitiesService.revokeAgentDelegation(agentId, target.id);
			delegations = delegations.filter((d) => d.id !== target.id);
		} catch {
			// The error is shown by the notifications.
		} finally {
			busy = false;
			revoking = undefined;
		}
	}
</script>

<section class="paper flex flex-col gap-3 p-4" aria-labelledby="agent-delegations-title">
	<div class="flex flex-col gap-1">
		<h2 id="agent-delegations-title" class="text-lg font-semibold">
			{m.identity_access_delegations_tab()}
		</h2>
		<p class="text-muted-content text-sm font-light">
			{m.identity_access_delegations_description()}
		</p>
	</div>

	{#if loading}
		<div class="flex justify-center py-4"><Loading class="size-6" /></div>
	{:else if failed}
		<p class="text-error text-sm">{m.identity_access_delegations_failed()}</p>
	{:else if rows.length === 0}
		<p class="text-muted-content py-4 text-center text-sm">
			{m.identity_access_delegations_none()}
		</p>
	{:else}
		<Table
			data={rows}
			fields={['user', 'firstApprovedAt', 'lastApprovedAt', 'lastUsedAt']}
			headers={[
				{ title: m.identity_access_delegations_col_user(), property: 'user' },
				{ title: m.identity_access_delegations_col_first_approved(), property: 'firstApprovedAt' },
				{ title: m.identity_access_delegations_col_last_approved(), property: 'lastApprovedAt' },
				{ title: m.identity_access_delegations_col_last_used(), property: 'lastUsedAt' }
			]}
		>
			{#snippet actions(d)}
				{#if !readonly}
					<button
						class="btn btn-error btn-soft btn-sm"
						disabled={busy}
						onclick={() => (revoking = delegations.find((delegation) => delegation.id === d.id))}
					>
						{m.identity_access_agents_revoke()}
					</button>
				{/if}
			{/snippet}
		</Table>
	{/if}
</section>

<Confirm
	msg={m.identity_access_delegations_confirm_revoke({
		user: revoking ? userName(revoking) : '',
		agent: agentName
	})}
	note={m.identity_access_delegations_confirm_revoke_note()}
	submitText={m.identity_access_agents_revoke()}
	show={Boolean(revoking)}
	loading={busy}
	onsuccess={revoke}
	oncancel={() => (revoking = undefined)}
/>
