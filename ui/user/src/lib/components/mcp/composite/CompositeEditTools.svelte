<script lang="ts">
	import ResponsiveDialog from '$lib/components/ResponsiveDialog.svelte';
	import Search from '$lib/components/Search.svelte';
	import { AlertTriangle } from 'lucide-svelte';
	import Toggle from '$lib/components/Toggle.svelte';
	import type { CompositeServerToolRow, MCPCatalogEntry, MCPCatalogServer } from '$lib/services';

	interface Props {
		configuringEntry?: MCPCatalogEntry | MCPCatalogServer;
		onClose?: () => void;
		onCancel?: () => void;
		onSuccess?: () => void;
		tools?: CompositeServerToolRow[];
	}

	let { configuringEntry, tools = [], onClose, onCancel, onSuccess }: Props = $props();
	let dialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let confirmDialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let search = $state('');
	let expandedTools = $state<Record<string, boolean>>({});

	// Track initial state to detect changes
	let initialToolsState = $state<string>('');

	let allToolsEnabled = $derived(tools.every((tool) => tool.enabled));

	let visibleTools = $derived(
		tools.filter(
			(tool) =>
				tool.name?.toLowerCase().includes(search.toLowerCase()) ||
				tool.overrideName?.toLowerCase().includes(search.toLowerCase()) ||
				tool.description?.toLowerCase().includes(search.toLowerCase()) ||
				tool.overrideDescription?.toLowerCase().includes(search.toLowerCase())
		)
	);

	// Check if there are any changes compared to initial state
	let hasChanges = $derived.by(() => {
		const currentState = JSON.stringify(tools);
		return initialToolsState !== currentState;
	});

	export function open() {
		// Capture initial state when dialog opens
		initialToolsState = JSON.stringify(tools);
		dialog?.open();
	}

	export function close() {
		dialog?.close();
	}

	function handleClose() {
		if (hasChanges) {
			confirmDialog?.open();
		} else {
			dialog?.close();
			onClose?.();
		}
	}

	function handleCancel() {
		onCancel?.();
		dialog?.close();
	}

	function confirmDiscard() {
		confirmDialog?.close();
		dialog?.close();
		onClose?.();
	}

	function cancelDiscard() {
		confirmDialog?.close();
	}
</script>

<ResponsiveDialog
	bind:this={dialog}
	animate="slide"
	title={`Configure ${configuringEntry?.manifest?.name ?? 'MCP Server'} Tools`}
	class="bg-surface1 md:w-2xl"
	classes={{ content: 'p-0', header: 'p-4 pb-0' }}
	onClickOutside={handleClose}
>
	<p class="text-on-surface1 px-4 text-xs font-light">
		Toggle what tools are available to users of this composite server. Or modify the name or
		description of a tool; this will override the default name or description provided by the
		server. It may affect the LLM's ability to understand the tool so be careful when adjusting
		these values.
	</p>
	<div class="relative flex flex-col gap-2 overflow-x-hidden p-4">
		<div class="flex w-full justify-end">
			<Toggle
				checked={allToolsEnabled}
				onChange={(checked) => {
					tools.forEach((tool) => {
						tool.enabled = checked;
					});
				}}
				label="Enable All Tools"
				labelInline
				classes={{
					label: 'text-sm gap-2'
				}}
				disablePortal
			/>
		</div>
		<Search
			class="dark:bg-surface1 dark:border-surface3 bg-background border border-transparent shadow-sm"
			onChange={(val) => (search = val)}
			placeholder="Search tools..."
		/>
		{#each visibleTools as tool (tool.id)}
			{@const overrideName = (tool.overrideName || '').trim()}
			{@const overrideDescription = (tool.overrideDescription || '').trim()}
			{@const currentName = overrideName || tool.name}
			{@const currentDescription = overrideDescription || tool.description}
			{@const isCustomized =
				(overrideName !== '' && overrideName !== tool.name) ||
				(overrideDescription !== '' && overrideDescription !== tool.description)}

			<div
				class="dark:bg-surface2 dark:border-surface3 bg-background flex items-start gap-2 rounded border border-transparent p-2 shadow-sm"
			>
				<div class="flex min-w-0 grow flex-col gap-2">
					<div class="flex items-start justify-between gap-2">
						<div class="min-w-0">
							<div class="truncate text-sm font-medium" title={currentName}>
								{currentName}
							</div>
							{#if currentDescription}
								<p class="line-clamp-2 text-xs" title={currentDescription}>
									{currentDescription}
								</p>
							{/if}
						</div>
						<div class="flex flex-shrink-0 items-center gap-2">
							<!-- Enabled/disabled toggle for this tool -->
							<Toggle
								checked={tool.enabled}
								onChange={(checked) => {
									tool.enabled = checked;
								}}
								label="Enabled"
								disablePortal
							/>
							<button
								type="button"
								class="button px-3 py-1 text-xs"
								onclick={() => {
									// When expanding, initialize inputs with current effective values
									if (!expandedTools[tool.id]) {
										tool.overrideName = (tool.overrideName || '').trim() || tool.name;
										tool.overrideDescription =
											(tool.overrideDescription || '').trim() || tool.description;
									}
									expandedTools[tool.id] = !expandedTools[tool.id];
								}}
							>
								{expandedTools[tool.id] ? 'Hide details' : 'Customize'}
							</button>
						</div>
					</div>

					{#if isCustomized}
						<div class="mt-1 flex items-center gap-1 text-[11px] text-amber-600">
							<AlertTriangle class="size-3 flex-shrink-0" />
							<p>
								Modified: This tool has been customized. The description or name has been changed.
							</p>
						</div>
					{/if}

					{#if expandedTools[tool.id]}
						<div class="mt-2 flex flex-col gap-2">
							<div class="flex flex-col gap-1">
								<p class="text-xs text-gray-500">Tool name</p>
								<input class="text-input-filled flex-1 text-sm" bind:value={tool.overrideName} />
							</div>

							<div class="flex flex-col gap-1">
								<p class="text-xs text-gray-500">Description</p>
								<textarea
									class="text-input-filled h-24 resize-none text-xs"
									bind:value={tool.overrideDescription}
									placeholder="Enter tool description..."
								></textarea>
							</div>

							<div class="mt-2 flex justify-end">
								<button
									type="button"
									class="button px-3 py-1 text-xs"
									onclick={() => {
										tool.overrideName = tool.name;
										tool.overrideDescription = tool.description;
									}}
								>
									Reset to default
								</button>
							</div>
						</div>
					{/if}
				</div>
			</div>
		{/each}
	</div>
	<div class="bg-surface1 sticky bottom-0 left-0 mt-4 flex w-full justify-end gap-2 p-4">
		<button class="button" onclick={handleCancel}>Cancel</button>
		<button
			class="button-primary"
			onclick={() => {
				onSuccess?.();
				dialog?.close();
			}}>Confirm</button
		>
	</div>
</ResponsiveDialog>

<!-- Confirmation Dialog for Unsaved Changes -->
<ResponsiveDialog bind:this={confirmDialog} title="Discard Changes?" class="max-w-xl">
	<p class="text-on-surface1 mb-4 text-sm">
		You have unsaved changes for {configuringEntry?.manifest?.name ?? 'MCP Server'} configuration. Are
		you sure you want to discard these changes?
	</p>

	<div class="flex justify-end gap-3">
		<button class="button" onclick={cancelDiscard}>Keep Editing</button>
		<button class="button-primary bg-red-600 hover:bg-red-700" onclick={confirmDiscard}>
			Discard Changes
		</button>
	</div>
</ResponsiveDialog>
