<script module>
	export const PROFILES_HINT_TEXT =
		'Control which tools different users, groups, or agents can access. Create profiles to give each identity the right set of tools—all through the same Virtual MCP.';
	export const TESTER_HINT_TEXT =
		'Explore and test your Virtual MCP before connecting it. Inspect available tools and resources, chat with your Virtual MCP, and see how everything works.';
	export const CONNECT_HINT_TEXT =
		'Ready to use your Virtual MCP? Connect it to popular AI clients and agents like Claude, Cursor, and Codex using the setup option that works for you.';
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
			title: 'Inspector',
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

	function highlightStyle(rect: DOMRect) {
		return `top: ${rect.top - 2}px; left: ${rect.left - 2}px; width: ${rect.width + 4}px; height: ${rect.height + 4}px;`;
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

{#if visible && current && anchorRect}
	<div
		class="pointer-events-none fixed z-69 rounded-md shadow-[0_0_0_9999px_rgba(0,0,0,0.4)] dark:shadow-[0_0_0_9999px_rgba(0,0,0,0.55)]"
		style={highlightStyle(anchorRect)}
		aria-hidden="true"
	></div>

	<button
		type="button"
		class="fixed inset-0 z-69 cursor-default bg-transparent"
		aria-label="Continue creation tips"
		onclick={advance}
	></button>

	<div
		class="pointer-events-none fixed z-70 rounded-md ring-2 ring-primary shadow-[0_0_0_4px_color-mix(in_oklab,var(--color-primary)_18%,transparent)]"
		style={highlightStyle(anchorRect)}
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

				<p id="vmcp-creation-hint-description" class="text-muted-content mt-2 text-xs font-light">
					{current.description}
				</p>
			</div>
		</div>
	{/key}
{/if}
