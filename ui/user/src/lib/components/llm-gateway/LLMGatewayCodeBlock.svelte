<script lang="ts">
	import CopyButton from '$lib/components/CopyButton.svelte';
	import type { SnippetBlock } from '$lib/services/llm-gateway/types';
	import type { Snippet } from 'svelte';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		block: SnippetBlock;
		class?: string;
		/** Shown between the title and the code, such as a control that changes the code. */
		children?: Snippet;
	}

	let { block, class: clazz = '', children }: Props = $props();
</script>

<div class={twMerge('flex flex-col gap-1', clazz)}>
	{#if block.title}
		<div class="text-muted-content flex items-center text-xs font-medium">
			{block.title}
		</div>
	{/if}
	{@render children?.()}
	<div class="bg-base-200 dark:bg-base-100 group relative rounded-md">
		<div class="absolute top-2 right-2 z-10 opacity-0 transition-opacity group-hover:opacity-100">
			<CopyButton
				text={block.code}
				showTextLeft
				classes={{
					button: 'bg-base-100 dark:bg-base-200 rounded p-1.5'
				}}
			/>
		</div>
		<pre
			class="default-scrollbar-thin my-0 overflow-x-auto bg-transparent p-3 pr-12 text-xs leading-relaxed"><code
				>{block.code}</code
			></pre>
	</div>
</div>
