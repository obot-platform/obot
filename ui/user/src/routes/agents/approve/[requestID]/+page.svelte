<script lang="ts">
	import Confirm from '$lib/components/Confirm.svelte';
	import AgentDelegationSetup from '$lib/components/agent-identities/AgentDelegationSetup.svelte';
	import BetaLogo from '$lib/components/navbar/BetaLogo.svelte';
	import { parseErrorContent } from '$lib/errors';
	import { m } from '$lib/i18n';
	import Loading from '$lib/icons/Loading.svelte';
	import { navigateTo } from '$lib/navigation';
	import { AgentIdentitiesService } from '$lib/services';
	import { isWebURL } from '$lib/services/agent-identities';
	import type { AgentDelegationSetup as Setup } from '$lib/services/agent-identities/types';
	import { profile } from '$lib/stores';
	import type { PageProps } from './$types';
	import { Bot, CircleAlert, ShieldCheck } from '@lucide/svelte';

	let { data }: PageProps = $props();

	// A refusal of approving or cancelling replaces the consent with the same plain error page as a
	// request that can't be approved at all.
	let failure = $state('');
	let setup = $state<Setup>();
	let busy = $state<'approve' | 'cancel'>();
	let confirmMissing = $state(false);

	const request = $derived(data.request);
	const error = $derived(failure || data.error || '');
	const agentName = $derived(request?.agent.displayName || request?.agent.id || '');
	const userName = $derived(
		profile.current.displayName || profile.current.email || profile.current.username
	);
	const userEmail = $derived(
		profile.current.email && profile.current.email !== userName ? profile.current.email : ''
	);
	const setupComplete = $derived(Boolean(setup?.vmcps.every((vmcp) => vmcp.ready)));
	const signOutHref = $derived(`/oauth2/sign_out?rd=${encodeURIComponent(data.returnPath)}`);

	// The browser goes only where the server says: the agent's return URL, which the server built and
	// checked against the agent's allowed hosts. The page never builds one.
	function leave(redirectURL: string) {
		if (!isWebURL(redirectURL)) {
			failure = m.identity_access_approve_bad_redirect();
			busy = undefined;
			return;
		}
		navigateTo(redirectURL);
	}

	async function approve(confirmed = false) {
		if (!request || busy) return;
		if (!confirmed && !setupComplete) {
			confirmMissing = true;
			return;
		}
		confirmMissing = false;
		busy = 'approve';
		try {
			const { redirectURL } = await AgentIdentitiesService.approveAgentDelegationRequest(
				request.id
			);
			leave(redirectURL);
		} catch (err) {
			failure = parseErrorContent(err).message;
			busy = undefined;
		}
	}

	async function cancel() {
		if (!request || busy) return;
		busy = 'cancel';
		try {
			const { redirectURL } = await AgentIdentitiesService.cancelAgentDelegationRequest(request.id);
			leave(redirectURL);
		} catch (err) {
			failure = parseErrorContent(err).message;
			busy = undefined;
		}
	}
</script>

<svelte:head>
	<title>Obot | {m.identity_access_approve_page_title()}</title>
</svelte:head>

<div class="bg-base-200 dark:bg-base-100 flex min-h-screen items-center justify-center p-4">
	<main class="paper w-full max-w-2xl gap-0 overflow-hidden p-0">
		<BetaLogo class="mt-6 self-center" />

		{#if error || !request}
			<section class="flex flex-col items-center gap-4 p-6 text-center">
				<CircleAlert class="text-error size-8" />
				<h1 class="text-xl font-semibold">{m.identity_access_approve_error_title()}</h1>
				<p class="text-sm break-words" role="alert">{error}</p>
				{#if !profile.current.isBootstrapUser?.()}
					<p class="text-muted-content text-sm">
						{m.identity_access_approve_signed_in_as({ name: userName })}
						<a class="link" href={signOutHref} rel="external">
							{m.identity_access_approve_switch_account()}
						</a>
					</p>
				{/if}
			</section>
		{:else}
			<section class="flex flex-col gap-5 p-6">
				<div class="flex flex-col items-center gap-2 text-center">
					<Bot class="size-8" />
					<h1 class="text-xl font-semibold">
						{m.identity_access_approve_title({ agent: agentName })}
					</h1>
					{#if request.agent.description}
						<p class="text-muted-content text-sm">{request.agent.description}</p>
					{/if}
				</div>

				<p class="bg-base-100 dark:bg-base-200 rounded-md p-3 text-sm">
					{m.identity_access_approve_signed_in_as({ name: userName })}
					{#if userEmail}<span class="text-muted-content">({userEmail})</span>{/if}
					<a class="link ml-1" href={signOutHref} rel="external">
						{m.identity_access_approve_switch_account()}
					</a>
				</p>

				<div class="notification-info flex items-start gap-3 p-3">
					<ShieldCheck class="mt-0.5 size-5 shrink-0" />
					<p class="text-sm">{m.identity_access_approve_acts_as_you({ agent: agentName })}</p>
				</div>

				{#if request.delegationID}
					<p class="text-muted-content text-sm">{m.identity_access_approve_again()}</p>
				{/if}

				<section class="flex flex-col gap-2" aria-labelledby="approve-access-title">
					<h2 id="approve-access-title" class="text-base font-semibold">
						{m.identity_access_approve_access_title({ agent: agentName })}
					</h2>
					<dl class="flex flex-col gap-3 text-sm">
						<div class="flex flex-col gap-1">
							<dt class="font-medium">{m.identity_access_approve_access_vmcps()}</dt>
							<dd>
								{#if request.access.vmcps.length === 0}
									<span class="text-muted-content">{m.identity_access_approve_access_none()}</span>
								{:else}
									<ul class="flex flex-col gap-1">
										{#each request.access.vmcps as vmcp (vmcp.vmcpID)}
											<li>
												{vmcp.vmcpDisplayName || vmcp.vmcpID}
												{#if vmcp.profiles.length}
													<span class="text-muted-content">
														{m.identity_access_approve_access_profiles({
															profiles: vmcp.profiles.map((p) => p.name || p.id).join(', ')
														})}
													</span>
												{/if}
											</li>
										{/each}
									</ul>
								{/if}
							</dd>
						</div>
						<div class="flex flex-col gap-1">
							<dt class="font-medium">{m.identity_access_approve_access_llm()}</dt>
							<dd>
								{#if request.access.modelAccessPolicies.length === 0}
									<span class="text-muted-content">{m.identity_access_approve_access_none()}</span>
								{:else}
									{m.identity_access_approve_access_policies({
										policies: request.access.modelAccessPolicies
											.map((p) => p.displayName || p.id)
											.join(', ')
									})}
								{/if}
							</dd>
						</div>
						<div class="flex flex-col gap-1">
							<dt class="font-medium">{m.identity_access_approve_access_skills()}</dt>
							<dd>
								{#if request.access.skillAccessRules.length === 0}
									<span class="text-muted-content">{m.identity_access_approve_access_none()}</span>
								{:else}
									{m.identity_access_approve_access_rules({
										rules: request.access.skillAccessRules
											.map((r) => r.displayName || r.id)
											.join(', ')
									})}
								{/if}
							</dd>
						</div>
					</dl>
				</section>

				<section class="flex flex-col gap-2" aria-labelledby="approve-setup-title">
					<h2 id="approve-setup-title" class="text-base font-semibold">
						{m.identity_access_approve_setup_title()}
					</h2>
					<p class="text-muted-content text-sm">
						{m.identity_access_approve_setup_description({ agent: agentName })}
					</p>
					<AgentDelegationSetup
						scope={{ kind: 'request', requestId: request.id }}
						oauthError={data.oauthError}
						onchange={(next) => (setup = next)}
					/>
				</section>
			</section>

			<footer
				class="border-base-300 bg-base-100 dark:bg-base-200 flex justify-end gap-3 border-t p-3 max-sm:flex-col-reverse"
			>
				<button class="btn btn-text" type="button" disabled={Boolean(busy)} onclick={cancel}>
					{#if busy === 'cancel'}<Loading class="size-4" />{/if}
					{m.common_cancel()}
				</button>
				<button
					class="btn btn-primary"
					type="button"
					disabled={Boolean(busy)}
					onclick={() => approve()}
				>
					{#if busy === 'approve'}<Loading class="size-4" />{/if}
					{m.identity_access_approve_approve()}
				</button>
			</footer>
		{/if}
	</main>
</div>

<Confirm
	show={confirmMissing}
	type="info"
	title={m.identity_access_approve_missing_title()}
	msg={m.identity_access_approve_missing_msg()}
	note=""
	submitText={m.identity_access_approve_approve()}
	onsuccess={() => approve(true)}
	oncancel={() => (confirmMissing = false)}
/>
