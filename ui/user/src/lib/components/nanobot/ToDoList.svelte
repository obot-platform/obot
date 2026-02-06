<script lang="ts">
	import type { ChatService } from '$lib/services/nanobot/chat/index.svelte';
	import type { ResourceContents } from '$lib/services/nanobot/types';
	import type { ChatMessageItemToolCall } from '$lib/services/nanobot/types';
	import { fly } from 'svelte/transition';
	import { Circle, CheckCircle2, Loader2 } from 'lucide-svelte';
	import { responsive } from '$lib/stores';

	interface Props {
		chat: ChatService;
		open: boolean;
	}

	let { chat, open }: Props = $props();

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

	const todoUri = 'todo:///list';
	let todo = $state<ResourceContents | null>(null);
	let hasTodoResource = $derived(chat.resources.some((r) => r.uri === todoUri));

	/** Todo list parsed from todo resource (watchResource / readResource) */
	let todoItemsFromResource = $derived.by((): TodoItem[] => {
		if (!todo?.text) return [];
		try {
			const parsed = JSON.parse(todo.text) as unknown;
			return Array.isArray(parsed)
				? (parsed as unknown[])
						.map((t) => parseTodoItem(t))
						.filter((t): t is TodoItem => t !== null)
				: [];
		} catch {
			return [];
		}
	});

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

	/** Prefer resource-based list when non-empty; otherwise use list derived from message tool calls */
	let todoItems = $derived(
		todoItemsFromResource.length > 0 ? todoItemsFromResource : todoItemsFromMessages
	);

	$effect(() => {
		if (!chat.chatId) return;
		if (hasTodoResource) {
			chat.readResource(todoUri).then((result) => {
				todo = result.contents?.[0] ?? null;
			});
		}

		const todoCleanup = hasTodoResource
			? chat.watchResource(todoUri, (updatedResource) => {
					todo = updatedResource;
				})
			: null;

		return () => {
			todoCleanup?.();
		};
	});
</script>

{#if chat.chatId && todoItems.length > 0 && !responsive.isMobile && open}
	<div
		class="h-[calc(100dvh-4rem)] w-sm min-w-sm overflow-hidden"
		in:fly={{ x: 100, duration: 150 }}
	>
		<div class="bg-base-100 flex h-full w-full flex-col">
			<div class="flex-1 overflow-auto p-4 pt-0">
				<div
					class="rounded-selector bg-base-200 dark:border-base-300 flex flex-col gap-2 border border-transparent p-4"
				>
					<h4 class="text-sm font-semibold">To Do List</h4>
					<ul class="flex flex-col gap-1.5">
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
					</ul>
				</div>
			</div>
		</div>
	</div>
{/if}
