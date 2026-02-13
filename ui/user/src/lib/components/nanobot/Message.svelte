<script lang="ts">
	import MessageItem from './MessageItem.svelte';
	import type {
		Attachment,
		ChatMessage,
		ChatMessageItem,
		ChatResult
	} from '$lib/services/nanobot/types';
	import MessageItemText from './MessageItemText.svelte';
	import { CANCELLATION_PHRASE_CLIENT, isCancellationError } from '$lib/services/nanobot/utils';

	interface Props {
		message: ChatMessage;
		timestamp?: Date;
		onSend?: (message: string, attachments?: Attachment[]) => Promise<ChatResult | void>;
		onFileOpen?: (filename: string) => void;
	}

	let { message, timestamp, onSend, onFileOpen }: Props = $props();

	function isMessageItemTool(item: ChatMessageItem): boolean {
		return item.type === 'tool' && item.name !== 'write';
	}

	/** Groups items so consecutive MessageItemTool items are in one group for collapse. */
	const itemGroups = $derived.by(
		(): Array<{ toolGroup: ChatMessageItem[] } | { single: ChatMessageItem }> => {
			const items = message.items ?? [];
			if (items.length === 0) return [];
			const groups: ({ toolGroup: ChatMessageItem[] } | { single: ChatMessageItem })[] = [];
			let i = 0;
			while (i < items.length) {
				if (isMessageItemTool(items[i])) {
					const run: ChatMessageItem[] = [];
					while (i < items.length && isMessageItemTool(items[i])) {
						run.push(items[i]);
						i++;
					}
					groups.push({ toolGroup: run });
				} else {
					groups.push({ single: items[i] });
					i++;
				}
			}
			return groups;
		}
	);

	function groupKey(group: { toolGroup: ChatMessageItem[] } | { single: ChatMessageItem }): string {
		return 'toolGroup' in group
			? `tool-${group.toolGroup.map((i) => i.id).join('-')}`
			: `single-${group.single.id}`;
	}

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

	function isCancelledErrorResource(item: ChatMessageItem): boolean {
		if (item.type !== 'resource') return false;
		const mime = item.resource.mimeType;
		return mime === 'application/vnd.nanobot.error+json' && isCancellationError(item.resource.text);
	}

	function isCancelledTextItem(item: ChatMessageItem): boolean {
		if (item.type !== 'text') return false;
		return item.text?.includes(CANCELLATION_PHRASE_CLIENT) ?? false;
	}

	const hasCancelledResource = $derived.by(
		() =>
			message.role === 'assistant' &&
			(message.items ?? []).some(
				(item) => isCancelledErrorResource(item) || isCancelledTextItem(item)
			)
	);
</script>

{#if promptDisplayItem}
	<MessageItemText item={promptDisplayItem} role="user" />
{:else if message.role === 'user' && toolCall?.type === 'tool' && toolCall.payload?.toolName}
	<!-- Don't print anything for tool calls -->
{:else if message.role === 'user'}
	<div class="group flex w-full justify-end">
		<div class="max-w-md">
			<div class="flex flex-col items-end">
				<div class="rounded-box bg-base-200 mt-4 p-2">
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
	<div class="flex w-full items-start gap-3" class:opacity-30={hasCancelledResource}>
		<!-- Assistant message content -->
		<div class="flex min-w-0 flex-1 flex-col items-start">
			<!-- Render all message items (consecutive tool items grouped in one collapse) -->
			{#if message.items && message.items.length > 0}
				<div class="w-full">
					{#each itemGroups as group (groupKey(group))}
						{#if 'toolGroup' in group}
							{@const isThinking = group.toolGroup.some(
								(item) => item.type === 'tool' && !item.output
							)}
							<div
								class="hover:collapse-arrow hover:border-base-300 collapse w-full border border-transparent"
							>
								<input type="checkbox" aria-label="Toggle tool group details" />
								<div
									class="collapse-title text-base-content/35 min-h-0 py-2 text-xs font-light italic"
								>
									{#if isThinking}
										<span class="skeleton skeleton-text bg-transparent">Thinking...</span>
									{:else}
										{`${group.toolGroup.length} tool call${group.toolGroup.length === 1 ? '' : 's'} completed`}
									{/if}
								</div>
								<div class="collapse-content">
									<div>
										{#each group.toolGroup as item (item.id)}
											<MessageItem {item} role={message.role} {onSend} {onFileOpen} />
										{/each}
									</div>
								</div>
							</div>
						{:else}
							<MessageItem item={group.single} role={message.role} {onSend} {onFileOpen} />
						{/if}
					{/each}
				</div>
			{:else}
				<!-- Fallback for messages without items -->
				<div class="prose bg-base-200 prose-invert w-full max-w-full rounded-lg p-3">
					<p>No content</p>
				</div>
			{/if}
		</div>
	</div>
{/if}
