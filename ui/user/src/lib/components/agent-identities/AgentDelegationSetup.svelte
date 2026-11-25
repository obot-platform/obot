<script lang="ts">
	import Confirm from '$lib/components/Confirm.svelte';
	import CatalogConfigureForm, {
		type CompositeLaunchFormData
	} from '$lib/components/mcp/CatalogConfigureForm.svelte';
	import McpLocalAuth from '$lib/components/mcp/McpLocalAuth.svelte';
	import { isAbortError, parseErrorContent } from '$lib/errors';
	import { m } from '$lib/i18n';
	import Loading from '$lib/icons/Loading.svelte';
	import { navigateTo } from '$lib/navigation';
	import { AgentIdentitiesService } from '$lib/services';
	import { isWebURL } from '$lib/services/agent-identities';
	import type {
		AgentDelegationSetup,
		AgentDelegationSetupComponent,
		AgentDelegationSetupField,
		AgentDelegationSetupScope,
		AgentDelegationSetupVMCP,
		AgentReadinessReason
	} from '$lib/services/agent-identities/types';
	import type { MCPServerInfo } from '$lib/services/user/mcp';
	import { CircleCheck, CircleAlert, Settings } from '@lucide/svelte';
	import { untrack } from 'svelte';

	// What a user sets up for each vMCP an agent will use on their behalf: the values they enter and the
	// upstream accounts they connect, through the agent only. The same setup API serves the approval page
	// (scope "request") and the user's management page (scope "delegation"). Values are never shown, only
	// whether each is set.

	interface Props {
		scope: AgentDelegationSetupScope;
		// The provider's refusal of an account connection, from the page the callback returned to.
		oauthError?: string | null;
		// Called with the setup each time it changes, and with undefined while it can't be read.
		onchange?: (setup: AgentDelegationSetup | undefined) => void;
	}

	let { scope, oauthError, onchange }: Props = $props();

	let setup = $state<AgentDelegationSetup>();
	let loadError = $state('');
	let reloadKey = $state(0);
	// The action running, as "<vmcpID>/<componentID>/<action>", so one runs at a time.
	let busy = $state<string>();
	// Why an action on a vMCP failed, by vMCP ID.
	let errors = $state<Record<string, string>>({});
	// A component whose provider only redirects to localhost: its login runs through `obot mcp login`.
	let localLogin = $state<{ vmcpID: string; componentID: string; authURL: string }>();
	let confirmDisconnect = $state<{
		vmcp: AgentDelegationSetupVMCP;
		component: AgentDelegationSetupComponent;
	}>();

	let configDialog = $state<ReturnType<typeof CatalogConfigureForm>>();
	let configuring = $state<AgentDelegationSetupVMCP>();
	let configForm = $state<CompositeLaunchFormData>();
	let configError = $state('');

	const scopeKey = $derived(
		scope.kind === 'request' ? `r:${scope.requestId}` : `d:${scope.delegationId}`
	);
	const vmcps = $derived(setup?.vmcps ?? []);
	const readyCount = $derived(vmcps.filter((vmcp) => vmcp.ready).length);

	function update(next: AgentDelegationSetup | undefined) {
		setup = next;
		onchange?.(next);
	}

	function replaceVMCP(next: AgentDelegationSetupVMCP) {
		if (!setup) return;
		update({ vmcps: setup.vmcps.map((vmcp) => (vmcp.vmcpID === next.vmcpID ? next : vmcp)) });
	}

	$effect(() => {
		void scopeKey;
		void reloadKey;
		const current = untrack(() => scope);
		const controller = new AbortController();
		untrack(() => {
			loadError = '';
			update(undefined);
		});
		AgentIdentitiesService.getAgentDelegationSetup(current, { signal: controller.signal })
			.then((loaded) => {
				if (!controller.signal.aborted) update(loaded);
			})
			.catch((err) => {
				if (controller.signal.aborted || isAbortError(err)) return;
				loadError = parseErrorContent(err).message;
			});
		return () => controller.abort();
	});

	async function reload() {
		try {
			update(await AgentIdentitiesService.getAgentDelegationSetup(scope));
		} catch (err) {
			loadError = parseErrorContent(err).message;
		}
	}

	function vmcpName(vmcp: AgentDelegationSetupVMCP) {
		return vmcp.displayName || vmcp.vmcpID;
	}

	function componentName(component: AgentDelegationSetupComponent) {
		return component.name || component.componentID;
	}

	function fieldName(component: AgentDelegationSetupComponent, key?: string) {
		return component.configuration.find((field) => field.key === key)?.name || key || '';
	}

	function reasonText(component: AgentDelegationSetupComponent, reason: AgentReadinessReason) {
		switch (reason.code) {
			case 'configuration_required':
				return m.identity_access_delegation_setup_reason_configuration({
					field: fieldName(component, reason.field)
				});
			case 'oauth_required':
				return m.identity_access_delegation_setup_reason_oauth();
			case 'oauth_invalid':
				return m.identity_access_delegation_setup_reason_oauth_invalid();
		}
	}

	async function run(key: string, vmcp: AgentDelegationSetupVMCP, action: () => Promise<void>) {
		if (busy) return;
		busy = key;
		delete errors[vmcp.vmcpID];
		try {
			await action();
		} catch (err) {
			errors[vmcp.vmcpID] = parseErrorContent(err).message;
		} finally {
			busy = undefined;
		}
	}

	// Values

	function formField(field: AgentDelegationSetupField): NonNullable<MCPServerInfo['env']>[number] {
		return {
			key: field.key,
			name: field.name,
			// A value that is set is never shown, so leaving the field empty keeps it.
			description: field.set
				? [field.description, m.identity_access_delegation_setup_keep_value()]
						.filter(Boolean)
						.join(' ')
				: field.description,
			sensitive: field.sensitive,
			required: field.required && !field.set,
			options: field.options,
			file: field.usage === 'file' || field.usage === 'dynamicFile',
			value: ''
		};
	}

	function configurable(vmcp: AgentDelegationSetupVMCP) {
		return vmcp.components.filter(
			(component) => !component.covered && component.configuration.length > 0
		);
	}

	function openConfiguration(vmcp: AgentDelegationSetupVMCP) {
		configuring = vmcp;
		configError = '';
		configForm = {
			componentConfigs: Object.fromEntries(
				configurable(vmcp).map((component) => [
					component.componentID,
					{
						name: componentName(component),
						envs: component.configuration
							.filter((field) => field.usage !== 'header')
							.map(formField),
						headers: component.configuration
							.filter((field) => field.usage === 'header')
							.map(formField)
					}
				])
			)
		};
		configDialog?.open();
	}

	async function saveConfiguration() {
		const vmcp = configuring;
		if (!vmcp || !configForm) return;
		// Only what the user entered is sent: an empty field keeps the value that is set, and clearing a
		// value is its own action.
		const components: Record<string, Record<string, string>> = {};
		for (const [componentID, component] of Object.entries(configForm.componentConfigs)) {
			for (const field of [...(component.envs ?? []), ...(component.headers ?? [])]) {
				if (!field.value) continue;
				components[componentID] ??= {};
				components[componentID][field.key] = field.value;
			}
		}
		if (Object.keys(components).length === 0) {
			closeConfiguration();
			return;
		}
		busy = `${vmcp.vmcpID}//configure`;
		configError = '';
		try {
			replaceVMCP(
				await AgentIdentitiesService.configureAgentDelegationSetup(scope, vmcp.vmcpID, {
					components
				})
			);
			closeConfiguration();
		} catch (err) {
			configError = parseErrorContent(err).message;
		} finally {
			busy = undefined;
		}
	}

	function closeConfiguration() {
		configDialog?.close();
		configuring = undefined;
		configForm = undefined;
		configError = '';
	}

	function clearValue(
		vmcp: AgentDelegationSetupVMCP,
		component: AgentDelegationSetupComponent,
		field: AgentDelegationSetupField
	) {
		return run(`${vmcp.vmcpID}/${component.componentID}/clear:${field.key}`, vmcp, async () => {
			replaceVMCP(
				await AgentIdentitiesService.configureAgentDelegationSetup(scope, vmcp.vmcpID, {
					components: { [component.componentID]: { [field.key]: null } }
				})
			);
		});
	}

	// Accounts

	function connect(vmcp: AgentDelegationSetupVMCP, component: AgentDelegationSetupComponent) {
		return run(`${vmcp.vmcpID}/${component.componentID}/connect`, vmcp, async () => {
			const authorization = await AgentIdentitiesService.startAgentDelegationSetupOAuth(
				scope,
				vmcp.vmcpID,
				component.componentID
			);
			if (authorization.localhostCallback) {
				localLogin = {
					vmcpID: vmcp.vmcpID,
					componentID: component.componentID,
					authURL: authorization.authURL
				};
				return;
			}
			// The provider's callback brings the browser back to this page. Only a web page is opened,
			// whatever the server said.
			if (!isWebURL(authorization.authURL)) {
				throw new Error(m.identity_access_delegation_setup_connect_failed());
			}
			navigateTo(authorization.authURL);
		});
	}

	async function finishLocalLogin() {
		await reload();
		const login = localLogin;
		const component = setup?.vmcps
			.find((vmcp) => vmcp.vmcpID === login?.vmcpID)
			?.components.find((candidate) => candidate.componentID === login?.componentID);
		if (component?.oauth === 'connected') localLogin = undefined;
	}

	async function disconnect() {
		const target = confirmDisconnect;
		if (!target) return;
		const { vmcp, component } = target;
		await run(`${vmcp.vmcpID}/${component.componentID}/disconnect`, vmcp, async () => {
			replaceVMCP(
				await AgentIdentitiesService.disconnectAgentDelegationSetupOAuth(
					scope,
					vmcp.vmcpID,
					component.componentID
				)
			);
		});
		confirmDisconnect = undefined;
	}
</script>

<div class="flex flex-col gap-4">
	{#if oauthError}
		<p class="notification-error flex items-center gap-2 text-sm" role="alert">
			<CircleAlert class="size-4 shrink-0" />
			{oauthError === 'access_denied'
				? m.identity_access_delegation_setup_oauth_denied()
				: m.identity_access_delegation_setup_oauth_failed()}
		</p>
	{/if}

	{#if loadError}
		<div class="flex flex-col items-start gap-2">
			<p class="text-error text-sm" role="alert">
				{m.identity_access_delegation_setup_load_failed()}
				{loadError}
			</p>
			<button type="button" class="btn btn-sm btn-secondary" onclick={() => reloadKey++}>
				{m.common_try_again()}
			</button>
		</div>
	{:else if !setup}
		<div class="flex justify-center py-4"><Loading class="size-6" /></div>
	{:else if vmcps.length === 0}
		<p class="text-muted-content text-sm" role="status">
			{m.identity_access_delegation_setup_none()}
		</p>
	{:else}
		<p class="flex items-center gap-2 text-sm font-medium" role="status">
			{#if readyCount === vmcps.length}
				<CircleCheck class="text-success size-4" />
			{/if}
			{m.identity_access_delegation_setup_progress({ ready: readyCount, total: vmcps.length })}
		</p>

		{#each vmcps as vmcp (vmcp.vmcpID)}
			{@const covered = vmcp.components.filter((component) => component.covered)}
			{@const uncovered = vmcp.components.filter((component) => !component.covered)}
			<section
				class="border-base-300 dark:border-base-400 flex flex-col gap-3 rounded-lg border p-4"
				aria-label={vmcpName(vmcp)}
			>
				<header class="flex flex-wrap items-center justify-between gap-2">
					<h3 class="text-sm font-semibold">{vmcpName(vmcp)}</h3>
					{#if vmcp.ready}
						<span class="badge badge-success badge-sm">
							{m.identity_access_delegation_setup_ready()}
						</span>
					{:else}
						<span class="badge badge-warning badge-sm">
							{m.identity_access_delegation_setup_needs_setup()}
						</span>
					{/if}
				</header>

				{#if covered.length > 0}
					<p class="text-muted-content flex items-start gap-2 text-sm">
						<CircleCheck class="text-success mt-0.5 size-4 shrink-0" />
						{m.identity_access_delegation_setup_covered({
							components: covered.map(componentName).join(', ')
						})}
					</p>
				{/if}

				{#each uncovered as component (component.componentID)}
					{@const id = component.componentID}
					{@const key = `${vmcp.vmcpID}/${id}`}
					<div
						role="group"
						aria-label={componentName(component)}
						class="bg-base-100 dark:bg-base-200 flex flex-col gap-2 rounded-md p-3"
					>
						<div class="flex flex-wrap items-center justify-between gap-2">
							<span class="text-sm font-medium">{componentName(component)}</span>
							{#if component.ready}
								<span class="text-success text-xs">
									{m.identity_access_delegation_setup_ready()}
								</span>
							{:else}
								<span class="text-warning text-xs">
									{m.identity_access_delegation_setup_needs_setup()}
								</span>
							{/if}
						</div>

						{#if component.reasons?.length}
							<ul class="text-muted-content flex flex-col gap-0.5 text-xs">
								{#each component.reasons as reason, i (i)}
									<li>{reasonText(component, reason)}</li>
								{/each}
							</ul>
						{/if}

						{#if component.configuration.length > 0}
							<ul class="flex flex-col gap-1">
								{#each component.configuration as field (field.key)}
									<li class="flex flex-wrap items-center justify-between gap-2 text-sm">
										<span>
											{field.name || field.key}
											{#if !field.required}
												<span class="text-muted-content">{m.mcps_optional()}</span>
											{/if}
										</span>
										<span class="flex items-center gap-2">
											{#if field.set}
												<span class="badge badge-ghost badge-sm">
													{m.identity_access_delegation_setup_value_set()}
												</span>
												<button
													type="button"
													class="btn btn-ghost btn-xs"
													aria-label={m.identity_access_delegation_setup_clear_named({
														name: field.name || field.key
													})}
													disabled={Boolean(busy)}
													onclick={() => clearValue(vmcp, component, field)}
												>
													{m.identity_access_delegation_setup_clear()}
												</button>
											{:else}
												<span class="badge badge-outline badge-sm">
													{m.identity_access_delegation_setup_value_unset()}
												</span>
											{/if}
										</span>
									</li>
								{/each}
							</ul>
						{/if}

						{#if component.oauth !== 'notRequired'}
							<div class="flex flex-wrap items-center justify-between gap-2 text-sm">
								{#if component.oauth === 'preconfigured'}
									<span class="text-muted-content">
										{m.identity_access_delegation_setup_oauth_preconfigured()}
									</span>
								{:else}
									<span class={component.oauth === 'connected' ? 'text-success' : ''}>
										{component.oauth === 'connected'
											? m.identity_access_delegation_setup_oauth_connected()
											: m.identity_access_delegation_setup_oauth_not_connected()}
									</span>
									<span class="flex items-center gap-2">
										<button
											type="button"
											class="btn btn-sm {component.oauth === 'connected'
												? 'btn-secondary'
												: 'btn-primary'}"
											disabled={Boolean(busy)}
											onclick={() => connect(vmcp, component)}
										>
											{#if busy === `${key}/connect`}<Loading class="size-4" />{/if}
											{component.oauth === 'connected'
												? m.identity_access_delegation_setup_reconnect()
												: m.identity_access_delegation_setup_connect()}
										</button>
										{#if component.oauth === 'connected'}
											<button
												type="button"
												class="btn btn-sm btn-ghost text-error"
												disabled={Boolean(busy)}
												onclick={() => (confirmDisconnect = { vmcp, component })}
											>
												{m.identity_access_delegation_setup_disconnect()}
											</button>
										{/if}
									</span>
								{/if}
							</div>
						{/if}

						{#if localLogin?.vmcpID === vmcp.vmcpID && localLogin.componentID === id}
							<div class="border-base-300 rounded-md border">
								<McpLocalAuth
									url={localLogin.authURL}
									serverName={componentName(component)}
									onComplete={finishLocalLogin}
								/>
							</div>
						{/if}
					</div>
				{/each}

				{#if configurable(vmcp).length > 0}
					<div>
						<button
							type="button"
							class="btn btn-sm btn-secondary flex items-center gap-2"
							disabled={Boolean(busy)}
							onclick={() => openConfiguration(vmcp)}
						>
							<Settings class="size-4" />
							{m.identity_access_delegation_setup_set_values()}
						</button>
					</div>
				{/if}

				{#if errors[vmcp.vmcpID]}
					<p class="text-error text-sm break-words" role="alert">{errors[vmcp.vmcpID]}</p>
				{/if}
			</section>
		{/each}
	{/if}
</div>

<CatalogConfigureForm
	bind:this={configDialog}
	bind:form={configForm}
	name={configuring ? vmcpName(configuring) : ''}
	onSave={saveConfiguration}
	onCancel={closeConfiguration}
	onClose={closeConfiguration}
	loading={busy === `${configuring?.vmcpID}//configure`}
	error={configError}
	showComponentToggle={false}
	displayDescriptionInline
	cancelText={m.core_close()}
	submitText={m.core_save()}
/>

<Confirm
	show={Boolean(confirmDisconnect)}
	type="info"
	title={m.identity_access_delegation_setup_disconnect_title()}
	msg={m.identity_access_delegation_setup_disconnect_msg({
		name: confirmDisconnect ? componentName(confirmDisconnect.component) : ''
	})}
	note={m.identity_access_delegation_setup_disconnect_note()}
	submitText={m.identity_access_delegation_setup_disconnect()}
	loading={Boolean(busy)}
	onsuccess={disconnect}
	oncancel={() => (confirmDisconnect = undefined)}
/>
