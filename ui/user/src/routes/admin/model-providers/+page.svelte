<script lang="ts">
	import ProviderCard from '$lib/components/admin/ProviderCard.svelte';
	import { AdminService, type ModelProvider as ModelProviderType } from '$lib/services';
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
	import { sortModelProviders } from '$lib/sort.js';
	import { AlertTriangle } from 'lucide-svelte';
	import { adminConfigStore } from '$lib/stores/adminConfig.svelte.js';
	import { profile } from '$lib/stores/index.js';

	let { data } = $props();
	let { modelProviders: initialModelProviders } = data;

	let modelProviders = $state(initialModelProviders);
	let providerConfigure = $state<ReturnType<typeof ProviderConfigure>>();
	let defaultModelsDialog = $state<ReturnType<typeof DefaultModels>>();
	let configuringModelProvider = $state<ModelProviderType>();
	let configuringModelProviderValues = $state<Record<string, string>>();
	let configureError = $state<string>();
	let loading = $state(false);
	let atLeastOneConfigured = $derived(modelProviders.some((provider) => provider.configured));
	let hasAnthropicAwsBedrockConfigured = $derived(
		!!modelProviders.find((provider) => provider.id === CommonModelProviderIds.ANTHROPIC_BEDROCK)
			?.configured
	);
	let modelProvidersToShow = $derived(
		hasAnthropicAwsBedrockConfigured
			? modelProviders
			: modelProviders.filter(
					(provider) => provider.id !== CommonModelProviderIds.ANTHROPIC_BEDROCK
				)
	);
	let isAdminReadonly = $derived(profile.current.isAdminReadonly?.());

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

	let sortedModelProviders = $derived(sortModelProviders(modelProvidersToShow));

	// waitForProviderReady blocks until the models of the model provider with the given providerID
	// are back populated.
	// If its models aren't populated or the provider becomes unconfigured within 10 seconds, it
	// throws an exception.
	async function waitForProviderReady(providerId: string) {
		const startTime = Date.now();
		const timeout = 30000; // 30 seconds

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
			const isAlreadyConfigured = configuringModelProvider.configured;
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
				adminConfigStore.updateModelProviders(modelProviders);
				adminModels.items = await AdminService.listModels();

				providerConfigure?.close();
				if (!isAlreadyConfigured) {
					// if first time configuring, open the default models dialog
					defaultModelsDialog?.open(true);
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

<Layout title="Model Providers">
	<div class="my-4" in:fade={{ duration }} out:fade={{ duration }}>
		<div class="flex flex-col gap-8">
			{#if !atLeastOneConfigured}
				<div class="notification-alert flex flex-col gap-2">
					<div class="flex items-center gap-2">
						<AlertTriangle class="size-6 flex-shrink-0 self-start text-yellow-500" />
						<p class="my-0.5 flex flex-col text-sm font-semibold">No Model Providers Configured!</p>
					</div>
					<span class="text-sm font-light break-all">
						To use Obot chat features, you'll need to set up a Model Provider. Select and configure
						one below to get started!
					</span>
				</div>
			{/if}
		</div>
		<div class="grid grid-cols-1 gap-4 py-8 md:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4">
			{#each sortedModelProviders as modelProvider (modelProvider.id)}
				<ProviderCard
					provider={modelProvider}
					deprecated={modelProvider.id === CommonModelProviderIds.ANTHROPIC_BEDROCK}
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
						adminConfigStore.updateModelProviders(modelProviders);
					}}
					readonly={isAdminReadonly}
				>
					{#snippet configuredActions(provider)}
						<ListModels {provider} readonly={isAdminReadonly} />
					{/snippet}
				</ProviderCard>
			{/each}
		</div>
	</div>

	{#snippet rightNavActions()}
		<DefaultModels
			bind:this={defaultModelsDialog}
			availableModels={adminModels.items}
			readonly={isAdminReadonly}
		/>
	{/snippet}
</Layout>

<ProviderConfigure
	bind:this={providerConfigure}
	provider={configuringModelProvider}
	onConfigure={handleModelProviderConfigure}
	values={configuringModelProviderValues}
	error={configureError}
	{loading}
	readonly={isAdminReadonly}
>
	{#snippet note()}
		{#if configuringModelProvider && isAnthropic(configuringModelProvider)}
			<p class="text-on-surface1 py-4 font-light">
				Note: Anthropic does not have an embeddings model and recommends Voyage AI.
			</p>
		{/if}
	{/snippet}
</ProviderConfigure>

<svelte:head>
	<title>Obot | Model Providers</title>
</svelte:head>
