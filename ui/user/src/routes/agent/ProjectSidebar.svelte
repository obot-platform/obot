<script lang="ts">
	import { page } from '$app/state';
	import Logo from '$lib/components/Logo.svelte';
	import Threads from '$lib/components/nanobot/Threads.svelte';
	import { getLayout } from '$lib/context/nanobotLayout.svelte';
	import { errors } from '$lib/stores';
	import { nanobotChat } from '$lib/stores/nanobotChat.svelte';
	import { goto } from '$lib/url';
	import {
		Clock3,
		Folders,
		FoldersIcon,
		Plus,
		SidebarClose,
		SidebarOpen,
		Workflow,
		WorkflowIcon
	} from 'lucide-svelte';
	import { fly, slide } from 'svelte/transition';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		selectedSessionId?: string;
		projectId: string;
	}

	let { selectedSessionId, projectId }: Props = $props();
	let activeView = $derived(page.url.pathname.split('/').pop());

	const layout = getLayout();
	async function handleRenameSession(sessionId: string, newTitle: string) {
		if (!$nanobotChat?.api) {
			errors.append(new Error('Nanobot API not found'));
			return;
		}
		try {
			await $nanobotChat.api.renameSession(sessionId, newTitle);
			nanobotChat.update((data) => {
				if (!data) return data;
				const sessionIndex = data.sessions.findIndex((s) => s.id === sessionId);
				if (sessionIndex === -1) return data;
				return {
					...data,
					sessions: data.sessions.map((s, i) =>
						i === sessionIndex ? { ...s, title: newTitle } : s
					)
				};
			});
		} catch (error) {
			console.error('Failed to rename thread:', error);
		}
	}

	async function handleDeleteSession(sessionId: string) {
		if (!$nanobotChat?.api) {
			errors.append(new Error('Nanobot API not found'));
			return;
		}
		const isCurrentViewedSession = selectedSessionId === sessionId;
		try {
			await $nanobotChat.api.deleteSession(sessionId);
			nanobotChat.update((data) => {
				if (data) {
					data.sessions = data.sessions.filter((s) => s.id !== sessionId);
					if (data.sessionId === sessionId) {
						data.sessionId = undefined;

						if (data.chat) {
							data.chat.close();
							data.chat = undefined;
						}
					}
				}
				return data;
			});

			if (isCurrentViewedSession) {
				goto(`/agent?projectId=${projectId}`, { replaceState: true });
			}
		} catch (error) {
			console.error('Failed to delete thread:', error);
		}
	}

	function handleCreateSession() {
		nanobotChat.update((data) => {
			if (data) {
				data.sessionId = undefined;
			}
			return data;
		});
		goto(`/agent?projectId=${projectId}`);
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
					<div
						in:slide={{ axis: 'x', duration: 150 }}
						class="flex items-center gap-1 self-end text-2xl font-semibold"
					>
						obot
						<span class="border-primary text-primary rounded-full border-2 px-2 text-sm">
							agent
						</span>
					</div>
				{/if}
			</div>
			{#if layout.sidebarOpen}
				<div
					class="flex min-h-0 min-w-0 grow flex-col gap-4 overflow-x-hidden overflow-y-auto"
					in:fly={{ x: -100, duration: 150 }}
				>
					<button
						type="button"
						class="btn btn-ghost text-base-content/50 text-md justify-between rounded-none"
						onclick={() => goto(`/agent/p/${projectId}/workflows`)}
						class:bg-base-100={activeView === 'workflows'}
					>
						Workflows <WorkflowIcon class="size-6" />
					</button>

					<button
						type="button"
						class="btn btn-ghost text-base-content/50 text-md justify-between rounded-none"
						onclick={() => goto(`/agent/p/${projectId}/scheduler`)}
						class:bg-base-100={activeView === 'scheduler'}
					>
						Scheduler <Clock3 class="size-6" />
					</button>

					<button
						type="button"
						class="btn btn-ghost text-base-content/50 text-md justify-between rounded-none"
						onclick={() => goto(`/agent/p/${projectId}/files`)}
						class:bg-base-100={activeView === 'files'}
					>
						Files <FoldersIcon class="size-6" />
					</button>

					<Threads
						sessions={$nanobotChat?.sessions ?? []}
						onRename={handleRenameSession}
						onDelete={handleDeleteSession}
						onCreateSession={handleCreateSession}
						isLoading={$nanobotChat?.isThreadsLoading ?? false}
						{selectedSessionId}
					/>
				</div>
			{:else}
				<div class="flex flex-shrink-0 flex-col items-center justify-center gap-4 pb-3">
					<div class="w-fit">
						<button
							class="btn btn-ghost btn-circle tooltip tooltip-right size-10 self-center"
							aria-label="Go to workflows"
							data-tip="Go to workflows"
							onclick={() => goto(`/agent/p/${projectId}/workflows`)}
						>
							<Workflow
								class={twMerge(
									'size-6',
									activeView === 'workflows' ? 'text-primary' : 'text-base-content/50'
								)}
							/>
						</button>
					</div>
					<div class="w-fit">
						<button
							class="btn btn-ghost btn-circle tooltip tooltip-right size-10 self-center"
							aria-label="Go to scheduler"
							data-tip="Go to scheduler"
							onclick={() => goto(`/agent/p/${projectId}/scheduler`)}
						>
							<Clock3
								class={twMerge(
									'size-6',
									activeView === 'scheduler' ? 'text-primary' : 'text-base-content/50'
								)}
							/>
						</button>
					</div>
					<div class="w-fit">
						<button
							class="btn btn-ghost btn-circle tooltip tooltip-right size-10 self-center"
							aria-label="Go to files"
							data-tip="Go to files"
							onclick={() => goto(`/agent/p/${projectId}/files`)}
						>
							<Folders
								class={twMerge(
									'size-6',
									activeView === 'files' ? 'text-primary' : 'text-base-content/50'
								)}
							/>
						</button>
					</div>
					<div class="w-fit">
						<button
							class="btn btn-ghost btn-circle tooltip tooltip-right size-10 self-center"
							aria-label="Start new conversation"
							data-tip="Start new conversation"
							onclick={handleCreateSession}
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
