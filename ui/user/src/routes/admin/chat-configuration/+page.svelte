<script lang="ts">
	import { autoHeight } from '$lib/actions/textarea.js';
	import MarkdownInput from '$lib/components/MarkdownInput.svelte';
	import DotDotDot from '$lib/components/DotDotDot.svelte';
	import InfoTooltip from '$lib/components/InfoTooltip.svelte';
	import Layout from '$lib/components/Layout.svelte';
	import ResponsiveDialog from '$lib/components/ResponsiveDialog.svelte';
	import Search from '$lib/components/Search.svelte';
	import Table from '$lib/components/table/Table.svelte';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants.js';
	import { HELPER_TEXTS } from '$lib/context/helperMode.svelte.js';
	import { AdminService, ModelUsage, type Model, type ModelProvider } from '$lib/services';
	import { sortModelProviders } from '$lib/sort';
	import { Check, Info, LoaderCircle, Plus, TriangleAlert } from 'lucide-svelte';
	import { onMount } from 'svelte';
	import { fade } from 'svelte/transition';
	import { twMerge } from 'tailwind-merge';
	import { profile } from '$lib/stores/index.js';

	const duration = PAGE_TRANSITION_DURATION;
	let { data } = $props();
	let prevAgent = $state(data.baseAgent);
	let baseAgent = $state(data.baseAgent);
	let saving = $state(false);
	let showSaved = $state(false);
	let timeout = $state<ReturnType<typeof setTimeout>>();

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

	let modelProvidersMap = $derived(
		new Map(modelsData.modelProviders.map((provider) => [provider.id, provider]))
	);
	let selectedModels = $derived(
		modelsData.models.filter((model) => baseAgent?.allowedModels?.includes(model.id))
	);
	let modelOptions = $derived(
		modelsData.models.filter((model) => !model.usage || model.usage === ModelUsage.LLM)
	);

	let filterAvailableModelSets = $derived(
		modelOptions.filter(
			(model) =>
				model.name.toLowerCase().includes(addModelsSearch.toLowerCase()) ||
				model.modelProviderName.toLowerCase().includes(addModelsSearch.toLowerCase())
		)
	);

	let modelProviderSets = $derived(compileModelsByModelProviders(filterAvailableModelSets));

	let sortedModelProviderAndModels = $derived(
		modelProviderSets && modelsData.modelProviders.length > 0
			? sortModelProviders(modelsData.modelProviders).map((modelProvider) => ({
					modelProvider,
					models: (modelProviderSets[modelProvider.id] ?? []).sort((a, b) => {
						const aStartsWithGpt = a.name.toLowerCase().startsWith('gpt');
						const bStartsWithGpt = b.name.toLowerCase().startsWith('gpt');

						if (aStartsWithGpt === bStartsWithGpt) {
							return a.name.localeCompare(b.name);
						}

						return aStartsWithGpt ? -1 : 1;
					})
				}))
			: []
	);

	let tableData = $derived(
		selectedModels.map((model) => ({
			...model,
			isDefault: model.id === baseAgent?.model
		}))
	);

	let isAdminReadonly = $derived(profile.current.isAdminReadonly?.());

	onMount(() => {
		Promise.all([AdminService.listModelProviders(), AdminService.listModels()]).then(
			([modelProviders, models]) => {
				modelsData = {
					modelProviders,
					models
				};

				if (baseAgent && baseAgent.model && (baseAgent.allowedModels ?? []).length === 0) {
					const match = models.find((m) => m.id === baseAgent!.model);
					if (match) {
						baseAgent.allowedModels = [match.name];
					}
				}
			}
		);
	});

	async function handleSave() {
		if (!baseAgent) return;
		if (timeout) {
			clearTimeout(timeout);
		}
		saving = true;
		try {
			const response = await AdminService.updateBaseAgent(baseAgent);
			prevAgent = baseAgent;
			baseAgent = response;
			showSaved = true;
			timeout = setTimeout(() => {
				showSaved = false;
			}, 3000);
		} catch (err) {
			console.error(err);
			// default behavior will show snackbar error
		} finally {
			saving = false;
		}
	}

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

<Layout title="Chat Configuration" classes={{ container: 'pb-0' }}>
	<div class="relative mt-4 h-full w-full" transition:fade={{ duration }}>
		<div class="flex flex-col gap-8">
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
				<div class="paper">
					<div class="flex gap-6">
						<div class="flex grow flex-col gap-4">
							<div class="flex flex-col gap-1">
								<label class="text-sm" for="name">Name</label>
								<input
									type="text"
									id="name"
									bind:value={baseAgent.name}
									class="text-input-filled dark:bg-background"
									disabled={isAdminReadonly}
								/>
							</div>
							<div class="flex flex-col gap-1">
								<label class="text-sm" for="description">Description</label>
								<input
									type="text"
									id="description"
									bind:value={baseAgent.description}
									class="text-input-filled dark:bg-background"
									disabled={isAdminReadonly}
								/>
							</div>
						</div>
					</div>

					<div class="flex flex-col gap-1">
						<label class="text-sm" for="prompt">Introductions</label>
						<MarkdownInput
							bind:value={baseAgent.introductionMessage}
							placeholder="Begin every conversation with an introduction."
							disabled={isAdminReadonly}
						/>
					</div>

					<div class="flex flex-col gap-1">
						<label class="text-sm" for="prompt">Instructions</label>
						<textarea
							rows={6}
							id="prompt"
							bind:value={baseAgent.prompt}
							class="text-input-filled dark:bg-background"
							placeholder={HELPER_TEXTS.prompt}
							use:autoHeight
							disabled={isAdminReadonly}
						></textarea>
					</div>
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

						{#if !isAdminReadonly}
							<button
								class="button-primary flex items-center gap-1"
								onclick={() => showAddModelsDialog?.open()}
							>
								<Plus class="size-4" />
								Add Model
							</button>
						{/if}
					</div>

					<Table
						data={tableData}
						fields={['name', 'isDefault']}
						headers={[
							{
								property: 'isDefault',
								title: 'Is Default'
							}
						]}
						headerClasses={[
							{
								property: 'isDefault',
								class: 'w-28'
							}
						]}
						noDataMessage="No models added."
					>
						{#snippet actions(d)}
							{#if !isAdminReadonly}
								<DotDotDot>
									<div class="default-dialog flex min-w-max flex-col p-2">
										<button
											class="menu-button"
											onclick={() => {
												if (!baseAgent) return;
												baseAgent.model = d.id;
											}}
										>
											Set as Default
										</button>
										{#if !d.isDefault}
											<button
												class="menu-button"
												onclick={() => {
													if (!baseAgent) return;
													baseAgent.allowedModels = baseAgent.allowedModels?.filter(
														(modelId) => modelId !== d.id
													);
												}}
											>
												Remove
											</button>
										{/if}
									</div>
								</DotDotDot>
							{/if}
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
							{:else if property === 'isDefault'}
								<div class="flex w-full items-center justify-center gap-2">
									{#if d.isDefault}
										<Check class="text-primary size-5" />
									{:else}
										<div class="size-5"></div>
									{/if}
								</div>
							{/if}
						{/snippet}
					</Table>
				</div>

				{#if !isAdminReadonly}
					<div
						class="bg-surface1 dark:bg-background sticky bottom-0 left-0 flex w-[calc(100%+2em)] -translate-x-4 justify-end gap-4 p-4 md:w-[calc(100%+4em)] md:-translate-x-8 md:px-8"
					>
						{#if showSaved}
							<span
								in:fade={{ duration: 200 }}
								class="text-on-surface1 flex min-h-10 items-center px-4 text-sm font-extralight"
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
					<div class="h-4"></div>
				{/if}
			{:else}
				<div class="h-full w-full items-center justify-center">
					<TriangleAlert class="text-on-surface1 size-24 opacity-50" />
					<h4 class="text-on-surface1 text-lg font-semibold">An Error Occurred!</h4>
					<p class="text-on-surface1 text-sm font-light">
						We were unable to load the default base agent. Please try again later or contact
						support.
					</p>
				</div>
			{/if}
		</div>
	</div>
</Layout>

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
		<div class="mb-4 px-4">
			<Search
				class="dark:border-surface3 bg-background border border-transparent shadow-sm"
				onChange={(val) => (addModelsSearch = val)}
				placeholder="Search models..."
			/>
		</div>

		<div class="default-scrollbar-thin flex h-96 flex-col gap-2 overflow-y-auto">
			{#each sortedModelProviderAndModels as { modelProvider, models } (modelProvider.id)}
				{#if models.length > 0}
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
						{#each models as model (model.id)}
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
									<Check class="text-primary size-4" />
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
