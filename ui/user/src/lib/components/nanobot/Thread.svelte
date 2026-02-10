<script lang="ts">
	import Elicitation from '$lib/components/nanobot/Elicitation.svelte';
	import Prompt from '$lib/components/nanobot/Prompt.svelte';
	import type {
		Agent,
		Attachment,
		ChatMessage,
		ChatMessageItem,
		ChatMessageItemToolCall,
		ChatResult,
		ElicitationResult,
		Elicitation as ElicitationType,
		Prompt as PromptType,
		Resource,
		UploadedFile,
		UploadingFile
	} from '$lib/services/nanobot/types';
	import MessageInput from './MessageInput.svelte';
	import Messages from './Messages.svelte';
	import AgentHeader from './AgentHeader.svelte';
	import { ChevronDown } from 'lucide-svelte';
	import type { Snippet } from 'svelte';
	import { slide, fade } from 'svelte/transition';

	interface Props {
		messages: ChatMessage[];
		prompts: PromptType[];
		resources: Resource[];
		elicitations?: ElicitationType[];
		onElicitationResult?: (elicitation: ElicitationType, result: ElicitationResult) => void;
		onSendMessage?: (message: string, attachments?: Attachment[]) => Promise<ChatResult | void>;
		onFileUpload?: (file: File, opts?: { controller?: AbortController }) => Promise<Attachment>;
		onFileOpen?: (filename: string) => void;
		cancelUpload?: (fileId: string) => void;
		uploadingFiles?: UploadingFile[];
		uploadedFiles?: UploadedFile[];
		isLoading?: boolean;
		isRestoring?: boolean;
		agent?: Agent;
		agents?: Agent[];
		selectedAgentId?: string;
		onAgentChange?: (agentId: string) => void;
		emptyStateContent?: Snippet;
		onRestart?: () => void;
		onRefreshResources?: () => void;
		suppressEmptyState?: boolean;
	}

	let {
		// Do not use _chat variable anywhere except these assignments
		messages,
		prompts,
		resources,
		onSendMessage,
		onFileUpload,
		onFileOpen,
		cancelUpload,
		uploadingFiles,
		uploadedFiles,
		elicitations,
		onElicitationResult,
		agent,
		agents = [],
		selectedAgentId = '',
		onAgentChange,
		isLoading,
		isRestoring,
		emptyStateContent,
		onRestart,
		onRefreshResources,
		suppressEmptyState
	}: Props = $props();

	let messagesContainer: HTMLElement;
	let messagesContentInner = $state<HTMLElement | undefined>(undefined);
	let showScrollButton = $state(false);
	let wasRestoring = false;
	let disabledAutoScroll = $state(false);
	const hasMessages = $derived((messages && messages.length > 0) || isRestoring);
	const pinInputToBottom = $derived(hasMessages || !!suppressEmptyState);
	const showInlineAgentHeader = $derived(!hasMessages && !emptyStateContent && !isLoading);
	let selectedPrompt = $state<string | undefined>();

	const selectedPromptData = $derived(
		selectedPrompt && prompts?.length ? prompts.find((p) => p.name === selectedPrompt) : undefined
	);

	const SCROLL_THRESHOLD = 10;

	const isNearBottom = () => {
		if (!messagesContainer) return false;
		const { scrollTop, scrollHeight, clientHeight } = messagesContainer;
		return scrollTop + clientHeight >= scrollHeight - SCROLL_THRESHOLD;
	};

	const getLastTextItem = (items?: ChatMessageItem[]) => {
		if (!items) return undefined;
		let lastTextItem: (typeof items)[number] | undefined;
		if (items && items.length > 0) {
			for (let i = items.length - 1; i >= 0; i--) {
				const item = items[i];
				if (item.type === 'text') {
					lastTextItem = item;
					break;
				}
			}
		}
		return lastTextItem;
	};

	// Content key that changes when the last message or its content changes (streaming, new items, etc.)
	const lastMessageContentKey = $derived.by(() => {
		if (!messages?.length) return '';
		const last = messages[messages.length - 1];
		const itemCount = last.items?.length ?? 0;
		const lastTextItem = getLastTextItem(last.items);
		const lastTextLen =
			lastTextItem && lastTextItem.type === 'text' ? (lastTextItem.text?.length ?? 0) : 0;
		return `${last.id}-${itemCount}-${lastTextLen}`;
	});

	// keeping track of scroll to bottom
	$effect(() => {
		if (!messagesContainer) return;
		void messages.length;
		void lastMessageContentKey;
		const loading = isLoading;
		if (disabledAutoScroll) return;
		if (!loading && !isNearBottom()) return;

		let raf1: number;
		let raf2: number | undefined;
		raf1 = requestAnimationFrame(() => {
			raf2 = requestAnimationFrame(() => {
				if (!messagesContainer) return;
				if (disabledAutoScroll || (!loading && !isNearBottom())) return;
				messagesContainer.scrollTo({
					top: messagesContainer.scrollHeight,
					behavior: loading ? 'auto' : 'smooth'
				});
			});
		});
		return () => {
			cancelAnimationFrame(raf1);
			if (typeof raf2 === 'number') cancelAnimationFrame(raf2);
		};
	});

	// have scroll at bottom after restoring existing session
	$effect(() => {
		const restoring = isRestoring === true;
		const justFinishedRestoring = wasRestoring && !restoring;

		if (!justFinishedRestoring || !messagesContainer || !messages?.length) {
			if (!justFinishedRestoring) wasRestoring = restoring;
			return;
		}
		wasRestoring = restoring;

		let raf1: number;
		let raf2: number | undefined;
		raf1 = requestAnimationFrame(() => {
			raf2 = requestAnimationFrame(() => {
				if (messagesContainer) {
					messagesContainer.scrollTo({
						top: messagesContainer.scrollHeight,
						behavior: 'auto'
					});
				}
			});
		});
		return () => {
			cancelAnimationFrame(raf1);
			if (typeof raf2 === 'number') cancelAnimationFrame(raf2);
		};
	});

	// When the inner content grows (e.g. streaming), keep view pinned to bottom. Observe the
	// element that actually changes height; the scroll container itself does not resize.
	$effect(() => {
		const container = messagesContainer;
		const inner = messagesContentInner ?? container;
		if (!container || !inner) return;
		void lastMessageContentKey;

		const ro = new ResizeObserver(() => {
			if (!container) return;
			if (disabledAutoScroll) return;
			if (isLoading || isNearBottom()) {
				container.scrollTo({ top: container.scrollHeight, behavior: 'auto' });
			}
		});
		ro.observe(inner);
		return () => ro.disconnect();
	});

	// Track processed tool call IDs to avoid re-triggering file open (non-reactive object)
	const processedWriteToolCalls: Record<string, boolean> = {};

	// Watch for "write" tool calls with file_path argument while loading
	$effect(() => {
		if (!isLoading || !messages || messages.length === 0) return;

		// Find all tool calls in the messages
		for (const message of messages) {
			if (message.role !== 'assistant' || !message.items) continue;

			for (const item of message.items) {
				if (item.type !== 'tool') continue;

				const toolCall = item as ChatMessageItemToolCall;
				if (toolCall.name !== 'write' || !toolCall.arguments) continue;

				// Wait until the tool call is complete (hasMore is false/undefined)
				if (toolCall.hasMore) continue;

				// Skip if we've already processed this tool call
				const toolCallId = toolCall.callID || item.id;
				if (processedWriteToolCalls[toolCallId]) continue;

				// Parse arguments to get file_path
				try {
					const args = JSON.parse(toolCall.arguments);
					if (args.file_path) {
						// Mark as processed (mutate directly, not reactive)
						processedWriteToolCalls[toolCallId] = true;

						// Defer side effects to avoid issues during render
						queueMicrotask(() => {
							onRefreshResources?.();
						});
					}
				} catch {
					// Ignore JSON parse errors
				}
			}
		}
	});

	function handleScroll() {
		if (!messagesContainer) return;

		const { scrollTop, scrollHeight, clientHeight } = messagesContainer;
		const nearBottom = scrollTop + clientHeight >= scrollHeight - SCROLL_THRESHOLD;
		showScrollButton = !nearBottom;
		if (nearBottom) {
			disabledAutoScroll = false;
		} else if (isLoading) {
			disabledAutoScroll = true;
		}
	}

	function scrollToBottom() {
		if (messagesContainer) {
			disabledAutoScroll = false;
			messagesContainer.scrollTo({
				top: messagesContainer.scrollHeight,
				behavior: 'smooth'
			});
		}
	}
</script>

<div
	class="flex h-[calc(100dvh-4rem)] w-full flex-col transition-transform md:relative peer-[.workspace]:md:w-1/4"
>
	<!-- Messages area - full height scrollable with bottom padding for floating input -->
	<div class="w-full overflow-y-auto px-4" bind:this={messagesContainer} onscroll={handleScroll}>
		<div class="mx-auto max-w-4xl" bind:this={messagesContentInner}>
			<!-- Prompts section - show when prompts available and no messages -->
			{#if prompts && prompts.length > 0}
				<div class="mb-6">
					<div class="grid grid-cols-1 gap-4 md:grid-cols-2 lg:grid-cols-3">
						{#if selectedPromptData}
							<Prompt
								prompt={selectedPromptData}
								onSend={async (m) => {
									selectedPrompt = undefined;
									if (onSendMessage) {
										return await onSendMessage(m);
									}
								}}
								onCancel={() => (selectedPrompt = undefined)}
								open
							/>
						{/if}
					</div>
				</div>
			{/if}

			<Messages
				{messages}
				onSend={onSendMessage}
				{isLoading}
				{agent}
				{onFileOpen}
				hideAgentHeader
			/>
		</div>
	</div>

	<!-- Message input - centered when no messages, bottom when messages exist or when empty state is suppressed -->
	<div
		class="absolute right-0 bottom-0 left-0 flex flex-col transition-all duration-500 ease-in-out {pinInputToBottom
			? 'bg-base-100/80 backdrop-blur-sm'
			: 'md:-translate-y-1/2 [@media(min-height:900px)]:md:top-1/2 [@media(min-height:900px)]:md:bottom-auto'}"
	>
		<!-- Scroll to bottom button -->
		{#if showScrollButton && hasMessages}
			<button
				class="btn btn-circle border-base-300 bg-base-100 btn-md mx-auto shadow-lg active:translate-y-0.5"
				onclick={scrollToBottom}
				aria-label="Scroll to bottom"
			>
				<ChevronDown class="size-5" />
			</button>
		{/if}
		{#if showInlineAgentHeader}
			<div class="mx-auto w-full max-w-4xl" out:slide={{ axis: 'y', duration: 300 }}>
				<div out:fade={{ duration: 200 }}>
					<AgentHeader {agent} onSend={onSendMessage} />
				</div>
			</div>
		{:else if !hasMessages && !isLoading && emptyStateContent && !suppressEmptyState}
			<div class="mx-auto w-full max-w-4xl" out:slide={{ axis: 'y', duration: 300 }}>
				<div out:fade={{ duration: 200 }}>
					{@render emptyStateContent()}
				</div>
			</div>
		{/if}
		<div class="mx-auto w-full max-w-4xl">
			<MessageInput
				placeholder={`Type your message...${prompts && prompts.length > 0 ? ' or / for prompts' : ''}`}
				onSend={onSendMessage}
				{resources}
				{messages}
				{agents}
				{selectedAgentId}
				{onAgentChange}
				onPrompt={(p) => (selectedPrompt = p)}
				{onFileUpload}
				disabled={isLoading}
				{prompts}
				{cancelUpload}
				{uploadingFiles}
				{uploadedFiles}
				{onRestart}
			/>
		</div>
	</div>

	{#if elicitations && elicitations.length > 0}
		{#key elicitations[0].id}
			<Elicitation
				elicitation={elicitations[0]}
				open
				onresult={(result) => {
					onElicitationResult?.(elicitations[0], result);
				}}
			/>
		{/key}
	{/if}
</div>
