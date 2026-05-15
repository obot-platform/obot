<script lang="ts">
	import { autoHeight } from '$lib/actions/textarea';
	import Loading from '$lib/icons/Loading.svelte';
	import { EditorService } from '$lib/services';
	import type { Project } from '$lib/services';
	import { errors } from '$lib/stores';
	import { Wand, ChevronDown, ChevronUp } from 'lucide-svelte/icons';
	import { fade } from 'svelte/transition';

	interface Props {
		project: Project;
		class?: string;
	}

	const DEFAULT_PROMPT =
		'Based on the following description of a mascot: "{description}", draw an animated profile picture in a modern style with an upbeat and vibrant color palette.';

	let { project = $bindable() }: Props = $props();
	let isGenerating = $state(false);
	let isCustomPrompt = $state(false);
	let filteredImageError = $state<string | null>(null);
	let customPrompt = $state(
		project.description ? DEFAULT_PROMPT.replace('{description}', project.description) : ''
	);
	let prevDescription = $state(project.description || '');

	// Only update customPrompt when description changes and user hasn't made manual edits
	$effect(() => {
		const currentDescription = project.description || '';
		if (currentDescription !== prevDescription) {
			// Only update if the custom prompt still contains the previous description
			// or if it's the default prompt
			const currentPromptIsDefault =
				customPrompt === DEFAULT_PROMPT.replace('{description}', prevDescription);

			if (customPrompt === '' || currentPromptIsDefault) {
				customPrompt = DEFAULT_PROMPT.replace('{description}', currentDescription);
			}

			prevDescription = currentDescription;
		}
	});

	async function generateIcon(useCustomPrompt = false) {
		if (!project.description && !useCustomPrompt) return;

		isGenerating = true;
		filteredImageError = null; // Clear the error before each attempt
		try {
			const prompt = useCustomPrompt
				? customPrompt
				: DEFAULT_PROMPT.replace('{description}', project.description ?? '');
			const result = await EditorService.generateImage(prompt);

			if (result?.imageUrl) {
				project.icons = { icon: result.imageUrl, iconDark: undefined };
			}
		} catch (error) {
			if (
				error instanceof Error &&
				(error.message.includes('generated image was filtered') ||
					error.message.includes('images were filtered out'))
			) {
				filteredImageError =
					'The generated image was filtered due to content policy. Please try a different prompt.';
				isCustomPrompt = true;
			} else {
				errors.append(error);
			}
		} finally {
			isGenerating = false;
		}
	}

	function focusTextarea(node: HTMLTextAreaElement) {
		setTimeout(() => {
			node.focus();
		}, 0);
	}
</script>

<div class="relative mt-2 flex flex-col gap-2">
	<div class="border-base-400 flex rounded-lg border">
		<button
			class="btn btn-secondary border-base-400 dark:border-base-400 hover:bg-base-300 bg-base-100 flex flex-1 cursor-pointer items-center justify-center gap-2 rounded-l-lg rounded-r-none border-r"
			onclick={() => (isCustomPrompt ? generateIcon(true) : generateIcon())}
			disabled={isGenerating || (!project.description && !isCustomPrompt)}
		>
			{#if isGenerating}
				<Loading />
				<span class="text-muted-content">Generating icon...</span>
			{:else}
				<Wand class="h-5 w-5" />
				<span class="text-muted-content"
					>{isCustomPrompt ? 'Generate from custom prompt' : 'Generate from description'}</span
				>
			{/if}
		</button>
		<button
			class="btn btn-secondary hover:bg-base-300 dark:border-base-400 bg-base-100 flex items-center rounded-l-none rounded-r-lg px-2 hover:shadow-inner"
			onclick={() => (isCustomPrompt = !isCustomPrompt)}
			disabled={isGenerating}
		>
			{#if isCustomPrompt}
				<ChevronUp class="h-5 w-5" />
			{:else}
				<ChevronDown class="h-5 w-5" />
			{/if}
		</button>
	</div>
	{#if isCustomPrompt}
		<div in:fade class="border-base-400 flex flex-col gap-2 border-b pt-2 pb-4">
			<textarea
				bind:value={customPrompt}
				use:autoHeight
				use:focusTextarea
				placeholder="Enter custom prompt for image generation..."
				class="dark:border-base-400 bg-base-100 text-base-content w-full resize-none rounded-lg p-2 text-sm outline-hidden dark:border"
				rows="3"
			></textarea>
		</div>
	{/if}
	{#if filteredImageError}
		<div in:fade class="notification-error mt-2">
			<p class="text-sm">{filteredImageError}</p>
		</div>
	{/if}
</div>
