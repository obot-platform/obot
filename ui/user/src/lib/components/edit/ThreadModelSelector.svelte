<script lang="ts">
	import type { Project } from '$lib/services';
	import { ModelUsage } from '$lib/services/admin/types';
	import {
		getThread,
		updateThread,
		getDefaultModelForThread,
		listGlobalModelProviders,
		listModels
	} from '$lib/services/chat/operations';
	import type { ModelProvider, Model, Thread as ThreadType } from '$lib/services/chat/types';
	import { darkMode } from '$lib/stores';
	import Logo from '../Logo.svelte';
	import { ChevronDown } from 'lucide-svelte';
	import { onMount, tick, untrack } from 'svelte';
	import { SvelteMap } from 'svelte/reactivity';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		threadId: string | undefined;
		project: Project;
		projectDefaultModelProvider?: string;
		projectDefaultModel?: string;
		onModelChanged?: () => void;
		onCreateThread?: (model?: string, modelProvider?: string) => Promise<void> | void;
		hasModelSelected?: boolean;
	}

	let {
		threadId,
		project,
		projectDefaultModel,
		projectDefaultModelProvider,
		onModelChanged,
		onCreateThread,
		hasModelSelected = $bindable(false)
	}: Props = $props();

	let showModelSelector = $state(false);
	let threadType = $state<ThreadType | null>(null);
	let isUpdatingModel = $state(false);
	let modelSelectorRef = $state<HTMLDivElement>();
	let modelButtonRef = $state<HTMLButtonElement>();

	// Available models fetched from API, filtered by active and usage
	let availableModels = $state<Model[]>([]);
	let isLoadingModels = $state(true);
	let modelsError = $state<string>();

	let threadDefaultModel = $state<string>();
	let threadDefaultModelProvider = $state<string>();
	let hasAttemptedDefaultModelFetch = $state(false);

	let defaultModel = $derived(threadDefaultModel ?? projectDefaultModel);
	let defaultModelProvider = $derived(threadDefaultModelProvider ?? projectDefaultModelProvider);

	let modelProvidersMap = new SvelteMap<string, ModelProvider>();
	let modelsMap = new SvelteMap<string, Model>();

	// Calculate fallback model when default model is empty
	let fallbackModel = $derived.by(() => {
		// Only use fallback if default model is empty/missing
		if (defaultModel) return undefined;

		// If current thread model is not in available models, we rely on fetchDefaultModel
		// which will be called by an effect
		if (
			threadType?.model &&
			!availableModels.find((m) => m.id === threadType?.model || m.name === threadType?.model)
		) {
			return undefined;
		}

		// Return first available LLM model as fallback
		if (availableModels.length > 0) {
			const firstModel = availableModels[0];
			return { id: firstModel.id, provider: firstModel.modelProvider };
		}

		return undefined;
	});

	// Selected model provider & model for the current thread
	// Auto-fallback to accessible model if thread's model is inaccessible
	let threadModel = $derived.by(() => {
		let modelObj;
		if (threadType?.model) {
			modelObj = modelObj = availableModels.find(
				(m) => m.id === threadType?.model || m.name === threadType?.model
			);
		}

		const fallbackModelRef = threadDefaultModel || defaultModel || fallbackModel?.id;
		if (fallbackModelRef) {
			modelObj = availableModels.find(
				(m) => m.id === fallbackModelRef || m.name === fallbackModelRef
			);
		}

		return modelObj?.id;
	});
	let threadModelProvider = $derived(
		threadModel ? availableModels.find((m) => m.id === threadModel)?.modelProvider : undefined
	);

	const isDefaultModelSelected = $derived(
		defaultModelProvider &&
			defaultModel &&
			defaultModelProvider === threadModelProvider &&
			defaultModel === threadModel
	);

	// Detect when we're using a fallback model (thread's original model is inaccessible)
	const isUsingFallback = $derived(
		// Case 1: Thread has explicit model that's inaccessible
		((threadType?.model &&
			!availableModels.some((m) => m.id === threadType?.model || m.name === threadType?.model)) ||
			// Case 2: Thread has no model (system default), default returned empty, we've fetched it
			(!threadType?.model && hasAttemptedDefaultModelFetch && !threadDefaultModel)) &&
			!!threadModel // And we have a fallback to use
	);

	$effect(() => {
		if (threadId) {
			hasAttemptedDefaultModelFetch = false;
			fetchThreadDetails();
		}
	});

	// Fetch thread default model if current model is not in available models
	$effect(() => {
		if (threadType && threadId && availableModels.length > 0) {
			const currentModelInAvailable = availableModels.find(
				(m) => m.id === threadType?.model || m.name === threadType?.model
			);

			// If thread has a model not in available list, fetch the default
			if (!currentModelInAvailable && !threadDefaultModel) {
				fetchDefaultModel();
			}
		}
	});

	// Update parent about model selection state
	$effect(() => {
		// Since threadModel now only contains accessible models, just check if it exists
		// Also disable during model updates to ensure PUT completes before message is sent
		hasModelSelected =
			!isLoadingModels && !isUpdatingModel && !!threadModel && availableModels.length > 0;
	});

	// Auto-update thread when using fallback model
	$effect(() => {
		if (
			isUsingFallback &&
			threadModel &&
			threadId &&
			threadType &&
			!isUpdatingModel &&
			hasAttemptedDefaultModelFetch &&
			!isLoadingModels
		) {
			// Find the actual model object to ensure we use the ID
			const modelObj = availableModels.find((m) => m.id === threadModel || m.name === threadModel);
			if (modelObj) {
				// Automatically update the thread to use the fallback model ID with empty provider
				setThreadModel(modelObj.id, '');
			}
		}
	});

	// Function to fetch thread details including model
	async function fetchThreadDetails() {
		if (!threadId) return;

		try {
			const thread = await getThread(project.assistantID, project.id, threadId);
			threadType = thread;

			// Fetch default model information
			fetchDefaultModel();
		} catch (err) {
			console.error('Error fetching thread details:', err);
		}
	}

	// Function to fetch default model for this thread
	async function fetchDefaultModel() {
		if (!threadId) return;

		try {
			const res = await getDefaultModelForThread(project.assistantID, project.id, threadId);

			threadDefaultModel = res.model;
			threadDefaultModelProvider = res.modelProvider;
		} catch (err) {
			console.error('Error fetching default model:', err);

			threadDefaultModel = undefined;
			threadDefaultModelProvider = undefined;
		} finally {
			hasAttemptedDefaultModelFetch = true;
		}
	}

	// Function to update thread model
	async function setThreadModel(model: string, provider: string) {
		if (!threadId || !threadType) {
			// User change model in chat view; Create a new thread with selected model and model provider
			const promise = onCreateThread?.(model, provider);

			// Check if returned type is a promise
			if (promise instanceof Promise) {
				// Wait for the promise
				await promise;
			}

			// Fetch newly created thread details
			await fetchThreadDetails();

			// Close dropdown
			showModelSelector = false;

			return;
		}

		// Prevent setting to empty if default model is empty
		if (!model && !provider && projectDefaultModel === '' && projectDefaultModelProvider === '') {
			return;
		}

		isUpdatingModel = true;

		try {
			let retryCount = 0;
			const maxTries = 5;

			while (retryCount < maxTries) {
				try {
					const updatedThread = await updateThread(
						project.assistantID,
						project.id,
						{
							...threadType,
							model: model,
							modelProvider: provider
						},
						{
							dontLogErrors: true
						}
					);

					// Update local state
					threadType = updatedThread;

					// If resetting to default, fetch the default model
					if (!model && !provider) {
						await fetchDefaultModel();
					} else {
						// Update thread default model state to reflect the explicit model selection
						threadDefaultModel = model || undefined;
						threadDefaultModelProvider = provider || undefined;
					}

					// Close dropdown
					showModelSelector = false;

					// Notify parent that model changed
					if (onModelChanged) {
						onModelChanged();
					}

					break;
				} catch (err) {
					if (err instanceof Error && err.message.includes('409')) {
						retryCount++;
						await fetchThreadDetails();
						await new Promise((resolve) => setTimeout(resolve, 100 * retryCount));
						continue;
					} else {
						throw err;
					}
				}
			}

			// If we've exhausted all retries, throw an error
			if (retryCount >= maxTries) {
				throw new Error('Failed to update thread model after multiple retries due to conflicts');
			}
		} catch (err) {
			console.error('Error updating thread model:', err);
		} finally {
			isUpdatingModel = false;
		}
	}

	onMount(() => {
		// Close model selector when clicking outside
		const handleClickOutside = (event: MouseEvent) => {
			if (
				showModelSelector &&
				modelSelectorRef &&
				modelButtonRef &&
				!modelSelectorRef.contains(event.target as Node) &&
				!modelButtonRef.contains(event.target as Node)
			) {
				showModelSelector = false;
			}
		};

		window.addEventListener('click', handleClickOutside);

		return () => {
			window.removeEventListener('click', handleClickOutside);
		};
	});

	$effect(() => {
		loadModelProviders();
		loadModels();
	});

	async function loadModels() {
		try {
			isLoadingModels = true;
			const allModels = await listModels();

			untrack(() => {
				// Filter models: active=true AND usage='llm'
				availableModels = (allModels ?? []).filter(
					(model) => model.active && model.usage === ModelUsage.LLM
				);

				// Also populate modelsMap for display purposes
				for (const model of allModels ?? []) {
					modelsMap.set(model.id, model);
				}
			});

			modelsError = undefined;
		} catch (error) {
			console.error('Failed to load models:', error);
			modelsError = 'Failed to load models';
			availableModels = [];
		} finally {
			isLoadingModels = false;
		}
	}

	// Function to fetch model providers
	async function loadModelProviders() {
		try {
			listGlobalModelProviders().then((res) => {
				untrack(() => {
					for (const provider of res.items ?? []) {
						modelProvidersMap.set(provider.id, provider);
					}
				});
			});
		} catch (error) {
			console.error('Failed to load model providers:', error);
		}
	}

	type ScrollIntoSelectedModelParams = {
		providerId?: string;
		modelId?: string;
	};

	// TODO: We are loading model providers in different location in the app
	// A better approach to load them once and share them, with the abbility to reload the results
	function scrollIntoSelectedModel(node: HTMLElement, params: ScrollIntoSelectedModelParams) {
		if (!params.modelId) return;
		if (!params.providerId) return;

		tick().then(() => {
			const modelElement = node.querySelector(
				`[data-provider="${params.providerId}"][data-model="${params.modelId}"]`
			);
			if (modelElement) {
				modelElement.scrollIntoView({ behavior: 'instant', block: 'center' });
			}
		});
	}
</script>

<!-- TODO: Refactor this to use a dropdown component either third-party or internally crafted -->
<div class="relative mr-2 md:mr-6 lg:mr-8">
	<button
		class={twMerge(
			'hover:bg-surface2/50 active:bg-surface2/80 flex h-10 items-center gap-3 rounded-full px-2  py-1 text-xs text-gray-600 md:px-4 lg:px-6',
			(isDefaultModelSelected || (!threadType?.model && defaultModel)) &&
				'text-primary hover:bg-primary/10 active:bg-primary/15 bg-transparent',
			isUsingFallback && 'text-orange-600'
		)}
		onclick={(e) => {
			e.stopPropagation();
			showModelSelector = !showModelSelector;
		}}
		onkeydown={(e) => e.key === 'Escape' && (showModelSelector = false)}
		aria-haspopup="listbox"
		aria-expanded={showModelSelector}
		id="thread-model-button"
		title={isDefaultModelSelected
			? 'Default model is selected'
			: isUsingFallback
				? 'Using fallback model. Your original model is no longer accessible.'
				: threadModel
					? ''
					: 'Select model for this chat'}
		bind:this={modelButtonRef}
	>
		<div class="max-w-40 truncate sm:max-w-60 md:max-w-96 lg:max-w-none">
			{#if isLoadingModels}
				Loading...
			{:else if threadModel}
				{@const modelInfo = modelsMap.get(threadModel)}
				{modelInfo?.name || threadModel}
			{:else}
				No Model Selected
			{/if}
		</div>

		<ChevronDown class="h-4 w-4" />
	</button>

	{#if showModelSelector}
		<div
			role="listbox"
			tabindex="-1"
			aria-labelledby="thread-model-button"
			class="available-models-popover default-scrollbar-thin border-surface1 dark:bg-surface2 bg-background absolute right-0 bottom-full z-10 mb-1 max-h-60 w-max max-w-sm overflow-hidden overflow-y-auto rounded-md border px-2 shadow-lg md:max-w-md lg:max-w-lg"
			onclick={(e) => e.stopPropagation()}
			onkeydown={(e) => {
				if (e.key === 'Escape') {
					showModelSelector = false;
					document.getElementById('thread-model-button')?.focus();
				}
			}}
			bind:this={modelSelectorRef}
			use:scrollIntoSelectedModel={{
				providerId: threadModelProvider,
				modelId: threadModel
			}}
		>
			{#if isLoadingModels}
				<div class="flex justify-center p-4">
					<div
						class="h-4 w-4 animate-spin rounded-full border-2 border-current border-t-transparent"
						aria-hidden="true"
					></div>
					<span class="sr-only">Loading models...</span>
				</div>
			{:else if modelsError}
				<div class="text-on-surface1 p-4 text-sm">
					{modelsError}
				</div>
			{:else if availableModels.length === 0}
				<div class="text-on-surface1 p-4 text-sm">No Model Available</div>
			{:else}
				<div class="flex flex-col">
					{#each (() => {
						// Group available models by provider
						const modelsByProvider = new Map<string, Model[]>();

						availableModels.forEach((model) => {
							const providerId = model.modelProvider;
							if (!modelsByProvider.has(providerId)) {
								modelsByProvider.set(providerId, []);
							}
							modelsByProvider.get(providerId)!.push(model);
						});

						return Array.from(modelsByProvider.entries());
					})() as [providerId, models] (providerId)}
						{#if models.length > 0}
							{@const provider = modelProvidersMap.get(providerId)}
							<div class="border-surface1 flex flex-col border-b py-2 last:border-transparent">
								<div class="mb-2 flex gap-1 text-xs">
									{#if provider?.icon || provider?.iconDark}
										<img
											src={darkMode.isDark && provider.iconDark ? provider.iconDark : provider.icon}
											alt={provider.name}
											class={twMerge(
												'size-4',
												darkMode.isDark && !provider.iconDark ? 'dark:invert' : ''
											)}
										/>
									{/if}
									<div>{provider?.name ?? ''}</div>
								</div>
								<div class="provider-models flex flex-col gap-1">
									{#each models as model (model.id)}
										{@const isModelSelected =
											threadModelProvider === providerId &&
											(threadModel === model.id || threadModel === model.name)}

										{@const isDefaultModel =
											defaultModelProvider === providerId &&
											(defaultModel === model.name || defaultModel === model.id)}

										<button
											role="option"
											aria-selected={isModelSelected}
											class={twMerge(
												'hover:bg-surface1/70 active:bg-surface1/80 focus:bg-surface1/70 flex w-full items-center gap-2 rounded px-2 py-1.5 text-left text-sm transition-colors duration-200 focus:outline-none',
												isModelSelected &&
													'text-primary bg-primary/10 hover:bg-primary/15 active:bg-primary/20'
											)}
											onclick={() => {
												if (isDefaultModel) {
													setThreadModel('', '');
												} else {
													setThreadModel(model.id, '');
												}
											}}
											tabindex="0"
											data-provider={providerId}
											data-model={model.id}
										>
											<div>{model.name || model.id}</div>

											{#if isDefaultModel}
												<Logo class={twMerge(' size-4', !isModelSelected && 'grayscale-100')} />
											{/if}

											{#if isModelSelected}
												<div class="text-primary ml-auto text-xs">✓</div>
											{/if}
										</button>
									{/each}
								</div>
							</div>
						{/if}
					{/each}
				</div>
			{/if}

			{#if isUpdatingModel}
				<div class="flex justify-center p-2">
					<div
						class="h-4 w-4 animate-spin rounded-full border-2 border-current border-t-transparent"
						aria-hidden="true"
					></div>
					<span class="sr-only">Loading...</span>
				</div>
			{/if}
		</div>
	{/if}
</div>

<style>
	.available-models-popover {
		display: grid;
		grid-template-columns: minmax(fit-content, auto);
	}
</style>
