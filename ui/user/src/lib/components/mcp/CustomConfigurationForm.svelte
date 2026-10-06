<script lang="ts">
	import { CATALOG_SERVER_FIELD_IDS } from '$lib/constants';
	import { m } from '$lib/i18n';
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
		showInvalid?: boolean;
		urlTemplateVariables?: boolean;
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
		showRequired,
		showInvalid,
		urlTemplateVariables = false
	}: Props = $props();

	// Separate secret-bound fields from user-configurable fields, preserving
	// original indices so bind:value still points at the right config slot.
	const indexedConfig = $derived((config ?? []).map((item, i) => ({ item, index: i })));
	const canBindSecrets = $derived(secretBindingTargets !== undefined);
	const userConfig = $derived(
		urlTemplateVariables
			? indexedConfig
			: canBindSecrets
				? indexedConfig
				: indexedConfig.filter(({ item }) => !hasSecretBinding(item))
	);
	const secretBoundEnvs = $derived(
		urlTemplateVariables || canBindSecrets
			? []
			: indexedConfig.filter(({ item }) => hasSecretBinding(item))
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
		id={CATALOG_SERVER_FIELD_IDS.configuration}
	>
		<h4 class="text-sm font-semibold">
			{urlTemplateVariables
				? m.mcps_catalog_custom_config_url_template_variables()
				: serverUserType === 'singleUser'
					? m.mcps_catalog_custom_config_user_supplied()
					: m.mcps_catalog_config_heading()}
		</h4>
		{#if urlTemplateVariables}
			<p class="text-muted-content text-xs font-light">
				{m.mcps_catalog_custom_config_url_template_description({ placeholder: '${VARIABLE}' })}
			</p>
		{/if}

		{#each userConfig as { item, index: i } (i)}
			{#if overrideEnvField?.includes(item.key) && overrideEnvTemplate}
				{@render overrideEnvTemplate({ config: config![i], index: i })}
			{:else}
				<div
					class="dark:border-base-400 bg-base-200 dark:bg-base-300 flex w-full items-start gap-4 rounded-lg border border-transparent p-4"
				>
					<div class="flex w-full flex-col gap-4">
						{#if !urlTemplateVariables}
							<div class="flex w-full flex-col gap-1">
								<label for={`env-type-${i}`} class="text-sm font-light">{m.core_type()}</label>
								<Select
									class="dark:border-base-400 bg-base-100 border border-transparent"
									classes={{
										root: 'flex grow'
									}}
									options={[
										{ label: m.mcps_catalog_config_usage_env(), id: 'environment_variable_type' },
										{ label: m.mcps_catalog_config_usage_file(), id: 'file_type' }
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
									{serverUserType === 'singleUser'
										? m.mcps_catalog_custom_config_file_help_user({ syntax: '${KEY_NAME}' })
										: m.mcps_catalog_custom_config_file_help_admin({ syntax: '${KEY_NAME}' })}
								{:else}
									{serverUserType === 'singleUser'
										? m.mcps_catalog_custom_config_env_help_user({ syntax: '${KEY_NAME}' })
										: m.mcps_catalog_custom_config_env_help_admin({ syntax: '${KEY_NAME}' })}
								{/if}
							</p>
						{/if}

						<CustomConfigurationFieldset
							id={`${CATALOG_SERVER_FIELD_IDS.env}-${i}`}
							bind:data={config![i]}
							{serverUserType}
							{readonly}
							{isPrebuiltEntry}
							{secretBindingTargets}
							classes={{
								input: inputClass
							}}
							{showRequired}
							{showInvalid}
							urlTemplateVariable={urlTemplateVariables}
						/>
					</div>
					{#if !readonly && !isPrebuiltEntry}
						<IconButton
							class="mt-6"
							id={`${CATALOG_SERVER_FIELD_IDS.removeConfigurationBtn}-${i}`}
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
					id={CATALOG_SERVER_FIELD_IDS.addConfigurationBtn}
					class="btn btn-secondary btn-sm flex items-center gap-1 text-xs"
					type="button"
					onclick={() => {
						if (config) {
							config.push({
								key: '',
								description: '',
								name: '',
								value: '',
								required: urlTemplateVariables,
								sensitive: false,
								file: false
							});
						}
					}}
				>
					<Plus class="size-4" />
					{urlTemplateVariables
						? m.mcps_catalog_custom_config_url_variable()
						: serverUserType === 'singleUser'
							? m.mcps_catalog_custom_config_user_configuration()
							: m.mcps_catalog_config_heading()}
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
		<h4 class="text-sm font-semibold">{m.mcps_catalog_custom_config_secret_bound()}</h4>

		{#each allSecretBound as { item, source }, sbIdx (`${source}:${item.key}`)}
			<div
				class="dark:border-base-400 bg-base-300 flex w-full items-center gap-4 rounded-lg border border-transparent p-4"
			>
				<div class="flex w-full flex-col gap-4">
					<div class="flex w-full flex-col gap-1">
						<label for={`sb-${sbIdx}-type`} class="text-sm font-light">{m.core_type()}</label>
						<input
							class={inputClass}
							id={`sb-${sbIdx}-type`}
							value={source === 'header'
								? m.mcps_config_usage_header()
								: item.file
									? m.mcps_catalog_config_usage_file()
									: m.mcps_catalog_config_usage_env()}
							disabled
						/>
					</div>

					<div class="flex w-full flex-col gap-1">
						<label for={`sb-${sbIdx}-name`} class="text-sm font-light">{m.core_name()}</label>
						<input
							class={inputClass}
							id={`sb-${sbIdx}-name`}
							value={item.name || item.key}
							disabled
						/>
					</div>

					{#if item.description}
						<div class="flex w-full flex-col gap-1">
							<label for={`sb-${sbIdx}-description`} class="text-sm font-light"
								>{m.core_description()}</label
							>
							<input
								class={inputClass}
								id={`sb-${sbIdx}-description`}
								value={item.description}
								disabled
							/>
						</div>
					{/if}

					<div class="flex w-full flex-col gap-1">
						<label for={`sb-${sbIdx}-key`} class="text-sm font-light">{m.mcps_field_key()}</label>
						<input class={inputClass} id={`sb-${sbIdx}-key`} value={item.key} disabled />
					</div>

					{#if item.secretBinding?.name && item.secretBinding?.key}
						<div class="flex w-full flex-col gap-1">
							<label for={`sb-${sbIdx}-secret`} class="text-sm font-light"
								>{m.mcps_catalog_secret_label()}</label
							>
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
							<span class="badge badge-secondary badge-xs">{m.mcps_catalog_badge_sensitive()}</span>
						{/if}
						{#if item.required}
							<span class="badge badge-secondary badge-xs">{m.mcps_catalog_badge_required()}</span>
						{/if}
						{#if source === 'env' && item.file}
							<span class="badge badge-secondary badge-xs">{m.mcps_catalog_badge_file()}</span>
						{/if}
						{#if source === 'env' && item.dynamicFile}
							<span class="badge badge-secondary badge-xs">{m.mcps_catalog_badge_dynamic()}</span>
						{/if}
					</div>
				</div>
			</div>
		{/each}
	</div>
{/if}
