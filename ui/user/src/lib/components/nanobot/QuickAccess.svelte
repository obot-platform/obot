<script lang="ts">
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import FileItem from '$lib/components/nanobot/FileItem.svelte';
	import type { ChatMessageItemToolCall, ProjectLayoutContext } from '$lib/services/nanobot/types';
	import { PROJECT_LAYOUT_CONTEXT } from '$lib/services/nanobot/types';
	import { responsive } from '$lib/stores';
	import { nanobotChat } from '$lib/stores/nanobotChat.svelte';
	import Profile from '../navbar/Profile.svelte';
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
	import { getContext } from 'svelte';
	import { fly } from 'svelte/transition';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		onToggle?: () => void;
		onToggleBrowserViewer?: () => void;
		open?: boolean;
		browserViewerOpen?: boolean;
		browserAvailable?: boolean;
		sessionId?: string;
		workflowId?: string;
		selectedFile?: string;
		agentId?: string;
		projectId?: string;
		impersonating?: boolean;
	}

	let {
		onToggle,
		onToggleBrowserViewer,
		open,
		browserViewerOpen = false,
		browserAvailable = false,
		sessionId,
		workflowId,
		selectedFile,
		agentId,
		projectId,
		impersonating
	}: Props = $props();

	/** Todo item shape from todo:///list resource or todo_write tool (application/json) */
	interface TodoItem {
		content: string;
		status: 'pending' | 'in_progress' | 'completed' | 'cancelled';
		activeForm?: string;
	}

	const projectLayout = getContext<ProjectLayoutContext>(PROJECT_LAYOUT_CONTEXT);
	const chatForSession = $derived(
		$nanobotChat?.chat?.chatId === sessionId ? ($nanobotChat?.chat ?? null) : null
	);

	function getScopedSessionId(uri: string): string | undefined {
		return uri.match(/^file:\/\/\/sessions\/([^/]+)\//)?.[1];
	}

	function isWorkflowSupportFile(uri: string, workflowName?: string): boolean {
		return !!workflowName && uri.startsWith(`file:///workflows/${workflowName}/`);
	}

	function isPlainThreadFile(uri: string): boolean {
		return (
			uri.startsWith('file:///') &&
			!uri.startsWith('file:///sessions/') &&
			!uri.startsWith('file:///workflows/') &&
			!uri.startsWith('file:///skills/')
		);
	}

	function shouldShowFile(uri: string, sessionId?: string, workflowName?: string): boolean {
		if (isWorkflowSupportFile(uri, workflowName)) {
			return true;
		}

		const scopedSessionId = getScopedSessionId(uri);
		if (scopedSessionId) {
			return scopedSessionId === sessionId;
		}

		return !!sessionId && isPlainThreadFile(uri);
	}

	function toOpenPath(uri: string, sessionId?: string): string {
		if (uri.startsWith('file:///sessions/') || uri.startsWith('file:///workflows/') || !sessionId) {
			return uri;
		}

		return uri.replace('file:///', `file:///sessions/${sessionId}/`);
	}

	const files = $derived.by(() => {
		const threadResources = chatForSession?.resources ?? [];
		const workflowResources = workflowId ? ($nanobotChat?.resources ?? []) : [];
		const deduped: Array<
			((typeof threadResources)[number] | (typeof workflowResources)[number]) & {
				openPath: string;
				sortName: string;
			}
		> = [];

		for (const resource of [...threadResources, ...workflowResources]) {
			if (!resource.uri.startsWith('file:///')) {
				continue;
			}
			if (!shouldShowFile(resource.uri, sessionId, workflowId)) {
				continue;
			}

			const openPath = toOpenPath(resource.uri, sessionId);
			if (deduped.some((file) => file.openPath === openPath)) {
				continue;
			}

			deduped.push({
				...resource,
				openPath,
				sortName: resource.name ?? resource.uri
			});
		}

		return deduped.sort((a, b) => {
			const aIsWorkflow = a.uri.startsWith('file:///workflows/');
			const bIsWorkflow = b.uri.startsWith('file:///workflows/');

			if (aIsWorkflow !== bIsWorkflow) {
				return aIsWorkflow ? -1 : 1;
			}

			return a.sortName.localeCompare(b.sortName);
		});
	});

	const hasSidebarContent = $derived(!!sessionId || !!workflowId || files.length > 0);

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
	let todoItems = $derived.by((): TodoItem[] => {
		const messages = chatForSession?.messages;
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
</script>

<div
	class={twMerge(
		'bg-base-100 border-base-300 h-[100dvh] w-18 min-w-18 overflow-y-auto border-l',
		responsive.isMobile
			? open
				? 'fixed top-0 left-0 h-[calc(100dvh-3.5rem)] w-full max-w-full'
				: 'hidden'
			: open
				? 'w-sm min-w-sm'
				: ''
	)}
>
	<div
		class={twMerge(
			'flex h-full w-full min-w-0 flex-col gap-4 pt-1',
			open ? 'p-4 pt-1' : 'pt-1 pb-4'
		)}
	>
		<div class={twMerge(open ? 'self-end' : 'w-14 self-center')}>
			<Profile {agentId} {projectId} {impersonating} />
		</div>

		{#if onToggleBrowserViewer && browserAvailable}
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
						'btn btn-circle size-10 self-center',
						browserViewerOpen ? 'btn-neutral' : 'btn-ghost'
					)}
					use:tooltip={{
						text: browserViewerOpen ? 'Hide browser view' : 'Show browser view',
						placement: 'left',
						variant: 'daisy'
					}}
					onclick={onToggleBrowserViewer}
					aria-label={browserViewerOpen ? 'Hide browser view' : 'Show browser view'}
				>
					<Monitor class="size-5" />
				</button>
			{/if}
		{/if}

		{#if hasSidebarContent}
			{#if open}
				<div in:fly={{ x: 100, duration: 150 }} class="flex flex-col gap-4">
					{#if !!sessionId}
						<div
							class="rounded-selector bg-base-200 dark:border-base-300 flex flex-col gap-2 border border-transparent p-4"
						>
							<h4 class="flex w-full items-center justify-between gap-2 text-sm font-semibold">
								To Do List
								<button
									class="btn btn-ghost btn-xs"
									use:tooltip={{
										text: showTodoList ? 'Hide To Do List' : 'Show To Do List',
										placement: 'left',
										variant: 'daisy'
									}}
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
											<li class="flex items-start gap-2 text-sm font-light">
												{#if item.status === 'completed' || item.status === 'cancelled'}
													<CheckCircle2 class="text-success mt-0.5 size-4 shrink-0" />
												{:else if item.status === 'in_progress'}
													<Loader2 class="text-primary mt-0.5 size-4 shrink-0 animate-spin" />
												{:else}
													<Circle class="text-base-content/40 mt-0.5 size-4 shrink-0" />
												{/if}
												<span
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
												>Running to-dos for longer tasks will display here. You do not currently
												have any running to-dos.</span
											>
										</li>
									{/if}
								</ul>
							{/if}
						</div>
					{/if}
					<div class="flex flex-col gap-2">
						{@render listThreadFiles(false)}
					</div>
				</div>
			{:else if onToggle}
				<button
					class="btn btn-ghost btn-circle size-10 self-center"
					use:tooltip={{ text: 'Expand to show to-do list', placement: 'left', variant: 'daisy' }}
					onclick={() => onToggle()}
					aria-label="Expand to show to-do list"
				>
					<ListCheck class="text-base-content/50 size-5" />
				</button>

				{@render listThreadFiles(true)}
			{/if}
			<div class="flex grow"></div>
			{#if onToggle && !responsive.isMobile}
				<div
					class={twMerge(
						'sticky right-0 bottom-2 flex flex-shrink-0',
						open ? 'justify-start' : 'justify-center'
					)}
				>
					<button
						class="btn btn-ghost btn-circle"
						use:tooltip={{
							text: open ? 'Close to-do & file list' : 'Open to-do & file list',
							placement: 'left',
							variant: 'daisy'
						}}
						onclick={() => onToggle()}
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
	{#each files ?? [] as file (file.openPath)}
		{@const openPath = file.openPath}
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
