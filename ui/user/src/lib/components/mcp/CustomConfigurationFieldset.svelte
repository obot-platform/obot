<script lang="ts">
	import type { MCPAllowedSecretBindingTarget, MCPSubField } from '$lib/services';
	import { version } from '$lib/stores';
	import Select from '../Select.svelte';
	import Toggle from '../Toggle.svelte';
	import SecretBindingPicker from './SecretBindingPicker.svelte';
	import { untrack } from 'svelte';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		data: MCPSubField & { secretBindingSource?: string };
		id: string;
		serverUserType?: 'singleUser' | 'multiUser';
		readonly?: boolean;
		isPrebuiltEntry?: boolean;
		secretBindingTargets?: MCPAllowedSecretBindingTarget[];
		classes?: {
			input?: string;
		};
		showRequired?: boolean;
	}

	let {
		id,
		data = $bindable(),
		serverUserType,
		readonly,
		isPrebuiltEntry,
		secretBindingTargets,
		classes,
		showRequired
	}: Props = $props();

	function usesSecretBindingSource(field: {
		secretBinding?: unknown;
		secretBindingSource?: string;
	}) {
		return Boolean(field.secretBinding) || field.secretBindingSource === 'secret';
	}

	let selectedType = $state(
		untrack(() =>
			(data.value?.length ?? 0) > 0 || usesSecretBindingSource(data) ? 'static' : 'user_supplied'
		)
	);

	let missingKey = $derived(showRequired && !data.key.trim());
	let missingName = $derived(showRequired && !data.name.trim());
	let missingValue = $derived(showRequired && !data.value?.trim());

	$effect(() => {
		if (data && usesSecretBindingSource(data)) {
			data.sensitive = true;
		}
	});
</script>

{#if serverUserType === 'singleUser'}
	<p class="text-muted-content text-xs font-light">
		The Name and Description fields will be displayed to the user when configuring this server. The
		Key field will not.
	</p>

	{@render keyInput()}
	{@render nameAndDescriptionInputs()}
	<div class="flex gap-8">
		<label class="flex items-center gap-2">
			<input type="checkbox" bind:checked={data.sensitive} disabled={readonly || isPrebuiltEntry} />
			<span class="text-sm">Sensitive</span>
		</label>
		<label class="flex items-center gap-2">
			<input type="checkbox" bind:checked={data.required} disabled={readonly || isPrebuiltEntry} />
			<span class="text-sm">Required</span>
		</label>
	</div>
{:else}
	<div class="flex w-full flex-col gap-1">
		{@render keyInput()}

		{#if isPrebuiltEntry && data.description}
			<p class="text-muted-content text-xs font-light break-all">
				{data.description}
			</p>
		{/if}
	</div>

	<div class="flex w-full flex-col gap-1" id={`${id}-value-type-container`}>
		{@render label('Value', `env-value-type-${id}`)}
		<Select
			class="bg-base-100 dark:border-base-400 border border-transparent shadow-none"
			classes={{
				root: 'flex grow'
			}}
			options={[
				{ label: 'Static', id: 'static' },
				{ label: 'User-Supplied', id: 'user_supplied' }
			]}
			selected={selectedType}
			onSelect={(option) => {
				if (!data) return;
				selectedType = option.id;

				// Reset state when switching between static and user-supplied modes.
				data.required = option.id === 'static';
				data.name = '';
				data.value = '';
				data.description = '';
				data.sensitive = false;

				if (option.id === 'user_supplied') {
					data.secretBinding = undefined;
					data.secretBindingSource = 'value';
				}
			}}
			readonly={readonly || isPrebuiltEntry}
			id={`env-value-type-${id}`}
		/>
	</div>

	{#if !isPrebuiltEntry && selectedType === 'user_supplied'}
		{@render nameAndDescriptionInputs()}
	{/if}
	{#if selectedType === 'static'}
		{#if secretBindingTargets && !version.current.hideK8sDetails}
			<SecretBindingPicker bind:field={data} targets={secretBindingTargets} {readonly} />
		{/if}
		{#if !usesSecretBindingSource(data)}
			<div class="flex w-full flex-col gap-1">
				<label for={`env-value-${id}`} class="sr-only">Static Value</label>
				{#if data.file}
					<textarea
						id={`env-value-${id}`}
						class="text-input-filled bg-base-100 min-h-24 w-full resize-y shadow-none"
						class:error={missingValue}
						bind:value={data.value}
						disabled={readonly}
						rows={(data.value ?? '').split('\n').length + 1}
					></textarea>
				{:else}
					<input
						id={`env-value-${id}`}
						class="text-input-filled bg-base-100 w-full shadow-none"
						class:error={missingValue}
						bind:value={data.value}
						placeholder="e.g. 123abcdef456"
						disabled={readonly}
						type={data.sensitive ? 'password' : 'text'}
					/>
				{/if}
			</div>
		{/if}
	{/if}
	<div class="flex w-full">
		{#if !usesSecretBindingSource(data)}
			<Toggle
				classes={{ label: 'text-sm text-inherit' }}
				disabled={readonly || isPrebuiltEntry}
				label="Sensitive"
				labelInline
				checked={!!data.sensitive}
				onChange={(checked) => {
					if (data) {
						data.sensitive = checked;
					}
				}}
			/>
			{#if selectedType !== 'static'}
				<div class="divider divider-horizontal"></div>
			{/if}
		{/if}
		{#if selectedType !== 'static'}
			<Toggle
				classes={{ label: 'text-sm text-inherit' }}
				disabled={readonly || isPrebuiltEntry}
				label="Required"
				labelInline
				checked={!!data.required}
				onChange={(checked) => {
					if (data) {
						data.required = checked;
					}
				}}
			/>
		{/if}
	</div>
{/if}

{#snippet label(title: string, forInput: string, required?: boolean, showError?: boolean)}
	<label for={forInput} class={twMerge('text-sm font-light', showError && 'text-error')}>
		{title}
		{#if !readonly && required}
			<span class={showError ? 'text-error' : ''} aria-hidden="true">*</span>
			<span class="sr-only">(required)</span>
		{/if}
	</label>
{/snippet}

{#snippet keyInput()}
	<div class="flex w-full flex-col gap-1" id={`${id}-key-container`}>
		{@render label('Key', `env-key-${id}`, true, missingKey)}
		<input
			id={`env-key-${id}`}
			class={classes?.input}
			class:error={missingKey}
			bind:value={data.key}
			placeholder="e.g. CUSTOM_API_KEY"
			disabled={readonly || isPrebuiltEntry}
		/>
	</div>
{/snippet}

{#snippet nameAndDescriptionInputs()}
	<div class="flex w-full flex-col gap-1" id={`${id}-name-container`}>
		{@render label('Name', `env-name-${id}`, true, missingName)}
		<input
			id={`env-name-${id}`}
			class={classes?.input}
			class:error={missingName}
			bind:value={data.name}
			disabled={readonly || isPrebuiltEntry}
		/>
	</div>
	<div class="flex w-full flex-col gap-1" id={`${id}-description-container`}>
		{@render label('Description', `env-description-${id}`, false)}
		<input
			id={`env-description-${id}`}
			class={classes?.input}
			bind:value={data.description}
			disabled={readonly || isPrebuiltEntry}
		/>
	</div>
{/snippet}
