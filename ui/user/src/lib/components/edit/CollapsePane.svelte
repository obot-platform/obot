<script lang="ts">
	import { onMount, type Snippet } from 'svelte';
	import { ChevronUp } from 'lucide-svelte';
	import { ChevronDown } from 'lucide-svelte/icons';
	import { slide } from 'svelte/transition';

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

	let { header, children, open = false, onOpen }: Props = $props();
</script>

<div class="flex flex-col">
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
	{#if open}
		<div in:slide class="flex flex-col border-t border-surface3 p-5">
			{@render children()}
		</div>
	{/if}
</div>
