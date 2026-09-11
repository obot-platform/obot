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
	import { ArrowRightLeft, RefreshCcw, Server, Settings2, Trash2 } from '@lucide/svelte';
	import { tick } from 'svelte';

	interface Props {
		flow: VMcpToolFlow;
	}

	let { flow }: Props = $props();
	let addedCreateDialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let asIsButton = $state<HTMLButtonElement>();
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
	const isLastComponent = $derived((flow.modifyingVMcp?.components ?? []).length <= 1);
	const lastComponentTooltip = 'VMCP requires at least one component.';

	function openDialog(dialog: VMcpToolDialog | undefined) {
		if (dialog === 'added-create') {
			addedCreateDialog?.open();
			void tick().then(() => asIsButton?.focus({ focusVisible: true }));
		}
		if (dialog === 'setup') setupDialog?.open();
		if (dialog === 'edit') editDialog?.open();
		if (dialog === 'actions') componentActionsDialog?.open();
		if (dialog === 'configure' && flow.configuringEntry) {
			configurationDialog?.open(flow.configuringEntry, {
				configuration: flow.configuringComponent?.configuration,
				submitLabel: flow.postCreateConfiguration ? 'Next' : 'Save',
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
	class="md:w-lg"
	bind:this={addedCreateDialog}
	title="Add Tools"
	onClose={() => handleDialogClose('added-create')}
>
	<div class="flex flex-col gap-4">
		{#if flow.dialog === 'added-create'}
			<div class="flex flex-col items-center gap-4">
				{@render serverHeading()}
				<p class="text-center text-sm font-light">
					How would you like to set up the MCP server tools?
				</p>
			</div>
			<div class="flex w-full flex-col gap-4">
				<button
					bind:this={asIsButton}
					class="dark:bg-base-300 hover:bg-base-200 focus:bg-base-200 dark:hover:bg-base-400 dark:focus:bg-base-400 dark:border-base-400 border-base-300 group bg-base-100 flex cursor-pointer items-center gap-4 rounded-md border px-2 py-4 text-left transition-colors duration-300"
					onclick={flow.close}
				>
					<ArrowRightLeft
						class="text-muted-content size-12 shrink-0 pl-1 transition-colors group-hover:text-inherit group-focus:text-inherit"
					/>
					<div>
						<p class="mb-1 text-sm font-semibold">As-is</p>
						<span class="text-muted-content block text-xs leading-4">
							Use the MCP server as-is. Tools and their definitions are passed through
							automatically, including future changes from the source. No authentication is required
							during setup.
						</span>
					</div>
				</button>
				<button
					class="dark:bg-base-300 hover:bg-base-200 focus:bg-base-200 dark:hover:bg-base-400 dark:focus:bg-base-400 dark:border-base-400 border-base-300 group bg-base-100 flex cursor-pointer items-center gap-4 rounded-md border px-2 py-4 text-left transition-colors duration-300"
					onclick={flow.selectToolsForAdded}
				>
					<Settings2
						class="text-muted-content size-12 shrink-0 pl-1 transition-colors group-hover:text-inherit group-focus:text-inherit"
					/>
					<div>
						<p class="mb-1 text-sm font-semibold">
							Managed <span class="text-muted-content font-normal">[Recommended]</span>
						</p>
						<span class="text-muted-content block text-xs leading-4">
							Authenticate to discover and select specific tools. Tool names and descriptions are
							captured and can be customized, protecting the vMCP from unexpected upstream changes.
						</span>
					</div>
				</button>
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
				Change Configuration
			</button>
		{/if}
		<div
			class="w-full"
			use:tooltip={isLastComponent
				? { text: lastComponentTooltip, disablePortal: true, placement: 'bottom' }
				: undefined}
		>
			<button
				class="btn btn-secondary hover:btn-error w-full"
				disabled={isLastComponent}
				onclick={flow.promptRemove}
			>
				Remove {flow.configuringEntry?.manifest.name ?? 'this server'}
			</button>
		</div>
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
	<div
		use:tooltip={isLastComponent
			? { text: lastComponentTooltip, disablePortal: true, placement: 'right' }
			: undefined}
	>
		<IconButton
			tooltip={{
				text: isLastComponent ? lastComponentTooltip : 'Delete MCP Server',
				disablePortal: true,
				placement: 'right'
			}}
			onclick={flow.promptRemove}
			variant="danger2"
			disabled={isLastComponent}
		>
			<Trash2 class="size-4" />
		</IconButton>
	</div>
{/snippet}
