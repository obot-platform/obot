<script lang="ts">
	import { Plus, Trash2 } from 'lucide-svelte';
	import { fade, slide } from 'svelte/transition';
	import type { MCPCatalogEntryManifest } from '$lib/services/admin/types';

	interface Props {
		config: MCPCatalogEntryManifest;
		custom?: boolean;
		showAdvancedOptions?: boolean;
		readonly?: boolean;
	}
	let {
		config = $bindable(),
		custom,
		showAdvancedOptions = $bindable(false),
		readonly
	}: Props = $props();

	function focusOnAdd(node: HTMLInputElement, shouldFocus: boolean) {
		if (shouldFocus) {
			node.focus();
		}
	}
</script>

<div class="flex flex-col gap-1">
	<h4 class="text-base font-semibold">Configuration</h4>
	{@render showConfigEnv(config.env ?? [])}
	{@render addEnvButton()}
</div>

{#if showAdvancedOptions || custom}
	<div class="flex flex-col gap-4" in:fade out:slide={{ axis: 'y' }}>
		<div class="flex items-center gap-4">
			<h4 class="text-base font-semibold">Command</h4>
			<input class="text-input-filled w-full" bind:value={config.command} disabled={readonly} />
		</div>

		{#if config.args}
			<div class="flex gap-4">
				<h4 class="mt-1.5 text-base font-semibold">Arguments</h4>
				<div class="flex grow flex-col gap-4">
					{#each config.args as _arg, i}
						<div class="flex items-center gap-2">
							<input
								class="text-input-filled w-full"
								bind:value={config.args[i]}
								disabled={readonly}
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
{/if}

{#if !custom}
	<div class="flex grow justify-start">
		<button
			class="mt-auto text-xs font-light text-gray-500 transition-colors hover:text-black dark:hover:text-white"
			onclick={() => (showAdvancedOptions = !showAdvancedOptions)}
		>
			{showAdvancedOptions ? 'Hide Advanced Options...' : 'Show Advanced Options...'}
		</button>
	</div>
{/if}

{#snippet addEnvButton()}
	{#if !readonly}
		<div class="flex justify-end">
			<button
				class="button flex items-center gap-1 text-xs"
				onclick={() => config.env?.push({ key: '', description: '' })}
			>
				<Plus class="size-4" /> Environment Variable
			</button>
		</div>
	{/if}
{/snippet}

{#snippet showConfigEnv(envs: { key: string; description: string }[])}
	{#each envs as env, i}
		<div class="flex w-full items-center gap-2">
			<div class="flex grow flex-col gap-1">
				<input
					class="text-input-filled w-full"
					bind:value={envs[i].key}
					placeholder="Key (ex. API_KEY)"
					use:focusOnAdd={i === envs.length - 1}
					disabled={readonly}
				/>
				<input
					class="text-input-filled w-full text-sm"
					bind:value={envs[i].description}
					placeholder="Description explaining the environment variable"
					disabled={readonly}
				/>
			</div>
			{#if !readonly}
				<button
					class="icon-button"
					onclick={() => {
						envs.splice(i, 1);
					}}
				>
					<Trash2 class="size-4" />
				</button>
			{/if}
		</div>
	{/each}
{/snippet}
