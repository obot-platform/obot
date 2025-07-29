<script lang="ts">
	import { Pen, Plus, Save, Trash2 } from 'lucide-svelte';
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

	interface Props {
		currentThreadID?: string;
		project: Project;
		editor?: boolean;
	}

	let { currentThreadID = $bindable(), project, editor }: Props = $props();

	let input = $state<HTMLInputElement>();
	let editMode = $state(false);
	let name = $state('');
	let isOpen = $state(false);
	let layout = getLayout();
	let lastSeenThreadID = $state('');
	let watchingThread: (() => void) | undefined;
	let displayCount = $state(10); // Number of threads to display initially

	function isCurrentThread(thread: Thread) {
		return currentThreadID === thread.id && !isSomethingSelected(layout);
	}

	function setCurrentThread(id: string) {
		lastSeenThreadID = id;
		currentThreadID = id;
		layout.items = [];
		closeAll(layout);
	}

	function loadMore() {
		displayCount += 10;
	}

	async function startEditName() {
		const thread = layout.threads?.find(isCurrentThread);
		name = thread?.name ?? '';
		editMode = true;
		tick().then(() => input?.focus());
	}

	async function saveName() {
		let thread = layout.threads?.find(isCurrentThread);
		if (!thread) {
			editMode = false;
			return;
		}

		thread.name = name;
		thread = await ChatService.updateThread(project.assistantID, project.id, thread);
		layout.threads?.forEach((t, i) => {
			if (t.id === thread.id) {
				layout.threads![i] = thread;
			}
		});
		editMode = false;
	}

	export async function createThread() {
		const thread = await ChatService.createThread(project.assistantID, project.id);
		const found = layout.threads?.find((t) => t.id === thread.id);
		if (!found) {
			layout.threads?.splice(0, 0, thread);
		}
		if (responsive.isMobile) {
			layout.sidebarOpen = false;
		}
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
		setCurrentThread(layout.threads?.[0]?.id ?? '');
	}

	function selectThread(id: string) {
		if (editMode) {
			return;
		}

		if (responsive.isMobile) {
			layout.sidebarOpen = false;
		}

		closeAll(layout);
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

	async function watchThreads(): Promise<void> {
		if (watchingThread) {
			return;
		}

		watchingThread = ChatService.watchThreads(project.assistantID, project.id, (thread) => {
			if (thread.deleted) {
				console.log('deleted thread', thread.id);
				layout.threads = layout.threads?.filter((t) => t.id !== thread.id);
				layout.taskRuns = layout.taskRuns?.filter((t) => t.id !== thread.id);
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
		if (!isOpen) {
			layout.sidebarOpen = false;
		}
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
						<Plus class="size-4" /> Start New Thread
					</button>
				</div>
			{/if}
		</CollapsePane>
	{:else}
		<div class="flex flex-col text-xs">
			<div class="flex items-center justify-between">
				<p class="text-md grow font-medium">Threads</p>
				<button
					class="p-2 text-gray-400 transition-colors duration-200 hover:text-black dark:text-gray-600 dark:hover:text-white"
					onclick={createThread}
					use:tooltip={'Start New Thread'}
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
		{#each (layout.threads ?? []).slice(0, displayCount) as thread (thread.id)}
			<li
				class:bg-surface2={isCurrentThread(thread)}
				class="group hover:bg-surface3 flex min-h-9 items-center gap-3 rounded-md font-light"
			>
				{#if editMode && isCurrentThread(thread)}
					<input
						bind:value={name}
						bind:this={input}
						onkeyup={(e) => {
							switch (e.key) {
								case 'Escape':
									editMode = false;
									break;
								case 'Enter':
									saveName();
									break;
							}
						}}
						class="mx-2 w-0 grow border-none bg-transparent ring-0 outline-hidden dark:text-white"
						placeholder="Enter name"
						type="text"
					/>
				{:else}
					<button
						use:overflowToolTip
						class:font-medium={isCurrentThread(thread)}
						class="h-full grow p-2 text-start"
						onclick={() => selectThread(thread.id)}
					>
						{thread.name || 'New Thread'}
					</button>
				{/if}
				{#if isCurrentThread(thread) && editMode}
					<button class="list-button-primary" onclick={() => (editMode = false)}>
						<CircleX class="h-4 w-4" />
					</button>
					<button class="list-button-primary" onclick={saveName}>
						<Save class="mr-2 h-4 w-4" />
					</button>
				{:else}
					<DotDotDot
						class="p-0 pr-2.5 transition-opacity duration-200 group-hover:opacity-100 md:opacity-0"
					>
						<div class="default-dialog flex min-w-max flex-col p-2">
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
