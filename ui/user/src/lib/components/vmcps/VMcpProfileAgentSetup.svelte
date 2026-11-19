<script lang="ts">
	import { page } from '$app/state';
	import Confirm from '$lib/components/Confirm.svelte';
	import McpLogin from '$lib/components/mcp/McpLogin.svelte';
	import {
		getHttpStatusCode,
		isAbortError,
		parseErrorContent,
		parseErrorReasons
	} from '$lib/errors';
	import { m } from '$lib/i18n';
	import Loading from '$lib/icons/Loading.svelte';
	import {
		UserService,
		type VMCP,
		type VMCPComponent,
		type VMCPProfileOAuthAuthorization,
		type VMCPProfileOAuthStatus,
		type VMCPProfileStatus
	} from '$lib/services';
	import { onOAuthCompletion } from '$lib/services/mcp/oauth-completion';
	import { vmcpComponentId, vmcpComponentName } from '$lib/services/vmcps/utils';
	import { notifications } from '$lib/stores';
	import {
		AGENT_SETUP_ANCHOR,
		configFieldAnchor,
		oauthAnchor,
		profileSetupAnchor,
		type ProfileSetupTarget
	} from '$lib/vmcpProfileLinks';
	import { CircleCheck, KeyRound, Lock } from '@lucide/svelte';
	import { onDestroy, tick, untrack } from 'svelte';

	interface Props {
		vmcp: VMCP;
		// The server-assigned ID of the saved profile. A profile that has not been saved has none.
		profileID?: string;
		// Names the administrator who authorized a grant, from the ID the status gives.
		userName?: (id: string) => string | undefined;
		// Identities were changed in the editor and not saved yet, so the status here is of the saved profile.
		unsaved?: boolean;
	}

	let { vmcp, profileID, userName, unsaved = false }: Props = $props();

	// How often the profile's status is read while an authorization is pending, and how long the
	// administrator has to finish one in a popup. A localhost login's command shows its own expiry, as
	// the login attempt reports it.
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

	let status = $state<VMCPProfileStatus>();
	let loadError = $state('');
	// Bumped to load the status again after it failed.
	let reloadKey = $state(0);
	// Why the profile cannot be changed right now, as the server said it (409).
	let blockedReason = $state('');

	// What the administrator typed, by component and key. A value is never read back from the server, so
	// nothing here outlives a save.
	let typed = $state<Record<string, string>>({});
	let clearing = $state<Record<string, boolean>>({});
	let replacing = $state<Record<string, boolean>>({});
	let saving = $state(false);
	let saveError = $state('');
	let confirmReset = $state(false);
	let resetting = $state(false);

	let flow = $state<Flow>();
	let starting = $state<string>();
	// fields are the configuration keys the server says are still missing, when it names them.
	let oauthErrors = $state<Record<string, { message: string; status: number; fields?: string[] }>>(
		{}
	);
	let confirmOAuthReset = $state<{ id: string; name: string }>();
	let resettingOAuth = $state(false);
	let handledHash = '';

	// The ID alone, so a vMCP that is saved again (a new object with the same ID) does not reload the setup.
	const vmcpID = $derived(vmcp.id);
	const components = $derived(vmcp.components ?? []);
	const deleting = $derived(Boolean(vmcp.deleted));
	const editable = $derived(!deleting && !blockedReason);
	const pendingChanges = $derived(
		Object.values(typed).some((value) => value.trim() !== '') ||
			Object.values(clearing).some(Boolean)
	);

	function componentName(id: string) {
		return vmcpComponentName(vmcp, id) || m.core_unknown();
	}

	function fieldId(id: string, key: string) {
		return `${id}\u0000${key}`;
	}

	function fieldConfig(id: string, key: string) {
		const component = components.find((candidate) => vmcpComponentId(candidate) === id);
		return component?.catalogEntry?.manifest?.config?.find((field) => field.key === key);
	}

	function fieldLabel(id: string, key: string) {
		return key === '__url' ? m.vmcps_server_url() : fieldConfig(id, key)?.name || key;
	}

	function fieldDescription(id: string, key: string) {
		if (key === '__url') {
			const component = components.find((candidate) => vmcpComponentId(candidate) === id);
			const hostname = component?.catalogEntry?.manifest?.remoteConfig?.hostname;
			return hostname ? m.vmcps_agent_setup_url_hostname({ hostname }) : '';
		}
		return fieldConfig(id, key)?.description ?? '';
	}

	function isRequired(id: string, key: string, missing: boolean) {
		return missing || key === '__url' || Boolean(fieldConfig(id, key)?.required);
	}

	// Fixed, catalog-supplied and Secret-bound values cannot be set by a profile. They apply to everyone,
	// so they are listed by name, and, as for every value, never shown.
	function fixedFields(component: VMCPComponent, configurable: Set<string>) {
		const fields: { key: string; name: string; kind: 'fixed' | 'catalog' | 'secret' }[] = [];
		for (const field of component.catalogEntry?.manifest?.config ?? []) {
			if (configurable.has(field.key) || field.key === '__url') continue;
			const policy = component.configuration?.find((candidate) => candidate.key === field.key);
			if (policy?.policy === 'prohibited') continue;
			const kind =
				policy?.secretBinding || field.secretBinding
					? 'secret'
					: policy?.policy === 'fixed'
						? 'fixed'
						: field.static
							? 'catalog'
							: undefined;
			if (kind) fields.push({ key: field.key, name: field.name || field.key, kind });
		}
		return fields;
	}

	const READONLY_LABELS = {
		fixed: () => m.vmcps_agent_setup_readonly_fixed(),
		catalog: () => m.vmcps_agent_setup_readonly_catalog(),
		secret: () => m.vmcps_agent_setup_readonly_secret()
	};

	// The keys a profile can set on a component: those set, and those still unset.
	function configurableKeys(
		configuration: VMCPProfileStatus['components'][number]['configuration']
	) {
		return [...configuration.set, ...configuration.missingRequired, ...configuration.optionalUnset];
	}

	// Drops what the administrator typed, cleared or started replacing.
	function clearEdits() {
		typed = {};
		clearing = {};
		replacing = {};
	}

	function oauthSignature(oauth?: VMCPProfileOAuthStatus) {
		return oauth ? `${oauth.state}|${oauth.authorizedAt ?? ''}|${oauth.authorizedBy ?? ''}` : '';
	}

	// What an administrator still has to do, each pointing at the field or button that does it.
	const todo = $derived.by(() => {
		const items: { text: string; target: ProfileSetupTarget }[] = [];
		for (const component of status?.components ?? []) {
			const name = componentName(component.componentID);
			for (const key of component.configuration.missingRequired) {
				items.push({
					text: m.vmcps_agent_setup_todo_field({
						component: name,
						field: fieldLabel(component.componentID, key)
					}),
					target: { componentID: component.componentID, field: key }
				});
			}
			const oauth = component.oauth;
			if (oauth?.state === 'invalid' || (oauth?.required && oauth.state === 'missing')) {
				items.push({
					text:
						oauth.state === 'invalid'
							? m.vmcps_agent_setup_todo_oauth_invalid({ component: name })
							: m.vmcps_agent_setup_todo_oauth({ component: name }),
					target: { componentID: component.componentID }
				});
			}
		}
		return items;
	});

	function reset() {
		stopFlow();
		status = undefined;
		loadError = '';
		blockedReason = '';
		clearEdits();
		saveError = '';
		oauthErrors = {};
		confirmReset = false;
		confirmOAuthReset = undefined;
		handledHash = '';
	}

	// Load the status of the saved profile, and again whenever the profile or vMCP changes.
	$effect(() => {
		const id = profileID;
		const vmcpIDNow = vmcpID;
		void reloadKey;
		untrack(reset);
		if (!id) return;
		const controller = new AbortController();
		UserService.getVMCPProfileStatus(vmcpIDNow, id, { signal: controller.signal })
			.then((loaded) => {
				if (!controller.signal.aborted) status = loaded;
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

	// A link from the agent page lands on a field or button of this section.
	$effect(() => {
		const hash = decodeURIComponent(page.url.hash.slice(1));
		if (!status || !hash || hash === handledHash) return;
		handledHash = hash;
		void tick().then(() => focusAnchor(hash));
	});

	function focusAnchor(id: string) {
		const element = document.getElementById(id);
		if (!element) return;
		element.scrollIntoView({ block: 'center' });
		if (element instanceof HTMLElement && !element.hasAttribute('disabled')) element.focus();
	}

	function focusTarget(target: ProfileSetupTarget) {
		// A set value shows no input until it is replaced, but a missing one always does.
		focusAnchor(profileSetupAnchor(target));
	}

	function startReplacing(id: string, key: string) {
		replacing[fieldId(id, key)] = true;
		delete clearing[fieldId(id, key)];
		void tick().then(() => focusAnchor(configFieldAnchor(id, key)));
	}

	function stopReplacing(id: string, key: string) {
		delete replacing[fieldId(id, key)];
		delete typed[fieldId(id, key)];
	}

	function clearKey(id: string, key: string) {
		clearing[fieldId(id, key)] = true;
		delete typed[fieldId(id, key)];
		delete replacing[fieldId(id, key)];
	}

	function undoClear(id: string, key: string) {
		delete clearing[fieldId(id, key)];
	}

	// An error that says the profile cannot be changed right now, whichever request found out.
	function noteBlocked(err: unknown) {
		const { status: code, message } = parseErrorContent(err);
		if (code === 409) blockedReason = message;
		return message;
	}

	async function saveConfiguration() {
		if (!profileID || !status || saving || !pendingChanges) return;
		const update: Record<string, Record<string, string | null>> = {};
		for (const component of status.components) {
			for (const key of configurableKeys(component.configuration)) {
				const id = fieldId(component.componentID, key);
				// A blank value would clear the key on the server, so only a typed value replaces it.
				const value = typed[id]?.trim() ? typed[id] : undefined;
				if (clearing[id]) (update[component.componentID] ??= {})[key] = null;
				else if (value !== undefined) (update[component.componentID] ??= {})[key] = value;
			}
		}
		saving = true;
		saveError = '';
		try {
			status = await UserService.updateVMCPProfileConfiguration(vmcpID, profileID, {
				components: update
			});
			// The typed values are dropped: from here on the keys only show as set.
			clearEdits();
			notifications.add(m.vmcps_agent_setup_saved(), 'success');
		} catch (err) {
			saveError = noteBlocked(err);
		} finally {
			saving = false;
		}
	}

	async function resetConfiguration() {
		if (!profileID || resetting) return;
		resetting = true;
		saveError = '';
		try {
			status = await UserService.resetVMCPProfileConfiguration(vmcpID, profileID);
			clearEdits();
			confirmReset = false;
			notifications.add(m.vmcps_agent_setup_reset_done(), 'success');
		} catch (err) {
			confirmReset = false;
			saveError = noteBlocked(err);
		} finally {
			resetting = false;
		}
	}

	// OAuth

	async function authorize(id: string) {
		if (!profileID || starting) return;
		starting = id;
		delete oauthErrors[id];
		stopFlow();
		try {
			const authorization = await UserService.authorizeVMCPProfileOAuth(vmcpID, profileID, id);
			beginFlow(id, authorization);
		} catch (err) {
			// The message is shown as returned: it can carry the placeholders that stand for a configured URL.
			oauthErrors[id] = {
				message: noteBlocked(err),
				status: getHttpStatusCode(err) ?? 0,
				fields: parseErrorReasons(err)
					.filter(
						(reason) =>
							reason.code === 'configuration_required' &&
							reason.field &&
							(!reason.componentID || reason.componentID === id)
					)
					.map((reason) => reason.field as string)
			};
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

	function beginFlow(id: string, authorization: VMCPProfileOAuthAuthorization) {
		const oauth = status?.components.find((component) => component.componentID === id)?.oauth;
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

	// Reads the status now while an authorization is pending, and resolves to whether it is over.
	let checkNow: (() => Promise<boolean>) | undefined;

	// McpLogin calls this when the login attempt has been used, which may have completed the login, and
	// to generate a new command. The status says first whether the grant changed, so a finished login
	// never shows as expired; otherwise a new command replaces the old one.
	async function finishLocalhostLogin(id: string) {
		if (await checkNow?.()) return;
		await authorize(id);
	}

	// While an authorization is pending, read the profile's status until the component's grant changes: that
	// is the only signal that the callback stored it, whichever window or machine completed it. The
	// callback page also tells this one (it shares an origin), which only makes the next read sooner.
	$effect(() => {
		const pending = flow;
		if (!pending || pending.expired || !profileID) return;
		const current: Flow = pending;
		const vmcpIDNow = untrack(() => vmcpID);
		const id = profileID;
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
				oauthErrors[current.componentID] = {
					message: m.vmcps_agent_setup_oauth_expired(),
					status: 0
				};
				return true;
			}
			try {
				const next = await UserService.getVMCPProfileStatus(vmcpIDNow, id, {
					signal: controller.signal
				});
				if (stopped) return true;
				status = next;
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
				// The vMCP or profile is gone: nothing more will change.
				if (getHttpStatusCode(err) === 404) {
					flow = undefined;
					oauthErrors[current.componentID] = {
						message: parseErrorContent(err).message,
						status: 404
					};
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

	async function resetOAuth() {
		if (!profileID || !confirmOAuthReset || resettingOAuth) return;
		const { id, name } = confirmOAuthReset;
		resettingOAuth = true;
		try {
			status = await UserService.resetVMCPProfileOAuth(vmcpID, profileID, id);
			notifications.add(m.vmcps_agent_setup_oauth_reset_done({ name }), 'success');
		} catch (err) {
			oauthErrors[id] = { message: noteBlocked(err), status: getHttpStatusCode(err) ?? 0 };
		} finally {
			resettingOAuth = false;
			confirmOAuthReset = undefined;
		}
	}

	function grantedBy(oauth: VMCPProfileOAuthStatus) {
		const by = oauth.authorizedBy ? (userName?.(oauth.authorizedBy) ?? oauth.authorizedBy) : '';
		const on = oauth.authorizedAt ? new Date(oauth.authorizedAt).toLocaleDateString() : '';
		if (by && on) return m.vmcps_agent_setup_oauth_authorized_by_on({ name: by, date: on });
		if (by) return m.vmcps_agent_setup_oauth_authorized_by({ name: by });
		return m.vmcps_agent_setup_oauth_authorized();
	}

	// The fields to set when an authorization is refused for lack of the component's URL: the keys the server
	// names, where the profile has them, and otherwise the first required key still missing.
	function fieldsToSet(id: string, named: string[] = []): ProfileSetupTarget[] {
		const configuration = status?.components.find(
			(component) => component.componentID === id
		)?.configuration;
		if (!configuration) return [];
		const known = new Set(configurableKeys(configuration));
		const keys = named.filter((key) => known.has(key));
		if (keys.length === 0 && configuration.missingRequired[0]) {
			keys.push(configuration.missingRequired[0]);
		}
		return keys.map((field) => ({ componentID: id, field }));
	}
</script>

<section
	id={AGENT_SETUP_ANCHOR}
	class="border-base-300 bg-base-100 dark:bg-base-300 mt-4 flex flex-col gap-4 rounded-xl border p-5 shadow-sm"
	aria-labelledby="agent-setup-title"
>
	<header class="flex flex-col gap-1">
		<h2 id="agent-setup-title" class="text-md flex items-center gap-2 font-semibold">
			<KeyRound class="size-4" />
			{m.vmcps_agent_setup_title()}
		</h2>
		<p class="text-muted-content text-sm font-light">{m.vmcps_agent_setup_description()}</p>
	</header>

	{#if profileID && status && !loadError && unsaved}
		<p class="notification-info text-sm" role="status">{m.vmcps_agent_setup_unsaved()}</p>
	{/if}
	{#if !profileID}
		<p class="notification-info text-sm" role="status">{m.vmcps_agent_setup_save_first()}</p>
	{:else if !status && !loadError}
		<Loading class="size-5" />
	{:else if loadError || !status}
		<div class="flex flex-col items-start gap-2">
			<p class="text-error text-sm" role="alert">
				{m.vmcps_agent_setup_load_failed()}
				{loadError}
			</p>
			<button type="button" class="btn btn-sm btn-secondary" onclick={retryLoad}>
				{m.common_try_again()}
			</button>
		</div>
	{:else if !status.agentCapable}
		<!-- The profile was saved as one no agent can use since it was opened: there is nothing to set up. -->
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
					{#each todo as item (`${item.target.componentID}/${item.target.field ?? ''}`)}
						<li>
							<button type="button" class="link text-left" onclick={() => focusTarget(item.target)}>
								{item.text}
							</button>
						</li>
					{/each}
				</ul>
			</div>
		{/if}

		{#if status.pendingCollection}
			<p class="notification-info text-sm" role="status">
				{m.vmcps_agent_setup_pending_collection()}
			</p>
		{/if}

		<div class="flex flex-col gap-4">
			{#each status.components as component (component.componentID)}
				{@const id = component.componentID}
				{@const vmcpComponent = components.find((candidate) => vmcpComponentId(candidate) === id)}
				{@const configuration = component.configuration}
				{@const configurable = new Set(configurableKeys(configuration))}
				{@const requiredKeys = [...configuration.missingRequired, ...configuration.set].filter(
					(key) => isRequired(id, key, configuration.missingRequired.includes(key))
				)}
				{@const optionalKeys = [...configuration.set, ...configuration.optionalUnset].filter(
					(key) => !isRequired(id, key, false)
				)}
				{@const fixed = vmcpComponent ? fixedFields(vmcpComponent, configurable) : []}
				<section
					class="border-base-300 dark:border-base-400 flex flex-col gap-3 rounded-lg border p-4"
					aria-label={componentName(id)}
				>
					<h3 class="flex items-center gap-2 text-sm font-semibold">
						{#if vmcpComponent?.catalogEntry?.manifest?.icon}
							<img src={vmcpComponent.catalogEntry.manifest.icon} alt="" class="size-4" />
						{/if}
						{componentName(id)}
					</h3>

					{#snippet fieldRow(key: string, required: boolean)}
						{@const fid = fieldId(id, key)}
						{@const isSet = configuration.set.includes(key)}
						{@const pendingClear = Boolean(clearing[fid])}
						{@const showInput = !isSet || Boolean(replacing[fid])}
						{@const config = fieldConfig(id, key)}
						{@const description = fieldDescription(id, key)}
						<div class="flex flex-col gap-1">
							<div class="flex flex-wrap items-center justify-between gap-2">
								<label class="text-sm" for={configFieldAnchor(id, key)}>
									{fieldLabel(id, key)}
									{#if required}
										<span class="text-error" aria-hidden="true">*</span>
										<span class="sr-only">{m.vmcps_agent_setup_required()}</span>
									{/if}
								</label>
								<span class="flex items-center gap-2">
									<span
										class={[
											'badge badge-sm',
											pendingClear
												? 'badge-warning badge-soft'
												: isSet
													? 'badge-success badge-soft'
													: required
														? 'badge-error badge-soft'
														: 'badge-ghost'
										]}
									>
										{pendingClear
											? m.vmcps_agent_setup_will_clear()
											: isSet
												? m.vmcps_agent_setup_set()
												: m.vmcps_agent_setup_not_set()}
									</span>
									{#if isSet && !replacing[fid] && !pendingClear}
										<button
											type="button"
											class="btn btn-xs btn-ghost"
											disabled={!editable}
											onclick={() => startReplacing(id, key)}
										>
											{m.vmcps_agent_setup_replace()}
										</button>
										<button
											type="button"
											class="btn btn-xs btn-ghost text-error"
											disabled={!editable}
											onclick={() => clearKey(id, key)}
										>
											{m.core_clear()}
										</button>
									{:else if pendingClear}
										<button
											type="button"
											class="btn btn-xs btn-ghost"
											onclick={() => undoClear(id, key)}
										>
											{m.vmcps_agent_setup_undo()}
										</button>
									{:else if isSet}
										<button
											type="button"
											class="btn btn-xs btn-ghost"
											onclick={() => stopReplacing(id, key)}
										>
											{m.common_cancel()}
										</button>
									{/if}
								</span>
							</div>
							{#if showInput && !pendingClear}
								{#if config?.options?.length}
									<select
										id={configFieldAnchor(id, key)}
										class="select select-sm w-full"
										disabled={!editable}
										bind:value={typed[fid]}
									>
										<option value="">{m.vmcps_agent_setup_choose()}</option>
										{#each config.options as option (option.value)}
											<option value={option.value}>{option.name}</option>
										{/each}
									</select>
								{:else}
									<input
										id={configFieldAnchor(id, key)}
										class="input-text-filled text-sm"
										type={config?.sensitive ? 'password' : 'text'}
										autocomplete="off"
										spellcheck="false"
										placeholder={isSet
											? m.vmcps_agent_setup_new_value()
											: m.vmcps_agent_setup_enter_value()}
										disabled={!editable}
										bind:value={typed[fid]}
									/>
								{/if}
							{/if}
							{#if description}
								<p class="text-muted-content text-xs font-light">{description}</p>
							{/if}
						</div>
					{/snippet}

					{#if requiredKeys.length > 0}
						<div class="flex flex-col gap-3">
							{#each requiredKeys as key (key)}
								{@render fieldRow(key, true)}
							{/each}
						</div>
					{/if}
					{#if optionalKeys.length > 0}
						<div class="flex flex-col gap-3">
							<h4 class="text-muted-content text-xs font-semibold uppercase">
								{m.vmcps_optional()}
							</h4>
							{#each optionalKeys as key (key)}
								{@render fieldRow(key, false)}
							{/each}
						</div>
					{/if}
					{#if fixed.length > 0}
						<div class="flex flex-col gap-1">
							<h4 class="text-muted-content text-xs font-semibold uppercase">
								{m.vmcps_agent_setup_readonly_heading()}
							</h4>
							<p class="text-muted-content text-xs font-light">
								{m.vmcps_agent_setup_readonly_note()}
							</p>
							<ul class="flex flex-col gap-0.5 text-sm">
								{#each fixed as field (field.key)}
									<li class="flex items-center justify-between gap-2">
										<span class="flex items-center gap-1.5">
											<Lock class="text-muted-content size-3" />
											{field.name}
										</span>
										<span class="badge badge-ghost badge-sm">{READONLY_LABELS[field.kind]()}</span>
									</li>
								{/each}
							</ul>
						</div>
					{/if}

					{#if component.oauth}
						{@const oauth = component.oauth}
						{@const error = oauthErrors[id]}
						{@const active = flow?.componentID === id ? flow : undefined}
						<div class="border-base-300 dark:border-base-400 flex flex-col gap-2 border-t pt-3">
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
										{:else}
											<span class="text-muted-content">{m.vmcps_agent_setup_oauth_missing()}</span>
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
									{#if oauth.state === 'invalid' && oauth.invalidReason}
										<p class="text-muted-content text-xs">{oauth.invalidReason}</p>
									{/if}
								</div>
								<span class="flex items-center gap-2">
									<button
										type="button"
										id={oauthAnchor(id)}
										class="btn btn-sm btn-primary"
										disabled={!editable || Boolean(starting) || Boolean(active)}
										onclick={() => authorize(id)}
									>
										{#if starting === id}<Loading class="size-4" />{/if}
										{oauth.state === 'missing'
											? m.vmcps_agent_setup_authorize()
											: m.vmcps_agent_setup_authorize_again()}
									</button>
									{#if oauth.state !== 'missing'}
										<button
											type="button"
											class="btn btn-sm btn-ghost text-error"
											disabled={deleting || resettingOAuth}
											onclick={() => (confirmOAuthReset = { id, name: componentName(id) })}
										>
											{m.vmcps_agent_setup_reset()}
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
								<div class="flex flex-col items-start gap-1" role="alert">
									<p class="text-error text-sm break-words">{error.message}</p>
									{#if error.status === 400}
										{#each fieldsToSet(id, error.fields) as target (target.field)}
											<button
												type="button"
												class="link text-sm"
												onclick={() => focusTarget(target)}
											>
												{m.vmcps_agent_setup_oauth_go_to_field({
													field: fieldLabel(id, target.field ?? '')
												})}
											</button>
										{/each}
									{/if}
								</div>
							{/if}
						</div>
					{/if}
				</section>
			{/each}
		</div>

		{#if saveError}
			<p class="text-error text-sm break-words" role="alert">{saveError}</p>
		{/if}
		<p class="text-muted-content text-xs font-light">{m.vmcps_agent_setup_write_only()}</p>
		<div class="flex flex-wrap items-center justify-between gap-2">
			<button
				type="button"
				class="btn btn-sm btn-ghost text-error"
				disabled={!editable ||
					resetting ||
					status.components.every((component) => component.configuration.set.length === 0)}
				onclick={() => (confirmReset = true)}
			>
				{m.vmcps_agent_setup_reset_config()}
			</button>
			<button
				type="button"
				class="btn btn-sm btn-primary"
				disabled={!editable || saving || !pendingChanges}
				onclick={saveConfiguration}
			>
				{#if saving}<Loading class="size-4" />{/if}
				{m.vmcps_agent_setup_save()}
			</button>
		</div>
	{/if}
</section>

<Confirm
	show={confirmReset}
	onsuccess={resetConfiguration}
	oncancel={() => (confirmReset = false)}
	loading={resetting}
	title={m.vmcps_agent_setup_reset_config_title()}
	msg={m.vmcps_agent_setup_reset_config_msg()}
	submitText={m.vmcps_agent_setup_reset()}
/>

<Confirm
	show={Boolean(confirmOAuthReset)}
	onsuccess={resetOAuth}
	oncancel={() => (confirmOAuthReset = undefined)}
	loading={resettingOAuth}
	title={m.vmcps_agent_setup_oauth_reset_title()}
	msg={m.vmcps_agent_setup_oauth_reset_msg({ name: confirmOAuthReset?.name ?? '' })}
	submitText={m.vmcps_agent_setup_reset()}
/>
