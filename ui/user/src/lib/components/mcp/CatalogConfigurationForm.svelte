<script lang="ts">
	import { CATALOG_SERVER_FIELD_IDS } from '$lib/constants';
	import { m } from '$lib/i18n';
	import type { MCPAllowedSecretBindingTarget, MCPConfig, MCPConfigUsage } from '$lib/services';
	import Select from '../Select.svelte';
	import IconButton from '../primitives/IconButton.svelte';
	import CustomConfigurationFieldset from './CustomConfigurationFieldset.svelte';
	import { Plus, Trash2 } from '@lucide/svelte';

	interface Props {
		config?: MCPConfig[];
		readonly?: boolean;
		secretBindingTargets?: MCPAllowedSecretBindingTarget[];
		showRequired?: boolean;
		showInvalid?: boolean;
	}

	let {
		config = $bindable(),
		readonly,
		secretBindingTargets,
		showRequired,
		showInvalid
	}: Props = $props();

	const usageOptions: { id: MCPConfigUsage; label: string }[] = [
		{ id: 'env', label: m.mcps_catalog_config_usage_env() },
		{ id: 'header', label: m.mcps_config_usage_header() },
		{ id: 'file', label: m.mcps_catalog_config_usage_file() },
		{ id: 'dynamicFile', label: m.mcps_config_usage_dynamic_file() },
		{ id: 'interpolated', label: m.mcps_config_usage_interpolated() }
	];
</script>

{#if !readonly || (config?.length ?? 0) > 0}
	<div
		class="dark:bg-base-200 dark:border-base-400 bg-base-100 flex flex-col gap-4 rounded-lg border border-transparent p-4 shadow-sm"
		id={CATALOG_SERVER_FIELD_IDS.configuration}
	>
		<div class="flex flex-col gap-1">
			<h4 class="text-sm font-semibold">{m.mcps_catalog_config_heading()}</h4>
			<p class="text-muted-content text-xs font-light">
				{m.mcps_config_description()}
			</p>
		</div>

		{#each config ?? [] as item, i (i)}
			<div
				class="dark:border-base-400 bg-base-200 dark:bg-base-300 flex w-full items-start gap-4 rounded-lg border border-transparent p-4"
			>
				<div class="flex w-full flex-col gap-4">
					<div class="flex w-full flex-col gap-1">
						<label for={`catalog-config-usage-${i}`} class="text-sm font-light"
							>{m.mcps_config_usage()}</label
						>
						<Select
							id={`catalog-config-usage-${i}`}
							class="dark:border-base-400 bg-base-100 border border-transparent"
							classes={{ root: 'flex grow' }}
							options={usageOptions}
							selected={item.usage}
							disabled={readonly}
							onSelect={(option) => {
								item.usage = option.id as MCPConfigUsage;
							}}
						/>
					</div>

					<CustomConfigurationFieldset
						id={`${CATALOG_SERVER_FIELD_IDS.env}-${i}`}
						bind:data={config![i]}
						serverUserType="multiUser"
						{readonly}
						{secretBindingTargets}
						classes={{ input: 'text-input-filled bg-base-100 w-full shadow-none' }}
						{showRequired}
						{showInvalid}
					/>

					{#if item.usage === 'header'}
						<div class="flex w-full flex-col gap-1">
							<label for={`catalog-config-prefix-${i}`} class="text-sm font-light"
								>{m.mcps_value_prefix()}</label
							>
							<input
								id={`catalog-config-prefix-${i}`}
								class="text-input-filled bg-base-100 w-full shadow-none"
								bind:value={item.prefix}
								disabled={readonly}
							/>
						</div>
					{/if}
				</div>
				{#if !readonly}
					<IconButton
						class="mt-6"
						id={`${CATALOG_SERVER_FIELD_IDS.removeConfigurationBtn}-${i}`}
						aria-label={m.mcps_config_remove()}
						variant="danger"
						onclick={() => config?.splice(i, 1)}
					>
						<Trash2 class="size-4" />
					</IconButton>
				{/if}
			</div>
		{/each}

		{#if !readonly}
			<div class="flex justify-end">
				<button
					id={CATALOG_SERVER_FIELD_IDS.addConfigurationBtn}
					class="btn btn-secondary btn-sm flex items-center gap-1 text-xs"
					type="button"
					onclick={() => {
						config ??= [];
						config.push({
							usage: 'env',
							key: '',
							description: '',
							name: '',
							value: '',
							required: false,
							sensitive: false
						});
					}}
				>
					<Plus class="size-4" />
					{m.mcps_catalog_config_heading()}
				</button>
			</div>
		{/if}
	</div>
{/if}
