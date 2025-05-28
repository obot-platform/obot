<script lang="ts" module>
	function sortGptModels(models: string[]) {
		const query = 'gpt';

		return [...models].sort((m1, m2) => {
			const m1LowerCase = m1.toLowerCase();
			const isM1StartWithQuery = m1LowerCase.startsWith(query);

			// m1 does not starts with query move it out
			if (!isM1StartWithQuery) return 1;

			const m2LowerCase = m2.toLowerCase();
			const isM2StartsWithQuery = m2LowerCase.startsWith(query);

			// Both starts with query; local compare
			if (isM1StartWithQuery && isM2StartsWithQuery) {
				return m1LowerCase.localeCompare(m2LowerCase);
			}

			// move m1 in
			return -1;
		});
	}
</script>

<script lang="ts">
	import type { ModelProvider, Project } from '$lib/services';
	import { AlertCircle, CheckCircleIcon, Loader2, Search } from 'lucide-svelte';
	import { darkMode } from '$lib/stores';
	import {
		updateProject,
		listAvailableModels,
		configureModelProvider,
		deconfigureModelProvider,
		getModelProviderConfig,

		validateModelProviderConfig

	} from '$lib/services/chat/operations';
	import { twMerge } from 'tailwind-merge';
	import { fade, slide } from 'svelte/transition';
	import { delay, throttle } from 'es-toolkit';
	import { untrack } from 'svelte';
	import Confirm from './Confirm.svelte';

	type Props = {
		provider: ModelProvider;
		project: Project;
		configuringProvider?: string | null;
		onError?: (error: string) => void;
	};

	let {
		provider,
		project = $bindable(),
		configuringProvider = $bindable(),
		onError
	}: Props = $props();

	let models: string[] = $state([]);

	let selectedModels: string[] = $derived(project.models?.[provider.id] ?? []);
	let error: string | null = $state(null);

	let defaultModelProvider = $derived(project.defaultModelProvider || '');
	let defaultModel = $derived(project.defaultModel || '');

	let isConfigured = $derived(provider?.configured ?? false);
	let providerId = $derived(provider.id);
	// let selectedModels = $derived<Record<string, string[]>>(project.models || {});

	let configuration: Record<string, string> = $state({});
	let oldConfiguration: Record<string, string> = $state(structuredClone({}));

	let isConfigurationLoading = $state(false);
	let isModelsLoading = $state(false);
	let isSaving = $state(false);
	let isProviderConfigurationShown = $state(false);
	let isUnconfigureProviderDialogShown = $state(false);
	let validationError: string | null = $state(null);

	let modelQuery: string = $state('');
	let filteredModels: string[] = $state([]);

	// Throttle search model for better performance
	const searchModels = throttle((query: string, models: string[] = []) => {
		// Filter out items and keep those include the query text
		// Sort results where items that starts with the query text goes first
		filteredModels = [
			...models.filter((model) => model.trim().toLowerCase().includes(query.trim().toLowerCase()))
		].sort((m1, m2) => {
			const m1LowerCase = m1.toLowerCase();
			const isM1StartWithQuery = m1LowerCase.startsWith(query);

			// m1 does not starts with query move it out
			if (!isM1StartWithQuery) return 1;

			const m2LowerCase = m2.toLowerCase();
			const isM2StartsWithQuery = m2LowerCase.startsWith(query);

			// Both starts with query; local compare
			if (isM1StartWithQuery && isM2StartsWithQuery) {
				return m1LowerCase.localeCompare(m2LowerCase);
			}

			// move m1 in
			return -1;
		});
	}, 100);

	$effect(() => {
		if (providerId.startsWith('openai-model-provider') && !modelQuery) {
			filteredModels = sortGptModels(models);
		} else {
			searchModels(modelQuery, models);
		}
	});

	// When a provider is selected for configuration, load its config
	$effect(() => {
		if (isConfigured) {
			untrack(async () => {
				try {
					await loadProviderConfig(providerId);

					await delay(1000);

					models = await loadAvailableModels(providerId);
				} catch (err) {
					console.error(err);
				}
			});
		} else {
			models = [];
		}
	});

	// Deconfigure model provider
	async function handleDeconfigureModelProvider(provider: ModelProvider) {
		try {
			// Unselect all model first
			await unselectModels(selectedModels);
			// Unconfigure model provider
			await deconfigureModelProvider(project.assistantID, project.id, provider.id);

			models = [];
			provider.configured = false;
			delete project.models?.[provider.id];

			// clear current configurations
			configuration = {};
			// clear previous configurations
			oldConfiguration = {};
		} catch (err) {
			console.error(`Failed to deconfigure ${provider.name}`, err);
		}
	}

	type UpdateProjectParams = {
		models: Project['models'];
		defaultModel?: Project['defaultModel'];
		defaultModelProvider?: Project['defaultModelProvider'];
	};
	// Update the project's model selection directly and trigger auto-save
	function updateProject_(params: UpdateProjectParams) {
		project = {
			...project,
			...params
		};

		saveChanges();
	}

	// Load configuration for a model provider
	async function loadProviderConfig(providerId: string) {
		try {
			configuration = await getModelProviderConfig(project.assistantID, project.id, providerId);
			oldConfiguration = structuredClone($state.snapshot(configuration));
		} catch (err) {
			console.error(`Failed to get configuration for provider ${providerId}`, err);
		} finally {
			isConfigurationLoading = false;
		}
	}

	// Configure model provider
	async function handleConfigureModelProvider(
		provider: ModelProvider,
		config: Record<string, string>
	): Promise<boolean> {
		try {
			try {
				await validateModelProviderConfig(project.assistantID, project.id, provider.id, config);
				validationError = null; // Clear any previous validation errors
			} catch (err) {
				console.error(err);

				let message = err instanceof Error ? err.message : 'Validation failed. Please check your configuration.';
				const parts = message.split(' logger=')[0].split('ERROR');
				message = parts[parts.length - 1].trim() || 'Validation failed. Please check your configuration.';

				// If there is an error JSON string in there, extract it and use that as the message instead.
				const errorMessageMatch =
					err instanceof Error && err.message.match(/{"error":\s*"(.*?)"}/);
				if (errorMessageMatch) {
					message = JSON.parse(errorMessageMatch[0]).error;
				}
				validationError = message;
				return false;
			}

			await configureModelProvider(project.assistantID, project.id, provider.id, config);
			const newProject = setProjectModels(project, providerId, []);

			await updateProject(newProject);

			project = newProject;
		} catch (err) {
			onError?.((error = `Failed to configure ${provider.name}`));

			console.error(error, err);
			return false;
		}
		return true;
	}

	// Load available models for a provider
	async function loadAvailableModels(providerId: string) {
		isModelsLoading = true;

		try {
			const response = await listAvailableModels(project.assistantID, project.id, providerId);

			return (response.data || [])
				.filter((m) => m.metadata && m.metadata.usage === 'llm')
				.map((m) => m.id)
				.sort((a, b) => a.localeCompare(b));
		} catch (err) {
			console.error(`Failed to load models for provider ${providerId}`, err);
			return [];
		} finally {
			await delay(500);
			isModelsLoading = false;
		}
	}

	async function saveHandler(ev: Event) {
		ev.preventDefault();

		try {
			isModelsLoading = true;
			if (!await handleConfigureModelProvider(provider, configuration)) {
				isModelsLoading = false;
				return;
			}

			provider.configured = true;

			await delay(400);

			const array = await loadAvailableModels(provider.id);

			isProviderConfigurationShown = false;

			await delay(300);

			models = array;
		} catch (err) {
			console.error(err);
			provider.configured = false;
		} finally {
			isModelsLoading = false;
		}
	}

	// Toggle model selection
	function toggleModel(model: string, isChecked: boolean) {
		// Take a snapshot of current models
		let projectModels = { ...(project.models || {}) };
		const currentProviderModels = new Set([...(projectModels[provider.id] || [])]);

		if (isChecked) {
			currentProviderModels.add(model);
		} else {
			currentProviderModels.delete(model);
		}

		let p = project;

		if (!isChecked && defaultModelProvider === provider.id) {
			p = setProjectDefaultProvider(p);
		}

		if (!isChecked && defaultModel === model) {
			p = setProjectDefaultModel(p, model);
		}

		p = setProjectModels(p, provider.id, currentProviderModels.values().toArray());

		project = p;
	}

	function selectModels(models: string[]) {
		// Take a snapshot of current models
		let projectModels = { ...(project.models || {}) };

		projectModels = {
			...projectModels,
			[provider.id]: models
		};

		return updateProject_({ models: projectModels });
	}

	function unselectModels(models: string[]) {
		// Take a snapshot of current models
		let projectModels = { ...(project.models || {}) };

		const array = new Set(projectModels[providerId] ?? []);

		for (const model of models) {
			array.delete(model);
		}

		projectModels = {
			...projectModels,
			[provider.id]: array.values().toArray()
		};

		return updateProject_({ models: projectModels });
	}

	function setProjectDefaultProvider(project: Project, providerId?: string) {
		const clone = structuredClone($state.snapshot(project));

		if (provider) {
			clone.defaultModelProvider = providerId;
		} else {
			delete clone.defaultModelProvider;
		}

		return clone;
	}

	function setProjectDefaultModel(project: Project, modelId: string) {
		const clone = structuredClone($state.snapshot(project));

		if (provider) {
			clone.defaultModel = modelId;
		} else {
			delete clone.defaultModel;
		}

		return clone;
	}

	function setProjectModels(project: Project, providerId: string, models: string[]) {
		const clone = structuredClone($state.snapshot(project));

		if (!clone.models) {
			clone.models = {};
		}

		if (!models) {
			delete clone.models[providerId];
		} else {
			clone.models[providerId] = models;
		}

		return clone;
	}

	// Save changes to the server
	async function saveChanges() {
		try {
			isSaving = true;
			// isSaving = true;
			// Update the project prop directly
			project = await updateProject(project);
		} catch (err) {
			console.error('Failed to save model configuration', err);
			error = 'Failed to save changes. Please try again.';
		} finally {
			await delay(500);
			isSaving = false;
		}
	}

	function onConfigureProviderClickHandler() {
		configuringProvider = provider.id;
		isProviderConfigurationShown = true;
		validationError = null; // Clear any previous validation errors when opening config
	}
</script>

<div
	class="model-provider-card border-surface2 flex max-h-[514px] min-h-fit w-full flex-col rounded-md border py-4 shadow-sm 2xl:mb-4 2xl:min-h-[514px] 2xl:last:mb-0"
	data-provider-id={provider.id}
>
	<div class="flex flex-col px-4">
		<div class="mb-2 flex items-center gap-2">
			{#if provider.icon || provider.iconDark}
				<img
					src={darkMode.isDark && provider.iconDark ? provider.iconDark : provider.icon}
					alt={provider.name}
					class="h-6 w-6 {darkMode.isDark && !provider.iconDark ? 'dark:invert' : ''}"
				/>
			{/if}
			<div class="flex items-center gap-4">
				<h4 class="truncate text-lg font-medium">{provider.name}</h4>
				{#if [isModelsLoading, isSaving, isConfigurationLoading].some(Boolean)}
					<div
						class="flex justify-center"
						in:fade={{ duration: 1000 }}
						out:fade={{ duration: 600 }}
					>
						<Loader2 class="size-5 animate-spin" />
					</div>
				{/if}
			</div>

			{#if provider.configured}
				<CheckCircleIcon class="text-blue ml-auto aspect-square h-5" />
			{/if}
		</div>
	</div>

	<div class="flex flex-1 flex-col gap-4 px-4 pt-4 pb-4">
		{#if isConfigured}
			<!-- Models Selection Section -->
			<div
				class={twMerge(
					'bg-surface1/0 flex flex-1 flex-col gap-2 rounded-md',
					isProviderConfigurationShown && 'pointer-events-none opacity-50'
				)}
				in:fade={{ duration: 200 }}
				out:fade={{ duration: 0 }}
			>
				<div
					class="model-provider-search-input bg-surface1 dark:bg-surface2 relative flex h-10 w-full items-center gap-2 rounded-md text-sm"
				>
					<div class="absolute inset-x-0 left-2 aspect-square h-5 opacity-50">
						<Search class="h-full" />
					</div>
					<input
						bind:value={modelQuery}
						class="h-full w-full bg-transparent px-3 pr-8 pl-9"
						type="text"
						placeholder="Search your model here..."
					/>

					{#if filteredModels.length !== models.length}
						<div
							class="absolute inset-y-0 right-4 flex h-full items-center justify-center text-sm font-medium opacity-50"
							transition:fade={{ duration: 100 }}
						>
							<div>{filteredModels.length}</div>
						</div>
					{/if}
				</div>

				<div class="border-surface1 flex flex-1 flex-col gap-2 rounded-md border">
					<div class="flex items-center gap-2 px-4 py-2 text-sm font-medium">
						<h5 class="flex items-center">
							<input
								class="bg-surface3 mr-2 h-4 w-4"
								type="checkbox"
								id={`model-${provider.id}-toggle-all`}
								bind:checked={
									() => models.length > 0 && models.length === selectedModels.length,
									(v) => {
										const indeterminate =
											selectedModels.length > 0 && models.length > selectedModels.length;

										if (indeterminate || v) {
											selectModels(models);
										} else {
											unselectModels(selectedModels);
										}
									}
								}
								indeterminate={selectedModels.length > 0 && models.length > selectedModels.length}
							/>
							<label class="inline" for={`model-${provider.id}-toggle-all`}>Available Models</label>

							{#if models.length}
								<span class="opacity-50">({models.length})</span>
							{/if}
						</h5>

						{#if selectedModels.length}
							<div class="h-full border-l"></div>
							<button class="inline font-normal">
								<span class="">Selected</span>
								<span class="opacity-50">({selectedModels.length})</span>
							</button>
						{/if}
					</div>

					<div class="relative flex min-h-[180px] flex-1 flex-col">
						<div
							class="default-scrollbar-thin scrollbar-track-rounded-full absolute inset-0 flex h-full max-h-full flex-col overflow-y-auto px-2"
						>
							{#each filteredModels as model (model)}
								<div class="hover:bg-surface1 bored flex items-center rounded px-2 py-2">
									<input
										class="bg-surface3 mr-2 h-4 w-4"
										type="checkbox"
										id={`model-${provider.id}-${model}`}
										bind:checked={
											() => (selectedModels ?? []).includes(model),
											(checked) => toggleModel(model, checked)
										}
									/>

									<label
										for={`model-${provider.id}-${model}`}
										class="flex-1 cursor-pointer truncate text-sm select-none"
									>
										{model}
									</label>
								</div>
							{:else}
								<div
									class="w-full h-full flex items-center justify-center text-gray-400 text-lg font-semibold rounded-lg absolute inset-0 p-8"
									transition:fade={{ duration: 100 }}
								>
									{#if isModelsLoading}
										<p>Loading models...</p>
									{:else}
										<p>No model is available</p>
									{/if}
								</div>
							{/each}
						</div>
					</div>
				</div>
			</div>
		{:else}
			<!-- TODO: this is an example placeholder, feel free to change if you have a better one -->
			<div
				class="bg-surface1 flex flex-1 flex-col items-center justify-center rounded-lg p-8"
				in:fade={{ duration: 200 }}
				out:fade={{ duration: 0 }}
			>
				<div class="mb-2 text-lg font-semibold text-gray-400">Provider is not yet configured</div>
				<p class="text-center text-xs opacity-50">
					Click on the "Configure" button below to set up this provider. We'll then validate your
					configuration and display available models.
				</p>
			</div>
		{/if}

		{#if isProviderConfigurationShown}
			<div transition:slide={{ duration: 200 }} class="flex flex-col gap-2">
				<div class="flex items-center gap-2">
					<div class="flex items-center font-medium">
						<span>Configurations</span>
					</div>
					<div class="border-surface2 flex-1 border-b"></div>
				</div>

				<div class="flex flex-col gap-4">
					{#each provider.requiredConfigurationParameters || [] as param}
						<div class="flex w-full flex-col gap-1">
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
								bind:value={configuration[param.name]}
								oninput={() => {
									// Clear validation error when user starts typing
									if (validationError) {
										validationError = null;
									}
								}}
								required
							/>
						</div>
					{/each}

					{#if validationError}
						<div 
							class="bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 text-red-700 dark:text-red-300 px-4 py-3 rounded-md text-sm"
							transition:slide={{ duration: 200 }}
						>
							<div class="flex items-center gap-2">
								<AlertCircle class="size-4" />
								<span>{validationError}</span>
							</div>
						</div>
					{/if}
				</div>
			</div>
		{/if}
	</div>

	<div class="border-surface2 flex min-h-14 w-full justify-between gap-4 border-t px-4 pt-4">
		{#if isProviderConfigurationShown}
			{@const isDirty =
				JSON.stringify($state.snapshot(configuration)) !==
				JSON.stringify($state.snapshot(oldConfiguration))}

			<div class="flex gap-2">
				{#if isDirty}
					<button
						in:fade={{ duration: 100 }}
						out:fade={{ duration: 0 }}
						type="button"
						class="button hover:bg-surface1 rounded-full px-4 py-2 text-sm transition-colors duration-100"
						onclick={() => {
							configuration = $state.snapshot(oldConfiguration);
						}}
					>
						Reset
					</button>
				{:else}
					<button
						in:fade={{ duration: 100 }}
						out:fade={{ duration: 0 }}
						type="button"
						class={twMerge(
							'button hover:bg-surface1 rounded-full px-4 py-2 text-sm transition-colors duration-100'
						)}
						onclick={() => {
							// Close configuration UI
							isProviderConfigurationShown = false;
						}}
					>
						Cancel
					</button>
				{/if}

				{#if isDirty}
					<button
						in:fade={{ duration: 100 }}
						out:fade={{ duration: 0 }}
						type="button"
						class="button bg-blue/10 text-blue hover:bg-blue/15 active:bg-blue/20 rounded-full border-none px-4 py-2 text-sm transition-colors duration-100"
						disabled={!isDirty}
						onclick={(ev) => saveHandler(ev)}
					>
						Save
					</button>
				{/if}
			</div>
		{/if}

		<div class="ml-auto flex gap-2">
			{#if !isProviderConfigurationShown}
				<button
					class="button hover:bg-surface3/80 active:bg-surface3/100 bg-transparent text-sm font-medium"
					onclick={onConfigureProviderClickHandler}
				>
					{provider.configured ? 'Reconfigure' : 'Configure'}
				</button>
			{/if}

			{#if provider.configured}
				<button
					class="button bg-red-500/5 text-sm font-medium text-red-500 transition-colors duration-100 hover:bg-red-500/10 active:bg-red-500/15"
					onclick={() => {
						isUnconfigureProviderDialogShown = true;
					}}
				>
					Unconfigure
				</button>
			{/if}
		</div>
	</div>
</div>

<Confirm
	show={isUnconfigureProviderDialogShown}
	msg={`Are you sure you want to unconfigure ${provider.name} models provider?`}
	onsuccess={async () => {
		await handleDeconfigureModelProvider(provider);
		isUnconfigureProviderDialogShown = false;
	}}
	oncancel={() => (isUnconfigureProviderDialogShown = false)}
/>

<style>
	.model-provider-card {
		break-inside: avoid;
	}
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
