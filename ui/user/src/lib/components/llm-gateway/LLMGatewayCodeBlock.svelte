<script lang="ts">
	import CopyButton from '$lib/components/CopyButton.svelte';
	import type { SnippetBlock } from '$lib/services/llm-gateway/types';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		block: SnippetBlock;
		class?: string;
	}

	let { block, class: clazz = '' }: Props = $props();
</script>

<div class={twMerge('flex flex-col gap-1', clazz)}>
	{#if block.title}
		<div class="text-muted-content flex items-center text-xs font-medium">
			{block.title}
		</div>
	{/if}
	<div class="bg-base-200 dark:bg-base-300 group relative rounded-md">
		<div class="absolute top-2 right-2 z-10 opacity-0 transition-opacity group-hover:opacity-100">
			<CopyButton
				text={block.code}
				classes={{ button: 'bg-base-100 dark:bg-base-200 rounded p-1.5' }}
			/>
		</div>
		<pre class="default-scrollbar-thin overflow-x-auto p-3 pr-12 text-xs leading-relaxed"><code
				class="font-mono">{block.code}</code
			></pre>
	</div>
</div>
