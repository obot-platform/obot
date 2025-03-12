<script lang="ts">
	import { Wand, LoaderCircle, ChevronDown, ChevronUp } from 'lucide-svelte/icons';
	import { EditorService } from '$lib/services';
	import type { Project } from '$lib/services';
	import { fade } from 'svelte/transition';
	import { autoHeight } from '$lib/actions/textarea';

	interface Props {
		project: Project;
	}

	const DEFAULT_PROMPT =
		'Based on the following description of a mascot: "{description}", draw an animated profile picture in a modern style with an upbeat and vibrant color palette.';

	let { project = $bindable() }: Props = $props();
	let isGenerating = $state(false);
	let isCustomPrompt = $state(false);
	let customPrompt = $state(DEFAULT_PROMPT.replace('{description}', project.description ?? ''));

	async function generateIcon(useCustomPrompt = false) {
		if (!project.description && !useCustomPrompt) return;

		isGenerating = true;
		try {
			const prompt = useCustomPrompt
				? customPrompt
				: DEFAULT_PROMPT.replace('{description}', project.description ?? '');
			const result = await EditorService.generateImage(prompt);

			if (result?.imageUrl) {
				project.icons = { icon: result.imageUrl, iconDark: undefined };
			}
		} catch (error) {
			console.error('Error generating image:', error);
		} finally {
			isGenerating = false;
		}
	}
</script>

<div class="relative mt-2 flex flex-col gap-2">
	<div class="flex gap-2">
		<button
			class="icon-button flex flex-1 items-center justify-center gap-2 py-2"
			onclick={() => (isCustomPrompt ? generateIcon(true) : generateIcon())}
			disabled={isGenerating || (!project.description && !isCustomPrompt)}
		>
			{#if isGenerating}
				<LoaderCircle class="h-5 w-5 animate-spin" />
				<span class="text-on-surface">Generating icon...</span>
			{:else}
				<Wand class="h-5 w-5" />
				<span class="text-on-surface"
					>{isCustomPrompt ? 'Generate from custom prompt' : 'Generate from description'}</span
				>
			{/if}
		</button>
		<button
			class="icon-button flex items-center px-2"
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
		<div in:fade class="flex flex-col gap-2 rounded-lg bg-surface2 p-3">
			<textarea
				bind:value={customPrompt}
				use:autoHeight
				placeholder="Enter custom prompt for image generation..."
				class="w-full resize-none rounded-lg bg-white p-2 text-sm outline-none dark:bg-black dark:text-gray-50"
				rows="3"
			></textarea>
		</div>
	{/if}
</div>
