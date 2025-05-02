<script lang="ts">
	import { onMount, type Snippet } from 'svelte';
	import { ChevronDown } from 'lucide-svelte/icons';
	import { twMerge } from 'tailwind-merge';
	import { fade, slide } from 'svelte/transition';
	import { getHelperMode } from '$lib/context/helperMode.svelte';
	import InfoTooltip from '$lib/components/InfoTooltip.svelte';

	interface Props {
		endContent?: Snippet;
		header: string | Snippet;
		children: Snippet;
		open?: boolean;
		onOpen?: () => void | Promise<void>;
		classes?: { header?: string; headerText?: string; content?: string; root?: string };
		showDropdown?: boolean;
		iconSize?: number;
		helpText?: string;
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
		iconSize = 6,
		helpText
	}: Props = $props();
</script>

<div class={twMerge('flex flex-col', classes.root)}>
	{#if header}
		<button
			class={twMerge('flex items-center gap-2 px-5 py-2 font-extralight', classes.header)}
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
						'flex grow items-center gap-1 text-start text-sm font-extralight',
						classes?.headerText
					)}
				>
					{header}
					{#if getHelperMode().isEnabled && helpText}
						<div in:fade>
							<InfoTooltip text={helpText} />
						</div>
					{/if}
				</span>
			{:else}
				{@render header()}
			{/if}

			{#if showDropdown}
				<span class:rotate-180={open} class="transition-transform duration-200">
					<ChevronDown class={`size-${iconSize}`} />
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
			class={twMerge(
				'border-surface1 bg-surface2 dark:bg-surface1 flex flex-col p-5 shadow-inner',
				classes.content
			)}
		>
			{@render children()}
		</div>
	{/if}
</div>
