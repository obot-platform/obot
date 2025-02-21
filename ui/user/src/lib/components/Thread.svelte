<script lang="ts">
	import { sticktobottom } from '$lib/actions/div';
	import Input from '$lib/components/messages/Input.svelte';
	import Message from '$lib/components/messages/Message.svelte';
	import { toHTMLFromMarkdown } from '$lib/markdown';
	import { type Assistant, EditorService, type Messages } from '$lib/services';
	import { Thread } from '$lib/services/chat/thread.svelte';
	import { assistants, context } from '$lib/stores';
	import { onDestroy } from 'svelte';
	import { fade } from 'svelte/transition';

	interface Props {
		id?: string;
	}

	let container = $state<HTMLDivElement>();
	let messages: Messages = $state({ messages: [], inProgress: false });
	let thread: Thread | undefined = $state<Thread>();
	let messagesDiv = $state<HTMLDivElement>();
	let currentAssistant = $state<Assistant>();
	let { id }: Props = $props();
	let intro = $derived(
		context.project?.introductionMessage || currentAssistant?.introductionMessage || undefined
	);
	let starters = $derived.by(() => {
		if (context.project?.starterMessages && context.project?.starterMessages.length > 0) {
			return context.project?.starterMessages;
		}
		return currentAssistant?.starterMessages;
	});

	let scrollSmooth = $state(false);
	$effect(() => {
		const update = () => (scrollSmooth = true);
		container?.addEventListener('scroll', update);
		return () => {
			container?.removeEventListener('scroll', update);
			scrollSmooth = false;
		};
	});

	$effect(() => {
		const a = assistants.current();
		if (a) {
			currentAssistant = a;
		} else {
			return;
		}

		if (thread && thread.threadID !== id) {
			thread?.close?.();
			thread = undefined;
			messages = {
				messages: [],
				inProgress: false
			};
		}

		if (thread || !id) {
			return;
		}

		const newThread = new Thread({
			threadID: id,
			onError: () => {
				// ignore errors they are rendered as messages
			}
		});

		messages = {
			messages: [],
			inProgress: false
		};
		newThread.onMessages = (newMessages) => {
			messages = newMessages;
		};

		thread = newThread;
	});

	onDestroy(() => {
		thread?.close?.();
	});

	function onLoadFile(filename: string) {
		EditorService.load(filename);
	}

	function onSendCredentials(id: string, credentials: Record<string, string>) {
		thread?.sendCredentials(id, credentials);
	}
</script>

<div class="relative w-full">
	<div
		bind:this={container}
		class="flex h-full grow justify-center overflow-y-auto scrollbar-none"
		class:scroll-smooth={scrollSmooth}
		use:sticktobottom
	>
		<div
			in:fade|global
			bind:this={messagesDiv}
			class="flex w-full max-w-[1000px] flex-col justify-start gap-8 p-5 transition-all"
			class:justify-center={!thread}
		>
			<div class="message-content self-center">
				{#if intro}
					{@html toHTMLFromMarkdown(intro)}
				{/if}
			</div>
			<div class="grid gap-2 self-center md:grid-cols-3">
				{#if thread}
					{#each starters ?? [] as msg}
						<button
							class="rounded-3xl border-2 border-blue p-5 hover:bg-surface1"
							onclick={() => {
								thread?.invoke(msg);
							}}
						>
							{msg}
						</button>
					{/each}
				{/if}
			</div>
			{#each messages.messages as msg}
				<Message
					{msg}
					{onLoadFile}
					{onSendCredentials}
					onSendCredentialsCancel={() => thread?.abort()}
				/>
			{/each}
			<div class="min-h-28">
				<!-- Vertical Spacer -->
			</div>
		</div>
		<div
			class="absolute inset-x-0 bottom-0 z-10 flex justify-center bg-gradient-to-t from-white px-3 pb-8 pt-10 dark:from-black"
		>
			{#if thread}
				<Input
					readonly={messages.inProgress}
					pending={thread?.pending}
					onAbort={async () => {
						await thread?.abort();
					}}
					onSubmit={async (i) => {
						container?.scrollTo({ top: container?.scrollHeight - container?.clientHeight });
						await thread?.invoke(i);
					}}
				/>
			{/if}
		</div>
	</div>
</div>
