<script lang="ts">
	import { onMount, type Snippet } from 'svelte';
	import { ChevronUp } from 'lucide-svelte';
	import { ChevronDown } from 'lucide-svelte/icons';
	import { fade } from 'svelte/transition';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		header: string | Snippet;
		children: Snippet;
		open?: boolean;
		onOpen?: () => void | Promise<void>;
		classes?: { header?: string; content?: string };
		showDropdown?: boolean;
	}

	onMount(() => {
		if (open) {
			onOpen?.();
		}
	});

	let {
		header,
		children,
		open = $bindable(false),
		onOpen,
		classes = {},
		showDropdown = true
	}: Props = $props();
</script>

<div class="flex flex-col">
	{#if header}
		<button
			class={twMerge('flex items-center gap-2 px-5 py-2', classes.header)}
			onclick={() => {
				if (!open) {
					onOpen?.();
				}
				open = !open;
			}}
		>
			{#if typeof header === 'string'}
				<span class="grow text-start text-lg">
					{header}
				</span>
			{:else}
				{@render header()}
			{/if}

			{#if showDropdown}
				<span>
					{#if open}
						<ChevronUp />
					{:else}
						<ChevronDown />
					{/if}
				</span>
			{/if}
		</button>
	{/if}
	{#if open}
		<div
			in:fade
			class={twMerge('flex flex-col border-surface3 p-5', classes.content)}
			class:border-t={header}
		>
			{@render children()}
		</div>
	{/if}
</div>
