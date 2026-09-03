<script lang="ts">
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import type { MCPTesterChat } from '$lib/services/mcp/tester-chat.svelte';
	import type { MCPTesterSession } from '$lib/services/mcp/tester.svelte';
	import StagedContextPreview from './StagedContextPreview.svelte';
	import { Ban, Send } from '@lucide/svelte';

	interface Props {
		chat: MCPTesterChat;
		session: MCPTesterSession;
	}

	// Single line at rest, grows to a few lines, then scrolls.
	const MIN_HEIGHT = 40;
	const MAX_HEIGHT = 128;

	let { chat, session }: Props = $props();
	let draft = $state('');
	let textareaElement = $state<HTMLTextAreaElement>();
	let stagedEndsWithAssistant = $derived.by(() => {
		const last = session.stagedContext.at(-1);
		if (!last || last.type === 'resource') return false;
		return last.messages.at(-1)?.role === 'assistant';
	});
	let hasContent = $derived(
		Boolean(draft.trim() || (session.stagedContext.length && !stagedEndsWithAssistant))
	);
	let responding = $derived(chat.status === 'snapshotting' || chat.status === 'streaming');

	function autoResize(): void {
		if (!textareaElement) return;
		textareaElement.style.height = '0';
		// The box is border-box but scrollHeight excludes the border, so the
		// borders have to be added back or a single line still overflows by 2px.
		const style = getComputedStyle(textareaElement);
		const borders =
			Number.parseFloat(style.borderTopWidth) + Number.parseFloat(style.borderBottomWidth);
		const content = textareaElement.scrollHeight + borders;
		const height = Math.min(Math.max(content, MIN_HEIGHT), MAX_HEIGHT);
		textareaElement.style.height = `${height}px`;
		// Only show the scrollbar once the draft actually outgrows the max height.
		textareaElement.style.overflowY = content > MAX_HEIGHT ? 'auto' : 'hidden';
	}

	function send(): void {
		if (!chat.canSend || !hasContent) return;
		const text = draft;
		draft = '';
		// Keep the caret in the composer so the user can keep typing while the
		// assistant responds; only submitting is gated on chat.canSend.
		textareaElement?.focus();
		void chat.send(text);
	}

	function handleKeydown(event: KeyboardEvent): void {
		if (event.key !== 'Enter' || event.shiftKey || event.isComposing) return;
		event.preventDefault();
		send();
	}

	$effect(() => {
		void draft;
		autoResize();
	});
</script>

<section
	class="border-base-300 dark:border-base-400 mt-3 flex max-h-[60%] shrink-0 flex-col border-t pt-3"
	aria-label="Chat composer"
>
	<div class="default-scrollbar-thin min-h-0 overflow-y-auto">
		<StagedContextPreview {session} />
	</div>
	<div class="flex shrink-0 items-end gap-2">
		<label class="min-w-0 flex-1" for="mcp-tester-chat-message">
			<span class="sr-only">Message</span>
			<textarea
				id="mcp-tester-chat-message"
				class="text-input-filled default-scrollbar-thin block w-full resize-none leading-6"
				style="height: {MIN_HEIGHT}px; max-height: {MAX_HEIGHT}px;"
				rows="1"
				placeholder="Test this MCP server…"
				bind:value={draft}
				bind:this={textareaElement}
				oninput={autoResize}
				onkeydown={handleKeydown}
			></textarea>
		</label>
		<button
			type="button"
			class={`btn btn-circle shrink-0 ${responding ? 'btn-secondary' : 'btn-primary'}`}
			aria-label={responding ? 'Stop' : 'Send'}
			use:tooltip={responding ? 'Stop' : 'Send'}
			disabled={!responding && (!chat.canSend || !hasContent)}
			onclick={() => (responding ? chat.stop() : send())}
		>
			{#if responding}
				<Ban class="size-4" aria-hidden="true" />
			{:else}
				<Send class="size-4" aria-hidden="true" />
			{/if}
		</button>
	</div>
	{#if stagedEndsWithAssistant && !draft.trim()}
		<p class="mt-2 text-xs text-warning">Add a user message to continue this staged prompt.</p>
	{/if}
</section>
