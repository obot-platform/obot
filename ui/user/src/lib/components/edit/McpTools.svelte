<script lang="ts">
	import CollapsePane from '$lib/components/edit/CollapsePane.svelte';
	import { HELPER_TEXTS } from '$lib/context/helperMode.svelte';
	import { getLayout, openMCPServerTools } from '$lib/context/layout.svelte';
	import { getProjectMCPs } from '$lib/context/projectMcps.svelte';
	import { type Project } from '$lib/services';
	import { ChevronRight, Server } from 'lucide-svelte';

	interface Props {
		project: Project;
	}

	let { project = $bindable() }: Props = $props();

	const projectMCPs = getProjectMCPs();
	const layout = getLayout();
</script>

<CollapsePane
	classes={{ header: 'pl-3 py-2 text-md', content: 'p-0' }}
	iconSize={5}
	header="Tools"
	helpText={HELPER_TEXTS.prompt}
>
	<div class="flex flex-col p-2">
		{#each projectMCPs.items as projectMcp}
			<button
				class="hover:bg-surface3 flex min-h-9 items-center justify-between rounded-md bg-transparent p-2 pr-3 text-xs transition-colors duration-200"
				onclick={() => {
					openMCPServerTools(layout, projectMcp);
				}}
			>
				<span class="flex items-center gap-2">
					{#if projectMcp.icon}
						<div class="bg-surface1 flex-shrink-0 rounded-sm p-1 dark:bg-gray-600">
							<img src={projectMcp.icon} class="size-4" alt={projectMcp.name} />
						</div>
					{:else}
						<div class="bg-surface1 flex-shrink-0 rounded-sm p-1 dark:bg-gray-600">
							<Server class="size-4" />
						</div>
					{/if}

					{projectMcp.name}
				</span>
				<ChevronRight class="size-4" />
			</button>
		{/each}
	</div>
</CollapsePane>
