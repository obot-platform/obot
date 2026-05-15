<script lang="ts">
	import Loading from '$lib/icons/Loading.svelte';
	import { Circle, CircleCheckBig, CircleX } from 'lucide-svelte';
	import type { Snippet } from 'svelte';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		open: boolean;
		title: string;
		loading: boolean;
		errors?: string | null;
		hasResults: boolean;
		children: Snippet;
		classes?: {
			content?: string;
		};
		showContent?: boolean;
	}
	let {
		open = $bindable(),
		loading,
		title,
		errors,
		hasResults,
		children,
		classes,
		showContent
	}: Props = $props();

	const canExpand = $derived(!!(errors || hasResults || showContent));
</script>

<details
	class={twMerge(
		'collapse bg-base-200 dark:bg-base-100',
		(hasResults || showContent) && 'collapse-arrow'
	)}
	name={title}
	bind:open
>
	<summary
		class="collapse-title font-semibold text-sm flex items-center gap-2"
		onclick={(e) => {
			if (!canExpand && !open) {
				e.preventDefault();
			}
		}}
	>
		{@render statusIcon()}
		{title}
	</summary>
	<div
		class={twMerge(
			'collapse-content text-sm bg-base-100 dark:bg-base-200 pt-4 border border-base-300 dark:border-base-200',
			classes?.content
		)}
	>
		{#if errors}
			<p class="text-error bg-error/10 p-2 rounded-md text-sm whitespace-pre-wrap">
				{errors}
			</p>
		{:else if hasResults || showContent}
			{@render children()}
		{/if}
	</div>
</details>

{#snippet statusIcon()}
	{#if loading}
		<Loading class="text-muted-content" />
	{:else if errors}
		<CircleX class="size-5 text-error" />
	{:else if hasResults}
		<CircleCheckBig class="size-5 text-primary" />
	{:else}
		<Circle class="size-5 text-muted-content" />
	{/if}
{/snippet}
