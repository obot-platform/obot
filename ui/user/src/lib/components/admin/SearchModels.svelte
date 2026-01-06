<script lang="ts">
	import { Check, Cpu } from 'lucide-svelte';
	import Search from '../Search.svelte';
	import ResponsiveDialog from '../ResponsiveDialog.svelte';
	import { twMerge } from 'tailwind-merge';
	import {
		ModelUsageLabels,
		type ModelUsage,
		ModelAlias,
		ModelAliasLabels,
		type DefaultModelAlias
	} from '$lib/services/admin/types';
	import { AdminService, type Model, type ModelProvider } from '$lib/services';
	import { sortModelProviders } from '$lib/sort';
	import Logo from '../Logo.svelte';
	import { onMount } from 'svelte';

	interface Props {
		onAdd: (modelIds: string[]) => void;
		models: Model[];
		defaultAliases: DefaultModelAlias[];
		exclude?: string[];
		title?: string;
	}

	let { onAdd, models, defaultAliases, exclude = [], title = 'Add Models' }: Props = $props();
	let addModelDialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let search = $state('');
	let selected = $state<string[]>([]);
	let selectedSet = $derived(new Set(selected));

	let modelProviders = $state<ModelProvider[]>([]);
	let defaultModelAliases = $derived(defaultAliases);

	onMount(async () => {
		modelProviders = await AdminService.listModelProviders();
	});

	// Filter models based on exclude list and search
	let filteredModels = $derived(
		models
			.filter((model) => !exclude?.includes(model.id))
			.filter((model) => {
				if (!search) return true;
				const lowerSearch = search.toLowerCase();
				return (
					(model.displayName || model.name).toLowerCase().includes(lowerSearch) ||
					(model.modelProviderName || '').toLowerCase().includes(lowerSearch)
				);
			})
	);

	// Group models by provider
	function compileModelsByModelProviders(models: Model[]) {
		return models.reduce(
			(acc, model) => {
				acc[model.modelProvider] = acc[model.modelProvider] || [];
				acc[model.modelProvider].push(model);
				return acc;
			},
			{} as Record<string, Model[]>
		);
	}

	let modelProviderSets = $derived(compileModelsByModelProviders(filteredModels));

	let sortedModelProviderAndModels = $derived(
		modelProviders.length > 0
			? sortModelProviders(modelProviders).map((modelProvider) => ({
					modelProvider,
					models: (modelProviderSets[modelProvider.id] ?? []).sort((a, b) => {
						const aStartsWithGpt = a.name.toLowerCase().startsWith('gpt');
						const bStartsWithGpt = b.name.toLowerCase().startsWith('gpt');

						if (aStartsWithGpt && !bStartsWithGpt) return -1;
						if (!aStartsWithGpt && bStartsWithGpt) return 1;

						return a.name.localeCompare(b.name);
					})
				}))
			: []
	);

	// Check if wildcard is available (hide if search is active)
	let wildcardAvailable = $derived(!exclude?.includes('*') && !search);

	// Map for quick model lookups
	let modelsMap = $derived(new Map(models.map((m) => [m.id, m])));

	// Prepare default aliases for display
	let aliasDisplayData = $derived(
		Object.values(ModelAlias).map((aliasName) => {
			const aliasId = `obot://${aliasName}`;
			const aliasData = defaultModelAliases.find((a) => a.alias === aliasName);
			const model = aliasData?.model ? modelsMap.get(aliasData.model) : undefined;

			return {
				id: aliasId,
				aliasName,
				label: ModelAliasLabels[aliasName as keyof typeof ModelAliasLabels] || aliasName,
				effectiveModelName: model?.displayName || model?.targetModel || 'Not configured',
				isConfigured: !!model,
				isExcluded: exclude?.includes(aliasId) ?? false
			};
		})
	);

	let availableAliases = $derived(aliasDisplayData.filter((a) => !a.isExcluded));

	// Filter aliases based on search
	let filteredAliases = $derived(
		availableAliases.filter((alias) => {
			if (!search) return true;
			const lowerSearch = search.toLowerCase();
			return (
				alias.aliasName.toLowerCase().includes(lowerSearch) ||
				alias.label.toLowerCase().includes(lowerSearch) ||
				alias.effectiveModelName.toLowerCase().includes(lowerSearch)
			);
		})
	);

	let defaultAliasesAvailable = $derived(filteredAliases.length > 0);

	export function open() {
		addModelDialog?.open();
	}

	function onClose() {
		search = '';
		selected = [];
	}

	function handleAdd() {
		onAdd(selected);
		addModelDialog?.close();
	}

	function toggleSelection(modelId: string) {
		if (selectedSet.has(modelId)) {
			selected = selected.filter((id) => id !== modelId);
		} else {
			selected = [...selected, modelId];
		}
	}
</script>

<ResponsiveDialog
	bind:this={addModelDialog}
	{onClose}
	{title}
	class="h-full w-full overflow-visible md:h-[500px] md:max-w-md"
	classes={{ header: 'p-4 md:pb-0', content: 'min-h-inherit p-0' }}
>
	<div class="default-scrollbar-thin flex grow flex-col gap-4 overflow-y-auto pt-1">
		<div class="flex flex-col gap-2">
			<div class="px-4">
				<Search
					class="dark:bg-surface1 dark:border-surface3 shadow-inner dark:border"
					onChange={(val) => (search = val)}
					value={search}
					placeholder="Search models..."
				/>
			</div>

			<div class="flex flex-col gap-2">
				{#if wildcardAvailable}
					<button
						class={twMerge(
							'hover:bg-surface3 dark:hover:bg-surface1 flex items-center justify-between gap-4 px-4 py-3 text-left',
							selectedSet.has('*') && 'dark:bg-gray-920 bg-gray-50'
						)}
						onclick={() => toggleSelection('*')}
					>
						<div class="flex items-center gap-2">
							<Cpu class="size-8 flex-shrink-0" />
							<div class="flex flex-col">
								<p class="font-medium">All Models</p>
								<span class="text-on-surface1 text-xs">
									Grants access to all current and future models
								</span>
							</div>
						</div>
						<div class="flex size-6 items-center justify-center">
							{#if selectedSet.has('*')}
								<Check class="text-primary size-6" />
							{/if}
						</div>
					</button>
				{/if}

				{#if defaultAliasesAvailable}
					<div class="flex flex-col gap-1 px-2 py-1">
						<h4 class="text-md mx-2 flex items-center gap-2 font-semibold">
							<Logo class="size-4" />
							Default Models
						</h4>
					</div>
					<div class="flex flex-col gap-1 px-8">
						{#each filteredAliases as alias (alias.id)}
							<button
								class={twMerge(
									'hover:bg-surface3 flex items-center justify-between gap-4 rounded-md bg-transparent p-2 font-light',
									selectedSet.has(alias.id) && 'bg-surface2'
								)}
								onclick={() => toggleSelection(alias.id)}
							>
								<div class="flex flex-col text-left">
									<div class="flex items-center gap-2">
										<span>{alias.aliasName}</span>
										<span
											class="text-on-surface1 text-xs"
											class:text-yellow-500={!alias.isConfigured}
										>
											{alias.effectiveModelName}
										</span>
									</div>
									<span class="text-on-surface1 text-xs">{alias.label}</span>
								</div>
								{#if selectedSet.has(alias.id)}
									<Check class="text-primary size-4" />
								{/if}
							</button>
						{/each}
					</div>
				{/if}

				{#each sortedModelProviderAndModels as { modelProvider, models } (modelProvider.id)}
					{#if models.length > 0}
						<div class="flex flex-col gap-1 px-2 py-1">
							<h4 class="text-md mx-2 flex items-center gap-2 font-semibold">
								<img src={modelProvider.icon} alt={modelProvider?.name} class="icon size-4" />
								{modelProvider.name}
							</h4>
						</div>
						<div class="flex flex-col gap-1 px-8">
							{#each models as model (model.id)}
								<button
									class={twMerge(
										'hover:bg-surface3 flex items-center justify-between gap-4 rounded-md bg-transparent p-2 font-light',
										selectedSet.has(model.id) && 'bg-surface2'
									)}
									onclick={() => toggleSelection(model.id)}
								>
									<div class="flex flex-col text-left">
										<span>{model.displayName || model.name}</span>
										{#if model.usage}
											<span class="text-on-surface1 text-xs">
												{ModelUsageLabels[model.usage as ModelUsage] || model.usage}
											</span>
										{/if}
									</div>
									{#if selectedSet.has(model.id)}
										<Check class="text-primary size-4" />
									{/if}
								</button>
							{/each}
						</div>
					{/if}
				{/each}
			</div>
		</div>
	</div>
	<div class="flex w-full flex-col justify-between gap-4 p-4 md:flex-row">
		<div class="flex items-center gap-1 font-light">
			{#if selected.length > 0}
				<Cpu class="size-4" />
				{selected.length} Selected
			{/if}
		</div>
		<div class="flex items-center gap-2">
			<button class="button w-full md:w-fit" onclick={() => addModelDialog?.close()}>
				Cancel
			</button>
			<button class="button-primary w-full md:w-fit" onclick={handleAdd}> Confirm </button>
		</div>
	</div>
</ResponsiveDialog>
