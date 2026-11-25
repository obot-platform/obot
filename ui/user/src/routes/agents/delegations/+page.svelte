<script lang="ts">
	import Confirm from '$lib/components/Confirm.svelte';
	import Layout from '$lib/components/Layout.svelte';
	import Table from '$lib/components/table/Table.svelte';
	import { m } from '$lib/i18n';
	import { AgentIdentitiesService } from '$lib/services';
	import { delegationTimes } from '$lib/services/agent-identities/helpers';
	import type { AgentDelegation } from '$lib/services/agent-identities/types';
	import { resolve } from '$lib/url';
	import { openUrl } from '$lib/utils';
	import type { PageProps } from './$types';
	import { untrack } from 'svelte';

	let { data }: PageProps = $props();

	let delegations = $state<AgentDelegation[]>(untrack(() => data.delegations ?? []));
	let revoking = $state<AgentDelegation>();
	let busy = $state(false);

	const rows = $derived(
		delegations.map((d) => ({
			id: d.id,
			agent: d.agent.displayName || d.agent.id,
			...delegationTimes(d)
		}))
	);

	async function revoke() {
		const target = revoking;
		if (!target) return;
		busy = true;
		try {
			await AgentIdentitiesService.revokeMyAgentDelegation(target.id);
			delegations = delegations.filter((d) => d.id !== target.id);
		} catch {
			// The error is shown by the notifications.
		} finally {
			busy = false;
			revoking = undefined;
		}
	}
</script>

<svelte:head>
	<title>Obot | {m.identity_access_acting_for_you_title()}</title>
</svelte:head>

<Layout title={m.identity_access_acting_for_you_title()}>
	<div class="flex flex-col gap-4 pb-8">
		<p class="text-muted-content text-sm font-light">
			{m.identity_access_acting_for_you_description()}
		</p>

		{#if rows.length === 0}
			<p class="text-muted-content py-8 text-center text-sm">
				{m.identity_access_acting_for_you_none()}
			</p>
		{:else}
			<Table
				data={rows}
				fields={['agent', 'firstApprovedAt', 'lastApprovedAt', 'lastUsedAt']}
				headers={[
					{ title: m.identity_access_acting_for_you_col_agent(), property: 'agent' },
					{
						title: m.identity_access_delegations_col_first_approved(),
						property: 'firstApprovedAt'
					},
					{ title: m.identity_access_delegations_col_last_approved(), property: 'lastApprovedAt' },
					{ title: m.identity_access_delegations_col_last_used(), property: 'lastUsedAt' }
				]}
				onClickRow={(row, isCtrlClick) =>
					openUrl(resolve(`/agents/delegations/${encodeURIComponent(row.id)}`), isCtrlClick)}
			>
				{#snippet actions(d)}
					<button
						class="btn btn-error btn-soft btn-sm"
						disabled={busy}
						onclick={(e) => {
							e.stopPropagation();
							revoking = delegations.find((delegation) => delegation.id === d.id);
						}}
					>
						{m.identity_access_agents_revoke()}
					</button>
				{/snippet}
			</Table>
		{/if}
	</div>
</Layout>

<Confirm
	msg={m.identity_access_acting_for_you_confirm_revoke({
		agent: revoking ? revoking.agent.displayName || revoking.agent.id : ''
	})}
	note={m.identity_access_acting_for_you_confirm_revoke_note()}
	submitText={m.identity_access_agents_revoke()}
	show={Boolean(revoking)}
	loading={busy}
	onsuccess={revoke}
	oncancel={() => (revoking = undefined)}
/>
