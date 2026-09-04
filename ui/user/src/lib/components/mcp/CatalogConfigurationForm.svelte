<script lang="ts">
	import { CATALOG_SERVER_FIELD_IDS } from '$lib/constants';
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
		{ id: 'env', label: 'Environment Variable' },
		{ id: 'header', label: 'Header' },
		{ id: 'file', label: 'File' },
		{ id: 'dynamicFile', label: 'Dynamic File' },
		{ id: 'interpolated', label: 'Interpolated Value' }
	];
</script>

{#if !readonly || (config?.length ?? 0) > 0}
	<div
		class="dark:bg-base-200 dark:border-base-400 bg-base-100 flex flex-col gap-4 rounded-lg border border-transparent p-4 shadow-sm"
		id={CATALOG_SERVER_FIELD_IDS.configuration}
	>
		<div class="flex flex-col gap-1">
			<h4 class="text-sm font-semibold">Configuration</h4>
			<p class="text-muted-content text-xs font-light">
				Configuration values can be supplied statically, by users, or selected from options.
				Interpolated values are available to templates but are not added to the server environment.
			</p>
		</div>

		{#each config ?? [] as item, i (i)}
			<div
				class="dark:border-base-400 bg-base-200 dark:bg-base-300 flex w-full items-start gap-4 rounded-lg border border-transparent p-4"
			>
				<div class="flex w-full flex-col gap-4">
					<div class="flex w-full flex-col gap-1">
						<label for={`catalog-config-usage-${i}`} class="text-sm font-light">Usage</label>
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
								>Value Prefix</label
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
						aria-label="Remove configuration"
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
					Configuration
				</button>
			</div>
		{/if}
	</div>
{/if}
