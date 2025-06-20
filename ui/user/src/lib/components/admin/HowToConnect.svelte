<script lang="ts">
	import { ChevronLeft, ChevronRight } from 'lucide-svelte';
	import { onMount } from 'svelte';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		url: string;
	}

	let { url }: Props = $props();
	let scrollContainer: HTMLUListElement;
	let showLeftChevron = $state(false);
	let showRightChevron = $state(false);

	const optionMap: Record<string, { label: string; icon: string }> = {
		cursor: {
			label: 'Cursor',
			icon: '/user/images/assistant/cursor-mark.svg'
		},
		claude: {
			label: 'Cursor',
			icon: '/user/images/assistant/claude-mark.svg'
		},
		vscode: {
			label: 'VSCode',
			icon: '/user/images/assistant/vscode-mark.svg'
		},
		cline: {
			label: 'Cline',
			icon: '/user/images/assistant/cline-mark.svg'
		},
		highlight: {
			label: 'Highlight AI',
			icon: '/user/images/assistant/highlightai-mark.svg'
		},
		augment: {
			label: 'Augment Code',
			icon: '/user/images/assistant/augmentcode-mark.svg'
		}
	};

	const options = Object.keys(optionMap).map((key) => ({ key, value: optionMap[key] }));
	let selected = $state(options[0].key);
	let previousSelected = $state(options[0].key);
	let isAnimating = $state(false);

	function checkScrollPosition() {
		if (!scrollContainer) return;

		const { scrollLeft, scrollWidth, clientWidth } = scrollContainer;
		showLeftChevron = scrollLeft > 0;
		showRightChevron = scrollLeft < scrollWidth - clientWidth - 1; // -1 for rounding errors
	}

	function scrollLeft() {
		if (scrollContainer) {
			scrollContainer.scrollBy({ left: -200, behavior: 'smooth' });
		}
	}

	function scrollRight() {
		if (scrollContainer) {
			scrollContainer.scrollBy({ left: 200, behavior: 'smooth' });
		}
	}

	function handleSelectionChange(newSelection: string) {
		if (newSelection !== selected) {
			previousSelected = selected;
			selected = newSelection;
			isAnimating = true;

			// Reset animation state after animation completes
			setTimeout(() => {
				isAnimating = false;
			}, 300); // Match the CSS animation duration
		}
	}

	onMount(() => {
		checkScrollPosition();
		scrollContainer?.addEventListener('scroll', checkScrollPosition);
		window.addEventListener('resize', checkScrollPosition);

		return () => {
			scrollContainer?.removeEventListener('scroll', checkScrollPosition);
			window.removeEventListener('resize', checkScrollPosition);
		};
	});
</script>

<div class="flex w-full items-center gap-2">
	<div class="size-4">
		{#if showLeftChevron}
			<button onclick={scrollLeft}>
				<ChevronLeft class="size-4" />
			</button>
		{/if}
	</div>

	<ul
		bind:this={scrollContainer}
		class="default-scrollbar-thin scrollbar-none flex overflow-x-auto"
		style="scroll-behavior: smooth;"
	>
		{#each options as option}
			<li class="w-36 flex-shrink-0">
				<button
					class={twMerge(
						'relative flex w-full items-center justify-center gap-1.5 rounded-t-sm border-b-2 border-transparent py-2 text-[13px] font-light transition-colors hover:bg-gray-50',
						selected === option.key && 'dark:bg-surface1 bg-white hover:bg-transparent'
					)}
					onclick={() => {
						handleSelectionChange(option.key);
					}}
				>
					<img src={option.value.icon} alt={option.value.label} class="size-4" />
					{option.value.label}

					{#if selected === option.key}
						<div
							class={twMerge(
								'absolute right-0 bottom-0 left-0 h-0.5 origin-left bg-blue-500',
								isAnimating && selected === option.key ? 'border-slide-in' : ''
							)}
						></div>
					{:else if isAnimating && previousSelected === option.key}
						<div
							class="border-slide-out absolute right-0 bottom-0 left-0 h-0.5 origin-left bg-blue-500"
						></div>
					{/if}
				</button>
			</li>
		{/each}
	</ul>

	<div class="size-4">
		{#if showRightChevron}
			<button onclick={scrollRight}>
				<ChevronRight class="size-4" />
			</button>
		{/if}
	</div>
</div>

<style>
	@keyframes slideOut {
		from {
			transform: scaleX(1);
			opacity: 1;
		}
		to {
			transform: scaleX(0);
			opacity: 0;
		}
	}

	@keyframes slideIn {
		from {
			transform: scaleX(0);
			opacity: 0;
		}
		to {
			transform: scaleX(1);
			opacity: 1;
		}
	}

	.border-slide-out {
		animation: slideOut 0.3s ease-out forwards;
	}

	.border-slide-in {
		animation: slideIn 0.3s ease-out forwards;
	}
</style>
