<script lang="ts">
	import Layout from '$lib/components/Layout.svelte';
	import ProjectSidebar from './ProjectSidebar.svelte';
	import { ChatAPI, ChatService } from '$lib/services/nanobot/chat/index.svelte';
	import ThreadFromChat from '$lib/components/nanobot/ThreadFromChat.svelte';
	import * as nanobotLayout from '$lib/context/nanobotLayout.svelte';
	import { page } from '$app/state';
	import { untrack } from 'svelte';

	let { data } = $props();
	let agent = $derived(data.agent);

	const apiToken = import.meta.env.VITE_API_TOKEN;
	const headers: Record<string, string> = apiToken ? { Authorization: `Bearer ${apiToken}` } : {};
	const chatApi = $derived(new ChatAPI(agent.connectURL, { headers }));

	let chat = $state<ChatService | null>(null);
	let threadId = $derived(page.url.searchParams.get('tid'));
	let prevThreadId: string | null | undefined = undefined; // undefined = not yet initialized

	$effect(() => {
		const currentThreadId = threadId; // Track id dependency

		const shouldSkip = untrack(
			() => prevThreadId !== undefined && currentThreadId === prevThreadId
		);
		if (shouldSkip) return;

		untrack(() => {
			prevThreadId = currentThreadId;
			chat?.close();
		});

		const newChat = new ChatService({
			api: chatApi
		});

		if (currentThreadId) {
			newChat.restoreChat(currentThreadId);
		} else {
			newChat.newChat();
		}

		untrack(() => {
			chat = newChat;
		});

		return () => {
			newChat.close();
		};
	});
</script>

<Layout
	title=""
	layoutContext={nanobotLayout}
	classes={{
		container: 'px-0 py-0 md:px-0',
		childrenContainer: 'max-w-full '
	}}
>
	{#snippet overrideSidebarContent()}
		<ProjectSidebar {chatApi} />
	{/snippet}

	<div class="flex w-full grow">
		{#if chat}
			{#key chat.chatId}
				<ThreadFromChat
					{chat}
					onToggleSidebar={(open: boolean) => {
						nanobotLayout.getLayout().sidebarOpen = open;
					}}
				/>
			{/key}
		{/if}
	</div>
</Layout>

<svelte:head>
	<title>Nanobot</title>
</svelte:head>
