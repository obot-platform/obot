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
	}

	let { id, block, class: clazz = '', children }: Props = $props();
</script>

<div class={twMerge('flex flex-col gap-1', clazz)}>
	{@render children?.()}

	{#if block.title}
		<div class={twMerge('text-muted-content flex items-center text-xs font-medium')}>
			{block.title}
		</div>
	{/if}
	<CopyField id={`${id}-copy-field`} variant="code" value={block.code} />
</div>
