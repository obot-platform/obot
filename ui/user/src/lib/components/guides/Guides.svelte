<script lang="ts">
	import { afterNavigate } from '$app/navigation';
	import { page } from '$app/state';
	import {
		generateLessonItems,
		getGuideSeen,
		getLessonsCompleted,
		resetGuide,
		setGuideSeen
	} from '$lib/services/guides/utils';
	import { guide, profile, userDeviceSettings, version } from '$lib/stores';
	import { adminConfigStore } from '$lib/stores/adminConfig.svelte';
	import IconButton from '../primitives/IconButton.svelte';
	import Obot from './Obot.svelte';
	import { createGuideHighlighter, type GuideHighlighter } from './highlight';
	import { ChevronRight, Circle, CircleCheck, Info, X } from '@lucide/svelte';
	import { isAfter } from 'date-fns';
	import { onMount } from 'svelte';
	import { fly } from 'svelte/transition';
	import { twMerge } from 'tailwind-merge';

	let lessonsCompleted = $state(getLessonsCompleted());
	let showLessons = $state(false);
	let hasSeenGuides = $state(false);

	const lessonItems = $derived(generateLessonItems(lessonsCompleted));
	const isAdminRoute = $derived(page.url.pathname.startsWith('/admin'));

	let highlighter: GuideHighlighter | undefined;
	let listenerHandler: ((e: MouseEvent) => void) | undefined;
	let rafId: number | undefined;

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
		lessonsCompleted = getLessonsCompleted();

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
	{@const incompleteLessonsCount = lessonItems.filter((lesson) => !lesson.completed).length}
	<div class="fixed bottom-4 right-4 z-50 flex flex-col gap-4 items-end">
		{#if showLessons}
			<div
				class="max-w-lg relative"
				in:fly={{ y: 100, duration: 150 }}
				out:fly={{ y: 100, duration: 150 }}
			>
				{#if guide.showObotInGuide}
					<Obot animation={['enter', 'idle']} class="absolute -top-23.5 left-0" />
				{/if}
				<IconButton
					onclick={() => {
						showLessons = false;
					}}
					class="btn-sm btn-primary absolute top-4 right-4 z-10"
				>
					<X class="size-5 text-primary-content" />
				</IconButton>
				{@render lessons(true)}
			</div>
		{/if}
		<div class="flex bg-primary rounded-full text-primary-content text-sm items-center gap-2">
			<button
				class="shrink-0 flex items-center gap-2 w-fit py-3 pl-4"
				onclick={() => (showLessons = !showLessons)}
				id="btn-get-started-guide"
			>
				<Info class="size-5" /> <span class="font-medium">Get Started</span>
				{#if incompleteLessonsCount > 0}
					<div class="badge badge-xs">{incompleteLessonsCount}</div>
				{/if}
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

{#snippet lessons(altTheme?: boolean)}
	<div class={twMerge('paper gap-0', altTheme ? 'bg-primary text-primary-content' : '')}>
		<h4
			class={twMerge(
				'font-semibold border-b-2 pb-3 mb-3',
				altTheme ? 'border-b-primary-content' : 'border-b-primary'
			)}
		>
			Quick Start Guides
		</h4>
		<div class="timeline timeline-compact timeline-vertical text-primary-content">
			{#each lessonItems as lessonItem, i (lessonItem.label)}
				<li>
					{#if i > 0}
						<hr class="bg-primary-content" />
					{/if}
					<div class="timeline-middle">
						{#if lessonItem.completed}
							<CircleCheck class={altTheme ? 'text-primary-content' : 'text-primary'} />
						{:else}
							<Circle class="text-primary-content" />
						{/if}
					</div>
					<button
						class={twMerge(
							'translate-x-2 timeline-end flex items-center justify-between gap-4 pl-3 pr-1 py-2 text-left rounded-md',
							altTheme ? 'hover:bg-primary-content/10' : 'hover:bg-base-200'
						)}
						onclick={() => {
							guide.selectedGuide = lessonItem.guide;
							showLessons = false;
						}}
					>
						<div class="flex flex-col gap-1 grow">
							<p class="text-sm font-semibold">{lessonItem.label}</p>
							<p
								class={twMerge(
									'text-xs font-light',
									altTheme ? 'text-primary-content/50' : 'text-muted-content'
								)}
							>
								{lessonItem.description}
							</p>
						</div>
						<ChevronRight
							class={twMerge(
								'size-4 shrink-0',
								altTheme ? 'text-primary-content' : 'text-muted-content'
							)}
						/>
					</button>
					{#if i < lessonItems.length - 1}
						<hr class="bg-primary-content" />
					{/if}
				</li>
			{/each}
		</div>
	</div>
{/snippet}
