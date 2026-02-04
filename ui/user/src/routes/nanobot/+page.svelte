<script lang="ts">
	import Layout from '$lib/components/Layout.svelte';
	import { ChatAPI, ChatService } from '$lib/services/nanobot/chat/index.svelte';
	import * as nanobotLayout from '$lib/context/nanobotLayout.svelte';
	import { page } from '$app/state';
	import { untrack } from 'svelte';
	import { MessageCircle, Plus, Sparkles } from 'lucide-svelte';
	import { goto } from '$app/navigation';

	const apiToken = import.meta.env.VITE_API_TOKEN;
	const headers: Record<string, string> = apiToken ? { Authorization: `Bearer ${apiToken}` } : {};
	const chatApi = new ChatAPI('/mcp-connect/default-nanobot-64d04a8bbvnzd', { headers });

	let chat = $state<ChatService | null>(null);
	let id = $derived(page.url.searchParams.get('id'));
	let prevId: string | null | undefined = undefined; // undefined = not yet initialized

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
	hideSidebar
>
	<div class="flex w-full grow items-center justify-center">
		<div class="flex w-full -translate-y-8 flex-col items-center gap-8 p-4 md:w-3xl">
			<div class="flex flex-col items-center gap-1">
				<h1 class="w-xs text-center text-3xl font-semibold md:w-full">
					What would you like to work on?
				</h1>
				<p class="text-base-content/50 text-md text-center font-light">
					Choose an entry point or pick up where you left off.
				</p>
			</div>
			<div class="grid grid-cols-1 items-stretch gap-4 md:grid-cols-2">
				<button
					class="bg-base-100 dark:bg-base-200 dark:border-base-300 rounded-field col-span-1 h-full p-4 text-left shadow-sm"
					onclick={() => {
						goto('/nanobot/p?planner=true');
					}}
				>
					<Sparkles class="mb-4 size-5" />
					<h3 class="text-lg font-semibold">Create a workflow</h3>
					<p class="text-base-content/50 text-sm font-light">
						Design an AI agent workflow through conversation
					</p>
				</button>
				<button
					class="bg-base-100 dark:bg-base-200 dark:border-base-300 rounded-field col-span-1 h-full p-4 text-left shadow-sm"
					onclick={() => {
						goto('/nanobot/p');
					}}
				>
					<MessageCircle class="mb-4 size-5" />
					<h3 class="text-lg font-semibold">Just explore</h3>
					<p class="text-base-content/50 min-h-[2lh] text-sm font-light">
						Chat and see where it goes
					</p>
				</button>
			</div>

			<div class="w-full">
				<div class="flex items-center justify-between gap-2">
					<h4 class="text-base-content/50 text-sm font-semibold">Recent Projects</h4>
					<button class="btn btn-primary">
						<Plus class="size-4" /> New Project
					</button>
				</div>
			</div>
		</div>
	</div>
</Layout>

<svelte:head>
	<title>Nanobot</title>
</svelte:head>
