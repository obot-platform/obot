<script lang="ts">
	import Confirm from '$lib/components/Confirm.svelte';
	import ResponsiveDialog from '$lib/components/ResponsiveDialog.svelte';
	import CompositeEditTools from '$lib/components/mcp/composite/CompositeEditTools.svelte';
	import CompositeToolsSetup from '$lib/components/mcp/composite/CompositeSelectServerAndToolsSetup.svelte';
	import IconButton from '$lib/components/primitives/IconButton.svelte';
	import { DEFAULT_MCP_CATALOG_ID } from '$lib/constants';
	import type { VMcpToolDialog, VMcpToolFlow } from '$lib/runes/vmcps/vmcpToolFlow.svelte';
	import { RefreshCcw, Server, Trash2 } from '@lucide/svelte';

	interface Props {
		flow: VMcpToolFlow;
	}

	let { flow }: Props = $props();
	let addedCreateDialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let setupDialog = $state<ReturnType<typeof CompositeToolsSetup>>();
	let editDialog = $state<ReturnType<typeof CompositeEditTools>>();
	let componentActionsDialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let renderedDialog: VMcpToolDialog | undefined;
	let synchronizing = false;

	function openDialog(dialog: VMcpToolDialog | undefined) {
		if (dialog === 'added-create') addedCreateDialog?.open();
		if (dialog === 'setup') setupDialog?.open();
		if (dialog === 'edit') editDialog?.open();
		if (dialog === 'actions') componentActionsDialog?.open();
	}

	function closeDialog(dialog: VMcpToolDialog | undefined) {
		if (dialog === 'added-create') addedCreateDialog?.close();
		if (dialog === 'setup') setupDialog?.close();
		if (dialog === 'edit') editDialog?.close();
		if (dialog === 'actions') componentActionsDialog?.close();
	}

	function handleDialogClose(dialog: VMcpToolDialog) {
		if (!synchronizing && flow.dialog === dialog) flow.close();
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
		from <b>{flow.pendingRemoval?.vmcp.manifest.name ?? 'this vMCP'}</b>? The tools for this server
		will no longer be available.
	{/snippet}
</Confirm>

<Confirm
	show={flow.dialog === 'added-confirm'}
	onsuccess={flow.selectToolsForAdded}
	oncancel={flow.close}
	title="Add MCP Server"
	type="info"
	submitText="Select Which Tools To Enable"
	cancelText="Add All Tools"
	classes={{
		actions: 'flex-col md:flex-col',
		confirm: 'btn-secondary'
	}}
>
	{#snippet msgContent()}
		{@render serverHeading()}
	{/snippet}
	{#snippet note()}
		<b>{flow.addedServer?.component.manifest.name ?? 'this server'}</b> has been added to
		<b>{flow.addedServer?.vmcp.manifest.name ?? 'this vMCP'}</b>. Would you like to add all tools or
		select which tools to enable?
	{/snippet}
</Confirm>

<ResponsiveDialog
	animate="slide"
	class="w-md"
	bind:this={addedCreateDialog}
	title="Add Tools"
	onClose={() => handleDialogClose('added-create')}
>
	{#if flow.dialog === 'added-create'}
		<div class="mb-4">
			{@render serverHeading()}
		</div>
		<p class="mb-4 text-sm font-light">
			<b>{flow.addedServer?.component.manifest.name ?? 'this server'}</b> has been added to
			<b>{flow.addedServer?.vmcp.manifest.name ?? 'this vMCP'}</b>. Would you like to add all tools
			or select which tools to enable?
		</p>
		<div class="flex flex-col gap-2">
			<button class="btn btn-secondary btn-sm text-xs" onclick={flow.close}>Add All Tools</button>
			<button class="btn btn-primary btn-sm text-xs" onclick={flow.selectToolsForAdded}>
				Select Which Tools To Enable
			</button>
		</div>
	{/if}
</ResponsiveDialog>

<CompositeToolsSetup
	bind:this={setupDialog}
	catalogId={DEFAULT_MCP_CATALOG_ID}
	configuringEntry={flow.configuringEntry}
	compositeEntryId={flow.modifyingVMcp?.id}
	componentId={flow.configuringComponentId}
	isNewComponent={!flow.modifyingExistingComponent}
	existingTools={flow.tools}
	existingToolPrefix={flow.existingToolPrefix}
	otherEffectiveNames={flow.otherEffectiveNames}
	otherToolPrefixes={flow.otherToolPrefixes}
	excluded={flow.excludedComponentIds}
	onCancel={flow.close}
	onSuccess={flow.saveTools}
>
	{#snippet additionalActions()}
		{#if flow.modifyingExistingComponent}
			{@render removeComponentButton()}
		{/if}
	{/snippet}
</CompositeToolsSetup>

<ResponsiveDialog
	class="md:w-sm"
	bind:this={componentActionsDialog}
	onClose={() => handleDialogClose('actions')}
>
	{#snippet titleContent()}
		<div class="flex items-center gap-2 text-base font-semibold">
			{#if flow.configuringEntry?.manifest.icon}
				<img src={flow.configuringEntry.manifest.icon} alt="" class="size-6 icon" />
			{:else}
				<div class="icon">
					<Server class="size-6" />
				</div>
			{/if}
			{flow.configuringEntry?.manifest.name}
		</div>
	{/snippet}
	<p class="mb-4 text-sm font-light">
		All tools are enabled on
		<b class="font-semibold">{flow.modifyingVMcp?.manifest.name ?? 'this vMCP'}</b> by default. What would
		you like to do?
	</p>
	<div class="flex flex-col gap-2">
		<button class="btn btn-primary" onclick={flow.modifyToolsFromActions}>Modify Tools</button>
		<button class="btn btn-error" onclick={flow.promptRemove}>Delete MCP Server</button>
	</div>
</ResponsiveDialog>

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
			{@render removeComponentButton()}
			<IconButton
				tooltip={{ text: 'Refresh Tools', disablePortal: true, placement: 'right' }}
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
