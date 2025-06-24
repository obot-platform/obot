<script lang="ts">
	import type { MCPServerInfo } from '$lib/services/chat/mcp';
	import { Plus, Trash2 } from 'lucide-svelte';
	import InfoTooltip from '$lib/components/InfoTooltip.svelte';
	import SensitiveInput from '$lib/components/SensitiveInput.svelte';
	import Toggle from '$lib/components/Toggle.svelte';
	import { fade, slide } from 'svelte/transition';

	interface Props {
		config: MCPServerInfo;
		showSubmitError: boolean;
		custom?: boolean;
		chatbot?: boolean;
	}
	let { config = $bindable(), showSubmitError, custom, chatbot = false }: Props = $props();

	let showAdvancedConfig = $state(false);

	function focusOnAdd(node: HTMLInputElement, shouldFocus: boolean) {
		if (shouldFocus) {
			node.focus();
		}
	}
</script>

<div class="space-y-6" in:fade out:slide={{ axis: 'y' }}>
	{#if custom || chatbot}
		{@render envsSection('all')}
		{@render commandSection()}
	{:else}
		{@render envsSection('default')}
		{#if showAdvancedConfig}
			<div class="mt-2 mb-4">
				<h4 class="text-base font-semibold text-gray-900 dark:text-gray-100">Advanced</h4>
			</div>
			{@render envsSection('custom')}
			{@render commandSection()}
		{/if}
		{@render toggleAdvancedConfigButton()}
	{/if}
</div>

{#snippet commandSection()}
	<div class="space-y-2">
		<!-- Command -->
		<div class="flex gap-4">
			<h4 class="mt-1.5 w-28 text-base font-semibold">Command</h4>
			<div class="flex-1">
				<input
					class="text-input-filled w-full"
					class:error={showSubmitError && (!config.command || config.command === '')}
					bind:value={config.command}
					placeholder="Command to use to start the MCP server; e.g. 'npx'"
					disabled={chatbot}
				/>
				{#if showSubmitError && (!config.command || config.command === '')}
					<div class="mt-1 text-xs text-red-500">This field is required.</div>
				{/if}
			</div>
		</div>

		<div class="flex gap-4">
			<h4 class="mt-1.5 w-28 text-base font-semibold">Arguments</h4>
			<div class="flex-1 space-y-4">
				{#if config.args}
					{#each config.args as _arg, i}
						<div class="flex gap-2">
							<input
								class="text-input-filled flex-1"
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
				{/if}

				{#if !chatbot}
					<div class="flex justify-end">
						<button
							class="button flex items-center gap-1 text-xs"
							onclick={() => {
								if (!config.args) config.args = [];
								config.args.push('');
							}}
						>
							<Plus class="size-4" /> Argument
						</button>
					</div>
				{/if}
			</div>
		</div>
	</div>
{/snippet}

{#snippet toggleAdvancedConfigButton()}
	<button
		class="text-xs font-light text-gray-500 transition-colors hover:text-black dark:hover:text-white"
		onclick={() => (showAdvancedConfig = !showAdvancedConfig)}
	>
		{showAdvancedConfig ? 'Hide Advanced Configuration...' : 'Show Advanced Configuration...'}
	</button>
{/snippet}

{#snippet envsSection(type: 'all' | 'default' | 'custom')}
	{@const envsToShow =
		type === 'all'
			? (config.env ?? [])
			: type === 'default'
				? (config.env?.filter((env) => !env.custom) ?? [])
				: (config.env?.filter((env) => env.custom) ?? [])}

	<div class="flex gap-4">
		{#if !chatbot && type !== 'default'}
			<h4 class="mt-1.5 w-28 text-base font-semibold">Environment</h4>
		{/if}
		<div class="flex-1 space-y-4">
			{#each envsToShow as env, i}
				<div class="flex gap-2">
					<div class="flex-1">
						{#if env.custom}
							<div class="mb-1 flex gap-2">
								<input
									class="ghost-input flex-1 py-0"
									bind:value={env.key}
									placeholder="Key (ex. API_KEY)"
									use:focusOnAdd={i === envsToShow.length - 1}
									disabled={chatbot}
								/>
								{#if type !== 'default'}
									<Toggle
										label="Required"
										labelInline
										checked={env.required}
										onChange={(checked) => (env.required = checked)}
										classes={{
											label: 'text-xs font-light text-gray-600 dark:text-gray-400 whitespace-nowrap'
										}}
									/>
								{/if}
							</div>
						{:else}
							<label for={env.key} class="mb-1 flex items-center gap-1 text-sm font-light">
								{env.required ? `${env.name || env.key}*` : `${env.name || env.key} (optional)`}
								{#if env.description}
									<InfoTooltip text={env.description} />
								{/if}
							</label>
						{/if}

						{#if env.sensitive}
							<SensitiveInput name={env.name} bind:value={env.value} />
						{:else}
							<input
								id={env.key}
								name={env.key}
								class="text-input-filled w-full"
								class:error={showSubmitError && !env.value && env.required}
								bind:value={env.value}
								type="text"
								disabled={chatbot}
							/>
						{/if}

						{#if showSubmitError && !env.value && env.required}
							<div class="mt-1 text-xs text-red-500">This field is required.</div>
						{/if}
					</div>

					{#if !chatbot && type !== 'default'}
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

			{#if !chatbot && type !== 'default'}
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
		</div>
	</div>
{/snippet}
