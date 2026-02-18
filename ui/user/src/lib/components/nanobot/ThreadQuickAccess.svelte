<script lang="ts">
	import type { ChatMessageItemToolCall } from '$lib/services/nanobot/types';
	import {
		Circle,
		CheckCircle2,
		Loader2,
		ChevronUp,
		ChevronDown,
		SidebarOpen,
		SidebarClose,
		ListCheck,
		FileIcon,
		WorkflowIcon
	} from 'lucide-svelte';
	import { twMerge } from 'tailwind-merge';
	import Profile from '../navbar/Profile.svelte';
	import { fly } from 'svelte/transition';
	import { nanobotChat } from '$lib/stores/nanobotChat.svelte';
	import { getContext } from 'svelte';

	interface Props {
		onToggle?: () => void;
		open?: boolean;
		files?: ChatMessageItemToolCall[];
	}

	let { onToggle, open, files }: Props = $props();

	/** Todo item shape from todo:///list resource or todo_write tool (application/json) */
	interface TodoItem {
		content: string;
		status: 'pending' | 'in_progress' | 'completed' | 'cancelled';
		activeForm?: string;
	}

	const TODO_WRITE_NAMES = ['todo_write', 'todoWrite'];
	const projectLayout = getContext<{
		handleFileOpen: (filename: string) => void;
	}>('nanobot-project-layout');

	function parseTodoItem(raw: unknown): TodoItem | null {
		if (!raw || typeof raw !== 'object') return null;
		const o = raw as Record<string, unknown>;
		const content = typeof o.content === 'string' ? o.content : '';
		const status = o.status;
		const validStatus =
			status === 'pending' ||
			status === 'in_progress' ||
			status === 'completed' ||
			status === 'cancelled'
				? status
				: 'pending';
		return { content, status: validStatus, activeForm: o.activeForm as string | undefined };
	}

	function parseTodosFromToolCall(item: ChatMessageItemToolCall): TodoItem[] {
		const out: TodoItem[] = [];
		// Prefer tool output (structuredContent or content with resource) when present
		const output = item.output;
		if (
			output?.structuredContent &&
			Array.isArray((output.structuredContent as { todos?: unknown[] }).todos)
		) {
			const todos = (output.structuredContent as { todos: unknown[] }).todos;
			for (const t of todos) {
				const parsed = parseTodoItem(t);
				if (parsed) out.push(parsed);
			}
			if (out.length > 0) return out;
		}
		if (output?.structuredContent && Array.isArray(output.structuredContent)) {
			for (const t of output.structuredContent as unknown[]) {
				const parsed = parseTodoItem(t);
				if (parsed) out.push(parsed);
			}
			if (out.length > 0) return out;
		}
		// Parse tool input (arguments): agent sends { merge, todos } or just { todos }
		if (!item.arguments) return [];
		try {
			const args = JSON.parse(item.arguments) as { todos?: unknown[] };
			if (Array.isArray(args.todos)) {
				for (const t of args.todos) {
					const parsed = parseTodoItem(t);
					if (parsed) out.push(parsed);
				}
			}
		} catch {
			// ignore
		}
		return out;
	}

	/** Todo list derived from latest todo_write / todoWrite tool call in messages (works even when server doesn't push resource updates) */
	let todoItemsFromMessages = $derived.by((): TodoItem[] => {
		const messages = $nanobotChat?.chat?.messages ?? [];
		if (!messages?.length) return [];
		let latest: TodoItem[] = [];
		for (const msg of messages) {
			if (msg.role !== 'assistant' || !msg.items) continue;
			for (const item of msg.items) {
				if (item.type !== 'tool') continue;
				const tool = item as ChatMessageItemToolCall;
				if (tool.name && TODO_WRITE_NAMES.includes(tool.name)) {
					const parsed = parseTodosFromToolCall(tool);
					if (parsed.length > 0) latest = parsed;
				}
			}
		}
		return latest;
	});

	let showTodoList = $state(true);
	let todoItems = $derived(todoItemsFromMessages);
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
			<Profile />
		</div>

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
				{@render listThreadFiles(false)}
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
	</div>
</div>

{#snippet listThreadFiles(compact?: boolean)}
	{#each files ?? [] as file (file.callID)}
		{@const args = (() => {
			try {
				return JSON.parse(file.arguments ?? '{}') as { file_path: string } | undefined;
			} catch (error) {
				console.error('Failed to parse file.arguments as JSON in ThreadQuickAccess', {
					error,
					file,
					arguments: file.arguments
				});
				return undefined;
			}
		})()}
		{@const displayLabel = args?.file_path.split('/').pop()?.split('.').shift()}
		{@const isWorkflow =
			(args?.file_path?.includes('workflows/') || args?.file_path?.startsWith('workflow://')) &&
			!args?.file_path?.includes('.runs')}
		{@const openPath = args?.file_path?.includes('://')
			? args.file_path
			: args?.file_path
				? `file:///${args.file_path}`
				: undefined}
		{#if compact}
			<button
				class="btn btn-ghost btn-circle tooltip tooltip-left size-10 self-center"
				in:fly={{ x: 100, duration: 150 }}
				onclick={() => {
					if (openPath) {
						projectLayout.handleFileOpen(openPath);
					} else {
						console.error('No file path found for tool call', file);
					}
				}}
				data-tip={`Open ${displayLabel}`}
			>
				{#if isWorkflow}
					<WorkflowIcon class="text-base-content/50 size-5" />
				{:else}
					<FileIcon class="text-base-content/50 size-5" />
				{/if}
			</button>
		{:else}
			<button
				class="rounded-selector hover:bg-base-300 flex items-center gap-2 border border-transparent px-4 py-2"
				onclick={() => {
					if (openPath) {
						projectLayout.handleFileOpen(openPath);
					} else {
						console.error('No file path found for tool call', file);
					}
				}}
			>
				{#if isWorkflow}
					<WorkflowIcon class="size-5" />
				{:else}
					<FileIcon class="size-5" />
				{/if}
				<span>{displayLabel}</span>
			</button>
		{/if}
	{/each}
{/snippet}
