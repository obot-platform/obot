<script module>
	export const PROFILES_HINT_TEXT = 'Click here to begin tailoring access and tools this VMCP.';
	export const TESTER_HINT_TEXT =
		"Open Tester to try this vMCP's tools in Obot before you connect a client.";
	export const CONNECT_HINT_TEXT = 'Connect this vMCP to get an endpoint for your AI client.';
</script>

<script lang="ts">
	import {
		hasSeenVMcpCreationHint,
		markVMcpCreationHintSeen,
		VMCP_CREATION_HINT_STORAGE_KEY
	} from '$lib/runes/vmcps/vmcpToolFlow.svelte';
	import { FlaskConical, Layers, MousePointer2, Unplug, X } from '@lucide/svelte';
	import { onMount } from 'svelte';
	import { fly } from 'svelte/transition';
	import { twMerge } from 'tailwind-merge';

	type HintStepId = 'profiles' | 'tester' | 'connect';

	interface HintStep {
		id: HintStepId;
		title: string;
		description: string;
		placement: 'right' | 'bottom';
	}

	interface Props {
		show?: boolean;
		profilesAnchorEl?: HTMLElement;
		testerAnchorEl?: HTMLElement;
		connectAnchorEl?: HTMLElement;
		includeProfiles?: boolean;
		includeTester?: boolean;
		includeConnect?: boolean;
		storageKey?: string;
		class?: string;
		onDismiss?: () => void;
	}

	let {
		show = false,
		profilesAnchorEl,
		testerAnchorEl,
		connectAnchorEl,
		includeProfiles = false,
		includeTester = false,
		includeConnect = false,
		storageKey = VMCP_CREATION_HINT_STORAGE_KEY,
		class: klass,
		onDismiss
	}: Props = $props();

	const allSteps: HintStep[] = [
		{
			id: 'profiles',
			title: 'Profiles',
			description: PROFILES_HINT_TEXT,
			placement: 'right'
		},
		{
			id: 'tester',
			title: 'Tester',
			description: TESTER_HINT_TEXT,
			placement: 'right'
		},
		{
			id: 'connect',
			title: 'Connect',
			description: CONNECT_HINT_TEXT,
			placement: 'bottom'
		}
	];

	const arrows = Array.from({ length: 10 }, (_, index) => ({
		angle: -90 + index * 36,
		length: [58, 64, 52, 68, 56, 62, 50, 66, 54, 60][index],
		width: 5,
		delay: index * 320
	}));

	let dismissed = $state(true);
	let stepIndex = $state(0);
	let anchorRect = $state<DOMRect>();

	let steps = $derived(
		allSteps.filter((step) => {
			if (step.id === 'profiles') return includeProfiles;
			if (step.id === 'tester') return includeTester;
			return includeConnect;
		})
	);
	let current = $derived(steps[Math.min(stepIndex, Math.max(steps.length - 1, 0))]);
	let isLast = $derived(stepIndex >= steps.length - 1);
	let anchorEl = $derived(
		current?.id === 'profiles'
			? profilesAnchorEl
			: current?.id === 'tester'
				? testerAnchorEl
				: connectAnchorEl
	);
	let visible = $derived(show && !dismissed && Boolean(current) && Boolean(anchorRect));

	function updateAnchorRect() {
		anchorRect = anchorEl?.getBoundingClientRect();
	}

	function hintStyle(rect: DOMRect, placement: HintStep['placement']) {
		if (placement === 'bottom') {
			const left = Math.min(Math.max(8, rect.left + rect.width / 2 - 144), window.innerWidth - 304);
			return `top: ${rect.bottom + 10}px; left: ${left}px;`;
		}
		return `top: ${rect.top}px; left: ${rect.right + 10}px;`;
	}

	function finish() {
		if (dismissed) return;
		dismissed = true;
		markVMcpCreationHintSeen(storageKey);
		onDismiss?.();
	}

	function advance() {
		if (isLast) {
			finish();
			return;
		}
		stepIndex += 1;
	}

	$effect(() => {
		void anchorEl;
		void show;
		void current?.id;
		updateAnchorRect();
	});

	$effect(() => {
		if (visible) {
			markVMcpCreationHintSeen(storageKey);
		}
	});

	onMount(() => {
		dismissed = hasSeenVMcpCreationHint(storageKey);

		const refresh = () => updateAnchorRect();
		window.addEventListener('resize', refresh);
		window.addEventListener('scroll', refresh, true);
		return () => {
			window.removeEventListener('resize', refresh);
			window.removeEventListener('scroll', refresh, true);
		};
	});
</script>

{#if visible && current && anchorRect}
	<button
		type="button"
		class="fixed inset-0 z-69 cursor-default bg-transparent"
		aria-label="Continue creation tips"
		onclick={advance}
	></button>

	<div
		class="pointer-events-none fixed z-70 rounded-md ring-2 ring-primary shadow-[0_0_0_4px_color-mix(in_oklab,var(--color-primary)_18%,transparent)]"
		style="top: {anchorRect.top - 2}px; left: {anchorRect.left - 2}px; width: {anchorRect.width +
			4}px; height: {anchorRect.height + 4}px;"
		aria-hidden="true"
	></div>

	{#key current.id}
		<div
			class={twMerge('pointer-events-auto fixed z-71 w-72', klass)}
			style={hintStyle(anchorRect, current.placement)}
			in:fly={{
				x: current.placement === 'right' ? -8 : 0,
				y: current.placement === 'bottom' ? -8 : 0,
				duration: 220
			}}
			role="dialog"
			aria-labelledby="vmcp-creation-hint-title"
			aria-describedby="vmcp-creation-hint-description"
		>
			<div
				class="bg-base-100/90 dark:bg-base-300/90 border-base-300 dark:border-base-400 relative rounded-lg border p-3 shadow-lg backdrop-blur-sm"
			>
				{#if current.placement === 'right'}
					<div
						class="bg-base-100 dark:bg-base-300 border-base-300 dark:border-base-400 absolute top-4 -left-1 size-2 rotate-45 border-b border-l"
						aria-hidden="true"
					></div>
				{:else}
					<div
						class="bg-base-100 dark:bg-base-300 border-base-300 dark:border-base-400 absolute -top-1 left-1/2 size-2 -translate-x-1/2 rotate-45 border-t border-l"
						aria-hidden="true"
					></div>
				{/if}

				<div class="flex items-start justify-between gap-2">
					<p
						id="vmcp-creation-hint-title"
						class="text-muted-content font-mono text-[0.625rem] tracking-[0.14em] uppercase"
					>
						{current.title}
					</p>
					<div class="flex items-center gap-1">
						<p class="text-muted-content font-mono text-[0.625rem] tracking-[0.14em]">
							{stepIndex + 1}/{steps.length}
						</p>
						<button
							type="button"
							class="text-muted-content hover:text-base-content -mt-1 -mr-1 rounded-sm p-1 transition-colors"
							aria-label={isLast ? 'Dismiss creation tips' : 'Next creation tip'}
							onclick={advance}
						>
							<X class="size-3" />
						</button>
					</div>
				</div>

				{#if current.id === 'profiles'}
					{@render profilesStage()}
				{:else if current.id === 'tester'}
					{@render testerStage()}
				{:else}
					{@render connectStage()}
				{/if}

				<p id="vmcp-creation-hint-description" class="text-muted-content mt-2 text-xs font-light">
					{current.description}
				</p>
			</div>
		</div>
	{/key}
{/if}

{#snippet profilesStage()}
	<div
		class="border-base-300 dark:border-base-400 bg-base-200/40 dark:bg-base-200/20 relative mt-2 h-36 overflow-hidden rounded-md border"
		aria-hidden="true"
	>
		<div class="vmcp-creation-hint-glow absolute inset-0"></div>

		<svg class="text-primary absolute inset-0 size-full" viewBox="0 0 160 160" aria-hidden="true">
			<g transform="translate(80 80)">
				{#each arrows as arrow (arrow.angle)}
					<g
						transform="rotate({arrow.angle})"
						style={`--hint-travel: ${arrow.length}px; --hint-delay: ${arrow.delay}ms`}
					>
						<g class="vmcp-creation-hint-flight">
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

{#snippet testerStage()}
	<div
		class="border-base-300 dark:border-base-400 bg-base-200/40 dark:bg-base-200/20 relative mt-2 h-36 overflow-hidden rounded-md border"
		aria-hidden="true"
	>
		<div class="vmcp-creation-hint-glow absolute inset-0"></div>
		<div
			class="bg-base-100 dark:bg-base-300 absolute top-1/2 left-1/2 flex -translate-x-1/2 -translate-y-1/2 gap-1 rounded-lg p-1 shadow-md"
		>
			<span class="text-muted-content px-2 py-1 font-mono text-[0.5rem] uppercase">Designer</span>
			<span
				class="vmcp-tester-hint-tab bg-base-300 dark:bg-base-100 flex items-center gap-1 rounded-md px-2 py-1 font-mono text-[0.5rem] uppercase"
			>
				<FlaskConical class="text-primary size-2.5" /> Tester
			</span>
		</div>
		<MousePointer2
			class="vmcp-tester-hint-cursor fill-base-content text-base-100 dark:text-base-300 absolute top-1/2 left-1/2 size-4 stroke-[1.5]"
		/>
	</div>
{/snippet}

{#snippet connectStage()}
	<div
		class="border-base-300 dark:border-base-400 bg-base-200/40 dark:bg-base-200/20 relative mt-2 h-36 overflow-hidden rounded-md border"
		aria-hidden="true"
	>
		<div class="vmcp-creation-hint-glow absolute inset-0"></div>
		<div
			class="vmcp-connect-hint-target absolute top-1/2 left-1/2 flex w-36 -translate-x-1/2 -translate-y-1/2 items-center overflow-hidden rounded-lg border border-base-300 dark:border-base-400"
		>
			<div
				class="bg-primary/10 text-primary flex grow items-center justify-center gap-1 py-2 font-mono text-[0.5rem] uppercase"
			>
				<Unplug class="size-2.5" /> Connect
			</div>
			<span class="bg-base-content/15 block h-6 w-px"></span>
			<span class="bg-base-content/15 mx-2 block h-1 w-4 rounded-full"></span>
		</div>
		<MousePointer2
			class="vmcp-connect-hint-cursor fill-base-content text-base-100 dark:text-base-300 absolute top-1/2 left-1/2 size-4 stroke-[1.5]"
		/>
	</div>
{/snippet}

<style>
	.vmcp-creation-hint-glow {
		background: radial-gradient(
			circle at center,
			color-mix(in oklab, var(--color-primary) 12%, transparent) 0%,
			transparent 68%
		);
	}

	.vmcp-creation-hint-flight {
		opacity: 0;
		transform: translateY(18px) scale(0.2);
		transform-origin: 0 0;
		animation: vmcp-creation-hint-fly 3.6s cubic-bezier(0.22, 1, 0.36, 1) both;
		animation-delay: var(--hint-delay, 0ms);
	}

	.vmcp-tester-hint-tab {
		animation: vmcp-creation-hint-pulse 2.4s ease-in-out infinite;
	}

	.vmcp-tester-hint-cursor {
		animation: vmcp-tester-hint-click 2.4s ease-in-out infinite;
	}

	.vmcp-connect-hint-target {
		animation: vmcp-creation-hint-pulse 2.4s ease-in-out infinite;
	}

	.vmcp-connect-hint-cursor {
		animation: vmcp-connect-hint-click 2.4s ease-in-out infinite;
	}

	@keyframes vmcp-creation-hint-fly {
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

	@keyframes vmcp-creation-hint-pulse {
		0%,
		100% {
			box-shadow: none;
		}
		50% {
			box-shadow: 0 0 0 4px color-mix(in oklab, var(--color-primary) 22%, transparent);
		}
	}

	@keyframes vmcp-tester-hint-click {
		0%,
		100% {
			transform: translate(18px, 10px) scale(1);
			opacity: 0.35;
		}
		40%,
		55% {
			transform: translate(8px, 4px) scale(0.9);
			opacity: 1;
		}
	}

	@keyframes vmcp-connect-hint-click {
		0%,
		100% {
			transform: translate(28px, 16px) scale(1);
			opacity: 0.35;
		}
		40%,
		55% {
			transform: translate(4px, 8px) scale(0.9);
			opacity: 1;
		}
	}

	@media (prefers-reduced-motion: reduce) {
		.vmcp-creation-hint-flight {
			animation: none;
			opacity: 1;
			transform: translateY(calc(-1 * var(--hint-travel) + 18px)) scale(1);
		}

		.vmcp-tester-hint-tab,
		.vmcp-tester-hint-cursor,
		.vmcp-connect-hint-target,
		.vmcp-connect-hint-cursor {
			animation: none;
		}

		.vmcp-tester-hint-cursor,
		.vmcp-connect-hint-cursor {
			opacity: 1;
			transform: translate(8px, 4px);
		}
	}
</style>
