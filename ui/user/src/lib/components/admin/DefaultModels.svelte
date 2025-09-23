<script lang="ts">
	import {
		ModelAliasLabels,
		ModelUsage,
		ModelAlias,
		type Model,
		type DefaultModelAlias,
		ModelAliasToUsageMap
	} from '$lib/services';
	import { onMount } from 'svelte';
	import ResponsiveDialog from '../ResponsiveDialog.svelte';
	import { AdminService } from '$lib/services';
	import Select from '../Select.svelte';
	import { LoaderCircle } from 'lucide-svelte';

	let { availableModels, readonly }: { availableModels: Model[]; readonly?: boolean } = $props();
	let dialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let defaultModelAliases = $state<DefaultModelAlias[]>([]);
	let sortedModelAliases = $derived(
		Object.values(ModelAlias)
			.map((alias) => defaultModelAliases.find((defaultAlias) => defaultAlias.alias === alias))
			.filter((x) => !!x)
	);
	let changes = $state<Partial<Record<ModelAlias, string>>>();
	let changed = $derived(
		defaultModelAliases.length > 0 &&
			defaultModelAliases.some((modelAlias) => {
				const currentSelection = changes?.[modelAlias.alias] ?? modelAlias.model;
				return currentSelection && currentSelection !== modelAlias.model;
			})
	);
	let loading = $state(false);
	let showSkip = $state(false);

	const SUGGESTED_MODEL_SELECTIONS: Record<ModelAlias, string> = {
		[ModelAlias.Llm]: 'gpt-4.1',
		[ModelAlias.LlmMini]: 'gpt-4.1-mini',
		[ModelAlias.TextEmbedding]: 'text-embedding-3-large',
		[ModelAlias.ImageGeneration]: 'dall-e-3',
		[ModelAlias.Vision]: 'gpt-4.1'
	};

	export function open(updateShowSkip = false) {
		setSuggestedModels();
		showSkip = updateShowSkip;
		dialog?.open();
	}

	onMount(async () => {
		defaultModelAliases = await AdminService.listDefaultModelAliases();
	});

	function setSuggestedModels() {
		if (!defaultModelAliases.length || !availableModels.length) return;

		const suggestedChanges: Partial<Record<ModelAlias, string>> = {};
		for (const modelAlias of defaultModelAliases) {
			// Only suggest if no model is currently set
			if (modelAlias.model) {
				continue;
			}

			const usage = getModelUsageFromAlias(modelAlias.alias);
			if (usage) {
				const activeModelOptions = filterModelsByActive(
					filterModelsByUsage(availableModels, usage)
				);
				const suggestedModelName = SUGGESTED_MODEL_SELECTIONS[modelAlias.alias];

				if (suggestedModelName) {
					const suggestedModel = activeModelOptions.find(
						(model) => model.name === suggestedModelName
					);
					if (suggestedModel) {
						suggestedChanges[modelAlias.alias] = suggestedModel.id;
					}
				}
			}
		}

		if (Object.keys(suggestedChanges).length > 0) {
			changes = { ...changes, ...suggestedChanges };
		}
	}

	function getModelUsageFromAlias(alias: string) {
		if (!(alias in ModelAliasToUsageMap)) return null;

		return ModelAliasToUsageMap[alias as keyof typeof ModelAliasToUsageMap];
	}

	function getModelAliasLabel(alias: string) {
		if (!(alias in ModelAliasLabels)) return alias;

		return ModelAliasLabels[alias as ModelAlias];
	}

	function filterModelsByActive(models: Model[]) {
		return models.filter((model) => model.active);
	}

	function filterModelsByUsage(
		models: Model[],
		usages: ModelUsage | ModelUsage[],
		sort = (a: Model, b: Model) => (b.name ?? '').localeCompare(a.name ?? '')
	) {
		const _usages = Array.isArray(usages) ? usages : [usages];

		// Vision models are LLMs
		if (_usages.includes(ModelUsage.Vision)) {
			_usages.push(ModelUsage.LLM);
		}

		return models.filter((model) => _usages.includes(model.usage as ModelUsage)).sort(sort);
	}

	function getSelectedModel(modelAlias: DefaultModelAlias, activeModelOptions: Model[]) {
		// If there's a pending change, use that
		if (changes?.[modelAlias.alias]) {
			return changes[modelAlias.alias];
		}

		// If a model is already set, use it
		if (modelAlias.model) {
			return modelAlias.model;
		}

		// Auto-select suggested model if available
		const suggestedModelName = SUGGESTED_MODEL_SELECTIONS[modelAlias.alias];
		if (suggestedModelName) {
			const suggestedModel = activeModelOptions.find((model) => model.name === suggestedModelName);
			if (suggestedModel) {
				return suggestedModel.id;
			}
		}

		// No selection
		return '';
	}

	async function handleSaveChanges() {
		loading = true;
		await Promise.all(
			Object.entries(changes ?? {}).map(([alias, model]) =>
				AdminService.updateDefaultModelAlias(alias as ModelAlias, {
					alias: alias as ModelAlias,
					model
				})
			)
		);
		defaultModelAliases = await AdminService.listDefaultModelAliases();
		changes = {};
		loading = false;
		dialog?.close();
	}

	function onClose() {
		changes = {};
	}
</script>

<button
	class="button-primary text-sm font-normal"
	disabled={availableModels.length === 0 || loading}
	onclick={() => open()}
>
	Set Default Models
</button>

<ResponsiveDialog
	{onClose}
	class="overflow-visible"
	bind:this={dialog}
	title="Default Model Aliases"
>
	<p class="pb-4 font-light text-gray-400 dark:text-gray-600">
		When no model is specified, a default model is used for creating a new project, running user
		tasks, or working with some tools, etc. Select your default models for the usage types below.
	</p>
	<div class="flex flex-col gap-4 py-4">
		{#each sortedModelAliases as modelAlias (modelAlias.alias)}
			{@const usage = getModelUsageFromAlias(modelAlias.alias)}
			{@const activeModelOptions = usage
				? filterModelsByActive(filterModelsByUsage(availableModels ?? [], usage))
				: []}
			<div class="flex items-center gap-2">
				<label class="w-1/2" for={modelAlias.alias}>{getModelAliasLabel(modelAlias.alias)}</label>
				<Select
					id={modelAlias.alias}
					classes={{ root: 'w-1/2' }}
					class="bg-surface1 dark:bg-surface2 dark:border-surface3 flex-1 border border-transparent shadow-inner"
					options={activeModelOptions
						.map((model) => ({
							label: SUGGESTED_MODEL_SELECTIONS[modelAlias.alias].includes(model.name)
								? `${model.name ?? ''} (Suggested)`
								: (model.name ?? ''),
							id: model.id
						}))
						.sort((a, b) => {
							const aIsSuggested = a.label.includes('(Suggested)');
							const bIsSuggested = b.label.includes('(Suggested)');
							// Sort suggested models to the top
							if (aIsSuggested && !bIsSuggested) return -1;
							if (!aIsSuggested && bIsSuggested) return 1;
							// Keep original order for models with same suggested status
							return 0;
						})}
					selected={getSelectedModel(modelAlias, activeModelOptions)}
					onSelect={async (option) => {
						changes = {
							...changes,
							[modelAlias.alias as ModelAlias]: option.id as string
						};
					}}
					disabled={readonly}
				/>
			</div>
		{/each}
	</div>
	{#if !readonly}
		<div class="flex flex-col gap-4 pt-4">
			<button
				class="button-primary w-full text-sm font-normal"
				onclick={handleSaveChanges}
				disabled={loading || !changed}
			>
				{#if loading}
					<LoaderCircle class="size-4 animate-spin" />
				{:else}
					Save Changes
				{/if}
			</button>
			{#if showSkip}
				<button class="button w-full text-sm font-normal" onclick={() => dialog?.close()}>
					Skip
				</button>
			{/if}
		</div>
	{/if}
</ResponsiveDialog>
