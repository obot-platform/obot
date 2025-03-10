<script lang="ts">
	import { onMount, type Snippet } from 'svelte';
	import { ChevronUp } from 'lucide-svelte';
	import { ChevronDown } from 'lucide-svelte/icons';
	import { fade } from 'svelte/transition';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		header: string;
		children: Snippet;
		open?: boolean;
		onOpen?: () => void | Promise<void>;
		classes?: {
			header?: string;
			content?: string;
		};
	}

	onMount(() => {
		if (open) {
			onOpen?.();
		}
	});

	let { header, children, open, onOpen, classes }: Props = $props();
</script>

<div class="flex flex-col">
	{#if header}
		<button
			class={twMerge('flex items-center justify-between gap-2 px-5 py-2', classes?.header)}
			onclick={() => {
				if (!open) {
					onOpen?.();
				}
				open = !open;
			}}
		>
			<span class="text-lg">{header}</span>
			<span>
				{#if open}
					<ChevronUp />
				{:else}
					<ChevronDown />
				{/if}
			</span>
		</button>
	{/if}
	{#if open}
		<div
			in:fade
			class={twMerge('flex flex-col border-surface3 p-5', classes?.content)}
			class:border-t={header}
		>
			{@render children()}
		</div>
	{/if}
</div>
