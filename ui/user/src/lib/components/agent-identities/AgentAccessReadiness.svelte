<script lang="ts">
	import { m } from '$lib/i18n';
	import Loading from '$lib/icons/Loading.svelte';
	import { AgentIdentitiesService } from '$lib/services';
	import type {
		AgentIdentityAccess,
		AgentReadinessReason,
		AgentSubjectMatch,
		AgentVMCPReadiness
	} from '$lib/services/agent-identities/types';
	import { CircleCheck, TriangleAlert } from '@lucide/svelte';
	import { untrack } from 'svelte';

	interface Props {
		agentId: string;
		// Changes when the agent's keys or status do, so the access problems are read again.
		refreshKey?: unknown;
	}

	let { agentId, refreshKey }: Props = $props();

	let access = $state<AgentIdentityAccess>();
	let readiness = $state<AgentVMCPReadiness[]>([]);
	let loading = $state(true);
	let failed = $state(false);

	async function load() {
		try {
			[access, readiness] = await Promise.all([
				AgentIdentitiesService.getAgentAccess(agentId),
				AgentIdentitiesService.listAgentVMCPReadiness(agentId)
			]);
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

	const matchLabels: Record<AgentSubjectMatch, string> = {
		explicit: m.identity_access_agents_match_explicit(),
		kindWildcard: m.identity_access_agents_match_kind_wildcard(),
		wildcard: m.identity_access_agents_match_wildcard()
	};

	function reasonLabel(reason: AgentReadinessReason): string {
		const component = reason.componentID ?? '';
		switch (reason.code) {
			case 'configuration_required':
				return m.identity_access_agents_reason_configuration_required({
					component,
					field: reason.field ?? ''
				});
			case 'oauth_required':
				return m.identity_access_agents_reason_oauth_required({ component });
			case 'oauth_invalid':
				return m.identity_access_agents_reason_oauth_invalid({ component });
			case 'selection_ambiguous':
				return m.identity_access_agents_reason_selection_ambiguous();
			default:
				return reason.code;
		}
	}
</script>

{#snippet policyList(
	title: string,
	items: { id: string; name: string; match: AgentSubjectMatch }[]
)}
	<div class="flex flex-col gap-1">
		<h3 class="text-sm font-semibold">{title}</h3>
		{#if items.length === 0}
			<p class="text-muted-content text-sm">{m.identity_access_agents_access_none()}</p>
		{:else}
			<ul class="flex flex-col gap-1">
				{#each items as item (item.id)}
					<li
						class="bg-base-200 flex items-center justify-between gap-2 rounded-lg px-3 py-2 text-sm"
					>
						<span class="truncate">{item.name}</span>
						<span class="badge badge-ghost badge-sm whitespace-nowrap"
							>{matchLabels[item.match]}</span
						>
					</li>
				{/each}
			</ul>
		{/if}
	</div>
{/snippet}

<section class="paper flex flex-col gap-4 p-4" aria-labelledby="agent-access-title">
	<h2 id="agent-access-title" class="text-lg font-semibold">
		{m.identity_access_agents_access_title()}
	</h2>
	{#if loading}
		<Loading class="size-5" />
	{:else if failed || !access}
		<p class="text-error text-sm" role="alert">{m.identity_access_agents_access_failed()}</p>
	{:else}
		{#each access.problems as problem (problem.code)}
			<div class="notification-alert flex items-center gap-2 text-sm" role="alert">
				<TriangleAlert class="size-4 shrink-0" />
				{problem.code === 'no_model_access_policy'
					? m.identity_access_agents_problem_no_model_access_policy()
					: problem.code === 'no_skill_access_rule'
						? m.identity_access_agents_problem_no_skill_access_rule()
						: problem.code}
			</div>
		{/each}

		{@render policyList(
			m.identity_access_agents_access_vmcp_profiles(),
			access.vmcpProfiles.map((p) => ({
				id: `${p.vmcpID}/${p.profileID ?? p.profileName}`,
				name: `${p.vmcpDisplayName || p.vmcpID} · ${p.profileName}`,
				match: p.match
			}))
		)}
		{@render policyList(
			m.identity_access_agents_access_model_policies(),
			access.modelAccessPolicies.map((p) => ({
				id: p.id,
				name: p.displayName || p.id,
				match: p.match
			}))
		)}
		{@render policyList(
			m.identity_access_agents_access_skill_rules(),
			access.skillAccessRules.map((p) => ({
				id: p.id,
				name: p.displayName || p.id,
				match: p.match
			}))
		)}

		<div class="flex flex-col gap-1">
			<h3 class="text-sm font-semibold">{m.identity_access_agents_readiness_title()}</h3>
			{#if readiness.length === 0}
				<p class="text-muted-content text-sm">{m.identity_access_agents_readiness_none()}</p>
			{:else}
				<ul class="flex flex-col gap-2">
					{#each readiness as item (item.vmcpID)}
						<li class="bg-base-200 flex flex-col gap-1 rounded-lg px-3 py-2 text-sm">
							<div class="flex items-center justify-between gap-2">
								<span class="truncate font-medium">{item.vmcpDisplayName || item.vmcpID}</span>
								{#if item.ready}
									<span class="badge badge-success badge-soft badge-sm flex items-center gap-1">
										<CircleCheck class="size-3" />
										{m.identity_access_agents_readiness_ready()}
									</span>
								{:else}
									<span class="badge badge-warning badge-soft badge-sm">
										{m.identity_access_agents_readiness_not_ready()}
									</span>
								{/if}
							</div>
							{#if item.disabled}
								<p class="text-muted-content text-xs">
									{m.identity_access_agents_readiness_disabled()}
								</p>
							{/if}
							{#if item.pendingSync}
								<p class="text-muted-content text-xs">
									{m.identity_access_agents_readiness_pending_sync()}
								</p>
							{/if}
							{#if item.reasons.length > 0}
								<ul class="text-xs">
									{#each item.reasons as reason, index (index)}
										<li class="text-warning">{reasonLabel(reason)}</li>
									{/each}
								</ul>
							{/if}
						</li>
					{/each}
				</ul>
			{/if}
		</div>
	{/if}
</section>
