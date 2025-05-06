<script lang="ts">
	import { onMount } from 'svelte';
	import { getLayout } from '$lib/context/layout.svelte';
	import CollapsePane from '$lib/components/edit/CollapsePane.svelte';
	import { HELPER_TEXTS } from '$lib/context/helperMode.svelte';
	import { listModelProviders, type Project } from '$lib/services';

	interface Props {
		project?: Project;
	}

	let { project }: Props = $props();
	const layout = getLayout();

	let providerNames = $state<Record<string, string>>({});
	let isLoading = $state(false);

	// Get a friendly name for a provider
	function getProviderName(providerId: string): string {
		return providerNames[providerId] || providerId;
	}

	// Function to open model providers config
	function openModelProvidersConfig() {
		layout.sidebarConfig = 'model-providers';
	}

	// Sort provider IDs by their display names
	function getSortedProviderIds(providerIds: string[]): string[] {
		return [...providerIds].sort((a, b) => {
			const nameA = getProviderName(a);
			const nameB = getProviderName(b);
			return nameA.localeCompare(nameB);
		});
	}

	// Function to fetch model providers
	async function fetchModelProviders() {
		if (!project || !project.assistantID || !project.id) {
			return;
		}

		isLoading = true;
		try {
			const providers = await listModelProviders(project.assistantID, project.id);
			const names: Record<string, string> = {};

			if (providers && providers.items) {
				providers.items.forEach((provider) => {
					if (provider.id && provider.name) {
						names[provider.id] = provider.name;
					}
				});
			}

			providerNames = names;
			console.log('Model providers loaded:', providerNames);
		} catch (error) {
			console.error('Failed to load model providers:', error);
		} finally {
			isLoading = false;
		}
	}

	onMount(() => {
		if (project) {
			fetchModelProviders();
		}
	});

	$effect(() => {
		if (project) {
			fetchModelProviders();
		}
	});
</script>

<CollapsePane
	classes={{ header: 'pl-3 py-2 text-md', content: 'p-0' }}
	iconSize={5}
	header="Model Providers"
	helpText={HELPER_TEXTS.modelProviders}
>
	<div class="flex flex-col p-2">
		<p class="pb-2 text-xs">
			Agents use our included LLM by default. Connect other model providers to unlock more LLMs you
			can chat with.
		</p>

		{#if isLoading}
			<p class="text-xs italic">Loading model providers...</p>
		{:else if project?.models && Object.keys(project.models).length > 0}
			<div class="pb-2">
				<p class="mb-1 text-xs font-medium">Configured providers:</p>
				<ul class="space-y-0.5 pl-3 text-xs">
					{#each getSortedProviderIds(Object.keys(project.models)) as providerId}
						<li class="flex items-center gap-1">
							<span>{getProviderName(providerId)}</span>
							<span class="text-muted-foreground"
								>({project.models[providerId].length}
								{project.models[providerId].length === 1 ? 'model' : 'models'})</span
							>
						</li>
					{/each}
				</ul>
			</div>
		{:else}
			<p class="text-xs">No model providers configured</p>
		{/if}

		<div class="flex justify-end">
			<button class="button flex items-center gap-1" onclick={openModelProvidersConfig}>
				<span class="text-xs">Manage Model Providers</span>
			</button>
		</div>
	</div>
</CollapsePane>
