<script lang="ts">
	import { Plus, Trash2 } from 'lucide-svelte';
	import type {
		MCPCatalogEntryFormData,
		MCPCatalogEntryServerManifest
	} from '$lib/services/admin/types';
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import Toggle from '../Toggle.svelte';

	interface Props {
		config: MCPCatalogEntryFormData;
		type: 'single' | 'multi';
		readonly?: boolean;
	}
	let { config = $bindable(), readonly, type = 'single' }: Props = $props();
</script>

<div
	class="dark:bg-surface1 dark:border-surface3 flex flex-col gap-4 rounded-lg border border-transparent bg-white p-4 shadow-sm"
>
	<h4 class="text-sm font-semibold">Launch Command</h4>
	<div class="flex items-center gap-4">
		<label for="command" class="text-sm font-light">Command</label>
		<input
			id="command"
			class="text-input-filled w-full dark:bg-black"
			bind:value={config.command}
			disabled={readonly}
		/>
	</div>

	{#if config.args}
		<div class="flex gap-4">
			<span class="pt-2.5 text-sm font-light">Arguments</span>
			<div class="flex min-h-10 grow flex-col gap-4">
				{#each config.args as _arg, i}
					<div class="flex items-center gap-2">
						<input
							class="text-input-filled w-full dark:bg-black"
							bind:value={config.args[i]}
							disabled={readonly}
							onpaste={(e) => {
								if (readonly || !config.args) return;
								e.preventDefault();
								const pastedText = e.clipboardData?.getData('text');
								if (!pastedText) return;

								const lines = pastedText.split(/[\r\n]+/).filter((line) => line.trim());
								if (lines.length <= 1) {
									config.args[i] = pastedText;
									return;
								}

								// Remove quotes, commas and trim each line
								const cleanedLines = lines.map((line) => {
									let trimmed = line.trim();
									if (trimmed.endsWith(',')) {
										trimmed = trimmed.slice(0, -1).trim();
									}

									if (
										(trimmed.startsWith('"') && trimmed.endsWith('"')) ||
										(trimmed.startsWith("'") && trimmed.endsWith("'"))
									) {
										trimmed = trimmed.slice(1, -1).trim();
									}
									return trimmed;
								});

								config.args[i] = cleanedLines[0];
								for (let j = 1; j < cleanedLines.length; j++) {
									config.args.splice(i + j, 0, cleanedLines[j]);
								}
							}}
						/>
						{#if !readonly}
							<button class="icon-button" onclick={() => config.args?.splice(i, 1)}>
								<Trash2 class="size-4" />
							</button>
						{/if}
					</div>
				{/each}

				{#if !readonly}
					<div class="flex justify-end">
						<button
							class="button flex items-center gap-1 text-xs"
							onclick={() => config.args?.push('')}
						>
							<Plus class="size-4" /> Argument
						</button>
					</div>
				{/if}
			</div>
		</div>
	{/if}
</div>

{#if !readonly || (readonly && config.env && config.env.length > 0)}
	<div
		class="dark:bg-surface1 dark:border-surface3 flex flex-col gap-4 rounded-lg border border-transparent bg-white p-4 shadow-sm"
	>
		<h4 class="text-sm font-semibold">
			{type === 'single' ? 'User Supplied Configuration' : 'Configuration'}
		</h4>
		<p class="text-xs font-light text-gray-400 dark:text-gray-600">
			{type === 'single' ? 'User supplied config' : 'Config'} values will be available as environment
			variables in the MCP server and can be referenced in the arguments section using the syntax $KEY_NAME.
		</p>
		{@render showConfigEnv(config.env ?? [])}
		{@render addEnvButton()}
	</div>
{/if}

{#snippet addEnvButton()}
	{#if !readonly}
		<div class="flex justify-end">
			<button
				class="button flex items-center gap-1 text-xs"
				onclick={() =>
					config.env?.push({
						key: '',
						description: '',
						name: '',
						value: '',
						required: false,
						sensitive: false
					})}
			>
				<Plus class="size-4" /> User Configuration
			</button>
		</div>
	{/if}
{/snippet}

{#snippet showConfigEnv(envs: MCPCatalogEntryServerManifest['env'])}
	{#if envs}
		{#each envs as env, i}
			<div
				class="dark:border-surface3 flex w-full items-center gap-4 rounded-lg border border-transparent bg-gray-50 p-4 dark:bg-gray-900"
			>
				{#if type === 'single'}
					<div class="flex w-full flex-col gap-4">
						<div class="flex w-full flex-col gap-1">
							<label for={`env-name-${i}`} class="text-sm font-light">Name</label>
							<input
								id={`env-name-${i}`}
								class="text-input-filled w-full"
								bind:value={envs[i].name}
								disabled={readonly}
							/>
						</div>
						<div class="flex w-full flex-col gap-1">
							<label for={`env-description-${i}`} class="text-sm font-light">Description</label>
							<input
								id={`env-description-${i}`}
								class="text-input-filled w-full"
								bind:value={envs[i].description}
								disabled={readonly}
							/>
						</div>
						<div class="flex w-full flex-col gap-1">
							<label for={`env-key-${i}`} class="text-sm font-light">Key</label>
							<input
								id={`env-key-${i}`}
								class="text-input-filled w-full"
								bind:value={envs[i].key}
								placeholder="(eg. CUSTOM_API_KEY)"
								disabled={readonly}
							/>
						</div>
						<div class="flex gap-8">
							<Toggle
								classes={{ label: 'text-sm text-inherit' }}
								label="Sensitive"
								labelInline
								checked={!!env.sensitive}
								onChange={(checked) => {
									envs[i].sensitive = checked;
								}}
							/>
							<Toggle
								classes={{ label: 'text-sm text-inherit' }}
								label="Required"
								labelInline
								checked={!!env.required}
								onChange={(checked) => {
									envs[i].required = checked;
								}}
							/>
						</div>
					</div>
				{:else}
					<div class="flex w-full flex-col gap-4">
						<div class="flex w-full flex-col gap-1">
							<label for={`env-key-${i}`} class="text-sm font-light">Key</label>
							<input
								id={`env-key-${i}`}
								class="text-input-filled w-full"
								bind:value={envs[i].key}
								placeholder="(eg. CUSTOM_API_KEY)"
								disabled={readonly}
							/>
						</div>
						<div class="flex w-full flex-col gap-1">
							<label for={`env-value-${i}`} class="text-sm font-light">Value</label>
							<input
								id={`env-value-${i}`}
								class="text-input-filled w-full"
								bind:value={envs[i].value}
								placeholder="(eg. 123abcdef456)"
								disabled={readonly}
							/>
						</div>
						<div>
							<Toggle
								classes={{ label: 'text-sm text-inherit w-fit' }}
								label="Sensitive"
								labelInline
								checked={!!env.sensitive}
								onChange={(checked) => {
									envs[i].sensitive = checked;
								}}
							/>
						</div>
					</div>
				{/if}

				{#if !readonly}
					<button
						class="icon-button"
						onclick={() => {
							envs.splice(i, 1);
						}}
						use:tooltip={'Delete Configuration'}
					>
						<Trash2 class="size-4" />
					</button>
				{/if}
			</div>
		{/each}
	{/if}
{/snippet}
