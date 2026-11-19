<script lang="ts">
	import { m } from '$lib/i18n';
	import Loading from '$lib/icons/Loading.svelte';
	import { AgentIdentitiesService, UserService, type VMCP } from '$lib/services';
	import type {
		AgentIdentityAccess,
		AgentReadinessReason,
		AgentReadinessSource,
		AgentVMCPReadiness
	} from '$lib/services/agent-identities/types';
	import { vmcpComponentName } from '$lib/services/vmcps/utils';
	import { resolve } from '$lib/url';
	import { vmcpProfileHref } from '$lib/vmcpProfileLinks';
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
	// The sources of each vMCP's values, read on request: they cost a query per OAuth component.
	let sources = $state<Record<string, { items?: AgentReadinessSource[]; failed?: boolean }>>({});
	let showSources = $state<Record<string, boolean>>({});
	// The vMCPs whose components the reasons and sources name, so that the components are named as the
	// vMCP's agent setup names them. Until one loads, or if it cannot, its components show their IDs.
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

	async function toggleSources(vmcpID: string) {
		showSources[vmcpID] = !showSources[vmcpID];
		if (!showSources[vmcpID] || sources[vmcpID]?.items) return;
		try {
			const detail = await AgentIdentitiesService.getAgentVMCPReadiness(agentId, vmcpID);
			sources[vmcpID] = { items: detail.sources ?? [] };
			if (detail.sources?.length) loadVMCP(vmcpID);
		} catch {
			sources[vmcpID] = { failed: true };
		}
	}

	async function load() {
		try {
			[access, readiness] = await Promise.all([
				AgentIdentitiesService.getAgentAccess(agentId),
				AgentIdentitiesService.listAgentVMCPReadiness(agentId)
			]);
			// Whatever was shown about the sources may have changed.
			sources = {};
			showSources = {};
			failed = false;
			for (const item of readiness) {
				if (item.reasons.some((reason) => reason.componentID)) loadVMCP(item.vmcpID);
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

	// The name of a profile of a vMCP, from the profiles that name the agent.
	function profileName(vmcpID: string, profileID: string | undefined) {
		if (!profileID) return '';
		const match = access?.vmcpProfiles.find(
			(profile) => profile.vmcpID === vmcpID && profile.profileID === profileID
		);
		return match?.profileName ?? '';
	}

	// Where an administrator fixes a reason: the profile it names, at the field or button that does it.
	function reasonLink(item: AgentVMCPReadiness, reason: AgentReadinessReason) {
		if (!reason.profileID) return undefined;
		const componentID = reason.componentID;
		const name = profileName(item.vmcpID, reason.profileID);
		return {
			href: vmcpProfileHref(
				item.vmcpID,
				reason.profileID,
				componentID
					? {
							componentID,
							field: reason.code === 'configuration_required' ? reason.field : undefined
						}
					: undefined
			),
			text: name
				? m.identity_access_agents_reason_open_profile({ profile: name })
				: m.identity_access_agents_reason_open_profile_unnamed()
		};
	}

	function sourceLabel(vmcpID: string, source: AgentReadinessSource) {
		const component = componentName(vmcpID, source.componentID);
		return source.kind === 'oauth'
			? m.identity_access_agents_source_oauth({ component })
			: m.identity_access_agents_source_configuration({ component, field: source.field ?? '' });
	}

	function sourceFrom(vmcpID: string, source: AgentReadinessSource) {
		const profile = profileName(vmcpID, source.profileID) || source.profileID;
		return m.identity_access_agents_source_from({ profile });
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
										{@const link = reasonLink(item, reason)}
										<li class="flex flex-col">
											<span class="text-warning">{reasonLabel(item.vmcpID, reason)}</span>
											{#if link}
												<a class="link w-fit" href={resolve(link.href)}>{link.text}</a>
											{/if}
										</li>
									{/each}
								</ul>
							{/if}
							<button
								type="button"
								class="link w-fit text-xs"
								aria-expanded={Boolean(showSources[item.vmcpID])}
								onclick={() => toggleSources(item.vmcpID)}
							>
								{showSources[item.vmcpID]
									? m.identity_access_agents_sources_hide()
									: m.identity_access_agents_sources_show()}
							</button>
							{#if showSources[item.vmcpID]}
								{@const loaded = sources[item.vmcpID]}
								{#if loaded?.failed}
									<p class="text-error text-xs" role="alert">
										{m.identity_access_agents_sources_failed()}
									</p>
								{:else if !loaded?.items}
									<Loading class="size-4" />
								{:else if loaded.items.length === 0}
									<p class="text-muted-content text-xs">
										{m.identity_access_agents_sources_none()}
									</p>
								{:else}
									<ul class="text-xs">
										{#each loaded.items as source, index (index)}
											<li>
												{sourceLabel(item.vmcpID, source)},
												<a
													class="link"
													href={resolve(
														vmcpProfileHref(
															item.vmcpID,
															source.profileID,
															source.kind === 'oauth'
																? { componentID: source.componentID }
																: { componentID: source.componentID, field: source.field }
														)
													)}>{sourceFrom(item.vmcpID, source)}</a
												>
											</li>
										{/each}
									</ul>
								{/if}
							{/if}
						</li>
					{/each}
				</ul>
			{/if}
		</div>
	{/if}
</section>
