<script lang="ts">
	import type { SnippetBlock } from '$lib/services/llm-gateway/types';
	import CopyField from '../CopyField.svelte';
	import type { Snippet } from 'svelte';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		id: string;
		block: SnippetBlock;
		class?: string;
		children?: Snippet;
		marker?: string;
		slotContent?: Snippet;
		actions?: Snippet;
	}

	let {
		id,
		block,
		class: clazz = '',
		children,
		marker = '',
		slotContent,
		actions
	}: Props = $props();

	let codeParts = $derived(marker && block.code.includes(marker) ? block.code.split(marker) : null);
	let showActions = $derived(Boolean(codeParts && actions));
</script>

<div class={twMerge('flex flex-col gap-1', clazz)}>
	{@render children?.()}

	{#if block.title || showActions}
		<div class="flex items-center justify-between gap-2">
			{#if block.title}
				<div class="text-muted-content min-w-0 text-xs font-medium">{block.title}</div>
			{/if}
			{#if showActions}
				<div class="ml-auto shrink-0">
					{@render actions?.()}
				</div>
			{/if}
		</div>
	{/if}
	<!-- Keep this on one line so the snippet does not insert whitespace into the code block. -->
	<!-- prettier-ignore -->
	<CopyField id={`${id}-copy-field`} variant="code" value={block.code}>
		{#snippet codeContent()}{#if codeParts}{#each codeParts as part, i (i)}{part}{#if i < codeParts.length - 1}{@render slotContent?.()}{/if}{/each}{:else}{block.code}{/if}{/snippet}
	</CopyField>
</div>
