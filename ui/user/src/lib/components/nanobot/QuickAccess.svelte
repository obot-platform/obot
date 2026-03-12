<script lang="ts">
	import {
		Circle,
		CheckCircle2,
		Loader2,
		ChevronUp,
		ChevronDown,
		Monitor,
		SidebarOpen,
		SidebarClose,
		ListCheck
	} from 'lucide-svelte';
	import { twMerge } from 'tailwind-merge';
	import Profile from '../navbar/Profile.svelte';
	import { fly } from 'svelte/transition';
	import { nanobotChat } from '$lib/stores/nanobotChat.svelte';
	import { getContext } from 'svelte';
	import type { ProjectLayoutContext } from '$lib/services/nanobot/types';
	import { PROJECT_LAYOUT_CONTEXT } from '$lib/services/nanobot/types';
	import FileItem from '$lib/components/nanobot/FileItem.svelte';
	import { parseJSON } from '$lib/services/nanobot/utils';

	interface Props {
		onToggle?: () => void;
		onToggleBrowserViewer?: () => void;
		open?: boolean;
		browserViewerOpen?: boolean;
		sessionId?: string;
		selectedFile?: string;
		agentId?: string;
		projectId?: string;
	}

	let {
		onToggle,
		onToggleBrowserViewer,
		open,
		browserViewerOpen = false,
		sessionId,
		selectedFile,
		agentId,
		projectId
	}: Props = $props();

	/** Todo item shape from todo:///list resource or todo_write tool (application/json) */
	interface TodoItem {
		content: string;
		status: 'pending' | 'in_progress' | 'completed' | 'cancelled';
		activeForm?: string;
	}

	const projectLayout = getContext<ProjectLayoutContext>(PROJECT_LAYOUT_CONTEXT);
	let todoItems = $state<TodoItem[]>([]);

	const chatForSession = $derived(
		$nanobotChat?.chat?.chatId === sessionId ? ($nanobotChat?.chat ?? null) : null
	);

	const files = $derived(
		chatForSession?.resources?.filter((r) => r.uri.startsWith('file:///')) ?? []
	);

	$effect(() => {
		const chat = chatForSession;
		if (!sessionId || !chat) {
			return;
		}
		const unwatch = chat.watchResource('todo:///list', (resource) => {
			todoItems = parseJSON<TodoItem[]>(resource.text) ?? [];
		});
		chat.readResource('todo:///list').then((resource) => {
			todoItems = parseJSON<TodoItem[]>(resource.contents[0].text) ?? [];
		});
		return () => {
			unwatch();
		};
	});

	let showTodoList = $state(true);
</script>

<div
	class={twMerge(
		'bg-base-100 border-base-300 h-[100dvh] w-18 min-w-18 border-l ',
		open ? 'w-sm min-w-sm overflow-y-auto' : 'overflow-y-visible'
	)}
>
	<div
		class={twMerge(
			'flex h-full w-full min-w-0 flex-col gap-4 pt-1',
			open ? 'p-4 pt-1' : 'pt-1 pb-4'
		)}
	>
		<div class={twMerge(open ? 'self-end' : 'w-14 self-center')}>
			<Profile {agentId} {projectId} />
		</div>

		{#if onToggleBrowserViewer}
			{#if open}
				<button
					class={twMerge(
						'btn justify-start gap-2 self-stretch rounded-xl px-4',
						browserViewerOpen ? 'btn-neutral' : 'btn-ghost border-base-300 border'
					)}
					onclick={onToggleBrowserViewer}
				>
					<Monitor class="size-4 shrink-0" />
					Browser
				</button>
			{:else}
				<button
					class={twMerge(
						'btn btn-circle tooltip tooltip-left size-10 self-center',
						browserViewerOpen ? 'btn-neutral' : 'btn-ghost'
					)}
					onclick={onToggleBrowserViewer}
					aria-label={browserViewerOpen ? 'Hide browser view' : 'Show browser view'}
					data-tip={browserViewerOpen ? 'Hide browser view' : 'Show browser view'}
				>
					<Monitor class="size-5" />
				</button>
			{/if}
		{/if}

		{#if !!sessionId}
			{#if open}
				<div in:fly={{ x: 100, duration: 150 }} class="flex flex-col gap-4">
					<div
						class="rounded-selector bg-base-200 dark:border-base-300 flex flex-col gap-2 border border-transparent p-4"
					>
						<h4 class="flex w-full items-center justify-between gap-2 text-sm font-semibold">
							To Do List
							<button
								class="btn btn-ghost btn-xs tooltip tooltip-left"
								data-tip={showTodoList ? 'Hide To Do List' : 'Show To Do List'}
								onclick={() => (showTodoList = !showTodoList)}
							>
								{#if showTodoList}
									<ChevronUp class="size-4" />
								{:else}
									<ChevronDown class="size-4" />
								{/if}
							</button>
						</h4>
						{#if showTodoList}
							<ul class="flex flex-col gap-1.5">
								{#if todoItems.length > 0}
									{#each todoItems as item, i (i)}
										<li class="flex min-w-0 items-start gap-2 text-sm font-light">
											{#if item.status === 'completed' || item.status === 'cancelled'}
												<CheckCircle2 class="text-success mt-0.5 size-4 shrink-0" />
											{:else if item.status === 'in_progress'}
												<Loader2 class="text-primary mt-0.5 size-4 shrink-0 animate-spin" />
											{:else}
												<Circle class="text-base-content/40 mt-0.5 size-4 shrink-0" />
											{/if}
											<span
												class="min-w-0 truncate"
												class:line-through={item.status === 'completed' ||
													item.status === 'cancelled'}
												class:opacity-50={item.status === 'cancelled'}
											>
												{item.content}
											</span>
										</li>
									{/each}
								{:else}
									<li
										class="text-base-content/50 flex min-w-0 items-start gap-2 text-xs font-light italic"
									>
										<span class="min-w-0 truncate"
											>Running to-dos for longer tasks will display here. You do not currently have
											any running to-dos.</span
										>
									</li>
								{/if}
							</ul>
						{/if}
					</div>
					<div class="flex flex-col gap-2">
						{@render listThreadFiles(false)}
					</div>
				</div>
			{:else if onToggle}
				<button
					class="btn btn-ghost btn-circle tooltip tooltip-left size-10 self-center"
					onclick={() => onToggle()}
					aria-label="Expand to show to-do list"
					data-tip="Expand to show to-do list"
				>
					<ListCheck class="text-base-content/50 size-5" />
				</button>

				{@render listThreadFiles(true)}
			{/if}
			<div class="flex grow"></div>
			{#if onToggle}
				<div
					class={twMerge(
						'sticky right-0 bottom-2 flex flex-shrink-0',
						open ? 'justify-start' : 'justify-center'
					)}
				>
					<button
						class={twMerge(
							'btn btn-ghost btn-circle tooltip',
							open ? 'tooltip-right' : 'tooltip-left'
						)}
						onclick={() => onToggle()}
						data-tip={open ? 'Close to-do & file list' : 'Open to-do & file list'}
					>
						{#if open}
							<SidebarOpen class="text-base-content/50 size-6" />
						{:else}
							<SidebarClose class="text-base-content/50 size-6" />
						{/if}
					</button>
				</div>
			{/if}
		{/if}
	</div>
</div>

{#snippet listThreadFiles(compact?: boolean)}
	{#each files ?? [] as file (file.uri)}
		{@const openPath = file.uri.startsWith('file:///workflows/')
			? file.uri
			: file.uri.replace('file:///', `file:///sessions/${sessionId}/`)}
		{@const isSelected = selectedFile === openPath}
		<FileItem
			uri={openPath}
			type="button"
			{compact}
			{isSelected}
			onClick={() => {
				if (openPath) {
					projectLayout.handleFileOpen(openPath);
				} else {
					console.error('No file path found for tool call', file);
				}
			}}
		/>
	{/each}
{/snippet}
