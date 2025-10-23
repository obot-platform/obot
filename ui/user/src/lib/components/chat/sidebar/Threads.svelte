<script lang="ts">
	import { Pen, Plus, Save, Trash2, Pin, PinOff } from 'lucide-svelte';
	import { ChatService, type Project, type Thread } from '$lib/services';
	import { onDestroy, onMount, tick } from 'svelte';
	import { CircleX } from 'lucide-svelte/icons';
	import { closeAll, getLayout, isSomethingSelected } from '$lib/context/chatLayout.svelte.js';
	import { fade } from 'svelte/transition';
	import { overflowToolTip } from '$lib/actions/overflow.js';
	import DotDotDot from '$lib/components/DotDotDot.svelte';
	import { responsive } from '$lib/stores';
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import CollapsePane from '$lib/components/edit/CollapsePane.svelte';
	import { HELPER_TEXTS } from '$lib/context/helperMode.svelte';
	import { localState } from '$lib/runes/localState.svelte';
	import { flip } from 'svelte/animate';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';

	interface Props {
		currentThreadID?: string;
		project: Project;
		editor?: boolean;
	}

	let { currentThreadID = $bindable(), project, editor }: Props = $props();

	let input = $state<HTMLInputElement>();
	let editMode = $state<string | null>(null);
	let name = $state('');
	let isOpen = $state(false);
	let layout = getLayout();
	let lastSeenThreadID = $state('');
	let watchingThread: (() => void) | undefined;
	let displayCount = $state(10); // Number of threads to display initially
	let localPinnedThreads = localState<Record<string, string[]>>('@obot/sidebar/pinned-threads', {
		[project.id]: []
	}); // Track pinned thread IDs

	const projectPinnedThreads = $derived(localPinnedThreads.current?.[project.id] || []) as string[];

	function isCurrentThread(thread: Thread) {
		return currentThreadID === thread.id && !isSomethingSelected(layout);
	}

	function setCurrentThread(id: string) {
		closeAll(layout);
		lastSeenThreadID = id;
		currentThreadID = id;
		layout.items = [];
	}

	function loadMore() {
		displayCount += 10;
	}

	function toggleThreadPin(threadId: string) {
		const pinnedThreadsSet = new Set(projectPinnedThreads);

		if (pinnedThreadsSet.has(threadId)) {
			pinnedThreadsSet.delete(threadId);
		} else {
			pinnedThreadsSet.add(threadId);
		}

		savePinnedThreads(pinnedThreadsSet.values().toArray());
	}

	function isThreadPinned(threadId: string): boolean {
		return projectPinnedThreads.includes(threadId);
	}

	function savePinnedThreads(threads: string[]) {
		try {
			const current = localPinnedThreads.current || {};

			current[project.id] = threads;

			localPinnedThreads.current = current;
		} catch (e) {
			console.error('Failed to save pinned threads:', e);
		}
	}

	// Derived value: sorted threads with pinned ones first
	let sortedThreads = $derived.by(() => {
		const pinnedThreadsSet = new Set(projectPinnedThreads);

		const threads = [...($state.snapshot(layout.threads) ?? [])];
		return threads.sort((a, b) => {
			const aPinned = pinnedThreadsSet.has(a.id);
			const bPinned = pinnedThreadsSet.has(b.id);
			if (aPinned && !bPinned) return -1;
			if (!aPinned && bPinned) return 1;
			return 0;
		});
	});

	async function startEditName() {
		const thread = layout.threads?.find(isCurrentThread);
		name = thread?.name ?? '';
		editMode = thread?.id ?? null;
		tick().then(() => input?.focus());
	}

	async function saveName() {
		let thread = layout.threads?.find(isCurrentThread);
		if (!thread) {
			editMode = null;
			return;
		}

		thread.name = name;
		thread = await ChatService.updateThread(project.assistantID, project.id, thread);
		layout.threads?.forEach((t, i) => {
			if (t.id === thread.id) {
				layout.threads![i] = thread;
			}
		});
		editMode = null;
	}

	export async function createThread() {
		editMode = null;
		const thread = await ChatService.createThread(project.assistantID, project.id);
		const found = layout.threads?.find((t) => t.id === thread.id);
		if (!found) {
			layout.threads?.splice(0, 0, thread);
		}
		if (responsive.isMobile) {
			layout.sidebarOpen = false;
		}
		layout.newChatMode = true;
		setCurrentThread(thread.id);
		focusChat();
	}

	function focusChat() {
		const e = window.document.querySelector('#main-input textarea');
		if (e instanceof HTMLTextAreaElement) {
			e.focus();
		}
	}

	async function deleteThread(id: string) {
		await ChatService.deleteThread(project.assistantID, project.id, id);
		layout.threads = layout.threads?.filter((thread) => thread.id !== id);

		// Check if threads are empty after deletion
		if (layout.threads?.length === 0) {
			// Delete 'thread' param from URL
			const url = new URL(page.url);
			url.searchParams.delete('thread');

			// Navigate to updated URL
			goto(url);
		} else {
			// Update 'thread' param
			setCurrentThread(layout.threads?.[0]?.id ?? '');
		}
	}

	function selectThread(id: string) {
		if (responsive.isMobile) {
			layout.sidebarOpen = false;
		}

		layout.newChatMode = false;
		editMode = null;
		setCurrentThread(id);
		focusChat();
	}

	onMount(() => {
		watchThreads();
	});

	onDestroy(() => {
		if (watchingThread) {
			watchingThread();
			watchingThread = undefined;
			console.log('stop watching threads', project.id);
		}
	});

	$effect(() => {
		if (layout.deleting) {
			watchingThread?.();
			watchingThread = undefined;
			console.log('project being deleted, stop watching threads', project.id);
		}
	});

	async function watchThreads(): Promise<void> {
		if (watchingThread) {
			return;
		}

		watchingThread = ChatService.watchThreads(project.assistantID, project.id, (thread) => {
			if (thread.deleted) {
				console.log('deleted thread', thread.id);
				layout.threads = layout.threads?.filter((t) => t.id !== thread.id);
				layout.taskRuns = layout.taskRuns?.filter((t) => t.id !== thread.id);

				// Clean up pinned thread if it was deleted
				if (isThreadPinned(thread.id)) {
					const pinnedThreadsSet = new Set(projectPinnedThreads);
					pinnedThreadsSet.delete(thread.id);
					savePinnedThreads(pinnedThreadsSet.values().toArray());
				}

				if (currentThreadID === thread.id) {
					setCurrentThread(layout.threads?.[0]?.id ?? '');
				}
				return;
			}

			let found = false;
			for (let i = 0; i < (layout.threads?.length ?? 0); i++) {
				if (layout.threads?.[i].id === thread.id) {
					layout.threads[i] = thread;
					found = true;
					break;
				}
			}

			for (let i = 0; i < (layout.taskRuns?.length ?? 0); i++) {
				if (layout.taskRuns?.[i].id === thread.id) {
					layout.taskRuns[i] = thread;
					found = true;
					break;
				}
			}

			if (!found) {
				if (thread.taskID) {
					layout.taskRuns?.splice(0, 0, thread);
					return;
				}
				layout.threads?.splice(0, 0, thread);
			}
		});
	}

	async function reloadThread() {
		const threads = (await ChatService.listThreads(project.assistantID, project.id)).items;
		layout.threads = threads.filter((t) => !t.deleted && !t.taskID);
		layout.taskRuns = threads.filter((t) => !t.deleted && !!t.taskID);
	}

	async function open() {
		await reloadThread();
		togglePanel();
	}

	function togglePanel() {
		isOpen = !isOpen;

		focusChat();
	}

	$effect(() => {
		if (layout.sidebarOpen && !isOpen) {
			open();
		}
	});

	$effect(() => {
		if (currentThreadID && !isSomethingSelected(layout)) {
			const thread = layout.threads?.find((t) => t.id === currentThreadID);
			if (thread) {
				name = thread.name;
				if (lastSeenThreadID !== currentThreadID) {
					reloadThread();
					setCurrentThread(currentThreadID);
				}
			}
		}
	});
</script>

{#if isOpen}
	{#if editor}
		<CollapsePane
			classes={{ header: 'pl-3 py-2', content: 'p-2' }}
			iconSize={5}
			header="Threads"
			helpText={HELPER_TEXTS.threads}
			open={(layout.threads?.length ?? 0) > 0}
		>
			<div class="flex flex-col gap-4 text-xs">
				{@render content()}
			</div>
			{#if (layout.threads?.length ?? 0) === 0}
				<div class="flex justify-end" in:fade>
					<button class="button flex items-center gap-1 text-xs" onclick={() => createThread()}>
						<Plus class="size-4" /> Start New Chat
					</button>
				</div>
			{/if}
		</CollapsePane>
	{:else}
		<div class="flex flex-col text-xs">
			<div class="flex items-center justify-between">
				<p class="text-md grow font-medium">Chats</p>
				<button
					class="p-2 text-gray-400 transition-colors duration-200 hover:text-black dark:text-gray-600 dark:hover:text-white"
					onclick={createThread}
					use:tooltip={'Start New Chat'}
				>
					<Plus class="size-5" />
				</button>
			</div>
			{@render content()}
		</div>
	{/if}
{/if}

{#snippet content()}
	<ul transition:fade>
		{#each sortedThreads.slice(0, displayCount) as thread (thread.id)}
			<li
				class:bg-surface2={isCurrentThread(thread)}
				class="hover:bg-surface3 group flex min-h-9 items-center gap-3 rounded-md font-light"
				animate:flip={{ duration: 200 }}
			>
				{#if editMode === thread.id}
					<input
						bind:value={name}
						bind:this={input}
						onkeyup={(e) => {
							switch (e.key) {
								case 'Escape':
									editMode = null;
									break;
								case 'Enter':
									saveName();
									break;
							}
						}}
						class="mx-2 w-0 flex-1 grow border-none bg-transparent ring-0 outline-hidden dark:text-white"
						placeholder="Enter name"
						type="text"
					/>

					<button class="list-button-primary" onclick={() => (editMode = null)}>
						<CircleX class="h-4 w-4" />
					</button>
					<button class="list-button-primary" onclick={saveName}>
						<Save class="mr-2 h-4 w-4" />
					</button>
				{:else}
					<button
						use:overflowToolTip
						class:font-medium={isCurrentThread(thread)}
						class="flex h-full flex-1 grow items-center gap-2 p-2 text-start"
						onclick={() => selectThread(thread.id)}
					>
						<span class="truncate">{thread.name || 'New Chat'}</span>
						{#if isThreadPinned(thread.id)}
							<span transition:fade={{ duration: 100 }}>
								<Pin class="h-3 w-3 shrink-0 text-blue-500" />
							</span>
						{/if}
					</button>

					<DotDotDot
						class="p-0 pr-2.5 transition-opacity duration-200 group-hover:opacity-100 md:opacity-0"
					>
						<div class="default-dialog flex min-w-max flex-col p-2">
							<button class="menu-button" onclick={() => toggleThreadPin(thread.id)}>
								{#if isThreadPinned(thread.id)}
									<PinOff class="h-4 w-4" /> Unpin
								{:else}
									<Pin class="h-4 w-4" /> Pin
								{/if}
							</button>
							<button
								class="menu-button"
								onclick={() => {
									selectThread(thread.id);
									startEditName();
								}}
							>
								<Pen class="h-4 w-4" /> Rename
							</button>
							<button class="menu-button" onclick={() => deleteThread(thread.id)}>
								<Trash2 class="h-4 w-4" /> Delete
							</button>
						</div>
					</DotDotDot>
				{/if}
			</li>
		{/each}
		{#if layout.threads?.length && layout.threads?.length > displayCount}
			<li class="hover:bg-surface3 flex w-full justify-center rounded-md p-2">
				<button class="w-full text-xs" onclick={loadMore}> Show More </button>
			</li>
		{/if}
	</ul>
{/snippet}
