<script lang="ts">
	import ProjectStartThread from '$lib/components/nanobot/ProjectStartThread.svelte';
	import { getContext } from 'svelte';
	import type { ProjectLayoutContext } from '$lib/services/nanobot/types';
	import { PROJECT_LAYOUT_CONTEXT } from '$lib/services/nanobot/types';

	let { data } = $props();
	let agent = $derived(data.agent);
	let projectId = $derived(data.projectId);

	const projectLayout = getContext<ProjectLayoutContext>(PROJECT_LAYOUT_CONTEXT);
</script>

{#if projectLayout.chat}
	{#key projectLayout.chat}
		<ProjectStartThread
			agentId={agent.id}
			{projectId}
			chat={projectLayout.chat}
			onFileOpen={projectLayout.handleFileOpen}
			suppressEmptyState
			onThreadContentWidth={projectLayout.setThreadContentWidth}
		/>
	{/key}
{/if}
