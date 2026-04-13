<script lang="ts">
	import Layout from '$lib/components/Layout.svelte';
	import ProjectStartThread from '$lib/components/nanobot/ProjectStartThread.svelte';
	import QuickAccess from '$lib/components/nanobot/QuickAccess.svelte';
	import Profile from '$lib/components/navbar/Profile.svelte';
	import * as nanobotLayout from '$lib/context/nanobotLayout.svelte';
	import type { ChatSession } from '$lib/services/nanobot/chat/index.svelte';
	import type { Attachment, UploadedFile } from '$lib/services/nanobot/types';
	import { responsive, profile } from '$lib/stores';
	import { nanobotChat } from '$lib/stores/nanobotChat.svelte';
	import { goto } from '$lib/url';
	import { clampThreadContentReportedWidth, randomUUID } from '$lib/utils';
	import ImpersonateBanner from './ImpersonateBanner.svelte';
	import MobileDock from './MobileDock.svelte';
	import ProjectSidebar from './ProjectSidebar.svelte';
	import { get } from 'svelte/store';

	let { data } = $props();
	let projects = $derived(data.projects);
	let agent = $derived(data.agent);
	let projectId = $derived(projects[0]?.id ?? '');
	let threadContentWidth = $state(0);
	let initialMessage = $state<string | undefined>(undefined);
	let pendingFiles = $state<UploadedFile[]>([]);
	let browserViewerOpen = $state(false);
	let browserAvailable = $state(false);

	function browserStatusUrl(baseUrl: string) {
		const trimmedBaseUrl = baseUrl.replace(/\/$/, '');
		const url = new URL(`${trimmedBaseUrl}/browser/status`);
		return url.toString();
	}

	function cancelPendingUpload(fileId: string) {
		const entry = pendingFiles.find((f) => f.id === fileId);
		if (entry?.uri?.startsWith('blob:')) {
			URL.revokeObjectURL(entry.uri);
		}
		pendingFiles = pendingFiles.filter((f) => f.id !== fileId);
	}

	function handleThreadContentWidth(w: number) {
		threadContentWidth = clampThreadContentReportedWidth(w);
	}

	async function handleFileUpload(
		file: File,
		_opts?: { controller?: AbortController }
	): Promise<Attachment> {
		const id = randomUUID();
		const uri = URL.createObjectURL(file);
		pendingFiles = [...pendingFiles, { id, file, uri, mimeType: file.type || undefined }];
		return { name: file.name, uri, mimeType: file.type || undefined };
	}

	$effect(() => {
		if (!agent.connectURL || typeof EventSource === 'undefined') {
			return;
		}

		const eventSource = new EventSource(browserStatusUrl(agent.connectURL));
		const handleStatus = (event: Event) => {
			try {
				const data = JSON.parse((event as MessageEvent<string>).data) as { available?: boolean };
				browserAvailable = !!data.available;
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

	const impersonating = $derived(data.agent.userID !== profile.current.id);
</script>

<Layout
	title=""
	layoutContext={nanobotLayout}
	classes={{
		container: 'px-0 py-0 md:px-0',
		childrenContainer: `max-w-full ${impersonating ? 'h-[calc(100dvh-8rem)]' : 'h-[calc(100dvh-4rem)]'}`,
		collapsedSidebarHeaderContent: 'pb-0',
		sidebar: 'pt-0 px-0',
		sidebarRoot: 'bg-base-200',
		navbar: impersonating ? 'top-15' : 'top-0'
	}}
	whiteBackground
	disableResize
	hideProfileButton={!responsive.isMobile}
	hideSidebar={responsive.isMobile}
>
	{#snippet banner()}
		<ImpersonateBanner {agent} />
	{/snippet}
	{#snippet leftSidebar()}
		{#if !responsive.isMobile}
			<ProjectSidebar projectId={projects[0].id} />
		{/if}
	{/snippet}

	<div
		class="flex w-full min-w-0 grow"
		style={threadContentWidth > 0 ? `min-width: min(${threadContentWidth}px, 100%)` : ''}
	>
		<ProjectStartThread
			agentId={agent.id}
			{projectId}
			browserBaseUrl={agent.connectURL}
			{browserAvailable}
			bind:browserViewerOpen
			chat={{
				sendMessage: async (message: string, attachments?: Attachment[]) => {
					initialMessage = message;
					const toUpload = [...pendingFiles];
					pendingFiles = [];
					toUpload.forEach((p) => {
						if (p.uri?.startsWith('blob:')) URL.revokeObjectURL(p.uri);
					});

					$nanobotChat?.api.createSession().then(async (session) => {
						const uploadedAttachments: Attachment[] = await Promise.all(
							toUpload.map((p) => session.uploadFile(p.file))
						);
						const allAttachments = [...uploadedAttachments, ...(attachments ?? [])];
						session.sendMessage(message, allAttachments.length > 0 ? allAttachments : undefined);
						const current = get(nanobotChat);
						nanobotChat.set({
							projectId,
							chat: session,
							sessionId: session.chatId,
							api: $nanobotChat?.api,
							sessions: current?.sessions ?? [],
							isThreadsLoading: current?.isThreadsLoading ?? false,
							resources: current?.resources ?? []
						});

						goto(`/agent/p/${projectId}?tid=${session.chatId}`, {
							replaceState: true,
							noScroll: true,
							keepFocus: true
						});
					});
				},
				messages: initialMessage
					? [
							{
								id: randomUUID(),
								role: 'user',
								created: new Date().toISOString(),
								items: [
									{
										id: randomUUID(),
										type: 'text',
										text: initialMessage
									}
								]
							}
						]
					: [],
				prompts: [],
				resources: [],
				agent: undefined,
				agents: [],
				selectedAgentId: '',
				elicitations: [],
				isLoading: false,
				isRestoring: false,
				chatId: '',
				uploadFile: handleFileUpload,
				uploadedFiles: pendingFiles,
				uploadingFiles: [],
				cancelUpload: cancelPendingUpload,
				sessionClient: undefined,
				closer: () => {},
				history: [],
				onChatDone: [],
				currentRequestId: undefined,
				subscribed: false
			} as unknown as ChatSession}
			onThreadContentWidth={handleThreadContentWidth}
			classes={{ root: impersonating ? 'h-[calc(100dvh-8rem)]' : 'h-[calc(100dvh-4rem)]' }}
		/>
	</div>

	{#snippet rightSidebar()}
		<QuickAccess
			{projectId}
			agentId={agent.id}
			{browserAvailable}
			{browserViewerOpen}
			onToggleBrowserViewer={() => (browserViewerOpen = !browserViewerOpen)}
			{impersonating}
		/>
	{/snippet}

	{#snippet rightMenu()}
		{#if responsive.isMobile}
			<Profile agentId={agent.id} {projectId} />
		{/if}
	{/snippet}

	{#snippet mobileDock()}
		{#if responsive.isMobile}
			<MobileDock {projectId} />
		{/if}
	{/snippet}
</Layout>

<svelte:head>
	<title>Obot | What would you like to work on?</title>
</svelte:head>
