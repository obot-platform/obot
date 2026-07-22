<script lang="ts">
	import { afterNavigate } from '$app/navigation';
	import { page } from '$app/state';
	import {
		generateLessonItems,
		getGuideSeen,
		resetGuide,
		setGuideSeen
	} from '$lib/services/guides/utils';
	import { guide, profile, userDeviceSettings, version } from '$lib/stores';
	import { adminConfigStore } from '$lib/stores/adminConfig.svelte';
	import IconButton from '../primitives/IconButton.svelte';
	import Obot from './Obot.svelte';
	import { createGuideHighlighter, type GuideHighlighter } from './highlight';
	import { ChevronRight, Info, X } from '@lucide/svelte';
	import { isAfter } from 'date-fns';
	import { onMount } from 'svelte';
	import { fly } from 'svelte/transition';
	import { twMerge } from 'tailwind-merge';

	let highlighter: GuideHighlighter | undefined;
	let listenerHandler: ((e: MouseEvent) => void) | undefined;
	let rafId: number | undefined;

	let showLessons = $state(false);
	let hasSeenGuides = $state(false);
	let viewportHeight = $state(typeof window !== 'undefined' ? window.innerHeight : 900);

	const lessonItems = $derived(generateLessonItems());
	const isAdminRoute = $derived(page.url.pathname.startsWith('/admin'));

	const visibleLessonItems = $derived.by(() => {
		// bottom-4 + pill + gap-4 + panel header/padding + Obot overhang + top margin
		const reserved = 210;
		const available = Math.max(0, viewportHeight - reserved);
		const itemHeight = 84;
		const maxVisible = Math.max(1, Math.floor(available / itemHeight));
		return lessonItems.slice(0, Math.min(maxVisible, lessonItems.length));
	});

	const canShowGuide = $derived.by(() => {
		if (!userDeviceSettings.showAllGuides) return false;

		// avoid showing guide on edit pages route (for admin routes: depth 4, power user plus routes: depth 3)
		// also avoid on routes with create new param (?new=true)
		const depthLimit = isAdminRoute ? 4 : 3;
		const hasCreateNewParam = page.url.searchParams.get('new') === 'true';
		if (page.url.pathname.split('/').length >= depthLimit || hasCreateNewParam) return false;

		// avoid showing when initial required configuration is incomplete
		if (profile.current?.hasAdminAccess?.() && isAdminRoute) {
			const isAuthProviderConfigured = version.current.authEnabled
				? $adminConfigStore.authProviderConfigured
				: true;
			const requiresModelProviderConfiguration =
				version.current.agentsEnabled !== false && !$adminConfigStore.modelProviderConfigured;

			if (!isAuthProviderConfigured || requiresModelProviderConfiguration) {
				return false;
			}
		}
		return true;
	});

	function initGuide() {
		const seenGuideDate = getGuideSeen();
		if (
			seenGuideDate &&
			profile.current?.created &&
			isAfter(new Date(profile.current.created), seenGuideDate)
		) {
			resetGuide();
		} else {
			hasSeenGuides = Boolean(seenGuideDate);
		}
	}

	onMount(() => {
		initGuide();
		const onResize = () => {
			viewportHeight = window.innerHeight;
		};
		onResize();
		window.addEventListener('resize', onResize);
		return () => window.removeEventListener('resize', onResize);
	});

	afterNavigate(() => {
		initGuide();
	});

	function cleanup() {
		if (rafId) {
			cancelAnimationFrame(rafId);
			rafId = undefined;
		}
		highlighter?.destroy();
		highlighter = undefined;
		if (listenerHandler) {
			window.removeEventListener('click', listenerHandler, true);
			listenerHandler = undefined;
		}
	}

	$effect(() => {
		if (!canShowGuide || hasSeenGuides) return;

		listenerHandler = (e: MouseEvent) => {
			let el: Element | null = e.target instanceof Element ? e.target : null;
			while (el) {
				if (el.id === 'btn-get-started-guide') {
					handleClose();
					return;
				}
				el = el.parentElement;
			}
		};

		function handleClose() {
			setGuideSeen();
			hasSeenGuides = true;
			highlighter?.destroy();
			if (listenerHandler) {
				window.removeEventListener('click', listenerHandler, true);
			}
		}

		highlighter = createGuideHighlighter({
			allowClose: true,
			onCloseClick: handleClose,
			overlayClickBehavior: handleClose,
			onObotVisibilityChange: (visible) => {
				guide.showObotInGuide = visible;
			}
		});

		rafId = requestAnimationFrame(() => {
			highlighter?.highlight({
				selector: { id: 'btn-get-started-guide' },
				title: 'First Time Here?',
				description: 'Check out our quick start guides to get you up and running quickly.',
				side: 'left',
				align: 'start'
			});

			if (listenerHandler) {
				window.addEventListener('click', listenerHandler, true);
			}
		});

		return () => cleanup();
	});

	function handleCloseGuides() {
		userDeviceSettings.setShowAllGuides(false);
		cleanup();
	}

	function handleConfirmCloseGuides() {
		cleanup();
		listenerHandler = (e: MouseEvent) => {
			let el: Element | null = e.target instanceof Element ? e.target : null;
			while (el) {
				if (el.id === 'btn-navbar-profile') {
					handleCloseGuides();
					return;
				}
				el = el.parentElement;
			}
		};

		highlighter = createGuideHighlighter({
			allowClose: true,
			onCloseClick: handleCloseGuides,
			overlayClickBehavior: handleCloseGuides,
			onObotVisibilityChange: (visible) => {
				guide.showObotInGuide = visible;
			}
		});

		rafId = requestAnimationFrame(() => {
			highlighter?.highlight({
				selector: { id: 'btn-navbar-profile' },
				title: 'Access Guides Again',
				side: 'left',
				description:
					'If at a later point in time you want to access the guides again, you can do so by clicking the profile button here and go to My Account.'
			});

			if (listenerHandler) {
				window.addEventListener('click', listenerHandler, true);
			}
		});
	}
</script>

{#if !guide.selectedGuide && canShowGuide}
	<div class="fixed bottom-4 right-4 z-50 flex flex-col gap-4 items-end">
		{#if showLessons}
			<div
				class="max-w-full md:max-w-md relative"
				in:fly={{ y: 100, duration: 150 }}
				out:fly={{ y: 100, duration: 150 }}
			>
				{#if guide.showObotInGuide}
					<Obot animation={['enter', 'idle']} class="absolute -top-15.5 left-0" />
				{/if}
				<IconButton
					onclick={() => {
						showLessons = false;
					}}
					class="btn-sm btn-primary absolute top-2 right-2 z-10"
				>
					<X class="size-5 text-primary-content" />
				</IconButton>
				{@render lessons()}
			</div>
		{/if}
		<div
			id="btn-get-started-guide"
			class="flex items-center gap-2 rounded-full bg-primary text-primary-content text-sm shadow-lg shadow-black/40 outline-2 outline-offset-0 outline-base-100/80 dark:outline-base-300/80"
		>
			<button
				class="shrink-0 flex items-center gap-2 w-fit py-3 pl-4"
				onclick={() => (showLessons = !showLessons)}
			>
				<Info class="size-5" /> <span class="font-medium">Get Started</span>
			</button>
			<IconButton
				tooltip={{ text: 'Close guides' }}
				onclick={handleConfirmCloseGuides}
				class="mr-4 shrink-0 btn btn-xs btn-circle btn-ghost text-primary-content/50 hover:bg-primary-content/10 hover:text-primary-content hover:border-0 border-0"
			>
				<X class="size-4" />
			</IconButton>
		</div>
	</div>
{/if}

{#snippet lessons()}
	<div class="paper gap-0 p-3 bg-primary text-primary-content">
		<h4 class="font-semibold border-b-2 pb-3 mb-3 text-sm border-b-primary-content">
			Quick Start Guides
		</h4>
		<div class="flex flex-col">
			{#each visibleLessonItems as lessonItem (lessonItem.label)}
				{@const isUnderConstruction = lessonItem.guide === undefined}
				<button
					class={twMerge(
						'flex items-center justify-between gap-4 pl-3 pr-1 py-2 text-left rounded-md w-full transition-colors',
						isUnderConstruction ? 'opacity-50 cursor-default' : 'hover:bg-primary-content/10'
					)}
					onclick={() => {
						guide.selectedGuide = lessonItem.guide;
						showLessons = false;
					}}
					disabled={isUnderConstruction}
					aria-disabled={isUnderConstruction}
				>
					<div class="flex flex-col gap-0.5 grow">
						<p class="text-sm font-semibold flex items-center gap-2">
							{lessonItem.label}
							{#if isUnderConstruction}
								<span class="shrink-0 font-light badge badge-xs badge-outline"> Coming soon</span>
							{/if}
						</p>
						<p class="text-xs font-light text-primary-content/50">
							{lessonItem.description}
						</p>
					</div>
					<ChevronRight class="size-4 shrink-0 text-primary-content" />
				</button>
			{/each}
		</div>
	</div>
{/snippet}
