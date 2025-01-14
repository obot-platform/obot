<script lang="ts">
	import Input from '$lib/components/messages/Input.svelte';
	import { autoscroll } from '$lib/actions/div';
	import { Thread } from '$lib/services/chat/thread.svelte';
	import { type Assistant, EditorService, type Messages } from '$lib/services';
	import Message from '$lib/components/messages/Message.svelte';
	import { fade } from 'svelte/transition';
	import { onDestroy } from 'svelte';
	import { currentAssistant, assistants } from '$lib/stores';
	import { toHTMLFromMarkdown } from '$lib/markdown';

	interface Props {
		assistant?: string;
	}

	let { assistant = '' }: Props = $props();
	let messages: Messages = $state({ messages: [], inProgress: false });
	let thread: Thread | undefined = $state<Thread>();
	let messagesDiv = $state<HTMLDivElement>();
	let current = $derived.by<Assistant | undefined>(() => {
		let a = $assistants.find((a) => a.id === assistant);
		if (!a && $currentAssistant.id === assistant) {
			a = $currentAssistant;
		}
		return a;
	});

	$effect(() => {
		if (!assistant || thread) {
			return;
		}

		const newThread = new Thread(assistant, {
			onError: () => {
				// ignore errors they are rendered as messages
			}
		});

		newThread.onMessages = (newMessages) => {
			messages = newMessages;
		};

		thread = newThread;
	});

	onDestroy(() => {
		thread?.close?.();
	});

	function onLoadFile(filename: string) {
		if (assistant) {
			EditorService.load(assistant, filename);
		}
	}
</script>

<div>
	<div
		bind:this={messagesDiv}
		class="flex h-dvh w-full justify-center overflow-auto transition-all scrollbar-none"
		use:autoscroll
	>
		<div class="flex w-full max-w-[900px] flex-col px-8 pt-24 transition-all">
			<div in:fade|global class="flex flex-col gap-8">
				{#if messages.messages.length < 7}
					<div class="message-content self-center">
						{#if current?.introductionMessage}
							{@html toHTMLFromMarkdown(current.introductionMessage)}
						{/if}
					</div>
					<div class="flex gap-2 self-center">
						{#each current?.starterMessages ?? [] as msg}
							<button
								class="rounded-3xl border-2 border-blue p-5"
								onclick={() => {
									thread?.invoke(msg);
								}}
							>
								{msg}
							</button>
						{/each}
					</div>
				{/if}
				{#each messages.messages as msg}
					<Message {msg} {onLoadFile} />
				{/each}
			</div>
			<div class="h-28 w-full flex-shrink-0"></div>
		</div>
	</div>
	<div
		class="absolute inset-x-0 bottom-0 z-30 flex justify-center bg-gradient-to-t from-white px-3 pb-8 pt-10 dark:from-black"
	>
		<Input
			readonly={messages.inProgress}
			pending={thread?.pending}
			onAbort={async () => {
				await thread?.abort();
			}}
			onSubmit={async (i) => {
				messagesDiv?.scrollTo({ top: messagesDiv?.scrollHeight });
				await thread?.invoke(i);
			}}
		/>
	</div>
</div>
