<script lang="ts">
	import { MessageCirclePlus, PanelLeftClose, Pen, Save, Trash2 } from 'lucide-svelte';
	import { ChatService, type Thread } from '$lib/services';
	import { threads, context } from '$lib/stores';
	import { tick } from 'svelte';
	import { CircleX } from 'lucide-svelte/icons';
	import { columnResize } from '$lib/actions/resize';

	let panel = $state<HTMLDivElement>();
	let input = $state<HTMLInputElement>();
	let editMode = $state(false);
	let name = $state('');
	let isOpen = $state(false);

	function isCurrentThread(thread: Thread) {
		return context.currentThreadID === thread.id;
	}

	function setCurrentThread(id: string) {
		context.currentThreadID = id;
	}

	async function startEditName() {
		const thread = threads.items.find(isCurrentThread);
		name = thread?.name ?? '';
		editMode = true;
		tick().then(() => input?.focus());
	}

	async function saveName() {
		let thread = threads.items.find(isCurrentThread);
		if (!thread) {
			editMode = false;
			return;
		}

		thread.name = name;
		thread = await ChatService.updateThread(thread);
		threads.items.forEach((t, i) => {
			if (t.id === thread.id) {
				threads.items[i] = thread;
			}
		});
		editMode = false;
	}

	async function createThread() {
		const thread = await ChatService.createThread();
		threads.items.splice(0, 0, thread);
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
		await ChatService.deleteThread(id);
		threads.items = threads.items.filter((thread) => thread.id !== id);
		setCurrentThread(threads.items[0]?.id ?? '');
	}

	function selectThread(id: string) {
		if (editMode) {
			return;
		}
		setCurrentThread(id);
		focusChat();
	}

	async function open() {
		threads.items = (await ChatService.listThreads()).items;
		togglePanel();
	}

	function togglePanel() {
		panel?.classList.toggle('hidden');
		panel?.classList.toggle('flex');
		isOpen = !isOpen;
		if (!isOpen) {
			context.sidebarOpen = false;
		}
		focusChat();
	}

	$effect(() => {
		if (context.sidebarOpen && !isOpen) {
			open();
		}
	});
</script>

<div bind:this={panel} class="hidden h-full w-[320px] min-w-[320px] flex-col bg-surface1 p-5">
	<div class="mb-5 flex items-center gap-4">
		<h2 class="text-lg">Threads</h2>
		<button class="text-gray" onclick={createThread}>
			<MessageCirclePlus class="h-5 w-5" />
		</button>
		<button onclick={togglePanel} class="ml-auto">
			<PanelLeftClose class="h-5 w-5 text-gray" />
		</button>
	</div>
	<ul class="flex flex-col">
		{#each threads.items as thread}
			<li
				class="flex items-center gap-2 rounded-lg px-3 py-2 {isCurrentThread(thread)
					? 'bg-gray-100 dark:bg-gray-900'
					: ''}"
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
					<button class="grow text-left" onclick={() => selectThread(thread.id)}
						>{thread.name || 'New Thread'}</button
					>
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
