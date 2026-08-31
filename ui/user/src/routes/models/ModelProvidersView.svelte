<script lang="ts">
	import ListModels from '$lib/components/admin/ListModels.svelte';
	import ProviderCard from '$lib/components/admin/ProviderCard.svelte';
	import ProviderConfigure from '$lib/components/admin/ProviderConfigure.svelte';
	import LicenseProviderDialog from '$lib/components/admin/license/LicenseProviderDialog.svelte';
	import { CommonModelProviderIds, PAGE_TRANSITION_DURATION } from '$lib/constants';
	import { getAdminModels } from '$lib/context/admin/models.svelte.js';
	import { HttpError } from '$lib/errors.js';
	import { AdminService, type ModelProvider as ModelProviderType } from '$lib/services';
	import { sortModelProviders } from '$lib/sort.js';
	import {
		accessibleModels,
		defaultModelAliases as defaultModelAliasesStore,
		license
	} from '$lib/stores';
	import { profile, version } from '$lib/stores';
	import { adminConfigStore } from '$lib/stores/adminConfig.svelte.js';
	import { delay } from '$lib/utils';
	import { TriangleAlert } from '@lucide/svelte';
	import { onMount, untrack } from 'svelte';
	import { fade } from 'svelte/transition';

	const nanobotIntegratedModels = [
		CommonModelProviderIds.OPENAI,
		CommonModelProviderIds.ANTHROPIC,
		CommonModelProviderIds.AMAZON_BEDROCK,
		CommonModelProviderIds.AMAZON_BEDROCK_API_KEY,
		CommonModelProviderIds.AZURE,
		CommonModelProviderIds.AZURE_ENTRA,
		CommonModelProviderIds.GENERIC_RESPONSES
	];

	interface Props {
		modelProviders: ModelProviderType[];
		onFirstConfigure?: (required: boolean) => void;
	}

	let { modelProviders: initialProviders, onFirstConfigure }: Props = $props();
	let modelProviders = $state(untrack(() => initialProviders));
	let providerConfigure = $state<ReturnType<typeof ProviderConfigure>>();
	let configuringModelProvider = $state<ModelProviderType>();
	let configuringModelProviderValues = $state<Record<string, string>>();
	let configureError = $state<string>();
	let loading = $state(false);
	let licenseRequiredProvider = $state<ModelProviderType>();

	let atLeastOneConfigured = $derived(modelProviders.some((provider) => provider.configured));
	let hasAnthropicAwsBedrockConfigured = $derived(
		!!modelProviders.find((provider) => provider.id === CommonModelProviderIds.ANTHROPIC_BEDROCK)
			?.configured
	);
	let availableModelProviders = $derived(
		hasAnthropicAwsBedrockConfigured
			? modelProviders
			: modelProviders.filter(
					(provider) => provider.id !== CommonModelProviderIds.ANTHROPIC_BEDROCK
				)
	);
	let modelProvidersToShow = $derived(
		availableModelProviders.filter((provider) => nanobotIntegratedModels.includes(provider.id))
	);
	let isAdminReadonly = $derived(profile.current.isAdminReadonly?.());
	const defaultModelAliases = $derived(defaultModelAliasesStore.current);
	const adminModels = getAdminModels();

	onMount(async () => {
		const models = await AdminService.listModels({ all: true });
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

	async function waitForProviderReady(providerId: string) {
		const startTime = Date.now();
		const timeout = 30000;

		while (Date.now() - startTime < timeout) {
			const provider = await AdminService.getModelProvider(providerId);
			if (provider.modelsBackPopulated === true) {
				return;
			}

			if (provider.configured === false) {
				throw new Error(`Model provider ${providerId} became unconfigured`);
			}

			await delay(500);
		}

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

				await waitForProviderReady(configuringModelProvider.id);

				modelProviders = await AdminService.listModelProviders();
				adminConfigStore.updateModelProviders(modelProviders);
				adminModels.items = await AdminService.listModels({ all: true });

				await accessibleModels.refresh();

				providerConfigure?.close();
				if (!isAlreadyConfigured) {
					const required =
						defaultModelAliases.length === 0 || defaultModelAliases.every((alias) => !alias.model);
					onFirstConfigure?.(required);
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

<div class="mb-4 @container" in:fade={{ duration }}>
	<div class="flex flex-col gap-8">
		{#if !atLeastOneConfigured && version.current.agentsEnabled !== false}
			<div class="notification-alert mb-4 flex flex-col gap-2">
				<div class="flex items-center gap-2">
					<TriangleAlert class="size-6 shrink-0 self-start text-warning" />
					<p class="my-0.5 flex flex-col text-sm font-semibold">No Model Providers Configured!</p>
				</div>
				<span class="text-sm font-light break-all">
					To use Obot chat features, you'll need to set up a Model Provider. Select and configure
					one below to get started!
				</span>
			</div>
		{/if}
	</div>
	<div class="grid grid-cols-1 gap-4 md:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4">
		{#each sortedModelProviders as modelProvider (modelProvider.id)}
			<ProviderCard
				provider={modelProvider}
				deprecated={modelProvider.id === CommonModelProviderIds.ANTHROPIC_BEDROCK}
				onConfigure={async () => {
					if (modelProvider.missingEntitlements && modelProvider.missingEntitlements.length > 0) {
						licenseRequiredProvider = modelProvider;
						return;
					}

					configuringModelProvider = modelProvider;
					try {
						configuringModelProviderValues = await AdminService.revealModelProvider(
							modelProvider.id
						);
					} catch (err) {
						if (!(err instanceof HttpError) || err.statusCode !== 404) {
							console.error('An error occurred while revealing model provider credentials', err);
						}
					}
					providerConfigure?.open();
				}}
				onDeconfigure={async () => {
					await AdminService.deconfigureModelProvider(modelProvider.id);
					modelProviders = await AdminService.listModelProviders();
					adminConfigStore.updateModelProviders(modelProviders);
					accessibleModels.refresh();
				}}
				readonly={isAdminReadonly}
				licenseKey={license.current.licenseKey}
			>
				{#snippet configuredActions(provider)}
					<ListModels provider={provider as ModelProviderType} readonly={isAdminReadonly} />
				{/snippet}
			</ProviderCard>
		{/each}
	</div>
</div>

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
			<p class="text-muted-content py-4 font-light">
				Note: Anthropic does not have an embeddings model.
			</p>
		{/if}
	{/snippet}
</ProviderConfigure>

<LicenseProviderDialog
	bind:provider={licenseRequiredProvider}
	licenseKey={license.current.licenseKey}
/>
