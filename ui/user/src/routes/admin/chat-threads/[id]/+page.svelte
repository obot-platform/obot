<script lang="ts">
	import Layout from '$lib/components/Layout.svelte';
	import { AdminService, type ProjectThread } from '$lib/services';

	import { LoaderCircle, MessageCircle } from 'lucide-svelte';
	import { onMount, onDestroy } from 'svelte';
	import { fly, fade } from 'svelte/transition';
	import { page } from '$app/stores';
	import { formatTimeAgo } from '$lib/time';
	import MessageComponent from '$lib/components/messages/Message.svelte';
	import type { CallFrame, Project } from '$lib/services/chat/types';
	import { Thread } from '$lib/services/chat/thread.svelte';
	import type { Messages } from '$lib/services/chat/types';
	import DebugCallFrames from '$lib/components/chat/DebugCallFrames.svelte';

	const threadId = $page.params.id;

	let messages = $state<Messages>({ messages: [], inProgress: false });
	let project = $state<Project | null>(null);
	let currentThread = $state<Thread | null>(null);
	let currentThreadData = $state<ProjectThread>();

	let loadingMessages = $state(true);
	let threadContainer = $state<HTMLDivElement>();
	let loadingOlderMessages = $state(false);
	let showLoadOlderButton = $state(false);

	let debugDialog = $state<ReturnType<typeof DebugCallFrames>>();
	let debugInformation = $state<{ runId: string; frames: Record<string, CallFrame> }>();

	onMount(() => {
		loadThread();
	});

	onDestroy(() => {
		if (currentThread) {
			currentThread.close();
		}
	});

	async function loadThread() {
		try {
			currentThreadData = await AdminService.getThread(threadId);

			if (currentThreadData.projectID) {
				project = await AdminService.getProject(currentThreadData.projectID);
			}

			try {
				await constructThread();
			} catch (msgErr) {
				console.error('Failed to load messages:', msgErr);
				messages = { messages: [], inProgress: false };
			} finally {
				// impose a delay to avoid flickering
				setTimeout(() => {
					loadingMessages = false;
				}, 1000);
			}
		} catch (err) {
			console.error('Failed to load thread:', err);
		}
	}

	async function constructThread() {
		if (!project) return;

		const newThread = new Thread(project, {
			threadID: threadId,
			onError: () => {},
			onClose: () => {
				return false;
			},
			items: [],
			onItemsChanged: (_items) => {}
		});

		currentThread = newThread;

		messages = {
			messages: [],
			inProgress: false
		};
		newThread.onMessages = (newMessages) => {
			messages = newMessages;
			loadingMessages = false;
		};
	}

	async function loadOlderMessages() {
		if (!messages.lastRunID || !messages.messages.length || loadingOlderMessages) return;

		// Use the parentRunID from the messages object if available
		const previousRunID = messages.parentRunID;
		if (!previousRunID) {
			// No older messages, bail out
			return;
		}

		loadingOlderMessages = true;

		// Store current scroll position to anchor the view when older messages are loaded
		const scrollTop = threadContainer?.scrollTop || 0;
		const scrollHeight = threadContainer?.scrollHeight || 0;

		try {
			// Load older messages
			const oldThread = new Thread(project!, {
				threadID: threadId,
				runID: previousRunID,
				follow: false,
				onError: () => {
					// Ignore errors
				}
			});

			// Wait for the thread to load the previous messages
			const prevMessages = await new Promise<Messages>((resolve) => {
				let resolved = false;
				oldThread.onMessages = (newMessages) => {
					if (oldThread.replayComplete && !resolved) {
						resolved = true;
						resolve(newMessages);
					}
				};

				// Set a timeout in case replayComplete is never triggered
				setTimeout(() => {
					if (!resolved) {
						resolved = true;
						resolve({ messages: [], inProgress: false });
					}
				}, 10000);
			});

			// Close the temporary thread
			oldThread.close();

			// Merge the previous messages with the current ones
			if (prevMessages.messages.length > 0) {
				const existingRunIDs = new Set(messages.messages.map((msg) => msg.runID));
				const newMessages = prevMessages.messages.filter((msg) => !existingRunIDs.has(msg.runID));

				// Update messages
				messages = {
					...messages,
					parentRunID: prevMessages.parentRunID,
					messages: [...newMessages, ...messages.messages]
				};

				// After the DOM updates, adjust the scroll position based on the actual height change
				requestAnimationFrame(() => {
					if (threadContainer) {
						const newScrollHeight = threadContainer.scrollHeight;
						const addedHeight = newScrollHeight - scrollHeight;
						threadContainer.scrollTop = scrollTop + addedHeight;
					}
				});
			} else {
				messages = {
					...messages,
					parentRunID: undefined
				};
			}
		} catch (error) {
			console.error('Error loading older messages:', error);
			messages = {
				...messages,
				parentRunID: undefined
			};
		} finally {
			loadingOlderMessages = false;
		}
	}

	$effect(() => {
		// Only update if messages change
		const messages_copy = messages; // Create a local reference

		if (messages_copy.messages.length === 0) {
			if (showLoadOlderButton) showLoadOlderButton = false;
			return;
		}

		const shouldShow = !!messages_copy.parentRunID;

		// Only update state if it needs to change
		if (shouldShow !== showLoadOlderButton) {
			showLoadOlderButton = shouldShow;
		}

		// Auto-scroll to bottom when new messages are loaded
		requestAnimationFrame(() => {
			if (threadContainer) {
				threadContainer.scrollTop = threadContainer.scrollHeight;
			}
		});
	});

	let title = $derived(`Admin - Thread ${threadId}`);
</script>

<Layout {title} whiteBackground={true} showBackButton>
	<div
		class="h-dvh w-full"
		in:fly={{ x: 100, duration: 300, delay: 150 }}
		out:fly={{ x: -100, duration: 300 }}
	>
		<div class="flex h-full flex-col">
			<div class="flex w-full grow justify-center" bind:this={threadContainer}>
				<div class="relative flex w-full max-w-[900px] flex-col">
					{#if messages.messages.length > 0}
						<div
							in:fade|global
							class="flex w-full grow flex-col justify-start gap-8 p-5 transition-all"
						>
							{#if showLoadOlderButton}
								<div class="mb-4 flex justify-center">
									<button
										class="border-surface3 hover:bg-surface2 bg-background rounded-full border px-4 py-2 text-sm font-light transition-all duration-300"
										onclick={loadOlderMessages}
										disabled={loadingOlderMessages}
									>
										{#if loadingOlderMessages}
											<div
												class="inline-block h-4 w-4 animate-spin rounded-full border-2 border-current border-t-transparent"
												role="status"
											>
												<span class="sr-only">Loading...</span>
											</div>
											<span class="ml-2">Loading...</span>
										{:else}
											Load older messages
										{/if}
									</button>
								</div>
							{/if}

							{#each messages.messages as msg, i (i)}
								{#if project}
									<MessageComponent
										{project}
										{msg}
										currentThreadID={threadId}
										disableMessageToEditor={true}
										noMemoryTool={true}
										onDebugMessageRun={async (runId, frames) => {
											debugInformation = { runId, frames };
											debugDialog?.open();
										}}
									/>
								{:else}
									<div
										class="flex gap-3 rounded-lg p-4 {msg.sent
											? 'bg-primary/10'
											: 'bg-gray-50 dark:bg-gray-900/20'}"
									>
										<div class="flex-shrink-0">
											{#if msg.sent}
												<div
													class="bg-primary flex h-8 w-8 items-center justify-center rounded-full text-sm font-medium text-white"
												>
													U
												</div>
											{:else}
												<div
													class="flex h-8 w-8 items-center justify-center rounded-full bg-gray-500 text-sm font-medium text-white"
												>
													A
												</div>
											{/if}
										</div>
										<div class="min-w-0 flex-1">
											<div class="mb-2 flex items-center gap-2">
												<span class="text-sm font-medium">
													{msg.sent ? 'User' : msg.sourceName || 'Assistant'}
												</span>
												{#if msg.time}
													<span class="text-on-surface1 text-xs">
														{formatTimeAgo(msg.time.toISOString())}
													</span>
												{/if}
											</div>
											<div class="text-sm text-gray-700 dark:text-gray-300">
												{#if msg.message && msg.message.length > 0}
													{#each msg.message as msgPart, i (i)}
														<p class="mb-2 last:mb-0">{msgPart}</p>
													{/each}
												{:else}
													<span class="text-on-surface1 italic">No message content</span>
												{/if}
											</div>
											{#if msg.toolCall}
												<div class="mt-2 rounded bg-gray-100 p-2 text-xs dark:bg-gray-800">
													<strong>Tool Call:</strong>
													{msg.toolCall.name || 'Unknown tool'}
												</div>
											{/if}
										</div>
									</div>
								{/if}
							{/each}
							<div class="min-h-4">
								<!-- Vertical Spacer -->
							</div>
						</div>
					{:else if loadingMessages}
						<div class="flex items-center justify-center py-12 text-center">
							<div class="text-on-surface1">
								<LoaderCircle class="text-primary mx-auto mb-4 size-8 animate-spin" />
								<h3 class="mb-2 text-lg font-medium">Loading Messages...</h3>
								<p class="text-sm">Please wait while we load the thread messages.</p>
							</div>
						</div>
					{:else}
						<div class="flex items-center justify-center py-12 text-center">
							<div class="text-on-surface1">
								<MessageCircle class="mx-auto mb-4 size-16" />
								<h3 class="mb-2 text-lg font-medium">No Messages Found</h3>
								<p class="text-sm">This thread doesn't have any messages yet.</p>
							</div>
						</div>
					{/if}
				</div>
			</div>
		</div>
	</div>
</Layout>

<DebugCallFrames
	bind:this={debugDialog}
	calls={debugInformation?.frames}
	runId={debugInformation?.runId}
/>

<svelte:head>
	<title>Obot | {title}</title>
</svelte:head>
