<script lang="ts">
	import ProjectStartThread from '$lib/components/nanobot/ProjectStartThread.svelte';
	import { getContext } from 'svelte';
	import type { ProjectLayoutContext } from '$lib/services/nanobot/types';
	import { PROJECT_LAYOUT_CONTEXT } from '$lib/services/nanobot/types';
	import { page } from '$app/state';
	import { nanobotChat } from '$lib/stores/nanobotChat.svelte.js';

	let { data } = $props();
	let agent = $derived(data.agent);
	let projectId = $derived(data.projectId);
	let thread = $derived(
		$nanobotChat?.threads?.find((t) => t.id === page.url.searchParams.get('tid'))
	);

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

<svelte:head>
	<title>Obot | {thread?.title}</title>
</svelte:head>
