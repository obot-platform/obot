<script lang="ts">
	import Logo from '$lib/components/Logo.svelte';
	import Threads from '$lib/components/nanobot/Threads.svelte';
	import { getLayout } from '$lib/context/nanobotLayout.svelte';
	import { ChatAPI } from '$lib/services/nanobot/chat/index.svelte';
	import { nanobotChat } from '$lib/stores/nanobotChat.svelte';
	import { goto } from '$lib/url';
	import {
		Folders,
		FoldersIcon,
		Plus,
		SidebarClose,
		SidebarOpen,
		Workflow,
		WorkflowIcon
	} from 'lucide-svelte';
	import { get } from 'svelte/store';
	import { twMerge } from 'tailwind-merge';
	import { fly, slide } from 'svelte/transition';
	import { resolve } from '$app/paths';

	interface Props {
		chatApi: ChatAPI;
		selectedThreadId?: string;
		projectId: string;
	}

	let { chatApi, selectedThreadId, projectId }: Props = $props();

	const layout = getLayout();
	async function handleRenameThread(threadId: string, newTitle: string) {
		try {
			await chatApi.renameThread(threadId, newTitle);
			const sharedChat = get(nanobotChat);
			const threadIndex = sharedChat?.threads.findIndex((t) => t.id === threadId) ?? -1;
			if (threadIndex !== -1 && sharedChat) {
				nanobotChat.update((data) => {
					if (data && threadIndex !== -1) {
						data.threads[threadIndex].title = newTitle;
					}
					return data;
				});
			}
		} catch (error) {
			console.error('Failed to rename thread:', error);
		}
	}

	async function handleDeleteThread(threadId: string) {
		const sharedChat = get(nanobotChat);
		const isCurrentViewedThread = selectedThreadId === threadId;
		try {
			await chatApi.deleteThread(threadId);
			if (sharedChat) {
				nanobotChat.update((data) => {
					if (data) {
						data.threads = data.threads.filter((t) => t.id !== threadId);
						if (data.threadId === threadId) {
							data.threadId = undefined;

							if (data.chat) {
								data.chat.close();
								data.chat = undefined;
							}
						}
					}
					return data;
				});
			}

			if (isCurrentViewedThread) {
				goto(`/nanobot`, { replaceState: true });
			}
		} catch (error) {
			console.error('Failed to delete thread:', error);
		}
	}

	function handleCreateThread() {
		nanobotChat.update((data) => {
			if (data) {
				data.threadId = undefined;
			}
			return data;
		});
		goto(`/nanobot`);
	}

	function toggleSidebar() {
		layout.sidebarOpen = !layout.sidebarOpen;
	}
</script>

<div
	class={twMerge(
		'bg-base-200 z-10 h-[100dvh] w-18 min-w-18 flex-shrink-0 overflow-visible',
		layout.sidebarOpen && 'w-[300px] md:min-w-[300px]'
	)}
>
	<div class="flex h-full w-full min-w-full flex-col gap-4 pt-1">
		<div class="flex min-h-0 flex-1 flex-col">
			<div class="flex w-fit gap-1 p-4 pt-2">
				<Logo />
				{#if layout.sidebarOpen}
					<span in:slide={{ axis: 'x', duration: 150 }} class="self-end text-2xl font-semibold"
						>workflows</span
					>
				{/if}
			</div>
			{#if layout.sidebarOpen}
				<div
					class="flex min-h-0 min-w-0 grow flex-col gap-4 overflow-x-hidden overflow-y-auto"
					in:fly={{ x: -100, duration: 150 }}
				>
					<a
						href={resolve(`/nanobot/p/${projectId}/workflows`)}
						class="btn btn-ghost text-base-content/50 text-md justify-between rounded-none"
					>
						Workflows <WorkflowIcon class="size-6" />
					</a>

					<a
						href={resolve(`/nanobot/p/${projectId}/files`)}
						class="btn btn-ghost text-base-content/50 text-md justify-between rounded-none"
					>
						Files <FoldersIcon class="size-6" />
					</a>

					<Threads
						threads={$nanobotChat?.threads ?? []}
						onRename={handleRenameThread}
						onDelete={handleDeleteThread}
						onCreateThread={handleCreateThread}
						isLoading={$nanobotChat?.isThreadsLoading ?? false}
						{selectedThreadId}
					/>
				</div>
			{:else}
				<div class="flex flex-shrink-0 flex-col items-center justify-center gap-4 pb-3">
					<div class="w-fit">
						<button
							class="btn btn-ghost btn-circle tooltip tooltip-right size-10 self-center"
							aria-label="Go to workflows"
							data-tip="Go to workflows"
							onclick={() => goto(`/nanobot/p/${projectId}/workflows`)}
						>
							<Workflow class="text-base-content/50 size-6" />
						</button>
					</div>
					<div class="w-fit">
						<button
							class="btn btn-ghost btn-circle tooltip tooltip-right size-10 self-center"
							aria-label="Go to files"
							data-tip="Go to files"
							onclick={() => goto(`/nanobot/p/${projectId}/files`)}
						>
							<Folders class="text-base-content/50 size-6" />
						</button>
					</div>
					<div class="w-fit">
						<button
							class="btn btn-ghost btn-circle tooltip tooltip-right size-10 self-center"
							aria-label="Start new conversation"
							data-tip="Start new conversation"
							onclick={handleCreateThread}
						>
							<Plus class="text-base-content/50 size-6" />
						</button>
					</div>
				</div>
				<div class="flex grow"></div>
			{/if}

			<div
				class="bg-base-200 sticky bottom-0 flex flex-shrink-0 justify-end overflow-visible pr-4 pb-3"
			>
				<button
					class={twMerge(
						'btn btn-ghost btn-circle tooltip size-10 self-center',
						layout.sidebarOpen ? 'tooltip-left' : 'tooltip-right'
					)}
					aria-label={layout.sidebarOpen ? 'Collapse sidebar' : 'Expand sidebar'}
					data-tip={layout.sidebarOpen ? 'Collapse sidebar' : 'Expand sidebar'}
					onclick={toggleSidebar}
				>
					{#if layout.sidebarOpen}
						<SidebarClose class="text-base-content/50 size-6" />
					{:else}
						<SidebarOpen class="text-base-content/50 size-6" />
					{/if}
				</button>
			</div>
		</div>
	</div>
</div>
