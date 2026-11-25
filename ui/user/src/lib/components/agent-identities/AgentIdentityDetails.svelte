<script module lang="ts">
	import { m } from '$lib/i18n';
	import { agentStatusLabel } from '$lib/services/agent-identities/helpers';
	import type { AgentIdentityStatus } from '$lib/services/agent-identities/types';

	// The list of agents shows the same badge.
	export { agentStatusBadge };
</script>

<script lang="ts">
	import Confirm from '$lib/components/Confirm.svelte';
	import CopyButton from '$lib/components/CopyButton.svelte';
	import ApiKeyRevealDialog from '$lib/components/agent-auth-scope/ApiKeyRevealDialog.svelte';
	import Table from '$lib/components/table/Table.svelte';
	import Loading from '$lib/icons/Loading.svelte';
	import { AgentIdentitiesService } from '$lib/services';
	import { agentKeyMask } from '$lib/services/agent-identities/helpers';
	import type {
		AgentAPIKey,
		AgentAPIKeyCreateResponse,
		AgentIdentity,
		AgentIdentityManifest
	} from '$lib/services/agent-identities/types';
	import { formatTimeAgo, formatTimeUntil } from '$lib/time';
	import AgentAccessReadiness from './AgentAccessReadiness.svelte';
	import AgentDelegations from './AgentDelegations.svelte';
	import AgentHistory from './AgentHistory.svelte';
	import AgentProfilesTab from './AgentProfilesTab.svelte';
	import CreateAgentKeyDialog from './CreateAgentKeyDialog.svelte';
	import { Bot, Plus, Trash2 } from '@lucide/svelte';
	import { untrack } from 'svelte';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		agent: AgentIdentity;
		// Every key of the agent, revoked ones included.
		keys: AgentAPIKey[];
		creatorName?: string;
		readonly?: boolean;
		onDeleted: () => void;
	}

	let {
		agent: initialAgent,
		keys: initialKeys,
		creatorName,
		readonly,
		onDeleted
	}: Props = $props();

	let agent = $state<AgentIdentity>(untrack(() => initialAgent));
	let keys = $state<AgentAPIKey[]>(untrack(() => initialKeys));
	let displayName = $state(untrack(() => initialAgent.displayName));
	let description = $state(untrack(() => initialAgent.description ?? ''));
	let showRevoked = $state(false);
	let selectedTab = $state<'keys' | 'profiles' | 'delegations'>('keys');
	let saving = $state(false);
	let busy = $state(false);
	let showValidation = $state(false);
	let confirmDisable = $state(false);
	let confirmDelete = $state(false);
	let revokingKey = $state<AgentAPIKey>();
	let createdKeyValue = $state<string>();
	let createKeyDialog = $state<ReturnType<typeof CreateAgentKeyDialog>>();

	const isActive = $derived(agent.status === 'active');
	const dirty = $derived(
		displayName !== agent.displayName || description !== (agent.description ?? '')
	);
	const isRevoked = (key: AgentAPIKey) => Boolean(key.revokedAt);
	const isExpired = (key: AgentAPIKey) =>
		Boolean(key.expiresAt && new Date(key.expiresAt) < new Date());
	// Changes when something that the access and history sections read does.
	const refreshKey = $derived(
		`${agent.status}:${keys.map((key) => `${key.id}${key.revokedAt ?? ''}`).join(',')}`
	);

	const tableData = $derived(
		keys
			.filter((key) => showRevoked || !isRevoked(key))
			.map((key) => ({
				...key,
				mask: agentKeyMask(key),
				lastUsedDisplay: key.lastUsedAt
					? formatTimeAgo(key.lastUsedAt).relativeTime
					: m.core_never_used(),
				expiresDisplay: key.expiresAt
					? formatTimeUntil(key.expiresAt).relativeTime
					: m.core_never_expires(),
				state: isRevoked(key)
					? m.core_revoked()
					: isExpired(key)
						? m.identity_access_agents_key_expired()
						: m.identity_access_users_status_active()
			}))
	);

	async function saveMetadata(e?: SubmitEvent) {
		e?.preventDefault();
		showValidation = true;
		if (!displayName.trim()) return;
		saving = true;
		try {
			await saveManifest({
				displayName: displayName.trim(),
				description: description.trim() || undefined
			});
			displayName = agent.displayName;
			description = agent.description ?? '';
			showValidation = false;
		} catch {
			// The error is shown by the notifications.
		} finally {
			saving = false;
		}
	}

	// The update replaces the whole manifest, so what isn't being changed is sent back as it was saved: the agent
	// profiles and the allowed return hosts with a new name, and the saved name and description, not what is typed in
	// the form above, with a new list. It throws when the server refuses it.
	async function saveManifest(
		change: Partial<AgentIdentityManifest>,
		opts?: { dontLogErrors?: boolean }
	) {
		const updated = await AgentIdentitiesService.updateAgent(
			agent.id,
			{
				displayName: agent.displayName,
				description: agent.description,
				profiles: agent.profiles ?? [],
				allowedReturnHosts: agent.allowedReturnHosts ?? [],
				...change
			},
			opts
		);
		// The server leaves out a list that is empty, which a merge would not clear.
		agent = {
			...agent,
			...updated,
			profiles: updated.profiles,
			allowedReturnHosts: updated.allowedReturnHosts
		};
	}

	async function toggleStatus() {
		busy = true;
		try {
			agent = {
				...agent,
				...(isActive
					? await AgentIdentitiesService.disableAgent(agent.id)
					: await AgentIdentitiesService.enableAgent(agent.id))
			};
		} finally {
			busy = false;
			confirmDisable = false;
		}
	}

	async function deleteAgent() {
		busy = true;
		try {
			await AgentIdentitiesService.deleteAgent(agent.id);
			onDeleted();
		} finally {
			busy = false;
			confirmDelete = false;
		}
	}

	function handleKeyCreated(created: AgentAPIKeyCreateResponse) {
		const { key, ...rest } = created;
		keys = [rest, ...keys];
		agent = { ...agent, activeAPIKeyCount: agent.activeAPIKeyCount + 1 };
		createdKeyValue = key;
	}

	async function revokeKey() {
		const key = revokingKey;
		if (!key) return;
		busy = true;
		try {
			await AgentIdentitiesService.revokeAgentAPIKey(agent.id, key.id);
			keys = keys.map((k) => (k.id === key.id ? { ...k, revokedAt: new Date().toISOString() } : k));
			agent = { ...agent, activeAPIKeyCount: Math.max(0, agent.activeAPIKeyCount - 1) };
		} finally {
			busy = false;
			revokingKey = undefined;
		}
	}
</script>

<div class="flex h-full w-full flex-col gap-4">
	<section class="paper flex flex-col gap-4 p-4">
		<div class="flex flex-wrap items-start justify-between gap-4">
			<div class="flex items-center gap-4">
				<div class="p-2"><Bot class="size-8" /></div>
				<div class="flex flex-col gap-0.5 text-sm">
					<div class="flex items-center gap-2">
						<h1 class="text-xl font-semibold">{agent.displayName}</h1>
						{@render agentStatusBadge(agent.status)}
					</div>
					{#if agent.status === 'disabled' && agent.disabledReason}
						<p class="text-muted-content">{agent.disabledReason}</p>
					{/if}
					<p class="text-muted-content flex items-center gap-2 font-light">
						{m.identity_access_agents_id_label()}
						{agent.id}
						<CopyButton
							text={agent.id}
							noButtonText
							tooltipText={m.identity_access_agents_copy_id()}
						/>
					</p>
					<p class="text-muted-content font-light">
						{m.identity_access_api_keys_created_time({
							time: formatTimeAgo(agent.created).relativeTime
						})}{#if creatorName}
							· {m.identity_access_agents_created_by({ name: creatorName })}{/if}
					</p>
				</div>
			</div>
			{#if !readonly}
				<div class="flex gap-2">
					<button
						class="btn btn-secondary text-sm"
						disabled={busy}
						onclick={() => (isActive ? (confirmDisable = true) : toggleStatus())}
					>
						{isActive ? m.identity_access_agents_disable() : m.identity_access_agents_enable()}
					</button>
					<button
						class="btn btn-error btn-soft text-sm"
						disabled={busy}
						onclick={() => (confirmDelete = true)}
					>
						<Trash2 class="size-4" />
						{m.core_delete()}
					</button>
				</div>
			{/if}
		</div>

		<form class="flex flex-col gap-4 text-sm" onsubmit={saveMetadata}>
			<label class="flex flex-col gap-1" for="agent-display-name">
				<span class="input-label">
					{m.core_name()}
					{#if showValidation && !displayName.trim()}
						<span class="text-error text-xs">{m.identity_access_api_keys_name_is_required()}</span>
					{/if}
				</span>
				<input
					id="agent-display-name"
					class="text-input-filled"
					bind:value={displayName}
					disabled={readonly || saving}
				/>
			</label>
			<label class="flex flex-col gap-1" for="agent-description">
				<span class="input-label">{m.core_description()}</span>
				<input
					id="agent-description"
					class="text-input-filled"
					bind:value={description}
					disabled={readonly || saving}
				/>
			</label>
			{#if !readonly}
				<div class="flex justify-end">
					<button class="btn btn-primary text-sm" type="submit" disabled={!dirty || saving}>
						{#if saving}
							<Loading class="size-4" />
						{:else}
							{m.core_save()}
						{/if}
					</button>
				</div>
			{/if}
		</form>
	</section>

	<div role="tablist" class="tabs tabs-box tabs-sm self-start">
		{#each [['keys', m.identity_access_api_keys_tab()], ['profiles', m.identity_access_agent_profiles_tab()], ['delegations', m.identity_access_delegations_tab()]] as const as [value, label] (value)}
			<button
				role="tab"
				class={twMerge('tab', selectedTab === value && 'tab-active')}
				aria-selected={selectedTab === value}
				onclick={() => (selectedTab = value)}
			>
				{label}
			</button>
		{/each}
	</div>

	{#if selectedTab === 'keys'}
		<section class="paper flex flex-col gap-3 p-4" aria-labelledby="agent-keys-title">
			<div class="flex flex-wrap items-center justify-between gap-2">
				<h2 id="agent-keys-title" class="text-lg font-semibold">
					{m.identity_access_api_keys_tab()}
				</h2>
				<div class="flex items-center gap-4">
					<label class="flex items-center gap-2 text-sm">
						<input type="checkbox" class="checkbox checkbox-xs" bind:checked={showRevoked} />
						{m.identity_access_agents_show_revoked()}
					</label>
					{#if !readonly && agent.status !== 'deleted'}
						<button class="btn btn-primary text-sm" onclick={() => createKeyDialog?.open()}>
							<Plus class="size-4" />
							{m.identity_access_create_api_key()}
						</button>
					{/if}
				</div>
			</div>
			{#if tableData.length === 0}
				<p class="text-muted-content py-4 text-center text-sm">
					{m.identity_access_agents_keys_none()}
				</p>
			{:else}
				<Table
					data={tableData}
					fields={['name', 'mask', 'lastUsedAt', 'expiresAt', 'state']}
					headers={[
						{ title: m.core_name(), property: 'name' },
						{ title: m.identity_access_agents_col_key(), property: 'mask' },
						{ title: m.identity_access_api_keys_col_last_used(), property: 'lastUsedAt' },
						{ title: m.identity_access_api_keys_col_expires(), property: 'expiresAt' },
						{ title: m.core_status(), property: 'state' }
					]}
				>
					{#snippet onRenderColumn(property, d)}
						{#if property === 'mask'}
							<span class="whitespace-nowrap">{d.mask}</span>
						{:else if property === 'lastUsedAt'}
							{d.lastUsedDisplay}
						{:else if property === 'expiresAt'}
							{d.expiresDisplay}
						{:else}
							{d[property as keyof typeof d]}
						{/if}
					{/snippet}
					{#snippet actions(d)}
						{#if !readonly && !isRevoked(d)}
							<button
								class="btn btn-error btn-soft btn-sm"
								disabled={busy}
								onclick={() => (revokingKey = keys.find((key) => key.id === d.id))}
							>
								{m.identity_access_agents_revoke()}
							</button>
						{/if}
					{/snippet}
				</Table>
			{/if}
		</section>
	{:else if selectedTab === 'profiles'}
		<AgentProfilesTab
			{agent}
			{readonly}
			onSave={(change) => saveManifest(change, { dontLogErrors: true })}
		/>
	{:else}
		<AgentDelegations agentId={agent.id} agentName={agent.displayName} {readonly} />
	{/if}

	<AgentAccessReadiness agentId={agent.id} {refreshKey} />
	<AgentHistory agentId={agent.id} keyIds={keys.map((key) => key.id)} {refreshKey} />
</div>

<CreateAgentKeyDialog bind:this={createKeyDialog} agentId={agent.id} onCreated={handleKeyCreated} />
<ApiKeyRevealDialog keyValue={createdKeyValue} onClose={() => (createdKeyValue = undefined)} />

<Confirm
	msg={m.identity_access_agents_confirm_disable({ name: agent.displayName })}
	note={m.identity_access_agents_confirm_disable_note()}
	submitText={m.identity_access_agents_disable()}
	show={confirmDisable}
	loading={busy}
	onsuccess={toggleStatus}
	oncancel={() => (confirmDisable = false)}
/>
<Confirm
	msg={m.identity_access_api_keys_confirm_delete_named({ name: agent.displayName })}
	note={m.identity_access_agents_confirm_delete_note()}
	show={confirmDelete}
	loading={busy}
	onsuccess={deleteAgent}
	oncancel={() => (confirmDelete = false)}
/>
<Confirm
	msg={m.identity_access_agents_confirm_revoke({ name: revokingKey?.name ?? '' })}
	submitText={m.identity_access_agents_revoke()}
	show={Boolean(revokingKey)}
	loading={busy}
	onsuccess={revokeKey}
	oncancel={() => (revokingKey = undefined)}
/>

{#snippet agentStatusBadge(status: AgentIdentityStatus)}
	<span
		class={[
			'badge badge-sm whitespace-nowrap',
			status === 'active' && 'badge-ghost',
			status === 'disabled' && 'badge-warning badge-soft',
			status === 'deleted' && 'badge-error badge-soft'
		]}
	>
		{agentStatusLabel(status)}
	</span>
{/snippet}
