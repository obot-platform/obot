<script lang="ts">
	import Layout from '$lib/components/Layout.svelte';
	import NanobotSidebar from './NanobotSidebar.svelte';
	import { ChatAPI, ChatService } from '$lib/services/nanobot/chat/index.svelte';
	import ThreadFromChat from '$lib/components/nanobot/ThreadFromChat.svelte';
	import * as nanobotLayout from '$lib/context/nanobotLayout.svelte';
	import './nanobot.css';
	import { page } from '$app/state';
	import { onMount, untrack } from 'svelte';
	import { browser } from '$app/environment';
	import { darkMode } from '$lib/stores';

	const apiToken = import.meta.env.VITE_API_TOKEN;
	const headers: Record<string, string> = apiToken ? { Authorization: `Bearer ${apiToken}` } : {};
	const chatApi = new ChatAPI('/mcp-connect/default-nanobot-64d04a8bbvnzd', { headers });

	let chat = $state<ChatService | null>(null);
	let id = $derived(page.url.searchParams.get('id'));
	let prevId: string | null | undefined = undefined; // undefined = not yet initialized

	$effect(() => {
		const scheme = darkMode.isDark ? 'nanobotdark' : 'nanobotlight';
		document.documentElement.setAttribute('data-theme', scheme);
	});

	$effect(() => {
		const currentId = id; // Track id dependency

		const shouldSkip = untrack(() => prevId !== undefined && currentId === prevId);
		if (shouldSkip) return;

		untrack(() => {
			prevId = currentId;
			chat?.close();
		});

		const newChat = new ChatService({
			api: chatApi
		});

		if (currentId) {
			newChat.restoreChat(currentId);
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
		<NanobotSidebar {chatApi} />
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
