<script lang="ts">
	import BrowserViewer from '$lib/components/nanobot/BrowserViewer.svelte';
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
		ResourceContents,
		UploadedFile,
		UploadingFile
	} from '$lib/services/nanobot/types';
	import { responsive } from '$lib/stores';
	import { clampThreadContentReportedWidth } from '$lib/utils';
	import AgentHeader from './AgentHeader.svelte';
	import MessageInput from './MessageInput.svelte';
	import Messages from './Messages.svelte';
	import { ChevronDown, Upload } from 'lucide-svelte';
	import { untrack, type Snippet } from 'svelte';
	import { slide, fade } from 'svelte/transition';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		messages: ChatMessage[];
		prompts: PromptType[];
		elicitations?: ElicitationType[];
		onElicitationResult?: (elicitation: ElicitationType, result: ElicitationResult) => void;
		onSendMessage?: (message: string, attachments?: Attachment[]) => Promise<ChatResult | void>;
		onFileUpload?: (file: File, opts?: { controller?: AbortController }) => Promise<Attachment>;
		onFileOpen?: (filename: string) => void;
		onReadResource?: (uri: string) => Promise<{ contents: ResourceContents[] }>;
		onCancel?: () => void;
		onContentWidthChange?: (width: number) => void;
		browserBaseUrl?: string;
		browserAvailable?: boolean;
		browserViewerOpen?: boolean;
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
		onRefreshResources?: () => void;
		suppressEmptyState?: boolean;
		classes?: {
			root?: string;
		};
	}

	let {
		// Do not use _chat variable anywhere except these assignments
		messages,
		prompts,
		elicitations,
		onElicitationResult,
		onSendMessage,
		onFileUpload,
		onFileOpen,
		onReadResource,
		onCancel,
		onContentWidthChange,
		browserBaseUrl = '',
		browserAvailable = false,
		browserViewerOpen = $bindable(false),
		cancelUpload,
		uploadingFiles,
		uploadedFiles,
		isLoading,
		isRestoring,
		agent,
		agents = [],
		selectedAgentId = '',
		onAgentChange,
		emptyStateContent,
		onRefreshResources,
		suppressEmptyState,
		classes
	}: Props = $props();

	let messagesContainer: HTMLElement;
	let messagesContentInner = $state<HTMLElement | undefined>(undefined);
	let showScrollButton = $state(false);
	let wasRestoring = false;
	let didInitialScrollToBottom = $state(false);
	let scrollToBottomWhenReady = $state(false);
	let disabledAutoScroll = $state(false);
	const hasMessages = $derived((messages && messages.length > 0) || isRestoring);
	let pinInputToBottom = $derived(hasMessages || !!suppressEmptyState);
	const showInlineAgentHeader = $derived(!emptyStateContent && !isLoading && !pinInputToBottom);
	let selectedPrompt = $state<string | undefined>();
	let browserViewerWidth = $state(50);
	let isResizing = $state(false);

	const selectedPromptData = $derived(
		selectedPrompt && prompts?.length ? prompts.find((p) => p.name === selectedPrompt) : undefined
	);

	// Split elicitations: question type renders inline, others render as modal
	const questionElicitation = $derived(
		elicitations?.find((e) => e._meta?.['ai.nanobot.meta/question']) ?? null
	);
	const modalElicitation = $derived(
		elicitations?.find((e) => !e._meta?.['ai.nanobot.meta/question']) ?? null
	);

	const SCROLL_THRESHOLD = 10;
	const CONTENT_WIDTH_NOTIFY_EPSILON = 6;

	function startResize(e: MouseEvent) {
		isResizing = true;
		e.preventDefault();
	}

	function stopResize() {
		isResizing = false;
	}

	function resize(e: MouseEvent) {
		if (!isResizing) return;
		const container = e.currentTarget as HTMLElement;
		if (!container) return;

		const rect = container.getBoundingClientRect();
		const newWidth = ((rect.right - e.clientX) / rect.width) * 100;
		browserViewerWidth = Math.max(20, Math.min(80, newWidth));
	}

	$effect(() => {
		if (isResizing) {
			window.addEventListener('mouseup', stopResize);
			return () => window.removeEventListener('mouseup', stopResize);
		}
	});

	$effect(() => {
		if (!browserAvailable && browserViewerOpen) {
			browserViewerOpen = false;
		}
	});

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

	// Scroll to bottom after restoring existing session
	$effect(() => {
		const restoring = isRestoring === true;
		const hasMessages = !!messages?.length;
		// reset
		if (!hasMessages || restoring) {
			didInitialScrollToBottom = false;
			scrollToBottomWhenReady = false;
			if (!hasMessages) wasRestoring = false;
		}
		const justFinishedRestoring = wasRestoring && !restoring;
		const shouldScrollOnce =
			hasMessages &&
			!restoring &&
			!didInitialScrollToBottom &&
			(justFinishedRestoring || !wasRestoring);

		if (!justFinishedRestoring) wasRestoring = restoring;
		if (!shouldScrollOnce) return;

		didInitialScrollToBottom = true;
		scrollToBottomWhenReady = true;

		if (!messagesContainer) return;
		const fallbackId = setTimeout(() => {
			if (messagesContainer && scrollToBottomWhenReady) {
				messagesContainer.scrollTo({
					top: messagesContainer.scrollHeight,
					behavior: 'auto'
				});
				scrollToBottomWhenReady = false;
			}
		}, 100);
		return () => clearTimeout(fallbackId);
	});

	$effect(() => {
		const container = messagesContainer;
		const inner = messagesContentInner ?? container;
		if (!container || !inner) return;

		let scrollRaf = 0;
		const ro = new ResizeObserver(() => {
			if (scrollRaf) cancelAnimationFrame(scrollRaf);
			scrollRaf = requestAnimationFrame(() => {
				scrollRaf = 0;
				if (!container) return;
				if (scrollToBottomWhenReady) {
					container.scrollTo({ top: container.scrollHeight, behavior: 'auto' });
					scrollToBottomWhenReady = false;
				}
				if (disabledAutoScroll) return;
				if (!isLoading && !isNearBottom()) return;
				const sh = container.scrollHeight;
				const ch = container.clientHeight;
				const st = container.scrollTop;
				const gap = sh - ch - st;
				if (gap <= SCROLL_THRESHOLD) return;
				container.scrollTo({ top: sh, behavior: 'auto' });
			});
		});
		ro.observe(inner);
		return () => {
			ro.disconnect();
			if (scrollRaf) cancelAnimationFrame(scrollRaf);
		};
	});

	$effect(() => {
		const inner = messagesContentInner;
		if (!inner) return;

		const notify = onContentWidthChange;
		if (!notify) return;

		let lastRounded = -1;
		let widthRaf = 0;

		const flushWidth = () => {
			widthRaf = 0;
			const capped = clampThreadContentReportedWidth(inner.getBoundingClientRect().width);
			if (lastRounded >= 0) {
				if (capped === lastRounded) return;
				if (Math.abs(capped - lastRounded) < CONTENT_WIDTH_NOTIFY_EPSILON) return;
			}
			lastRounded = capped;
			untrack(() => notify?.(capped));
		};

		const ro = new ResizeObserver(() => {
			if (widthRaf) cancelAnimationFrame(widthRaf);
			widthRaf = requestAnimationFrame(flushWidth);
		});
		ro.observe(inner);
		flushWidth();
		return () => {
			ro.disconnect();
			if (widthRaf) cancelAnimationFrame(widthRaf);
		};
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

	// Drag-and-drop file upload
	let isDragging = $state(false);
	let dragCounter = 0;

	function handleDragEnter(e: DragEvent) {
		e.preventDefault();
		dragCounter++;
		if (e.dataTransfer?.types.includes('Files')) {
			isDragging = true;
		}
	}

	function handleDragLeave(e: DragEvent) {
		e.preventDefault();
		dragCounter--;
		if (dragCounter === 0) {
			isDragging = false;
		}
	}

	function handleDragOver(e: DragEvent) {
		e.preventDefault();
	}

	async function handleDrop(e: DragEvent) {
		e.preventDefault();
		dragCounter = 0;
		isDragging = false;

		if (!onFileUpload || !e.dataTransfer?.files.length) return;

		for (const file of e.dataTransfer.files) {
			onFileUpload(file);
		}
	}
</script>

<div
	class={twMerge(
		'flex w-full h-[calc(100dvh-4rem)] flex-row transition-transform md:relative peer-[.workspace]:md:w-1/4',
		classes?.root,
		responsive.isMobile ? 'h-[calc(100dvh-8rem-env(safe-area-inset-bottom,0px))]' : ''
	)}
	onmousemove={resize}
	ondragenter={handleDragEnter}
	ondragleave={handleDragLeave}
	ondragover={handleDragOver}
	ondrop={handleDrop}
	role="region"
	aria-label="Drag and drop files to upload"
>
	<!-- Drag-and-drop overlay -->
	{#if isDragging}
		<div
			class="bg-base-300/60 pointer-events-none absolute inset-0 z-50 flex items-center justify-center backdrop-blur-sm"
		>
			<div
				class="border-primary bg-base-100/90 flex flex-col items-center gap-3 rounded-2xl border-2 border-dashed px-10 py-8 shadow-xl"
			>
				<Upload class="text-primary size-10" />
				<p class="text-base-content text-lg font-semibold">Drop files to upload</p>
			</div>
		</div>
	{/if}

	<div
		class="relative flex flex-col"
		style="width: {browserViewerOpen ? `${100 - browserViewerWidth}%` : '100%'}"
	>
		<!-- Messages area - full height scrollable with bottom padding for floating input -->
		<div
			class="h-full w-full flex-1 overflow-y-auto px-4"
			bind:this={messagesContainer}
			onscroll={handleScroll}
		>
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
											pinInputToBottom = true;
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
					onSend={async (m) => {
						pinInputToBottom = true;
						return await onSendMessage?.(m);
					}}
					{isLoading}
					{agent}
					{onFileOpen}
					{onReadResource}
					hideAgentHeader
				/>
			</div>
		</div>

		<!-- Message input - centered when no messages, bottom when messages exist or when empty state is suppressed -->
		<div
			class={twMerge(
				'absolute right-0 left-0 flex flex-col transition-all duration-500 ease-in-out',
				pinInputToBottom
					? 'bg-base-100/80 top-auto bottom-0 ' +
							(questionElicitation ? 'h-full' : 'backdrop-blur-xs')
					: twMerge(
							'bottom-auto -translate-y-1/2',
							responsive.isMobile
								? 'top-[calc(50%-min(1.0rem,4vh)-min(0.5rem,env(safe-area-inset-bottom,0px)))]'
								: 'top-1/2'
						)
			)}
		>
			{#if pinInputToBottom}
				<div
					class={twMerge(
						'bg-base-100/30 absolute inset-0 z-0 transition-[opacity,backdrop-filter] duration-500 ease-out',
						questionElicitation
							? 'bg-base-100 opacity-75 backdrop-blur-[1px]'
							: 'pointer-events-none opacity-0 backdrop-blur-none'
					)}
					aria-hidden="true"
				></div>
			{/if}
			{#if questionElicitation}
				<div class="relative z-10 flex grow"></div>
			{/if}
			{#if showScrollButton && hasMessages}
				<button
					class="btn btn-circle border-base-300 bg-base-100 btn-md relative z-10 mx-auto shadow-lg active:translate-y-0.5"
					onclick={scrollToBottom}
					aria-label="Scroll to bottom"
				>
					<ChevronDown class="size-5" />
				</button>
			{/if}
			{#if showInlineAgentHeader}
				<div
					class="relative z-10 mx-auto w-full max-w-4xl"
					out:slide={{ axis: 'y', duration: 500 }}
				>
					<div out:fade={{ duration: 500 }}>
						<AgentHeader {agent} onSend={onSendMessage} />
					</div>
				</div>
			{:else if emptyStateContent && !pinInputToBottom}
				<div
					class="relative z-10 mx-auto w-full max-w-4xl"
					out:slide={{ axis: 'y', duration: 500 }}
				>
					<div out:fade={{ duration: 500 }}>
						{@render emptyStateContent()}
					</div>
				</div>
			{/if}
			<div class="relative z-10 mx-auto w-full max-w-4xl px-2">
				{#if questionElicitation}
					{#key questionElicitation.id}
						<div class="elicitation-slide-in mb-8">
							<Elicitation
								elicitation={questionElicitation}
								open
								onresult={(result) => {
									onElicitationResult?.(questionElicitation, result);
								}}
							/>
						</div>
					{/key}
				{:else}
					<MessageInput
						placeholder={`Type your message...${prompts && prompts.length > 0 ? ' or / for prompts' : ''}`}
						onSend={onSendMessage}
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
						{onCancel}
					/>
				{/if}
			</div>
		</div>
	</div>

	{#if browserViewerOpen && browserAvailable}
		<div
			class="bg-base-300 hover:bg-primary w-1 cursor-col-resize transition-colors"
			onmousedown={startResize}
			aria-label="Resize browser viewer panel"
			role="separator"
			aria-orientation="vertical"
			aria-valuenow={browserViewerWidth}
			aria-valuemin="20"
			aria-valuemax="80"
			tabindex="0"
			onkeydown={(event) => {
				if (event.key === 'ArrowLeft' || event.key === 'ArrowRight') {
					event.preventDefault();
					const delta = event.key === 'ArrowLeft' ? -5 : 5;
					const minWidth = 20;
					const maxWidth = 80;
					let nextWidth = browserViewerWidth + delta;
					if (nextWidth < minWidth) nextWidth = minWidth;
					if (nextWidth > maxWidth) nextWidth = maxWidth;
					browserViewerWidth = nextWidth;
				}
			}}
		></div>
	{/if}

	{#if browserViewerOpen && browserAvailable}
		<div class="flex flex-col" style="width: {browserViewerWidth}%">
			<BrowserViewer bind:visible={browserViewerOpen} {browserBaseUrl} />
		</div>
	{/if}

	<!-- Modal elicitations (OAuth, generic form) -->
	{#if modalElicitation}
		{#key modalElicitation.id}
			<Elicitation
				elicitation={modalElicitation}
				open
				onresult={(result) => {
					onElicitationResult?.(modalElicitation, result);
				}}
			/>
		{/key}
	{/if}
</div>

<style>
	@keyframes elicitation-slide-in {
		from {
			opacity: 0;
			transform: translateY(24px);
		}
		to {
			opacity: 1;
			transform: translateY(0);
		}
	}
	.elicitation-slide-in {
		animation: elicitation-slide-in 0.5s ease-out;
	}
</style>
