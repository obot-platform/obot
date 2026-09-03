<script lang="ts">
	import CopyButton from '$lib/components/CopyButton.svelte';
	import { toHTMLFromMarkdownWithNewTabLinks } from '$lib/markdown';
	import type {
		MCPTesterChat,
		TesterChatTimelineMessage,
		TesterToolApproval
	} from '$lib/services/mcp/tester-chat.svelte';
	import type { MCPTesterSession } from '$lib/services/mcp/tester.svelte';
	import ChatComposer from './ChatComposer.svelte';
	import ToolApprovalPrompt from './ToolApprovalPrompt.svelte';
	import ToolCallRecord from './ToolCallRecord.svelte';
	import { RotateCw } from '@lucide/svelte';

	interface Props {
		chat: MCPTesterChat;
		session: MCPTesterSession;
	}

	let { chat, session }: Props = $props();
	let messagesElement: HTMLElement;
	let latestMessageContent = $derived.by(() => {
		const latest = chat.timeline.at(-1);
		if (!latest) return '';
		// Finished tool calls are appended to the transcript, so they have to move
		// the scroll position too.
		const finished = latest.toolCalls?.filter((call) => call.execution === 'complete').length ?? 0;
		return `${latest.id}:${latest.text?.length ?? 0}:${latest.state ?? ''}:${chat.timeline.length}:${finished}`;
	});

	$effect(() => {
		void latestMessageContent;
		if (!messagesElement) return;

		const frame = requestAnimationFrame(() => {
			messagesElement.scrollTo({ top: messagesElement.scrollHeight, behavior: 'auto' });
		});
		return () => cancelAnimationFrame(frame);
	});

	function completedCalls(message: TesterChatTimelineMessage): TesterToolApproval[] {
		return message.toolCalls?.filter((call) => call.execution === 'complete') ?? [];
	}
</script>

<div class="flex h-full min-h-0 flex-col">
	<h2 class="sr-only">Chat</h2>

	<div
		class="default-scrollbar-thin min-h-0 flex-1 space-y-4 overflow-y-auto pr-1"
		aria-label="Chat messages"
		bind:this={messagesElement}
	>
		{#if chat.timeline.length === 0}
			<div
				class="bg-base-200 dark:bg-base-300 rounded-lg p-6 text-center text-sm text-muted-content"
			>
				Send a message or stage a prompt or text resource to begin.
			</div>
		{/if}

		{#each chat.timeline as message (message.id)}
			{#if message.role === 'user'}
				<article class="ml-auto max-w-[90%] sm:max-w-[80%]" aria-label="User message">
					<div class="bg-base-200 dark:bg-base-300 rounded-lg p-4">
						{#if message.stagedName}
							<p class="mb-2 text-xs font-medium text-muted-content">
								Staged {message.stagedName}
							</p>
						{/if}
						{#each message.content ?? [] as content, index (index)}
							{#if content.type === 'resource'}
								<p class="mb-1 text-xs break-all text-muted-content">
									{content.uri} · {content.mimeType || 'text'}
								</p>
							{/if}
							<p class="whitespace-pre-wrap wrap-break-word">{content.text}</p>
						{/each}
					</div>
				</article>
			{:else if message.role === 'assistant'}
				<article class="max-w-full" aria-label="Assistant message">
					{#if message.stagedName}
						<p class="mb-2 text-xs font-medium text-muted-content">
							Staged {message.stagedName} · assistant
						</p>
					{/if}
					{#if message.text}
						<div
							class="group border-base-300 dark:border-base-400 bg-base-100 relative rounded-lg border p-4 pr-12"
						>
							<div
								class="absolute top-2 right-2 opacity-0 transition-opacity group-hover:opacity-100 focus-within:opacity-100"
							>
								<CopyButton
									noButtonText
									text={message.text}
									tooltipText="Copy response"
									classes={{
										button:
											'bg-base-200 dark:bg-base-300 hover:bg-base-300 dark:hover:bg-base-400 rounded p-1.5'
									}}
								/>
							</div>
							<div class="milkdown-content tester-markdown min-w-0 max-w-none">
								<!-- eslint-disable-next-line svelte/no-at-html-tags -- sanitized by the markdown helper -->
								{@html toHTMLFromMarkdownWithNewTabLinks(message.text)}
							</div>
						</div>
					{:else if message.state === 'streaming'}
						<p class="text-sm text-muted-content">Thinking…</p>
					{/if}
					{#if completedCalls(message).length}
						<ToolCallRecord calls={completedCalls(message)} />
					{/if}
					{#if message.state === 'failed'}
						<div class="notification-error mt-3 p-3" role="alert">
							<p class="font-medium">Response failed</p>
							<p class="mt-1 text-sm">{message.error?.message}</p>
							{#if message.error?.retryable}
								<button class="btn btn-secondary btn-sm mt-3" onclick={() => chat.retry()}>
									<RotateCw class="size-4" aria-hidden="true" /> Retry response
								</button>
							{/if}
						</div>
					{:else if message.state === 'cancelled'}
						<p class="mt-2 text-sm text-muted-content">Generation stopped.</p>
					{/if}
				</article>
			{/if}
		{/each}
	</div>

	{#if chat.status === 'snapshotting' || chat.status === 'streaming'}
		<p class="sr-only" aria-live="polite">
			{chat.status === 'snapshotting' ? 'Snapshotting server tools…' : 'Assistant is responding…'}
		</p>
	{:else if chat.status === 'round-limit' && chat.error}
		<div class="notification-alert mt-4 p-3" role="status">
			<p class="font-medium">Turn stopped</p>
			<p class="mt-1 text-sm">{chat.error.message}</p>
		</div>
	{:else if chat.error && !chat.timeline.some((message) => message.error)}
		<p class="mt-4 text-sm text-error" role="alert">{chat.error.message}</p>
	{/if}

	<ToolApprovalPrompt {chat} />

	<ChatComposer {chat} {session} />
</div>
