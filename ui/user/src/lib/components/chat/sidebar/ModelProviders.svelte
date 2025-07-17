<script lang="ts">
	import { untrack } from 'svelte';
	import { getLayout } from '$lib/context/chatLayout.svelte';
	import CollapsePane from '$lib/components/edit/CollapsePane.svelte';
	import { HELPER_TEXTS } from '$lib/context/helperMode.svelte';
	import { listModelProviders, type ModelProvider, type Project } from '$lib/services';
	import { Info } from 'lucide-svelte';
	import { SvelteMap } from 'svelte/reactivity';
	import { darkMode } from '$lib/stores';
	import { twMerge } from 'tailwind-merge';
	import { delay } from '$lib/utils';

	interface Props {
		project?: Project;
	}

	let { project }: Props = $props();
	const layout = getLayout();

	let isLoading = $state(false);

	let projectProviderModelIds = $derived(
		Object.keys(project?.models ?? {}).sort((a, b) => a.localeCompare(b))
	);
	let providersWithMoreData: Map<string, ModelProvider> = new SvelteMap();

	// Get a friendly name for a provider
	function getProviderName(providerId: string): string {
		return providersWithMoreData.get(providerId)?.name || providerId;
	}

	// Function to open model providers config
	function openModelProvidersConfig() {
		layout.sidebarConfig = 'model-providers';
	}

	// Function to fetch model providers
	async function loadModelProviders(project: Project) {
		isLoading = true;

		try {
			listModelProviders(project.assistantID, project.id).then((res) => {
				untrack(() => {
					for (const provider of res.items ?? []) {
						providersWithMoreData.set(provider.id, provider);
					}
				});
			});
		} catch (error) {
			console.error('Failed to load model providers:', error);
		} finally {
			isLoading = false;
		}
	}

	$effect(() => {
		if (!project || !project.assistantID || !project.id) return;

		loadModelProviders(project);
	});
</script>

<CollapsePane
	classes={{ header: 'pl-3 py-2 text-md', content: 'p-0' }}
	iconSize={5}
	header="Model Providers"
	helpText={HELPER_TEXTS.modelProviders}
>
	<div class="flex flex-col gap-8 p-2">
		<div class="flex flex-col gap-4">
			<p class="text-xs font-light text-gray-500">
				Projects use our included LLM by default. Connect other model providers to unlock more LLMs
				you can chat with.
			</p>

			{#if isLoading}
				<p class="text-xs italic">Loading model providers...</p>
			{:else if project?.models && Object.keys(project.models).length > 0}
				<div class="pb-2">
					<p class="mb-1 text-sm font-medium">Configured providers:</p>
					<ul class="flex flex-col text-xs">
						{#each projectProviderModelIds as providerId (providerId)}
							{@const provider = providersWithMoreData.get(providerId)}

							<li class="model-provider w-full">
								<button
									class="hover:bg-surface3 flex w-full items-center gap-1 rounded-md p-2 transition-colors duration-200"
									onclick={async () => {
										openModelProvidersConfig();

										// Wait for the UI to be ready
										await delay(300);

										requestAnimationFrame(() => {
											const cardElement = document.querySelector(
												`.model-provider-card[data-provider-id="${providerId}"]`
											);

											if (cardElement) {
												cardElement.scrollIntoView({ behavior: 'smooth', block: 'center' });
											}
										});
									}}
								>
									<div class="size-4">
										{#if provider?.icon || provider?.iconDark}
											<img
												src={darkMode.isDark && provider.iconDark
													? provider.iconDark
													: provider.icon}
												alt={provider.name}
												class={twMerge(
													'size-full',
													darkMode.isDark && !provider.iconDark ? 'dark:invert' : ''
												)}
											/>
										{/if}
									</div>

									<span>{getProviderName(providerId)}</span>
									<span class="text-muted-foreground"
										>({project.models[providerId].length}
										{project.models[providerId].length === 1 ? 'model' : 'models'})</span
									>
								</button>
							</li>
						{/each}
					</ul>
				</div>
			{:else}
				<div class="text-gray flex items-center gap-2 rounded-lg py-4 text-xs">
					<div>
						<Info class="h-5" />
					</div>

					<div>No model providers configured</div>
				</div>
			{/if}
		</div>

		<div class="flex justify-end">
			<button class="button flex items-center gap-1" onclick={openModelProvidersConfig}>
				<span class="text-xs">Manage Model Providers</span>
			</button>
		</div>
	</div>
</CollapsePane>
