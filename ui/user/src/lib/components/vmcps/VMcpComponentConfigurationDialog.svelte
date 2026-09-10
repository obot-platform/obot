<script lang="ts">
	import InfoTooltip from '$lib/components/InfoTooltip.svelte';
	import ResponsiveDialog from '$lib/components/ResponsiveDialog.svelte';
	import Select from '$lib/components/Select.svelte';
	import SensitiveInput from '$lib/components/SensitiveInput.svelte';
	import {
		configurationSelectOptions,
		isMissingRequiredConfigurationField,
		selectedConfigurationOption
	} from '$lib/components/mcp/configurationOptions';
	import Loading from '$lib/icons/Loading.svelte';
	import type {
		MCPCatalogEntry,
		MCPConfig,
		VMCPConfigurationPolicy,
		VMCPConfigurationPolicyType
	} from '$lib/services';
	import { catalogConfigurationFields } from '$lib/services/vmcps/utils';
	import McpServerIcon from './McpServerIcon.svelte';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		onNext?: (configuration: VMCPConfigurationPolicy[]) => void | Promise<void>;
		onClose?: () => void;
	}

	let { onNext, onClose }: Props = $props();

	interface PolicyDraft {
		field: MCPConfig;
		policy?: VMCPConfigurationPolicyType;
		value: string;
	}

	const POLICY_OPTIONS: { id: VMCPConfigurationPolicyType; label: string }[] = [
		{ id: 'fixed', label: 'Fixed' },
		{ id: 'userAllowed', label: 'User-Supplied' },
		{ id: 'prohibited', label: 'Prohibited' }
	];

	let dialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let entry = $state<MCPCatalogEntry>();
	let drafts = $state<PolicyDraft[]>([]);
	let highlighted = $state<string[]>([]);
	let error = $state<string>();
	let saving = $state(false);
	let submitLabel = $state('Next');
	let failureMessage = $state('Failed to add MCP server to vMCP.');

	let displayName = $derived(entry?.manifest.name || entry?.id || 'MCP server');
	let envDrafts = $derived(
		drafts.filter((draft) => draft.field.usage !== 'header' && !isFileField(draft.field))
	);
	let headerDrafts = $derived(drafts.filter((draft) => draft.field.usage === 'header'));
	let fileDrafts = $derived(
		drafts.filter((draft) => draft.field.usage !== 'header' && isFileField(draft.field))
	);

	export function open(
		target: MCPCatalogEntry,
		options?: {
			configuration?: VMCPConfigurationPolicy[];
			submitLabel?: string;
			errorMessage?: string;
		}
	) {
		entry = target;
		const existing = new Map((options?.configuration ?? []).map((policy) => [policy.key, policy]));
		drafts = catalogConfigurationFields(target).map((field) => {
			const policy = existing.get(field.key);
			return {
				field,
				policy: policy?.policy ?? 'fixed',
				value: policy?.value ?? field.value ?? ''
			};
		});
		submitLabel = options?.submitLabel ?? 'Next';
		failureMessage = options?.errorMessage ?? 'Failed to add MCP server to vMCP.';
		highlighted = [];
		error = undefined;
		saving = false;
		dialog?.open();
	}

	export function close() {
		dialog?.close();
	}

	function isFileField(field: MCPConfig) {
		return field.usage === 'file' || field.usage === 'dynamicFile';
	}

	function fieldLabel(field: MCPConfig) {
		return field.name || field.key;
	}

	function setPolicy(index: number, policy: VMCPConfigurationPolicyType) {
		drafts[index].policy = policy;
		error = undefined;
		highlighted = highlighted.filter((key) => key !== drafts[index].field.key);
	}

	function configurationPayload(): VMCPConfigurationPolicy[] {
		return drafts.map((draft) => ({
			key: draft.field.key,
			policy: draft.policy,
			...(draft.policy === 'fixed' ? { value: draft.value } : {})
		}));
	}

	function validate() {
		const missingPolicy = drafts.some((draft) => !draft.policy);
		const missingFixed: string[] = [];
		for (const draft of drafts) {
			if (draft.policy !== 'fixed') continue;
			if (isMissingRequiredConfigurationField({ ...draft.field, value: draft.value }, true)) {
				missingFixed.push(draft.field.key);
			}
		}
		highlighted = missingFixed;
		if (missingPolicy) {
			error = 'Select a policy for each configuration field.';
			return false;
		}
		if (missingFixed.length > 0) {
			error = 'Please complete all fixed configuration fields with valid values.';
			return false;
		}
		error = undefined;
		return true;
	}

	async function handleNext() {
		if (saving || !validate()) return;
		saving = true;
		try {
			await onNext?.(configurationPayload());
			dialog?.close();
		} catch {
			error = failureMessage;
		} finally {
			saving = false;
		}
	}

	function handleClose() {
		if (saving) return;
		entry = undefined;
		drafts = [];
		error = undefined;
		highlighted = [];
		onClose?.();
	}
</script>

{#snippet policyField(draft: PolicyDraft, index: number)}
	{@const highlightRequired = highlighted.includes(draft.field.key)}
	<div class="flex flex-col gap-2 rounded-lg border border-base-300 p-3 dark:border-base-400">
		<div class="flex items-center justify-between gap-3">
			<span class="flex min-w-0 items-center gap-2">
				<span id={`${draft.field.key}-label`} class={highlightRequired ? 'text-error' : ''}>
					{fieldLabel(draft.field)}
					{#if !draft.field.required}
						<span class="text-muted-content">(optional)</span>
					{/if}
				</span>
				{#if draft.field.description}
					<InfoTooltip text={draft.field.description} />
				{/if}
			</span>
			<select
				class="select select-sm w-36 shrink-0 bg-base-200 border-base-300"
				aria-label={`${fieldLabel(draft.field)} policy`}
				value={draft.policy}
				disabled={saving}
				onchange={(event) =>
					setPolicy(index, event.currentTarget.value as VMCPConfigurationPolicyType)}
			>
				{#each POLICY_OPTIONS as option (option.id)}
					<option value={option.id}>{option.label}</option>
				{/each}
			</select>
		</div>
		{#if draft.policy === 'fixed'}
			{#if draft.field.options?.length}
				<Select
					id={`fixed-${draft.field.key}`}
					class="bg-base-200 border-base-300 dark:border-base-400 border"
					options={configurationSelectOptions(draft.field.options)}
					selected={draft.value}
					placeholder="Select a value"
					ariaLabelledby={`${draft.field.key}-label`}
					onSelect={(option) => (draft.value = option.value)}
					onClear={() => (draft.value = '')}
				/>
			{:else if draft.field.sensitive}
				<SensitiveInput
					error={highlightRequired}
					name={fieldLabel(draft.field)}
					bind:value={draft.value}
					textarea={isFileField(draft.field)}
					growable
					disabled={saving}
				/>
			{:else if isFileField(draft.field)}
				<textarea
					id={`fixed-${draft.field.key}`}
					bind:value={draft.value}
					rows="8"
					disabled={saving}
					class={twMerge(
						'text-input-filled h-32 min-h-32 resize-y overflow-auto whitespace-pre-wrap',
						highlightRequired && 'border-error bg-error/20 ring-error focus:ring-1'
					)}
				></textarea>
			{:else}
				<input
					type="text"
					id={`fixed-${draft.field.key}`}
					bind:value={draft.value}
					disabled={saving}
					class={twMerge(
						'text-input-filled',
						highlightRequired && 'border-error bg-error/20 ring-error focus:ring-1'
					)}
				/>
			{/if}
			{#if selectedConfigurationOption({ ...draft.field, value: draft.value })?.description}
				<p class="text-muted-content text-xs font-light break-all">
					{selectedConfigurationOption({ ...draft.field, value: draft.value })?.description}
				</p>
			{/if}
		{:else if draft.policy === 'userAllowed'}
			<p class="text-muted-content italic text-sm font-light break-all">
				This will be requested on user connection to the vMCP.
			</p>
		{:else}
			<p class="text-muted-content italic text-sm font-light break-all">
				This field is prohibited from being modified.
			</p>
		{/if}
	</div>
{/snippet}

{#snippet fieldGroup(items: PolicyDraft[])}
	{#if items.length > 0}
		{#each items as draft (draft.field.key)}
			{@render policyField(draft, drafts.indexOf(draft))}
		{/each}
	{/if}
{/snippet}

<ResponsiveDialog
	bind:this={dialog}
	animate="slide"
	class="max-w-lg"
	title="Supply Configuration"
	onClose={handleClose}
	hideClose
>
	{#snippet titleContent()}
		{#if entry?.manifest.icon}
			<McpServerIcon icon={entry?.manifest.icon} />
		{/if}
		Configure {displayName}
	{/snippet}
	<p class="text-sm font-light text-muted-content mb-4">
		Choose how each configuration value for <b class="font-semibold text-base-content"
			>{displayName}</b
		>
		is provided. Fixed values are stored on the vMCP. User-supplied values are requested when someone
		connects.
	</p>
	{#if error}
		<p class="notification-error mb-4 text-sm" role="alert">{error}</p>
	{/if}
	<div class="flex max-h-[60dvh] flex-col gap-3 overflow-y-auto pr-1">
		{@render fieldGroup(envDrafts)}
		{@render fieldGroup(headerDrafts)}
		{@render fieldGroup(fileDrafts)}
	</div>
	<div class="mt-4 flex justify-end gap-2">
		<button class="btn btn-ghost btn-sm text-xs" onclick={() => dialog?.close()} disabled={saving}>
			Cancel
		</button>
		<button class="btn btn-primary btn-sm text-xs" onclick={handleNext} disabled={saving}>
			{#if saving}
				<Loading class="text-primary-content size-4" />
			{:else}
				{submitLabel}
			{/if}
		</button>
	</div>
</ResponsiveDialog>
