<script lang="ts">
	import { onMount } from 'svelte';
	import type { Project, ModelProvider } from '$lib/services/chat/types';
	import {
		updateProject,
		listAvailableModels,
		listModelProviders,
		configureModelProvider,
		deconfigureModelProvider,
		getModelProviderConfig
	} from '$lib/services/chat/operations';
	import { CheckCircleIcon, CircleIcon, Loader2 } from 'lucide-svelte';
	import { toHTMLFromMarkdown } from '$lib/markdown';
	import { darkMode } from '$lib/stores';

	let { project = $bindable() }: { project: Project } = $props();

	let modelProviders: ModelProvider[] = $state([]);
	let isLoading = $state(true);
	let error: string | null = $state(null);
	let configuringProvider: string | null = $state(null);
	let configFormData: Record<string, string> = $state({});
	let configIsLoading = $state(true);
	let isSaving = $state(false);

	// Convenience getters for derived values
	let defaultModelProvider = $derived(project.defaultModelProvider || '');
	let defaultModel = $derived(project.defaultModel || '');
	let selectedModels = $derived<Record<string, string[]>>(project.models || {});

	// Track available models for each provider
	let availableModels: Record<string, string[]> = $state({});
	let loadingModels: Record<string, boolean> = $state({});

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
			const models = await listAvailableModels(project.assistantID, project.id, providerId);

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

	// Configure model provider
	async function handleConfigureModelProvider(
		provider: ModelProvider,
		config: Record<string, string>
	) {
		try {
			await configureModelProvider(project.assistantID, project.id, provider.id, config);
			configuringProvider = null;
			await loadModelProviders();

			// Load models for newly configured provider
			loadAvailableModels(provider.id);
		} catch (err) {
			console.error(`Failed to configure ${provider.name}`, err);
		}
	}

	// Deconfigure model provider
	async function handleDeconfigureModelProvider(provider: ModelProvider) {
		try {
			await deconfigureModelProvider(project.assistantID, project.id, provider.id);

			// Update the project model list directly
			const updatedModels = { ...(project.models || {}) };
			delete updatedModels[provider.id];

			// Handle default model updates
			let updatedDefaultProvider = defaultModelProvider;
			let updatedDefaultModel = defaultModel;

			if (defaultModelProvider === provider.id) {
				updatedDefaultProvider = '';
				updatedDefaultModel = '';
			}

			// Update the project directly
			updateProjectModels(updatedModels, updatedDefaultModel, updatedDefaultProvider);

			// Refresh providers list
			await loadModelProviders();
		} catch (err) {
			console.error(`Failed to deconfigure ${provider.name}`, err);
		}
	}

	// Toggle model selection
	function toggleModelSelection(providerId: string, modelName: string) {
		// Take a snapshot of current models
		const currentModels = { ...(project.models || {}) };
		const currentProviderModels = [...(currentModels[providerId] || [])];

		const isSelected = currentProviderModels.includes(modelName);

		let updatedProviderModels;
		if (isSelected) {
			// Remove model
			updatedProviderModels = currentProviderModels.filter((m) => m !== modelName);
		} else {
			// Add model
			updatedProviderModels = [...currentProviderModels, modelName];
		}

		// Update models
		const updatedModels = {
			...currentModels,
			[providerId]: updatedProviderModels
		};

		// If there are no models for this provider, remove the provider entry
		if (updatedProviderModels.length === 0) {
			delete updatedModels[providerId];
		}

		// Check if we're removing the default model
		let updatedDefaultModel = defaultModel;
		let updatedDefaultProvider = defaultModelProvider;

		if (defaultModelProvider === providerId && defaultModel === modelName && isSelected) {
			// We're removing the current default model, so clear it
			updatedDefaultModel = '';
			updatedDefaultProvider = '';
		}

		// Update the project directly and trigger auto-save
		updateProjectModels(updatedModels, updatedDefaultModel, updatedDefaultProvider);
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

	// Load configuration for a model provider
	async function loadProviderConfig(providerId: string) {
		configIsLoading = true;
		configFormData = {};

		try {
			const data = await getModelProviderConfig(project.assistantID, project.id, providerId);
			configFormData = data || {};
		} catch (err) {
			console.error(`Failed to get configuration for provider ${providerId}`, err);
		} finally {
			configIsLoading = false;
		}
	}

	function handleFormSubmit(provider: ModelProvider, event: Event) {
		event.preventDefault();
		handleConfigureModelProvider(provider, configFormData);
	}

	function updateFormValue(key: string, value: string) {
		configFormData = { ...configFormData, [key]: value };
	}

	// When a provider is selected for configuration, load its config
	$effect(() => {
		if (configuringProvider) {
			loadProviderConfig(configuringProvider);
		}
	});

	onMount(() => {
		if (project && project.id) {
			loadModelProviders();
		}
	});
</script>

<div class="px-4 pt-0 pb-4">
	<div class="mb-4 flex items-center justify-between">
		<h3 class="text-lg font-semibold">Model Providers</h3>

		{#if isSaving}
			<div class="text-muted flex items-center gap-1 text-sm">
				<div
					class="h-4 w-4 animate-spin rounded-full border-2 border-current border-t-transparent"
				></div>
				<span>Saving...</span>
			</div>
		{/if}
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
		<div class="mb-6">
			<h4 class="text-md mb-2 font-medium">Default Model</h4>
			<p class="text-muted mb-3 text-sm">
				Select the default model for this project. This will be used when no specific model is
				specified.
			</p>

			{#if !Object.keys(selectedModels).length}
				<div class="mb-2 text-sm text-yellow-500">
					Configure at least one provider and select models to set a default model.
				</div>
			{:else}
				<div>
					<select
						class="w-full rounded-md border p-2 text-sm"
						value={defaultModelProvider && defaultModel
							? `${defaultModel}|||${defaultModelProvider}`
							: ''}
						onchange={(e) => updateDefaultModel(e.currentTarget.value)}
					>
						<option value="">Select a default model</option>
						{#each Object.entries(selectedModels) as [providerId, models]}
							{#each models as model}
								{#if providerId}
									{@const provider = modelProviders.find((p) => p.id === providerId)}
									{#if provider}
										<option value={`${model}|||${providerId}`}>
											{model} ({provider.name})
										</option>
									{/if}
								{/if}
							{/each}
						{/each}
					</select>
				</div>
			{/if}
		</div>

		<!-- Available Model Providers -->
		<h4 class="text-md mb-2 font-medium">Available Model Providers</h4>
		<div class="grid grid-cols-1 gap-4 md:grid-cols-2">
			{#each modelProviders as provider}
				<div class="flex flex-col gap-2 rounded-md border p-4">
					<div class="flex items-center gap-2">
						{#if provider.icon || provider.iconDark}
							<img
								src={darkMode.isDark && provider.iconDark ? provider.iconDark : provider.icon}
								alt={provider.name}
								class="h-6 w-6 {darkMode.isDark && !provider.iconDark ? 'dark:invert' : ''}"
							/>
						{/if}
						<h4 class="font-medium">{provider.name}</h4>
					</div>

					{#if provider.description}
						<div class="markdown-content text-muted mb-2 text-sm">
							{@html toHTMLFromMarkdown(provider.description)}
						</div>
					{/if}

					<div class="mt-1 text-sm">
						{#if provider.configured}
							<span class="flex items-center gap-1 text-green-500">
								<CheckCircleIcon class="h-4 w-4" />
								Configured
							</span>
						{:else}
							<span class="flex items-center gap-1 text-yellow-500">
								<CircleIcon class="h-4 w-4" />
								Not Configured
							</span>
						{/if}
					</div>

					{#if provider.configured && !configuringProvider}
						<!-- Models Selection Section -->
						<div class="mt-2">
							<h5 class="mb-2 text-sm font-medium">Available Models</h5>

							{#if loadingModels[provider.id]}
								<div class="flex justify-center p-2">
									<Loader2 class="size-6 animate-spin" />
								</div>
							{:else if availableModels[provider.id]?.length > 0}
								<div class="max-h-48 overflow-y-auto rounded-md border p-2">
									{#each availableModels[provider.id] as model}
										<div class="hover:bg-surface1 flex items-center rounded px-1 py-1.5">
											<input
												type="checkbox"
												id={`model-${provider.id}-${model}`}
												checked={(selectedModels[provider.id] || []).includes(model)}
												onchange={() => toggleModelSelection(provider.id, model)}
												class="mr-2 h-4 w-4"
											/>
											<label
												for={`model-${provider.id}-${model}`}
												class="flex-1 cursor-pointer text-sm select-none"
											>
												{model}
												{#if defaultModelProvider === provider.id && defaultModel === model}
													<span class="text-primary ml-2 text-xs font-medium">(Default Model)</span>
												{/if}
											</label>
										</div>
									{/each}
								</div>
							{:else}
								<p class="text-muted text-sm">No models available for this provider.</p>
							{/if}
						</div>
					{/if}

					<div class="mt-auto pt-2">
						{#if configuringProvider === provider.id}
							<!-- Configuration Form -->
							{#if configIsLoading}
								<div class="flex justify-center p-4">
									<Loader2 class="h-4 w-4 animate-spin" />
								</div>
							{:else}
								<form onsubmit={(e) => handleFormSubmit(provider, e)}>
									{#each provider.requiredConfigurationParameters || [] as param}
										<div class="mb-3">
											<label class="mb-1 block text-sm font-medium" for={param.name}>
												{param.friendlyName || param.name}
												{#if param.description}
													<span class="text-muted text-xs">({param.description})</span>
												{/if}
											</label>
											<input
												type={param.sensitive ? 'password' : 'text'}
												id={param.name}
												class="w-full rounded-md border p-2 text-sm"
												value={configFormData[param.name] || ''}
												oninput={(e) => updateFormValue(param.name, e.currentTarget.value)}
												required
											/>
										</div>
									{/each}

									<div class="mt-4 flex gap-2">
										<button
											type="submit"
											class="bg-primary text-primary-foreground flex-1 rounded-md px-4 py-2 text-sm font-medium"
										>
											Save
										</button>
										<button
											type="button"
											class="border-input flex-1 rounded-md border bg-transparent px-4 py-2 text-sm font-medium"
											onclick={() => (configuringProvider = null)}
										>
											Cancel
										</button>
									</div>
								</form>
							{/if}
						{:else}
							<button
								class="w-full px-4 py-2 {provider.configured
									? 'bg-secondary text-secondary-foreground'
									: 'bg-primary text-primary-foreground'} rounded-md text-sm font-medium"
								onclick={() => (configuringProvider = provider.id)}
							>
								{provider.configured ? 'Modify' : 'Configure'}
							</button>

							{#if provider.configured}
								<button
									class="border-input mt-2 w-full rounded-md border bg-transparent px-4 py-2 text-sm font-medium"
									onclick={() => {
										if (confirm(`Are you sure you want to deconfigure ${provider.name}?`)) {
											handleDeconfigureModelProvider(provider);
										}
									}}
								>
									Deconfigure
								</button>
							{/if}
						{/if}
					</div>
				</div>
			{/each}
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
