<script lang="ts" module>
	import type { Snippet } from 'svelte';

	export type TabView = {
		label: string;
		value: string;
		content: Snippet;
	};
</script>

<script lang="ts">
	import { page } from '$app/state';
	import Layout from '$lib/components/Layout.svelte';
	import OverflowContainer from '$lib/components/OverflowContainer.svelte';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants';
	import { clearUrlParams, goto } from '$lib/url';
	import { ChevronLeft, ChevronRight } from '@lucide/svelte';
	import type { Component } from 'svelte';
	import { fly } from 'svelte/transition';
	import { twMerge } from 'tailwind-merge';

	const duration = PAGE_TRANSITION_DURATION;
	const VIEW_PARAM = 'view';

	interface Props {
		title: string;
		views: TabView[];
		defaultView?: string;
		rightNavActions?: Snippet<[string]>;
		showBackButton?: boolean;
		onBackButtonClick?: () => void;
		classes?: {
			container?: string;
			childrenContainer?: string;
			navbar?: string;
			noSidebarTitle?: string;
		};
		main?: { component: Component; props?: Record<string, unknown> };
		rightSidebar?: Snippet<[string]>;
	}

	let {
		title,
		views,
		defaultView,
		rightNavActions,
		showBackButton,
		onBackButtonClick,
		classes,
		main,
		rightSidebar
	}: Props = $props();

	let selectedView = $derived.by(() => {
		const requested = page.url.searchParams.get(VIEW_PARAM);
		if (requested && views.some((candidate) => candidate.value === requested)) {
			return requested;
		}
		if (defaultView && views.some((candidate) => candidate.value === defaultView)) {
			return defaultView;
		}
		return views[0]?.value;
	});

	let selected = $derived(views.find((candidate) => candidate.value === selectedView));

	function selectView(value: string) {
		clearUrlParams(Array.from(page.url.searchParams.keys()).filter((key) => key !== VIEW_PARAM));
		goto(`${page.url.pathname}?${VIEW_PARAM}=${value}`);
	}
</script>

{#snippet layoutNav()}
	{#if rightNavActions && selectedView}
		{@render rightNavActions(selectedView)}
	{/if}
{/snippet}

{#snippet layoutRightSidebar()}
	{#if rightSidebar && selectedView}
		{@render rightSidebar(selectedView)}
	{/if}
{/snippet}

<Layout
	{title}
	{showBackButton}
	{onBackButtonClick}
	{main}
	rightNavActions={rightNavActions ? layoutNav : undefined}
	rightSidebar={rightSidebar ? layoutRightSidebar : undefined}
	classes={{
		...classes,
		container: twMerge('justify-start pt-0', classes?.container),
		childrenContainer: twMerge('pt-0', classes?.childrenContainer)
	}}
>
	<div class="flex h-full w-full gap-4 flex-col" in:fly={{ x: 100, duration, delay: duration }}>
		<div class="w-full mt-4">
			<OverflowContainer
				class="scrollbar-none flex shrink-0 min-h-12 w-full items-center gap-2 overflow-x-auto"
				style="scroll-behavior: smooth;"
			>
				{#snippet children({ x, hasMoreLeft, hasMoreRight, scrollLeft, scrollRight })}
					{#if x}
						<button
							disabled={!hasMoreLeft}
							onclick={scrollLeft}
							class="shrink-0 z-20 bg-base-200 dark:bg-base-100 sticky left-0 flex aspect-square h-full items-center justify-center rounded-l-md p-2.5 opacity-100 transition-all duration-200 disabled:opacity-30"
						>
							<ChevronLeft class="size-full" />
						</button>
					{/if}

					<div class="flex flex-1 flex-col">
						<div class="flex flex-1 relative z-10">
							{#each views as viewOption (viewOption.value)}
								<button
									id={`tab-${viewOption.value}`}
									class={twMerge(
										'border-b-2 font-light text-nowrap border-transparent px-8 py-2 transition-colors duration-400',
										selectedView === viewOption.value
											? 'border-primary border-b-3 bg-base-100 dark:bg-base-300 rounded-t-md font-medium'
											: 'hover:border-primary/25 text-muted-content hover:text-base-content'
									)}
									onclick={() => selectView(viewOption.value)}
								>
									{viewOption.label}
								</button>
							{/each}
						</div>
						<div class="bg-base-400 h-0.5 w-full shrink-0 -translate-y-0.5"></div>
					</div>

					{#if x}
						<button
							disabled={!hasMoreRight}
							onclick={scrollRight}
							class="shrink-0 z-20 bg-base-200 dark:bg-base-100 sticky right-0 flex aspect-square h-full items-center justify-center rounded-r-md p-2.5 opacity-100 transition-all duration-200 disabled:opacity-30"
						>
							<ChevronRight class="size-full" />
						</button>
					{/if}
				{/snippet}
			</OverflowContainer>
		</div>
		{#if selected}
			{@render selected.content()}
		{/if}
	</div>
</Layout>
