<script lang="ts">
	import { page } from '$app/state';
	import Confirm from '$lib/components/Confirm.svelte';
	import McpLogin from '$lib/components/mcp/McpLogin.svelte';
	import { getHttpStatusCode, isAbortError, parseErrorContent } from '$lib/errors';
	import { m } from '$lib/i18n';
	import Loading from '$lib/icons/Loading.svelte';
	import {
		UserService,
		type VMCP,
		type VMCPAgentOAuthAuthorization,
		type VMCPAgentOAuthStatus,
		type VMCPAgentSetup,
		type VMCPAgentSetupComponent
	} from '$lib/services';
	import { onOAuthCompletion } from '$lib/services/mcp/oauth-completion';
	import { vmcpComponentId, vmcpComponentName } from '$lib/services/vmcps/utils';
	import { notifications } from '$lib/stores';
	import {
		AGENT_SETUP_ANCHOR,
		agentSetupAnchor,
		componentAnchor,
		oauthAnchor,
		type AgentSetupTarget
	} from '$lib/vmcpAgentSetupLinks';
	import { CircleCheck, KeyRound, TriangleAlert } from '@lucide/svelte';
	import { onDestroy, tick, untrack } from 'svelte';

	// What agents need from a shared vMCP, for each component that a profile naming an agent grants.
	// It is the vMCP's alone: agents get only the configuration the vMCP or its catalog entry fixes, and
	// an administrator authorizes each OAuth component once, here, for every agent the component is
	// granted to. Profiles carry no agent setup.

	interface Props {
		vmcp: VMCP;
		// Names the administrator who authorized a grant, from the ID the setup gives.
		userName?: (id: string) => string | undefined;
	}

	let { vmcp, userName }: Props = $props();

	// How often the setup is read while an authorization is pending, and how long the administrator has
	// to finish one in a popup. A localhost login's command shows its own expiry, as the login attempt
	// reports it.
	const POLL_INTERVAL_MS = 2000;
	const OAUTH_DEADLINE_MS = 10 * 60 * 1000;

	type Flow = {
		componentID: string;
		// The grant as it was when the authorization started. Completing it changes this.
		signature: string;
		authURL: string;
		localhost: boolean;
		deadline: number;
		blocked: boolean;
		// The localhost login command expired: there is nothing more to wait for until a new one.
		expired: boolean;
	};

	let setup = $state<VMCPAgentSetup>();
	let loadError = $state('');
	// Bumped to load the setup again after it failed.
	let reloadKey = $state(0);
	// Why the vMCP's grants cannot be changed right now, as the server said it (409).
	let blockedReason = $state('');

	let flow = $state<Flow>();
	let starting = $state<string>();
	// Why authorizing or revoking a component's grant failed, by component.
	let oauthErrors = $state<Record<string, string>>({});
	let confirmRevoke = $state<{ id: string; name: string }>();
	let revoking = $state(false);
	let handledHash = '';

	// The ID alone, so a vMCP that is saved again (a new object with the same ID) does not reload the setup.
	const vmcpID = $derived(vmcp.id);
	const components = $derived(vmcp.components ?? []);
	const deleting = $derived(Boolean(vmcp.deleted));
	const editable = $derived(!deleting && !blockedReason);

	function componentName(id: string) {
		return vmcpComponentName(vmcp, id) || m.core_unknown();
	}

	function fieldConfig(id: string, key: string) {
		const component = components.find((candidate) => vmcpComponentId(candidate) === id);
		return component?.catalogEntry?.manifest?.config?.find((field) => field.key === key);
	}

	function fieldLabel(id: string, key: string) {
		return key === '__url' ? m.vmcps_server_url() : fieldConfig(id, key)?.name || key;
	}

	// The keys of a component that agents can't be given, as its reasons name them.
	function missingFields(component: VMCPAgentSetupComponent) {
		return component.reasons
			.filter((reason) => reason.code === 'configuration_required' && reason.field)
			.map((reason) => reason.field as string);
	}

	function oauthSignature(oauth?: VMCPAgentOAuthStatus) {
		return oauth ? `${oauth.state}|${oauth.authorizedAt ?? ''}|${oauth.authorizedBy ?? ''}` : '';
	}

	// What an administrator still has to do, each pointing at the component or button it is about.
	const todo = $derived.by(() => {
		const items: { text: string; target: AgentSetupTarget }[] = [];
		for (const component of setup?.components ?? []) {
			const name = componentName(component.componentID);
			for (const reason of component.reasons) {
				switch (reason.code) {
					case 'configuration_required':
						// Agents get only the values the vMCP or its catalog entry fix, so these are fixed
						// there, or the component is not granted to agents.
						items.push({
							text: m.vmcps_agent_setup_todo_configuration({
								component: name,
								field: fieldLabel(component.componentID, reason.field ?? '')
							}),
							target: { componentID: component.componentID, configuration: true }
						});
						break;
					case 'oauth_required':
						items.push({
							text: m.vmcps_agent_setup_todo_oauth({ component: name }),
							target: { componentID: component.componentID }
						});
						break;
					case 'oauth_invalid':
						items.push({
							text: m.vmcps_agent_setup_todo_oauth_invalid({ component: name }),
							target: { componentID: component.componentID }
						});
						break;
				}
			}
		}
		return items;
	});

	function reset() {
		stopFlow();
		setup = undefined;
		loadError = '';
		blockedReason = '';
		oauthErrors = {};
		confirmRevoke = undefined;
		handledHash = '';
	}

	// Load the setup, and again whenever the vMCP changes.
	$effect(() => {
		const vmcpIDNow = vmcpID;
		void reloadKey;
		untrack(reset);
		const controller = new AbortController();
		UserService.getVMCPAgentSetup(vmcpIDNow, { signal: controller.signal })
			.then((loaded) => {
				if (!controller.signal.aborted) setup = loaded;
			})
			.catch((err) => {
				if (controller.signal.aborted || isAbortError(err)) return;
				loadError = parseErrorContent(err).message;
			});
		return () => controller.abort();
	});

	function retryLoad() {
		reloadKey++;
	}

	// A link from the agent page lands on a component or button of this section.
	$effect(() => {
		const hash = decodeURIComponent(page.url.hash.slice(1));
		if (!setup || !hash || hash === handledHash) return;
		handledHash = hash;
		void tick().then(() => focusAnchor(hash));
	});

	function focusAnchor(id: string) {
		const element = document.getElementById(id);
		if (!element) return;
		element.scrollIntoView({ block: 'center' });
		if (element instanceof HTMLElement && !element.hasAttribute('disabled')) element.focus();
	}

	function focusTarget(target: AgentSetupTarget) {
		focusAnchor(agentSetupAnchor(target));
	}

	// An error that says the vMCP's grants cannot be changed right now, whichever request found out.
	function noteBlocked(err: unknown) {
		const { status: code, message } = parseErrorContent(err);
		if (code === 409) blockedReason = message;
		return message;
	}

	// OAuth

	async function authorize(id: string) {
		if (starting) return;
		starting = id;
		delete oauthErrors[id];
		stopFlow();
		try {
			const authorization = await UserService.authorizeVMCPAgentOAuth(vmcpID, id);
			beginFlow(id, authorization);
		} catch (err) {
			// The message is shown as returned: it can carry the placeholders that stand for a configured URL,
			// and, for a URL agents can't get, says where it is fixed.
			oauthErrors[id] = noteBlocked(err);
		} finally {
			starting = undefined;
		}
	}

	function openAuthorizationWindow(url: string) {
		let target: URL;
		try {
			target = new URL(url, window.location.origin);
		} catch {
			return false;
		}
		// Only a web page is opened, whatever the server said.
		if (target.protocol !== 'https:' && target.protocol !== 'http:') return false;
		const popup = window.open(target.href, '_blank', 'popup,width=600,height=760');
		// The provider's pages must not reach back into this one.
		if (popup) popup.opener = null;
		return Boolean(popup);
	}

	function beginFlow(id: string, authorization: VMCPAgentOAuthAuthorization) {
		const oauth = setup?.components.find((component) => component.componentID === id)?.oauth;
		const localhost = Boolean(authorization.localhostCallback);
		flow = {
			componentID: id,
			signature: oauthSignature(oauth),
			authURL: authorization.authURL,
			localhost,
			// A localhost login's command expires on the server's schedule, which McpLogin shows.
			deadline: localhost ? Infinity : Date.now() + OAUTH_DEADLINE_MS,
			// A localhost login is run in a terminal, not opened.
			blocked: localhost ? false : !openAuthorizationWindow(authorization.authURL),
			expired: false
		};
	}

	function stopFlow() {
		flow = undefined;
	}

	function reopenWindow() {
		if (!flow) return;
		flow.blocked = !openAuthorizationWindow(flow.authURL);
	}

	// Reads the setup now while an authorization is pending, and resolves to whether it is over.
	let checkNow: (() => Promise<boolean>) | undefined;

	// McpLogin calls this when the login attempt has been used, which may have completed the login, and
	// to generate a new command. The setup says first whether the grant changed, so a finished login
	// never shows as expired; otherwise a new command replaces the old one.
	async function finishLocalhostLogin(id: string) {
		if (await checkNow?.()) return;
		await authorize(id);
	}

	// While an authorization is pending, read the setup until the component's grant changes: that is the
	// only signal that the callback stored it, whichever window or machine completed it. The callback page
	// also tells this one (it shares an origin), which only makes the next read sooner.
	$effect(() => {
		const pending = flow;
		if (!pending || pending.expired) return;
		const current: Flow = pending;
		const vmcpIDNow = untrack(() => vmcpID);
		const controller = new AbortController();
		let timer: ReturnType<typeof setTimeout> | undefined;
		let checking: Promise<boolean> | undefined;
		let stopped = false;

		const schedule = () => {
			clearTimeout(timer);
			if (!stopped) timer = setTimeout(check, POLL_INTERVAL_MS);
		};
		// One read at a time: a check asked for while one is running gets that one's answer.
		function check(): Promise<boolean> {
			if (stopped) return Promise.resolve(true);
			checking ??= read().finally(() => (checking = undefined));
			return checking;
		}
		async function read(): Promise<boolean> {
			if (Date.now() > current.deadline) {
				flow = undefined;
				oauthErrors[current.componentID] = m.vmcps_agent_setup_oauth_expired();
				return true;
			}
			try {
				const next = await UserService.getVMCPAgentSetup(vmcpIDNow, { signal: controller.signal });
				if (stopped) return true;
				setup = next;
				const oauth = next.components.find(
					(component) => component.componentID === current.componentID
				)?.oauth;
				if (oauthSignature(oauth) !== current.signature) {
					flow = undefined;
					notifications.add(
						m.vmcps_agent_setup_oauth_done({ name: componentName(current.componentID) }),
						'success'
					);
					return true;
				}
			} catch (err) {
				if (stopped || isAbortError(err)) return true;
				// The vMCP is gone: nothing more will change.
				if (getHttpStatusCode(err) === 404) {
					flow = undefined;
					oauthErrors[current.componentID] = parseErrorContent(err).message;
					return true;
				}
			}
			schedule();
			return false;
		}
		checkNow = check;

		const onVisible = () => {
			if (document.visibilityState === 'visible') void check();
		};
		const stopListening = onOAuthCompletion(() => void check());
		document.addEventListener('visibilitychange', onVisible);
		schedule();
		return () => {
			stopped = true;
			checkNow = undefined;
			clearTimeout(timer);
			controller.abort();
			stopListening();
			document.removeEventListener('visibilitychange', onVisible);
		};
	});

	onDestroy(stopFlow);

	async function revoke() {
		if (!confirmRevoke || revoking) return;
		const { id, name } = confirmRevoke;
		revoking = true;
		try {
			setup = await UserService.revokeVMCPAgentOAuth(vmcpID, id);
			notifications.add(m.vmcps_agent_setup_oauth_revoke_done({ name }), 'success');
		} catch (err) {
			oauthErrors[id] = noteBlocked(err);
		} finally {
			revoking = false;
			confirmRevoke = undefined;
		}
	}

	function grantedBy(oauth: VMCPAgentOAuthStatus) {
		const by = oauth.authorizedBy ? (userName?.(oauth.authorizedBy) ?? oauth.authorizedBy) : '';
		const on = oauth.authorizedAt ? new Date(oauth.authorizedAt).toLocaleDateString() : '';
		if (by && on) return m.vmcps_agent_setup_oauth_authorized_by_on({ name: by, date: on });
		if (by) return m.vmcps_agent_setup_oauth_authorized_by({ name: by });
		return m.vmcps_agent_setup_oauth_authorized();
	}
</script>

<section
	id={AGENT_SETUP_ANCHOR}
	class="border-base-300 bg-base-100 dark:bg-base-300 flex flex-col gap-4 rounded-xl border p-5 shadow-sm"
	aria-labelledby="agent-setup-title"
>
	<header class="flex flex-col gap-1">
		<h2 id="agent-setup-title" class="text-md flex items-center gap-2 font-semibold">
			<KeyRound class="size-4" />
			{m.vmcps_agent_setup_title()}
		</h2>
		<p class="text-muted-content text-sm font-light">{m.vmcps_agent_setup_description()}</p>
	</header>

	{#if !setup && !loadError}
		<Loading class="size-5" />
	{:else if loadError || !setup}
		<div class="flex flex-col items-start gap-2">
			<p class="text-error text-sm" role="alert">
				{m.vmcps_agent_setup_load_failed()}
				{loadError}
			</p>
			<button type="button" class="btn btn-sm btn-secondary" onclick={retryLoad}>
				{m.common_try_again()}
			</button>
		</div>
	{:else if setup.components.length === 0}
		<p class="text-muted-content text-sm" role="status">{m.vmcps_agent_setup_no_components()}</p>
	{:else}
		{#if deleting}
			<p class="notification-alert text-sm" role="alert">{m.vmcps_agent_setup_deleting()}</p>
		{:else if blockedReason}
			<p class="notification-alert text-sm" role="alert">{blockedReason}</p>
		{/if}

		{#if todo.length === 0}
			<p class="flex items-center gap-2 text-sm" role="status">
				<CircleCheck class="text-success size-4" />
				{m.vmcps_agent_setup_ready()}
			</p>
		{:else}
			<div class="flex flex-col gap-1 text-sm">
				<p class="font-medium">{m.vmcps_agent_setup_not_ready()}</p>
				<ul class="flex flex-col gap-0.5">
					{#each todo as item (item.text)}
						<li>
							<button type="button" class="link text-left" onclick={() => focusTarget(item.target)}>
								{item.text}
							</button>
						</li>
					{/each}
				</ul>
			</div>
		{/if}

		<div class="flex flex-col gap-4">
			{#each setup.components as component (component.componentID)}
				{@const id = component.componentID}
				{@const vmcpComponent = components.find((candidate) => vmcpComponentId(candidate) === id)}
				{@const missing = missingFields(component)}
				<section
					id={componentAnchor(id)}
					tabindex="-1"
					class="border-base-300 dark:border-base-400 flex flex-col gap-3 rounded-lg border p-4"
					aria-label={componentName(id)}
				>
					<h3 class="flex items-center gap-2 text-sm font-semibold">
						{#if vmcpComponent?.catalogEntry?.manifest?.icon}
							<img src={vmcpComponent.catalogEntry.manifest.icon} alt="" class="size-4" />
						{/if}
						{componentName(id)}
					</h3>

					<div class="flex flex-col gap-1">
						<h4 class="text-sm font-medium">{m.vmcps_agent_setup_configuration_heading()}</h4>
						{#if missing.length > 0}
							<p class="notification-alert flex items-start gap-2 text-sm">
								<TriangleAlert class="mt-0.5 size-4 shrink-0" />
								{m.vmcps_agent_setup_configuration_note({
									fields: missing.map((key) => fieldLabel(id, key)).join(', ')
								})}
							</p>
						{:else}
							<p class="text-success text-sm">{m.vmcps_agent_setup_configuration_ready()}</p>
						{/if}
					</div>

					<div class="border-base-300 dark:border-base-400 flex flex-col gap-2 border-t pt-3">
						{#if !component.oauth}
							<h4 class="text-sm font-medium">{m.vmcps_agent_setup_oauth_heading()}</h4>
							<p class="text-muted-content text-sm" data-oauth-state="none">
								{m.vmcps_agent_setup_oauth_not_required()}
							</p>
						{:else}
							{@const oauth = component.oauth}
							{@const error = oauthErrors[id]}
							{@const active = flow?.componentID === id ? flow : undefined}
							<div class="flex flex-wrap items-center justify-between gap-2">
								<div class="flex flex-col">
									<h4 class="text-sm font-medium">
										{m.vmcps_agent_setup_oauth_heading()}
										{#if oauth.required}
											<span class="badge badge-ghost badge-sm ml-1"
												>{m.vmcps_agent_setup_required()}</span
											>
										{/if}
									</h4>
									<p class="text-sm" data-oauth-state={oauth.state}>
										{#if oauth.state === 'authorized'}
											<span class="text-success">{grantedBy(oauth)}</span>
										{:else if oauth.state === 'invalid'}
											<span class="text-warning">{m.vmcps_agent_setup_oauth_invalid()}</span>
										{:else if oauth.required}
											<span class="text-warning">{m.vmcps_agent_setup_oauth_missing()}</span>
										{:else}
											<span class="text-muted-content"
												>{m.vmcps_agent_setup_oauth_not_required()}</span
											>
										{/if}
										<!-- The target is text, never a link: a URL that comes from configuration is not shown at all. -->
										{#if oauth.state !== 'missing'}
											{#if oauth.target}
												<span class="text-muted-content break-all"
													>{m.vmcps_agent_setup_oauth_target({ target: oauth.target })}</span
												>
											{:else if oauth.targetHostname}
												<span class="text-muted-content break-all"
													>{m.vmcps_agent_setup_oauth_target_host({
														hostname: oauth.targetHostname
													})}</span
												>
											{/if}
										{/if}
									</p>
									{#if oauth.state === 'authorized' && oauth.scopes?.length}
										<p class="text-muted-content text-xs">
											{m.vmcps_agent_setup_oauth_scopes({ scopes: oauth.scopes.join(' ') })}
										</p>
									{/if}
									{#if oauth.state === 'invalid' && oauth.invalidReason}
										<p class="text-muted-content text-xs">{oauth.invalidReason}</p>
									{/if}
									{#if oauth.state === 'missing' && !oauth.required}
										<p class="text-muted-content text-xs">
											{m.vmcps_agent_setup_oauth_not_required_hint()}
										</p>
									{/if}
								</div>
								<span class="flex items-center gap-2">
									{#if oauth.state !== 'authorized'}
										<button
											type="button"
											id={oauthAnchor(id)}
											class="btn btn-sm btn-primary"
											disabled={!editable || Boolean(starting) || Boolean(active)}
											onclick={() => authorize(id)}
										>
											{#if starting === id}<Loading class="size-4" />{/if}
											{oauth.state === 'invalid'
												? m.vmcps_agent_setup_authorize_again()
												: m.vmcps_agent_setup_authorize()}
										</button>
									{/if}
									{#if oauth.state !== 'missing'}
										<button
											type="button"
											class="btn btn-sm btn-ghost text-error"
											disabled={deleting || revoking}
											onclick={() => (confirmRevoke = { id, name: componentName(id) })}
										>
											{m.vmcps_agent_setup_revoke()}
										</button>
									{/if}
								</span>
							</div>

							{#if active}
								<div class="notification-info flex flex-col gap-2 text-sm" role="status">
									{#if active.localhost}
										<p>{m.vmcps_agent_setup_oauth_localhost_intro()}</p>
										<McpLogin
											url={active.authURL}
											serverName={componentName(id)}
											showTitle={false}
											showStatus={false}
											onComplete={() => finishLocalhostLogin(id)}
											onExpiredChange={(expired) => {
												if (active.expired !== expired) active.expired = expired;
											}}
										/>
									{:else}
										<p class="flex items-center gap-2">
											<Loading class="size-4 shrink-0" />
											{m.vmcps_agent_setup_oauth_waiting()}
										</p>
										{#if active.blocked}
											<p class="text-warning">{m.vmcps_agent_setup_oauth_blocked()}</p>
										{/if}
										<div>
											<button type="button" class="btn btn-sm btn-secondary" onclick={reopenWindow}>
												{m.vmcps_agent_setup_oauth_open()}
											</button>
										</div>
									{/if}
									<div>
										<button type="button" class="btn btn-sm btn-ghost" onclick={stopFlow}>
											{m.common_cancel()}
										</button>
									</div>
								</div>
							{/if}

							{#if error}
								<p class="text-error text-sm break-words" role="alert">{error}</p>
							{/if}
						{/if}
					</div>
				</section>
			{/each}
		</div>
	{/if}
</section>

<Confirm
	show={Boolean(confirmRevoke)}
	onsuccess={revoke}
	oncancel={() => (confirmRevoke = undefined)}
	loading={revoking}
	title={m.vmcps_agent_setup_oauth_revoke_title()}
	msg={m.vmcps_agent_setup_oauth_revoke_msg({ name: confirmRevoke?.name ?? '' })}
	submitText={m.vmcps_agent_setup_revoke()}
/>
