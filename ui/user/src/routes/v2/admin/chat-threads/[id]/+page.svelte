<script lang="ts">
	import Layout from '$lib/components/Layout.svelte';
	import BackLink from '$lib/components/admin/BackLink.svelte';
	import type { ProjectThread } from '$lib/services/admin/types';
	import { AdminService } from '$lib/services';

	import { LoaderCircle, ArrowLeft, MessageCircle } from 'lucide-svelte';
	import { onMount } from 'svelte';
	import { fly } from 'svelte/transition';
	import { goto } from '$app/navigation';
	import { formatTimeAgo } from '$lib/time';
	import MessageComponent from '$lib/components/messages/Message.svelte';
	import type { Project } from '$lib/services/chat/types';
	import type { OrgUser } from '$lib/services/admin/types';
	import { Thread } from '$lib/services/chat/thread.svelte';
	import type { Messages } from '$lib/services/chat/types';

	const { data } = $props();
	const threadId = data.threadId;

	let thread = $state<ProjectThread | null>(null);
	let messages = $state<Messages>({ messages: [], inProgress: false });
	let project = $state<Project | null>(null);
	let user = $state<OrgUser | null>(null);

	let loading = $state(true);
	let loadingMessages = $state(false);
	let error = $state<string | null>(null);
	let chatContainer = $state<HTMLDivElement>();

	onMount(() => {
		loadThread();
	});

	async function loadThread() {
		loading = true;
		error = null;
		try {
			const loadedThread = await AdminService.getThread(threadId);
			thread = loadedThread;

			if (loadedThread.userID) {
				try {
					user = await AdminService.getUser(loadedThread.userID);
				} catch (userErr) {
					console.error('Failed to load user data:', userErr);
				}
			}

			if (loadedThread.projectID) {
				project = await AdminService.getProject(loadedThread.projectID);
			}

			loadingMessages = true;
			try {
				await constructThread();
			} catch (msgErr) {
				console.error('Failed to load messages:', msgErr);
				messages = { messages: [], inProgress: false };
			} finally {
				loadingMessages = false;
			}
		} catch (err) {
			console.error('Failed to load thread:', err);
			error = 'Failed to load thread';
		} finally {
			loading = false;
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

		messages = {
			messages: [],
			inProgress: false
		};
		newThread.onMessages = (newMessages) => {
			messages = newMessages;
		};
	}

	function handleBack() {
		goto('/v2/admin/chat-threads');
	}

	function scrollToBottom() {
		if (chatContainer) {
			chatContainer.scrollTop = chatContainer.scrollHeight;
		}
	}

	$effect(() => {
		if (messages.messages.length > 0) {
			requestAnimationFrame(() => {
				scrollToBottom();
			});
		}
	});
</script>

<Layout>
	<div
		class="h-screen w-full"
		in:fly={{ x: 100, duration: 300, delay: 150 }}
		out:fly={{ x: -100, duration: 300 }}
	>
		<div class="flex h-full flex-col gap-6 p-4">
			<div class="flex items-center gap-4">
				<BackLink fromURL="chat-threads" currentLabel="Thread Details" />
			</div>

			{#if loading}
				<div class="flex w-full justify-center py-12">
					<LoaderCircle class="size-8 animate-spin text-blue-600" />
				</div>
			{:else if error}
				<div class="flex w-full flex-col items-center justify-center py-12 text-center">
					<div class="mb-4 text-red-500">
						<LoaderCircle class="size-16" />
					</div>
					<h3 class="text-lg font-semibold text-red-600">{error}</h3>
					<button
						onclick={handleBack}
						class="mt-4 flex items-center gap-2 rounded-md bg-blue-600 px-4 py-2 text-sm font-medium text-white hover:bg-blue-700"
					>
						<ArrowLeft class="size-4" />
						Back to Threads
					</button>
				</div>
			{:else if thread}
				<div class="flex min-h-0 flex-1 flex-col gap-6">
					<div
						class="rounded-lg border border-gray-200 bg-white p-6 dark:border-gray-700 dark:bg-gray-800"
					>
						<div class="mb-4 flex items-center justify-between">
							<h1 class="text-2xl font-semibold">
								{thread.name || 'Unnamed Thread'}
							</h1>
							<span class="inline-flex items-center rounded-full px-3 py-1 text-sm font-medium">
								{#if thread.taskID}
									<span
										class="bg-purple-100 text-purple-800 dark:bg-purple-900 dark:text-purple-200"
									>
										Task Run
									</span>
								{:else}
									<span class="bg-blue-100 text-blue-800 dark:bg-blue-900 dark:text-blue-200">
										Chat Thread
									</span>
								{/if}
							</span>
						</div>

						<div class="grid grid-cols-1 gap-4 text-sm md:grid-cols-2">
							<div>
								<span class="font-medium text-gray-700 dark:text-gray-300">Thread ID</span>
								<p class="font-mono text-gray-600 dark:text-gray-400">{thread.id}</p>
							</div>
							<div>
								<span class="font-medium text-gray-700 dark:text-gray-300">Created</span>
								<p class="text-gray-600 dark:text-gray-400">
									{formatTimeAgo(thread.created).relativeTime}
								</p>
							</div>
							{#if user}
								<div>
									<span class="font-medium text-gray-700 dark:text-gray-300">User</span>
									<p class="text-gray-600 dark:text-gray-400">
										{user.displayName || user.username}
									</p>
								</div>
								<div>
									<span class="font-medium text-gray-700 dark:text-gray-300">Email</span>
									<p class="text-gray-600 dark:text-gray-400">{user.email}</p>
								</div>
							{:else if thread.userID}
								<div>
									<span class="font-medium text-gray-700 dark:text-gray-300">User ID</span>
									<p class="font-mono text-gray-600 dark:text-gray-400">{thread.userID}</p>
								</div>
							{/if}
							{#if thread.projectID}
								<div>
									<span class="font-medium text-gray-700 dark:text-gray-300">Project ID</span>
									<p class="font-mono text-gray-600 dark:text-gray-400">{thread.projectID}</p>
								</div>
							{/if}
							{#if thread.assistantID}
								<div>
									<span class="font-medium text-gray-700 dark:text-gray-300">Assistant ID</span>
									<p class="font-mono text-gray-600 dark:text-gray-400">{thread.assistantID}</p>
								</div>
							{/if}
							{#if thread.taskID}
								<div>
									<span class="font-medium text-gray-700 dark:text-gray-300">Task ID</span>
									<p class="font-mono text-gray-600 dark:text-gray-400">{thread.taskID}</p>
								</div>
							{/if}
						</div>
					</div>

					<!-- Chat History Section -->
					<div
						class="mb-6 flex min-h-0 flex-col rounded-lg border border-gray-200 bg-white dark:border-gray-700 dark:bg-gray-800"
						style="height: 60vh;"
					>
						<div class="border-b border-gray-200 px-6 py-4 dark:border-gray-700">
							<h2 class="text-xl font-semibold">Chat History</h2>
						</div>

						<div
							class="default-scrollbar-thin min-h-0 flex-1 overflow-y-auto p-6"
							bind:this={chatContainer}
						>
							{#if loadingMessages}
								<div class="flex items-center justify-center py-8">
									<LoaderCircle class="size-6 animate-spin text-blue-600" />
									<span class="ml-2 text-sm text-gray-600">Loading messages...</span>
								</div>
							{:else if messages.messages.length > 0}
								<div class="space-y-4">
									{#each messages.messages as message, index (message.runID + index)}
										{#if project}
											<MessageComponent
												msg={message}
												{project}
												currentThreadID={threadId}
												disableMessageToEditor={true}
												noMemoryTool={true}
											/>
										{:else}
											<div
												class="flex gap-3 rounded-lg p-4 {message.sent
													? 'bg-blue-50 dark:bg-blue-900/20'
													: 'bg-gray-50 dark:bg-gray-900/20'}"
											>
												<div class="flex-shrink-0">
													{#if message.sent}
														<div
															class="flex h-8 w-8 items-center justify-center rounded-full bg-blue-500 text-sm font-medium text-white"
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
															{message.sent ? 'User' : message.sourceName || 'Assistant'}
														</span>
														{#if message.time}
															<span class="text-xs text-gray-500">
																{formatTimeAgo(message.time.toISOString())}
															</span>
														{/if}
													</div>
													<div class="text-sm text-gray-700 dark:text-gray-300">
														{#if message.message && message.message.length > 0}
															{#each message.message as msgPart, i (i)}
																<p class="mb-2 last:mb-0">{msgPart}</p>
															{/each}
														{:else}
															<span class="text-gray-500 italic">No message content</span>
														{/if}
													</div>
													{#if message.toolCall}
														<div class="mt-2 rounded bg-gray-100 p-2 text-xs dark:bg-gray-800">
															<strong>Tool Call:</strong>
															{message.toolCall.name || 'Unknown tool'}
														</div>
													{/if}
												</div>
											</div>
										{/if}
									{/each}
								</div>
							{:else if !loadingMessages}
								<div class="flex items-center justify-center py-12 text-center">
									<div class="text-gray-500">
										<MessageCircle class="mx-auto mb-4 size-16" />
										<h3 class="mb-2 text-lg font-medium">No Messages Found</h3>
										<p class="text-sm">This thread doesn't have any messages yet.</p>
									</div>
								</div>
							{/if}
						</div>
					</div>
				</div>
			{/if}
		</div>
	</div>
</Layout>

<svelte:head>
	<title>Obot | Admin - Thread {threadId}</title>
</svelte:head>
