<script lang="ts">
	import Loading from '$lib/icons/Loading.svelte';
	import { UserService, type OrgGroup } from '$lib/services';
	import Search from '../Search.svelte';
	import CursorPagination from '../table/CursorPagination.svelte';
	import { Check, TriangleAlert } from '@lucide/svelte';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		onSelect: (group: OrgGroup) => void;
		selectedId?: string;
		excludeIds?: string[];
		subtitle?: (group: OrgGroup) => string | undefined;
		pageSize?: number;
		placeholder?: string;
	}

	let {
		onSelect,
		selectedId,
		excludeIds,
		subtitle,
		pageSize = 50,
		placeholder = 'Search groups...'
	}: Props = $props();

	let query = $state('');
	let pageIndex = $state(0);
	// One cursor per visited page: the first page has none, and every Next pushes the cursor that
	// reaches the following page. Going back needs the stack rather than a single saved cursor,
	// because a cursor only ever points forwards.
	let cursorStack = $state<(string | undefined)[]>([undefined]);
	let nextCursor = $state<string | undefined>(undefined);
	let groups = $state<OrgGroup[]>([]);
	let degraded = $state(false);
	let loading = $state(false);
	let errored = $state(false);

	// A directory can hold tens of thousands of groups, so searching and paging both happen on the
	// server. Only the current page is ever held here.
	let inFlight: AbortController | undefined;

	let skipNextLoad = false;

	async function load() {
		inFlight?.abort();
		const controller = new AbortController();
		inFlight = controller;

		loading = true;
		errored = false;
		try {
			const page = await UserService.listGroups({
				query,
				limit: pageSize,
				cursor: cursorStack[pageIndex],
				signal: controller.signal
			});
			if (controller.signal.aborted) return;

			if (page.reset) {
				cursorStack = [undefined];
				if (pageIndex !== 0) {
					skipNextLoad = true;
					pageIndex = 0;
				}
			}

			groups = page.items;
			nextCursor = page.nextCursor;
			degraded = page.degraded;
		} catch (error) {
			if (controller.signal.aborted) return;
			console.error('Failed to load groups:', error);
			groups = [];
			nextCursor = undefined;
			degraded = false;
			errored = true;
		} finally {
			if (!controller.signal.aborted) {
				loading = false;
			}
		}
	}

	$effect(() => {
		// Re-runs whenever the query or page changes.
		void query;
		void pageIndex;

		if (skipNextLoad) {
			skipNextLoad = false;
			return;
		}

		load();
		return () => inFlight?.abort();
	});

	const excluded = $derived(new Set(excludeIds ?? []));
	const visibleGroups = $derived(groups.filter((group) => !excluded.has(group.id)));

	function handleSearch(value: string) {
		query = value;
		// A cursor belongs to the search it was created for, so a new search starts over.
		cursorStack = [undefined];
		pageIndex = 0;
	}

	function goToNextPage() {
		cursorStack = [...cursorStack.slice(0, pageIndex + 1), nextCursor];
		pageIndex += 1;
	}

	export function refresh() {
		load();
	}
</script>

<div class="flex h-full flex-col gap-4 overflow-hidden">
	{#if degraded}
		<div
			class="flex items-start gap-2 rounded-lg border border-amber-500/40 bg-amber-500/10 p-3 text-xs"
			role="status"
		>
			<TriangleAlert class="mt-0.5 size-4 shrink-0 text-amber-600 dark:text-amber-400" />
			<span>
				Showing only groups Obot has already recorded. Obot could not list your identity provider's
				directory &mdash; directory-wide group read permission may not be granted.
			</span>
		</div>
	{/if}

	<div class="shrink-0">
		<Search
			class="dark:bg-base-200 dark:border-base-400 shadow-inner dark:border"
			value={query}
			onChange={handleSearch}
			{placeholder}
		/>
	</div>

	<div class="default-scrollbar-thin flex grow flex-col overflow-y-auto">
		{#if loading}
			<div class="flex grow items-center justify-center py-8">
				<Loading class="size-6" />
			</div>
		{:else if errored}
			<p class="text-muted-content py-8 text-center text-sm">Failed to load groups. Try again.</p>
		{:else if visibleGroups.length === 0}
			<p class="text-muted-content py-8 text-center text-sm">
				{query ? 'No groups found matching your search.' : 'No groups available.'}
			</p>
		{:else}
			<div class="flex flex-col gap-2">
				{#each visibleGroups as group (group.id)}
					{@const groupSubtitle = subtitle?.(group)}
					<button
						class={twMerge(
							'border-base-400 hover:bg-base-100/5 flex items-center gap-3 rounded-lg border p-3 text-left transition-colors',
							selectedId === group.id && 'bg-primary/10 border-primary'
						)}
						onclick={() => onSelect(group)}
					>
						<div class="flex grow flex-col">
							<span class="font-medium">{group.name}</span>
							{#if groupSubtitle}
								<span class="text-muted-content text-xs">{groupSubtitle}</span>
							{/if}
						</div>
						{#if selectedId === group.id}
							<Check class="text-primary size-5 shrink-0" />
						{/if}
					</button>
				{/each}
			</div>
		{/if}
	</div>

	{#if pageIndex > 0 || nextCursor}
		<div class="shrink-0">
			<CursorPagination
				{pageIndex}
				hasPrevious={pageIndex > 0}
				hasNext={Boolean(nextCursor)}
				{loading}
				onPrevious={() => (pageIndex -= 1)}
				onNext={goToNextPage}
			/>
		</div>
	{/if}
</div>
