<script lang="ts">
	import {
		hasSeenVMcpProfilesHint,
		markVMcpProfilesHintSeen,
		VMCP_PROFILES_HINT_STORAGE_KEY
	} from '$lib/runes/vmcps/vmcpToolFlow.svelte';
	import { Layers, X } from '@lucide/svelte';
	import { onMount } from 'svelte';
	import { fly } from 'svelte/transition';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		show?: boolean;
		anchorEl?: HTMLElement;
		storageKey?: string;
		class?: string;
		onDismiss?: () => void;
	}

	let {
		show = false,
		anchorEl,
		storageKey = VMCP_PROFILES_HINT_STORAGE_KEY,
		class: klass,
		onDismiss
	}: Props = $props();

	let dismissed = $state(true);
	let anchorRect = $state<DOMRect>();

	const arrows = Array.from({ length: 10 }, (_, index) => ({
		angle: -90 + index * 36,
		length: [58, 64, 52, 68, 56, 62, 50, 66, 54, 60][index],
		width: 5,
		delay: index * 320
	}));

	let visible = $derived(show && !dismissed && Boolean(anchorRect));

	function updateAnchorRect() {
		anchorRect = anchorEl?.getBoundingClientRect();
	}

	function dismiss() {
		dismissed = true;
		markVMcpProfilesHintSeen(storageKey);
		onDismiss?.();
	}

	$effect(() => {
		void anchorEl;
		void show;
		updateAnchorRect();
	});

	$effect(() => {
		if (visible) {
			markVMcpProfilesHintSeen(storageKey);
		}
	});

	onMount(() => {
		dismissed = hasSeenVMcpProfilesHint(storageKey);

		const refresh = () => updateAnchorRect();
		window.addEventListener('resize', refresh);
		window.addEventListener('scroll', refresh, true);
		return () => {
			window.removeEventListener('resize', refresh);
			window.removeEventListener('scroll', refresh, true);
		};
	});
</script>

{#if visible && anchorRect}
	<div
		class="pointer-events-none fixed z-70 rounded-md ring-2 ring-primary shadow-[0_0_0_4px_color-mix(in_oklab,var(--color-primary)_18%,transparent)]"
		style="top: {anchorRect.top - 2}px; left: {anchorRect.left - 2}px; width: {anchorRect.width +
			4}px; height: {anchorRect.height + 4}px;"
		aria-hidden="true"
	></div>

	<div
		class={twMerge('pointer-events-auto fixed z-71 w-72', klass)}
		style="top: {anchorRect.top}px; left: {anchorRect.right + 10}px;"
		in:fly={{ x: -8, duration: 220 }}
		role="dialog"
		aria-labelledby="vmcp-profiles-hint-title"
		aria-describedby="vmcp-profiles-hint-description"
	>
		<div
			class="bg-base-100/90 dark:bg-base-300/90 border-base-300 dark:border-base-400 relative rounded-lg border p-3 shadow-lg backdrop-blur-sm"
		>
			<div
				class="bg-base-100 dark:bg-base-300 border-base-300 dark:border-base-400 absolute top-4 -left-1 size-2 rotate-45 border-b border-l"
				aria-hidden="true"
			></div>

			<div class="flex items-start justify-between gap-2">
				<p
					id="vmcp-profiles-hint-title"
					class="text-muted-content font-mono text-[0.625rem] tracking-[0.14em] uppercase"
				>
					Profiles
				</p>
				<button
					type="button"
					class="text-muted-content hover:text-base-content -mt-1 -mr-1 rounded-sm p-1 transition-colors"
					aria-label="Dismiss profiles tip"
					onclick={dismiss}
				>
					<X class="size-3" />
				</button>
			</div>

			{@render stage()}

			<p id="vmcp-profiles-hint-description" class="text-muted-content mt-2 text-xs font-light">
				Click here to begin tailoring access and tools this VMCP.
			</p>
		</div>
	</div>
{/if}

{#snippet stage()}
	<div
		class="border-base-300 dark:border-base-400 bg-base-200/40 dark:bg-base-200/20 relative mt-2 h-36 overflow-hidden rounded-md border"
		aria-hidden="true"
	>
		<div class="vmcp-profiles-hint-glow absolute inset-0"></div>

		<svg class="text-primary absolute inset-0 size-full" viewBox="0 0 160 160" aria-hidden="true">
			<g transform="translate(80 80)">
				{#each arrows as arrow (arrow.angle)}
					<g
						transform="rotate({arrow.angle})"
						style={`--hint-travel: ${arrow.length}px; --hint-delay: ${arrow.delay}ms`}
					>
						<g class="vmcp-profiles-hint-flight">
							<line
								x1="0"
								y1="10"
								x2="0"
								y2={-arrow.length + 8}
								stroke="currentColor"
								stroke-width={arrow.width}
								stroke-linecap="round"
							/>
							<polygon
								points={`0,${-arrow.length - 8} ${arrow.width + 2},${-arrow.length + 4} ${-(arrow.width + 2)},${-arrow.length + 4}`}
								fill="currentColor"
							/>
						</g>
					</g>
				{/each}
			</g>
		</svg>

		<div
			class="bg-base-100 dark:bg-base-300 border-base-300 dark:border-base-400 absolute top-1/2 left-1/2 flex size-11 -translate-x-1/2 -translate-y-1/2 items-center justify-center rounded-full border shadow-md"
		>
			<Layers class="text-primary size-5" />
		</div>
	</div>
{/snippet}

<style>
	.vmcp-profiles-hint-glow {
		background: radial-gradient(
			circle at center,
			color-mix(in oklab, var(--color-primary) 12%, transparent) 0%,
			transparent 68%
		);
	}

	.vmcp-profiles-hint-flight {
		opacity: 0;
		transform: translateY(18px) scale(0.2);
		transform-origin: 0 0;
		animation: vmcp-profiles-hint-fly 3.6s cubic-bezier(0.22, 1, 0.36, 1) both;
		animation-delay: var(--hint-delay, 0ms);
	}

	@keyframes vmcp-profiles-hint-fly {
		0% {
			opacity: 0;
			transform: translateY(18px) scale(0.2);
		}
		12% {
			opacity: 1;
		}
		70%,
		100% {
			opacity: 1;
			transform: translateY(calc(-1 * var(--hint-travel) + 18px)) scale(1);
		}
	}

	@media (prefers-reduced-motion: reduce) {
		.vmcp-profiles-hint-flight {
			animation: none;
			opacity: 1;
			transform: translateY(calc(-1 * var(--hint-travel) + 18px)) scale(1);
		}
	}
</style>
