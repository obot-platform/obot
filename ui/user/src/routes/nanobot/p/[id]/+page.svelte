<script lang="ts">
	import Layout from '$lib/components/Layout.svelte';
	import ProjectSidebar from '../../ProjectSidebar.svelte';
	import { ChatAPI, ChatService } from '$lib/services/nanobot/chat/index.svelte';
	import * as nanobotLayout from '$lib/context/nanobotLayout.svelte';
	import { page } from '$app/state';
	import { goto } from '$lib/url';
	import { get } from 'svelte/store';
	import { untrack } from 'svelte';
	import type { Chat } from '$lib/services/nanobot/types';
	import ProjectStartThread from '$lib/components/nanobot/ProjectStartThread.svelte';
	import { nanobotChat } from '$lib/stores/nanobotChat.svelte';
	import FileEditor from '$lib/components/nanobot/FileEditor.svelte';

	let { data } = $props();
	let agent = $derived(data.agent);
	let projectId = $derived(data.projectId);

	const chatApi = $derived(new ChatAPI(agent.connectURL));

	let chat = $state<ChatService | null>(null);
	let threadId = $derived(page.url.searchParams.get('tid'));
	let prevThreadId: string | null | undefined = undefined; // undefined = not yet initialized
	let sidebarRef: { refreshThreads: () => Promise<void> } | undefined = $state();
	let initialPlannerMode = $derived(page.url.searchParams.get('planner') === 'true');

	let selectedFile = $state('');
	let drawerInput = $state<HTMLInputElement | null>(null);

	// Get layout reference during component initialization (required for Svelte context API)
	const layout = nanobotLayout.getLayout();

	function handleThreadCreated(thread: Chat) {
		prevThreadId = thread.id;

		// Update URL with the new thread ID without navigation
		const url = new URL(page.url);
		url.searchParams.set('tid', thread.id);
		goto(url, { replaceState: true, noScroll: true, keepFocus: true });

		sidebarRef?.refreshThreads();
	}

	$effect(() => {
		const currentThreadId = threadId;
		const currentProjectId = projectId;

		const shouldSkip = untrack(
			() => prevThreadId !== undefined && currentThreadId === prevThreadId
		);
		if (shouldSkip) return;

		const storedChat = get(nanobotChat);
		if (
			storedChat &&
			storedChat.projectId === currentProjectId &&
			storedChat.threadId === currentThreadId
		) {
			nanobotChat.set(null);
			untrack(() => {
				prevThreadId = currentThreadId;
				chat = storedChat.chat;
			});
			return () => {};
		}

		untrack(() => {
			prevThreadId = currentThreadId;
			chat?.close();
		});

		const newChat = new ChatService({
			api: chatApi,
			onThreadCreated: handleThreadCreated
		});

		newChat.selectedAgentId = initialPlannerMode ? 'planner' : 'explorer';

		if (currentThreadId) {
			newChat.restoreChat(currentThreadId);
		}

		untrack(() => {
			chat = newChat;
		});

		return () => {
			untrack(() => {
				if (chat !== newChat) {
					newChat.close();
				}
			});
		};
	});
</script>

<Layout
	title=""
	layoutContext={nanobotLayout}
	classes={{
		container: 'px-0 py-0 md:px-0',
		childrenContainer: 'max-w-full h-[calc(100dvh-4rem)]',
		collapsedSidebarHeaderContent: 'pb-0',
		sidebar: 'pt-0'
	}}
	whiteBackground
>
	{#snippet overrideLeftSidebarContent()}
		<ProjectSidebar {chatApi} {projectId} bind:this={sidebarRef} />
	{/snippet}

	<div class="flex w-full grow">
		{#if chat}
			{#key chat}
				<ProjectStartThread
					agentId={agent.id}
					{chat}
					onFileOpen={(filename) => {
						layout.sidebarOpen = false;
						drawerInput?.click();
						selectedFile = filename;
					}}
					hasFileOpen={!!selectedFile}
				/>
			{/key}
		{/if}
	</div>

	{#snippet rightSidebar()}
		{#if chat && selectedFile}
			<FileEditor
				filename={selectedFile}
				{chat}
				onClose={() => {
					selectedFile = '';
					layout.sidebarOpen = true;
				}}
			/>
		{/if}
	{/snippet}
</Layout>

<svelte:head>
	<title>Nanobot</title>
</svelte:head>
