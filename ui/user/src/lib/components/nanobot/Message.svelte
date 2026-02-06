<script lang="ts">
	import MessageItem from './MessageItem.svelte';
	import type { Attachment, ChatMessage, ChatResult } from '$lib/services/nanobot/types';
	import MessageItemText from './MessageItemText.svelte';

	interface Props {
		message: ChatMessage;
		timestamp?: Date;
		onSend?: (message: string, attachments?: Attachment[]) => Promise<ChatResult | void>;
		onFileOpen?: (filename: string) => void;
	}

	let { message, timestamp, onSend, onFileOpen }: Props = $props();

	const displayTime = $derived(
		timestamp || (message.created ? new Date(message.created) : new Date())
	);
	const toolCall = $derived.by(() => {
		try {
			const item = message.items?.[0];
			return message.role === 'user' && item?.type === 'text' ? JSON.parse(item.text) : null;
		} catch {
			// ignore parse error
			return null;
		}
	});

	const promptDisplayItem = $derived.by(() => {
		const promptText = toolCall?.type === 'prompt' ? toolCall.payload?.prompt : undefined;
		if (message.role !== 'user' || !promptText) return null;
		return {
			id: `${message.id}-prompt`,
			type: 'text' as const,
			text: promptText
		};
	});
</script>

{#if promptDisplayItem}
	<MessageItemText item={promptDisplayItem} role="user" />
{:else if message.role === 'user' && toolCall?.type === 'tool' && toolCall.payload?.toolName}
	<!-- Don't print anything for tool calls -->
{:else if message.role === 'user'}
	<div class="group flex w-full justify-end">
		<div class="max-w-md">
			<div class="flex flex-col items-end">
				<div class="rounded-box bg-base-200 p-2">
					{#if message.items && message.items.length > 0}
						{#each message.items as item (item.id)}
							<MessageItem {item} role={message.role} />
						{/each}
					{:else}
						<!-- Fallback for messages without items -->
						<p>No content</p>
					{/if}
				</div>
				<div
					class="transition-duration-500 mb-1 text-sm font-medium opacity-0 transition-opacity group-hover:opacity-100"
				>
					<time class="ml-2 text-xs opacity-50">{displayTime.toLocaleTimeString()}</time>
				</div>
			</div>
		</div>
	</div>
{:else}
	<div class="flex w-full items-start gap-3">
		<!-- Assistant message content -->
		<div class="flex min-w-0 flex-1 flex-col items-start">
			<!-- Render all message items -->
			{#if message.items && message.items.length > 0}
				{#each message.items as item (item.id)}
					<MessageItem {item} role={message.role} {onSend} {onFileOpen} />
				{/each}
			{:else}
				<!-- Fallback for messages without items -->
				<div class="prose bg-base-200 prose-invert w-full max-w-full rounded-lg p-3">
					<p>No content</p>
				</div>
			{/if}
		</div>
	</div>
{/if}
