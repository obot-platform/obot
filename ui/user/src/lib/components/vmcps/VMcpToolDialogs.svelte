<script lang="ts">
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import Confirm from '$lib/components/Confirm.svelte';
	import ResponsiveDialog from '$lib/components/ResponsiveDialog.svelte';
	import CompositeEditTools from '$lib/components/mcp/composite/CompositeEditTools.svelte';
	import IconButton from '$lib/components/primitives/IconButton.svelte';
	import type { VMcpToolDialog, VMcpToolFlow } from '$lib/runes/vmcps/vmcpToolFlow.svelte';
	import McpServerIcon from './McpServerIcon.svelte';
	import VMcpComponentConfigurationDialog from './VMcpComponentConfigurationDialog.svelte';
	import VMcpToolsSetup from './VMcpToolsSetup.svelte';
	import { RefreshCcw, Server, Trash2 } from '@lucide/svelte';

	interface Props {
		flow: VMcpToolFlow;
	}

	let { flow }: Props = $props();
	let addedCreateDialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let setupDialog = $state<ReturnType<typeof VMcpToolsSetup>>();
	let editDialog = $state<ReturnType<typeof CompositeEditTools>>();
	let componentActionsDialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let configurationDialog = $state<ReturnType<typeof VMcpComponentConfigurationDialog>>();
	let renderedDialog: VMcpToolDialog | undefined;
	let synchronizing = false;
	const toolsLockedByUserSupplied = $derived(
		flow.configuringComponent?.configuration?.some((field) => field.policy === 'userAllowed') ??
			false
	);

	function openDialog(dialog: VMcpToolDialog | undefined) {
		if (dialog === 'added-create') addedCreateDialog?.open();
		if (dialog === 'setup') setupDialog?.open();
		if (dialog === 'edit') editDialog?.open();
		if (dialog === 'actions') componentActionsDialog?.open();
		if (dialog === 'configure' && flow.configuringEntry) {
			configurationDialog?.open(flow.configuringEntry, {
				configuration: flow.configuringComponent?.configuration,
				submitLabel: 'Save',
				errorMessage: 'Failed to update configuration.'
			});
		}
	}

	function closeDialog(dialog: VMcpToolDialog | undefined) {
		if (dialog === 'added-create') addedCreateDialog?.close();
		if (dialog === 'setup') setupDialog?.close();
		if (dialog === 'edit') editDialog?.close();
		if (dialog === 'actions') componentActionsDialog?.close();
		if (dialog === 'configure') configurationDialog?.close();
	}

	function handleDialogClose(dialog: VMcpToolDialog) {
		if (synchronizing || flow.dialog !== dialog) return;
		if (dialog === 'configure') {
			flow.returnToActions();
			return;
		}
		flow.close();
	}

	$effect(() => {
		const next = flow.dialog;
		if (next === renderedDialog) return;

		synchronizing = true;
		closeDialog(renderedDialog);
		renderedDialog = next;
		openDialog(next);
		queueMicrotask(() => (synchronizing = false));
	});
</script>

<Confirm
	show={Boolean(flow.pendingRemoval)}
	onsuccess={flow.removeComponent}
	oncancel={flow.cancelRemove}
	msg=""
	loading={flow.removing}
	title="Confirm Remove"
>
	{#snippet note()}
		Are you sure you want to remove "<b>{flow.pendingRemoval?.component.name ?? 'this server'}</b>"
		from <b>{flow.pendingRemoval?.vmcp.displayName ?? 'this vMCP'}</b>? The tools for this server
		will no longer be available.
	{/snippet}
</Confirm>

<ResponsiveDialog
	animate="slide"
	class="w-sm"
	bind:this={addedCreateDialog}
	title="Add Tools"
	onClose={() => handleDialogClose('added-create')}
>
	<div class="flex flex-col gap-4 items-center">
		{#if flow.dialog === 'added-create'}
			{@render serverHeading()}
			<p class="text-sm font-light text-center">
				<b>{flow.addedServer?.component.manifest.name ?? 'this server'}</b> has been added to
				<b>{flow.addedServer?.vmcp.displayName ?? 'this vMCP'}</b>.
			</p>
			<p class="text-sm font-light text-center">
				It is recommended to select which tools to enable to properly secure the vMCP. Otherwise,
				you can skip this step and allow all tools to be enabled.
			</p>
			<div class="flex flex-col gap-2 w-full">
				<button class="btn btn-primary" onclick={flow.selectToolsForAdded}> Modify Tools </button>
				<button class="btn btn-ghost rounded-full text-xs" onclick={flow.close}
					>I understand, skip & allow all tools</button
				>
			</div>
		{/if}
	</div>
</ResponsiveDialog>

<VMcpToolsSetup
	bind:this={setupDialog}
	component={flow.configuringComponent}
	vmcpID={flow.modifyingVMcp?.id}
	refresh={flow.refresh}
	existingTools={flow.tools}
	existingToolPrefix={flow.existingToolPrefix}
	otherEffectiveNames={flow.otherEffectiveNames}
	otherToolPrefixes={flow.otherToolPrefixes}
	onCancel={flow.close}
	onSuccess={(config) => {
		const component = flow.configuringComponent;
		if (!component) return;
		void flow.saveTools({
			...component,
			toolPrefix: config.toolPrefix,
			toolOverrides: config.toolOverrides
		});
	}}
>
	{#snippet additionalActions()}
		{#if flow.modifyingExistingComponent && !flow.collecting}
			{@render removeComponentButton()}
		{/if}
	{/snippet}
</VMcpToolsSetup>

<ResponsiveDialog
	class="md:w-sm"
	bind:this={componentActionsDialog}
	onClose={() => handleDialogClose('actions')}
>
	{#snippet titleContent()}
		<div class="flex items-center gap-2 font-semibold">
			{#if flow.configuringEntry?.manifest.icon}
				<McpServerIcon icon={flow.configuringEntry.manifest.icon} />
			{:else}
				<div class="icon">
					<Server class="size-6" />
				</div>
			{/if}
			{flow.configuringEntry?.manifest.name}
		</div>
	{/snippet}
	<div class="flex flex-col gap-2 md:px-0 px-4">
		<p class="text-sm text-center mb-3 md:mt-0 mt-4">What would you like to do?</p>
		<div
			class="w-full"
			use:tooltip={toolsLockedByUserSupplied
				? {
						text: "Tools can't be modified because this server has user-supplied configuration.",
						disablePortal: true
					}
				: undefined}
		>
			<button
				class="btn btn-secondary w-full"
				disabled={toolsLockedByUserSupplied}
				onclick={flow.modifyToolsFromActions}
			>
				Modify Tools
			</button>
		</div>
		{#if flow.hasConfigurableFields}
			<button class="btn btn-secondary w-full" onclick={flow.editConfiguration}>
				Edit Configuration
			</button>
		{/if}
		<button class="btn btn-secondary hover:btn-error" onclick={flow.promptRemove}
			>Remove {flow.configuringEntry?.manifest.name ?? 'this server'}</button
		>
	</div>
</ResponsiveDialog>

<VMcpComponentConfigurationDialog
	bind:this={configurationDialog}
	onNext={flow.saveConfiguration}
	onClose={() => handleDialogClose('configure')}
/>

<CompositeEditTools
	bind:this={editDialog}
	configuringEntry={flow.configuringEntry}
	tools={flow.tools}
	bind:toolPrefix={flow.toolPrefix}
	otherEffectiveNames={flow.otherEffectiveNames}
	otherToolPrefixes={flow.otherToolPrefixes}
	onCancel={flow.close}
	onClose={() => handleDialogClose('edit')}
	onSuccess={flow.saveEditedTools}
>
	{#snippet additionalActions()}
		<div class="flex items-center gap-3">
			<IconButton
				tooltip={{ text: 'Refresh tools', disablePortal: true, placement: 'right' }}
				onclick={flow.refreshTools}
				class="dark:hover:bg-base-300"
			>
				<RefreshCcw class="size-4" />
			</IconButton>
		</div>
	{/snippet}
</CompositeEditTools>

{#snippet serverHeading()}
	<span class="flex items-center gap-2 text-base font-semibold">
		{#if flow.addedServer?.component.manifest.icon}
			<img src={flow.addedServer.component.manifest.icon} alt="" class="size-6 icon" />
		{:else}
			<div class="icon">
				<Server class="size-6" />
			</div>
		{/if}
		{flow.addedServer?.component.manifest.name}
	</span>
{/snippet}

{#snippet removeComponentButton()}
	<IconButton
		tooltip={{ text: 'Delete MCP Server', disablePortal: true, placement: 'right' }}
		onclick={flow.promptRemove}
		variant="danger2"
	>
		<Trash2 class="size-4" />
	</IconButton>
{/snippet}
