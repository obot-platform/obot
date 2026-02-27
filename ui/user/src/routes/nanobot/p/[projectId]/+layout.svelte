<script lang="ts">
	import Layout from '$lib/components/Layout.svelte';
	import ProjectSidebar from '../../ProjectSidebar.svelte';
	import { ChatSession } from '$lib/services/nanobot/chat/index.svelte';
	import * as nanobotLayout from '$lib/context/nanobotLayout.svelte';
	import { page } from '$app/state';
	import { setContext, untrack } from 'svelte';
	import { nanobotChat } from '$lib/stores/nanobotChat.svelte';
	import FileEditor from '$lib/components/nanobot/FileEditor.svelte';
	import QuickAccess from '$lib/components/nanobot/QuickAccess.svelte';
	import { afterNavigate } from '$app/navigation';
	import { PROJECT_LAYOUT_CONTEXT, type ProjectLayoutContext } from '$lib/services/nanobot/types';
	import { isRecent } from '$lib/time';

	let { data, children } = $props();
	let projectId = $derived(data.projects[0].id);
	let agentId = $derived(data.agent.id);
	let parentWorkflowId = $derived(
		(page.data as { workflowName?: string } | undefined)?.workflowName ??
			page.url.searchParams.get('pwid') ??
			undefined
	);
	let workflowId = $derived(page.url.searchParams.get('wid') ?? undefined);

	let chat = $state<ChatSession | null>(null);
	let sessionId = $derived(page.url.searchParams.get('tid') ?? undefined);
	let prevSessionId: string | null | undefined = undefined;
	let initialQuickBarAccessOpen = $state(false);
	let selectedFile = $state('');
	let threadContentWidth = $state(0);
	let needsRefreshThreads = $state(true);
	let titleRefreshTimeoutId: ReturnType<typeof setTimeout> | null = null;
	let layoutName = $state('');
	let showBackButton = $state(false);

	const layout = nanobotLayout.getLayout();
	const projectLayoutContext = $state<ProjectLayoutContext>({
		handleFileOpen,
		setThreadContentWidth: (w: number) => (threadContentWidth = w),
		setLayoutName: (name: string) => (layoutName = name),
		setShowBackButton: (show: boolean) => (showBackButton = show)
	});

	setContext(PROJECT_LAYOUT_CONTEXT, projectLayoutContext);

	function handleFileOpen(filename: string) {
		layout.quickBarAccessOpen = false;
		selectedFile = filename;
	}

	$effect(() => {
		if (parentWorkflowId || workflowId) {
			const workflow = $nanobotChat?.resources?.find((r) =>
				parentWorkflowId
					? r.uri === `workflow:///${parentWorkflowId}`
					: r.uri === `workflow:///${workflowId}`
			);
			const name = (workflow?._meta?.name as string) ?? workflow?.name ?? '';
			projectLayoutContext.setLayoutName(name);
			projectLayoutContext.setShowBackButton(true);
		} else {
			projectLayoutContext.setLayoutName('');
			projectLayoutContext.setShowBackButton(false);
		}
	});

	$effect(() => {
		if (initialQuickBarAccessOpen || !sessionId) return;
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

	async function updateThreads() {
		const tid = chat?.chatId;
		if (!tid) return;

		const sessions = (await $nanobotChat?.api.listSessions()) ?? [];
		const match = sessions.find((s) => s.id === tid);
		if (!match) return;

		nanobotChat.update((data) => {
			// just update/add the session if it doesn't exist in store
			if (data?.sessions) {
				const matchingIndex = data.sessions.findIndex((s) => s.id === tid);
				if (matchingIndex === -1) {
					data.sessions.unshift(match);
				} else {
					data.sessions[matchingIndex].title = match?.title ?? '';
				}
			}
			return data;
		});
	}

	$effect(() => {
		const clearTitleRefreshTimeout = () => {
			if (titleRefreshTimeoutId != null) {
				clearTimeout(titleRefreshTimeoutId);
				titleRefreshTimeoutId = null;
			}
		};

		if (!chat || chat.messages.length < 2) return clearTitleRefreshTimeout;

		const tid = chat.chatId;
		const sessions = $nanobotChat?.sessions ?? [];
		const inSessions = sessions.find((s) => s.id === tid);

		if (!inSessions) {
			if (needsRefreshThreads) {
				updateThreads();
				needsRefreshThreads = false;
			}
			return clearTitleRefreshTimeout;
		}

		const hasTitle =
			(inSessions.title && inSessions.title.trim().length > 0) || !isRecent(inSessions.created);
		if (!hasTitle) {
			if (titleRefreshTimeoutId == null) {
				titleRefreshTimeoutId = setTimeout(() => {
					titleRefreshTimeoutId = null;
					updateThreads();
				}, 5000); // 5 sec poll
			}
		} else {
			if (titleRefreshTimeoutId != null) {
				clearTimeout(titleRefreshTimeoutId);
				titleRefreshTimeoutId = null;
			}
		}

		if (needsRefreshThreads) {
			needsRefreshThreads = false;
		}

		return clearTitleRefreshTimeout;
	});

	$effect(() => {
		if (!$nanobotChat?.api) return;

		const currentSessionId = sessionId;
		const currentProjectId = projectId;

		const shouldSkip = untrack(
			() => prevSessionId !== undefined && currentSessionId === prevSessionId
		);
		if (shouldSkip) return;

		// Already showing the right session (e.g. restored from store or same thread) — don't replace with getSession
		if (chat?.chatId === currentSessionId) return;

		const storedChat = $nanobotChat;
		const sameProject = storedChat?.projectId === currentProjectId;
		const sessionMatches = storedChat?.sessionId === currentSessionId;
		if (sameProject && sessionMatches && !chat && storedChat?.chat) {
			chat = storedChat?.chat;
			return;
		}

		untrack(() => {
			prevSessionId = currentSessionId;
			chat?.close();
		});

		if (currentSessionId) {
			$nanobotChat.api.getSession(currentSessionId).then((existingSession) => {
				const nowTid = page.url.searchParams.get('tid') ?? undefined;
				if (nowTid !== currentSessionId) {
					existingSession.close();
					return;
				}
				chat = existingSession;
				nanobotChat.update((data) => {
					if (data) {
						data.chat = existingSession;
						data.sessionId = currentSessionId ?? undefined;
					}
					return data;
				});
			});
		}

		return () => {
			untrack(() => {
				const nextTid = sessionId;
				if (chat && nextTid !== chat.chatId) {
					chat.close();
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

		if (!sessionId) {
			initialQuickBarAccessOpen = false;
			layout.quickBarAccessOpen = false;
		}
	});
</script>

<Layout
	title={layoutName}
	layoutContext={nanobotLayout}
	classes={{
		container: 'px-0 py-0 md:px-0',
		childrenContainer: 'max-w-full h-[calc(100dvh-4rem)]',
		collapsedSidebarHeaderContent: 'pb-0',
		sidebar: 'pt-0 px-0',
		sidebarRoot: 'bg-base-200'
	}}
	{showBackButton}
	whiteBackground
	disableResize
	hideProfileButton
	alwaysShowHeaderTitle
>
	{#snippet leftSidebar()}
		<ProjectSidebar selectedSessionId={sessionId} {projectId} />
	{/snippet}

	<div
		class="flex w-full min-w-0 grow"
		style={threadContentWidth > 0 ? `min-width: ${threadContentWidth}px` : ''}
	>
		{@render children?.()}
	</div>

	{#snippet rightSidebar()}
		{#if selectedFile}
			<FileEditor
				filename={selectedFile}
				open={!!selectedFile}
				onClose={() => {
					selectedFile = '';
				}}
				quickBarAccessOpen={layout.quickBarAccessOpen}
				{threadContentWidth}
			/>
		{/if}
		<QuickAccess
			onToggle={() => (layout.quickBarAccessOpen = !layout.quickBarAccessOpen)}
			open={layout.quickBarAccessOpen}
			{sessionId}
			{selectedFile}
			{agentId}
			{projectId}
		/>
	{/snippet}
</Layout>
