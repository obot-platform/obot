<script module>
	export const INTRODUCTION_HINT_TEXT =
		"You've created your Virtual MCP! Take a quick tour of three capabilities that help you control access, test your setup, and connect it to your AI clients and agents.";
	export const PROFILES_HINT_TEXT =
		'Create and edit Profiles to control which tools different users, groups, and agents can access—all through the same Virtual MCP endpoint.';
	export const TESTER_HINT_TEXT =
		'Use Inspector to interact with your Virtual MCP before connecting it. Chat with your tools, explore available capabilities, and test how your Virtual MCP behaves.';
	export const CONNECT_HINT_TEXT =
		'Ready to put it to work? Use Connect to quickly configure your Virtual MCP with popular AI clients and agents using setup links, configuration, or CLI commands.';
</script>

<script lang="ts">
	import {
		hasSeenVMcpCreationHint,
		markVMcpCreationHintSeen,
		VMCP_CREATION_HINT_STORAGE_KEY
	} from '$lib/runes/vmcps/vmcpToolFlow.svelte';
	import { X } from '@lucide/svelte';
	import { onMount } from 'svelte';
	import { fly } from 'svelte/transition';
	import { twMerge } from 'tailwind-merge';

	const HINT_WIDTH_PX = 384;
	const HINT_VIEWPORT_PADDING_PX = 8;

	type HintStepId = 'introduction' | 'profiles' | 'tester' | 'connect';

	interface HintStep {
		id: HintStepId;
		title: string;
		description: string;
		placement: 'center' | 'right' | 'bottom';
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
			id: 'introduction',
			title: 'Your Virtual MCP is ready',
			description: INTRODUCTION_HINT_TEXT,
			placement: 'center'
		},
		{
			id: 'profiles',
			title: 'Control access with Profiles',
			description: PROFILES_HINT_TEXT,
			placement: 'right'
		},
		{
			id: 'tester',
			title: 'Explore with Inspector',
			description: TESTER_HINT_TEXT,
			placement: 'right'
		},
		{
			id: 'connect',
			title: 'Connect your Virtual MCP',
			description: CONNECT_HINT_TEXT,
			placement: 'bottom'
		}
	];

	let dismissed = $state(true);
	let stepIndex = $state(0);
	let anchorRect = $state<DOMRect>();

	let steps = $derived(
		allSteps.filter((step) => {
			if (step.id === 'introduction') {
				return includeProfiles || includeTester || includeConnect;
			}
			if (step.id === 'profiles') return includeProfiles;
			if (step.id === 'tester') return includeTester;
			return includeConnect;
		})
	);
	let current = $derived(steps[Math.min(stepIndex, Math.max(steps.length - 1, 0))]);
	let isFirst = $derived(stepIndex === 0);
	let isLast = $derived(stepIndex >= steps.length - 1);
	let tourStepCount = $derived(steps.filter((step) => step.id !== 'introduction').length);
	let anchorEl = $derived(
		current?.id === 'profiles'
			? profilesAnchorEl
			: current?.id === 'tester'
				? testerAnchorEl
				: connectAnchorEl
	);
	let visible = $derived(
		show && !dismissed && Boolean(current) && (current.id === 'introduction' || Boolean(anchorRect))
	);

	function updateAnchorRect() {
		anchorRect = anchorEl?.getBoundingClientRect();
	}

	function highlightStyle(rect: DOMRect) {
		return `top: ${rect.top - 2}px; left: ${rect.left - 2}px; width: ${rect.width + 4}px; height: ${rect.height + 4}px;`;
	}

	function hintStyle(rect: DOMRect | undefined, placement: HintStep['placement']) {
		if (placement === 'center') {
			return 'top: 50%; left: 50%; transform: translate(-50%, -50%);';
		}
		if (!rect) return '';
		if (placement === 'bottom') {
			const halfWidth = HINT_WIDTH_PX / 2;
			const left = Math.min(
				Math.max(HINT_VIEWPORT_PADDING_PX, rect.left + rect.width / 2 - halfWidth),
				window.innerWidth - HINT_WIDTH_PX - HINT_VIEWPORT_PADDING_PX
			);
			return `top: ${rect.bottom + 10}px; left: ${left}px;`;
		}
		const left = Math.min(
			Math.max(HINT_VIEWPORT_PADDING_PX, rect.right + 10),
			window.innerWidth - HINT_WIDTH_PX - HINT_VIEWPORT_PADDING_PX
		);
		return `top: ${rect.top}px; left: ${left}px;`;
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
		if (visible && isLast) {
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

{#if visible && current}
	{#if current.id === 'introduction'}
		<div
			class="pointer-events-none fixed inset-0 z-69 bg-black/40 dark:bg-black/55"
			aria-hidden="true"
		></div>
	{:else if anchorRect}
		<div
			class="pointer-events-none fixed z-69 rounded-md shadow-[0_0_0_9999px_rgba(0,0,0,0.4)] dark:shadow-[0_0_0_9999px_rgba(0,0,0,0.55)]"
			style={highlightStyle(anchorRect)}
			aria-hidden="true"
		></div>
	{/if}

	<button
		type="button"
		class="fixed inset-0 z-69 cursor-default bg-transparent"
		aria-label="Continue creation tips"
		onclick={advance}
	></button>

	{#if current.id !== 'introduction' && anchorRect}
		<div
			class="pointer-events-none fixed z-70 rounded-md ring-2 ring-primary shadow-[0_0_0_4px_color-mix(in_oklab,var(--color-primary)_18%,transparent)]"
			style={highlightStyle(anchorRect)}
			aria-hidden="true"
		></div>
	{/if}

	{#key current.id}
		<div
			class={twMerge('pointer-events-auto fixed z-71', klass)}
			style={`width: ${HINT_WIDTH_PX}px; ${hintStyle(anchorRect, current.placement)}`}
			in:fly={{
				x: current.placement === 'right' ? -8 : 0,
				y: current.placement === 'bottom' || current.placement === 'center' ? -8 : 0,
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
				{:else if current.placement === 'bottom'}
					<div
						class="bg-base-100 dark:bg-base-300 border-base-300 dark:border-base-400 absolute -top-1 left-1/2 size-2 -translate-x-1/2 rotate-45 border-t border-l"
						aria-hidden="true"
					></div>
				{/if}

				<div class="flex items-start justify-between gap-2">
					<p
						id="vmcp-creation-hint-title"
						class="font-mono text-[0.625rem] tracking-[0.14em] uppercase"
					>
						{current.title}
					</p>
					<div class="flex items-center gap-1">
						{#if current.id !== 'introduction'}
							<p class="font-mono uppercase font-semibold text-[0.625rem] tracking-[0.14em]">
								tutorial {stepIndex}/{tourStepCount}
							</p>
						{/if}
						<button
							type="button"
							class="text-muted-content hover:text-base-content -mt-1 -mr-1 rounded-sm p-1 transition-colors"
							aria-label="Dismiss creation tips"
							onclick={finish}
						>
							<X class="size-3" />
						</button>
					</div>
				</div>

				<p id="vmcp-creation-hint-description" class="mt-2 text-xs font-light leading-relaxed">
					{current.description}
				</p>

				<div class="mt-3 flex justify-end">
					<button
						type="button"
						class="btn btn-primary btn-xs text-xs"
						aria-label={isLast ? 'Finish tour' : isFirst ? 'Start tour' : 'Go to next tip'}
						onclick={advance}
					>
						{isLast ? 'Done' : isFirst ? 'Start Tour' : 'Next'}
					</button>
				</div>
			</div>
		</div>
	{/key}
{/if}
