<script lang="ts">
	import CopyButton from '$lib/components/CopyButton.svelte';
	import type { Model } from '$lib/services';
	import { renderCurlExample } from '$lib/services/llm-gateway/curl';
	import type { RenderContext } from '$lib/services/llm-gateway/types';
	import LLMGatewayCodeBlock from './LLMGatewayCodeBlock.svelte';
	import LLMGatewayModelList from './LLMGatewayModelList.svelte';
	import { ChevronDown, ChevronRight } from 'lucide-svelte';

	interface Props {
		ctx: RenderContext;
		models: Model[];
	}

	let { ctx, models }: Props = $props();

	let expanded = $state(true);

	// Prefer a brand icon from one of the models (they all carry the provider's icon).
	let icon = $derived(models[0]?.icon);
	let iconDark = $derived(models[0]?.iconDark);

	let curlBlock = $derived(renderCurlExample(ctx));
</script>

<section class="border-base-300 dark:border-base-400 rounded-lg border">
	<button
		type="button"
		class="hover:bg-base-200 dark:hover:bg-base-300 flex w-full items-center justify-between gap-3 rounded-t-lg px-4 py-3 text-left transition-colors"
		onclick={() => (expanded = !expanded)}
	>
		<div class="flex items-center gap-3">
			{#if icon}
				<img src={icon} alt={ctx.provider.displayName} class="icon size-6 shrink-0 dark:hidden" />
			{/if}
			{#if iconDark}
				<img
					src={iconDark}
					alt={ctx.provider.displayName}
					class="icon hidden size-6 shrink-0 dark:block"
				/>
			{/if}
			<div class="flex flex-col">
				<h3 class="text-lg font-semibold">{ctx.provider.displayName}</h3>
				<span class="text-muted-content text-xs">
					{models.length} model{models.length === 1 ? '' : 's'} available
				</span>
			</div>
		</div>
		{#if expanded}
			<ChevronDown class="text-muted-content size-5 shrink-0" />
		{:else}
			<ChevronRight class="text-muted-content size-5 shrink-0" />
		{/if}
	</button>

	{#if expanded}
		<div class="border-base-300 dark:border-base-400 flex flex-col gap-6 border-t p-4">
			<div class="flex flex-col gap-2">
				<h4 class="text-sm font-semibold">Base URL</h4>
				<div
					class="bg-base-200 dark:bg-base-300 flex items-center justify-between gap-3 rounded-md px-3 py-2"
				>
					<code class="truncate font-mono text-xs">{ctx.baseURL}</code>
					<CopyButton text={ctx.baseURL} tooltipText="Copy base URL" />
				</div>
			</div>

			<div class="flex flex-col gap-2">
				<h4 class="text-sm font-semibold">Example request</h4>
				<LLMGatewayCodeBlock block={curlBlock} />
			</div>

			<div class="flex flex-col gap-2">
				<h4 class="text-sm font-semibold">Available models</h4>
				<LLMGatewayModelList {models} />
			</div>
		</div>
	{/if}
</section>
