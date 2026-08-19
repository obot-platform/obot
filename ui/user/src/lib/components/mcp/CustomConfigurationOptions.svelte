<script lang="ts">
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import IconButton from '$lib/components/primitives/IconButton.svelte';
	import type { MCPSubField } from '$lib/services';
	import InfoTooltip from '../InfoTooltip.svelte';
	import Label from './CatalogFormLabel.svelte';
	import { Plus, Trash2 } from '@lucide/svelte';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		id: string;
		data: MCPSubField & { secretBindingSource?: string };
		readonly?: boolean;
		isPrebuiltEntry?: boolean;
		showRequired?: boolean;
		showInvalid?: boolean;
		showTitle?: boolean;
		classes?: {
			optionInput?: string;
		};
	}

	let {
		id,
		data = $bindable(),
		readonly,
		isPrebuiltEntry,
		showRequired,
		showInvalid,
		showTitle,
		classes
	}: Props = $props();

	function handleAddOption() {
		data.options = [...(data.options || []), { name: '', value: '', description: '' }];
	}

	function isDuplicateOptionValue(value: string, index: number) {
		return Boolean(
			showInvalid &&
			value.trim() &&
			data.options?.some((option, optionIndex) => optionIndex < index && option.value === value)
		);
	}
</script>

<div class="flex flex-col gap-2">
	{#if showTitle}
		<div class="flex justify-between items-center gap-4">
			<p class="text-sm font-light flex items-center gap-1">
				Options
				<InfoTooltip text="Supply specific options that can be selected by the user." />
			</p>
			{#if (data.options ?? []).length === 0}
				{@render addOptionButton()}
			{/if}
		</div>
	{/if}
	{#if data.options}
		{#each data.options as option, i (i)}
			{@const missingOptionName = showRequired && !option.name.trim()}
			{@const missingOptionValue = showRequired && !option.value.trim()}
			{@const duplicateOptionValue = isDuplicateOptionValue(option.value, i)}
			<div
				class="border border-transparent bg-base-300 dark:bg-base-400/50 dark:border-base-400 p-4 rounded-md flex gap-4 items-center"
			>
				<div class="flex flex-col gap-4 grow">
					<div class="flex w-full flex-col gap-1" id={`${id}-option-name-container-${i}`}>
						<Label
							title="Name"
							forInput={`env-option-name-${id}-${i}`}
							required
							showError={missingOptionName}
						/>
						<input
							id={`env-option-name-${id}-${i}`}
							class={twMerge(
								'text-input-filled bg-base-100 w-full shadow-none',
								classes?.optionInput
							)}
							class:error={missingOptionName}
							bind:value={option.name}
							disabled={readonly || isPrebuiltEntry}
							aria-required={!readonly ? 'true' : undefined}
							aria-invalid={missingOptionName}
						/>
					</div>
					<div class="flex w-full flex-col gap-1" id={`${id}-option-value-container-${i}`}>
						<Label
							title="Value"
							forInput={`env-option-value-${id}-${i}`}
							required
							showError={missingOptionValue || duplicateOptionValue}
						/>
						<input
							id={`env-option-value-${id}-${i}`}
							class={twMerge(
								'text-input-filled bg-base-100 w-full shadow-none',
								classes?.optionInput
							)}
							class:error={missingOptionValue || duplicateOptionValue}
							bind:value={option.value}
							disabled={readonly || isPrebuiltEntry}
							aria-required={!readonly ? 'true' : undefined}
							aria-invalid={missingOptionValue || duplicateOptionValue}
							aria-errormessage={duplicateOptionValue
								? `env-option-value-${id}-${i}-error`
								: undefined}
						/>
						{#if duplicateOptionValue}
							<p id={`env-option-value-${id}-${i}-error`} class="text-xs text-error" role="alert">
								Option values must be unique.
							</p>
						{/if}
					</div>
					<div class="flex w-full flex-col gap-1" id={`${id}-option-description-container-${i}`}>
						<Label title="Description" forInput={`env-option-description-${id}-${i}`} />
						<input
							id={`env-option-description-${id}-${i}`}
							class={twMerge(
								'text-input-filled bg-base-100 w-full shadow-none',
								classes?.optionInput
							)}
							bind:value={option.description}
							disabled={readonly || isPrebuiltEntry}
						/>
					</div>
				</div>
				{#if !readonly && !isPrebuiltEntry}
					<div
						use:tooltip={{
							text: data.options?.length === 1 ? 'At least one option is required.' : undefined
						}}
					>
						<IconButton
							id={`env-option-remove-${id}-${i}`}
							variant="danger"
							onclick={() => {
								data.options?.splice(i, 1);
							}}
							disabled={(data.options ?? []).length === 1}
						>
							<Trash2 class="size-4" />
						</IconButton>
					</div>
				{/if}
			</div>
		{/each}
	{:else if readonly || isPrebuiltEntry}
		<p class="text-muted-content text-sm font-light">No included options.</p>
	{/if}
	{#if (data.options ?? []).length > 0}
		<div class="flex justify-end w-full">
			{@render addOptionButton()}
		</div>
	{/if}
</div>

{#snippet addOptionButton()}
	{#if !readonly && !isPrebuiltEntry}
		<button
			type="button"
			class="btn btn-secondary flex items-center gap-1 btn-sm"
			onclick={handleAddOption}
		>
			<Plus class="size-4" /> Option
		</button>
	{/if}
{/snippet}
