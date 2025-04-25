<script lang="ts">
	import { onMount, type Snippet } from 'svelte';
	import { ChevronDown } from 'lucide-svelte/icons';
	import { twMerge } from 'tailwind-merge';
	import { slide } from 'svelte/transition';

	interface Props {
		endContent?: Snippet;
		header: string | Snippet;
		children: Snippet;
		open?: boolean;
		onOpen?: () => void | Promise<void>;
		classes?: { header?: string; content?: string; root?: string };
		showDropdown?: boolean;
		compact?: boolean;
	}

	onMount(() => {
		if (open) {
			onOpen?.();
		}
	});

	let {
		header,
		endContent,
		children,
		open = $bindable(false),
		onOpen,
		classes = {},
		showDropdown = true,
		compact
	}: Props = $props();
</script>

<div class={twMerge('flex flex-col', classes.root)}>
	{#if header}
		<button
			class={twMerge('flex items-center gap-2 px-5 py-2', compact && 'pr-1.5 pl-3', classes.header)}
			onclick={() => {
				if (!open) {
					onOpen?.();
				}
				open = !open;
			}}
		>
			{#if typeof header === 'string'}
				<span
					class={twMerge(
						'grow text-start text-base font-extralight',
						compact && 'text-sm font-semibold'
					)}
				>
					{header}
				</span>
			{:else}
				{@render header()}
			{/if}

			{#if showDropdown}
				<span
					class:rotate-180={open}
					class={twMerge('transition-transform duration-200', compact && 'text-gray-500')}
				>
					<ChevronDown />
				</span>
			{/if}
			{#if endContent}
				{@render endContent()}
			{/if}
		</button>
	{/if}
	{#if open && showDropdown}
		<div
			transition:slide
			class={twMerge('border-surface1 bg-surface2 flex flex-col p-5 shadow-inner', classes.content)}
		>
			{@render children()}
		</div>
	{/if}
</div>
