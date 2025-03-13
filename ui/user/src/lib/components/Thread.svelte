<script lang="ts">
	import { stickToBottom, type StickToBottomControls } from '$lib/actions/div.svelte';
	import Input from '$lib/components/messages/Input.svelte';
	import Message from '$lib/components/messages/Message.svelte';
	import { Thread } from '$lib/services/chat/thread.svelte';
	import {
		type AssistantTool,
		ChatService,
		EditorService,
		type Messages,
		type Project,
		type Version
	} from '$lib/services';
	import { fade } from 'svelte/transition';
	import { onDestroy } from 'svelte';
	import { toHTMLFromMarkdown } from '$lib/markdown';
	import { getLayout } from '$lib/context/layout.svelte';
	import Files from '$lib/components/edit/Files.svelte';
	import Tools from '$lib/components/navbar/Tools.svelte';
	import type { UIEventHandler } from 'svelte/elements';
	import AssistantIcon from '$lib/icons/AssistantIcon.svelte';

	interface Props {
		id?: string;
		project: Project;
		tools: AssistantTool[];
		version: Version;
	}

	let { id = $bindable(), project, version, tools }: Props = $props();

	let container = $state<HTMLDivElement>();
	let messages = $state<Messages>({ messages: [], inProgress: false });
	let thread = $state<Thread>();
	let messagesDiv = $state<HTMLDivElement>();
	let scrollSmooth = $state(false);

	$effect(() => {
		// Close and recreate thread if id changes
		if (thread && thread.threadID !== id) {
			scrollSmooth = false;
			thread?.close?.();
			thread = undefined;
			messages = {
				messages: [],
				inProgress: false
			};
		}

		scrollSmooth = false;

		if (id && !thread) {
			constructThread();
		}
	});

	let scrollControls = $state<StickToBottomControls>();

	onDestroy(() => {
		thread?.close?.();
	});

	const layout = getLayout();
	function onLoadFile(filename: string) {
		EditorService.load(layout.items, project, filename, {
			threadID: id
		});
		layout.fileEditorOpen = true;
	}

	async function ensureThread() {
		if (thread && thread.closed && id) {
			await constructThread();
		}
		if (!id) {
			id = (await ChatService.createThread(project.assistantID, project.id)).id;
			await constructThread();
		}
	}

	async function constructThread() {
		const newThread = new Thread(project, {
			threadID: id,
			onError: () => {
				// ignore errors they are rendered as messages
			},
			onClose: () => {
				// false means don't reconnect
				return false;
			},
			items: layout.items
		});

		messages = {
			messages: [],
			inProgress: false
		};
		newThread.onMessages = (newMessages) => {
			messages = newMessages;
		};

		thread = newThread;
	}

	const onScrollEnd: UIEventHandler<HTMLDivElement> = (e) => {
		const isAtBottom =
			e.currentTarget.scrollHeight - e.currentTarget.scrollTop - e.currentTarget.clientHeight <= 0;

		if (isAtBottom) {
			scrollSmooth = true;
		}
	};

	function onSendCredentials(id: string, credentials: Record<string, string>) {
		thread?.sendCredentials(id, credentials);
	}
</script>

<div class="relative w-full pb-32">
	<div
		class="absolute inset-x-0 bottom-32 z-30 h-14 w-full bg-gradient-to-t from-white dark:from-black"
	></div>
	<div
		bind:this={container}
		class="flex h-full grow justify-center overflow-y-auto scrollbar-none"
		class:scroll-smooth={scrollSmooth}
		use:stickToBottom={{
			contentEl: messagesDiv,
			setControls: (controls) => (scrollControls = controls)
		}}
		onscrollend={onScrollEnd}
	>
		<div
			in:fade|global
			bind:this={messagesDiv}
			class="flex h-fit w-full max-w-[1000px] flex-col justify-start gap-8 p-5 transition-all"
			class:justify-center={!thread}
		>
			<div class="message-content self-center">
				<div class="flex flex-col items-center justify-center pt-8 text-center">
					<AssistantIcon {project} class="h-24 w-24 shadow-lg" />
					<h4 class="!mb-1">{project.name || 'Untitled'}</h4>
					{#if project.description}
						<p class="max-w-md text-gray">{project.description}</p>
					{/if}
					<div class="mt-4 h-[1px] w-96 max-w-sm rounded-full bg-surface1 dark:bg-surface2"></div>
				</div>
				{#if project?.introductionMessage}
					<div class="pt-8">
						{@html toHTMLFromMarkdown(project?.introductionMessage)}
					</div>
				{/if}
			</div>
			<div class="flex flex-wrap justify-center gap-4 px-4">
				{#each project.starterMessages ?? [] as msg}
					<button
						class="w-52 rounded-2xl border border-surface3 bg-transparent p-4 text-left text-sm font-light transition-all duration-300 hover:bg-surface2"
						onclick={async () => {
							await ensureThread();
							await thread?.invoke(msg);
						}}
					>
						<span class="line-clamp-3">{msg}</span>
					</button>
				{/each}
			</div>
			{#each messages.messages as msg}
				<Message
					{project}
					{msg}
					{onLoadFile}
					{onSendCredentials}
					onSendCredentialsCancel={() => thread?.abort()}
				/>
			{/each}
			<div class="min-h-16">
				<!-- Vertical Spacer -->
			</div>
		</div>
		<div class="absolute inset-x-0 bottom-0 z-30 flex justify-center py-8">
			<Input
				readonly={messages.inProgress}
				pending={thread?.pending}
				onAbort={async () => {
					await thread?.abort();
				}}
				onSubmit={async (i) => {
					await ensureThread();
					scrollSmooth = false;
					scrollControls?.stickToBottom();
					await thread?.invoke(i);
				}}
				bind:items={layout.items}
			>
				<div class="flex w-fit items-center gap-1">
					<Files thread {project} bind:currentThreadID={id} />
					<Tools {project} {version} {tools} />
				</div>
			</Input>
		</div>
	</div>
</div>
