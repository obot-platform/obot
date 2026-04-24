<script lang="ts">
	import type { Chat } from '$lib/services/nanobot/types';
	import { parseJSON } from '$lib/services/nanobot/utils';
	import { responsive } from '$lib/stores';
	import { nanobotChat } from '$lib/stores/nanobotChat.svelte';
	import { isRecent } from '$lib/time';
	import { goto } from '$lib/url';
	import { Check, Edit, EllipsisVertical, Trash2, X, Plus } from 'lucide-svelte';
	import { tick } from 'svelte';
	import { get } from 'svelte/store';
	import { fly } from 'svelte/transition';

	interface Props {
		sessions: Chat[];
		onRename: (sessionId: string, newTitle: string) => void;
		onDelete: (sessionId: string) => void;
		isLoading?: boolean;
		onSessionClick?: () => void;
		onCreateSession?: () => void;
		selectedSessionId?: string;
	}

	let {
		sessions,
		onRename,
		onDelete,
		isLoading = false,
		onSessionClick,
		onCreateSession,
		selectedSessionId
	}: Props = $props();

	let editingSessionId = $state<string | null>(null);
	let editTitle = $state('');
	let renameInputEl = $state<HTMLInputElement | null>(null);
	let sessionsToShow = $derived.by(() => {
		// eslint-disable-next-line svelte/prefer-svelte-reactivity
		const sessionIdsToFilter = new Set<string>();
		for (let i = 0; i < localStorage.length; i++) {
			const key = localStorage.key(i);
			if (key?.startsWith('mcp-session-')) {
				const parsed = parseJSON<{ sessionId: string }>(localStorage.getItem(key) ?? '');
				if (parsed?.sessionId) sessionIdsToFilter.add(parsed.sessionId);
			}
		}
		return sessions.filter(
			(t) => !sessionIdsToFilter.has(t.id) && t.id !== $nanobotChat?.api?.sessionId
		);
	});

	function navigateToSession(sessionId: string) {
		const storedChat = get(nanobotChat);
		onSessionClick?.();
		goto(`/agent/p/${storedChat?.projectId}?tid=${sessionId}`);
	}

	function formatTime(timestamp: string): string {
		const now = new Date();
		const diff = now.getTime() - new Date(timestamp).getTime();
		const minutes = Math.floor(diff / (1000 * 60));
		const hours = Math.floor(diff / (1000 * 60 * 60));
		const days = Math.floor(diff / (1000 * 60 * 60 * 24));

		if (minutes < 1) return 'now';
		if (minutes < 60) return `${minutes}m`;
		if (hours < 24) return `${hours}h`;
		return `${days}d`;
	}

	async function startRename(sessionId: string, currentTitle: string) {
		editingSessionId = sessionId;
		editTitle = currentTitle || '';
		await tick();
		renameInputEl?.focus();
		renameInputEl?.select();
	}

	function saveRename() {
		if (editingSessionId && editTitle.trim()) {
			onRename(editingSessionId, editTitle.trim());
			editingSessionId = null;
			editTitle = '';
		}
	}

	function cancelRename() {
		editingSessionId = null;
		editTitle = '';
	}

	function handleDelete(threadId: string) {
		onDelete(threadId);
	}

	function handleKeydown(e: KeyboardEvent) {
		if (e.key === 'Enter') {
			saveRename();
		} else if (e.key === 'Escape') {
			cancelRename();
		}
	}

	const DROPDOWN_MENU_HEIGHT = 132;
	const DROPDOWN_TOP_BUFFER = 16;
	let listHost = $state<HTMLDivElement | null>(null);
	let indicesNeedingDropdownTop = $state<Set<number>>(new Set());

	function getScrollParent(el: HTMLElement | null): HTMLElement | null {
		if (!el?.parentElement) return null;
		const { overflowY } = getComputedStyle(el.parentElement);
		if (/(auto|scroll|overlay)/.test(overflowY)) return el.parentElement;
		return getScrollParent(el.parentElement);
	}

	function getVisibleBottom(scrollEl: HTMLElement): number {
		const rect = scrollEl.getBoundingClientRect();
		const style = getComputedStyle(scrollEl);
		const paddingBottom = parseFloat(style.paddingBottom) || 0;
		return rect.bottom - paddingBottom;
	}

	$effect(() => {
		const host = listHost;
		if (!host) return;
		void sessions.length;

		const scrollEl = getScrollParent(host) ?? host;

		const update = () => {
			const visibleBottom = getVisibleBottom(scrollEl);
			const rows = host.querySelectorAll<HTMLElement>('[data-thread-row]');
			// eslint-disable-next-line svelte/prefer-svelte-reactivity
			const next = new Set<number>();
			rows.forEach((row) => {
				const i = parseInt(row.dataset.threadRow ?? '-1', 10);
				if (i < 0) return;
				const rowRect = row.getBoundingClientRect();
				// Apply dropdown-top if opening below would be clipped (with buffer)
				if (rowRect.bottom + DROPDOWN_MENU_HEIGHT + DROPDOWN_TOP_BUFFER > visibleBottom) {
					next.add(i);
				}
			});
			if (
				next.size !== indicesNeedingDropdownTop.size ||
				[...next].some((n) => !indicesNeedingDropdownTop.has(n))
			) {
				indicesNeedingDropdownTop = next;
			}
		};

		const scheduleUpdate = () => requestAnimationFrame(update);
		requestAnimationFrame(() => requestAnimationFrame(update));

		update();
		scrollEl.addEventListener('scroll', scheduleUpdate);
		const ro = new ResizeObserver(scheduleUpdate);
		ro.observe(scrollEl);
		return () => {
			scrollEl.removeEventListener('scroll', scheduleUpdate);
			ro.disconnect();
		};
	});
</script>

<div class="flex w-full grow flex-col">
	{#if !responsive.isMobile}
		<!-- Header -->
		<div class="mb-2 flex flex-shrink-0 items-center justify-between gap-2 pr-3 pl-4">
			<h2 class="text-base-content/50 text-md font-semibold">Sessions</h2>
			<button
				class="btn btn-square btn-ghost btn-sm tooltip tooltip-left"
				data-tip="Start New Conversation"
				onclick={onCreateSession}
			>
				<Plus class="text-base-content/50 size-6" />
			</button>
		</div>
	{/if}

	<!-- Thread list (scroll container so header tooltips are not clipped by overflow) -->
	<div class="flex min-h-0 grow flex-col" bind:this={listHost}>
		{#if isLoading}
			<!-- Skeleton UI when loading -->
			{#each Array(5).fill(null) as _, index (index)}
				<div class="border-base-200 flex items-center border-b p-3">
					<div class="flex-1">
						<div class="flex items-center justify-between gap-2">
							<div class="flex min-w-0 flex-1 items-center gap-2">
								<div class="skeleton h-5 w-48"></div>
							</div>
							<div class="skeleton h-4 w-8"></div>
						</div>
					</div>
					<div class="w-8"></div>
					<!-- Space for the menu button -->
				</div>
			{/each}
		{:else}
			{#each sessionsToShow as session, index (session.id)}
				<div
					data-thread-row={index}
					class="group border-base-200 dark:hover:bg-base-100/25 hover:bg-base-100/65 flex items-center border-b"
					class:bg-base-100={selectedSessionId === session.id}
					in:fly={{ x: 100, duration: 150 }}
				>
					<!-- Thread title area (clickable) -->
					<button
						class="flex-1 truncate p-3 text-left transition-colors focus:outline-none"
						onclick={() => {
							if (editingSessionId === session.id) return;
							navigateToSession(session.id);
						}}
					>
						<div class="flex items-center justify-between gap-2">
							<div class="flex min-w-0 flex-1 items-center gap-2">
								{#if editingSessionId === session.id}
									<input
										bind:this={renameInputEl}
										type="text"
										bind:value={editTitle}
										onkeydown={handleKeydown}
										class="input input-sm min-w-0 flex-1"
										onclick={(e) => e.stopPropagation()}
										onfocus={(e) => (e.target as HTMLInputElement).select()}
									/>
								{:else if isRecent(session.created) && !session.title}
									<span class="skeleton skeleton-text text-fm font-medium">...</span>
								{:else}
									<h3 class="truncate text-sm font-medium">{session.title || 'Untitled'}</h3>
								{/if}
							</div>
							{#if editingSessionId !== session.id}
								<span class="text-base-content/50 flex-shrink-0 text-xs">
									{formatTime(session.created)}
								</span>
							{/if}
						</div>
					</button>

					<!-- Save/Cancel buttons for editing -->
					{#if editingSessionId === session.id}
						<div class="flex items-center gap-1 px-2">
							<button
								class="btn btn-ghost btn-xs"
								onclick={cancelRename}
								aria-label="Cancel editing"
							>
								<X class="h-3 w-3" />
							</button>
							<button
								class="btn text-success btn-ghost btn-xs hover:bg-success/20"
								onclick={saveRename}
								aria-label="Save changes"
							>
								<Check class="h-3 w-3" />
							</button>
						</div>
					{/if}

					{#if editingSessionId !== session.id}
						<!-- Dropdown menu - only show on hover -->
						<div
							class="dropdown dropdown-end mr-2 w-8 opacity-100 transition-[width,opacity] group-hover:w-8 group-hover:opacity-100 md:w-0 md:opacity-0"
							class:dropdown-top={indicesNeedingDropdownTop.has(index)}
						>
							<div tabindex="0" role="button" class="btn btn-square btn-ghost btn-sm">
								<EllipsisVertical class="h-4 w-4" />
							</div>
							<ul class="dropdown-content menu dropdown-menu z-[1] w-32">
								<li>
									<button onclick={() => startRename(session.id, session.title)} class="text-sm">
										<Edit class="h-4 w-4" />
										Rename
									</button>
								</li>
								<li>
									<button onclick={() => handleDelete(session.id)} class="text-error text-sm">
										<Trash2 class="h-4 w-4" />
										Delete
									</button>
								</li>
							</ul>
						</div>
					{/if}
				</div>
			{/each}
		{/if}
	</div>
</div>
