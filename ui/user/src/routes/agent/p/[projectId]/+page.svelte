<script lang="ts">
	import ProjectStartThread from '$lib/components/nanobot/ProjectStartThread.svelte';
	import { getContext } from 'svelte';
	import type { ProjectLayoutContext } from '$lib/services/nanobot/types';
	import { PROJECT_LAYOUT_CONTEXT } from '$lib/services/nanobot/types';
	import { page } from '$app/state';
	import { nanobotChat } from '$lib/stores/nanobotChat.svelte';

	let { data } = $props();
	let agent = $derived(data.agent);
	let projectId = $derived(data.projects[0].id);
	let tid = $derived(page.url.searchParams.get('tid'));
	let session = $derived($nanobotChat?.sessions?.find((s) => s.id === tid));
	let browserBaseUrl = $derived(data.agent.connectURL);

	const projectLayout = getContext<ProjectLayoutContext>(PROJECT_LAYOUT_CONTEXT);

	let displayChat = $derived($nanobotChat?.chat);
</script>

{#if displayChat}
	{#key displayChat}
		<ProjectStartThread
			agentId={agent.id}
			{projectId}
			{browserBaseUrl}
			browserAvailable={projectLayout.browserAvailable}
			bind:browserViewerOpen={projectLayout.browserViewerOpen}
			chat={displayChat}
			onFileOpen={projectLayout.handleFileOpen}
			suppressEmptyState
			onThreadContentWidth={projectLayout.setThreadContentWidth}
		/>
	{/key}
{/if}

<svelte:head>
	<title>Obot | {session?.title || 'Untitled'}</title>
</svelte:head>
