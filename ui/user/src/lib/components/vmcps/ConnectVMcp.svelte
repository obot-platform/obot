<script lang="ts">
	import Confirm from '$lib/components/Confirm.svelte';
	import CopyField from '$lib/components/CopyField.svelte';
	import ResponsiveDialog from '$lib/components/ResponsiveDialog.svelte';
	import CatalogConfigureForm, {
		type CompositeLaunchFormData
	} from '$lib/components/mcp/CatalogConfigureForm.svelte';
	import HowToConnect from '$lib/components/mcp/HowToConnect.svelte';
	import { UserService, type VMCP, type VMCPConfiguration, type VMCPInstance } from '$lib/services';
	import {
		resolveVMcpComponents,
		vmcpComponentId,
		vmcpConnectURL
	} from '$lib/services/vmcps/utils';
	import { vmcpInstances } from '$lib/stores';
	import VMcpIcon from './VMcpIcon.svelte';

	let vmcp = $state<VMCP>();
	let instance = $state<VMCPInstance>();
	let connectDialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let configureDialog = $state<ReturnType<typeof CatalogConfigureForm>>();
	let howToConnect = $state<ReturnType<typeof HowToConnect>>();
	let connectionUrlField = $state<ReturnType<typeof CopyField>>();
	let configureForm = $state<CompositeLaunchFormData>();
	let saving = $state(false);
	let error = $state<string>();
	let configureOpen = $state(false);
	let showIntroDialog = $state(false);

	let connectURL = $derived(vmcp ? vmcpConnectURL(vmcp) : undefined);
	let displayName = $derived(vmcp?.displayName || 'vMCP');
	let componentViews = $derived(vmcp ? resolveVMcpComponents(vmcp) : []);
	let hasUserConfiguration = $derived(
		vmcp?.components?.some((component) =>
			component.configuration?.find((field) => field.policy === 'userAllowed')
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
		error = undefined;
		showIntroDialog = false;
		connectionUrlField?.clear?.();
		howToConnect?.resetCopied?.();
		connectDialog?.open();
	}

	function handleConfigure() {
		showIntroDialog = false;
		initConfigureForm();
	}

	function initLaunch() {
		connectDialog?.close();
		showIntroDialog = true;
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
		configureForm = { componentConfigs };
		error = undefined;
		configureOpen = true;
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

	async function saveConfiguration() {
		const target = vmcp;
		if (!target || !configureForm || saving) return;
		saving = true;
		error = undefined;
		try {
			const targetInstance = instance ?? (await UserService.createVMCPInstance(target.id));
			const configured = await UserService.configureVMCPInstance(
				targetInstance.id,
				configurationPayload(configureForm)
			);
			instance = {
				...configured,
				status: { ...configured.status, configured: true }
			};
			vmcpInstances.upsert(instance);
			configureDialog?.close();
			configureOpen = false;
			connectDialog?.open();
		} catch (err) {
			error = err instanceof Error ? err.message : 'Failed to configure this vMCP.';
		} finally {
			saving = false;
		}
	}
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
		if (saving || configureOpen) return;
		vmcp = undefined;
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
			onLaunch={hasUserConfiguration && !instance ? initLaunch : undefined}
			onEdit={hasUserConfiguration && instance ? initConfigureForm : undefined}
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
	onClose={() => {
		configureOpen = false;
		error = undefined;
		if (vmcp) connectDialog?.open();
	}}
	submitText={instance ? 'Update' : 'Configure'}
	loading={saving}
	{error}
	isNew={false}
	showComponentToggle={false}
	configurationTitle="User Specific Configuration"
>
	{#snippet icon()}
		<VMcpIcon components={componentViews} />
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
