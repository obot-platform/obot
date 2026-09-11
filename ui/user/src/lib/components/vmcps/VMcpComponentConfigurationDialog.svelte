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
	import { profile } from '$lib/stores';
	import McpServerIcon from './McpServerIcon.svelte';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		onNext?: (
			configuration: VMCPConfigurationPolicy[],
			forceSingleUser: boolean
		) => void | Promise<void>;
		onClose?: () => void;
	}

	let { onNext, onClose }: Props = $props();

	interface PolicyDraft {
		field: MCPConfig;
		policy?: VMCPConfigurationPolicyType;
		value: string;
	}

	const POLICY_OPTIONS: { id: VMCPConfigurationPolicyType; label: string }[] = [
		{ id: 'fixed', label: 'Preconfigured' },
		{ id: 'userAllowed', label: 'Provided at connection' },
		{ id: 'prohibited', label: 'Ignore' }
	];

	function policyOptions(required?: boolean) {
		return required
			? POLICY_OPTIONS.filter((option) => option.id !== 'prohibited')
			: POLICY_OPTIONS;
	}

	function initialPolicy(
		field: MCPConfig,
		existing?: VMCPConfigurationPolicy
	): VMCPConfigurationPolicyType {
		if (profile.current.hasAdminAccess?.()) {
			const policy = existing?.policy ?? (field.required ? 'fixed' : 'prohibited');
			return field.required && policy === 'prohibited' ? 'fixed' : policy;
		}

		return 'fixed';
	}

	let dialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let entry = $state<MCPCatalogEntry>();
	let drafts = $state<PolicyDraft[]>([]);
	let highlighted = $state<string[]>([]);
	let error = $state<string>();
	let saving = $state(false);
	let forceSingleUser = $state(false);
	let hasUserAllowedNonHeaderConfiguration = $derived(
		drafts.some((draft) => draft.policy === 'userAllowed' && draft.field.usage !== 'header')
	);
	let submitLabel = $state('Next');
	let failureMessage = $state('Failed to add MCP server to vMCP.');

	let displayName = $derived(entry?.manifest.name || entry?.id || 'MCP server');
	let requiredDrafts = $derived(drafts.filter((draft) => draft.field.required));
	let optionalDrafts = $derived(drafts.filter((draft) => !draft.field.required));

	export function open(
		target: MCPCatalogEntry,
		options?: {
			configuration?: VMCPConfigurationPolicy[];
			forceSingleUser?: boolean;
			submitLabel?: string;
			errorMessage?: string;
		}
	) {
		entry = target;
		const existing = new Map((options?.configuration ?? []).map((policy) => [policy.key, policy]));
		drafts = catalogConfigurationFields(target)
			.toSorted((a, b) => Number(Boolean(b.required)) - Number(Boolean(a.required)))
			.map((field) => {
				const policy = existing.get(field.key);
				return {
					field,
					policy: initialPolicy(field, policy),
					value: policy?.value ?? field.value ?? ''
				};
			});
		forceSingleUser = !hasUserAllowedNonHeaderConfiguration && (options?.forceSingleUser ?? false);
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
		if (hasUserAllowedNonHeaderConfiguration) forceSingleUser = false;
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
			await onNext?.(
				configurationPayload(),
				!hasUserAllowedNonHeaderConfiguration && forceSingleUser
			);
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
		forceSingleUser = false;
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
				</span>
				{#if draft.field.description}
					<InfoTooltip text={draft.field.description} />
				{/if}
			</span>
			{#if profile.current.hasAdminAccess?.()}
				<select
					class="select select-sm w-48 shrink-0 bg-base-100 dark:bg-base-300 dark:border-base-400 border-base-300"
					aria-label={`${fieldLabel(draft.field)} policy`}
					value={draft.policy}
					disabled={saving}
					onchange={(event) =>
						setPolicy(index, event.currentTarget.value as VMCPConfigurationPolicyType)}
				>
					{#each policyOptions(draft.field.required) as option (option.id)}
						<option value={option.id}>{option.label}</option>
					{/each}
				</select>
			{/if}
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
				This field will be requested on user connection to the vMCP.
			</p>
		{/if}
	</div>
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
	<div class="p-4 pb-0 md:p-0">
		{#if drafts.length > 0}
			<p class="text-sm font-light mb-2">
				This MCP Server requires the following configurations to be set before it can be used.
				Choose how each configuration value for <b class="font-semibold text-base-content"
					>{displayName}</b
				>
				is provided.
			</p>
			<ul class="text-xs font-light mb-4 list-disc space-y-4 pl-5">
				<li>
					<b class="font-semibold">Preconfigured</b> - Set the value now. This value will be used automatically
					for every connection.
				</li>
				<li>
					<b class="font-semibold">Provided at connection</b> - Leave the value unset. Each user will
					be prompted to provide their own value when they connect.
				</li>
				<li>
					<b class="font-semibold">Ignore</b> - Ignore this field.
				</li>
			</ul>
		{/if}
		{#if error}
			<p class="notification-error mb-4 text-sm" role="alert">{error}</p>
		{/if}
		<div class="flex flex-col gap-3">
			{#each requiredDrafts as draft, index (draft.field.key)}
				{@render policyField(draft, index)}
			{/each}
			{#if requiredDrafts.length > 0 && optionalDrafts.length > 0}
				<div class="divider my-1 text-xs text-muted-content">Optional</div>
				<p class="text-xs font-light text-muted-content">
					These are additional optional fields for the MCP Server. Generally these can be ignored as
					they are often used for advanced use cases.
				</p>
			{/if}
			{#each optionalDrafts as draft, index (draft.field.key)}
				{@render policyField(draft, requiredDrafts.length + index)}
			{/each}
			{#if !hasUserAllowedNonHeaderConfiguration}
				<label class="flex items-center gap-2">
					<input
						type="checkbox"
						class="checkbox checkbox-sm"
						bind:checked={forceSingleUser}
						disabled={saving}
					/>
					<span>Force single-user</span>
				</label>
				<p class="text-xs font-light text-muted-content">
					Run a separate instance of this component for each user.
				</p>
			{/if}
		</div>
	</div>
	<div class="flex grow"></div>
	<div class="mt-4 flex justify-end gap-2 p-4 md:p-0 pt-0">
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
