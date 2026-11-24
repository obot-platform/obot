<script lang="ts">
	import Confirm from '$lib/components/Confirm.svelte';
	import McpLogin from '$lib/components/mcp/McpLogin.svelte';
	import { getHttpStatusCode, isAbortError, parseErrorContent } from '$lib/errors';
	import { m } from '$lib/i18n';
	import Loading from '$lib/icons/Loading.svelte';
	import {
		UserService,
		VMCPPreconfiguredOAuthStates as States,
		type VMCP,
		type VMCPPreconfiguredOAuth,
		type VMCPPreconfiguredOAuthAuthorization
	} from '$lib/services';
	import { onOAuthCompletion } from '$lib/services/mcp/oauth-completion';
	import { vmcpComponentName } from '$lib/services/vmcps/utils';
	import { notifications, profile } from '$lib/stores';
	import { TriangleAlert } from '@lucide/svelte';
	import { onDestroy, untrack } from 'svelte';

	// The preconfigured OAuth of one remote component of a shared vMCP: an administrator authorizes the
	// component once, and everyone who uses the vMCP, users and agents alike, connects to it upstream as
	// that account. Users aren't asked to sign in to the component while it is set. The grant is kept
	// outside the vMCP's manifest, so it can be changed on a catalog-synced vMCP too. Auditors see it
	// read-only.

	interface Props {
		vmcp: VMCP;
		componentID: string;
		// Names the administrator who authorized the grant, from the ID the state gives.
		userName?: (id: string) => string | undefined;
	}

	let { vmcp, componentID, userName }: Props = $props();

	// How often the state is read while an authorization is pending, and how long the administrator has
	// to finish one in a popup. A localhost login's command shows its own expiry, as the login attempt
	// reports it.
	const POLL_INTERVAL_MS = 2000;
	const OAUTH_DEADLINE_MS = 10 * 60 * 1000;

	type Flow = {
		// The grant as it was when the authorization started. Completing it changes this, even when it
		// replaces an authorized grant, whose state stays authorized.
		signature: string;
		authURL: string;
		localhost: boolean;
		deadline: number;
		blocked: boolean;
		// The localhost login command expired: there is nothing more to wait for until a new one.
		expired: boolean;
	};

	let oauth = $state<VMCPPreconfiguredOAuth>();
	let loadError = $state('');
	// Bumped to load the state again after it failed.
	let reloadKey = $state(0);
	// Why the grant cannot be changed right now, as the server said it (409).
	let blockedReason = $state('');

	let flow = $state<Flow>();
	let starting = $state(false);
	// Why authorizing or revoking failed.
	let actionError = $state('');
	let confirmRevoke = $state(false);
	let revoking = $state(false);

	// The IDs alone, so a vMCP that is saved again (a new object with the same ID) does not reload.
	const vmcpID = $derived(vmcp.id);
	const name = $derived(vmcpComponentName(vmcp, componentID));
	const canManage = $derived(Boolean(profile.current.isAdmin?.()));
	const deleting = $derived(Boolean(vmcp.deleted));
	const editable = $derived(canManage && !deleting && !blockedReason);
	const granted = $derived(oauth?.state === States.Authorized || oauth?.state === States.Invalid);

	// Who authorized the grant and when, not only its state: authorizing an authorized grant again
	// replaces it once the new authorization completes, and leaves it authorized.
	function signatureOf(state?: VMCPPreconfiguredOAuth) {
		return state ? `${state.state}|${state.authorizedAt ?? ''}|${state.authorizedBy ?? ''}` : '';
	}

	function reset() {
		stopFlow();
		oauth = undefined;
		loadError = '';
		blockedReason = '';
		actionError = '';
		confirmRevoke = false;
	}

	// Load the state, and again whenever the vMCP or component changes.
	$effect(() => {
		const vmcpIDNow = vmcpID;
		const componentIDNow = componentID;
		void reloadKey;
		untrack(reset);
		const controller = new AbortController();
		UserService.getVMCPPreconfiguredOAuth(vmcpIDNow, componentIDNow, {
			signal: controller.signal
		})
			.then((loaded) => {
				if (!controller.signal.aborted) oauth = loaded;
			})
			.catch((err) => {
				if (controller.signal.aborted || isAbortError(err)) return;
				loadError = parseErrorContent(err).message;
			});
		return () => controller.abort();
	});

	// An error that says the grant cannot be changed right now, whichever request found out.
	function noteBlocked(err: unknown) {
		const { status: code, message } = parseErrorContent(err);
		if (code === 409) blockedReason = message;
		return message;
	}

	async function authorize() {
		if (starting) return;
		starting = true;
		actionError = '';
		stopFlow();
		try {
			beginFlow(await UserService.authorizeVMCPPreconfiguredOAuth(vmcpID, componentID));
		} catch (err) {
			// The message is shown as returned: for a URL that needs values the vMCP doesn't fix, it says
			// which, and where to fix them.
			actionError = noteBlocked(err);
		} finally {
			starting = false;
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

	function beginFlow(authorization: VMCPPreconfiguredOAuthAuthorization) {
		const localhost = Boolean(authorization.localhostCallback);
		flow = {
			signature: signatureOf(oauth),
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

	// Reads the state now while an authorization is pending, and resolves to whether it is over.
	let checkNow: (() => Promise<boolean>) | undefined;

	// McpLogin calls this when the login attempt has been used, which may have completed the login, and
	// to generate a new command. The state says first whether the grant changed, so a finished login
	// never shows as expired; otherwise a new command replaces the old one.
	async function finishLocalhostLogin() {
		if (await checkNow?.()) return;
		await authorize();
	}

	// While an authorization is pending, read the state until the grant changes: that is the only signal
	// that the callback stored it, whichever window or machine completed it. The callback page also
	// tells this one (it shares an origin), which only makes the next read sooner.
	$effect(() => {
		const pending = flow;
		if (!pending || pending.expired) return;
		const current: Flow = pending;
		const vmcpIDNow = untrack(() => vmcpID);
		const componentIDNow = untrack(() => componentID);
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
				actionError = m.vmcps_preconfigured_oauth_expired();
				return true;
			}
			try {
				const next = await UserService.getVMCPPreconfiguredOAuth(vmcpIDNow, componentIDNow, {
					signal: controller.signal
				});
				if (stopped) return true;
				oauth = next;
				if (signatureOf(next) !== current.signature) {
					flow = undefined;
					notifications.add(m.vmcps_preconfigured_oauth_done({ name }), 'success');
					return true;
				}
			} catch (err) {
				if (stopped || isAbortError(err)) return true;
				// The vMCP or component is gone: nothing more will change.
				if (getHttpStatusCode(err) === 404) {
					flow = undefined;
					actionError = parseErrorContent(err).message;
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
		revoking = true;
		actionError = '';
		try {
			await UserService.revokeVMCPPreconfiguredOAuth(vmcpID, componentID);
			notifications.add(m.vmcps_preconfigured_oauth_revoke_done({ name }), 'success');
			// The DELETE returns nothing: read the state again, as when the section loads.
			reloadKey++;
		} catch (err) {
			actionError = noteBlocked(err);
		} finally {
			revoking = false;
			confirmRevoke = false;
		}
	}

	function authorizedBy(state: VMCPPreconfiguredOAuth) {
		const by = state.authorizedBy ? (userName?.(state.authorizedBy) ?? state.authorizedBy) : '';
		const on = state.authorizedAt ? new Date(state.authorizedAt).toLocaleDateString() : '';
		if (by && on) return m.vmcps_preconfigured_oauth_authorized_by_on({ name: by, date: on });
		if (by) return m.vmcps_preconfigured_oauth_authorized_by({ name: by });
		return m.vmcps_preconfigured_oauth_authorized();
	}
</script>

<section class="flex flex-col gap-3" aria-labelledby="preconfigured-oauth-title">
	<h3 id="preconfigured-oauth-title" class="text-sm font-semibold">
		{m.vmcps_preconfigured_oauth_title()}
	</h3>
	<p class="text-muted-content text-sm font-light">
		{m.vmcps_preconfigured_oauth_description({ name })}
	</p>
	<!-- Only once an account is preconfigured, working or not, does everyone using the vMCP act as it. -->
	{#if granted}
		<p class="notification-alert flex items-start gap-2 text-sm" role="note">
			<TriangleAlert class="mt-0.5 size-4 shrink-0" />
			{m.vmcps_preconfigured_oauth_warning({ name })}
		</p>
	{/if}

	{#if !oauth && !loadError}
		<Loading class="size-5" />
	{:else if loadError || !oauth}
		<div class="flex flex-col items-start gap-2">
			<p class="text-error text-sm break-words" role="alert">
				{m.vmcps_preconfigured_oauth_load_failed()}
				{loadError}
			</p>
			<button type="button" class="btn btn-sm btn-secondary" onclick={() => reloadKey++}>
				{m.common_try_again()}
			</button>
		</div>
	{:else}
		{#if canManage && deleting}
			<p class="notification-alert text-sm" role="alert">
				{m.vmcps_preconfigured_oauth_deleting()}
			</p>
		{:else if canManage && blockedReason}
			<p class="notification-alert text-sm" role="alert">{blockedReason}</p>
		{/if}

		<div
			class="border-base-300 dark:border-base-400 flex flex-col gap-2 rounded-lg border p-3"
			data-oauth-state={oauth.state}
		>
			<p class="text-sm" role="status">
				{#if oauth.state === States.Authorized}
					<span class="text-success">{authorizedBy(oauth)}</span>
				{:else if oauth.state === States.Invalid}
					<span class="text-warning">{m.vmcps_preconfigured_oauth_invalid()}</span>
				{:else if oauth.state === States.NeedsAuthorization}
					<span class="text-warning">{m.vmcps_preconfigured_oauth_needs_authorization()}</span>
				{:else}
					<span class="text-muted-content">{m.vmcps_preconfigured_oauth_not_required()}</span>
				{/if}
				<!-- The target is text, never a link: a URL that comes from configuration shows its host only. -->
				{#if granted}
					{#if oauth.target}
						<span class="text-muted-content break-all"
							>{m.vmcps_preconfigured_oauth_target({ target: oauth.target })}</span
						>
					{:else if oauth.targetHostname}
						<span class="text-muted-content break-all"
							>{m.vmcps_preconfigured_oauth_target_host({ hostname: oauth.targetHostname })}</span
						>
					{/if}
				{/if}
			</p>
			{#if granted && oauth.scopes?.length}
				<p class="text-muted-content text-xs">
					{m.vmcps_preconfigured_oauth_scopes({ scopes: oauth.scopes.join(' ') })}
				</p>
			{/if}
			{#if oauth.state === States.Invalid && oauth.invalidReason}
				<p class="text-muted-content text-xs break-words">{oauth.invalidReason}</p>
			{/if}
			{#if oauth.state === States.Invalid}
				<p class="text-muted-content text-xs">{m.vmcps_preconfigured_oauth_invalid_hint()}</p>
			{:else if oauth.state === States.NeedsAuthorization}
				<p class="text-muted-content text-xs">
					{m.vmcps_preconfigured_oauth_needs_authorization_hint()}
				</p>
			{:else if oauth.state === States.NotRequired}
				<p class="text-muted-content text-xs">
					{m.vmcps_preconfigured_oauth_not_required_hint()}
				</p>
			{/if}

			{#if canManage && oauth.state === States.Authorized}
				<p class="text-muted-content text-xs">
					{m.vmcps_preconfigured_oauth_authorize_again_hint()}
				</p>
			{/if}
			{#if canManage}
				<span class="flex flex-wrap items-center gap-2">
					<!-- Authorizing a granted component again replaces its grant only once the new
					     authorization completes, so switching accounts needs no Revoke first. -->
					<button
						type="button"
						class={oauth.state === States.Authorized
							? 'btn btn-sm btn-secondary'
							: 'btn btn-sm btn-primary'}
						disabled={!editable || starting || Boolean(flow)}
						onclick={authorize}
					>
						{#if starting}<Loading class="size-4" />{/if}
						{granted
							? m.vmcps_preconfigured_oauth_authorize_again()
							: m.vmcps_preconfigured_oauth_authorize()}
					</button>
					{#if granted}
						<button
							type="button"
							class="btn btn-sm btn-ghost text-error"
							disabled={deleting || revoking}
							onclick={() => (confirmRevoke = true)}
						>
							{m.vmcps_preconfigured_oauth_revoke()}
						</button>
					{/if}
				</span>
			{/if}
		</div>

		{#if flow}
			{@const active = flow}
			<div class="notification-info flex flex-col gap-2 text-sm" role="status">
				{#if active.localhost}
					<p>{m.vmcps_preconfigured_oauth_localhost_intro()}</p>
					<McpLogin
						url={active.authURL}
						serverName={name}
						showTitle={false}
						showStatus={false}
						onComplete={finishLocalhostLogin}
						onExpiredChange={(expired) => {
							if (active.expired !== expired) active.expired = expired;
						}}
					/>
				{:else}
					<p class="flex items-center gap-2">
						<Loading class="size-4 shrink-0" />
						{m.vmcps_preconfigured_oauth_waiting()}
					</p>
					{#if active.blocked}
						<p class="text-warning">{m.vmcps_preconfigured_oauth_blocked()}</p>
					{/if}
					<div>
						<button type="button" class="btn btn-sm btn-secondary" onclick={reopenWindow}>
							{m.vmcps_preconfigured_oauth_open()}
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

		{#if actionError}
			<p class="text-error text-sm break-words" role="alert">{actionError}</p>
		{/if}
	{/if}
</section>

<Confirm
	show={confirmRevoke}
	onsuccess={revoke}
	oncancel={() => (confirmRevoke = false)}
	loading={revoking}
	title={m.vmcps_preconfigured_oauth_revoke_title()}
	msg={m.vmcps_preconfigured_oauth_revoke_msg({ name })}
	submitText={m.vmcps_preconfigured_oauth_revoke()}
/>
