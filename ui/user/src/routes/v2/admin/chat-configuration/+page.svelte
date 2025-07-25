<script lang="ts">
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import MarkdownInput from '$lib/components/admin/MarkdownInput.svelte';
	import EditIcon from '$lib/components/edit/EditIcon.svelte';
	import InfoTooltip from '$lib/components/InfoTooltip.svelte';
	import Layout from '$lib/components/Layout.svelte';
	import PageLoading from '$lib/components/PageLoading.svelte';
	import ResponsiveDialog from '$lib/components/ResponsiveDialog.svelte';
	import Search from '$lib/components/Search.svelte';
	import Table from '$lib/components/Table.svelte';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants.js';
	import { AdminService, type Model, type ModelProvider } from '$lib/services';
	import { sortModelProviders } from '$lib/sort.js';
	import { darkMode } from '$lib/stores/index.js';
	import { Check, Info, LoaderCircle, Plus, Trash2, TriangleAlert } from 'lucide-svelte';
	import { onMount } from 'svelte';
	import { fade } from 'svelte/transition';
	import { twMerge } from 'tailwind-merge';

	const duration = PAGE_TRANSITION_DURATION;
	let { data } = $props();
	let prevAgent = $state(data.baseAgent);
	let baseAgent = $state(data.baseAgent);
	let saving = $state(false);
	let showSaved = $state(false);
	let timeout = $state<ReturnType<typeof setTimeout>>();
	let loading = $state(false);

	let showAddModelsDialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let addModelsSelected = $state<Record<string, boolean>>({});
	let addModelsSearch = $state('');

	let modelsData = $state<{
		modelProviders: ModelProvider[];
		models: Model[];
	}>({
		modelProviders: [],
		models: []
	});

	let selectedModelProviders = $derived(new Set(baseAgent?.allowedModelProviders ?? []));
	let modelProvidersMap = $derived(
		new Map(modelsData.modelProviders.map((provider) => [provider.id, provider]))
	);
	let selectedModels = $derived(
		modelsData.models.filter((model) => baseAgent?.allowedModels?.includes(model.id))
	);
	let filterAvailableModelSets = $derived(
		modelsData.models.filter(
			(model) =>
				model.name.toLowerCase().includes(addModelsSearch.toLowerCase()) ||
				model.modelProviderName.toLowerCase().includes(addModelsSearch.toLowerCase())
		)
	);

	onMount(() => {
		Promise.all([AdminService.listModelProviders(), AdminService.listModels()]).then(
			([modelProviders, models]) => {
				modelsData = {
					modelProviders,
					models
				};
			}
		);
	});

	$effect(() => {
		if (showSaved) {
			clearTimeout(timeout);
			timeout = setTimeout(() => {
				showSaved = false;
			}, 3000);
		}
		return () => {
			clearTimeout(timeout);
		};
	});

	async function handleSave() {
		if (!baseAgent) return;
		loading = true;
		try {
			const response = await AdminService.updateBaseAgent(baseAgent);
			prevAgent = baseAgent;
			baseAgent = response;
			showSaved = true;
		} catch (err) {
			console.error(err);
			// default behavior will show snackbar error
		} finally {
			loading = false;
		}
	}

	function compileModelsByModelProviders(models: Model[]) {
		return models
			.filter((model) => selectedModelProviders.has(model.modelProvider))
			.reduce(
				(acc, model) => {
					acc[model.modelProvider] = acc[model.modelProvider] || [];
					acc[model.modelProvider].push(model);
					return acc;
				},
				{} as Record<string, Model[]>
			);
	}

	function resetAddModels() {
		addModelsSelected = {};
		addModelsSearch = '';
		showAddModelsDialog?.close();
	}

	function handleAddModels() {
		if (!baseAgent) return;
		const alreadyAddedMap = new Set(baseAgent.allowedModels ?? []);
		const newAddedModels = Object.keys(addModelsSelected).filter(
			(modelId) => !alreadyAddedMap.has(modelId)
		);
		baseAgent.allowedModels = [...(baseAgent.allowedModels ?? []), ...newAddedModels];
		resetAddModels();
	}
</script>

<Layout>
	<div class="relative my-4 h-full w-full" transition:fade={{ duration }}>
		<div class="flex flex-col gap-8">
			<h1 class="text-2xl font-semibold">Chat Configuration</h1>

			<div class="notification-info p-3 text-sm font-light">
				<div class="flex items-center gap-3">
					<Info class="size-6" />
					<div>
						Modifying the default chat configuration will affect all users. Projects created will
						inherit the properties below.
					</div>
				</div>
			</div>

			{#if baseAgent}
				<div
					class="dark:bg-surface1 dark:border-surface3 flex h-fit w-full flex-col gap-4 rounded-lg border border-transparent bg-white p-6 shadow-sm"
				>
					<div class="flex gap-6">
						<div>
							<EditIcon project={baseAgent} edit classes={{ icon: 'size-36' }} />
						</div>

						<div class="flex grow flex-col gap-4">
							<div class="flex flex-col gap-1">
								<label class="text-sm" for="name">Name</label>
								<input
									type="text"
									id="name"
									bind:value={baseAgent.name}
									class="text-input-filled dark:bg-black"
								/>
							</div>
							<div class="flex flex-col gap-1">
								<label class="text-sm" for="description">Description</label>
								<input
									type="text"
									id="description"
									bind:value={baseAgent.description}
									class="text-input-filled dark:bg-black"
								/>
							</div>
						</div>
					</div>

					<div class="flex flex-col gap-1">
						<label class="text-sm" for="prompt">Introductions</label>
						<MarkdownInput
							bind:value={baseAgent.introductionMessage}
							placeholder="Begin every conversation with an introduction."
						/>
					</div>
				</div>

				<div class="flex flex-col gap-2">
					<div class="flex items-center gap-2">
						<h2 class="text-xl font-semibold">Model Providers</h2>
						<InfoTooltip
							text="Select the model providers that projects can utilize."
							class="size-4"
							classes={{ icon: 'size-4' }}
						/>
					</div>

					{@render modelProvidersView()}
				</div>

				<div class="flex flex-col gap-2">
					<div class="flex items-center justify-between gap-4">
						<div class="flex items-center gap-2">
							<h2 class="text-xl font-semibold">Allowed Models</h2>
							<InfoTooltip
								text="Select the specific models for each model provider that projects can use."
								class="size-4"
								classes={{ icon: 'size-4' }}
							/>
						</div>

						<button
							class="button-primary flex items-center gap-1"
							onclick={() => showAddModelsDialog?.open()}
						>
							<Plus class="size-4" />
							Add Model
						</button>
					</div>

					<Table data={selectedModels} fields={['name']} noDataMessage="No models added.">
						{#snippet actions(d)}
							<button
								class="icon-button hover:text-red-500"
								onclick={() => {
									if (!baseAgent) return;
									baseAgent.allowedModels = baseAgent.allowedModels?.filter(
										(modelId) => modelId !== d.id
									);
								}}
								use:tooltip={'Remove Model'}
							>
								<Trash2 class="size-4" />
							</button>
						{/snippet}

						{#snippet onRenderColumn(property, d)}
							{#if property === 'name'}
								<div class="flex items-center gap-2">
									<img
										src={modelProvidersMap.get(d.modelProvider)?.icon}
										alt={d.modelProvider}
										class="size-6 rounded-md bg-gray-50 p-1 dark:bg-gray-600"
									/>
									{d.name}
								</div>
							{/if}
						{/snippet}
					</Table>
				</div>

				<div
					class="bg-surface1 sticky bottom-0 left-0 flex w-[calc(100%+2em)] -translate-x-4 justify-end gap-4 p-4 md:w-[calc(100%+4em)] md:-translate-x-8 md:px-8 dark:bg-black"
				>
					{#if showSaved}
						<span
							in:fade={{ duration: 200 }}
							class="flex min-h-10 items-center px-4 text-sm font-extralight text-gray-500"
						>
							Your changes have been saved.
						</span>
					{/if}

					<button
						class="button hover:bg-surface3 flex items-center gap-1 bg-transparent"
						onclick={() => {
							baseAgent = prevAgent;
						}}
					>
						Reset
					</button>
					<button
						class="button-primary flex items-center gap-1"
						disabled={saving}
						onclick={handleSave}
					>
						{#if saving}
							<LoaderCircle class="size-4 animate-spin" />
						{:else}
							Save
						{/if}
					</button>
				</div>
			{:else}
				<div class="h-full w-full items-center justify-center">
					<TriangleAlert class="size-24 text-gray-200 dark:text-gray-900" />
					<h4 class="text-lg font-semibold text-gray-400 dark:text-gray-600">An Error Occurred!</h4>
					<p class="text-sm font-light text-gray-400 dark:text-gray-600">
						We were unable to load the default base agent. Please try again later or contact
						support.
					</p>
				</div>
			{/if}
		</div>
	</div>
</Layout>

{#snippet modelProvidersView()}
	{#if baseAgent}
		<div class="grid grid-cols-2 gap-2">
			{#each sortModelProviders(modelsData.modelProviders) as modelProvider}
				<button
					class={twMerge(
						'dark:bg-surface1 dark:border-surface3 flex items-center justify-between rounded-md border border-transparent bg-white p-2 shadow-sm transition-colors duration-300 hover:border-blue-500 hover:bg-blue-500/30 dark:hover:border-blue-500 dark:hover:bg-blue-500/30',
						selectedModelProviders.has(modelProvider.id) &&
							'border-blue-500/75 bg-blue-500/10 dark:border-blue-500/75 dark:bg-blue-500/10'
					)}
					onclick={() => {
						if (!baseAgent) return;
						if (selectedModelProviders.has(modelProvider.id)) {
							baseAgent.allowedModelProviders =
								baseAgent.allowedModelProviders?.filter((id) => id !== modelProvider.id) ?? [];
						} else {
							baseAgent.allowedModelProviders = [
								...(baseAgent.allowedModelProviders ?? []),
								modelProvider.id
							];
						}
					}}
				>
					<div class="flex items-center gap-3">
						<div
							class={twMerge(
								'rounded-md p-1',
								darkMode.isDark && !modelProvider.iconDark && 'bg-surface1 dark:bg-gray-600'
							)}
						>
							{#if modelProvider.iconDark && darkMode.isDark}
								<img src={modelProvider.iconDark} alt={modelProvider.name} class="size-6" />
							{:else if modelProvider.icon}
								<img src={modelProvider.icon} alt={modelProvider.name} class="size-6" />
							{/if}
						</div>
						{modelProvider.name}
					</div>
					{#if selectedModelProviders.has(modelProvider.id)}
						<Check class="mx-2 size-6 text-blue-500" />
					{/if}
				</button>
			{/each}
		</div>
	{/if}
{/snippet}

<ResponsiveDialog
	bind:this={showAddModelsDialog}
	title="Add Models"
	class="dark:bg-surface1 p-0"
	classes={{
		header: 'p-4 pb-0'
	}}
	onClose={() => {
		addModelsSearch = '';
		addModelsSelected = {};
	}}
>
	{#if baseAgent}
		{@const modelProviderSets = compileModelsByModelProviders(filterAvailableModelSets)}
		<div class="mb-4 px-4">
			<Search
				class="dark:border-surface3 border border-transparent bg-white shadow-sm dark:bg-black"
				onChange={(val) => (addModelsSearch = val)}
				placeholder="Search models..."
			/>
		</div>

		<div class="default-scrollbar-thin flex h-96 flex-col gap-2 overflow-y-auto">
			{#each Object.keys(modelProviderSets) as modelProviderId (modelProviderId)}
				{@const models = modelProviderSets[modelProviderId]}
				{@const modelProvider = modelProvidersMap.get(modelProviderId)}
				{#if modelProvider}
					<div class="flex flex-col gap-1 px-2 py-1">
						<h4 class="text-md mx-2 flex items-center gap-2 font-semibold">
							<img
								src={modelProvider.icon}
								alt={modelProvider?.name}
								class="size-4 rounded-md bg-gray-50 p-0.5 dark:bg-gray-600"
							/>
							{modelProvider.name}
						</h4>
					</div>
					<div class="flex flex-col gap-1 px-8">
						{#each models as model}
							<button
								class={twMerge(
									'hover:bg-surface3 flex items-center justify-between gap-4 rounded-md bg-transparent p-2 font-light',
									addModelsSelected[model.id] && 'bg-surface2'
								)}
								onclick={() => {
									if (addModelsSelected[model.id]) {
										delete addModelsSelected[model.id];
									} else {
										addModelsSelected[model.id] = true;
									}
								}}
							>
								{model.name}
								{#if addModelsSelected[model.id]}
									<Check class="size-4 text-blue-500" />
								{/if}
							</button>
						{/each}
					</div>
				{/if}
			{/each}
		</div>
	{/if}

	<div class="mt-auto flex justify-end gap-4 p-4">
		<button class="button" onclick={resetAddModels}> Cancel </button>
		<button class="button-primary" onclick={handleAddModels}> Add </button>
	</div>
</ResponsiveDialog>

<svelte:head>
	<title>Obot | Chat Configuration</title>
</svelte:head>
