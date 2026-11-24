<script lang="ts">
	import Confirm from '$lib/components/Confirm.svelte';
	import Layout from '$lib/components/Layout.svelte';
	import AgentDelegationSetup from '$lib/components/agent-identities/AgentDelegationSetup.svelte';
	import { m } from '$lib/i18n';
	import { AgentIdentitiesService } from '$lib/services';
	import { delegationTimes } from '$lib/services/agent-identities/helpers';
	import { goto, resolve } from '$lib/url';
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();

	let confirmRevoke = $state(false);
	let revoking = $state(false);

	const delegation = $derived(data.delegation);
	const agentName = $derived(delegation.agent.displayName || delegation.agent.id);
	const times = $derived(delegationTimes(delegation));

	async function revoke() {
		revoking = true;
		try {
			await AgentIdentitiesService.revokeMyAgentDelegation(delegation.id);
			confirmRevoke = false;
			await goto(resolve('/agents/delegations'));
		} catch {
			// The error is shown by the notifications.
		} finally {
			revoking = false;
		}
	}
</script>

<svelte:head>
	<title>Obot | {agentName}</title>
</svelte:head>

<Layout
	title={agentName}
	showBackButton
	onBackButtonClick={() => goto(resolve('/agents/delegations'))}
>
	<div class="flex flex-col gap-6 pb-8">
		<section class="paper flex flex-col gap-3 p-4">
			<div class="flex flex-wrap items-start justify-between gap-3">
				<div class="flex flex-col gap-1">
					{#if delegation.agent.description}
						<p class="text-sm">{delegation.agent.description}</p>
					{/if}
					<p class="text-muted-content text-sm font-light">
						{m.identity_access_acting_for_you_detail_description({ agent: agentName })}
					</p>
				</div>
				<button
					type="button"
					class="btn btn-error btn-soft btn-sm"
					onclick={() => (confirmRevoke = true)}
				>
					{m.identity_access_agents_revoke()}
				</button>
			</div>
			<dl class="grid grid-cols-1 gap-2 text-sm sm:grid-cols-3">
				{#each [[m.identity_access_delegations_col_first_approved(), times.firstApprovedAt], [m.identity_access_delegations_col_last_approved(), times.lastApprovedAt], [m.identity_access_delegations_col_last_used(), times.lastUsedAt]] as [label, time] (label)}
					<div>
						<dt class="text-muted-content">{label}</dt>
						<dd>{time}</dd>
					</div>
				{/each}
			</dl>
		</section>

		<section class="paper flex flex-col gap-3 p-4" aria-labelledby="delegation-setup-title">
			<div class="flex flex-col gap-1">
				<h2 id="delegation-setup-title" class="text-lg font-semibold">
					{m.identity_access_acting_for_you_setup_title()}
				</h2>
				<p class="text-muted-content text-sm font-light">
					{m.identity_access_acting_for_you_setup_description({ agent: agentName })}
				</p>
			</div>
			<AgentDelegationSetup
				scope={{ kind: 'delegation', delegationId: delegation.id }}
				oauthError={data.oauthError}
			/>
		</section>
	</div>
</Layout>

<Confirm
	msg={m.identity_access_acting_for_you_confirm_revoke({ agent: agentName })}
	note={m.identity_access_acting_for_you_confirm_revoke_note()}
	submitText={m.identity_access_agents_revoke()}
	show={confirmRevoke}
	loading={revoking}
	onsuccess={revoke}
	oncancel={() => (confirmRevoke = false)}
/>
