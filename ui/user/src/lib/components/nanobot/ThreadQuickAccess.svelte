<script lang="ts">
	import type { ChatService } from '$lib/services/nanobot/chat/index.svelte';
	import type { ChatMessageItemToolCall } from '$lib/services/nanobot/types';
	import {
		Circle,
		CheckCircle2,
		Loader2,
		File,
		Folder,
		ChevronUp,
		ChevronDown,
		ChevronRight,
		FolderOpen,
		SidebarOpen,
		SidebarClose,
		ListCheck,
		Folders
	} from 'lucide-svelte';
	import { twMerge } from 'tailwind-merge';
	import Profile from '../navbar/Profile.svelte';
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import { fly } from 'svelte/transition';

	interface Props {
		chat: ChatService;
		onFileOpen?: (filename: string) => void;
		selectedFile?: string;
		onToggle?: () => void;
		open?: boolean;
	}

	let { chat, onFileOpen, selectedFile, onToggle, open }: Props = $props();

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

	let todoItems = $derived(todoItemsFromMessages);
	let resourceFiles = $derived(
		chat.resources ? chat.resources.filter((r) => r.uri.startsWith('file:///')) : []
	);

	type FileTreeNode =
		| { type: 'folder'; name: string; children: FileTreeNode[] }
		| { type: 'file'; name: string; uri: string };

	function buildFileTreeSimple(files: { uri: string; name?: string }[]): FileTreeNode[] {
		const root: Extract<FileTreeNode, { type: 'folder' }> = {
			type: 'folder',
			name: '',
			children: []
		};
		function ensurePath(segments: string[]): Extract<FileTreeNode, { type: 'folder' }> {
			let current = root;
			for (const seg of segments) {
				let found = current.children.find((c) => c.type === 'folder' && c.name === seg) as
					| Extract<FileTreeNode, { type: 'folder' }>
					| undefined;
				if (!found) {
					found = { type: 'folder', name: seg, children: [] };
					current.children.push(found);
				}
				current = found;
			}
			return current;
		}
		for (const f of files) {
			const path = f.uri.replace(/^file:\/\/+/, '');
			const segments = path.split('/').filter(Boolean);
			if (segments.length === 0) continue;
			const fileName = segments.pop()!;
			const parent = ensurePath(segments);
			parent.children.push({ type: 'file', name: fileName, uri: f.uri });
		}
		// Sort: folders first then files, both alphabetically
		function sortNodes(nodes: FileTreeNode[]): void {
			nodes.sort((a, b) => {
				if (a.type === 'folder' && b.type === 'file') return -1;
				if (a.type === 'file' && b.type === 'folder') return 1;
				return (a.name || '').localeCompare(b.name || '');
			});
			for (const n of nodes) {
				if (n.type === 'folder') sortNodes(n.children);
			}
		}
		sortNodes(root.children);
		return root.children;
	}

	let fileTree = $derived(buildFileTreeSimple(resourceFiles));

	type FlatNode = { depth: number; path: string; node: FileTreeNode };
	function flattenTree(
		nodes: FileTreeNode[],
		depth: number,
		pathPrefix: string,
		isOpen: (path: string) => boolean
	): FlatNode[] {
		const out: FlatNode[] = [];
		for (const n of nodes) {
			const path = pathPrefix ? `${pathPrefix}/${n.name}` : n.name;
			out.push({ depth, path, node: n });
			if (n.type === 'folder' && isOpen(path)) {
				out.push(...flattenTree(n.children, depth + 1, path, isOpen));
			}
		}
		return out;
	}

	let folderOpen = $state<Record<string, boolean>>({});
	function toggleFolder(path: string) {
		folderOpen[path] = !(folderOpen[path] ?? true);
		folderOpen = { ...folderOpen };
	}
	function isFolderOpen(path: string): boolean {
		return folderOpen[path] ?? true;
	}

	let flatFileList = $derived.by(() => {
		const open = folderOpen;
		return flattenTree(fileTree, 0, '', (path) => open[path] ?? true);
	});
</script>

<div
	class={twMerge(
		'bg-base-100 border-base-300 h-[100dvh] w-18 min-w-18 overflow-hidden overflow-y-auto border-l ',
		open && 'w-sm min-w-sm'
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
			<div
				class="rounded-selector bg-base-200 dark:border-base-300 flex flex-col gap-2 border border-transparent p-4"
				in:fly={{ x: 100, duration: 150 }}
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
										class:line-through={item.status === 'completed' || item.status === 'cancelled'}
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
				in:fly={{ x: 100, duration: 150 }}
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
					<ul class="flex flex-col">
						{#if flatFileList.length > 0}
							{#each flatFileList as { depth, path, node } (node.type === 'file' ? node.uri : `folder:${path}`)}
								<li class="w-full text-sm font-light">
									{#if node.type === 'folder'}
										<button
											class="btn btn-ghost flex w-full min-w-0 items-center justify-start gap-2 rounded-none text-left"
											style="padding-left: {depth * 0.75}rem;"
											onclick={() => toggleFolder(path)}
											aria-expanded={isFolderOpen(path)}
										>
											<span class="flex shrink-0 pl-2">
												{#if isFolderOpen(path)}
													<ChevronDown class="text-base-content/60 size-4" />
												{:else}
													<ChevronRight class="text-base-content/60 size-4" />
												{/if}
											</span>
											<div class="bg-base-200 shrink-0 rounded-md p-1">
												{#if isFolderOpen(path)}
													<FolderOpen class="text-primary/80 size-4" />
												{:else}
													<Folder class="text-primary/80 size-4" />
												{/if}
											</div>
											<span class="min-w-0 truncate font-normal">{node.name}</span>
										</button>
									{:else}
										<button
											class={twMerge(
												'btn btn-ghost flex w-full min-w-0 items-center justify-start gap-2 rounded-none text-left',
												selectedFile === node.uri ? 'bg-base-300' : ''
											)}
											style="padding-left: {depth * 0.65}rem;"
											onclick={() => onFileOpen?.(node.uri)}
										>
											<span class="w-[14px] shrink-0" aria-hidden="true"></span>
											<div class="bg-base-200 shrink-0 rounded-md p-1">
												<File class="size-4" />
											</div>
											<span class="min-w-0 truncate font-normal">{node.name}</span>
										</button>
									{/if}
								</li>
							{/each}
						{:else}
							<li
								class="text-base-content/50 flex items-start gap-2 px-4 text-xs font-light italic"
							>
								<span>No files found.</span>
							</li>
						{/if}
					</ul>
				{/if}
			</div>
		{:else if onToggle}
			<button
				class="btn btn-ghost btn-circle tooltip tooltip-left size-10 self-center"
				onclick={() => onToggle()}
				aria-label="Expand to show to-do list"
				use:tooltip={'Expand'}
			>
				<ListCheck class="text-base-content/50 size-6" />
			</button>
			<button
				class="btn btn-ghost btn-circle tooltip tooltip-left size-10 self-center"
				onclick={() => onToggle()}
				aria-label="Expand to show file list"
				use:tooltip={'Expand'}
			>
				<Folders class="text-base-content/50 size-6" />
			</button>
		{/if}
		<div class="flex grow"></div>
		{#if onToggle}
			<div
				class={twMerge(
					'sticky right-0 bottom-2 flex flex-shrink-0',
					open ? 'justify-end' : 'justify-center'
				)}
			>
				<button
					class="btn btn-ghost btn-circle tooltip tooltip-left"
					onclick={() => onToggle()}
					use:tooltip={open ? 'Close to-do & file list' : 'Open to-do & file list'}
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
