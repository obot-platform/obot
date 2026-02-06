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
	import { NanobotService } from '$lib/services';
	import { errors } from '$lib/stores';
	import { LoaderCircle } from 'lucide-svelte';

	let { data } = $props();
	let projects = $derived(data.projects);
	let agent = $derived(data.agent);
	let isNewAgent = $derived(data.isNewAgent);
	let chat = $state<ChatService | null>(null);
	let loading = $state(true);
	let sidebarRef: { refreshThreads: () => Promise<void> } | undefined = $state();

	const layout = nanobotLayout.getLayout();
	layout.sidebarOpen = false;

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
	});

	const chatApi = $derived(new ChatAPI(agent.connectURL));

	function handleThreadCreated(thread: Chat) {
		const projectId = projects[0].id;
		if (chat) {
			nanobotChat.set({ projectId, threadId: thread.id, chat });
		}
		goto(`/nanobot/p/${projectId}?tid=${thread.id}`, {
			replaceState: true,
			noScroll: true,
			keepFocus: true
		});

		sidebarRef?.refreshThreads();
		layout.sidebarOpen = true;
	}

	$effect(() => {
		const newChat = new ChatService({
			api: chatApi,
			onThreadCreated: handleThreadCreated
		});

		newChat.selectedAgentId = 'explorer';

		untrack(() => {
			chat = newChat;
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
		sidebar: 'pt-0'
	}}
	whiteBackground
>
	{#snippet overrideLeftSidebarContent()}
		<ProjectSidebar {chatApi} projectId={projects[0].id} bind:this={sidebarRef} />
	{/snippet}

	<div class="flex w-full grow">
		{#if chat && !loading}
			{#key chat.chatId}
				<ProjectStartThread agentId={agent.id} projectId={projects[0].id} {chat} />
			{/key}
		{:else}
			<LoaderCircle class="size-6 animate-spin" />
		{/if}
	</div>
</Layout>

<svelte:head>
	<title>Nanobot</title>
</svelte:head>
