<script lang="ts">
	import type {
		Attachment,
		ChatMessage,
		ChatMessageItem,
		ChatResult,
		ResourceContents
	} from '$lib/services/nanobot/types';
	import {
		CANCELLATION_PHRASE_CLIENT,
		isCancellationError,
		parseToolFilePath
	} from '$lib/services/nanobot/utils';
	import MessageItem from './MessageItem.svelte';
	import MessageItemText from './MessageItemText.svelte';
	import { SvelteSet } from 'svelte/reactivity';

	interface Props {
		message: ChatMessage;
		timestamp?: Date;
		onSend?: (message: string, attachments?: Attachment[]) => Promise<ChatResult | void>;
		onFileOpen?: (filename: string) => void;
		onReadResource?: (uri: string) => Promise<{ contents: ResourceContents[] }>;
	}

	let { message, timestamp, onSend, onFileOpen, onReadResource }: Props = $props();

	let viewableMessageItems = $derived(
		message.items?.filter((item) => item.type !== 'reasoning') ?? []
	);
	let openToolGroups = new SvelteSet<string>();

	function toolGroupOpenKey(groupIndex: number): string {
		return `${message.id}-tool-group-${groupIndex}`;
	}

	function toggleToolGroupOpen(key: string): void {
		if (openToolGroups.has(key)) {
			openToolGroups.delete(key);
		} else {
			openToolGroups.add(key);
		}
	}

	function isVisibleWrittenFile(item: ChatMessageItem): boolean {
		if (item.type !== 'tool' || item.name !== 'write') return false;
		const filePath = item.arguments ? parseToolFilePath(item) : null;
		return !!filePath && !filePath.includes('/.nanobot/');
	}

	function isMessageItemTool(item: ChatMessageItem): boolean {
		return item.type === 'tool' && !isVisibleWrittenFile(item);
	}

	/** Groups items so consecutive MessageItemTool items are in one group for collapse. */
	const itemGroups = $derived.by(
		(): Array<{ toolGroup: ChatMessageItem[] } | { single: ChatMessageItem }> => {
			const items = viewableMessageItems;
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

	function groupKey(
		group: { toolGroup: ChatMessageItem[] } | { single: ChatMessageItem },
		index: number
	): string {
		return 'toolGroup' in group ? `tool-group-${index}` : `single-${group.single.id}`;
	}

	const displayTime = $derived(
		timestamp || (message.created ? new Date(message.created) : new Date())
	);
	const toolCall = $derived.by(() => {
		try {
			const item = viewableMessageItems[0];
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
			viewableMessageItems.some(
				(item) => isCancelledErrorResource(item) || isCancelledTextItem(item)
			)
	);

	const hasCompactionSummary = $derived(
		viewableMessageItems[0]?._meta?.['ai.nanobot.meta/compaction-summary']
	);

	const isAttachmentContext = $derived(
		viewableMessageItems[0]?._meta?.['ai.nanobot.meta/attachment']
	);
</script>

{#if isAttachmentContext}
	<!-- Hidden attachment context message for model instructions -->
{:else if !hasCompactionSummary}
	{#if promptDisplayItem}
		<MessageItemText item={promptDisplayItem} role="user" />
	{:else if message.role === 'user' && toolCall?.type === 'tool' && toolCall.payload?.toolName}
		<!-- Don't print anything for tool calls -->
	{:else if message.role === 'user'}
		<div class="group flex w-full justify-end">
			<div class="max-w-md md:max-w-3/4">
				<div class="flex flex-col items-end">
					<div class="rounded-box bg-base-200 mt-4 p-2">
						{#if viewableMessageItems.length > 0}
							{#each viewableMessageItems as item (item.id)}
								<MessageItem {item} role={message.role} {onFileOpen} {onReadResource} />
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
				{#if viewableMessageItems.length > 0}
					<div class="w-full">
						{#each itemGroups as group, groupIndex (groupKey(group, groupIndex))}
							{#if 'toolGroup' in group}
								{@const isThinking = group.toolGroup.some(
									(item) => item.type === 'tool' && item.hasMore
								)}
								{@const openKey = toolGroupOpenKey(groupIndex)}
								<div
									class="hover:collapse-arrow hover:border-base-300 collapse w-full border border-transparent"
									class:collapse-open={openToolGroups.has(openKey)}
								>
									<input
										type="checkbox"
										aria-label="Toggle tool group details"
										checked={openToolGroups.has(openKey)}
										onchange={() => toggleToolGroupOpen(openKey)}
									/>
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
												<MessageItem
													{item}
													role={message.role}
													{onSend}
													{onFileOpen}
													{onReadResource}
												/>
											{/each}
										</div>
									</div>
								</div>
							{:else}
								<MessageItem
									item={group.single}
									role={message.role}
									{onSend}
									{onFileOpen}
									{onReadResource}
								/>
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
{/if}
