<script lang="ts">
	import Layout from '$lib/components/Layout.svelte';
	import LLMGatewayProviderSection from '$lib/components/llm-gateway/LLMGatewayProviderSection.svelte';
	import { CommonModelProviderIds, PAGE_TRANSITION_DURATION } from '$lib/constants';
	import { PROVIDER_CONNECTIONS, type RenderContext } from '$lib/services/llm-gateway/types';
	import { KeyRound } from 'lucide-svelte';
	import { onMount } from 'svelte';
	import { fly } from 'svelte/transition';

	let { data } = $props();

	let obotURL = $state('');

	onMount(() => {
		obotURL = window.location.origin;
	});

	let openaiModels = $derived(
		data.models.filter((m) => m.modelProvider === CommonModelProviderIds.OPENAI)
	);
	let anthropicModels = $derived(
		data.models.filter((m) => m.modelProvider === CommonModelProviderIds.ANTHROPIC)
	);

	function buildCtx(shortKey: 'openai' | 'anthropic', models: typeof data.models): RenderContext {
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

	let hasAny = $derived(openaiModels.length > 0 || anthropicModels.length > 0);
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

		{#if !hasAny}
			<div class="mt-12 flex w-md flex-col items-center gap-4 self-center text-center">
				<KeyRound class="text-base-content/80 size-24 opacity-25" />
				<h4 class="text-muted-content text-lg font-semibold">No gateway models available</h4>
				<p class="text-muted-content text-sm font-light">
					You don't currently have access to any OpenAI or Anthropic models through the gateway.
					Contact an administrator to request access.
				</p>
			</div>
		{:else if ready}
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
