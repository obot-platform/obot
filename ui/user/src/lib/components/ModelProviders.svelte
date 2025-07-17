<script lang="ts">
	import { onMount } from 'svelte';
	import type { Project, ModelProvider } from '$lib/services/chat/types';
	import {
		updateProject,
		listAvailableProjectModels,
		listModelProviders
	} from '$lib/services/chat/operations';
	import { ChevronDown, Loader2 } from 'lucide-svelte';
	import ModelProviderCard from './ModelProviderCard.svelte';
	import { twMerge } from 'tailwind-merge';
	import { fade } from 'svelte/transition';

	let { project = $bindable() }: { project: Project } = $props();

	let modelProviders: ModelProvider[] = $state([]);
	let isLoading = $state(true);
	let error: string | null = $state(null);
	let isSaving = $state(false);

	// Convenience getters for derived values
	let defaultModelProvider = $derived(project.defaultModelProvider || '');
	let defaultModel = $derived(project.defaultModel || '');
	let selectedModels = $derived<Record<string, string[]>>(project.models || {});

	// Track available models for each provider
	let availableModels: Record<string, string[]> = $state({});
	let loadingModels: Record<string, boolean> = $state({});

	let isDefaultModelSelectOpen = $state(false);

	const hasOneModelSelected = $derived(
		Object.values(selectedModels).some((provider) => provider.length)
	);

	// Load model providers
	async function loadModelProviders() {
		isLoading = true;
		error = null;

		try {
			const response = await listModelProviders(project.assistantID, project.id);

			// Sort providers by name for consistent display order
			modelProviders = (response.items || []).sort((a, b) => {
				return a.name.localeCompare(b.name);
			});

			// Load available models for each configured provider
			for (const provider of modelProviders) {
				if (provider.configured) {
					loadAvailableModels(provider.id);
				}
			}
		} catch (err) {
			error = 'Failed to load model providers';
			console.error(err);
		} finally {
			isLoading = false;
		}
	}

	// Load available models for a provider
	async function loadAvailableModels(providerId: string) {
		loadingModels[providerId] = true;

		try {
			const models = await listAvailableProjectModels(project.assistantID, project.id, providerId);

			availableModels[providerId] = (models.data || [])
				.filter((m) => m.metadata && m.metadata.usage === 'llm')
				.map((m) => m.id)
				.sort((a, b) => a.localeCompare(b));
		} catch (err) {
			console.error(`Failed to load models for provider ${providerId}`, err);
			availableModels[providerId] = [];
		} finally {
			loadingModels[providerId] = false;
		}
	}

	// Save changes to the server
	async function saveChanges() {
		try {
			isSaving = true;
			const updatedProject = await updateProject(project);

			// Update the project prop directly
			project = updatedProject;
		} catch (err) {
			console.error('Failed to save model configuration', err);
			error = 'Failed to save changes. Please try again.';
		} finally {
			isSaving = false;
		}
	}

	// Update the project's model selection directly and trigger auto-save
	function updateProjectModels(
		models: Record<string, string[]>,
		defModel: string,
		defProvider: string
	) {
		project = {
			...project,
			models,
			defaultModelProvider: defProvider,
			defaultModel: defModel
		};

		saveChanges();
	}

	// Update default model/provider and trigger auto-save
	function updateDefaultModel(value: string) {
		if (!value) {
			// Handle clearing the selection
			updateProjectModels({ ...selectedModels }, '', '');
			return;
		}

		// Split the value to get model and provider
		const [modelName, providerId] = value.split('|||');
		if (modelName && providerId) {
			updateProjectModels({ ...selectedModels }, modelName, providerId);
		}
	}

	onMount(() => {
		if (project && project.id) {
			loadModelProviders();
		}
	});
</script>

<div class="flex w-full flex-col px-4 pt-10 pb-10 lg:px-32">
	<div class="mb-4 flex items-center justify-between">
		<div class="flex w-full flex-col">
			<div class="flex justify-between">
				<h3 class="text-lg font-semibold">Model Providers</h3>
				{#if isSaving}
					<div
						class="text-muted flex items-center gap-1 text-xs"
						in:fade={{ duration: 200 }}
						out:fade={{ duration: 200, delay: 300 }}
					>
						<div
							class="size-3 animate-spin rounded-full border-2 border-current border-t-transparent"
						></div>
						<span>Saving...</span>
					</div>
				{/if}
			</div>
			<p class="text-gray text-xs">
				Configure model providers and select models to make them available to all threads and tasks.
				The default model will be used in place of Obot’s built-in model.
			</p>
		</div>
	</div>

	{#if error}
		<div class="mb-4 text-red-500">{error}</div>
		<button
			class="bg-secondary text-secondary-foreground rounded-md px-4 py-2 text-sm font-medium"
			onclick={loadModelProviders}
		>
			Retry
		</button>
	{/if}

	{#if isLoading}
		<div class="flex justify-center p-4">
			<Loader2 class="h-4 w-4 animate-spin" />
		</div>
	{:else if modelProviders.length === 0}
		<div class="text-muted">No model providers available</div>
	{:else}
		<!-- Default Model Provider Section -->
		<div class={twMerge('mb-6 flex flex-col gap-2', !hasOneModelSelected && 'opacity-50')}>
			<h4 class="text-md font-medium">Default Model</h4>

			<!-- Not the best approach -->
			<!-- TODO: Use a 3rd-party UI library or create an internal custom component in the future -->
			<div class="relative">
				<select
					class="border-surface2 dark:bg-surface1 w-full appearance-none rounded-md border px-4 py-4 text-sm"
					value={defaultModelProvider && defaultModel
						? `${defaultModel}|||${defaultModelProvider}`
						: ''}
					onchange={(e) => updateDefaultModel(e.currentTarget.value)}
					onclick={() => (isDefaultModelSelectOpen = !isDefaultModelSelectOpen)}
				>
					<option class="dark:bg-surface3" value="" disabled
						>Select the default model for this project</option
					>
					{#if hasOneModelSelected}
						{#each Object.entries(selectedModels) as [providerId, models] (providerId)}
							{#each models as model (model)}
								{#if providerId}
									{@const provider = modelProviders.find((p) => p.id === providerId)}

									{#if provider}
										<option class="dark:bg-surface3" value={`${model}|||${providerId}`}>
											{model} ({provider.name})
										</option>
									{/if}
								{/if}
							{/each}
						{/each}
					{/if}
				</select>

				<div
					class={twMerge(
						'absolute inset-y-0 right-0 flex aspect-square h-full items-center justify-center transition-transform duration-200',
						isDefaultModelSelectOpen && 'rotate-180'
					)}
				>
					<ChevronDown />
				</div>
			</div>
		</div>

		<!-- Available Model Providers -->
		<div class="flex flex-col gap-0">
			<h4 class="text-md mb-2 font-medium">Available Model Providers</h4>
			<div class="model-providers-cards flex flex-col gap-4 2xl:grid 2xl:grid-cols-2">
				{#each modelProviders as provider (provider.id)}
					<ModelProviderCard {provider} bind:project />
				{/each}
			</div>
		</div>
	{/if}
</div>

<style>
	.markdown-content :global(a) {
		text-decoration: underline;
		font-weight: 500;
	}
	.markdown-content :global(a:hover) {
		opacity: 0.8;
	}
	.markdown-content :global(p) {
		margin-bottom: 0.5rem;
	}
	.markdown-content :global(ul),
	.markdown-content :global(ol) {
		margin-left: 1.5rem;
		margin-bottom: 0.5rem;
	}
	.markdown-content :global(ul) {
		list-style-type: disc;
	}
	.markdown-content :global(ol) {
		list-style-type: decimal;
	}
	.markdown-content :global(code) {
		font-family: monospace;
		background-color: rgba(0, 0, 0, 0.1);
		padding: 0.125rem 0.25rem;
		border-radius: 0.25rem;
	}
	.markdown-content :global(h1),
	.markdown-content :global(h2),
	.markdown-content :global(h3),
	.markdown-content :global(h4),
	.markdown-content :global(h5),
	.markdown-content :global(h6) {
		font-weight: 600;
		margin-top: 0.5rem;
		margin-bottom: 0.5rem;
	}
</style>
