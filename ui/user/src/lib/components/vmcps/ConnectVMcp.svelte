<script lang="ts">
	import { resolve } from '$app/paths';
	import { dialogAnimation } from '$lib/actions/dialogAnimation';
	import Confirm from '$lib/components/Confirm.svelte';
	import CopyField from '$lib/components/CopyField.svelte';
	import ResponsiveDialog from '$lib/components/ResponsiveDialog.svelte';
	import CatalogConfigureForm, {
		type CompositeLaunchFormData
	} from '$lib/components/mcp/CatalogConfigureForm.svelte';
	import HowToConnect from '$lib/components/mcp/HowToConnect.svelte';
	import IconButton from '$lib/components/primitives/IconButton.svelte';
	import { isAbortError } from '$lib/errors';
	import { m } from '$lib/i18n';
	import { UserService, type VMCP, type VMCPConfiguration, type VMCPInstance } from '$lib/services';
	import type { VMcpConnectOptions } from '$lib/services/vmcps/types';
	import {
		resolveVMcpComponents,
		vmcpComponentId,
		vmcpConnectURL,
		vmcpMissingStaticOAuthComponent,
		vmcpInstanceNeedsUserConfiguration
	} from '$lib/services/vmcps/utils';
	import { profile, vmcpInstances } from '$lib/stores';
	import { goto } from '$lib/url';
	import VMcpIcon from './VMcpIcon.svelte';
	import { CircleAlert, MessageCircle, X } from '@lucide/svelte';
	import { onMount } from 'svelte';
	import { fade } from 'svelte/transition';
	import { twMerge } from 'tailwind-merge';

	let vmcp = $state<VMCP>();
	let instance = $state<VMCPInstance>();
	let connectDialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let configureDialog = $state<ReturnType<typeof CatalogConfigureForm>>();
	let howToConnect = $state<ReturnType<typeof HowToConnect>>();
	let connectionUrlField = $state<ReturnType<typeof CopyField>>();
	let configureForm = $state<CompositeLaunchFormData>();
	let saving = $state(false);
	let error = $state<string>();
	let showIntroDialog = $state(false);
	let launchError = $state<string>();
	let launchProgress = $state<number>(0);
	let launchState = $state<'relaunching' | 'launching' | undefined>();
	let oauthDialog = $state<HTMLDialogElement>();
	let oauthURL = $state<string>('');
	let oauthVerifying = $state(false);
	let onConnected = $state<VMcpConnectOptions['onConnected']>();
	let onDismissed = $state<VMcpConnectOptions['onDismissed']>();
	let ignoreNextConfigureClose = false;
	let skipConnectDialog = false;
	let editConfigurationController: AbortController | undefined;

	let connectURL = $derived(vmcp ? vmcpConnectURL(vmcp) : undefined);
	let displayName = $derived(vmcp?.displayName || 'vMCP');
	let componentViews = $derived(vmcp ? resolveVMcpComponents(vmcp) : []);
	let missingOAuthComponent = $derived(vmcp ? vmcpMissingStaticOAuthComponent(vmcp) : undefined);
	let hasUserConfiguration = $derived(
		vmcp?.components?.some((component) =>
			component.configuration?.find((field) => field.policy === 'userAllowed')
		)
	);
	let isReauthenticatable = $derived(
		Boolean(instance) &&
			!hasUserConfiguration &&
			Boolean(
				vmcp?.components?.some(
					(component) =>
						component.catalogEntry?.manifest?.runtime === 'remote' ||
						Boolean(component.oauthCredentialID)
				)
			)
	);
	let hasConfiguredInstance = $derived.by(() => {
		if (!vmcp) return false;
		const candidates = instance ? [instance] : vmcpInstances.current.items;
		return candidates.some(
			(candidate) =>
				candidate.vmcpID === vmcp?.id &&
				candidate.userID === profile.current.id &&
				!candidate.deleted &&
				candidate.status?.configured === true
		);
	});

	function generateIdFromName(name: string) {
		return name
			.toLowerCase()
			.replace(/ /g, '-')
			.replace(/[^a-z0-9-_]/g, '');
	}

	function resetDialogState(
		target: VMCP,
		targetInstance?: VMCPInstance,
		options?: VMcpConnectOptions
	) {
		vmcp = target;
		instance = targetInstance;
		onConnected = options?.onConnected;
		onDismissed = options?.onDismissed;
		configureForm = undefined;
		error = undefined;
		launchError = undefined;
		launchProgress = 0;
		launchState = undefined;
		oauthURL = '';
		oauthVerifying = false;
		showIntroDialog = false;
		ignoreNextConfigureClose = false;
		connectionUrlField?.clear?.();
		howToConnect?.resetCopied?.();
	}

	export function open(target: VMCP, targetInstance?: VMCPInstance, options?: VMcpConnectOptions) {
		resetDialogState(target, targetInstance, options);
		skipConnectDialog = Boolean(options?.onConnected);

		if (options?.onConnected) {
			initLaunch();
		} else {
			connectDialog?.open();
		}
	}

	export async function openEditConfiguration(
		target: VMCP,
		targetInstance: VMCPInstance,
		options?: VMcpConnectOptions
	) {
		editConfigurationController?.abort();
		const controller = new AbortController();
		editConfigurationController = controller;

		let resolved: VMCP;
		try {
			resolved = await UserService.getVMCP(target.id, {
				dontLogErrors: true,
				signal: controller.signal
			});
		} catch (err) {
			if (controller.signal.aborted || isAbortError(err)) return;
			resolved = target;
		}

		if (controller.signal.aborted || editConfigurationController !== controller) return;

		resetDialogState(resolved, targetInstance, options);
		skipConnectDialog = true;
		connectDialog?.close();
		await initConfigureForm();

		if (editConfigurationController === controller) editConfigurationController = undefined;
	}

	function handleConfigure() {
		if (missingOAuthComponent) return;
		showIntroDialog = false;
		ensureOauthVisibilityListener();
		if (hasUserConfiguration) {
			void initConfigureForm();
		} else {
			void launchWithoutConfiguration();
		}
	}

	async function launchWithoutConfiguration() {
		configureForm = undefined;
		initUpdatingOrLaunchProgress();
		await configureDialog?.open();
		if (launchState !== 'launching') return;
		await saveConfiguration();
	}

	function initLaunch() {
		showIntroDialog = true;
		connectDialog?.close();
	}

	function goToTester() {
		if (!vmcp) return;
		connectDialog?.close();
		goto(`/vmcps/${vmcp.id}?view=inspector`);
	}

	function handleTest() {
		if (hasConfiguredInstance) {
			goToTester();
			return;
		}
		onConnected = goToTester;
		skipConnectDialog = true;
		initLaunch();
	}

	async function initConfigureForm() {
		if (!vmcp) return;
		connectDialog?.close();
		const revealed = await revealedInstanceConfiguration();
		const componentConfigs: CompositeLaunchFormData['componentConfigs'] = {};
		for (const component of vmcp.components ?? []) {
			const id = vmcpComponentId(component);
			if (!id) continue;
			const allowed = (component.configuration ?? []).filter(
				(field) => field.policy === 'userAllowed'
			);
			if (allowed.length === 0) continue;

			const manifestFields = new Map(
				(component.catalogEntry?.manifest?.config ?? []).map((field) => [field.key, field])
			);
			const fields = allowed.map((policy) => {
				const field = manifestFields.get(policy.key);
				return {
					key: policy.key,
					name: field?.name || policy.key,
					description: field?.description || '',
					required: field?.required ?? false,
					sensitive: field?.sensitive ?? false,
					options: field?.options,
					usage: field?.usage ?? 'env',
					value: revealed[id]?.[policy.key] ?? '',
					isStatic: false,
					file: field?.usage === 'file' || field?.usage === 'dynamicFile',
					dynamicFile: field?.usage === 'dynamicFile',
					interpolated: field?.usage === 'interpolated'
				};
			});
			componentConfigs[id] = {
				name: component.name || component.catalogEntry?.manifest?.name || id,
				icon: component.catalogEntry?.manifest?.icon,
				disabled: false,
				envs: fields.filter((field) => field.usage !== 'header'),
				headers: fields.filter((field) => field.usage === 'header')
			};
		}
		if (Object.keys(componentConfigs).length === 0) {
			await launchWithoutConfiguration();
			return;
		}
		configureForm = { componentConfigs };
		error = undefined;
		await configureDialog?.open();
	}

	async function revealedInstanceConfiguration() {
		if (!instance) return {};
		try {
			const revealed = await UserService.revealVMCPInstance(instance.id, { dontLogErrors: true });
			return revealed.components ?? {};
		} catch {
			return {};
		}
	}

	function configurationPayload(form: CompositeLaunchFormData): VMCPConfiguration {
		const components: VMCPConfiguration['components'] = {};
		for (const [componentID, component] of Object.entries(form.componentConfigs)) {
			components[componentID] = {};
			for (const field of [...(component.envs ?? []), ...(component.headers ?? [])]) {
				components[componentID][field.key] = field.value ?? '';
			}
		}
		return { components };
	}

	function configuredInstance(submitted: VMCPInstance): VMCPInstance {
		return {
			...submitted,
			status: {
				...submitted.status,
				configured: true,
				...(vmcpInstanceNeedsUserConfiguration(submitted)
					? { missingRequiredConfiguration: [] }
					: {})
			}
		};
	}

	async function refreshConfiguredInstance(submitted: VMCPInstance): Promise<VMCPInstance> {
		const optimistic = configuredInstance(submitted);
		vmcpInstances.upsert(optimistic);
		if (!vmcpInstanceNeedsUserConfiguration(submitted)) {
			return optimistic;
		}

		for (let attempt = 0; attempt < 8; attempt++) {
			if (attempt > 0) {
				await new Promise((resolve) => setTimeout(resolve, 250));
			}
			try {
				const latest = await UserService.getVMCPInstance(submitted.id, { dontLogErrors: true });
				if (!vmcpInstanceNeedsUserConfiguration(latest)) {
					vmcpInstances.upsert(latest);
					return latest;
				}
			} catch {
				// Status is reconciled asynchronously after configure.
			}
		}
		return optimistic;
	}

	function initUpdatingOrLaunchProgress() {
		launchError = undefined;
		launchProgress = 0;
		launchState = 'launching';

		const timeout1 = setTimeout(() => {
			launchProgress = 10;
		}, 100);

		const timeout2 = setTimeout(() => {
			launchProgress = 30;
		}, 3000);

		const timeout3 = setTimeout(() => {
			launchProgress = 80;
		}, 10000);

		return { timeout1, timeout2, timeout3 };
	}

	function takeConnectCallbacks() {
		const connected = onConnected;
		const dismissed = onDismissed;
		onConnected = undefined;
		onDismissed = undefined;
		return { connected, dismissed };
	}

	function dismissConnect() {
		takeConnectCallbacks().dismissed?.();
	}

	function closeConfigureWithoutDismissing() {
		ignoreNextConfigureClose = true;
		configureDialog?.close();
	}

	function finishLaunch() {
		const { connected } = takeConnectCallbacks();
		closeConfigureWithoutDismissing();
		if (connected) {
			connected();
			return;
		}
		if (skipConnectDialog) return;
		connectDialog?.open();
	}

	async function getOauthURL() {
		if (!vmcp) return '';
		return (await UserService.getMcpServerOauthURL(vmcp.id)) || '';
	}

	async function handleOauthVisibilityChange() {
		if (!oauthURL && !oauthVerifying) return;
		if (document.visibilityState === 'visible') {
			oauthURL = await getOauthURL();
			if (!oauthURL) {
				oauthDialog?.close();
				finishLaunch();
			}
			oauthVerifying = false;
		}
	}

	function ensureOauthVisibilityListener() {
		document.removeEventListener('visibilitychange', handleOauthVisibilityChange);
		document.addEventListener('visibilitychange', handleOauthVisibilityChange);
	}

	async function verifyOauthOrConnect() {
		oauthVerifying = false;
		oauthURL = await getOauthURL();
		launchProgress = 100;
		await new Promise((resolve) => setTimeout(resolve, 1000));
		launchState = undefined;
		launchProgress = 0;
		if (oauthURL) {
			closeConfigureWithoutDismissing();
			oauthDialog?.showModal();
		} else {
			finishLaunch();
		}
	}

	function handleOauthClose() {
		oauthDialog?.close();
		oauthURL = '';
		finishLaunch();
	}

	export async function authenticate() {
		if (!vmcp) return;
		ensureOauthVisibilityListener();
		oauthVerifying = false;
		oauthURL = await getOauthURL();
		if (oauthURL) {
			oauthDialog?.showModal();
		} else {
			finishLaunch();
		}
	}

	async function reauthenticate() {
		if (!vmcp) return;
		await UserService.clearMcpServerOAuth(vmcp.id);
		await authenticate();
	}

	async function saveConfiguration() {
		const target = vmcp;
		if (!target || saving || missingOAuthComponent) return;
		saving = true;
		error = undefined;
		const { timeout1, timeout2, timeout3 } = initUpdatingOrLaunchProgress();
		try {
			const targetInstance = instance ?? (await UserService.createVMCPInstance(target.id));
			if (configureForm) {
				instance = await refreshConfiguredInstance(
					await UserService.configureVMCPInstance(
						targetInstance.id,
						configurationPayload(configureForm)
					)
				);
			} else {
				instance = targetInstance;
				vmcpInstances.upsert(instance);
			}

			const launchResponse = await UserService.validateSingleOrRemoteMcpServerLaunched(target.id);
			if (!launchResponse.success) {
				launchError = launchResponse.message ?? m.vmcps_failed_to_launch();
				return;
			}

			await verifyOauthOrConnect();
		} catch (err) {
			launchError = err instanceof Error ? err.message : m.vmcps_failed_to_launch();
		} finally {
			clearTimeout(timeout1);
			clearTimeout(timeout2);
			clearTimeout(timeout3);
			saving = false;
		}
	}

	onMount(() => {
		ensureOauthVisibilityListener();
		return () => {
			document.removeEventListener('visibilitychange', handleOauthVisibilityChange);
		};
	});
</script>

{#snippet dialogTitle()}
	<VMcpIcon components={componentViews} />
	{displayName}
{/snippet}

{#snippet oauthSetupGuidance()}
	{#if missingOAuthComponent}
		<p>
			{m.vmcps_connect_requires_oauth_setup({ name: missingOAuthComponent.name })}
		</p>
		{#if profile.current.isAdmin?.()}
			<a
				class="btn btn-primary"
				href={resolve(
					`/mcp-servers/c/${encodeURIComponent(missingOAuthComponent.mcpServerCatalogEntryID)}?configure-oauth=true`
				)}>{m.vmcps_configure_named_oauth({ name: missingOAuthComponent.name })}</a
			>
		{:else}
			<p>{m.vmcps_ask_admin_configure_oauth()}</p>
		{/if}
	{/if}
{/snippet}

<ResponsiveDialog
	bind:this={connectDialog}
	animate="slide"
	id="connect-to-vmcp-dialog"
	onClose={() => {
		howToConnect?.resetCopied();
		connectionUrlField?.clear();
	}}
>
	{#snippet titleContent()}
		<div class="flex items-center gap-2">
			{@render dialogTitle()}
		</div>
	{/snippet}

	{#if connectURL}
		<div id="connection-url-container" class="flex items-end gap-2 md:p-0 pb-0 p-4">
			<div class="min-w-0 grow">
				<CopyField
					bind:this={connectionUrlField}
					value={connectURL}
					id="connectURL"
					label={m.vmcps_connection_url()}
				/>
			</div>
			<button
				type="button"
				aria-label={m.vmcps_test_vmcp()}
				class="btn btn-primary"
				onclick={handleTest}
			>
				<MessageCircle class="size-4" />
				{m.vmcps_test_vmcp()}
			</button>
		</div>
		<HowToConnect
			bind:this={howToConnect}
			url={connectURL}
			id={generateIdFromName(displayName)}
			{displayName}
			onLaunch={!instance ? initLaunch : undefined}
			onEdit={instance && hasUserConfiguration ? initConfigureForm : undefined}
			onReauthenticate={isReauthenticatable
				? () => {
						connectDialog?.close();
						void reauthenticate();
					}
				: undefined}
		/>
	{:else if missingOAuthComponent}
		<div class="flex flex-col items-start gap-3 md:p-0 p-4 text-sm">
			{@render oauthSetupGuidance()}
		</div>
	{:else}
		<div class="flex flex-col items-start gap-3 md:p-0 p-4">
			<p class="text-sm text-muted-content font-light">
				{m.vmcps_not_ready_complete_setup()}
			</p>
			<button class="btn btn-primary btn-sm" onclick={initLaunch}
				>{m.vmcps_preconfigure_server()}</button
			>
		</div>
	{/if}
</ResponsiveDialog>

<CatalogConfigureForm
	bind:this={configureDialog}
	bind:form={configureForm}
	name={displayName}
	onSave={saveConfiguration}
	onClose={() => {
		if (ignoreNextConfigureClose) {
			ignoreNextConfigureClose = false;
			return;
		}
		dismissConnect();
	}}
	submitText={instance ? m.core_update() : m.vmcps_configure()}
	loading={saving || launchState === 'launching'}
	{error}
	isNew={false}
	showComponentToggle={false}
	configurationTitle={m.vmcps_user_specific_configuration()}
>
	{#snippet icon()}
		<VMcpIcon components={componentViews} />
	{/snippet}
	{#snippet loadingContent()}
		<div in:fade class="h-full w-full flex items-center justify-center">
			{#if launchError}
				<div class="flex flex-col gap-2 w-full h-full" in:fade>
					<div class="notification-error">
						<div class="flex items-center gap-2">
							<CircleAlert class="size-5 text-error" />
							<h4 class="text-md font-medium">{m.vmcps_launch_failed()}</h4>
						</div>

						<div class="text-xs mt-2">
							{m.vmcps_launch_issue()}

							<ul class="list-disc px-4 py-1 space-y-1">
								{#if hasUserConfiguration}
									<li>{m.vmcps_launch_verify_configurations()}</li>
								{/if}
								<li>{m.vmcps_launch_contact_support()}</li>
							</ul>
						</div>
					</div>
					<p class="text-sm self-start">{launchError}</p>
					<div class="flex w-full flex-col items-center gap-2 md:flex-row mt-2">
						{#if hasUserConfiguration}
							<button
								class="btn btn-primary w-full md:w-1/2 md:flex-1"
								onclick={() => {
									launchState = 'relaunching';
									launchError = undefined;
									launchProgress = 0;
									saving = false;
								}}
							>
								{m.vmcps_update_configuration_and_retry()}
							</button>
						{/if}
						<button
							class="btn btn-secondary w-full md:w-1/2 md:flex-1"
							onclick={() => {
								launchState = undefined;
								launchError = undefined;
								launchProgress = 0;
								saving = false;
								configureDialog?.close();
								if (skipConnectDialog) {
									dismissConnect();
									return;
								}
								if (vmcp) connectDialog?.open();
							}}
						>
							{m.core_close()}
						</button>
					</div>
				</div>
			{:else}
				<div class="flex flex-col gap-1 mb-4">
					<div class="w-full text-xl font-extralight text-center">
						{Math.round(launchProgress ?? 0)}%
					</div>

					<div class="bg-base-400 h-3 w-full overflow-hidden rounded-full">
						<div
							class={twMerge('bg-primary h-full rounded-full transition-all duration-500 ease-out')}
							style="width: {launchProgress ?? 0}%"
						></div>
					</div>

					<div class="flex w-md flex-col justify-center gap-2 text-center">
						<p class="text-xs font-light">{m.vmcps_launching_vmcp()}</p>
					</div>
				</div>
			{/if}
		</div>
	{/snippet}
</CatalogConfigureForm>

<Confirm
	show={showIntroDialog}
	onsuccess={handleConfigure}
	submitText={m.core_continue()}
	disabled={Boolean(missingOAuthComponent)}
	type="info"
	title={m.vmcps_connect_to_server()}
	oncancel={() => {
		showIntroDialog = false;
		dismissConnect();
	}}
	hideCancelButton
>
	{#snippet msgContent()}
		<div class="flex items-center gap-2 text-lg font-semibold mb-2">
			{@render dialogTitle()}
		</div>
	{/snippet}
	{#snippet note()}
		{#if missingOAuthComponent}
			{@render oauthSetupGuidance()}
		{:else}<p>
				{m.vmcps_initial_setup_begin()}
				{#if hasUserConfiguration}
					{m.vmcps_initial_setup_additional_config()}
				{:else}
					<br />{m.vmcps_click_below_to_begin()}
				{/if}
			</p>{/if}
	{/snippet}
</Confirm>

<dialog bind:this={oauthDialog} class="dialog" use:dialogAnimation={{ type: 'slide' }}>
	<div class="dialog-container md:w-sm">
		<div class="flex flex-col gap-4 p-4">
			{#if oauthURL}
				<div class="absolute top-2 right-2">
					<IconButton onclick={handleOauthClose}>
						<X class="size-4" />
					</IconButton>
				</div>
				<div class="flex items-center gap-2">
					<VMcpIcon components={componentViews} />
					<h3 class="text-lg leading-5.5 font-semibold">
						{displayName}
					</h3>
				</div>

				<p>
					{m.vmcps_oauth_required_named({ name: displayName })}
				</p>

				<p>{m.vmcps_click_link_to_authenticate()}</p>

				<a
					href={oauthURL}
					rel="external noopener noreferrer"
					target="_blank"
					class="btn btn-primary text-center text-sm outline-none"
					onclick={() => {
						oauthVerifying = true;
					}}
				>
					{#if oauthVerifying}
						{m.vmcps_authenticating()}
					{:else}
						{m.vmcps_authenticate()}
					{/if}
				</a>
			{/if}
		</div>
	</div>
	<form class="dialog-backdrop">
		<button type="button" aria-label={m.common_close_dialog()} onclick={handleOauthClose}
			>{m.common_close()}</button
		>
	</form>
</dialog>
