<script lang="ts">
	import { m } from '$lib/i18n';
	import Loading from '$lib/icons/Loading.svelte';
	import { AgentIdentitiesService } from '$lib/services';
	import type { AdminAuditEvent } from '$lib/services/agent-identities/types';
	import { formatTimeAgo } from '$lib/time';
	import { untrack } from 'svelte';

	interface Props {
		agentId: string;
		// The IDs of the agent's keys, revoked ones included: the audit has no filter by parent, so
		// their events are asked for by ID.
		keyIds: number[];
		// Changes when the agent or its keys do, so the history is read again.
		refreshKey?: unknown;
	}

	let { agentId, keyIds, refreshKey }: Props = $props();

	const LIMIT = 100;

	let events = $state<AdminAuditEvent[]>([]);
	let total = $state(0);
	let loading = $state(true);
	let failed = $state(false);

	async function load() {
		try {
			const [agentEvents, keyEvents] = await Promise.all([
				AgentIdentitiesService.listAdminAuditEvents({
					resourceKind: 'agent',
					resourceId: agentId,
					limit: LIMIT
				}),
				keyIds.length
					? AgentIdentitiesService.listAdminAuditEvents({
							resourceKind: 'apiKey',
							resourceId: keyIds.join(','),
							limit: LIMIT
						})
					: Promise.resolve({ items: [], total: 0, limit: LIMIT, offset: 0 })
			]);
			events = [...(agentEvents.items ?? []), ...(keyEvents.items ?? [])]
				.filter((event) => event.resourceKind === 'agent' || event.parentID === agentId)
				.sort((a, b) => b.createdAt.localeCompare(a.createdAt));
			total = agentEvents.total + keyEvents.total;
			failed = false;
		} catch {
			failed = true;
		} finally {
			loading = false;
		}
	}

	$effect(() => {
		void refreshKey;
		untrack(load);
	});

	const actionLabels: Record<string, string> = {
		'agent.create': m.identity_access_agents_history_agent_create(),
		'agent.update': m.identity_access_agents_history_agent_update(),
		'agent.disable': m.identity_access_agents_history_agent_disable(),
		'agent.enable': m.identity_access_agents_history_agent_enable(),
		'agent.delete': m.identity_access_agents_history_agent_delete(),
		'agent.key.create': m.identity_access_agents_history_key_create(),
		'agent.key.revoke': m.identity_access_agents_history_key_revoke()
	};

	function actorLabel(event: AdminAuditEvent): string {
		if (event.actor.actorType === 'system') return m.audit_usage_exports_admin_actor_system();
		return event.actor.displayName || event.actor.id || m.core_unknown();
	}
</script>

<section class="paper flex flex-col gap-2 p-4" aria-labelledby="agent-history-title">
	<h2 id="agent-history-title" class="text-lg font-semibold">
		{m.identity_access_agents_history_title()}
	</h2>
	{#if loading}
		<Loading class="size-5" />
	{:else if failed}
		<p class="text-error text-sm" role="alert">{m.identity_access_agents_history_failed()}</p>
	{:else if events.length === 0}
		<p class="text-muted-content text-sm">{m.identity_access_agents_history_none()}</p>
	{:else}
		<ul class="flex flex-col gap-1">
			{#each events as event (event.id)}
				<li
					class="bg-base-200 flex flex-wrap items-center justify-between gap-2 rounded-lg px-3 py-2 text-sm"
				>
					<span>{actionLabels[event.action] ?? event.action}</span>
					<span class="text-muted-content text-xs">
						{actorLabel(event)} · {formatTimeAgo(event.createdAt).relativeTime}
					</span>
				</li>
			{/each}
		</ul>
		{#if total > events.length}
			<p class="text-muted-content text-xs">
				{m.identity_access_agents_history_truncated({ shown: events.length, total })}
			</p>
		{/if}
	{/if}
</section>
