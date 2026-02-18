<script lang="ts">
	import {
		AlertTriangle,
		ChevronDown,
		ChevronUp,
		Maximize,
		Minimize,
		RefreshCw,
		Search,
		X
	} from 'lucide-svelte';
	import { fade } from 'svelte/transition';
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		messages: string[];
		error?: string;
		refreshing?: boolean;
		onRefresh?: () => void;
		onClear?: () => void;
		title?: string;
		showRefresh?: boolean;
	}

	const {
		messages = $bindable([]),
		error,
		refreshing = false,
		onRefresh,
		onClear,
		title = 'Deployment Logs',
		showRefresh = true
	}: Props = $props();

	let logsContainer: HTMLDivElement;
	let modalContainer: HTMLDivElement;
	let isMaximized = $state(false);
	let query = $state('');
	let userScrolledUp = $state(false);
	let searchInput = $state<HTMLInputElement>();
	let currentMatchIndex = $state(0);

	// Find all matching indices
	let matchingIndices = $derived.by(() => {
		if (!query) return [];
		return messages
			.map((msg, idx) => (msg.toLowerCase().includes(query.toLowerCase()) ? idx : -1))
			.filter((idx) => idx !== -1);
	});
	let matchingIndexSet = $derived(new Set(matchingIndices));

	const hasMessages = $derived(messages.length > 0);
	const hasMatches = $derived(matchingIndices.length > 0);

	$effect(() => {
		if (!messages.length) return;

		// Auto-scroll to bottom when new messages arrive, unless user scrolled up
		if (logsContainer && !userScrolledUp) {
			setTimeout(() => {
				if (!userScrolledUp) scrollToBottom(logsContainer);
			}, 50);
		}
	});

	// Reset/clamp current match index when query or matches update, then scroll to current match
	$effect(() => {
		// No query: reset index and do not scroll
		if (!query) {
			currentMatchIndex = 0;
			return;
		}

		// No matches: reset index and do not scroll
		if (!matchingIndices.length) {
			currentMatchIndex = 0;
			return;
		}

		// Clamp currentMatchIndex into valid range
		setTimeout(
			() => scrollToMatch(Math.max(0, Math.min(currentMatchIndex, matchingIndices.length - 1))),
			100
		);
	});

	function isScrolledToBottom(element: HTMLElement): boolean {
		return Math.abs(element.scrollHeight - element.clientHeight - element.scrollTop) < 10;
	}

	function scrollToBottom(element: HTMLElement) {
		element.scrollTop = element.scrollHeight;
	}

	function handleUserScroll() {
		if (logsContainer) {
			userScrolledUp = !isScrolledToBottom(logsContainer);
		}
	}

	function clearLogs() {
		// Clear logs and reset UI state
		query = ''; // Also clear search
		userScrolledUp = false; // Reset scroll tracking

		// Call parent callback if provided
		onClear?.();
	}

	function handleKeydown(e: KeyboardEvent) {
		if (e.key === 'Escape' && isMaximized) {
			isMaximized = false;
		} else if ((e.ctrlKey || e.metaKey) && e.key === 'f' && hasMessages) {
			// Only override browser find when logs are maximized or the event originates within the logs.
			let eventInsideLogs = false;

			if (logsContainer) {
				const target = e.target as Node | null;

				// Prefer composedPath when available to correctly handle shadow DOM and nested components.
				const path = typeof e.composedPath === 'function' ? e.composedPath() : null;
				if (path) {
					eventInsideLogs = path.includes(logsContainer);
				} else if (target) {
					eventInsideLogs = logsContainer.contains(target);
				}
			}

			// If the logs are not maximized and the event did not originate from within them,
			// let the browser handle Ctrl/Cmd+F normally.
			if (!isMaximized && !eventInsideLogs) {
				return;
			}

			e.preventDefault();
			searchInput?.focus();
		}
	}

	function handleSearchKeydown(e: KeyboardEvent) {
		if (e.key === 'Enter' && query) {
			e.preventDefault();
			if (e.shiftKey) {
				navigateToPreviousMatch();
			} else {
				navigateToNextMatch();
			}
		} else if (e.key === 'ArrowDown' && query) {
			e.preventDefault();
			navigateToNextMatch();
		} else if (e.key === 'ArrowUp' && query) {
			e.preventDefault();
			navigateToPreviousMatch();
		}
	}

	function handleModalClick(e: MouseEvent) {
		if (e.target === modalContainer) {
			isMaximized = false;
		}
	}

	function escapeHtml(text: string): string {
		return text
			.replace(/&/g, '&amp;')
			.replace(/</g, '&lt;')
			.replace(/>/g, '&gt;')
			.replace(/"/g, '&quot;')
			.replace(/'/g, '&#39;');
	}

	function highlightText(text: string, search: string): string {
		if (!search) return escapeHtml(text);
		const regex = new RegExp(`(${search.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')})`, 'gi');
		const parts = text.split(regex);

		return parts
			.map((part, index) =>
				index % 2 === 1
					? `<mark class="bg-yellow-300 dark:bg-yellow-600">${escapeHtml(part)}</mark>`
					: escapeHtml(part)
			)
			.join('');
	}

	function isMatch(index: number): boolean {
		return matchingIndexSet.has(index);
	}

	function isCurrentMatch(index: number): boolean {
		return !!query && hasMatches && matchingIndices[currentMatchIndex] === index;
	}

	function navigateToNextMatch() {
		if (!hasMatches) return;
		currentMatchIndex = (currentMatchIndex + 1) % matchingIndices.length;
		scrollToMatch(currentMatchIndex);
	}

	function navigateToPreviousMatch() {
		if (!hasMatches) return;
		currentMatchIndex = (currentMatchIndex - 1 + matchingIndices.length) % matchingIndices.length;
		scrollToMatch(currentMatchIndex);
	}

	function scrollToMatch(matchIdx: number) {
		if (!hasMatches || !logsContainer) return;
		const messageIndex = matchingIndices[matchIdx];
		const element = logsContainer.querySelector(`[data-message-index="${messageIndex}"]`);
		if (element) {
			element.scrollIntoView({ behavior: 'smooth', block: 'center' });
		}
	}

	export function scroll() {
		// Respect the userScrolledUp state to avoid disrupting the user's reading position
		if (logsContainer && !userScrolledUp) {
			scrollToBottom(logsContainer);
		}
	}
</script>

<svelte:window onkeydown={handleKeydown} />

<div>
	<div class="mb-2 flex items-center gap-2">
		<h2 class="text-lg font-semibold">{title}</h2>
		{#if showRefresh && onRefresh}
			<button
				onclick={onRefresh}
				use:tooltip={'Refresh logs'}
				class="text-on-surface1/60 hover:bg-surface2 hover:text-on-surface1 rounded-md p-1 disabled:opacity-50"
				disabled={refreshing}
				aria-label="Refresh logs"
			>
				<RefreshCw class="size-4 {refreshing ? 'animate-spin' : ''}" />
			</button>
		{/if}
		{#if error}
			<div
				use:tooltip={`An error occurred in connecting to the event stream. This is normal if the server is still starting up.`}
			>
				<AlertTriangle class="size-4 text-yellow-500" />
			</div>
		{/if}

		<div class="ml-auto flex items-center gap-1">
			<button
				onclick={clearLogs}
				use:tooltip={'Clear logs'}
				class="text-on-surface1/60 hover:bg-surface2 hover:text-on-surface1 rounded-md p-1 disabled:opacity-50"
				disabled={!hasMessages || refreshing}
				aria-label="Clear logs"
			>
				<X class="size-4" />
			</button>

			<button
				onclick={() => {
					isMaximized = true;
				}}
				use:tooltip={'Maximize (Esc to close)'}
				class="text-on-surface1/60 hover:bg-surface2 hover:text-on-surface1 rounded-md p-1 disabled:opacity-50"
				disabled={!hasMessages}
				aria-label="Maximize logs"
			>
				<Maximize class="size-4" />
			</button>
		</div>
	</div>

	<div
		bind:this={modalContainer}
		onclick={handleModalClick}
		class={twMerge(
			isMaximized
				? 'bg-background/50 fixed inset-0 z-50 flex flex-col items-center justify-center p-4 md:p-8 lg:p-10'
				: 'contents'
		)}
		role={isMaximized ? 'dialog' : undefined}
		aria-modal={isMaximized ? 'true' : undefined}
		aria-label={isMaximized ? title : undefined}
	>
		<div
			onscroll={handleUserScroll}
			bind:this={logsContainer}
			class={twMerge(
				'dark:bg-surface1 dark:border-surface3 default-scrollbar-thin bg-background flex min-h-64 flex-col overflow-y-auto rounded-lg border border-transparent shadow-sm',
				isMaximized ? 'h-full max-h-full w-full' : 'max-h-84 '
			)}
		>
			{#if hasMessages}
				<div class="dark:bg-surface1 bg-background border-surface3 sticky top-0 z-10 border-b p-4">
					<div
						class={twMerge(
							'border-surface2 bg-surface1/50 focus-within:outline-primary flex h-10 w-full items-center gap-2 rounded-sm border pr-2 pl-2 text-xs focus-within:outline-2',
							isMaximized && 'text-md h-12'
						)}
					>
						<div class="flex h-full max-h-8 items-center py-1.5">
							<Search class="h-full opacity-30" />
						</div>

						<input
							bind:this={searchInput}
							class="placeholder:text-on-surface1/40 flex-1 bg-transparent py-3 outline-none"
							type="text"
							placeholder="Search logs... (Ctrl/Cmd+F)"
							bind:value={query}
							onkeydown={handleSearchKeydown}
							aria-label="Search logs"
						/>

						<div class="flex h-full items-center gap-1 p-0.5">
							{#if query}
								<span class="text-on-surface1/60 text-xs">
									{#if hasMatches}
										{currentMatchIndex + 1} / {matchingIndices.length}
									{:else}
										0 / 0
									{/if}
								</span>
								<button
									class="hover:bg-surface2/80 active:bg-surface2/100 flex h-full max-h-8 items-center justify-center rounded-md p-1.5 opacity-30 hover:opacity-60 disabled:opacity-20"
									onclick={navigateToPreviousMatch}
									disabled={!hasMatches}
									use:tooltip={'Previous match (↑ or Shift+Enter)'}
									aria-label="Previous match"
								>
									<ChevronUp class="size-full text-current" />
								</button>
								<button
									class="hover:bg-surface2/80 active:bg-surface2/100 flex h-full max-h-8 items-center justify-center rounded-md p-1.5 opacity-30 hover:opacity-60 disabled:opacity-20"
									onclick={navigateToNextMatch}
									disabled={!hasMatches}
									use:tooltip={'Next match (↓ or Enter)'}
									aria-label="Next match"
								>
									<ChevronDown class="size-full text-current" />
								</button>
								<button
									class="hover:bg-surface2/80 active:bg-surface2/100 flex h-full max-h-8 items-center justify-center rounded-md p-1.5 opacity-30 hover:opacity-60"
									onclick={() => {
										query = '';
									}}
									aria-label="Clear search"
								>
									<X class="size-full text-current" />
								</button>
							{/if}

							{#if isMaximized}
								<button
									class="hover:bg-surface2/80 active:bg-surface2/100 flex h-full max-h-8 items-center justify-center rounded-md p-1.5 opacity-30 hover:opacity-60"
									onclick={() => {
										isMaximized = false;
									}}
									use:tooltip={'Close (Esc)'}
									aria-label="Close maximized view"
								>
									<Minimize class="size-full text-current" />
								</button>
							{/if}
						</div>
					</div>
				</div>
			{/if}

			{#if hasMessages}
				<div class="space-y-1 p-4">
					{#each messages as message, i (i)}
						{@const isMatchingLine = query ? isMatch(i) : true}
						{@const isCurrentMatchLine = isCurrentMatch(i)}
						<div
							data-message-index={i}
							class={twMerge(
								'group grid gap-2 rounded px-2 py-1 font-mono text-sm transition-all',
								isMaximized && 'text-base',
								!isMatchingLine && 'opacity-50',
								isMatchingLine && 'hover:bg-surface2',
								isCurrentMatchLine && 'outline-primary outline-2 outline-offset-2'
							)}
							style="grid-template-columns: auto 1fr;"
							in:fade
						>
							<div class="border-surface3 border-r pr-1 text-right">
								<span class="text-on-surface1/40 select-none">{i + 1}</span>
							</div>
							<span class="text-on-surface1 flex-1">
								{@html highlightText(message, query)}
							</span>
						</div>
					{/each}
				</div>
			{:else}
				<div class="flex w-full flex-1 items-center justify-center p-6">
					<div class="text-center">
						<div class="text-on-surface1/80 font-medium">No deployment logs.</div>
						<p class="text-on-surface1/60 mt-1 text-sm">Try refreshing the logs.</p>
					</div>
				</div>
			{/if}
		</div>
	</div>
</div>
