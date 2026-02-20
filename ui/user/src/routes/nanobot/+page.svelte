<script lang="ts">
	import Layout from '$lib/components/Layout.svelte';
	import * as nanobotLayout from '$lib/context/nanobotLayout.svelte';
	import ProjectSidebar from './ProjectSidebar.svelte';
	import { ChatAPI, ChatService } from '$lib/services/nanobot/chat/index.svelte';
	import { onMount, untrack } from 'svelte';
	import ProjectStartThread from '$lib/components/nanobot/ProjectStartThread.svelte';
	import type { Chat } from '$lib/services/nanobot/types';
	import { goto } from '$lib/url';
	import { get } from 'svelte/store';
	import { nanobotChat } from '$lib/stores/nanobotChat.svelte';
	import { loadNanobotThreads } from './loadNanobotThreads';
	import { NanobotService } from '$lib/services';
	import { errors } from '$lib/stores';
	import { LoaderCircle } from 'lucide-svelte';
	import ThreadQuickAccess from '$lib/components/nanobot/ThreadQuickAccess.svelte';

	let { data } = $props();
	let projects = $derived(data.projects);
	let agent = $derived(data.agent);
	let isNewAgent = $derived(data.isNewAgent);
	let chat = $state<ChatService | null>(null);
	let loading = $state(true);
	let threadContentWidth = $state(0);

	const layout = nanobotLayout.getLayout();

	onMount(async () => {
		loading = true;
		if (isNewAgent) {
			try {
				await NanobotService.launchProjectV2Agent(projects[0].id, agent.id);
			} catch (error) {
				console.error(error);
				errors.append(error);
			} finally {
				loading = false;
			}
		} else {
			loading = false;
		}

		await loadNanobotThreads(chatApi, projects[0].id);
	});

	const chatApi = $derived(new ChatAPI(agent.connectURL));

	function handleThreadCreated(thread: Chat) {
		const projectId = projects[0].id;
		if (chat) {
			nanobotChat.update((data) => {
				if (data) {
					data.chat = chat!;
					data.threadId = thread.id;
				}
				return data;
			});
		}
		goto(`/nanobot/p/${projectId}?tid=${thread.id}`, {
			replaceState: true,
			noScroll: true,
			keepFocus: true
		});
	}

	$effect(() => {
		const newChat = new ChatService({
			api: chatApi,
			onThreadCreated: handleThreadCreated
		});

		newChat.selectedAgentId = 'explorer';

		untrack(() => {
			chat = newChat;
			// Sync chat into store so sidebar (Workflows, FileExplorer) can read resources
			const projectId = projects[0].id;
			nanobotChat.update((data) => {
				if (data) {
					data.chat = newChat;
					data.threadId = undefined;
				}
				return data;
			});
			// Store may still be null before loadNanobotThreads runs in onMount
			if (get(nanobotChat) === null) {
				nanobotChat.set({
					projectId,
					threadId: undefined,
					chat: newChat,
					threads: [],
					isThreadsLoading: true,
					resources: []
				});
			}
		});

		return () => {
			const storedChat = get(nanobotChat);
			if (storedChat?.chat === newChat) {
				return;
			}
			newChat.close();
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
		sidebar: 'pt-0 px-0',
		sidebarRoot: 'bg-base-200'
	}}
	whiteBackground
	disableResize
	hideProfileButton
>
	{#snippet leftSidebar()}
		<ProjectSidebar {chatApi} projectId={projects[0].id} />
	{/snippet}

	<div
		class="flex w-full min-w-0 grow"
		style={threadContentWidth > 0 ? `min-width: ${threadContentWidth}px` : ''}
	>
		{#if chat && !loading}
			{#key chat.chatId}
				<ProjectStartThread
					agentId={agent.id}
					projectId={projects[0].id}
					{chat}
					onThreadContentWidth={(w) => (threadContentWidth = w)}
				/>
			{/key}
		{:else}
			<LoaderCircle class="size-6 animate-spin" />
		{/if}
	</div>

	{#snippet rightSidebar()}
		{#if chat}
			<ThreadQuickAccess
				open={layout.quickBarAccessOpen}
				onToggle={() => (layout.quickBarAccessOpen = !layout.quickBarAccessOpen)}
			/>
		{/if}
	{/snippet}
</Layout>

<svelte:head>
	<title>Nanobot</title>
</svelte:head>
