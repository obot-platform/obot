<script lang="ts">
	import { ChatService, type ProjectShare, type ToolReference } from '$lib/services';
	import FeaturedAgentCard from '$lib/components/agents/FeaturedAgentCard.svelte';
	import { sortByFeaturedNameOrder } from '$lib/sort';
	import { goto } from '$app/navigation';

	interface Props {
		shares: ProjectShare[];
		tools: ToolReference[];
	}

	let { shares, tools: referencedTools }: Props = $props();

	let tools = $derived(new Map(referencedTools.map((t) => [t.id, t])));
	let featured = $derived(shares.sort(sortByFeaturedNameOrder));

	async function makeCopyFromShare(project: ProjectShare) {
		// TEMPORARY HACK TO FAKE COPY
		const response = await ChatService.createProjectFromShare(project.publicID);
		const copy = await ChatService.copyProject(response.assistantID, response.id);
		await ChatService.deleteProject(response.assistantID, response.id);
		await goto(`/o/${copy.id}`);
	}
</script>

<div class="flex flex-col items-center justify-center gap-4">
	<h2 class="px-12 text-2xl font-semibold md:text-4xl">Agent Catalog</h2>
	<p class="mb-4 max-w-full px-4 text-center text-sm font-light md:max-w-xl md:px-12 md:text-base">
		Check out our agents below to find the perfect one for you. Or if you're feeling adventurous,
		get started and create your own obot!
	</p>
	{#if featured.length > 0}
		<div class="mb-4 flex w-full flex-col items-center justify-center px-4 md:px-12">
			<div class="grid grid-cols-1 gap-4 md:grid-cols-2 lg:grid-cols-3">
				{#each featured.slice(0, 4) as featuredShare}
					<FeaturedAgentCard
						project={featuredShare}
						{tools}
						onclick={() => makeCopyFromShare(featuredShare)}
					/>
				{/each}
			</div>
		</div>
	{/if}
</div>
