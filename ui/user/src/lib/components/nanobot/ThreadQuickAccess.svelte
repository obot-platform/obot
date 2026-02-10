<script lang="ts">
	import type { ChatService } from '$lib/services/nanobot/chat/index.svelte';
	import type { ChatMessageItemToolCall } from '$lib/services/nanobot/types';
	import { fly } from 'svelte/transition';
	import { Circle, CheckCircle2, Loader2, File, ChevronUp, ChevronDown } from 'lucide-svelte';
	import { slide } from 'svelte/transition';

	interface Props {
		chat: ChatService;
		onFileOpen?: (filename: string) => void;
	}

	let { chat, onFileOpen }: Props = $props();

	/** Todo item shape from todo:///list resource or todo_write tool (application/json) */
	interface TodoItem {
		content: string;
		status: 'pending' | 'in_progress' | 'completed' | 'cancelled';
		activeForm?: string;
	}

	const TODO_WRITE_NAMES = ['todo_write', 'todoWrite'];

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
		const messages = chat.messages;
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
	let showFiles = $state(true);

	/** Prefer resource-based list when non-empty; otherwise use list derived from message tool calls */
	let todoItems = $derived(todoItemsFromMessages);
	let resourceFiles = $derived(
		chat.resources ? chat.resources.filter((r) => r.uri.startsWith('file:///')) : []
	);
</script>

<div
	class="h-[calc(100dvh-4rem)] w-sm min-w-sm overflow-hidden overflow-y-auto"
	in:fly={{ x: 100, duration: 150 }}
>
	<div class="bg-base-100 flex h-full w-full flex-col gap-4 p-4">
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
				<ul class="flex flex-col gap-1.5" in:slide={{ axis: 'y' }}>
					{#if todoItems.length > 0}
						{#each todoItems as item, i (i)}
							<li class="flex items-start gap-2 text-sm font-light">
								{#if item.status === 'completed' || item.status === 'cancelled'}
									<CheckCircle2 class="text-success mt-0.5 size-4 shrink-0" />
								{:else if item.status === 'in_progress'}
									<Loader2 class="text-primary mt-0.5 size-4 shrink-0 animate-spin" />
								{:else}
									<Circle class="text-base-content/40 mt-0.5 size-4 shrink-0" />
								{/if}
								<span
									class:line-through={item.status === 'completed' || item.status === 'cancelled'}
									class:opacity-50={item.status === 'cancelled'}
								>
									{item.content}
								</span>
							</li>
						{/each}
					{:else}
						<li class="text-base-content/50 flex items-start gap-2 text-xs font-light italic">
							<span
								>Running to-dos for longer tasks will display here. You do not currently have any
								running to-dos.</span
							>
						</li>
					{/if}
				</ul>
			{/if}
		</div>
		<div
			class="rounded-selector bg-base-200 dark:border-base-300 flex flex-col gap-2 border border-transparent py-4"
		>
			<h4 class="flex w-full justify-between gap-2 px-4 text-sm font-semibold">
				Files
				<button
					class="btn btn-ghost btn-xs tooltip tooltip-left"
					data-tip={showFiles ? 'Hide Files' : 'Show Files'}
					onclick={() => (showFiles = !showFiles)}
				>
					{#if showFiles}
						<ChevronUp class="size-4" />
					{:else}
						<ChevronDown class="size-4" />
					{/if}
				</button>
			</h4>
			{#if showFiles}
				<ul class="flex flex-col" in:slide={{ axis: 'y' }}>
					{#if resourceFiles.length > 0}
						{#each resourceFiles as resourceFile (resourceFile.uri)}
							<li class="flex items-start gap-2 text-sm font-light">
								<button
									class="btn btn-ghost w-full justify-start rounded-none text-left"
									onclick={() => {
										onFileOpen?.(resourceFile.uri);
									}}
								>
									<div class="bg-base-200 rounded-md p-1">
										<File class="size-4" />
									</div>
									<span class="break-all">{resourceFile.name}</span>
								</button>
							</li>
						{/each}
					{:else}
						<li class="text-base-content/50 flex items-start gap-2 text-xs font-light italic">
							<span>No files found.</span>
						</li>
					{/if}
				</ul>
			{/if}
		</div>
	</div>
</div>
