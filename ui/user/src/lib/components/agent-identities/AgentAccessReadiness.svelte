<script lang="ts">
	import { m } from '$lib/i18n';
	import Loading from '$lib/icons/Loading.svelte';
	import { AgentIdentitiesService, UserService, type VMCP } from '$lib/services';
	import type {
		AgentIdentityAccess,
		AgentReadinessReason,
		AgentVMCPReadiness
	} from '$lib/services/agent-identities/types';
	import { vmcpComponentName } from '$lib/services/vmcps/utils';
	import { resolve } from '$lib/url';
	import { vmcpComponentHref } from '$lib/vmcpComponentLinks';
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
	// The vMCPs whose components the reasons or the preconfigured components name, so that the
	// components are named as the vMCP editor names them. Until one loads, or if it cannot, its components show their IDs.
	let vmcps = $state<Record<string, VMCP>>({});

	function loadVMCP(vmcpID: string) {
		if (vmcps[vmcpID]) return;
		UserService.getVMCP(vmcpID, { dontLogErrors: true })
			.then((vmcp) => {
				vmcps[vmcpID] = vmcp;
			})
			.catch(() => {});
	}

	function componentName(vmcpID: string, componentID: string | undefined) {
		return componentID ? vmcpComponentName(vmcps[vmcpID], componentID) : '';
	}

	async function load() {
		try {
			[access, readiness] = await Promise.all([
				AgentIdentitiesService.getAgentAccess(agentId),
				AgentIdentitiesService.listAgentVMCPReadiness(agentId)
			]);
			failed = false;
			for (const item of readiness) {
				if (
					item.preconfiguredComponents.length > 0 ||
					item.reasons.some((reason) => reason.componentID)
				) {
					loadVMCP(item.vmcpID);
				}
			}
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

	// Where an administrator acts on a reason: the component's settings in the vMCP editor, on its
	// preconfigured OAuth or, for a value agents can't set, on its configuration, where the value is
	// fixed. Profiles hold nothing for agents.
	function reasonHref(item: AgentVMCPReadiness, reason: AgentReadinessReason) {
		const componentID = reason.componentID;
		return vmcpComponentHref(
			item.vmcpID,
			componentID
				? {
						componentID,
						settings: reason.code === 'configuration_required' ? 'configuration' : 'oauth'
					}
				: undefined
		);
	}

	// Whether a user supplies what the reason is about when the agent acts on their behalf: a value
	// users enter, or their own upstream account for a component without preconfigured OAuth. Such a
	// component is the user's to set up (OBO), not a problem, though an administrator can still set it
	// for everyone on the vMCP. Preconfigured OAuth that no longer works (oauth_invalid) is used in
	// place of the user's account, so only an administrator can fix it.
	function userSupplied(reason: AgentReadinessReason) {
		return reason.code === 'configuration_required' || reason.code === 'oauth_required';
	}

	// A vMCP the agent can't use for itself, but can on behalf of users once they supply the rest.
	function onlyUserSupplied(item: AgentVMCPReadiness) {
		return !item.ready && item.reasons.length > 0 && item.reasons.every(userSupplied);
	}

	function reasonLabel(vmcpID: string, reason: AgentReadinessReason): string {
		const component = componentName(vmcpID, reason.componentID);
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
			default:
				return reason.code;
		}
	}
</script>

{#snippet policyList(title: string, items: { id: string; name: string }[])}
	<div class="flex flex-col gap-1">
		<h3 class="text-sm font-semibold">{title}</h3>
		{#if items.length === 0}
			<p class="text-muted-content text-sm">{m.identity_access_agents_access_none()}</p>
		{:else}
			<ul class="flex flex-col gap-1">
				{#each items as item (item.id)}
					<li class="bg-base-200 rounded-lg px-3 py-2 text-sm">
						<span class="block truncate">{item.name}</span>
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
				name: `${p.vmcpDisplayName || p.vmcpID} · ${p.profileName}`
			}))
		)}
		{@render policyList(
			m.identity_access_agents_access_model_policies(),
			access.modelAccessPolicies.map((p) => ({ id: p.id, name: p.displayName || p.id }))
		)}
		{@render policyList(
			m.identity_access_agents_access_skill_rules(),
			access.skillAccessRules.map((p) => ({ id: p.id, name: p.displayName || p.id }))
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
								{:else if onlyUserSupplied(item)}
									<span class="badge badge-info badge-soft badge-sm">
										{m.identity_access_agents_readiness_obo()}
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
							{#if item.reasons.length > 0}
								<ul class="text-xs">
									{#each item.reasons as reason, index (index)}
										<li class="flex flex-col">
											{#if userSupplied(reason)}
												<span class="flex items-start justify-between gap-2">
													<span>{reasonLabel(item.vmcpID, reason)}</span>
													<span class="badge badge-info badge-soft badge-sm shrink-0">
														{m.identity_access_agents_readiness_obo()}
													</span>
												</span>
												<a class="link w-fit" href={resolve(reasonHref(item, reason))}
													>{m.identity_access_agents_reason_open_vmcp_instead()}</a
												>
											{:else}
												<span class="text-warning">{reasonLabel(item.vmcpID, reason)}</span>
												<a class="link w-fit" href={resolve(reasonHref(item, reason))}
													>{m.identity_access_agents_reason_open_vmcp()}</a
												>
											{/if}
										</li>
									{/each}
								</ul>
							{/if}
							{#if item.preconfiguredComponents.length > 0}
								<!-- The components whose OAuth an administrator preconfigured on the vMCP, which the agent uses. -->
								<ul class="flex flex-col gap-1 text-xs">
									{#each item.preconfiguredComponents as componentID (componentID)}
										<li class="flex items-center justify-between gap-2">
											<span class="truncate">{componentName(item.vmcpID, componentID)}</span>
											<span class="badge badge-info badge-soft badge-sm">
												{m.identity_access_agents_readiness_preconfigured()}
											</span>
										</li>
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
