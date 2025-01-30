<script lang="ts">
	import Input from '$lib/components/messages/Input.svelte';
	import { autoscroll } from '$lib/actions/div';
	import { Thread } from '$lib/services/chat/thread.svelte';
	import { type Assistant, EditorService, type Messages } from '$lib/services';
	import Message from '$lib/components/messages/Message.svelte';
	import { fade } from 'svelte/transition';
	import { onDestroy } from 'svelte';
	import { toHTMLFromMarkdown } from '$lib/markdown';
	import { assistants } from '$lib/stores';

	let messages: Messages = $state({ messages: [], inProgress: false });
	let thread: Thread | undefined = $state<Thread>();
	let messagesDiv = $state<HTMLDivElement>();
	let currentAssistant = $state<Assistant>();

	$effect(() => {
		const a = assistants.current();
		if (a) {
			currentAssistant = a;
		} else {
			return;
		}

		if (thread) {
			return;
		}

		const newThread = new Thread({
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
		EditorService.load(filename);
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
				<div class="message-content self-center">
					{#if currentAssistant?.introductionMessage}
						{@html toHTMLFromMarkdown(currentAssistant.introductionMessage)}
					{/if}
				</div>
				<div class="grid gap-2 self-center md:grid-cols-3">
					{#each currentAssistant?.starterMessages ?? [] as msg}
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
