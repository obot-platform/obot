<script lang="ts">
	import { afterNavigate } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import Layout from '$lib/components/Layout.svelte';
	import FileEditor from '$lib/components/nanobot/FileEditor.svelte';
	import QuickAccess from '$lib/components/nanobot/QuickAccess.svelte';
	import Profile from '$lib/components/navbar/Profile.svelte';
	import * as nanobotLayout from '$lib/context/nanobotLayout.svelte';
	import { ChatSession } from '$lib/services/nanobot/chat/index.svelte';
	import { PROJECT_LAYOUT_CONTEXT, type ProjectLayoutContext } from '$lib/services/nanobot/types';
	import { responsive, profile } from '$lib/stores';
	import { nanobotChat } from '$lib/stores/nanobotChat.svelte';
	import { clampThreadContentReportedWidth } from '$lib/utils';
	import ImpersonateBanner from '../../ImpersonateBanner.svelte';
	import MobileDock from '../../MobileDock.svelte';
	import ProjectSidebar from '../../ProjectSidebar.svelte';
	import { Menu, MessageCirclePlus, X } from 'lucide-svelte';
	import { setContext, untrack } from 'svelte';
	import { onDestroy } from 'svelte';
	import { get } from 'svelte/store';
	import { twMerge } from 'tailwind-merge';

	let { data, children } = $props();
	let projectId = $derived(data.projects[0].id);
	let agentId = $derived(data.agent.id);
	let parentWorkflowId = $derived(
		(page.data as { workflowId?: string } | undefined)?.workflowId ??
			page.url.searchParams.get('pwid') ??
			undefined
	);
	let schedulerId = $derived(
		(page.data as { scheduleId?: string } | undefined)?.scheduleId ??
			page.url.searchParams.get('sid') ??
			undefined
	);
	let workflowId = $derived(page.url.searchParams.get('wid') ?? undefined);
	let activeWorkflowId = $derived(parentWorkflowId ?? workflowId);

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
		setThreadContentWidth: (w: number) => (threadContentWidth = clampThreadContentReportedWidth(w)),
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
		if (!schedulerId && !parentWorkflowId && !workflowId) {
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
		if (initialQuickBarAccessOpen || !sessionId || responsive.isMobile) return;
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
		const sameLiveSession =
			storedChat?.projectId === currentProjectId &&
			!!currentSessionId &&
			storedChat.chat?.chatId === currentSessionId;

		if (sameLiveSession && storedChat.chat) {
			const live = storedChat.chat;
			untrack(() => {
				if (chat && chat !== live) {
					chat.close();
				}
				chat = live;
			});
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
				const current = get(nanobotChat);
				if (
					current?.chat &&
					current.chat.chatId === currentSessionId &&
					current.chat !== existingSession
				) {
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
		} else if (storedChat?.chat) {
			const tidParam = page.url.searchParams.get('tid');
			if (tidParam && storedChat.chat.chatId === tidParam) {
				return;
			}
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
				if (chat && nextTid !== undefined && nextTid !== chat.chatId) {
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

	const impersonating = $derived(data.agent.userID !== profile.current.id);
</script>

<Layout
	title={layoutName}
	layoutContext={nanobotLayout}
	classes={{
		container: 'px-0 py-0 md:px-0',
		childrenContainer: `max-w-full ${impersonating ? 'h-[calc(100dvh-8rem)]' : 'h-[calc(100dvh-4rem)]'}`,
		collapsedSidebarHeaderContent: 'pb-0',
		sidebar: 'pt-0 px-0',
		sidebarRoot: 'bg-base-200',
		noSidebarTitle: 'md:text-xl text-base',
		navbar: impersonating ? 'top-15' : 'top-0'
	}}
	showBackButton={responsive.isMobile
		? showBackButton && !layout.quickBarAccessOpen
		: showBackButton}
	whiteBackground
	disableResize
	hideProfileButton
	alwaysShowHeaderTitle={responsive.isMobile ? !layout.quickBarAccessOpen : true}
	hideSidebar={responsive.isMobile}
>
	{#snippet leftMenu()}
		{#if responsive.isMobile}
			{#if layout.quickBarAccessOpen}
				<button
					class="btn btn-square btn-ghost"
					onclick={() => (layout.quickBarAccessOpen = false)}
				>
					<X class="text-base-content size-5" />
				</button>
			{:else if !activeWorkflowId}
				<a href={resolve('/agent')} class="btn btn-square">
					<MessageCirclePlus class="text-base-content size-5" />
				</a>
			{/if}
		{/if}
	{/snippet}
	{#snippet banner()}
		<ImpersonateBanner agent={data.agent} />
	{/snippet}
	{#snippet leftSidebar()}
		{#if !responsive.isMobile}
			<ProjectSidebar selectedSessionId={sessionId} {projectId} />
		{/if}
	{/snippet}

	<div
		class={twMerge('flex w-full min-w-0 grow', impersonating && !sessionId && 'pt-8')}
		class:pb-12={responsive.isMobile}
		style={threadContentWidth > 0 ? `min-width: min(${threadContentWidth}px, 100%)` : ''}
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
			onToggleBrowserViewer={() => (browserViewerOpen = !browserViewerOpen)}
			open={layout.quickBarAccessOpen}
			{browserAvailable}
			{browserViewerOpen}
			{sessionId}
			workflowId={activeWorkflowId}
			{selectedFile}
			{agentId}
			{projectId}
			{impersonating}
		/>
	{/snippet}

	{#snippet rightMenu()}
		{#if (sessionId || activeWorkflowId) && responsive.isMobile && !layout.quickBarAccessOpen}
			<button
				class="btn btn-square btn-ghost"
				onclick={() => (layout.quickBarAccessOpen = !layout.quickBarAccessOpen)}
			>
				<Menu class="text-base-content size-5" />
			</button>
		{:else if responsive.isMobile}
			<Profile {agentId} {projectId} />
		{/if}
	{/snippet}

	{#snippet mobileDock()}
		{#if responsive.isMobile}
			<MobileDock {projectId} />
		{/if}
	{/snippet}
</Layout>
