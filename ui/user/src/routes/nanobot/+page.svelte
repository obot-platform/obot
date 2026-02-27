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
	import ThreadQuickAccess from '$lib/components/nanobot/QuickAccess.svelte';

	let { data } = $props();
	let projects = $derived(data.projects);
	let agent = $derived(data.agent);
	let isNewAgent = $derived(data.isNewAgent);
	let chat = $state<ChatService | null>(null);
	let loading = $state(true);
	let threadContentWidth = $state(0);

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
		const newChat = new ChatService(chatApi, {
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
			<div class="h-[calc(100dvh-4rem)] w-full px-4">
				<div class="absolute top-1/2 left-1/2 w-full -translate-x-1/2 -translate-y-1/2 md:w-4xl">
					<div class="flex flex-col items-center gap-4 px-5 pb-5 md:pb-0">
						<div class="flex w-full flex-col items-center gap-1">
							<div class="h-8 w-xs"></div>
							<p class="text-md skeleton skeleton-text text-center font-light">
								Just a moment, setting up your agent...
							</p>
						</div>
						<div class="flex w-full flex-col items-center justify-center gap-4 md:flex-row">
							<div class="rounded-field skeleton h-[132px] w-full md:w-70"></div>
							<div class="rounded-field skeleton h-[132px] w-full md:w-70"></div>
						</div>

						<div class="skeleton mb-6 h-[124px] w-full"></div>
					</div>
				</div>
			</div>
		{/if}
	</div>

	{#snippet rightSidebar()}
		{#if chat}
			<ThreadQuickAccess />
		{/if}
	{/snippet}
</Layout>

<svelte:head>
	<title>Obot | What would you like to work on?</title>
</svelte:head>
