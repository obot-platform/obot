<script lang="ts">
	import { toHTMLFromMarkdown } from '$lib/markdown';
	import type { ChatMessageItemText } from '$lib/services/nanobot/types';

	interface Props {
		item: ChatMessageItemText;
		role: 'user' | 'assistant';
	}

	let { item, role }: Props = $props();

	const renderedContent = $derived(
		role === 'assistant' ? toHTMLFromMarkdown(item.text) : item.text
	);
</script>

<div
	class={[
		'prose rounded-box text-base-content flex w-full max-w-none flex-col gap-2 p-2',
		{
			'mb-3': role === 'assistant',
			'p-4': role === 'assistant',
			'bg-base-200': role === 'user',
			'whitespace-pre-wrap': role === 'user'
		}
	]}
>
	{@html renderedContent}
</div>
