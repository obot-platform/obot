<script lang="ts">
	import { AlertCircle } from 'lucide-svelte';
	import { toHTMLFromMarkdown } from '$lib/markdown';
	import type { ChatMessageItemText } from '$lib/services/nanobot/types';
	import { CANCELLATION_PHRASE_CLIENT } from '$lib/services/nanobot/utils';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		item: ChatMessageItemText;
		role: 'user' | 'assistant';
	}

	let { item, role }: Props = $props();

	const hasClientCancellation = $derived(item.text?.includes(CANCELLATION_PHRASE_CLIENT) ?? false);
	const textWithoutCancellation = $derived(
		hasClientCancellation ? item.text.replace(CANCELLATION_PHRASE_CLIENT, '').trimEnd() : item.text
	);
	const renderedContent = $derived(
		role === 'assistant' ? toHTMLFromMarkdown(textWithoutCancellation) : textWithoutCancellation
	);
</script>

<div
	class={twMerge(
		'prose rounded-box flex w-full max-w-none flex-col gap-2 p-2',
		role === 'assistant' ? 'p-4' : '',
		role === 'user' ? 'bg-base-200 whitespace-pre-wrap' : ''
	)}
>
	{#if renderedContent}
		{@html renderedContent}
	{/if}
	{#if hasClientCancellation}
		<div class="text-base-content/50 my-4 flex items-center gap-1 text-xs italic">
			<AlertCircle class="size-3" />
			Aborted. This message has been discarded.
		</div>
	{/if}
</div>
