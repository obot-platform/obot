<script lang="ts">
	import { TITLE_CHROME, type TitleChrome } from './titleChrome';
	import { getContext, type Snippet } from 'svelte';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		title: string;
		description?: string;
		element?: 'h3' | 'h4' | 'h5' | 'h6';
		rightContent?: Snippet;
		children?: Snippet;
	}

	const { title, description, rightContent, children, element = 'h3' }: Props = $props();
	const chrome = getContext<TitleChrome | undefined>(TITLE_CHROME);
	const rootClasses = 'border-b dark:border-base-400 border-base-300 px-4 py-2 rounded-t-md';
	const titleClasses = 'text-xs uppercase font-medium tracking-wider';
	const hasChrome = $derived(Boolean(chrome?.leading || chrome?.trailing));
</script>

{#if hasChrome}
	<div
		class={twMerge(
			rootClasses,
			'relative flex items-center gap-2 transition-[padding] duration-200 group-hover:pr-8.5 group-[.is-dragging]:pr-8.5 group-[.is-menu-open]:pr-8.5'
		)}
	>
		{#if chrome?.leading}
			{@render chrome.leading()}
		{/if}
		<svelte:element this={element} class={twMerge(titleClasses, 'min-w-0 flex-1')}>
			{@render maincontent()}
		</svelte:element>
		{#if rightContent}
			{@render rightContent()}
		{/if}
		{#if chrome?.trailing}
			{@render chrome.trailing()}
		{/if}
	</div>
{:else if rightContent}
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
