<script lang="ts">
	import type { Chat } from '$lib/services/nanobot/types';
	import { goto } from '$lib/url';
	import { Check, Edit, MoreVertical, Trash2, X, Plus } from 'lucide-svelte';

	interface Props {
		threads: Chat[];
		onRename: (threadId: string, newTitle: string) => void;
		onDelete: (threadId: string) => void;
		isLoading?: boolean;
		onThreadClick?: () => void;
		projectId: string;
	}

	let {
		threads,
		onRename,
		onDelete,
		isLoading = false,
		onThreadClick,
		projectId
	}: Props = $props();

	let editingThreadId = $state<string | null>(null);
	let editTitle = $state('');

	function navigateToThread(threadId: string) {
		onThreadClick?.();
		goto(`/nanobot/p/${projectId}?tid=${threadId}`);
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

	function startRename(threadId: string, currentTitle: string) {
		editingThreadId = threadId;
		editTitle = currentTitle || '';
	}

	function saveRename() {
		if (editingThreadId && editTitle.trim()) {
			onRename(editingThreadId, editTitle.trim());
			editingThreadId = null;
			editTitle = '';
		}
	}

	function cancelRename() {
		editingThreadId = null;
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
</script>

<div class="flex h-full flex-col">
	<!-- Header -->
	<div class="flex flex-shrink-0 items-center justify-between gap-2 p-2">
		<h2 class="text-base-content/60 font-semibold">Conversations</h2>
		<button
			class="btn btn-square btn-ghost btn-sm tooltip tooltip-left"
			data-tip="Start New Conversation"
			onclick={() => {
				goto(`/nanobot`);
			}}
		>
			<Plus class="size-4" />
		</button>
	</div>

	<!-- Thread list -->
	<div class="flex-1">
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
			{#each threads as thread (thread.id)}
				<div class="group border-base-200 hover:bg-base-100 flex items-center border-b">
					<!-- Thread title area (clickable) -->
					<button
						class="flex-1 truncate p-3 text-left transition-colors focus:outline-none"
						onclick={() => {
							if (editingThreadId === thread.id) return;
							navigateToThread(thread.id);
						}}
					>
						<div class="flex items-center justify-between gap-2">
							<div class="flex min-w-0 flex-1 items-center gap-2">
								{#if editingThreadId === thread.id}
									<input
										type="text"
										bind:value={editTitle}
										onkeydown={handleKeydown}
										class="input input-sm min-w-0 flex-1"
										onclick={(e) => e.stopPropagation()}
										onfocus={(e) => (e.target as HTMLInputElement).select()}
									/>
								{:else}
									<h3 class="truncate text-sm font-medium">{thread.title || 'Untitled'}</h3>
								{/if}
							</div>
							{#if editingThreadId !== thread.id}
								<span class="text-base-content/50 flex-shrink-0 text-xs">
									{formatTime(thread.created)}
								</span>
							{/if}
						</div>
					</button>

					<!-- Save/Cancel buttons for editing -->
					{#if editingThreadId === thread.id}
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

					{#if editingThreadId !== thread.id}
						<!-- Dropdown menu - only show on hover -->
						<div class="dropdown dropdown-end opacity-0 transition-opacity group-hover:opacity-100">
							<div tabindex="0" role="button" class="btn btn-square btn-ghost btn-sm">
								<MoreVertical class="h-4 w-4" />
							</div>
							<ul class="dropdown-content menu dropdown-menu z-[1] w-32">
								<li>
									<button onclick={() => startRename(thread.id, thread.title)} class="text-sm">
										<Edit class="h-4 w-4" />
										Rename
									</button>
								</li>
								<li>
									<button onclick={() => handleDelete(thread.id)} class="text-error text-sm">
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
