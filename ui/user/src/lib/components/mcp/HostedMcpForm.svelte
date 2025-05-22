<script lang="ts">
	import type { MCPServerInfo } from '$lib/services/chat/mcp';
	import { Plus, Trash2 } from 'lucide-svelte';
	import InfoTooltip from '$lib/components/InfoTooltip.svelte';
	import SensitiveInput from '$lib/components/SensitiveInput.svelte';
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

	function focusOnAdd(node: HTMLInputElement, shouldFocus: boolean) {
		if (shouldFocus) {
			node.focus();
		}
	}
</script>

{#if custom || chatbot}
	<div class="flex flex-col gap-1">
		<h4 class="text-base font-semibold">Configuration</h4>
		{@render showConfigEnv('all')}
		{@render addEnvButton()}
	</div>
{/if}

{#if !custom && (config.env ?? []).length > 0}
	<div class="flex flex-col gap-1">
		<h4 class="text-base font-semibold">Configuration</h4>
		{@render showConfigEnv('default')}
	</div>
{/if}

{#if showAdvancedOptions || custom || chatbot}
	<div class="flex flex-col gap-4" in:fade out:slide={{ axis: 'y' }}>
		{#if !custom && !chatbot}
			<div class="flex flex-col gap-1">
				<h4 class="text-base font-semibold">Custom Configurations</h4>
				{@render showConfigEnv('custom')}
				{@render addEnvButton()}
			</div>
		{/if}

		<div class="flex items-center gap-4">
			<h4 class="text-base font-semibold">Command</h4>
			<input class="text-input-filled w-full" bind:value={config.command} disabled={chatbot} />
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
								disabled={chatbot}
							/>
							{#if !chatbot}
								<button class="icon-button" onclick={() => config.args?.splice(i, 1)}>
									<Trash2 class="size-4" />
								</button>
							{/if}
						</div>
					{/each}

					{#if !chatbot}
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

{#snippet showConfigEnv(type: 'all' | 'default' | 'custom')}
	{@const envsToShow =
		type === 'all'
			? (config.env ?? [])
			: type === 'default'
				? (config.env?.filter((env) => !env.custom) ?? [])
				: (config.env?.filter((env) => env.custom) ?? [])}
	{#each envsToShow as env, i}
		<div class="flex w-full items-center gap-2">
			<div class="flex grow flex-col gap-1">
				{#if env.custom}
					<input
						class="ghost-input w-full py-0"
						bind:value={env.key}
						placeholder="Key (ex. API_KEY)"
						use:focusOnAdd={i === envsToShow.length - 1}
					/>
				{:else}
					<label for={env.key} class="flex items-center gap-1 text-sm font-light">
						{env.required ? `${env.name || env.key}*` : `${env.name || env.key} (optional)`}
						<InfoTooltip text={env.description} />
					</label>
				{/if}
				{#if env.sensitive}
					<SensitiveInput name={env.name} bind:value={env.value} />
				{:else}
					<input
						data-1p-ignore
						id={env.key}
						name={env.key}
						class="text-input-filled w-full"
						class:error={showSubmitError && !env.value && env.required}
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
			{#if (custom || env.custom) && !chatbot}
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
{/snippet}
