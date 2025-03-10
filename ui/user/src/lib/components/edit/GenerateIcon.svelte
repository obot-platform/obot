<script lang="ts">
	import { Wand, LoaderCircle, RefreshCw } from 'lucide-svelte/icons';
	import { EditorService } from '$lib/services';
	import type { Project } from '$lib/services';

	interface Props {
		project: Project;
	}

	let { project = $bindable() }: Props = $props();
	let isGenerating = $state(false);
	let hasGeneratedImage = $state(false);

	$effect(() => {
		hasGeneratedImage = !!project.icons?.icon?.startsWith('/api/generated/images/');
	});

	async function generateIcon(regenerate = false) {
		if (!project.description) return;

		isGenerating = true;
		try {
			const prompt = `Create a cool and unique sci-fi profile picture with upbeat colors and a family-friendly theme for an assistant with the following description: "${project.description}". Favor generating profile pictures like cute cartoon animals, plants, or other natural elements.`;
			const result = await EditorService.generateImage(prompt);

			if (result?.imageUrl) {
				project.icons = { icon: result.imageUrl, iconDark: result.imageUrl };
			}
		} catch (error) {
			console.error('Error generating image:', error);
		} finally {
			isGenerating = false;
		}
	}
</script>

<div class="mt-2 flex flex-col gap-2">
	<div class="flex gap-2">
		<button
			class="icon-button flex flex-1 items-center justify-center gap-2 py-2"
			onclick={() => generateIcon(false)}
			disabled={isGenerating || !project.description}
		>
			{#if isGenerating}
				<LoaderCircle class="h-5 w-5 animate-spin" />
				<span class="text-on-surface"> Generating icon... </span>
			{:else if hasGeneratedImage}
				<RefreshCw class="h-5 w-5" />
				<span class="text-on-surface"> Regenerate icon </span>
			{:else}
				<Wand class="h-5 w-5" />
				<span class="text-on-surface"> Generate icon from description </span>
			{/if}
		</button>
	</div>
</div>
