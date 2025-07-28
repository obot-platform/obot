<script lang="ts">
	import ProviderCard from '$lib/components/admin/ProviderCard.svelte';
	import {
		AdminService,
		type ModelProvider as ModelProviderType,
		ModelAliasToUsageMap,
		ModelAlias
	} from '$lib/services';
	import { delay } from '$lib/utils';
	import Layout from '$lib/components/Layout.svelte';
	import {
		CommonModelProviderIds,
		PAGE_TRANSITION_DURATION,
		RecommendedModelProviders
	} from '$lib/constants';
	import { fade } from 'svelte/transition';
	import ProviderConfigure from '$lib/components/admin/ProviderConfigure.svelte';
	import ListModels from '$lib/components/admin/ListModels.svelte';
	import { getAdminModels, initModels } from '$lib/context/admin/models.svelte.js';
	import { onMount } from 'svelte';
	import DefaultModels from '$lib/components/admin/DefaultModels.svelte';

	let { data } = $props();
	let { modelProviders: initialModelProviders } = data;

	let modelProviders = $state(initialModelProviders);
	let providerConfigure = $state<ReturnType<typeof ProviderConfigure>>();
	let defaultModelsDialog = $state<ReturnType<typeof DefaultModels>>();
	let configuringModelProvider = $state<ModelProviderType>();
	let configuringModelProviderValues = $state<Record<string, string>>();
	let configureError = $state<string>();
	let loading = $state(false);

	initModels([]);
	const adminModels = getAdminModels();

	onMount(async () => {
		const models = await AdminService.listModels();
		adminModels.items = models;
	});

	const duration = PAGE_TRANSITION_DURATION;

	function isAnthropic(provider: ModelProviderType) {
		return (
			provider.id === CommonModelProviderIds.ANTHROPIC ||
			provider.id === CommonModelProviderIds.ANTHROPIC_BEDROCK
		);
	}

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

			return a.id.localeCompare(b.id);
		});
	}

	let sortedModelProviders = $derived(sortModelProviders(modelProviders));

	// shouldOpenDefaultModelsDialog returns true IFF the model provider with the given providerID
	// providers models that can be used for the unset default model aliases.
	async function shouldOpenDefaultModelsDialog(providerId: string): Promise<boolean> {
		try {
			// Get current default model aliases
			const defaultModelAliases = await AdminService.listDefaultModelAliases();

			// Find aliases with empty model fields
			const unsetAliases = defaultModelAliases
				.filter((alias) => !alias.model || alias.model.trim() === '')
				.map((alias) => alias.alias);

			if (unsetAliases.length === 0) {
				return false; // No unset aliases, no need to open dialog
			}

			// Get usage types for unset aliases
			const unsetUsageTypes = unsetAliases
				.map((alias) => ModelAliasToUsageMap[alias as ModelAlias])
				.filter((usage) => !!usage);

			if (unsetUsageTypes.length === 0) {
				return false; // No valid usage types found
			}

			// Get models from the newly configured provider
			const providerModels = adminModels.items.filter(
				(model) => model.modelProvider === providerId && model.active
			);

			// Check if any provider models have usage types that match unset aliases
			// eslint-disable-next-line @typescript-eslint/no-explicit-any
			return providerModels.some((model) => unsetUsageTypes.includes(model.usage as any));
		} catch (error) {
			console.error('Error checking if default models dialog should open:', error);
			return false;
		}
	}

	// waitForProviderReady blocks until the models of the model provider with the given providerID
	// are back populated.
	// If its models aren't populated or the provider becomes unconfigured within 10 seconds, it
	// throws an exception.
	async function waitForProviderReady(providerId: string) {
		const startTime = Date.now();
		const timeout = 10000; // 10 seconds

		while (Date.now() - startTime < timeout) {
			const provider = await AdminService.getModelProvider(providerId);
			if (provider.modelsBackPopulated === true) {
				return;
			}

			if (provider.configured === false) {
				throw new Error(`Model provider ${providerId} became unconfigured`);
			}

			// Wait before next poll
			await delay(500);
		}

		// Timeout waiting for models to be back populated
		throw new Error(`Timeout waiting for models to be populated for provider ${providerId}`);
	}

	async function handleModelProviderConfigure(form: Record<string, string>) {
		if (configuringModelProvider) {
			loading = true;
			configureError = undefined;
			try {
				await AdminService.validateModelProvider(configuringModelProvider.id, form);
				await AdminService.configureModelProvider(configuringModelProvider.id, form);

				// Wait for the provider's models to be back populated before fetching its models.
				// Note: If we skip this check, the provider's models won't be returned when listing
				// available models.
				await waitForProviderReady(configuringModelProvider.id);

				// Fetch the updated model providers and available models
				modelProviders = await AdminService.listModelProviders();
				adminModels.items = await AdminService.listModels();

				// Close the provider configuration dialog
				providerConfigure?.close();

				// Open the default models dialog if the model provider adds models that can be used
				// for unset default model aliases.
				if (await shouldOpenDefaultModelsDialog(configuringModelProvider.id)) {
					defaultModelsDialog?.open();
				}
			} catch (err: unknown) {
				if (err instanceof Error) {
					const errorMessageMatch = err.message.match(/{"error":\s*"(.*?)"}/);
					if (errorMessageMatch) {
						const errorMessage = JSON.parse(errorMessageMatch[0]).error;
						configureError = errorMessage;
					}
				} else {
					configureError = 'Failed to configure model provider';
				}
			} finally {
				loading = false;
			}
		}
	}
</script>

<Layout>
	<div class="my-4" in:fade={{ duration }} out:fade={{ duration }}>
		<div class="flex flex-col gap-8">
			<h1 class="flex items-center justify-between gap-4 text-2xl font-semibold">
				Model Providers
				<DefaultModels bind:this={defaultModelsDialog} availableModels={adminModels.items} />
			</h1>
		</div>
		<div class="grid grid-cols-2 gap-4 py-8 md:grid-cols-3 lg:grid-cols-4">
			{#each sortedModelProviders as modelProvider (modelProvider.id)}
				<ProviderCard
					provider={modelProvider}
					recommended={RecommendedModelProviders.includes(modelProvider.id)}
					onConfigure={async () => {
						configuringModelProvider = modelProvider;
						try {
							configuringModelProviderValues = await AdminService.revealModelProvider(
								modelProvider.id
							);
						} catch (err) {
							// if 404, ignore, it means no credentials are set
							if (err instanceof Error && !err.message.includes('404')) {
								console.error('An error occurred while revealing model provider credentials', err);
							}
						}
						providerConfigure?.open();
					}}
					onDeconfigure={async () => {
						await AdminService.deconfigureModelProvider(modelProvider.id);
						modelProviders = await AdminService.listModelProviders();
					}}
				>
					{#snippet configuredActions(provider)}
						<ListModels {provider} />
					{/snippet}
				</ProviderCard>
			{/each}
		</div>
	</div>
</Layout>

<ProviderConfigure
	bind:this={providerConfigure}
	provider={configuringModelProvider}
	onConfigure={handleModelProviderConfigure}
	values={configuringModelProviderValues}
	error={configureError}
	{loading}
>
	{#snippet note()}
		{#if configuringModelProvider && isAnthropic(configuringModelProvider)}
			<p class="py-4 font-light text-gray-400 dark:text-gray-600">
				Note: Anthropic does not have an embeddings model and recommends Voyage AI.
			</p>
		{/if}
	{/snippet}
</ProviderConfigure>

<svelte:head>
	<title>Obot | Model Providers</title>
</svelte:head>
