<script lang="ts">
	import { page } from '$app/state';
	import { columnResize } from '$lib/actions/resize';
	import { toHTMLFromMarkdownWithNewTabLinks } from '$lib/markdown';
	import {
		type GuideAction,
		type GuideButton,
		type GuideContent,
		type GuideDialog,
		type GuideListener
	} from '$lib/services/guides';
	import { darkMode, errors, guide, mcpServersAndEntries } from '$lib/stores';
	import CopyField from '../CopyField.svelte';
	import ResponsiveDialog from '../ResponsiveDialog.svelte';
	import IconButton from '../primitives/IconButton.svelte';
	import Obot from './Obot.svelte';
	import { createGuideHighlighter, type GuideHighlighter } from './highlight';
	import { GripVertical, X } from '@lucide/svelte';
	import { noop } from 'es-toolkit';
	import { onDestroy, onMount, tick } from 'svelte';
	import { fade, fly } from 'svelte/transition';

	const CONTENT_FADE_MS = 500;
	// Short poll after clicks (nav expand). Highlight waits separately for post-nav targets.
	const ACTION_RESOLVE_ATTEMPTS = 10;
	const ACTION_RESOLVE_INTERVAL_MS = 50;

	function youtubeEmbedUrl(url: string): string | undefined {
		try {
			const parsed = new URL(url);
			const host = parsed.hostname.replace(/^www\./, '');
			let id: string | undefined;

			if (host === 'youtu.be') {
				id = parsed.pathname.slice(1).split('/')[0];
			} else if (host === 'youtube.com' || host === 'm.youtube.com') {
				if (parsed.pathname === '/watch') {
					id = parsed.searchParams.get('v') ?? undefined;
				} else {
					const match = parsed.pathname.match(/^\/(embed|shorts|live)\/([^/?#]+)/);
					id = match?.[2];
				}
			}

			return id ? `https://www.youtube.com/embed/${id}` : undefined;
		} catch {
			return undefined;
		}
	}
	let highlighter: GuideHighlighter | undefined = undefined;
	let actionGeneration = 0;
	let guideSessionGeneration = 0;

	let panel = $state<HTMLDivElement>();
	let popoverRoot = $state<HTMLDivElement>();
	let spacerWidth = $state(384); // 24rem — matches default panel width
	let revealGeneration = 0;

	let guideSuccessDialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let stepDialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let stepDialogContent = $state<GuideDialog>();
	let stepDialogOpen = $state(false);

	// scroll states
	const SCROLL_THRESHOLD = 48;
	let scrollContainer = $state<HTMLDivElement | undefined>(undefined);
	let scrollContent = $state<HTMLDivElement | undefined>(undefined);
	let disabledAutoScroll = $state(false);
	let ignoreScrollEvent = false;
	let ignoreScrollTimeout: ReturnType<typeof setTimeout> | undefined;

	let listenerHandler: ((e: MouseEvent) => void) | undefined = undefined;
	let activeListener = $state<GuideListener | undefined>(undefined);
	let pendingNext = $state<{ action: GuideAction | GuideAction[] } | undefined>(undefined);
	let guideCompleted = $state(false);
	let initializedGuideId: string | undefined;
	let wasMcpServersLoading = false;

	onMount(() => {
		highlighter = createGuideHighlighter({
			allowClose: false,
			overlayClickBehavior: noop,
			onObotVisibilityChange: (visible) => {
				guide.showObotInPanel = visible;
			}
		});
	});

	$effect(() => {
		const loading = mcpServersAndEntries.current.loading;
		// include making sure entries have loaded before proceeding with action/rerendering highlight
		if (loading) {
			wasMcpServersLoading = true;
			return;
		}
		if (!wasMcpServersLoading || !guide.selectedGuide) return;
		wasMcpServersLoading = false;

		const sessionGeneration = guideSessionGeneration;
		const step = guide.stream[guide.currentStep];
		if (!step?.action) {
			highlighter?.refresh();
			return;
		}

		void (async () => {
			await tick();
			await new Promise<void>((resolve) => requestAnimationFrame(() => resolve()));
			if (!isCurrentGuideSession(sessionGeneration) || !step.action) return;
			void handleStepAction(step.action);
		})();
	});

	$effect(() => {
		const el = popoverRoot;
		if (!el || !guide.selectedGuide) return;

		let bumpRaf = 0;

		const show = () => {
			if (!el.isConnected) return;
			if (!el.matches(':popover-open')) {
				el.showPopover();
			}
		};

		/**
		 * Modal <dialog>s and popovers share the top layer (LIFO). If something still opens
		 * a true modal while the guide is up, re-enter the top layer so the panel stays above it.
		 * (App dialogs normally open non-modally via openDialog / dialogAnimation.)
		 */
		const bumpAboveDialogs = () => {
			if (!el.isConnected || !guide.selectedGuide) return;
			const blockingModal = Array.from(document.querySelectorAll('dialog[open]')).find(
				(d) => d instanceof HTMLDialogElement && d.matches(':modal') && !el.contains(d)
			);
			if (!blockingModal) return;

			cancelAnimationFrame(bumpRaf);
			bumpRaf = requestAnimationFrame(() => {
				if (!el.isConnected || !guide.selectedGuide) return;
				if (!document.querySelector('dialog[open]:modal')) return;
				if (el.matches(':popover-open')) {
					el.hidePopover();
				}
				el.showPopover();
				highlighter?.refresh();
			});
		};

		queueMicrotask(show);

		const observer = new MutationObserver((mutations) => {
			for (const mutation of mutations) {
				if (mutation.attributeName !== 'open') continue;
				const target = mutation.target;
				if (!(target instanceof HTMLDialogElement) || !target.open) continue;
				// Dialogs opened from within the guide (e.g. PreferredClient) should stay on top
				if (el.contains(target)) continue;
				bumpAboveDialogs();
				break;
			}
		});

		observer.observe(document.documentElement, {
			attributes: true,
			subtree: true,
			attributeFilter: ['open']
		});

		if (document.querySelector('dialog[open]')) {
			queueMicrotask(bumpAboveDialogs);
		}

		return () => {
			cancelAnimationFrame(bumpRaf);
			observer.disconnect();
			if (el.matches(':popover-open')) {
				el.hidePopover();
			}
		};
	});

	$effect(() => {
		const el = popoverRoot;
		if (!el || !guide.selectedGuide) return;

		const sync = () => {
			const width = el.getBoundingClientRect().width;
			spacerWidth = width;
			document.documentElement.style.setProperty('--guide-panel-width', `${width}px`);
		};
		sync();
		const ro = new ResizeObserver(sync);
		ro.observe(el);
		return () => {
			ro.disconnect();
			document.documentElement.style.removeProperty('--guide-panel-width');
		};
	});

	$effect(() => {
		void darkMode.isDark;
		highlighter?.setOverlayColor(darkMode.isDark ? 'rgba(0, 0, 0, 1)' : 'rgba(0, 0, 0, 0.35)');
	});

	$effect(() => {
		const selected = guide.selectedGuide;
		if (!selected) {
			initializedGuideId = undefined;
			return;
		}
		if (initializedGuideId === selected.id) return;
		initializedGuideId = selected.id;

		guideSessionGeneration += 1;
		actionGeneration += 1;
		cleanupListener();
		disabledAutoScroll = false;
		guideCompleted = false;

		if (selected.id !== guide.previousGuide?.id) {
			guide.previousGuide = selected;
			revealGeneration += 1;
			guide.activeSteps = [...selected.steps];
			guide.stream = guide.activeSteps[0] ? [guide.activeSteps[0]] : [];
			guide.currentStep = 0;
			guide.revealed = [];
			if (guide.stream.length) {
				void animateStepReveal(0);
			}
			return;
		}

		// Route navigation replaces Layout (and this component) while guide state remains in
		// the shared store. Resume an interrupted reveal and restore the current step action.
		if (!guide.activeSteps.length) {
			guide.activeSteps = [...selected.steps];
		}
		const step = guide.stream[guide.currentStep];
		if (!step) return;

		const reveal = guide.revealed[guide.currentStep];
		const hasButtons = Boolean(step.buttons?.length);
		const revealIncomplete =
			!reveal || reveal.contentCount < step.content.length || (hasButtons && !reveal.showButton);
		if (revealIncomplete) {
			revealGeneration += 1;
			void animateStepReveal(guide.currentStep);
		} else {
			if (step.action) void handleStepAction(step.action);
			if (isLastStepComplete(guide.currentStep, step)) {
				guideCompleted = true;
			}
		}
	});

	/** True when a step/action still has listener, next (Next-button chain), or dialog work. */
	function actionHasPendingChain(action: GuideAction | GuideAction[] | undefined): boolean {
		if (!action) return false;
		for (const a of Array.isArray(action) ? action : [action]) {
			if (a.dialog) return true;
			if (a.listener) {
				// Listener requires progression; nested actions under it are also pending.
				return true;
			}
			if (a.next) return true;
		}
		return false;
	}

	function isLastStepComplete(
		stepIndex: number,
		step: { action?: GuideAction | GuideAction[]; buttons?: GuideButton[] }
	) {
		return (
			stepIndex === guide.activeSteps.length - 1 &&
			!step.buttons?.length &&
			!actionHasPendingChain(step.action) &&
			!activeListener &&
			!pendingNext
		);
	}

	function maybeCompleteGuideAfterAction(resolved: GuideAction) {
		if (resolved.success) {
			handleNextStep();
			return;
		}
		// Nested listener/next chain ended on the last step without success — mark complete.
		if (
			!resolved.listener &&
			!resolved.next &&
			!resolved.dialog &&
			guide.currentStep === guide.activeSteps.length - 1 &&
			!guide.stream[guide.currentStep]?.buttons?.length
		) {
			guideCompleted = true;
		}
	}

	function cleanupListener() {
		if (listenerHandler) {
			window.removeEventListener('click', listenerHandler, true);
			listenerHandler = undefined;
		}
		activeListener = undefined;
		pendingNext = undefined;
	}

	function sleep(ms: number) {
		return new Promise<void>((resolve) => setTimeout(resolve, ms));
	}

	function isCurrentGuideSession(generation: number) {
		return generation === guideSessionGeneration && Boolean(guide.selectedGuide);
	}

	function findListenerTarget(listener: GuideListener): HTMLElement | undefined {
		if (listener.id) {
			const el = document.getElementById(listener.id);
			if (el instanceof HTMLElement) return el;
		}
		if (listener.beginsWith) {
			for (const prefix of listener.beginsWith) {
				const el = document.querySelector(`[id^="${CSS.escape(prefix)}"]`);
				if (el instanceof HTMLElement) return el;
			}
		}
		const active = highlighter?.getDriver().getActiveElement();
		return active instanceof HTMLElement ? active : undefined;
	}

	function clickGuideTarget(el: HTMLElement) {
		const interactive = el.matches('a, button, [role="button"], input, select, textarea')
			? el
			: el.querySelector<HTMLElement>('a, button, [role="button"]');
		(interactive ?? el).click();
	}

	function handleCompleteGuide() {
		guideCompleted = true;
		guide.selectedGuide = undefined;
		requestAnimationFrame(() => {
			guideSuccessDialog?.open();
		});
	}

	async function handlePrimaryNext() {
		const sessionGeneration = guideSessionGeneration;
		if (!isCurrentGuideSession(sessionGeneration)) return;

		if (activeListener) {
			const listener = activeListener;

			if (listener.skipClickTargetOnNext) {
				highlighter?.destroy();
				void handleStepAction(listener.action, { wait: true });
				return;
			}

			let target = findListenerTarget(listener);
			if (!target) {
				for (let i = 0; i < ACTION_RESOLVE_ATTEMPTS && !target; i++) {
					await tick();
					await sleep(ACTION_RESOLVE_INTERVAL_MS);
					if (!isCurrentGuideSession(sessionGeneration)) return;
					target = findListenerTarget(listener);
				}
			}
			if (target && isCurrentGuideSession(sessionGeneration)) {
				clickGuideTarget(target);
				return;
			}
		}

		if (pendingNext) {
			const nextAction = pendingNext.action;
			highlighter?.destroy();
			void handleStepAction(nextAction, { wait: true });
			return;
		}

		const step = guide.stream[guide.currentStep];
		if (step?.buttons?.length) return;

		if (isCurrentGuideSession(sessionGeneration)) {
			handleNextStep();
		}
	}

	async function animateStepReveal(stepIndex: number) {
		const generation = revealGeneration;
		const step = guide.stream[stepIndex];
		if (!step) return;

		if (step.action) {
			void handleStepAction(step.action);
		}

		const next = guide.revealed.slice(0, stepIndex);
		next[stepIndex] = { contentCount: 0, showButton: false };
		guide.revealed = next;

		for (let j = 0; j < step.content.length; j++) {
			if (j > 0) await sleep(CONTENT_FADE_MS);
			if (generation !== revealGeneration) return;
			guide.revealed = guide.revealed.map((r, idx) =>
				idx === stepIndex ? { ...r, contentCount: j + 1 } : r
			);
		}

		if (step.buttons?.length) {
			await sleep(CONTENT_FADE_MS);
			if (generation !== revealGeneration) return;
			guide.revealed = guide.revealed.map((r, idx) =>
				idx === stepIndex ? { ...r, showButton: true } : r
			);
		}

		if (generation !== revealGeneration) return;
		if (isLastStepComplete(stepIndex, step)) {
			handleCompleteGuide();
		} else if (!step.action && !step.buttons?.length) {
			handleNextStep();
		}
	}

	async function handleGuideButton(button: GuideButton, stepIndex: number) {
		const sessionGeneration = guideSessionGeneration;
		if (!isCurrentGuideSession(sessionGeneration) || guide.currentStep !== stepIndex) return;

		if (button.steps) {
			guide.activeSteps = [...guide.activeSteps.slice(0, stepIndex + 1), ...button.steps];
			guideCompleted = false;
		}

		if (button.action) {
			const action = await resolveAction(button.action);
			if (!isCurrentGuideSession(sessionGeneration)) return;
			if (!action) {
				errors.append(
					`Failed to resolve guide action (guide=${guide.selectedGuide?.id ?? 'unknown'}, step=${stepIndex})`
				);
				return;
			}
			await handleStepAction(action);
		}

		if (
			button.steps &&
			guide.currentStep === stepIndex &&
			isCurrentGuideSession(sessionGeneration)
		) {
			handleNextStep();
		}
	}

	function actionMatches(action: GuideAction): boolean {
		if (action.routeContains && !page.url.pathname.includes(action.routeContains)) {
			return false;
		}
		if (action.elementExists && !document.getElementById(action.elementExists)) {
			return false;
		}
		if (action.elementMissing && document.getElementById(action.elementMissing)) {
			return false;
		}
		return true;
	}

	function hasActionGuard(action: GuideAction): boolean {
		return Boolean(action.routeContains || action.elementExists || action.elementMissing);
	}

	function canAppearLater(action: GuideAction): boolean {
		// After a click (nav expand, route change), element presence may change.
		return Boolean(action.elementExists || action.elementMissing);
	}

	async function resolveAction(
		action?: GuideAction | GuideAction[],
		options?: { wait?: boolean }
	): Promise<GuideAction | undefined> {
		if (!action) return undefined;
		if (!Array.isArray(action)) return action;

		const attempts = options?.wait && action.some(canAppearLater) ? ACTION_RESOLVE_ATTEMPTS : 1;

		for (let i = 0; i < attempts; i++) {
			const guarded = action.find((a) => hasActionGuard(a) && actionMatches(a));
			if (guarded) return guarded;

			if (i < attempts - 1) {
				await tick();
				await sleep(ACTION_RESOLVE_INTERVAL_MS);
			}
		}

		return action.find((a) => !hasActionGuard(a) && actionMatches(a));
	}

	function closeGuideElement(action: GuideAction) {
		if (!action.closeExistingElement || !action.elementExists) return;
		const el = document.getElementById(action.elementExists);
		if (el instanceof HTMLDialogElement) {
			if (el.open) el.close();
			return;
		}
		const closeBtn = el?.querySelector<HTMLElement>('.dialog-close-btn, [data-guide-close]');
		closeBtn?.click();
	}

	async function handleStepAction(
		action: GuideAction | GuideAction[],
		options?: { wait?: boolean }
	) {
		const sessionGeneration = guideSessionGeneration;
		cleanupListener();
		const generation = ++actionGeneration;
		const resolved = await resolveAction(action, options);
		if (generation !== actionGeneration || !isCurrentGuideSession(sessionGeneration) || !resolved)
			return;

		if (resolved.closeExistingElement) {
			closeGuideElement(resolved);
		}

		if (resolved.highlight) {
			void highlighter?.highlight(resolved.highlight);
		}

		if (resolved.listener) {
			registerGuideListener(resolved.listener);
		}

		if (resolved.next) {
			pendingNext = resolved.next;
		}

		if (resolved.dialog) {
			stepDialogContent = resolved.dialog;
			stepDialog?.open();
		}

		maybeCompleteGuideAfterAction(resolved);
	}

	function eventMatchesListener(e: MouseEvent, listener: GuideListener): boolean {
		const path = e.composedPath();
		for (const node of path) {
			if (!(node instanceof Element)) continue;
			if (listener.id && node.id === listener.id) return true;
			if (listener.beginsWith?.some((prefix) => node.id.startsWith(prefix))) return true;
		}

		// Fallback: click landed on the currently highlighted element (id may be on a wrapper).
		const active = highlighter?.getDriver().getActiveElement();
		const target = e.target;
		return Boolean(active instanceof Element && target instanceof Node && active.contains(target));
	}

	function registerGuideListener(listener: GuideListener) {
		activeListener = listener;
		listenerHandler = (e: MouseEvent) => {
			if (!eventMatchesListener(e, listener)) return;
			highlighter?.destroy();
			void handleStepAction(listener.action, { wait: true });
		};
		// Capture phase so target stopPropagation (e.g. Connect button) still reaches us
		window.addEventListener('click', listenerHandler, true);
	}

	function handleNextStep() {
		guide.currentStep += 1;
		if (guide.activeSteps[guide.currentStep]) {
			guide.stream = [...guide.stream, guide.activeSteps[guide.currentStep]];
			void animateStepReveal(guide.currentStep);
		} else if (guide.selectedGuide && guide.currentStep >= guide.activeSteps.length) {
			handleCompleteGuide();
		}
	}

	function isNearBottom() {
		if (!scrollContainer) return false;
		const { scrollTop, scrollHeight, clientHeight } = scrollContainer;
		return scrollTop + clientHeight >= scrollHeight - SCROLL_THRESHOLD;
	}

	function scrollToBottom(behavior: 'auto' | 'smooth' = 'smooth') {
		if (!scrollContainer || disabledAutoScroll) return;

		ignoreScrollEvent = true;
		if (ignoreScrollTimeout) clearTimeout(ignoreScrollTimeout);
		scrollContainer.scrollTo({
			top: scrollContainer.scrollHeight,
			behavior
		});

		ignoreScrollTimeout = setTimeout(
			() => {
				ignoreScrollEvent = false;
				ignoreScrollTimeout = undefined;
			},
			behavior === 'smooth' ? CONTENT_FADE_MS + 100 : 50
		);
	}

	function handleScroll() {
		if (!scrollContainer || ignoreScrollEvent) return;
		disabledAutoScroll = !isNearBottom();
	}

	function closeGuide() {
		guideSessionGeneration += 1;
		actionGeneration += 1;
		revealGeneration += 1;
		guide.selectedGuide = undefined;
		guide.stream = [];
		guide.revealed = [];
		guide.currentStep = 0;
		guide.previousGuide = undefined;
		guide.activeSteps = [];
		guideCompleted = false;

		cleanupListener();
		highlighter?.destroy();
	}

	async function handleStepDialogClose() {
		const sessionGeneration = guideSessionGeneration;
		if (!isCurrentGuideSession(sessionGeneration)) return;

		stepDialogOpen = false;
		if (stepDialogContent?.next) {
			await new Promise((resolve) => setTimeout(resolve, 500));
			if (!isCurrentGuideSession(sessionGeneration)) return;
			stepDialogContent = stepDialogContent.next;
			stepDialog?.open();
		} else if (isCurrentGuideSession(sessionGeneration)) {
			handleNextStep();
		}
	}

	// Stick to bottom when steps / reveal progress / next-lessons change
	$effect(() => {
		if (!scrollContainer) return;
		void guide.stream.length;
		void guideCompleted;
		for (const r of guide.revealed) {
			void r?.contentCount;
			void r?.showButton;
		}
		if (disabledAutoScroll) return;

		const raf = requestAnimationFrame(() => scrollToBottom('smooth'));
		const timeout = setTimeout(() => scrollToBottom('smooth'), CONTENT_FADE_MS);
		return () => {
			cancelAnimationFrame(raf);
			clearTimeout(timeout);
		};
	});

	// Keep pinned while animated content continues to grow
	$effect(() => {
		const container = scrollContainer;
		const content = scrollContent;
		if (!container || !content) return;

		const ro = new ResizeObserver(() => {
			if (disabledAutoScroll) return;
			scrollToBottom('auto');
		});
		ro.observe(content);
		return () => ro.disconnect();
	});

	onDestroy(() => {
		guideSessionGeneration += 1;
		actionGeneration += 1;
		revealGeneration += 1;
		cleanupListener();
		highlighter?.destroy();
		if (ignoreScrollTimeout) clearTimeout(ignoreScrollTimeout);
	});
</script>

{#if guide.selectedGuide}
	<div class="min-h-dvh shrink-0" style="width: {spacerWidth}px" aria-hidden="true"></div>
	<div
		bind:this={popoverRoot}
		popover="manual"
		id="quick-start-guide-panel"
		class="guide-panel-popover"
	>
		{#if panel}
			<div
				role="none"
				class="min-h-dvh max-h-dvh h-full w-1 cursor-col-resize bg-primary relative shrink-0"
				use:columnResize={{ column: panel, direction: 'right' }}
			>
				<div
					class="absolute top-1/2 left-0 -translate-y-1/2 p-1 bg-primary rounded-md z-20 -translate-x-2"
				>
					<GripVertical class="size-3 text-primary-content" />
				</div>
			</div>
		{/if}
		<div
			bind:this={panel}
			in:fly={{ x: 100, duration: 200 }}
			class="min-h-dvh max-h-dvh min-w-sm max-w-2xl bg-base-100 relative flex flex-col"
			style="width: 24rem;"
		>
			<div
				class="flex w-full gap-4 items-center justify-between p-4 bg-primary text-primary-content"
			>
				<div>
					<h2 class="font-semibold text-md">Quick Start Guide</h2>
					<p class="text-xs font-light">{guide.selectedGuide.title}</p>
				</div>
				<IconButton onclick={closeGuide} class="btn-sm btn-primary">
					<X class="size-5 text-primary-content" />
				</IconButton>
			</div>
			<div
				bind:this={scrollContainer}
				class="flex flex-col grow overflow-y-auto default-scrollbar-thin"
				onscroll={handleScroll}
			>
				<div bind:this={scrollContent} class="flex flex-col gap-4 p-4">
					{#each guide.stream as step, i (`${guide.selectedGuide.id}-${i}`)}
						{@const stepReveal = guide.revealed[i]}
						{@const stepButtons = step.buttons ?? []}
						{#if stepReveal && stepReveal.contentCount > 0}
							<div class="rounded-box bg-base-200 flex flex-col gap-2 p-4 text-sm">
								{#each step.content.slice(0, stepReveal.contentCount) as content, j (j)}
									<div in:fade={{ duration: CONTENT_FADE_MS }}>
										{@render renderStepContent(content, `${i}-${j}`)}
									</div>
								{/each}
							</div>
						{/if}

						{#if stepButtons.length && stepReveal?.showButton}
							<div class="flex flex-col gap-2" in:fade={{ duration: CONTENT_FADE_MS }}>
								{#each stepButtons as button, buttonIndex (`${i}-${buttonIndex}`)}
									<button
										class="btn btn-sm btn-primary w-full"
										onclick={() => void handleGuideButton(button, i)}
										disabled={guide.currentStep !== i}
									>
										{button.text}
									</button>
								{/each}
							</div>
						{/if}
					{/each}

					{#if guideCompleted}
						<div
							in:fade={{ duration: CONTENT_FADE_MS }}
							class="bg-primary/10 rounded-md p-3 text-xs font-light text-primary"
						>
							Congratulations! You've completed the guide. <br />
							You can close this guide now.
						</div>
					{/if}
				</div>
			</div>
			<div class="flex items-center justify-end gap-2 p-3 border-t border-base-300 relative">
				{#if guide.showObotInPanel}
					<Obot animation={['enter', 'idle']} class="absolute bottom-0 left-2.5" />
				{/if}
				<div class="flex gap-2">
					{#if !guideCompleted}
						<button class="btn btn-secondary btn-sm min-w-24" onclick={() => closeGuide()}
							>Skip All</button
						>
					{/if}
					<button
						class="btn btn-sm btn-primary min-w-24"
						onclick={() => void handlePrimaryNext()}
						disabled={Boolean(guide.stream[guide.currentStep]?.buttons?.length)}
					>
						Next
					</button>
				</div>
			</div>
		</div>

		<ResponsiveDialog
			bind:this={stepDialog}
			title={stepDialogContent?.title}
			onOpen={() => {
				stepDialogOpen = true;
			}}
			onClose={handleStepDialogClose}
			class="w-[75dvw] max-w-[95dvw] max-h-[calc(95dvh)]"
		>
			{#if stepDialogContent && stepDialogOpen}
				{#each stepDialogContent.content as content, i (i)}
					{@render renderStepContent(content, `guide-dialog-content-${i}`)}
				{/each}

				<div class="flex justify-end pt-4 mt-4 border-t border-base-300">
					<button class="btn btn-sm btn-primary" onclick={() => stepDialog?.close()}>
						{stepDialogContent?.next ? 'Next' : 'Close'}
					</button>
				</div>
			{/if}
		</ResponsiveDialog>
	</div>
{/if}

{#if guideCompleted}
	<ResponsiveDialog
		class="max-w-xs"
		animate="slide"
		bind:this={guideSuccessDialog}
		onClose={() => {
			closeGuide();
		}}
		title="Guide Completed"
	>
		<Obot animation={['enter', 'idle']} class="mx-auto" size={96} />
		<h4 class="font-semibold text-center text-lg mb-2">And You're Done!</h4>
		<p class="font-base text-center">You've completed this guide.</p>
		<p class="font-base text-center mb-6 px-8">
			Close this guide and continue exploring the platform.
		</p>
		<button
			class="btn btn-sm btn-primary w-full"
			onclick={() => {
				guideSuccessDialog?.close();
				closeGuide();
			}}
		>
			Close
		</button>
	</ResponsiveDialog>
{/if}

{#snippet renderStepContent(content: GuideContent, index: string | number)}
	{#if typeof content === 'string'}
		<!-- eslint-disable-next-line svelte/no-at-html-tags -- sanitized by toHTMLFromMarkdownWithNewTabLinks -->
		{@html toHTMLFromMarkdownWithNewTabLinks(content)}
	{:else if 'text' in content && content.type === 'code'}
		<CopyField
			id={`code-snippet-${index}`}
			value={content.text}
			variant="code"
			class="bg-base-300"
		/>
	{:else if 'text' in content}
		<p class="text-sm">{content.text}</p>
	{:else if 'imageUrl' in content}
		<img src={content.imageUrl} alt={content.alt} class="w-full h-auto rounded-md" loading="lazy" />
	{:else if 'videoUrl' in content}
		{@const embedUrl = youtubeEmbedUrl(content.videoUrl)}
		{#if embedUrl}
			<div class="aspect-video w-full overflow-hidden rounded-md">
				<iframe
					src={embedUrl}
					title={content.title}
					class="h-full w-full border-0"
					allow="accelerometer; autoplay; clipboard-write; encrypted-media; gyroscope; picture-in-picture; web-share"
					allowfullscreen
					loading="lazy"
				></iframe>
			</div>
		{/if}
	{/if}
{/snippet}
