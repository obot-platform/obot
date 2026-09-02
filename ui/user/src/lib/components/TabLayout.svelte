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
	import { clearUrlParams, goto } from '$lib/url';
	import { ChevronLeft, ChevronRight } from '@lucide/svelte';
	import type { Component } from 'svelte';
	import { twMerge } from 'tailwind-merge';

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

	let selectedIndex = $derived(views.findIndex((candidate) => candidate.value === selectedView));

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
	<div class={twMerge('flex h-full w-full gap-4 flex-col', views.length === 1 ? 'pt-4' : '')}>
		{#if views.length > 1}
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
							<div class={twMerge('flex flex-1 relative z-10 pr-2', x && 'pl-2')}>
								{#each views as viewOption, index (viewOption.value)}
									{@const isSelected = selectedView === viewOption.value}
									<button
										id={`tab-${viewOption.value}`}
										class={twMerge(
											'tab-flare relative font-light text-md rounded-t-lg text-nowrap px-8 py-2',
											isSelected
												? 'tab-selected z-10 font-medium bg-primary text-white after:absolute after:-bottom-0.75 after:left-1/2 after:-translate-x-1/2 after:h-0 after:w-0 after:border-x-5 after:border-x-transparent after:border-b-5 after:border-b-base-300 after:dark:border-base-100 after:content-[""]'
												: 'tab-hover-flare hover:bg-primary/20',
											!isSelected && index === selectedIndex + 1 && 'tab-flare-no-left',
											!isSelected && index === selectedIndex - 1 && 'tab-flare-no-right',
											!isSelected &&
												index !== selectedIndex - 1 &&
												index !== views.length - 1 &&
												'tab-separator'
										)}
										onclick={() => selectView(viewOption.value)}
									>
										{viewOption.label}
									</button>
								{/each}
							</div>
							<div class="bg-primary h-0.75 w-full shrink-0"></div>
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
		{/if}
		{#if selected}
			{@render selected.content()}
		{/if}
	</div>
</Layout>

<style>
	/* Both bottom corner flares are drawn as two layers of a single pseudo-element,
	   leaving ::after free for the separator */
	.tab-flare {
		--tab-flare-left: radial-gradient(
			circle at top left,
			transparent 0.5rem,
			var(--tab-flare-color) 0.5rem
		);
		--tab-flare-right: radial-gradient(
			circle at top right,
			transparent 0.5rem,
			var(--tab-flare-color) 0.5rem
		);
	}

	.tab-flare::before {
		content: '';
		position: absolute;
		bottom: 0;
		left: -0.5rem;
		right: -0.5rem;
		height: 0.5rem;
		pointer-events: none;
		background-image: var(--tab-flare-left), var(--tab-flare-right);
		background-position:
			left bottom,
			right bottom;
		background-size: 0.5rem 0.5rem;
		background-repeat: no-repeat;
	}

	.tab-flare-no-left {
		--tab-flare-left: none;
	}

	.tab-flare-no-right {
		--tab-flare-right: none;
	}

	.tab-selected {
		--tab-flare-color: var(--color-primary);
	}

	.tab-hover-flare {
		--tab-flare-color: color-mix(in oklab, var(--color-primary) 20%, transparent);
	}

	.tab-hover-flare::before {
		opacity: 0;
	}

	.tab-hover-flare:hover::before {
		opacity: 1;
	}

	.tab-separator::after {
		content: '';
		position: absolute;
		right: 0;
		top: 50%;
		translate: 0 -50%;
		width: 1px;
		height: 1.25rem;
		background-color: color-mix(in oklab, var(--color-base-content) 20%, transparent);
		pointer-events: none;
	}

	/* A flare takes the separator's place on whichever side is hovered */
	.tab-separator:hover::after,
	.tab-separator:has(+ button:hover)::after {
		display: none;
	}
</style>
