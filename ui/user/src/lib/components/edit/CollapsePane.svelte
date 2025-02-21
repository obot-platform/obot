<script lang="ts">
	import { onMount, type Snippet } from 'svelte';
	import { ChevronUp } from 'lucide-svelte';
	import { ChevronDown } from 'lucide-svelte/icons';
	import { fade } from 'svelte/transition';

	interface Props {
		header: string;
		children: Snippet;
		open?: boolean;
		onOpen?: () => void | Promise<void>;
	}

	onMount(() => {
		if (open) {
			onOpen?.();
		}
	});

	let { header, children, open, onOpen }: Props = $props();
</script>

<div class="flex flex-col">
	{#if header}
		<button
			class="flex items-center gap-2 px-5 py-2"
			onclick={() => {
				if (!open) {
					onOpen?.();
				}
				open = !open;
			}}
		>
			<span class="text-lg">{header}</span>
			<span class="grow">
				{#if open}
					<ChevronUp />
				{:else}
					<ChevronDown />
				{/if}
			</span>
		</button>
	{/if}
	{#if open}
		<div in:fade class="flex flex-col border-surface3 p-5" class:border-t={header}>
			{@render children()}
		</div>
	{/if}
</div>
