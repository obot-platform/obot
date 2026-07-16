<script lang="ts">
	import type { MCPAllowedSecretBindingTarget, MCPCatalogEntryFieldManifest } from '$lib/services';
	import { hasSecretBinding } from '$lib/services/user/mcp';
	import Select from '../Select.svelte';
	import IconButton from '../primitives/IconButton.svelte';
	import CustomConfigurationFieldset from './CustomConfigurationFieldset.svelte';
	import { Plus, Trash2 } from '@lucide/svelte';
	import type { Snippet } from 'svelte';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		readonly?: boolean;
		config?: MCPCatalogEntryFieldManifest[];
		secretBoundHeaders?: MCPCatalogEntryFieldManifest[];
		serverUserType?: 'singleUser' | 'multiUser';
		isPrebuiltEntry?: boolean;
		secretBindingTargets?: MCPAllowedSecretBindingTarget[];
		overrideEnvField?: string[];
		overrideEnvTemplate?: Snippet<[{ config: MCPCatalogEntryFieldManifest; index: number }]>;
		showRequired?: boolean;
	}

	let {
		readonly,
		config = $bindable(),
		secretBoundHeaders,
		serverUserType,
		isPrebuiltEntry,
		secretBindingTargets,
		overrideEnvField,
		overrideEnvTemplate,
		showRequired
	}: Props = $props();

	// Separate secret-bound fields from user-configurable fields, preserving
	// original indices so bind:value still points at the right config slot.
	const indexedConfig = $derived((config ?? []).map((item, i) => ({ item, index: i })));
	const canBindSecrets = $derived(secretBindingTargets !== undefined);
	const userConfig = $derived(
		canBindSecrets ? indexedConfig : indexedConfig.filter(({ item }) => !hasSecretBinding(item))
	);
	const secretBoundEnvs = $derived(
		canBindSecrets ? [] : indexedConfig.filter(({ item }) => hasSecretBinding(item))
	);
	const allSecretBound = $derived([
		...secretBoundEnvs.map(({ item }) => ({ item, source: 'env' as const })),
		...(secretBoundHeaders ?? []).map((item) => ({ item, source: 'header' as const }))
	]);

	const inputClass = 'text-input-filled bg-base-100 w-full shadow-none';
</script>

<!-- Environment Variables / Files Section -->
{#if !readonly || (readonly && userConfig.length > 0)}
	<div
		class="dark:bg-base-200 dark:border-base-400 bg-base-100 flex flex-col gap-4 rounded-lg border border-transparent p-4 shadow-sm"
	>
		<h4 class="text-sm font-semibold">
			{serverUserType === 'singleUser' ? 'User Supplied Configuration' : 'Configuration'}
		</h4>

		{#each userConfig as { item, index: i } (i)}
			{#if overrideEnvField?.includes(item.key) && overrideEnvTemplate}
				{@render overrideEnvTemplate({ config: config![i], index: i })}
			{:else}
				<div
					class="dark:border-base-400 bg-base-300 flex w-full items-center gap-4 rounded-lg border border-transparent p-4"
				>
					<div class="flex w-full flex-col gap-4">
						<div class="flex w-full flex-col gap-1">
							<label for={`env-type-${i}`} class="text-sm font-light">Type</label>
							<Select
								class="dark:border-base-400 bg-base-100 border border-transparent"
								classes={{
									root: 'flex grow'
								}}
								options={[
									{ label: 'Environment Variable', id: 'environment_variable_type' },
									{ label: 'File', id: 'file_type' }
								]}
								disabled={readonly || isPrebuiltEntry}
								selected={config![i].file ? 'file_type' : 'environment_variable_type'}
								onSelect={(option) => {
									if (option.id === 'file_type') {
										config![i].file = true;
									} else {
										config![i].file = false;
									}
								}}
								id={`env-type-${i}`}
							/>
						</div>

						<p class="text-muted-content text-xs font-light">
							{#if config![i].file}
								The value {serverUserType === 'singleUser' ? 'the user supplies' : 'you provide'} will
								be written to a file. An environment variable will be created using the name you specify
								in the Key field and its value will be the path to that file. This environment variable
								will be set inside your deployment and you can reference it in the arguments section above
								using the syntax ${'{KEY_NAME}'}.
							{:else}
								{serverUserType === 'singleUser'
									? 'The value the user supplies'
									: 'The value you provide'} will be set as an environment variable using the name you
								specify in the Key field. This environment variable will be set inside your deployment
								and you can reference it in the arguments section above using the syntax ${'{KEY_NAME}'}.
							{/if}
						</p>

						<CustomConfigurationFieldset
							id={`env-${i}`}
							bind:data={config![i]}
							{serverUserType}
							{readonly}
							{isPrebuiltEntry}
							{secretBindingTargets}
							classes={{
								input: inputClass
							}}
							{showRequired}
						/>
					</div>
					{#if !readonly && !isPrebuiltEntry}
						<IconButton
							variant="danger"
							onclick={() => {
								config!.splice(i, 1);
							}}
							disabled={isPrebuiltEntry}
						>
							<Trash2 class="size-4" />
						</IconButton>
					{/if}
				</div>
			{/if}
		{/each}

		{#if !readonly && !isPrebuiltEntry}
			<div class="flex justify-end">
				<button
					class="btn btn-secondary btn-sm flex items-center gap-1 text-xs"
					type="button"
					onclick={() => {
						if (config) {
							config.push({
								key: '',
								description: '',
								name: '',
								value: '',
								required: false,
								sensitive: false,
								file: false
							});
						}
					}}
				>
					<Plus class="size-4" />
					{serverUserType === 'singleUser' ? 'User Configuration' : 'Configuration'}
				</button>
			</div>
		{/if}
	</div>
{/if}

<!-- Secret-bound Configuration Section -->
{#if allSecretBound.length > 0}
	<div
		class="dark:bg-base-200 dark:border-base-400 bg-base-100 flex flex-col gap-4 rounded-lg border border-transparent p-4 shadow-sm"
	>
		<h4 class="text-sm font-semibold">Secret-bound Configuration</h4>

		{#each allSecretBound as { item, source }, sbIdx (`${source}:${item.key}`)}
			<div
				class="dark:border-base-400 bg-base-300 flex w-full items-center gap-4 rounded-lg border border-transparent p-4"
			>
				<div class="flex w-full flex-col gap-4">
					<div class="flex w-full flex-col gap-1">
						<label for={`sb-${sbIdx}-type`} class="text-sm font-light">Type</label>
						<input
							class={inputClass}
							id={`sb-${sbIdx}-type`}
							value={source === 'header' ? 'Header' : item.file ? 'File' : 'Environment Variable'}
							disabled
						/>
					</div>

					<div class="flex w-full flex-col gap-1">
						<label for={`sb-${sbIdx}-name`} class="text-sm font-light">Name</label>
						<input
							class={inputClass}
							id={`sb-${sbIdx}-name`}
							value={item.name || item.key}
							disabled
						/>
					</div>

					{#if item.description}
						<div class="flex w-full flex-col gap-1">
							<label for={`sb-${sbIdx}-description`} class="text-sm font-light">Description</label>
							<input
								class={inputClass}
								id={`sb-${sbIdx}-description`}
								value={item.description}
								disabled
							/>
						</div>
					{/if}

					<div class="flex w-full flex-col gap-1">
						<label for={`sb-${sbIdx}-key`} class="text-sm font-light">Key</label>
						<input class={inputClass} id={`sb-${sbIdx}-key`} value={item.key} disabled />
					</div>

					{#if item.secretBinding?.name && item.secretBinding?.key}
						<div class="flex w-full flex-col gap-1">
							<label for={`sb-${sbIdx}-secret`} class="text-sm font-light">Secret</label>
							<input
								class={twMerge(inputClass, 'font-mono')}
								id={`sb-${sbIdx}-secret`}
								value={`${item.secretBinding?.name} / ${item.secretBinding?.key}`}
								disabled
							/>
						</div>
					{/if}

					<div class="flex flex-wrap gap-2">
						{#if item.sensitive}
							<span class="badge badge-secondary badge-xs">sensitive</span>
						{/if}
						{#if item.required}
							<span class="badge badge-secondary badge-xs">required</span>
						{/if}
						{#if source === 'env' && item.file}
							<span class="badge badge-secondary badge-xs">file</span>
						{/if}
						{#if source === 'env' && item.dynamicFile}
							<span class="badge badge-secondary badge-xs">dynamic</span>
						{/if}
					</div>
				</div>
			</div>
		{/each}
	</div>
{/if}
