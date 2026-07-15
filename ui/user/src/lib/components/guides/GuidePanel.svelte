<script lang="ts">
	import { page } from '$app/state';
	import { columnResize } from '$lib/actions/resize';
	import { type GuideAction, type GuideListener, type GuideDialog } from '$lib/services/guides';
	import {
		generateLessonItems,
		getLessonsCompleted,
		setLessonCompleted
	} from '$lib/services/guides/utils';
	import { darkMode, errors, guide, userDeviceSettings } from '$lib/stores';
	import ResponsiveDialog from '../ResponsiveDialog.svelte';
	import IconButton from '../primitives/IconButton.svelte';
	import Obot from './Obot.svelte';
	import PreferredClient from './PreferredClient.svelte';
	import { createGuideHighlighter, type GuideHighlighter } from './highlight';
	import { ChevronRight, GripVertical, X } from '@lucide/svelte';
	import { noop } from 'es-toolkit';
	import { onDestroy, onMount, tick } from 'svelte';
	import { fade, fly, slide } from 'svelte/transition';

	const CONTENT_FADE_MS = 500;
	const ACTION_RESOLVE_ATTEMPTS = 10;

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

	let panel = $state<HTMLDivElement>();
	let popoverRoot = $state<HTMLDivElement>();
	let spacerWidth = $state(384); // 24rem — matches default panel width
	let revealGeneration = 0;

	let preferredClientDialog = $state<ReturnType<typeof PreferredClient>>();
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
	let lessonsCompleted = $state(getLessonsCompleted());
	let lessonItems = $derived(generateLessonItems(lessonsCompleted));
	let nextLessonsReady = $state(false);
	let availableLessons = $derived(
		lessonItems.filter(
			(lesson) => !lesson.completed && lesson.guide?.id !== guide.selectedGuide?.id
		)
	);

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
		const el = popoverRoot;
		if (!el || !guide.selectedGuide) return;

		const show = () => {
			if (!el.isConnected) return;
			if (!el.matches(':popover-open')) {
				el.showPopover();
			}
		};

		const bumpAboveDialogs = () => {
			if (!el.isConnected || !guide.selectedGuide) return;
			if (!document.querySelector('dialog[open]')) return;
			if (el.matches(':popover-open')) {
				el.hidePopover();
			}
			el.showPopover();
			highlighter?.refresh();
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
		if (selected && selected.id !== guide.previousGuide?.id) {
			cleanupListener();
			guide.previousGuide = selected;
			revealGeneration += 1;
			guide.stream = selected.steps[0] ? [selected.steps[0]] : [];
			guide.currentStep = 0;
			guide.revealed = [];
			disabledAutoScroll = false;
			nextLessonsReady = false;
			if (guide.stream.length) {
				void animateStepReveal(0);
			}
		}
	});

	function cleanupListener() {
		if (listenerHandler) {
			window.removeEventListener('click', listenerHandler, true);
			listenerHandler = undefined;
		}
	}

	function sleep(ms: number) {
		return new Promise<void>((resolve) => setTimeout(resolve, ms));
	}

	async function animateStepReveal(stepIndex: number) {
		const generation = revealGeneration;
		const step = guide.stream[stepIndex];
		if (!step) return;

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

		if (step.button) {
			await sleep(CONTENT_FADE_MS);
			if (generation !== revealGeneration) return;
			guide.revealed = guide.revealed.map((r, idx) =>
				idx === stepIndex ? { ...r, showButton: true } : r
			);
		}

		if (generation !== revealGeneration) return;
		const total = guide.selectedGuide?.steps.length ?? 0;
		if (stepIndex === total - 1) {
			setLessonCompleted(guide.selectedGuide?.id);
			lessonsCompleted = getLessonsCompleted();
			nextLessonsReady = true;
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
		// Route won't change while waiting; element presence may after a click expands UI.
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
				await new Promise<void>((resolve) => requestAnimationFrame(() => resolve()));
			}
		}

		return action.find((a) => !hasActionGuard(a) && actionMatches(a));
	}

	async function handleStepAction(
		action: GuideAction | GuideAction[],
		options?: { wait?: boolean }
	) {
		cleanupListener();
		const generation = ++actionGeneration;
		const resolved = await resolveAction(action, options);
		if (generation !== actionGeneration || !resolved) return;

		if (resolved.highlight) {
			void highlighter?.highlight(resolved.highlight);
		}

		if (resolved.listener) {
			registerGuideListener(resolved.listener);
		}

		if (resolved.dialog) {
			stepDialogContent = resolved.dialog;
			stepDialog?.open();
		}

		if (resolved.setPreferredClient) {
			preferredClientDialog?.open();
		}

		if (resolved.success) {
			handleNextStep();
		}
	}

	function registerGuideListener(listener: GuideListener) {
		listenerHandler = (e: MouseEvent) => {
			let el: Element | null = e.target instanceof Element ? e.target : null;
			while (el) {
				const matchesId = Boolean(listener.id && el.id === listener.id);
				const matchesPrefix = Boolean(
					listener.beginsWith?.some((prefix) => el!.id.startsWith(prefix))
				);
				if (matchesId || matchesPrefix) {
					highlighter?.destroy();
					// Wait for DOM updates from the click (e.g. expanding a nav section).
					void handleStepAction(listener.action, { wait: true });
					return;
				}
				el = el.parentElement;
			}
		};
		// Capture phase so target stopPropagation (e.g. Connect button) still reaches us
		window.addEventListener('click', listenerHandler, true);
	}

	function handleNextStep() {
		guide.currentStep += 1;
		if (guide.selectedGuide?.steps[guide.currentStep]) {
			guide.stream = [...guide.stream, guide.selectedGuide.steps[guide.currentStep]];
			void animateStepReveal(guide.currentStep);
		} else if (guide.selectedGuide && guide.currentStep >= guide.selectedGuide.steps.length) {
			setLessonCompleted(guide.selectedGuide?.id);
			lessonsCompleted = getLessonsCompleted();
			nextLessonsReady = true;
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
		guide.selectedGuide = undefined;
		guide.stream = [];
		guide.revealed = [];
		guide.currentStep = 0;
		guide.previousGuide = undefined;
		nextLessonsReady = false;

		cleanupListener();
		highlighter?.destroy();
	}

	async function handleStepDialogClose() {
		stepDialogOpen = false;
		if (stepDialogContent?.next) {
			await new Promise((resolve) => setTimeout(resolve, 500));
			stepDialogContent = stepDialogContent.next;
			stepDialog?.open();
		} else {
			handleNextStep();
		}
	}

	// Stick to bottom when steps / reveal progress / next-lessons change
	$effect(() => {
		if (!scrollContainer) return;
		void guide.stream.length;
		void nextLessonsReady;
		void availableLessons.length;
		for (const r of guide.revealed) {
			void r?.contentCount;
			void r?.showButton;
		}
		if (disabledAutoScroll) return;

		const raf = requestAnimationFrame(() => scrollToBottom('smooth'));
		// Follow slide/fade height growth after the transition
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
						{#if stepReveal && stepReveal.contentCount > 0}
							<div class="rounded-box bg-base-200 flex flex-col gap-2 p-4">
								{#each step.content.slice(0, stepReveal.contentCount) as content, j (j)}
									<p class="text-sm" in:slide={{ axis: 'y', duration: CONTENT_FADE_MS }}>
										{content}
									</p>
								{/each}
							</div>
						{/if}

						{#if step.button && stepReveal?.showButton}
							<button
								class="btn btn-sm btn-primary w-full"
								in:fade={{ duration: CONTENT_FADE_MS }}
								onclick={async () => {
									const action = await resolveAction(step?.button?.action);
									if (action) {
										await handleStepAction(action);
									} else {
										errors.append(
											`Failed to resolve guide action (guide=${guide.selectedGuide?.id ?? 'unknown'}, step=${i})`
										);
									}
								}}
								disabled={guide.currentStep !== i}
							>
								{step.button.text}
							</button>
						{/if}
					{/each}
					{#if nextLessonsReady && availableLessons.length > 0}
						<div class="divider my-2"></div>
						<div in:fade={{ duration: CONTENT_FADE_MS }} class="flex flex-col gap-2">
							<p class="text-xs font-semibold uppercase tracking-wide text-muted-content">
								Continue learning
							</p>
							{#each availableLessons as lesson (lesson.label)}
								<button
									class="btn btn-primary gap-2 items-center text-start h-fit rounded-md! p-2! pr-1!"
									onclick={() => {
										if (!lesson.guide) return;
										guide.selectedGuide = lesson.guide;
									}}
								>
									<div>
										<p class="text-sm font-semibold">{lesson.label}</p>
										<p class="text-xs font-light text-primary-content/50">{lesson.description}</p>
									</div>
									<ChevronRight class="size-5 shrink-0" />
								</button>
							{/each}
						</div>
					{/if}
				</div>
			</div>
			{#if guide.showObotInPanel}
				<Obot animation={['enter', 'idle']} class="absolute bottom-0 bg-base-100 left-1" />
			{/if}
			<div class="p-4">
				<div class="divider"></div>
			</div>
		</div>

		<PreferredClient
			bind:this={preferredClientDialog}
			onSelect={(selected) => {
				userDeviceSettings.setAiClientPreference(selected);
				handleNextStep();
			}}
		/>

		<ResponsiveDialog
			bind:this={stepDialog}
			title={stepDialogContent?.title}
			onOpen={() => {
				stepDialogOpen = true;
			}}
			onClose={handleStepDialogClose}
			class="w-[75dvw] max-w-[95dvw] max-h-[calc(95dvh)]"
		>
			{#if stepDialogContent}
				{#each stepDialogContent.content as content, i (i)}
					{#if 'text' in content}
						<p class="text-sm">{content.text}</p>
					{/if}
					{#if 'imageUrl' in content}
						{#if stepDialogOpen}
							<img src={content.imageUrl} alt={content.alt} class="w-full h-auto" loading="lazy" />
						{:else}
							<div class="bg-base-200 aspect-video w-full rounded-md" aria-hidden="true"></div>
						{/if}
					{/if}
					{#if 'videoUrl' in content}
						{#if stepDialogOpen}
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
						{:else}
							<div class="bg-base-200 aspect-video w-full rounded-md" aria-hidden="true"></div>
						{/if}
					{/if}
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
