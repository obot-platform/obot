<script lang="ts">
	import { untrack } from 'svelte';
	import { ChevronDown } from 'lucide-svelte/icons';
	import type { Task, ModelProvider, Model } from '$lib/services/chat/types';
	import { listGlobalModelProviders, listModels } from '$lib/services/chat/operations';
	import { twMerge } from 'tailwind-merge';
	import { SvelteMap } from 'svelte/reactivity';
	import { darkMode } from '$lib/stores';
	import { ModelUsage } from '$lib/services/admin/types';
	import { popover } from '$lib/actions';

	interface Props {
		task: Task;
		readOnly?: boolean;
	}

	let { task = $bindable(), readOnly = false }: Props = $props();

	const { ref, tooltip, toggle } = popover({
		placement: 'bottom-start'
	});

	let availableModels = $state<Model[]>([]);
	let isLoadingModels = $state(true);
	let modelsError = $state<string>();

	let modelProvidersMap = new SvelteMap<string, ModelProvider>();
	let modelsMap = new SvelteMap<string, Model>();

	// Get current selected model info
	let selectedModel = $derived(
		task.model ? availableModels.find((m) => m.id === task.model || m.name === task.model) : null
	);

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

	function selectModel(model: Model | null) {
		if (model) {
			task.model = model.id;
			// Leave modelProvider empty - system resolves it from model ID
			task.modelProvider = '';
		} else {
			// Clear to use project default
			task.model = undefined;
			task.modelProvider = undefined;
		}
		toggle(false);
	}

	// Group models by provider
	let modelsByProvider = $derived.by(() => {
		const grouped: Record<string, Model[]> = {};
		availableModels.forEach((model) => {
			const providerId = model.modelProvider;
			if (!grouped[providerId]) {
				grouped[providerId] = [];
			}
			grouped[providerId].push(model);
		});
		return Object.entries(grouped);
	});
</script>

<div class="flex flex-col gap-2">
	<label class="text-sm font-medium">Model</label>
	<p class="text-gray text-sm">
		Select a model for this task, or leave empty to use project default.
	</p>

	{#if readOnly}
		<span
			class="text-gray bg-surface2 flex items-center justify-between gap-2 rounded-3xl p-3 px-4"
		>
			{#if isLoadingModels}
				Loading...
			{:else if selectedModel}
				{selectedModel.name || selectedModel.id}
			{:else}
				Use project default
			{/if}
			<ChevronDown class="text-gray" />
		</span>
	{:else}
		<button
			use:ref
			type="button"
			onclick={() => toggle()}
			class="bg-surface2 hover:bg-surface2/80 flex items-center justify-between gap-2 rounded-3xl p-3 px-4"
		>
			{#if isLoadingModels}
				Loading...
			{:else if selectedModel}
				{selectedModel.name || selectedModel.id}
			{:else}
				Use project default
			{/if}
			<ChevronDown />
		</button>

		<div
			use:tooltip
			class="bg-background default-scrollbar-thin max-h-60 min-w-[200px] overflow-y-auto rounded-xl shadow dark:bg-gray-900"
		>
			{#if isLoadingModels}
				<div class="flex justify-center p-4">
					<div
						class="h-4 w-4 animate-spin rounded-full border-2 border-current border-t-transparent"
						aria-hidden="true"
					></div>
				</div>
			{:else if modelsError}
				<div class="text-on-surface1 p-4 text-sm">
					{modelsError}
				</div>
			{:else}
				<ul>
					<!-- Project default option -->
					<li>
						<button
							class={twMerge(
								'w-full px-6 py-2.5 text-start hover:bg-gray-100 dark:hover:bg-gray-800',
								!task.model && 'bg-gray-70 dark:bg-gray-800'
							)}
							onclick={() => selectModel(null)}
						>
							Use project default
						</button>
					</li>

					<!-- Grouped models by provider -->
					{#each modelsByProvider as [providerId, models] (providerId)}
						{@const provider = modelProvidersMap.get(providerId)}
						<li class="border-surface1 border-t px-4 py-2">
							<div class="mb-1 flex items-center gap-1 text-xs text-gray-500">
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
								<span>{provider?.name ?? providerId}</span>
							</div>
						</li>
						{#each models as model (model.id)}
							<li>
								<button
									class={twMerge(
										'w-full px-6 py-2 text-start text-sm hover:bg-gray-100 dark:hover:bg-gray-800',
										task.model === model.id && 'bg-gray-70 dark:bg-gray-800'
									)}
									onclick={() => selectModel(model)}
								>
									{model.name || model.id}
									{#if task.model === model.id}
										<span class="text-primary ml-2">✓</span>
									{/if}
								</button>
							</li>
						{/each}
					{/each}
				</ul>
			{/if}
		</div>
	{/if}
</div>
