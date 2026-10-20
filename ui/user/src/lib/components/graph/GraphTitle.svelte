<script lang="ts">
	import type { Snippet } from 'svelte';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		title: string;
		description?: string;
		element?: 'h3' | 'h4' | 'h5' | 'h6';
		rightContent?: Snippet;
		children?: Snippet;
	}

	const { title, description, rightContent, children, element = 'h3' }: Props = $props();
	const rootClasses = 'border-b dark:border-base-400 border-base-300 px-4 py-2 rounded-t-md';
	const titleClasses = 'text-xs uppercase font-medium tracking-wider';
</script>

{#if rightContent}
	<div class={twMerge(rootClasses, 'flex items-center justify-between')}>
		<svelte:element this={element} class={titleClasses}>
			{@render maincontent()}
		</svelte:element>
		{@render rightContent()}
	</div>
{:else}
	<svelte:element this={element} class={twMerge(rootClasses, titleClasses)}>
		{@render maincontent()}
	</svelte:element>
{/if}

{#snippet maincontent()}
	{title}
	{#if description}
		<span class="text-muted-content text-xs font-light">{description}</span>
	{/if}
	{#if children}
		{@render children()}
	{/if}
{/snippet}
