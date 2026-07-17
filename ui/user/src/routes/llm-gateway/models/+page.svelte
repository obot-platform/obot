<script lang="ts">
	import Layout from '$lib/components/Layout.svelte';
	import LLMGatewayProviderSection from '$lib/components/llm-gateway/LLMGatewayProviderSection.svelte';
	import { CommonModelProviderIds, PAGE_TRANSITION_DURATION } from '$lib/constants';
	import {
		PROVIDER_CONNECTIONS,
		type ProviderShortKey,
		type RenderContext
	} from '$lib/services/llm-gateway/types';
	import { onMount } from 'svelte';
	import { fly } from 'svelte/transition';

	let obotURL = $state('');
	let { data } = $props();

	onMount(() => {
		obotURL = window.location.origin;
	});

	let openaiModels = $derived(
		data.models.filter((m) => m.modelProvider === CommonModelProviderIds.OPENAI)
	);
	let anthropicModels = $derived(
		data.models.filter((m) => m.modelProvider === CommonModelProviderIds.ANTHROPIC)
	);
	let genericResponsesModels = $derived(
		data.models.filter((m) => m.modelProvider === CommonModelProviderIds.GENERIC_RESPONSES)
	);
	let genericResponsesDisplayModels = $derived(toCallableModelNames(genericResponsesModels));
	let bedrockModels = $derived(
		data.models.filter((m) => m.modelProvider === CommonModelProviderIds.AMAZON_BEDROCK)
	);
	let bedrockAPIKeyModels = $derived(
		data.models.filter((m) => m.modelProvider === CommonModelProviderIds.AMAZON_BEDROCK_API_KEY)
	);
	let bedrockAnthropicModels = $derived(bedrockModels.filter(isBedrockAnthropicModel));
	let bedrockOpenAIModels = $derived(bedrockModels.filter(isBedrockOpenAICompatibleModel));
	let bedrockAPIKeyAnthropicModels = $derived(bedrockAPIKeyModels.filter(isBedrockAnthropicModel));
	let bedrockAPIKeyOpenAIModels = $derived(
		bedrockAPIKeyModels.filter(isBedrockOpenAICompatibleModel)
	);
	let bedrockAnthropicDisplayModels = $derived(toCallableModelNames(bedrockAnthropicModels));
	let bedrockOpenAIDisplayModels = $derived(toCallableModelNames(bedrockOpenAIModels));
	let bedrockAPIKeyAnthropicDisplayModels = $derived(
		toCallableModelNames(bedrockAPIKeyAnthropicModels)
	);
	let bedrockAPIKeyOpenAIDisplayModels = $derived(toCallableModelNames(bedrockAPIKeyOpenAIModels));
	let azureModels = $derived(
		data.models.filter((m) => m.modelProvider === CommonModelProviderIds.AZURE)
	);
	let azureEntraModels = $derived(
		data.models.filter((m) => m.modelProvider === CommonModelProviderIds.AZURE_ENTRA)
	);
	let azureAnthropicModels = $derived(azureModels.filter(isAnthropicDialect));
	let azureOpenAIModels = $derived(azureModels.filter(isOpenAIDialect));
	let azureEntraAnthropicModels = $derived(azureEntraModels.filter(isAnthropicDialect));
	let azureEntraOpenAIModels = $derived(azureEntraModels.filter(isOpenAIDialect));
	let azureAnthropicDisplayModels = $derived(toCallableModelNames(azureAnthropicModels));
	let azureOpenAIDisplayModels = $derived(toCallableModelNames(azureOpenAIModels));
	let azureEntraAnthropicDisplayModels = $derived(toCallableModelNames(azureEntraAnthropicModels));
	let azureEntraOpenAIDisplayModels = $derived(toCallableModelNames(azureEntraOpenAIModels));

	function modelID(model: (typeof data.models)[number]) {
		return model.targetModel || model.name;
	}

	function isBedrockAnthropicModel(model: (typeof data.models)[number]) {
		return modelID(model).startsWith('anthropic.');
	}

	function isBedrockOpenAICompatibleModel(model: (typeof data.models)[number]) {
		const id = modelID(model);
		return id.startsWith('openai.') || id.startsWith('google.');
	}

	function isAnthropicDialect(model: (typeof data.models)[number]) {
		return model.dialect === 'AnthropicMessages';
	}

	function isOpenAIDialect(model: (typeof data.models)[number]) {
		return model.dialect === 'OpenAIResponses';
	}

	function toCallableModelNames(models: typeof data.models): typeof data.models {
		return models.map((model) => ({
			...model,
			name: modelID(model)
		}));
	}

	function buildCtx(shortKey: ProviderShortKey, models: typeof data.models): RenderContext {
		const provider = PROVIDER_CONNECTIONS[shortKey];
		return {
			provider,
			obotURL,
			baseURL: `${obotURL}/api/llm-proxy/${provider.routePath}`,
			exampleModel: models[0]?.name
		};
	}

	let openaiCtx = $derived(buildCtx('openai', openaiModels));
	let anthropicCtx = $derived(buildCtx('anthropic', anthropicModels));
	let genericResponsesCtx = $derived(buildCtx('generic-responses', genericResponsesDisplayModels));
	let bedrockAnthropicCtx = $derived(
		buildCtx('aws-bedrock-anthropic', bedrockAnthropicDisplayModels)
	);
	let bedrockOpenAICtx = $derived(buildCtx('aws-bedrock-openai', bedrockOpenAIDisplayModels));
	let bedrockAPIKeyAnthropicCtx = $derived(
		buildCtx('aws-bedrock-api-key-anthropic', bedrockAPIKeyAnthropicDisplayModels)
	);
	let bedrockAPIKeyOpenAICtx = $derived(
		buildCtx('aws-bedrock-api-key-openai', bedrockAPIKeyOpenAIDisplayModels)
	);
	let azureAnthropicCtx = $derived(buildCtx('azure-anthropic', azureAnthropicDisplayModels));
	let azureOpenAICtx = $derived(buildCtx('azure-openai', azureOpenAIDisplayModels));
	let azureEntraAnthropicCtx = $derived(
		buildCtx('azure-entra-anthropic', azureEntraAnthropicDisplayModels)
	);
	let azureEntraOpenAICtx = $derived(buildCtx('azure-entra-openai', azureEntraOpenAIDisplayModels));

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
			Use the Obot LLM Gateway to call OpenAI, Anthropic, Generic Responses, Amazon Bedrock, and
			Azure models with your Obot credentials. Configure your client below, then pick from the
			models you have access to.
		</p>

		{#if ready}
			<div class="flex flex-col gap-4">
				{#if anthropicModels.length > 0}
					<LLMGatewayProviderSection ctx={anthropicCtx} models={anthropicModels} />
				{/if}
				{#if openaiModels.length > 0}
					<LLMGatewayProviderSection ctx={openaiCtx} models={openaiModels} />
				{/if}
				{#if genericResponsesModels.length > 0}
					<LLMGatewayProviderSection
						ctx={genericResponsesCtx}
						models={genericResponsesDisplayModels}
					/>
				{/if}
				{#if bedrockAnthropicModels.length > 0}
					<LLMGatewayProviderSection
						ctx={bedrockAnthropicCtx}
						models={bedrockAnthropicDisplayModels}
					/>
				{/if}
				{#if bedrockOpenAIModels.length > 0}
					<LLMGatewayProviderSection ctx={bedrockOpenAICtx} models={bedrockOpenAIDisplayModels} />
				{/if}
				{#if bedrockAPIKeyAnthropicModels.length > 0}
					<LLMGatewayProviderSection
						ctx={bedrockAPIKeyAnthropicCtx}
						models={bedrockAPIKeyAnthropicDisplayModels}
					/>
				{/if}
				{#if bedrockAPIKeyOpenAIModels.length > 0}
					<LLMGatewayProviderSection
						ctx={bedrockAPIKeyOpenAICtx}
						models={bedrockAPIKeyOpenAIDisplayModels}
					/>
				{/if}
				{#if azureAnthropicModels.length > 0}
					<LLMGatewayProviderSection ctx={azureAnthropicCtx} models={azureAnthropicDisplayModels} />
				{/if}
				{#if azureOpenAIModels.length > 0}
					<LLMGatewayProviderSection ctx={azureOpenAICtx} models={azureOpenAIDisplayModels} />
				{/if}
				{#if azureEntraAnthropicModels.length > 0}
					<LLMGatewayProviderSection
						ctx={azureEntraAnthropicCtx}
						models={azureEntraAnthropicDisplayModels}
					/>
				{/if}
				{#if azureEntraOpenAIModels.length > 0}
					<LLMGatewayProviderSection
						ctx={azureEntraOpenAICtx}
						models={azureEntraOpenAIDisplayModels}
					/>
				{/if}
			</div>
		{/if}
	</div>
</Layout>

<svelte:head>
	<title>Obot | LLM Gateway Models</title>
</svelte:head>
