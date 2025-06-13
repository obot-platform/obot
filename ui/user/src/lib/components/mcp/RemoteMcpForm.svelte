<script lang="ts">
	import { Plus, Trash2 } from 'lucide-svelte';
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import type { MCPCatalogEntryManifest } from '$lib/services/admin/types';

	interface Props {
		config: MCPCatalogEntryManifest;
		custom?: boolean;
		readonly?: boolean;
	}
	let { config = $bindable(), custom, readonly }: Props = $props();

	let keepEditable = $state(false);
</script>

<div class="flex items-center gap-4">
	<h4 class="w-24 text-base font-semibold">URL</h4>
	{#if custom || !config.url || keepEditable}
		<input
			class="text-input-filled flex grow"
			bind:value={config.url}
			onkeydown={() => (keepEditable = true)}
			disabled={readonly}
		/>
	{:else}
		<p
			class="line-clamp-1 -translate-x-2 break-all"
			use:tooltip={{ text: config.url ?? '', disablePortal: true }}
		>
			{config.url}
		</p>
	{/if}
</div>

<div class="flex flex-col gap-1">
	<h4 class="text-base font-semibold">Environment Variables</h4>
	{@render showConfigEnvVars(config.env ?? [])}
	{#if custom}
		{@render addEnvVarButton()}
	{/if}
</div>

{#if custom}
	<div class="flex flex-col gap-1">
		<h4 class="text-base font-semibold">Headers Configuration</h4>
		{@render showConfigHeaders(config.headers ?? [])}
		{@render addHeaderButton()}
	</div>
{/if}

{#snippet addHeaderButton()}
	{#if !readonly}
		<div class="flex justify-end">
			<button
				class="button flex items-center gap-1 text-xs"
				onclick={() => config.headers?.push({ key: '', description: '' })}
			>
				<Plus class="size-4" /> Header
			</button>
		</div>
	{/if}
{/snippet}

{#snippet addEnvVarButton()}
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

{#snippet showConfigEnvVars(envs: { key: string; description: string }[])}
	{#if envs.length > 0}
		{#each envs as env, i}
			<div class="flex w-full items-center gap-2">
				<div class="flex grow flex-col gap-1">
					<input
						class="ghost-input w-full py-0 pl-1"
						bind:value={envs[i].key}
						placeholder="Key (ex. API_KEY)"
						disabled={readonly}
					/>
					<input
						class="ghost-input w-full py-0 pl-1 text-sm"
						bind:value={envs[i].description}
						placeholder="Description explaining the environment variable"
						disabled={readonly}
					/>
				</div>
				{#if custom && !readonly}
					<button class="icon-button" onclick={() => envs.splice(i, 1)}>
						<Trash2 class="size-4" />
					</button>
				{/if}
			</div>
		{/each}
	{/if}
{/snippet}

{#snippet showConfigHeaders(headers: { key: string; description: string }[])}
	{#if headers.length > 0}
		{#each headers as header, i}
			<div class="flex w-full items-center gap-2">
				<div class="flex grow flex-col gap-1">
					<input
						class="ghost-input w-full py-0 pl-1"
						bind:value={headers[i].key}
						placeholder="Key (ex. API_KEY)"
						disabled={readonly}
					/>
					<input
						class="ghost-input w-full py-0 pl-1 text-sm"
						bind:value={headers[i].description}
						placeholder="Description explaining the header variable"
						disabled={readonly}
					/>
				</div>
				{#if custom && !readonly}
					<button class="icon-button" onclick={() => headers.splice(i, 1)}>
						<Trash2 class="size-4" />
					</button>
				{/if}
			</div>
		{/each}
	{/if}
{/snippet}
