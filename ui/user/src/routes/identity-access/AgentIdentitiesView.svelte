<script lang="ts">
	import Confirm from '$lib/components/Confirm.svelte';
	import CopyButton from '$lib/components/CopyButton.svelte';
	import DotDotDot from '$lib/components/DotDotDot.svelte';
	import { agentStatusBadge } from '$lib/components/agent-identities/AgentIdentityDetails.svelte';
	import Table from '$lib/components/table/Table.svelte';
	import { m } from '$lib/i18n';
	import { AgentIdentitiesService, type OrgUser } from '$lib/services';
	import { agentStatusLabel } from '$lib/services/agent-identities/helpers';
	import type { AgentIdentity } from '$lib/services/agent-identities/types';
	import { profile } from '$lib/stores';
	import { formatTimeAgo } from '$lib/time';
	import { goto, resolve } from '$lib/url';
	import { getUserDisplayName, openUrl } from '$lib/utils';
	import CreateAgentDialog from './CreateAgentDialog.svelte';
	import { Bot } from '@lucide/svelte';
	import { untrack } from 'svelte';

	interface Props {
		agents: AgentIdentity[];
		users: OrgUser[];
	}

	let { agents: initialAgents, users: initialUsers }: Props = $props();
	let agents = $state<AgentIdentity[]>(untrack(() => initialAgents));
	let usersMap = $derived(new Map(initialUsers.map((user) => [user.id, user])));
	let isAdminReadonly = $derived(profile.current.isAdminReadonly?.());

	let createDialog = $state<ReturnType<typeof CreateAgentDialog>>();
	let disablingAgent = $state<AgentIdentity>();
	let deletingAgent = $state<AgentIdentity>();
	let loading = $state(false);

	const tableData = $derived(
		agents.map((agent) => ({
			...agent,
			statusLabel: agentStatusLabel(agent.status),
			creator: agent.creatorUserID ? getUserDisplayName(usersMap, agent.creatorUserID) : '-'
		}))
	);

	export function openCreate() {
		createDialog?.open();
	}

	function replaceAgent(updated: AgentIdentity) {
		agents = agents.map((agent) => (agent.id === updated.id ? { ...agent, ...updated } : agent));
	}

	async function disable() {
		const agent = disablingAgent;
		if (!agent) return;
		loading = true;
		try {
			replaceAgent(await AgentIdentitiesService.disableAgent(agent.id));
		} finally {
			loading = false;
			disablingAgent = undefined;
		}
	}

	async function enable(agent: AgentIdentity) {
		replaceAgent(await AgentIdentitiesService.enableAgent(agent.id));
	}

	async function remove() {
		const agent = deletingAgent;
		if (!agent) return;
		loading = true;
		try {
			await AgentIdentitiesService.deleteAgent(agent.id);
			agents = agents.filter((a) => a.id !== agent.id);
		} finally {
			loading = false;
			deletingAgent = undefined;
		}
	}

	function detailPath(id: string): `/${string}` {
		return `/identity-access/agent-identities/${encodeURIComponent(id)}`;
	}
</script>

<div class="flex flex-col gap-4">
	<p class="text-muted-content mb-1 whitespace-pre-line text-sm">
		{m.identity_access_agents_description()}
	</p>
	{#if agents.length === 0}
		<div class="mt-16 flex w-lg max-w-full flex-col items-center gap-4 self-center text-center">
			<Bot class="text-base-content/80 size-24 opacity-50" />
			<h4 class="text-muted-content text-lg font-semibold">{m.identity_access_agents_none()}</h4>
			<p class="text-muted-content text-sm font-light">
				{m.identity_access_agents_none_description()}
			</p>
		</div>
	{:else}
		<Table
			data={tableData}
			fields={['displayName', 'statusLabel', 'creator', 'activeAPIKeyCount', 'created']}
			headers={[
				{ title: m.core_name(), property: 'displayName' },
				{ title: m.core_status(), property: 'statusLabel' },
				{ title: m.core_col_created_by(), property: 'creator' },
				{ title: m.identity_access_agents_col_active_keys(), property: 'activeAPIKeyCount' },
				{ title: m.core_col_created(), property: 'created' }
			]}
			filterable={['displayName', 'statusLabel', 'creator']}
			sortable={['displayName', 'statusLabel', 'creator', 'activeAPIKeyCount', 'created']}
			initSort={{ property: 'created', order: 'desc' }}
			onClickRow={(d, isCtrlClick) => openUrl(detailPath(d.id), isCtrlClick)}
		>
			{#snippet onRenderColumn(property, d)}
				{#if property === 'statusLabel'}
					<div class="flex flex-col gap-0.5">
						{@render agentStatusBadge(d.status)}
						{#if d.status === 'disabled' && d.disabledReason}
							<span class="text-muted-content text-xs">{d.disabledReason}</span>
						{/if}
					</div>
				{:else if property === 'created'}
					{formatTimeAgo(d.created).relativeTime}
				{:else}
					{d[property as keyof typeof d]}
				{/if}
			{/snippet}
			{#snippet actions(d)}
				<DotDotDot>
					<a class="menu-button" href={resolve(detailPath(d.id))}>
						{m.identity_access_agents_view_details()}
					</a>
					<CopyButton
						text={d.id}
						buttonText={m.identity_access_agents_copy_id()}
						classes={{ button: 'menu-button' }}
					/>
					{#if !isAdminReadonly}
						{#if d.status === 'active'}
							<button class="menu-button" onclick={() => (disablingAgent = d)}>
								{m.identity_access_agents_disable()}
							</button>
						{:else if d.status === 'disabled'}
							<button class="menu-button" onclick={() => enable(d)}>
								{m.identity_access_agents_enable()}
							</button>
						{/if}
						<button class="menu-button text-error" onclick={() => (deletingAgent = d)}>
							{m.core_delete()}
						</button>
					{/if}
				</DotDotDot>
			{/snippet}
		</Table>
	{/if}
</div>

<CreateAgentDialog
	bind:this={createDialog}
	onCreated={(agent) => {
		// Keys are made on the agent's page, so the new agent opens there.
		agents = [agent, ...agents];
		goto(detailPath(agent.id));
	}}
/>

<Confirm
	msg={m.identity_access_agents_confirm_disable({ name: disablingAgent?.displayName ?? '' })}
	note={m.identity_access_agents_confirm_disable_note()}
	submitText={m.identity_access_agents_disable()}
	show={Boolean(disablingAgent)}
	{loading}
	onsuccess={disable}
	oncancel={() => (disablingAgent = undefined)}
/>

<Confirm
	msg={m.identity_access_api_keys_confirm_delete_named({ name: deletingAgent?.displayName ?? '' })}
	note={m.identity_access_agents_confirm_delete_note()}
	show={Boolean(deletingAgent)}
	{loading}
	onsuccess={remove}
	oncancel={() => (deletingAgent = undefined)}
/>
