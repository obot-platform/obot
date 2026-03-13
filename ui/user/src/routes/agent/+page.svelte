<script lang="ts">
	import Layout from '$lib/components/Layout.svelte';
	import * as nanobotLayout from '$lib/context/nanobotLayout.svelte';
	import ProjectSidebar from './ProjectSidebar.svelte';
	import ProjectStartThread from '$lib/components/nanobot/ProjectStartThread.svelte';
	import ThreadQuickAccess from '$lib/components/nanobot/QuickAccess.svelte';
	import AgentRestartBanner from '$lib/components/nanobot/AgentRestartBanner.svelte';
	import type { ChatSession } from '$lib/services/nanobot/chat/index.svelte';
	import { nanobotChat } from '$lib/stores/nanobotChat.svelte';
	import { goto } from '$lib/url';
	import type { Attachment, UploadedFile } from '$lib/services/nanobot/types';

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

	async function handleFileUpload(
		file: File,
		_opts?: { controller?: AbortController }
	): Promise<Attachment> {
		const id = crypto.randomUUID();
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
		<ProjectSidebar projectId={projects[0].id} />
	{/snippet}

	<div class="flex min-w-0 grow flex-col gap-3 p-3 md:p-4">
		<AgentRestartBanner agentId={agent.id} {projectId} />
		<div
			class="flex w-full min-w-0 grow"
			style={threadContentWidth > 0 ? `min-width: ${threadContentWidth}px` : ''}
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
							nanobotChat.update((data) => {
								if (data) {
									data.projectId = projectId;
									data.chat = session;
									data.sessionId = session.chatId;
								}
								return data;
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
									id: crypto.randomUUID(),
									role: 'user',
									created: new Date().toISOString(),
									items: [
										{
											id: crypto.randomUUID(),
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
				onThreadContentWidth={(w) => (threadContentWidth = w)}
			/>
		</div>
	</div>

	{#snippet rightSidebar()}
		<ThreadQuickAccess
			{projectId}
			agentId={agent.id}
			{browserAvailable}
			{browserViewerOpen}
			onToggleBrowserViewer={() => (browserViewerOpen = !browserViewerOpen)}
		/>
	{/snippet}
</Layout>

<svelte:head>
	<title>Obot | What would you like to work on?</title>
</svelte:head>
