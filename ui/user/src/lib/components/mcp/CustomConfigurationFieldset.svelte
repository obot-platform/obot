<script lang="ts">
	import type { MCPAllowedSecretBindingTarget, MCPSubField } from '$lib/services';
	import { version } from '$lib/stores';
	import Select from '../Select.svelte';
	import Toggle from '../Toggle.svelte';
	import Label from './CatalogFormLabel.svelte';
	import CustomConfigurationOptions from './CustomConfigurationOptions.svelte';
	import SecretBindingPicker from './SecretBindingPicker.svelte';
	import { untrack } from 'svelte';

	interface Props {
		data: MCPSubField & { secretBindingSource?: string };
		id: string;
		serverUserType?: 'singleUser' | 'multiUser';
		readonly?: boolean;
		isPrebuiltEntry?: boolean;
		secretBindingTargets?: MCPAllowedSecretBindingTarget[];
		classes?: {
			input?: string;
			optionInput?: string;
		};
		showRequired?: boolean;
		showInvalid?: boolean;
		urlTemplateVariable?: boolean;
	}

	let {
		id,
		data = $bindable(),
		serverUserType,
		readonly,
		isPrebuiltEntry,
		secretBindingTargets,
		classes,
		showRequired,
		showInvalid,
		urlTemplateVariable = false
	}: Props = $props();

	function usesSecretBindingSource(field: {
		secretBinding?: unknown;
		secretBindingSource?: string;
	}) {
		return Boolean(field.secretBinding) || field.secretBindingSource === 'secret';
	}

	let selectedType = $state(
		untrack(() =>
			data.options && data.options.length > 0
				? 'options'
				: (data.value?.length ?? 0) > 0 || usesSecretBindingSource(data)
					? 'static'
					: 'user_supplied'
		)
	);

	let missingKey = $derived(showRequired && !data.key.trim());
	let missingName = $derived(showRequired && !data.name.trim());
	let missingValue = $derived(
		showRequired && !data.value?.trim() && (data.options ?? []).length === 0
	);

	$effect(() => {
		if (urlTemplateVariable) {
			data.secretBinding = undefined;
			data.secretBindingSource = 'value';
			data.required = true;
			data.sensitive = false;
			data.file = false;
		} else if (data && usesSecretBindingSource(data)) {
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

	<CustomConfigurationOptions
		bind:data
		{id}
		{readonly}
		{isPrebuiltEntry}
		{showRequired}
		{showInvalid}
		{classes}
		showTitle
	/>

	{#if !urlTemplateVariable}
		<div class="divider my-0"></div>
		<div class="flex gap-8">
			<label class="flex items-center gap-2">
				<input
					type="checkbox"
					bind:checked={data.sensitive}
					disabled={readonly || isPrebuiltEntry}
				/>
				<span class="text-sm">Sensitive</span>
			</label>
			<label class="flex items-center gap-2">
				<input
					type="checkbox"
					bind:checked={data.required}
					disabled={readonly || isPrebuiltEntry}
				/>
				<span class="text-sm">Required</span>
			</label>
		</div>
	{/if}
{:else}
	<div class="flex w-full flex-col gap-1">
		{@render keyInput()}

		{#if isPrebuiltEntry && data.description}
			<p class="text-muted-content text-xs font-light break-all">
				{data.description}
			</p>
		{/if}
	</div>

	{#if !urlTemplateVariable}
		<div class="flex w-full flex-col gap-1" id={`${id}-value-type-container`}>
			<Label title="Value" forInput={`env-value-type-${id}`} required />
			<Select
				class="bg-base-100 dark:border-base-400 border border-transparent shadow-none"
				classes={{
					root: 'flex grow'
				}}
				options={[
					{ label: 'User-Supplied', id: 'user_supplied' },
					{ label: 'Static', id: 'static' },
					{ label: 'Options', id: 'options' }
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

					if (option.id === 'user_supplied' || option.id === 'options') {
						data.secretBinding = undefined;
						data.secretBindingSource = 'value';
					}

					if (option.id === 'options') {
						data.options = [{ name: '', value: '', description: '' }];
					} else {
						data.options = undefined;
					}
				}}
				readonly={readonly || isPrebuiltEntry}
				id={`env-value-type-${id}`}
			/>
		</div>
	{/if}

	{#if !isPrebuiltEntry && selectedType === 'user_supplied'}
		{@render nameAndDescriptionInputs()}
	{/if}
	{#if selectedType === 'static'}
		{#if secretBindingTargets && !version.current.hideK8sDetails}
			<SecretBindingPicker
				bind:field={data}
				targets={secretBindingTargets}
				{readonly}
				{showRequired}
			/>
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
						aria-required={!readonly ? 'true' : undefined}
						aria-invalid={missingValue}
					></textarea>
				{:else}
					<input
						id={`env-value-${id}`}
						class="text-input-filled bg-base-100 w-full shadow-none"
						class:error={missingValue}
						bind:value={data.value}
						placeholder="e.g. 123abcdef456"
						disabled={readonly || (data.options ?? []).length > 0}
						type={data.sensitive ? 'password' : 'text'}
						aria-required={!readonly ? 'true' : undefined}
						aria-invalid={missingValue}
					/>
				{/if}
			</div>
		{/if}
	{:else if selectedType === 'options'}
		<CustomConfigurationOptions
			bind:data
			{id}
			{readonly}
			{isPrebuiltEntry}
			{showRequired}
			{showInvalid}
			{classes}
		/>
	{/if}
	{#if !urlTemplateVariable}
		<div class="divider my-0"></div>
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
{/if}

{#snippet keyInput()}
	<div class="flex w-full flex-col gap-1" id={`${id}-key-container`}>
		<Label title="Key" forInput={`env-key-${id}`} required showError={missingKey} />
		<input
			id={`env-key-${id}`}
			class={classes?.input}
			class:error={missingKey}
			bind:value={data.key}
			placeholder="e.g. CUSTOM_API_KEY"
			disabled={readonly || isPrebuiltEntry}
			aria-required={!readonly ? 'true' : undefined}
			aria-invalid={missingKey}
		/>
	</div>
{/snippet}

{#snippet nameAndDescriptionInputs()}
	<div class="flex w-full flex-col gap-1" id={`${id}-name-container`}>
		<Label title="Name" forInput={`env-name-${id}`} required showError={missingName} />
		<input
			id={`env-name-${id}`}
			class={classes?.input}
			class:error={missingName}
			bind:value={data.name}
			disabled={readonly || isPrebuiltEntry}
			aria-required={!readonly ? 'true' : undefined}
			aria-invalid={missingName}
		/>
	</div>
	<div class="flex w-full flex-col gap-1" id={`${id}-description-container`}>
		<Label title="Description" forInput={`env-description-${id}`} />
		<input
			id={`env-description-${id}`}
			class={classes?.input}
			bind:value={data.description}
			disabled={readonly || isPrebuiltEntry}
		/>
	</div>
{/snippet}
