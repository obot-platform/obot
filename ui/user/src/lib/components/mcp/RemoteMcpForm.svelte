<script lang="ts">
	import type { MCPServerInfo } from '$lib/services/chat/mcp';
	import { Plus, Trash2 } from 'lucide-svelte';
	import SensitiveInput from '$lib/components/SensitiveInput.svelte';
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import { fade, slide } from 'svelte/transition';

	interface Props {
		config: MCPServerInfo;
		showSubmitError: boolean;
		custom?: boolean;
		showAdvancedOptions?: boolean;
		chatbot?: boolean;
	}
	let {
		config = $bindable(),
		showSubmitError,
		custom,
		chatbot = false,
		showAdvancedOptions = $bindable(false)
	}: Props = $props();

	let keepEditable = $state(false);

	function focusOnAdd(node: HTMLInputElement, shouldFocus: boolean) {
		if (shouldFocus) {
			node.focus();
		}
	}
</script>

<div class="flex items-center gap-4">
	<h4 class="w-24 text-base font-semibold">URL</h4>
	{#if showAdvancedOptions || custom || !config.url || keepEditable}
		<input
			class="text-input-filled flex grow"
			bind:value={config.url}
			onkeydown={() => (keepEditable = true)}
			disabled={chatbot}
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

{#if custom || chatbot || (config.env ?? []).some((env) => env.required)}
	<div class="flex flex-col gap-1">
		<h4 class="text-base font-semibold">Environment Variables</h4>
		{@render showConfigEnvVars('all')}
		{#if custom || showAdvancedOptions}
			{@render addEnvVarButton()}
		{/if}
	</div>
{/if}

{#if custom || chatbot}
	<div class="flex flex-col gap-1">
		<h4 class="text-base font-semibold">Headers Configuration</h4>
		{@render showConfigHeaders('all')}
		{@render addHeaderButton()}
	</div>
{/if}

{#if !custom && (config.headers ?? []).length > 0}
	<div class="flex flex-col gap-1">
		<h4 class="text-base font-semibold">Headers Configuration</h4>
		{@render showConfigHeaders('default')}
	</div>
{/if}

{#if showAdvancedOptions}
	<div class="flex flex-col gap-4" in:fade out:slide={{ axis: 'y' }}>
		{#if !custom}
			<div class="flex flex-col gap-1">
				<h4 class="text-base font-semibold">Custom Headers Configuration</h4>
				{@render showConfigHeaders('custom')}
				{@render addHeaderButton()}
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

{#snippet addHeaderButton()}
	{#if !chatbot}
		<div class="flex justify-end">
			<button
				class="button flex items-center gap-1 text-xs"
				onclick={() =>
					config.headers?.push({
						name: '',
						key: '',
						description: '',
						sensitive: false,
						required: false,
						file: false,
						value: '',
						custom: crypto.randomUUID()
					})}
			>
				<Plus class="size-4" /> Header
			</button>
		</div>
	{/if}
{/snippet}

{#snippet addEnvVarButton()}
	{#if !chatbot}
		<div class="flex justify-end">
			<button
				class="button flex items-center gap-1 text-xs"
				onclick={() =>
					config.env?.push({
						name: '',
						key: '',
						description: '',
						sensitive: false,
						required: false,
						file: false,
						value: '',
						custom: crypto.randomUUID()
					})}
			>
				<Plus class="size-4" /> Environment Variable
			</button>
		</div>
	{/if}
{/snippet}

{#snippet showConfigEnvVars(type: 'all' | 'default' | 'custom')}
	{@const envsToShow =
		type === 'all'
			? (config.env ?? [])
			: type === 'default'
				? (config.env?.filter((env) => !env.custom && env.required) ?? [])
				: (config.env?.filter((env) => env.custom || (!custom && showAdvancedOptions)) ?? [])}
	{#if envsToShow.length > 0}
		{#each envsToShow as env, i}
			<div class="flex w-full items-center gap-2">
				<div class="flex grow flex-col gap-1">
					<input
						class="ghost-input w-full py-0 pl-1"
						bind:value={env.key}
						placeholder="Key (ex. API_KEY)"
						use:focusOnAdd={i === envsToShow.length - 1}
						disabled={chatbot}
					/>
					{#if env.sensitive}
						<SensitiveInput name={env.name} bind:value={env.value} />
					{:else}
						<input
							data-1p-ignore
							id={env.name}
							name={env.name}
							class="text-input-filled w-full"
							bind:value={env.value}
							type="text"
						/>
					{/if}
					<div class="min-h-4 text-xs text-red-500">
						{#if showSubmitError && !env.value && env.required}
							This field is required.
						{/if}
					</div>
				</div>
				{#if (!env.required || custom) && !chatbot}
					<button
						class="icon-button"
						onclick={() => {
							const matchingIndex = config.env?.findIndex((e) =>
								e.key ? e.key === env.key : e.custom === env.custom
							);
							if (typeof matchingIndex !== 'number') return;
							config.env?.splice(matchingIndex, 1);
						}}
					>
						<Trash2 class="size-4" />
					</button>
				{/if}
			</div>
		{/each}
	{/if}
{/snippet}

{#snippet showConfigHeaders(type: 'all' | 'default' | 'custom')}
	{@const headersToShow =
		type === 'all'
			? (config.headers ?? [])
			: type === 'default'
				? (config.headers?.filter((header) => !header.custom) ?? [])
				: (config.headers?.filter((header) => header.custom) ?? [])}
	{#if headersToShow.length > 0}
		{#each headersToShow as header, i}
			<div class="flex w-full items-center gap-2">
				<div class="flex grow flex-col gap-1">
					<input
						class="ghost-input w-full py-0 pl-1"
						bind:value={header.key}
						placeholder="Key (ex. API_KEY)"
						use:focusOnAdd={i === headersToShow.length - 1}
						disabled={chatbot}
					/>
					{#if header.sensitive}
						<SensitiveInput name={header.name} bind:value={header.value} />
					{:else}
						<input
							data-1p-ignore
							id={header.name}
							name={header.name}
							class="text-input-filled w-full"
							bind:value={header.value}
							type="text"
						/>
					{/if}
					<div class="min-h-4 text-xs text-red-500">
						{#if showSubmitError && !header.value && header.required}
							This field is required.
						{/if}
					</div>
				</div>
				{#if (!header.required || custom) && !chatbot}
					<button
						class="icon-button"
						onclick={() => {
							const matchingIndex = config.headers?.findIndex((e) =>
								e.key ? e.key === header.key : e.custom === header.custom
							);
							if (typeof matchingIndex !== 'number') return;
							config.headers?.splice(matchingIndex, 1);
						}}
					>
						<Trash2 class="size-4" />
					</button>
				{/if}
			</div>
		{/each}
	{/if}
{/snippet}
