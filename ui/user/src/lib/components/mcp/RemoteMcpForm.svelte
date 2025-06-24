<script lang="ts">
	import type { MCPSubField } from '$lib/services/chat/types';
	import type { MCPServerInfo } from '$lib/services/chat/mcp';
	import { Plus, Trash2 } from 'lucide-svelte';
	import SensitiveInput from '$lib/components/SensitiveInput.svelte';
	import { fade, slide } from 'svelte/transition';
	import Toggle from '$lib/components/Toggle.svelte';
	import InfoTooltip from '$lib/components/InfoTooltip.svelte';

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
		{@render urlSection()}
		{@render envsSection('all')}
		{@render headersSection('all')}
	{:else}
		{@render envsSection('default')}
		{@render headersSection('default')}
		{@render urlSection()}
		{#if showAdvancedConfig}
			<div class="mt-2 mb-4">
				<h4 class="text-base font-semibold text-gray-900 dark:text-gray-100">Advanced</h4>
			</div>
			{@render envsSection('custom')}
			{@render headersSection('custom')}
		{/if}
		{@render toggleAdvancedConfigButton()}
	{/if}
</div>

{#snippet toggleAdvancedConfigButton()}
	<button
		class="text-xs font-light text-gray-500 transition-colors hover:text-black dark:hover:text-white"
		onclick={() => (showAdvancedConfig = !showAdvancedConfig)}
	>
		{showAdvancedConfig ? 'Hide Advanced Configuration...' : 'Show Advanced Configuration...'}
	</button>
{/snippet}

{#snippet urlSection()}
	<div class="space-y-2">
		<div class="flex gap-4">
			<h4 class="mt-1.5 w-28 text-base font-semibold">URL</h4>
			<div class="flex-1">
				<input
					class="text-input-filled w-full"
					class:error={showSubmitError && !config.url}
					bind:value={config.url}
					disabled={chatbot}
				/>
				{#if showSubmitError && !config.url}
					<div class="mt-1 text-xs text-red-500">This field is required.</div>
				{/if}
			</div>
		</div>
	</div>
{/snippet}

{#snippet keyValueSection(
	type: 'all' | 'default' | 'custom',
	{
		items,
		title,
		placeholder,
		buttonText
	}: {
		items: (MCPSubField & { value: string; custom?: string })[] | undefined;
		title: string;
		placeholder: string;
		buttonText: string;
	}
)}
	{@const itemsToShow =
		type === 'all'
			? (items ?? [])
			: type === 'default'
				? (items?.filter((item) => !item.custom) ?? [])
				: (items?.filter((item) => item.custom) ?? [])}

	<div class="flex gap-4">
		{#if !chatbot && type !== 'default'}
			<h4 class="mt-1.5 w-28 text-base font-semibold">{title}</h4>
		{/if}

		<div class="flex-1 space-y-4">
			{#each itemsToShow as item, i}
				<div class="flex gap-2">
					<div class="flex-1">
						{#if item.custom}
							<div class="mb-1 flex gap-2">
								<input
									class="ghost-input flex-1 py-0"
									bind:value={item.key}
									{placeholder}
									use:focusOnAdd={i === itemsToShow.length - 1}
									disabled={chatbot}
								/>
								{#if type !== 'default'}
									<Toggle
										label="Required"
										labelInline
										checked={item.required}
										onChange={(checked) => (item.required = checked)}
										classes={{
											label: 'text-xs font-light text-gray-600 dark:text-gray-400 whitespace-nowrap'
										}}
									/>
								{/if}
							</div>
						{:else}
							<label for={item.key} class="mb-1 flex items-center gap-1 text-sm font-light">
								{item.required
									? `${item.name || item.key}*`
									: `${item.name || item.key} (optional)`}
								{#if item.description}
									<InfoTooltip text={item.description} />
								{/if}
							</label>
						{/if}

						{#if item.sensitive}
							<SensitiveInput name={item.name} bind:value={item.value} />
						{:else}
							<input
								data-1p-ignore
								id={item.name}
								name={item.name}
								class="text-input-filled w-full"
								bind:value={item.value}
								type="text"
								disabled={chatbot}
							/>
						{/if}

						{#if showSubmitError && !item.value && item.required}
							<div class="mt-1 text-xs text-red-500">This field is required.</div>
						{/if}
					</div>

					{#if !chatbot && type !== 'default'}
						<button
							class="icon-button"
							onclick={() => {
								const matchingIndex = items?.findIndex((e) =>
									e.key ? e.key === item.key : e.custom === item.custom
								);
								if (typeof matchingIndex !== 'number') return;
								items?.splice(matchingIndex, 1);
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
							items?.push({
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
						<Plus class="size-4" />
						{buttonText}
					</button>
				</div>
			{/if}
		</div>
	</div>
{/snippet}

{#snippet envsSection(type: 'all' | 'default' | 'custom')}
	{@render keyValueSection(type, {
		items: config.env,
		title: 'Environment',
		placeholder: 'Key (ex. API_KEY)',
		buttonText: 'Environment Variable'
	})}
{/snippet}

{#snippet headersSection(type: 'all' | 'default' | 'custom')}
	{@render keyValueSection(type, {
		items: config.headers,
		title: 'Headers',
		placeholder: 'Header Name (ex. Authorization)',
		buttonText: 'Header'
	})}
{/snippet}
