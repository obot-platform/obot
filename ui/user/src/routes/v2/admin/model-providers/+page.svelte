<script lang="ts">
	import ProviderCard from '$lib/components/admin/ProviderCard.svelte';
	import { AdminService, type ModelProvider as ModelProviderType } from '$lib/services';
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

	let { data } = $props();
	let { modelProviders: initialModelProviders } = data;

	let modelProviders = $state(initialModelProviders);
	let providerConfigure = $state<ReturnType<typeof ProviderConfigure>>();
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

	let sortedModelProviders = $derived(sortModelProviders(modelProviders));

	async function handleModelProviderConfigure(form: Record<string, string>) {
		if (configuringModelProvider) {
			loading = true;
			configureError = undefined;
			try {
				await AdminService.validateModelProvider(configuringModelProvider.id, form);
				await AdminService.configureModelProvider(configuringModelProvider.id, form);
				modelProviders = await AdminService.listModelProviders();
				adminModels.items = await AdminService.listModels();
				providerConfigure?.close();
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
				<DefaultModels />
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
