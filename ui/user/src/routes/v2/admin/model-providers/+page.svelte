<script lang="ts">
	import ProviderCard from '$lib/components/admin/ProviderCard.svelte';
	import type { ModelProvider as ModelProviderType } from '$lib/services';
	import Layout from '$lib/components/Layout.svelte';
	import {
		CommonModelProviderIds,
		PAGE_TRANSITION_DURATION,
		RecommendedModelProviders
	} from '$lib/constants';
	import { fade } from 'svelte/transition';
	import ProviderConfigure from '$lib/components/admin/ProviderConfigure.svelte';

	let { data } = $props();
	let { modelProviders } = data;

	let providerConfigure = $state<ReturnType<typeof ProviderConfigure>>();
	let configuringModelProvider = $state<ModelProviderType>();

	const duration = PAGE_TRANSITION_DURATION;

	function sortModelProviders(modelProviders: ModelProviderType[]) {
		return [...modelProviders].sort((a, b) => {
			const preferredOrder = [
				CommonModelProviderIds.OPENAI,
				CommonModelProviderIds.AZURE_OPENAI,
				CommonModelProviderIds.ANTHROPIC,
				CommonModelProviderIds.ANTHROPIC_BEDROCK,
				CommonModelProviderIds.XAI,
				CommonModelProviderIds.OLLAMA,
				CommonModelProviderIds.VOYAGE,
				CommonModelProviderIds.GROQ,
				CommonModelProviderIds.VLLM,
				CommonModelProviderIds.DEEPSEEK,
				CommonModelProviderIds.GEMINI_VERTEX,
				CommonModelProviderIds.GENERIC_OPENAI
			];
			const aIndex = preferredOrder.indexOf(a.id);
			const bIndex = preferredOrder.indexOf(b.id);

			// If both providers are in preferredOrder, sort by their order
			if (aIndex !== -1 && bIndex !== -1) {
				return aIndex - bIndex;
			}

			// If only a is in preferredOrder, it comes first
			if (aIndex !== -1) return -1;
			// If only b is in preferredOrder, it comes first
			if (bIndex !== -1) return 1;

			// For all other providers, sort alphabetically by name
			return a.name.localeCompare(b.name);
		});
	}

	let sortedModelProviders = $derived(sortModelProviders(modelProviders));
</script>

<Layout>
	<div class="my-4" in:fade={{ duration }} out:fade={{ duration }}>
		<div class="flex flex-col gap-8">
			<h1 class="text-2xl font-semibold">Model Providers</h1>
		</div>
		<div class="grid grid-cols-2 gap-4 py-8 md:grid-cols-3 lg:grid-cols-4">
			{#each sortedModelProviders as modelProvider}
				<ProviderCard
					provider={modelProvider}
					recommended={RecommendedModelProviders.includes(modelProvider.id)}
					onConfigure={() => {
						configuringModelProvider = modelProvider;
						providerConfigure?.open();
					}}
				/>
			{/each}
		</div>
	</div>
</Layout>

<ProviderConfigure bind:this={providerConfigure} provider={configuringModelProvider} />

<svelte:head>
	<title>Obot | Model Providers</title>
</svelte:head>
