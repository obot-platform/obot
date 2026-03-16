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
	import AgentRestartBanner from '$lib/components/nanobot/AgentRestartBanner.svelte';
	import { afterNavigate } from '$app/navigation';
	import { onDestroy } from 'svelte';
	import { PROJECT_LAYOUT_CONTEXT, type ProjectLayoutContext } from '$lib/services/nanobot/types';

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
	let layoutName = $state('');
	let showBackButton = $state(false);
	let browserViewerOpen = $state(false);
	let browserAvailable = $state(false);
	/** Session ID we already tried to refresh for when it was missing (avoids loop if listSessions never returns it). */
	let refreshedForMissingSessionId: string | null = null;
	let titleInterval: ReturnType<typeof setInterval> | null = null;
	let titleIntervalAttempts = 0;
	const MAX_TITLE_INTERVAL_ATTEMPTS = 5;

	const layout = nanobotLayout.getLayout();
	const projectLayoutContext = $state<ProjectLayoutContext>({
		handleFileOpen,
		setThreadContentWidth: (w: number) => (threadContentWidth = w),
		setLayoutName: (name: string) => (layoutName = name),
		setShowBackButton: (show: boolean) => (showBackButton = show),
		get browserAvailable() {
			return browserAvailable;
		},
		setBrowserAvailable: (show: boolean) => (browserAvailable = show),
		get browserViewerOpen() {
			return browserViewerOpen;
		},
		setBrowserViewerOpen: (show: boolean) => (browserViewerOpen = show)
	});

	setContext(PROJECT_LAYOUT_CONTEXT, projectLayoutContext);

	onDestroy(() => {
		if (titleInterval) {
			clearInterval(titleInterval);
			titleInterval = null;
		}
		chat?.close();
		nanobotChat.update((data) => {
			if (data) {
				data.chat = undefined;
				data.sessionId = undefined;
			}
			return data;
		});
	});

	function handleFileOpen(filename: string) {
		layout.quickBarAccessOpen = false;
		selectedFile = filename;
	}

	function browserStatusUrl(baseUrl: string) {
		const trimmedBaseUrl = baseUrl.replace(/\/$/, '');
		const url = new URL(`${trimmedBaseUrl}/browser/status`);
		return url.toString();
	}

	$effect(() => {
		if (!data.agent.connectURL || typeof EventSource === 'undefined') {
			return;
		}

		const eventSource = new EventSource(browserStatusUrl(data.agent.connectURL));
		const handleStatus = (event: Event) => {
			try {
				const payload = JSON.parse((event as MessageEvent<string>).data) as { available?: boolean };
				browserAvailable = !!payload.available;
				if (!browserAvailable) {
					browserViewerOpen = false;
				}
			} catch {
				// Ignore malformed status events and keep the last known state.
			}
		};

		eventSource.addEventListener('status', handleStatus);

		return () => {
			eventSource.removeEventListener('status', handleStatus);
			eventSource.close();
		};
	});

	$effect(() => {
		if (parentWorkflowId || workflowId) {
			const workflow = $nanobotChat?.resources?.find((r) =>
				parentWorkflowId
					? r.uri === `workflow:///${parentWorkflowId}`
					: r.uri === `workflow:///${workflowId}`
			);
			const name =
				(workflow?._meta?.displayName as string) ??
				(workflow?._meta?.name as string) ??
				workflow?.name ??
				'';
			projectLayoutContext.setLayoutName(name);
			projectLayoutContext.setShowBackButton(true);
		} else {
			projectLayoutContext.setLayoutName('');
			projectLayoutContext.setShowBackButton(false);
		}
	});

	$effect(() => {
		const api = $nanobotChat?.api;
		const sessions = $nanobotChat?.sessions ?? [];
		const sid = sessionId;
		const c = chat;
		if (!api || !sid || !c) return;
		if (sessions.some((s) => s.id === sid)) {
			refreshedForMissingSessionId = null;
			return;
		}
		if (refreshedForMissingSessionId === sid) return;
		if (titleInterval) {
			clearInterval(titleInterval);
			titleInterval = null;
			titleIntervalAttempts = 0;
		}
		refreshedForMissingSessionId = sid;
		api.listSessions().then((list) => {
			const matchingSession = list.find((s) => s.id === sid);
			if (!matchingSession) return;
			if (!matchingSession.title) {
				titleInterval = setInterval(() => {
					if (titleIntervalAttempts > MAX_TITLE_INTERVAL_ATTEMPTS && titleInterval) {
						clearInterval(titleInterval);
						titleInterval = null;
						titleIntervalAttempts = 0;
						return;
					}
					api.listSessions().then((list) => {
						titleIntervalAttempts++;
						const matchingSession = list.find((s) => s.id === sid);
						if (!matchingSession) return;
						if (matchingSession.title) {
							if (titleInterval) clearInterval(titleInterval);
							titleInterval = null;
							titleIntervalAttempts = 0;
							nanobotChat.update((data) => {
								if (data) data.sessions = list ?? [];
								return data;
							});
						}
					});
				}, 5000);
			}
			nanobotChat.update((data) => {
				if (data) data.sessions = list ?? [];
				return data;
			});
		});
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
			chat = null;
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
		} else {
			nanobotChat.update((data) => {
				if (data) {
					data.chat = undefined;
					data.sessionId = undefined;
				}
				return data;
			});
		}

		return () => {
			untrack(() => {
				const nextTid = sessionId;
				if (chat && nextTid !== chat.chatId) {
					chat.close();
					chat = null;
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

	<div class="flex min-w-0 grow flex-col gap-3 p-3 md:p-4">
		<AgentRestartBanner {agentId} {projectId} />
		<div
			class="flex w-full min-w-0 grow"
			style={threadContentWidth > 0 ? `min-width: ${threadContentWidth}px` : ''}
		>
			{@render children?.()}
		</div>
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
			onToggleBrowserViewer={() => (browserViewerOpen = !browserViewerOpen)}
			open={layout.quickBarAccessOpen}
			{browserAvailable}
			{browserViewerOpen}
			{sessionId}
			{selectedFile}
			{agentId}
			{projectId}
		/>
	{/snippet}
</Layout>
