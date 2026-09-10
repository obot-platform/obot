<script lang="ts">
	import { dialogAnimation } from '$lib/actions/dialogAnimation';
	import Confirm from '$lib/components/Confirm.svelte';
	import CopyField from '$lib/components/CopyField.svelte';
	import ResponsiveDialog from '$lib/components/ResponsiveDialog.svelte';
	import CatalogConfigureForm, {
		type CompositeLaunchFormData
	} from '$lib/components/mcp/CatalogConfigureForm.svelte';
	import HowToConnect from '$lib/components/mcp/HowToConnect.svelte';
	import IconButton from '$lib/components/primitives/IconButton.svelte';
	import { UserService, type VMCP, type VMCPConfiguration, type VMCPInstance } from '$lib/services';
	import {
		resolveVMcpComponents,
		vmcpComponentId,
		vmcpConnectURL
	} from '$lib/services/vmcps/utils';
	import { vmcpInstances } from '$lib/stores';
	import VMcpIcon from './VMcpIcon.svelte';
	import { CircleAlert, X } from '@lucide/svelte';
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

	let connectURL = $derived(vmcp ? vmcpConnectURL(vmcp) : undefined);
	let displayName = $derived(vmcp?.displayName || 'vMCP');
	let componentViews = $derived(vmcp ? resolveVMcpComponents(vmcp) : []);
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

	function generateIdFromName(name: string) {
		return name
			.toLowerCase()
			.replace(/ /g, '-')
			.replace(/[^a-z0-9-_]/g, '');
	}

	export function open(target: VMCP, targetInstance?: VMCPInstance) {
		vmcp = target;
		instance = targetInstance;
		configureForm = undefined;
		error = undefined;
		launchError = undefined;
		launchProgress = 0;
		launchState = undefined;
		oauthURL = '';
		oauthVerifying = false;
		showIntroDialog = false;
		connectionUrlField?.clear?.();
		howToConnect?.resetCopied?.();
		connectDialog?.open();
	}

	function handleConfigure() {
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

	async function initConfigureForm() {
		if (!vmcp) return;
		connectDialog?.close();
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
					value: '',
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

	function finishLaunch() {
		configureDialog?.close();
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
			configureDialog?.close();
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
		if (!target || saving) return;
		saving = true;
		error = undefined;
		const { timeout1, timeout2, timeout3 } = initUpdatingOrLaunchProgress();
		try {
			const targetInstance = instance ?? (await UserService.createVMCPInstance(target.id));
			if (configureForm) {
				const configured = await UserService.configureVMCPInstance(
					targetInstance.id,
					configurationPayload(configureForm)
				);
				instance = {
					...configured,
					status: { ...configured.status, configured: true }
				};
			} else {
				instance = targetInstance;
			}
			vmcpInstances.upsert(instance);

			const launchResponse = await UserService.validateSingleOrRemoteMcpServerLaunched(target.id);
			if (!launchResponse.success) {
				launchError = launchResponse.message ?? 'Failed to launch this vMCP.';
				return;
			}

			await verifyOauthOrConnect();
		} catch (err) {
			launchError = err instanceof Error ? err.message : 'Failed to launch this vMCP.';
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
		<div id="connection-url-container" class="flex flex-col gap-3 md:p-0 pb-0 p-4">
			<CopyField
				bind:this={connectionUrlField}
				value={connectURL}
				id="connectURL"
				label="Connection URL"
			/>
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
	{:else}
		<p class="text-sm text-muted-content font-light md:p-0 p-4">
			This vMCP does not have a connection URL yet.
		</p>
	{/if}
</ResponsiveDialog>

<CatalogConfigureForm
	bind:this={configureDialog}
	bind:form={configureForm}
	name={displayName}
	onSave={saveConfiguration}
	submitText={instance ? 'Update' : 'Configure'}
	loading={saving || launchState === 'launching'}
	{error}
	isNew={false}
	showComponentToggle={false}
	configurationTitle="User Specific Configuration"
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
							<h4 class="text-md font-medium">vMCP Launch Failed</h4>
						</div>

						<div class="text-xs mt-2">
							There was an issue launching this vMCP.

							<ul class="list-disc px-4 py-1 space-y-1">
								{#if hasUserConfiguration}
									<li>Verify your configurations provided at launch are correct and try again.</li>
								{/if}
								<li>If the issue persists, please contact support.</li>
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
								Update Configuration and Try Again
							</button>
						{/if}
						<button
							class="btn btn-secondary w-full md:w-1/2 md:flex-1"
							onclick={() => {
								launchState = undefined;
								launchError = undefined;
								launchProgress = 0;
								configureDialog?.close();
								if (vmcp) connectDialog?.open();
							}}
						>
							Close
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
						<p class="text-xs font-light">Launching vMCP...</p>
					</div>
				</div>
			{/if}
		</div>
	{/snippet}
</CatalogConfigureForm>

<Confirm
	show={showIntroDialog}
	onsuccess={handleConfigure}
	submitText="Continue"
	type="info"
	title="Connect To Server"
	oncancel={() => (showIntroDialog = false)}
	hideCancelButton
>
	{#snippet msgContent()}
		<div class="flex items-center gap-2 text-lg font-semibold mb-2">
			{@render dialogTitle()}
		</div>
	{/snippet}
	{#snippet note()}
		<p>
			This will begin the initial setup process for this server.
			{#if hasUserConfiguration}
				Additional configuration details may also be required before the server can be used.
			{:else}
				<br />Click below to begin.
			{/if}
		</p>
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
					In order to use {displayName}, authentication with the MCP server is required.
				</p>

				<p>Click the link below to authenticate.</p>

				<!-- eslint-disable svelte/no-navigation-without-resolve -- external OAuth URL -->
				<a
					href={oauthURL}
					rel="external"
					target="_blank"
					class="btn btn-primary text-center text-sm outline-none"
					onclick={() => {
						oauthVerifying = true;
					}}
				>
					{#if oauthVerifying}
						Authenticating...
					{:else}
						Authenticate
					{/if}
				</a>
			{/if}
		</div>
	</div>
	<form class="dialog-backdrop">
		<button type="button" aria-label="Close dialog" onclick={handleOauthClose}>close</button>
	</form>
</dialog>
