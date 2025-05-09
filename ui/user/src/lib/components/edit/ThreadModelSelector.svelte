<script lang="ts">
	import { onMount } from 'svelte';
	import { ChevronDown } from 'lucide-svelte';
	import type { Thread as ThreadType } from '$lib/services/chat/types';
	import type { Project } from '$lib/services';
	import { getThread, updateThread, getDefaultModelForThread } from '$lib/services/chat/operations';

	interface Props {
		threadId: string | undefined;
		project: Project;
		onModelChanged?: () => void;
	}

	let { threadId, project, onModelChanged }: Props = $props();

	let showModelSelector = $state(false);
	let threadDetails = $state<ThreadType | null>(null);
	let isUpdatingModel = $state(false);
	let modelSelectorRef = $state<HTMLDivElement>();
	let modelButtonRef = $state<HTMLButtonElement>();
	let defaultModel = $state<{ model: string; modelProvider: string } | null>(null);

	// Function to fetch thread details including model
	async function fetchThreadDetails() {
		if (!threadId) return;

		try {
			const thread = await getThread(project.assistantID, project.id, threadId);
			threadDetails = thread;

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
			defaultModel = await getDefaultModelForThread(project.assistantID, project.id, threadId);
		} catch (err) {
			console.error('Error fetching default model:', err);
			defaultModel = null;
		}
	}

	// Function to update thread model
	async function setThreadModel(model: string, provider: string) {
		if (!threadId || !threadDetails) return;

		// Prevent setting to empty if default model is empty
		if (!model && !provider && defaultModel?.model === '' && defaultModel?.modelProvider === '') {
			return;
		}

		isUpdatingModel = true;
		try {
			const updatedThread = await updateThread(project.assistantID, project.id, {
				...threadDetails,
				model: model || undefined,
				modelProvider: provider || undefined
			});

			// Update local state
			threadDetails = updatedThread;

			// If resetting to default, fetch the default model
			if (!model && !provider) {
				fetchDefaultModel();
			}

			// Close dropdown
			showModelSelector = false;

			// Notify parent that model changed
			if (onModelChanged) {
				onModelChanged();
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
		if (threadId) {
			fetchThreadDetails().then(() => {
				if (threadDetails && threadDetails.model && threadDetails.modelProvider) {
					// Make sure that the thread model is available on the project, and replace it with default if not.
					if (
						!project.models ||
						!project.models[threadDetails.modelProvider] ||
						!project.models[threadDetails.modelProvider].includes(threadDetails.model)
					) {
						setThreadModel('', '');
					}
				}
			});
		}
	});
</script>

<div class="relative pr-2">
	<button
		class="hover:bg-surface1 focus:ring-blue flex items-center gap-1 rounded-md border border-[--color-border] bg-transparent px-2 py-1 text-sm focus:ring-1 focus:outline-none"
		onclick={(e) => {
			e.stopPropagation();
			showModelSelector = !showModelSelector;
		}}
		onkeydown={(e) => e.key === 'Escape' && (showModelSelector = false)}
		aria-haspopup="listbox"
		aria-expanded={showModelSelector}
		id="thread-model-button"
		title="Select model for this thread"
		bind:this={modelButtonRef}
	>
		<span>
			{#if threadDetails?.modelProvider && threadDetails?.model}
				{threadDetails.model}
			{:else if defaultModel?.model && defaultModel.model !== ''}
				Default ({defaultModel.model})
			{:else if defaultModel?.model === '' && defaultModel?.modelProvider === ''}
				No Default Model
			{:else}
				Default Model
			{/if}
		</span>
		<ChevronDown class="h-4 w-4" />
	</button>

	{#if showModelSelector}
		<div
			role="listbox"
			tabindex="-1"
			aria-labelledby="thread-model-button"
			class="absolute bottom-full z-10 mb-1 max-h-60 w-60 overflow-auto rounded-md border border-[--color-border] bg-white p-1 shadow-lg dark:bg-black"
			onclick={(e) => e.stopPropagation()}
			onkeydown={(e) => {
				if (e.key === 'Escape') {
					showModelSelector = false;
					document.getElementById('thread-model-button')?.focus();
				}
			}}
			bind:this={modelSelectorRef}
		>
			<button
				role="option"
				aria-selected={!threadDetails?.modelProvider && !threadDetails?.model}
				class="hover:bg-surface1 focus:bg-surface1 w-full rounded px-2 py-1.5 text-left text-sm focus:outline-none"
				onclick={() => setThreadModel('', '')}
				tabindex="0"
				disabled={defaultModel?.model === '' && defaultModel?.modelProvider === ''}
				class:opacity-50={defaultModel?.model === '' && defaultModel?.modelProvider === ''}
			>
				Default
				{#if defaultModel?.model && defaultModel.model !== ''}
					<span class="text-xs text-gray-500">({defaultModel.model})</span>
				{:else if defaultModel?.model === '' && defaultModel?.modelProvider === ''}
					<span class="text-xs text-red-500">(No default model available)</span>
				{/if}
				{#if !threadDetails?.modelProvider && !threadDetails?.model}
					<span class="ml-1 text-xs text-green-500">✓</span>
				{/if}
			</button>

			{#each Object.entries(project.models || {}) as [providerId, models]}
				{#if Array.isArray(models) && models.length > 0 && providerId}
					{#each models as model}
						<button
							role="option"
							aria-selected={threadDetails?.modelProvider === providerId &&
								threadDetails?.model === model}
							class="hover:bg-surface1 focus:bg-surface1 w-full rounded px-2 py-1.5 text-left text-sm focus:outline-none"
							onclick={() => setThreadModel(model, providerId)}
							tabindex="0"
						>
							{model}
							{#if threadDetails?.modelProvider === providerId && threadDetails?.model === model}
								<span class="ml-1 text-xs text-green-500">✓</span>
							{/if}
						</button>
					{/each}
				{/if}
			{/each}

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
