<script lang="ts">
	import type { SnippetBlock } from '$lib/services/llm-gateway/types';
	import CopyField from '../CopyField.svelte';
	import type { Snippet } from 'svelte';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		id: string;
		block: SnippetBlock;
		class?: string;
		/** Shown between the title and the code, such as a control that changes the code. */
		children?: Snippet;
		tokenSlot?: string;
		tokenSlotContent?: Snippet;
	}

	let {
		id,
		block,
		class: clazz = '',
		children,
		tokenSlot = '',
		tokenSlotContent
	}: Props = $props();

	let codeParts = $derived(
		tokenSlot && block.code.includes(tokenSlot) ? block.code.split(tokenSlot) : null
	);
</script>

<div class={twMerge('flex flex-col gap-1', clazz)}>
	{@render children?.()}

	{#if block.title}
		<div class={twMerge('text-muted-content flex items-center text-xs font-medium')}>
			{block.title}
		</div>
	{/if}
	<!-- Keep this on one line so the snippet does not insert whitespace into the code block. -->
	<!-- prettier-ignore -->
	<CopyField id={`${id}-copy-field`} variant="code" value={block.code}>
		{#snippet codeContent()}{#if codeParts}{#each codeParts as part, i (i)}{part}{#if i < codeParts.length - 1}{@render tokenSlotContent?.()}{/if}{/each}{:else}{block.code}{/if}{/snippet}
	</CopyField>
</div>
