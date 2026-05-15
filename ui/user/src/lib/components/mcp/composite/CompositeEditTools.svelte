<script lang="ts">
	import ResponsiveDialog from '$lib/components/ResponsiveDialog.svelte';
	import Search from '$lib/components/Search.svelte';
	import Toggle from '$lib/components/Toggle.svelte';
	import type { CompositeServerToolRow, MCPCatalogEntry, MCPCatalogServer } from '$lib/services';
	import {
		conflictIssue,
		duplicateToolNames,
		effectiveToolName,
		MAX_TOOL_PREFIX_LENGTH,
		TOOL_NAME_CHARSET_REGEX,
		TOOL_NAME_SPECIAL_CHAR_WARNING,
		toolNameIssue
	} from '$lib/services/chat/mcp';
	import ToolNameIssueIcon from '../ToolNameIssueIcon.svelte';
	import { TriangleAlert } from 'lucide-svelte';

	interface Props {
		configuringEntry?: MCPCatalogEntry | MCPCatalogServer;
		onClose?: () => void;
		onCancel?: () => void;
		onSuccess?: () => void;
		tools?: CompositeServerToolRow[];
		toolPrefix?: string;
		// Effective names of enabled tools from OTHER components of the composite,
		// so the modal can flag cross-component final-name conflicts live as the
		// admin edits overrides or the prefix.
		otherEffectiveNames?: string[];
		otherToolPrefixes?: string[];
	}

	let {
		configuringEntry,
		tools = [],
		toolPrefix = $bindable(),
		otherEffectiveNames,
		otherToolPrefixes,
		onClose,
		onCancel,
		onSuccess
	}: Props = $props();

	let ownEnabledEffectiveNames = $derived(
		tools.filter((t) => t.enabled).map((t) => effectiveToolName(t.name, t.overrideName, toolPrefix))
	);
	let conflictSet = $derived(
		duplicateToolNames([...(otherEffectiveNames ?? []), ...ownEnabledEffectiveNames])
	);

	let prefixInvalid = $derived(!TOOL_NAME_CHARSET_REGEX.test(toolPrefix ?? ''));
	let prefixTooLong = $derived((toolPrefix ?? '').length > MAX_TOOL_PREFIX_LENGTH);
	let prefixSpecialChar = $derived(/[./]/.test(toolPrefix ?? ''));
	let duplicatePrefix = $derived.by(() => {
		const prefix = (toolPrefix ?? '').trim();
		if (!prefix) return false;
		return (otherToolPrefixes ?? []).some((p) => p === prefix);
	});
	let prefixIssue = $derived(
		prefixInvalid
			? ({
					severity: 'error',
					message: "Prefix may only contain letters, digits, '.', '/', '_', and '-'."
				} as const)
			: prefixTooLong
				? ({
						severity: 'error',
						message: `Prefix must be at most ${MAX_TOOL_PREFIX_LENGTH} characters.`
					} as const)
				: duplicatePrefix
					? ({
							severity: 'error',
							message: `Another component already uses the prefix "${(toolPrefix ?? '').trim()}". Non-empty prefixes must be unique across components.`
						} as const)
					: prefixSpecialChar
						? ({
								severity: 'warning',
								message: TOOL_NAME_SPECIAL_CHAR_WARNING
							} as const)
						: undefined
	);

	// Only enabled tools contribute to blocking errors; disabled tools aren't exposed.
	let hasBlockingToolNameErrors = $derived(
		tools.some((t) => {
			if (!t.enabled) return false;
			const name = effectiveToolName(t.name, t.overrideName, toolPrefix);
			if (toolNameIssue(name)?.severity === 'error') return true;
			return conflictSet.has(name);
		})
	);
	let dialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let confirmDialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let search = $state('');
	let expandedTools = $state<Record<string, boolean>>({});

	// Track initial state to detect changes
	let initialConfigState = $state<string>('');

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
		const currentState = JSON.stringify({ tools, toolPrefix: toolPrefix ?? '' });
		return initialConfigState !== currentState;
	});

	export function open() {
		// Capture initial state when dialog opens
		initialConfigState = JSON.stringify({ tools, toolPrefix: toolPrefix ?? '' });
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
	class="bg-base-200 md:w-2xl"
	classes={{ content: 'p-0', header: 'p-4 pb-0' }}
	onClickOutside={handleClose}
>
	<p class="text-muted-content px-4 text-xs font-light">
		Toggle what tools are available to users of this composite server. Or modify the name or
		description of a tool; this will override the default name or description provided by the
		server. It may affect the LLM's ability to understand the tool so be careful when adjusting
		these values.
	</p>
	<div class="relative flex flex-col gap-2 overflow-x-hidden p-4">
		<div class="flex flex-col gap-1">
			<p class="flex items-center gap-1.5 text-xs text-muted-content">
				<span>Tool name prefix</span>
				{#if prefixIssue}
					<ToolNameIssueIcon issue={prefixIssue} disablePortal />
				{/if}
			</p>
			<div class="flex items-center gap-2">
				<input
					class="text-input-filled flex-1 text-sm"
					placeholder="No prefix"
					bind:value={toolPrefix}
				/>
				<button
					type="button"
					class="btn btn-secondary btn-sm px-3 py-1"
					onclick={() => {
						toolPrefix = '';
					}}
				>
					Clear
				</button>
			</div>
			{#if prefixIssue}
				<p class={`text-xs ${prefixIssue.severity === 'error' ? 'text-error' : 'text-warning'}`}>
					{prefixIssue.message}
				</p>
			{:else}
				<p class="text-muted-content text-[11px]">
					Prepended to every tool name exposed by this component. Clear to remove.
				</p>
			{/if}
		</div>
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
			class="dark:bg-base-200 dark:border-base-400 bg-base-100 border border-transparent shadow-sm"
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

			{@const effectiveName = effectiveToolName(tool.name, tool.overrideName, toolPrefix)}
			{@const conflict = tool.enabled ? conflictIssue(effectiveName, conflictSet) : undefined}
			<div
				class="dark:bg-base-300 dark:border-base-400 bg-base-100 flex items-start gap-2 rounded border border-transparent p-2 shadow-sm"
			>
				<div class="flex min-w-0 grow flex-col gap-2">
					<div class="flex items-start justify-between gap-2">
						<div class="min-w-0 flex-1">
							<div class="flex min-w-0 items-center gap-1.5">
								<div class="min-w-0 flex-1 truncate text-sm font-medium" title={effectiveName}>
									{#if toolPrefix}<span class="text-muted-content">{toolPrefix}</span
										>{/if}{currentName}
								</div>
								{#if tool.enabled}
									<ToolNameIssueIcon
										issue={conflict ?? toolNameIssue(effectiveName)}
										disablePortal
									/>
								{/if}
							</div>
							{#if currentDescription}
								<p class="line-clamp-2 text-xs" title={currentDescription}>
									{currentDescription}
								</p>
							{/if}
						</div>
						<div class="flex shrink-0 items-center gap-2">
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
								class="btn btn-secondary btn-sm px-3 py-1"
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
							<TriangleAlert class="size-3 shrink-0" />
							<p>
								Modified: This tool has been customized. The description or name has been changed.
							</p>
						</div>
					{/if}

					{#if expandedTools[tool.id]}
						<div class="mt-2 flex flex-col gap-2">
							<div class="flex flex-col gap-1">
								<p class="text-xs text-muted-content">Tool name</p>
								<input class="text-input-filled flex-1 text-sm" bind:value={tool.overrideName} />
							</div>

							<div class="flex flex-col gap-1">
								<p class="text-xs text-muted-content">Description</p>
								<textarea
									class="text-input-filled h-24 resize-none text-xs"
									bind:value={tool.overrideDescription}
									placeholder="Enter tool description..."
								></textarea>
							</div>

							<div class="mt-2 flex justify-end">
								<button
									type="button"
									class="btn btn-sm btn-secondary px-3 py-1"
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
	<div class="bg-base-200 sticky bottom-0 left-0 mt-4 flex w-full justify-end gap-2 p-4">
		<button class="btn btn-secondary" onclick={handleCancel}>Cancel</button>
		<button
			class="btn btn-primary"
			disabled={hasBlockingToolNameErrors || prefixIssue?.severity === 'error'}
			onclick={() => {
				onSuccess?.();
				dialog?.close();
			}}>Confirm</button
		>
	</div>
</ResponsiveDialog>

<!-- Confirmation Dialog for Unsaved Changes -->
<ResponsiveDialog bind:this={confirmDialog} title="Discard Changes?" class="max-w-xl">
	<p class="text-muted-content mb-4 text-sm">
		You have unsaved changes for {configuringEntry?.manifest?.name ?? 'MCP Server'} configuration. Are
		you sure you want to discard these changes?
	</p>

	<div class="flex justify-end gap-3">
		<button class="btn btn-secondary" onclick={cancelDiscard}>Keep Editing</button>
		<button class="btn btn-error" onclick={confirmDiscard}> Discard Changes </button>
	</div>
</ResponsiveDialog>
