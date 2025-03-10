<script lang="ts">
	import { MessageCirclePlus, Pen, Save, ScrollText, Trash2 } from 'lucide-svelte';
	import { ChatService, type Project, type Thread } from '$lib/services';
	import { tick } from 'svelte';
	import { CircleX } from 'lucide-svelte/icons';
	import { columnResize } from '$lib/actions/resize';
	import { getLayout } from '$lib/context/layout.svelte.js';
	import { fade } from 'svelte/transition';
	import { overflowToolTip } from '$lib/actions/overflow.js';

	interface Props {
		currentThreadID?: string;
		project: Project;
	}

	let { currentThreadID = $bindable(), project }: Props = $props();

	let threads = $state<Thread[]>([]);
	let panel = $state<HTMLDivElement>();
	let input = $state<HTMLInputElement>();
	let editMode = $state(false);
	let name = $state('');
	let isOpen = $state(false);
	let layout = getLayout();
	let lastSeenThreadID = $state('');

	function isCurrentThread(thread: Thread) {
		return currentThreadID === thread.id && layout.editTaskID === undefined;
	}

	function setCurrentThread(id: string) {
		lastSeenThreadID = id;
		currentThreadID = id;
		layout.items = [];
	}

	async function startEditName() {
		const thread = threads.find(isCurrentThread);
		name = thread?.name ?? '';
		editMode = true;
		tick().then(() => input?.focus());
	}

	async function saveName() {
		let thread = threads.find(isCurrentThread);
		if (!thread) {
			editMode = false;
			return;
		}

		thread.name = name;
		thread = await ChatService.updateThread(project.assistantID, project.id, thread);
		threads.forEach((t, i) => {
			if (t.id === thread.id) {
				threads[i] = thread;
			}
		});
		editMode = false;
	}

	export async function createThread() {
		const thread = await ChatService.createThread(project.assistantID, project.id);
		threads.splice(0, 0, thread);
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
		threads = threads.filter((thread) => thread.id !== id);
		setCurrentThread(threads[0]?.id ?? '');
		if (threads.length === 0) {
			togglePanel();
		}
	}

	function selectThread(id: string) {
		if (editMode) {
			return;
		}
		layout.editTaskID = undefined;
		setCurrentThread(id);
		focusChat();
	}

	async function reloadThread() {
		threads = (await ChatService.listThreads(project.assistantID, project.id)).items;
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
		if (currentThreadID) {
			const thread = threads.find((t) => t.id === currentThreadID);
			if (thread) {
				name = thread.name;
			}
		}
	});

	$effect(() => {
		if (currentThreadID && lastSeenThreadID !== currentThreadID) {
			reloadThread();
			setCurrentThread(currentThreadID);
		}
	});
</script>

{#if isOpen}
	<div bind:this={panel} class="flex flex-col">
		<div class="mb-5 flex items-center gap-4">
			<ScrollText class="icon-default text-gray" />
			<h2 class="grow text-lg">Threads</h2>
			<button class="text-gray" onclick={createThread}>
				<MessageCirclePlus class="h-5 w-5" />
			</button>
		</div>
		{#if threads.length === 0}
			<p class="p-6 text-center text-sm text-gray dark:text-gray-300">No threads</p>
		{/if}
		<ul transition:fade>
			{#each threads as thread}
				<li
					class:bg-surface2={isCurrentThread(thread)}
					class="group flex items-center gap-2 rounded-lg px-3 py-2 hover:bg-surface3"
				>
					{#if editMode && isCurrentThread(thread)}
						<!-- I have no idea why w-0 is needed here, otherwise the minimum width is too large -->
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
							class="w-0 grow border-none bg-gray-100 outline-none ring-0 dark:bg-gray-900 dark:text-white"
							placeholder="Enter name"
							type="text"
						/>
					{:else}
						<button
							use:overflowToolTip
							class="grow text-start"
							onclick={() => selectThread(thread.id)}
						>
							{thread.name || 'New Thread'}
						</button>
					{/if}
					{#if isCurrentThread(thread)}
						{#if editMode}
							<button onclick={() => (editMode = false)}>
								<CircleX class="h-4 w-4" />
							</button>
							<button onclick={saveName}>
								<Save class="h-4 w-4" />
							</button>
						{:else}
							<button onclick={startEditName}>
								<Pen class="h-4 w-4" />
							</button>
							<button onclick={() => deleteThread(thread.id)}>
								<Trash2 class="h-4 w-4" />
							</button>
						{/if}
					{/if}
				</li>
			{/each}
		</ul>
	</div>

	<div class="w-2 cursor-col-resize" use:columnResize={panel}></div>
{/if}
