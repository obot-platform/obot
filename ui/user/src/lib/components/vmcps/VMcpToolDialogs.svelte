<script lang="ts">
	import { page } from '$app/state';
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import Confirm from '$lib/components/Confirm.svelte';
	import ResponsiveDialog from '$lib/components/ResponsiveDialog.svelte';
	import CompositeEditTools from '$lib/components/mcp/composite/CompositeEditTools.svelte';
	import type { VMcpToolDialog, VMcpToolFlow } from '$lib/runes/vmcps/vmcpToolFlow.svelte';
	import { UserService, type ToolOverride, type VMCPProfile } from '$lib/services';
	import { configurationWithRevealedValues, vmcpComponentId } from '$lib/services/vmcps/utils';
	import { goto, setUrlParam } from '$lib/url';
	import McpServerIcon from './McpServerIcon.svelte';
	import VMcpComponentConfigurationDialog from './VMcpComponentConfigurationDialog.svelte';
	import VMcpToolsSetup from './VMcpToolsSetup.svelte';
	import { ArrowRightLeft, RefreshCcw, Server, Settings2 } from '@lucide/svelte';
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
	let pendingAffectedProfiles = $state<VMCPProfile[]>([]);
	const isLastComponent = $derived((flow.modifyingVMcp?.components ?? []).length <= 1);
	const lastComponentTooltip = 'VMCP requires at least one component.';

	async function openConfigureDialog() {
		if (!flow.configuringEntry) return;

		let configuration = flow.configuringComponent?.configuration;
		const vmcpId = flow.modifyingVMcp?.id;
		const componentId = flow.configuringComponent
			? vmcpComponentId(flow.configuringComponent)
			: undefined;
		if (vmcpId && componentId) {
			try {
				const revealed = await UserService.revealVMCP(vmcpId, { dontLogErrors: true });
				configuration = configurationWithRevealedValues(
					configuration,
					revealed.components[componentId]
				);
			} catch {
				// Continue without revealed values when configuration is unavailable.
			}
		}

		configurationDialog?.open(flow.configuringEntry, {
			configuration,
			forceSingleUser: flow.configuringComponent?.forceSingleUser,
			submitLabel: flow.postCreateConfiguration ? 'Next' : 'Save',
			errorMessage: 'Failed to update configuration.'
		});
	}

	function openDialog(dialog: VMcpToolDialog | undefined) {
		if (dialog === 'added-create') {
			addedCreateDialog?.open();
			void tick().then(() => asIsButton?.focus({ focusVisible: true }));
		}
		if (dialog === 'setup') setupDialog?.open();
		if (dialog === 'edit') editDialog?.open();
		if (dialog === 'actions') componentActionsDialog?.open();
		if (dialog === 'configure' && flow.configuringEntry) {
			void openConfigureDialog();
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
		flow.close();
	}

	function dialogReady(dialog: VMcpToolDialog | undefined) {
		if (!dialog) return true;
		if (dialog === 'added-create') return Boolean(addedCreateDialog);
		if (dialog === 'setup') return Boolean(setupDialog);
		if (dialog === 'edit') return Boolean(editDialog);
		if (dialog === 'actions') return Boolean(componentActionsDialog);
		if (dialog === 'configure') return Boolean(configurationDialog);
		return true;
	}

	function droppedToolNames(
		previous: ToolOverride[] | undefined,
		next: ToolOverride[] | undefined
	) {
		const previousByName = new Map((previous ?? []).map((tool) => [tool.name, tool]));
		const nextByName = new Map((next ?? []).map((tool) => [tool.name, tool]));
		// eslint-disable-next-line svelte/prefer-svelte-reactivity
		const dropped = new Set<string>();
		for (const [name, tool] of previousByName) {
			const updated = nextByName.get(name);
			const wasEnabled = tool.enabled !== false;
			const stillEnabled = Boolean(updated && updated.enabled !== false && !updated.removed);
			if ((wasEnabled && !stillEnabled) || !updated || updated.removed) dropped.add(name);
		}
		for (const [name, tool] of nextByName) {
			if (!previousByName.has(name) && (tool.removed || tool.enabled === false)) dropped.add(name);
		}
		return dropped;
	}

	function profilesAffectedByToolSave(
		componentId: string | undefined,
		previous: ToolOverride[] | undefined,
		next: ToolOverride[] | undefined,
		profiles: VMCPProfile[] | undefined
	) {
		if (!componentId || !profiles?.length) return [];
		const dropped = droppedToolNames(previous, next);
		if (dropped.size === 0) return [];
		return profiles.filter((profile) => {
			const allowed = profile.vmcpPermissions?.allowedComponents?.[componentId]?.allowedTools;
			return Array.isArray(allowed) && allowed.some((name) => dropped.has(name));
		});
	}

	async function saveAndOfferProfileUpdate(
		config: {
			toolPrefix?: string;
			toolOverrides?: ToolOverride[];
		},
		offerProfileUpdate: boolean
	) {
		const component = flow.configuringComponent;
		if (!component) return;
		const affected = offerProfileUpdate
			? profilesAffectedByToolSave(
					flow.configuringComponentId,
					component.toolOverrides,
					config.toolOverrides,
					flow.modifyingVMcp?.profiles
				)
			: [];
		const saved = await flow.saveTools({
			...component,
			toolPrefix: config.toolPrefix,
			toolOverrides: config.toolOverrides
		});
		if (saved && affected.length > 0) pendingAffectedProfiles = affected;
	}

	function openAffectedProfiles() {
		const targets = pendingAffectedProfiles;
		pendingAffectedProfiles = [];
		const url = new URL(page.url);
		setUrlParam(url, 'view', 'profiles');
		setUrlParam(url, 'profile', targets.length === 1 ? targets[0].name : null);
		goto(url, { replaceState: true, noScroll: true, keepFocus: true });
	}

	$effect(() => {
		const next = flow.dialog;
		if (next === renderedDialog) return;
		if (!dialogReady(next)) return;

		synchronizing = true;
		closeDialog(renderedDialog);
		renderedDialog = next;
		openDialog(next);
		queueMicrotask(() => (synchronizing = false));
	});
</script>

<Confirm
	show={pendingAffectedProfiles.length > 0}
	onsuccess={openAffectedProfiles}
	oncancel={() => (pendingAffectedProfiles = [])}
	type="info"
	title="Update Profile(s)?"
	submitText={pendingAffectedProfiles.length === 1 ? 'Update Profile' : 'Go to Profiles'}
	cancelText="Skip"
	msg="Update existing profile(s) now?"
>
	{#snippet note()}
		<p class="text-sm font-light">
			{pendingAffectedProfiles.length === 1 ? 'Your profile is' : 'There are existing profile(s)'} affected
			by the tool changes. It is recommended to check the profiles and modify to the updated tool changes.
			Would you like to do this now?
		</p>
	{/snippet}
</Confirm>

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
	<div class="flex flex-col gap-4 p-4 md:p-0">
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
		void saveAndOfferProfileUpdate(config, flow.refresh);
	}}
/>

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
		<button class="btn btn-secondary w-full" onclick={flow.modifyToolsFromActions}>
			Modify Tools
		</button>
		{#if flow.canConfigureComponent}
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
	profiles={flow.modifyingVMcp?.profiles}
	componentId={flow.configuringComponentId}
	bind:toolPrefix={flow.toolPrefix}
	otherEffectiveNames={flow.otherEffectiveNames}
	otherToolPrefixes={flow.otherToolPrefixes}
	onCancel={flow.close}
	onClose={() => handleDialogClose('edit')}
	onSuccess={flow.saveEditedTools}
>
	{#snippet additionalActions()}
		<div class="flex items-center gap-3">
			<button
				onclick={() => flow.refreshTools()}
				class="btn-sm btn-outline btn not-hover:border-muted-content/50 not-hover:text-muted-content rounded-full hover:btn-primary hover:btn-outline"
			>
				<RefreshCcw class="size-4" /> Refresh tools
			</button>
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
