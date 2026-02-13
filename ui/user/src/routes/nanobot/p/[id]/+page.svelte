<script lang="ts">
	import ProjectStartThread from '$lib/components/nanobot/ProjectStartThread.svelte';
	import { getContext } from 'svelte';

	let { data } = $props();
	let agent = $derived(data.agent);
	let projectId = $derived(data.projectId);

	const projectLayout = getContext<{
		chat: import('$lib/services/nanobot/chat/index.svelte').ChatService | null;
		handleFileOpen: (filename: string) => void;
		setThreadContentWidth: (w: number) => void;
	}>('nanobot-project-layout');
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
