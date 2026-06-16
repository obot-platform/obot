<script lang="ts">
	import Layout from '$lib/components/Layout.svelte';
	import LLMGatewayProviderSection from '$lib/components/llm-gateway/LLMGatewayProviderSection.svelte';
	import { CommonModelProviderIds, PAGE_TRANSITION_DURATION } from '$lib/constants';
	import { PROVIDER_CONNECTIONS, type RenderContext } from '$lib/services/llm-gateway/types';
	import { accessibleModels } from '$lib/stores';
	import { onMount } from 'svelte';
	import { fly } from 'svelte/transition';

	let obotURL = $state('');

	onMount(() => {
		obotURL = window.location.origin;
	});

	let openaiModels = $derived(
		accessibleModels.current.filter((m) => m.modelProvider === CommonModelProviderIds.OPENAI)
	);
	let anthropicModels = $derived(
		accessibleModels.current.filter((m) => m.modelProvider === CommonModelProviderIds.ANTHROPIC)
	);

	function buildCtx(
		shortKey: 'openai' | 'anthropic',
		models: typeof accessibleModels.current
	): RenderContext {
		const provider = PROVIDER_CONNECTIONS[shortKey];
		return {
			provider,
			obotURL,
			baseURL: `${obotURL}/api/llm-proxy/${provider.shortKey}`,
			exampleModel: models[0]?.name
		};
	}

	let openaiCtx = $derived(buildCtx('openai', openaiModels));
	let anthropicCtx = $derived(buildCtx('anthropic', anthropicModels));

	let ready = $derived(obotURL !== '');

	const duration = PAGE_TRANSITION_DURATION;
</script>

<Layout title="LLM Gateway Models">
	<div
		class="flex h-full w-full flex-col gap-6"
		in:fly={{ x: 100, duration, delay: duration }}
		out:fly={{ x: -100, duration }}
	>
		<p class="text-muted-content max-w-3xl text-sm">
			Use the Obot LLM Gateway to call OpenAI and Anthropic models with your Obot credentials.
			Configure your client below, then pick from the models you have access to.
		</p>

		{#if ready}
			<div class="flex flex-col gap-4">
				{#if anthropicModels.length > 0}
					<LLMGatewayProviderSection ctx={anthropicCtx} models={anthropicModels} />
				{/if}
				{#if openaiModels.length > 0}
					<LLMGatewayProviderSection ctx={openaiCtx} models={openaiModels} />
				{/if}
			</div>
		{/if}
	</div>
</Layout>

<svelte:head>
	<title>Obot | LLM Gateway Models</title>
</svelte:head>
