<script lang="ts">
	import Layout from '$lib/components/Layout.svelte';
	import ProjectSidebar from '../../ProjectSidebar.svelte';
	import { ChatAPI, ChatService } from '$lib/services/nanobot/chat/index.svelte';
	import * as nanobotLayout from '$lib/context/nanobotLayout.svelte';
	import { page } from '$app/state';
	import { get } from 'svelte/store';
	import { onMount, untrack } from 'svelte';
	import { nanobotChat } from '$lib/stores/nanobotChat.svelte';
	import { loadNanobotThreads } from '../../loadNanobotThreads';
	import FileEditor from '$lib/components/nanobot/FileEditor.svelte';
	import ThreadQuickAccess from '$lib/components/nanobot/ThreadQuickAccess.svelte';
	import { afterNavigate } from '$app/navigation';
	import { setContext } from 'svelte';
	import type { ChatMessageItemToolCall } from '$lib/services/nanobot/types';

	let { data, children } = $props();
	let agent = $derived(data.agent);
	let projectId = $derived(data.projectId);
	let workflowId = $derived(page.url.searchParams.get('wid') ?? undefined);

	const chatApi = $derived(new ChatAPI(agent.connectURL));

	let chat = $state<ChatService | null>(null);
	let threadId = $derived(page.url.searchParams.get('tid') ?? undefined);
	let prevThreadId: string | null | undefined = undefined;
	let initialQuickBarAccessOpen = $state(false);
	let selectedFile = $state(
		page.url.searchParams.get('wid') ? `workflow:///${page.url.searchParams.get('wid')}` : undefined
	);
	let threadContentWidth = $state(0);
	let needsRefreshThreads = $state(true);

	const layout = nanobotLayout.getLayout();

	const threadWriteToolItems = $derived.by((): ChatMessageItemToolCall[] => {
		const items: ChatMessageItemToolCall[] = [];
		if (chat?.messages?.length) {
			for (const message of chat.messages) {
				if (message.role !== 'assistant') continue;
				for (const item of message.items || []) {
					if (
						item.type === 'tool' &&
						(item.name === 'todoWrite' || item.name === 'write') &&
						item.arguments
					) {
						try {
							const args = JSON.parse(item.arguments);
							if (args.file_path) {
								items.push(item as ChatMessageItemToolCall);
							}
						} catch {
							console.error('Failed to parse tool call arguments', item);
						}
					}
				}
			}
		}
		if (workflowId) {
			items.push({
				type: 'tool',
				name: 'write',
				callID: `workflow-${workflowId}`,
				arguments: JSON.stringify({ file_path: `workflow:///${workflowId}` })
			} as ChatMessageItemToolCall);
		}
		return items;
	});

	function handleFileOpen(filename: string) {
		layout.quickBarAccessOpen = false;
		selectedFile = filename;
	}

	const projectLayoutContext = $state({
		chat: null as ChatService | null,
		threadWriteToolItems: [] as ChatMessageItemToolCall[],
		handleFileOpen,
		setThreadContentWidth: (w: number) => (threadContentWidth = w)
	});
	$effect(() => {
		projectLayoutContext.chat = chat;
		projectLayoutContext.threadWriteToolItems = threadWriteToolItems;
	});
	setContext('nanobot-project-layout', projectLayoutContext);

	onMount(() => {
		loadNanobotThreads(chatApi, projectId, threadId ?? undefined);
	});

	$effect(() => {
		if (initialQuickBarAccessOpen) return;
		if (chat && chat.messages.length > 0) {
			let foundTool = false;
			for (const message of chat.messages) {
				if (message.role !== 'assistant') continue;
				for (const item of message.items || []) {
					if (item.type === 'tool' && (item.name === 'todoWrite' || item.name === 'write')) {
						initialQuickBarAccessOpen = true;

						if (!layout.quickBarAccessOpen) {
							layout.quickBarAccessOpen = true;
						}

						foundTool = true;
						break;
					}
				}
				if (foundTool) break;
			}
		}
	});

	async function loadThreads() {
		const threads = await chatApi.getThreads();
		nanobotChat.update((data) => {
			if (data) {
				data.threads = threads ?? [];
			}
			return data;
		});
	}

	$effect(() => {
		if (chat && chat.messages.length >= 2 && needsRefreshThreads) {
			const tid = chat.chatId;
			const inThreads = $nanobotChat?.threads.find((t) => t.id === tid);
			if (!inThreads) {
				loadThreads();
			}

			needsRefreshThreads = false;
		}
	});

	$effect(() => {
		const currentThreadId = threadId;
		const currentProjectId = projectId;

		const shouldSkip = untrack(
			() => prevThreadId !== undefined && currentThreadId === prevThreadId
		);
		if (shouldSkip) return;

		const storedChat = get(nanobotChat);
		const sameProject = storedChat?.projectId === currentProjectId && storedChat?.chat;
		const threadMatches = storedChat?.threadId === currentThreadId;
		// Reuse stored chat when thread matches, or when we have no tid (e.g. on /workflows) so resources stay visible
		if (sameProject && (threadMatches || currentThreadId === undefined)) {
			untrack(() => {
				prevThreadId = currentThreadId;
				chat = storedChat!.chat!;
			});
			return () => {};
		}

		untrack(() => {
			prevThreadId = currentThreadId;
			chat?.close();
		});

		const newChat = new ChatService({
			api: chatApi
		});

		if (currentThreadId) {
			newChat.restoreChat(currentThreadId);
		}

		untrack(() => {
			chat = newChat;
			nanobotChat.update((data) => {
				if (data) {
					data.chat = newChat;
					data.threadId = currentThreadId ?? undefined;
				}
				return data;
			});
		});

		return () => {
			untrack(() => {
				if (chat !== newChat) {
					newChat.close();
				}
			});
		};
	});

	afterNavigate(() => {
		if (workflowId) {
			selectedFile = `workflow:///${workflowId}`;
		} else {
			selectedFile = '';
		}
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
		<ProjectSidebar {chatApi} selectedThreadId={threadId} {projectId} />
	{/snippet}

	<div
		class="flex w-full min-w-0 grow"
		style={threadContentWidth > 0 ? `min-width: ${threadContentWidth}px` : ''}
	>
		{@render children?.()}
	</div>

	{#snippet rightSidebar()}
		{#if chat}
			{#if selectedFile}
				<FileEditor
					filename={selectedFile}
					{chat}
					open={!!selectedFile}
					onClose={() => {
						selectedFile = '';
					}}
					quickBarAccessOpen={layout.quickBarAccessOpen}
					{threadContentWidth}
				/>
			{/if}

			<ThreadQuickAccess
				onToggle={() => (layout.quickBarAccessOpen = !layout.quickBarAccessOpen)}
				open={layout.quickBarAccessOpen}
				files={threadWriteToolItems}
			/>
		{/if}
	{/snippet}
</Layout>

<svelte:head>
	<title>Nanobot</title>
</svelte:head>
