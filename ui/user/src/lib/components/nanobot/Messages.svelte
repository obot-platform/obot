<script lang="ts">
	import Message from './Message.svelte';
	import type {
		ChatMessage,
		ChatMessageItem,
		ChatResult,
		Agent,
		Attachment
	} from '$lib/services/nanobot/types';
	import AgentHeader from '$lib/components/nanobot/AgentHeader.svelte';

	interface Props {
		messages: ChatMessage[];
		onSend?: (message: string, attachments?: Attachment[]) => Promise<ChatResult | void>;
		onFileOpen?: (filename: string) => void;
		isLoading?: boolean;
		agent?: Agent;
		hideAgentHeader?: boolean;
	}

	let {
		messages,
		onFileOpen,
		onSend,
		isLoading = false,
		agent,
		hideAgentHeader = false
	}: Props = $props();

	let messageGroups = $derived.by(() => {
		return messages.reduce((acc, message) => {
			if (message.role === 'user' || acc.length === 0) {
				acc.push([message]);
			} else {
				acc[acc.length - 1].push(message);
			}
			return acc;
		}, [] as ChatMessage[][]);
	});

	let displayMessageGroups = $derived.by((): ChatMessage[][] => {
		return messageGroups.map((group) => {
			const out: ChatMessage[] = [];
			let assistantAccum: ChatMessageItem[] = [];
			let assistantAccumIds: string[] = [];
			let assistantAccumCreated: string | undefined;
			const flushAssistant = () => {
				if (assistantAccum.length === 0) return;
				out.push({
					id: `merged-assistant-${assistantAccumIds.join('-')}`,
					created: assistantAccumCreated,
					role: 'assistant',
					items: [...assistantAccum]
				});
				assistantAccum = [];
				assistantAccumIds = [];
				assistantAccumCreated = undefined;
			};
			for (const msg of group) {
				if (msg.role === 'user') {
					flushAssistant();
					out.push(msg);
				} else {
					if (msg.items?.length) {
						assistantAccum.push(...msg.items);
						assistantAccumIds.push(msg.id);
						if (!assistantAccumCreated && msg.created) assistantAccumCreated = msg.created;
					} else {
						flushAssistant();
						out.push(msg);
					}
				}
			}
			flushAssistant();
			return out;
		});
	});

	// Check if any messages have content (text items)
	let hasMessageContent = $derived(
		messageGroups[messageGroups.length - 1]?.some(
			(message) =>
				message.role === 'assistant' &&
				message.items &&
				message.items.some(
					(item) =>
						item.type === 'tool' ||
						(item.type === 'text' && item.text && item.text.trim().length > 0)
				)
		)
	);

	// Show loading indicator when loading and no content has been printed yet
	let showLoadingIndicator = $derived(isLoading && !hasMessageContent);
</script>

<div id="message-groups" class="flex flex-col space-y-4 pt-4">
	{#if messages.length === 0}
		{#if !hideAgentHeader}
			<AgentHeader {agent} {onSend} />
		{/if}
	{:else}
		{@const lastIndex = displayMessageGroups.length - 1}

		{#each displayMessageGroups as displayGroup, i (messageGroups[i]?.[0]?.id)}
			{@const isLast = i === lastIndex}
			{@const messageGroup = messageGroups[i]}

			<div
				id={`group-${messageGroup?.[0]?.id}`}
				class="contents"
				data-message-id={messageGroup?.[0]?.id}
			>
				{#each displayGroup as message (message.id)}
					<Message {message} {onSend} {onFileOpen} />
				{/each}
				{#if isLast}
					{#if showLoadingIndicator}
						<div class="flex w-full items-start gap-3">
							<div class="flex min-w-0 flex-1 flex-col items-start">
								<div class="flex items-center justify-center p-8">
									<span class="loading loading-lg loading-spinner text-base-content/30"></span>
								</div>
							</div>
						</div>
					{/if}
					<div class="h-59"></div>
				{/if}
			</div>
		{/each}
	{/if}
</div>
