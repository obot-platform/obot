<script lang="ts">
	import { page } from '$app/state';
	import type { DateRange } from '$lib/components/Calendar.svelte';
	import Layout from '$lib/components/Layout.svelte';
	import Search from '$lib/components/Search.svelte';
	import Select from '$lib/components/Select.svelte';
	import AuditLogCalendar from '$lib/components/admin/audit-logs/AuditLogCalendar.svelte';
	import StackedTimeline from '$lib/components/graph/StackedTimeline.svelte';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants';
	import Loading from '$lib/icons/Loading.svelte';
	import {
		AdminService,
		type Model,
		type OrgUser,
		type TokenUsage,
		type TotalTokenUsage,
		type TokenUsageWithCategory
	} from '$lib/services';
	import { errors } from '$lib/stores';
	import { goto } from '$lib/url';
	import { getUserDisplayName } from '$lib/utils';
	import { aggregateTimelineDataByBucket, getUserLabels } from './utils';
	import { subDays } from 'date-fns';
	import { Coins, X } from 'lucide-svelte';
	import { onMount } from 'svelte';
	import { SvelteMap } from 'svelte/reactivity';
	import { fade } from 'svelte/transition';
	import { slide } from 'svelte/transition';
	import { twMerge } from 'tailwind-merge';

	let loadingTableData = $state(true);
	let loadingTotalTokensData = $state(true);
	let usersData = $state<OrgUser[]>([]);
	let modelsData = $state<Model[]>([]);

	let end = $derived(page.url.searchParams.get('end'));
	let start = $derived(page.url.searchParams.get('start'));
	let lastStart = $state<string | null>(null);
	let lastEnd = $state<string | null>(null);

	let endDate = $derived(end ? new Date(end) : new Date());
	let startDate = $derived(start ? new Date(start) : subDays(endDate, 7));

	const selectedModelIds = $derived(page.url.searchParams.getAll('model'));
	let filteredByModel = $derived(
		selectedModelIds.length > 0 ? selectedModelIds.join(',') : 'all_models'
	);
	const selectedUserIds = $derived(page.url.searchParams.getAll('user'));
	const selectedUserIdsForSelect = $derived(
		selectedUserIds.length > 0 ? selectedUserIds.join(',') : 'all_users'
	);
	let selectedTokenType = $derived(
		(page.url.searchParams.get('token_type') as 'input' | 'output') ?? 'input'
	);

	let totalTokensData = $state<TotalTokenUsage>();
	let data = $state<TokenUsage[]>([]);
	const selectedTargetModels = $derived.by(() => {
		const ids = selectedModelIds.filter((id) => id !== 'all_models');
		if (ids.length === 0) return null;
		// eslint-disable-next-line svelte/prefer-svelte-reactivity
		const targetModels = new Set<string>();
		for (const id of ids) {
			const model = modelsToDisplayName.get(id);
			if (model?.targetModel) targetModels.add(model.targetModel);
		}
		return targetModels.size > 0 ? targetModels : null;
	});

	const filteredData = $derived.by(() => {
		let result = data;
		const userIdsToFilter = selectedUserIds.filter((id) => id !== 'all_users');
		if (userIdsToFilter.length > 0) {
			const userSet = new Set(userIdsToFilter);
			result = result.filter((row) => row.userID != null && userSet.has(row.userID));
		}
		if (selectedTargetModels) {
			result = result.filter((row) => row.model != null && selectedTargetModels.has(row.model));
		}
		return result;
	});
	let groupBy = $derived(
		(page.url.searchParams.get('group_by') as 'group_by_users' | 'group_by_models' | null) ??
			'group_by_default'
	);

	let selectedSubview = $state<'models' | 'users'>('models');
	type SubViewSortBy =
		| 'sort_by_name'
		| 'sort_by_name_reverse'
		| 'sort_by_total_tokens'
		| 'sort_by_total_tokens_reverse';
	let subViewSortBy = $state<SubViewSortBy>('sort_by_total_tokens');
	let subViewSearchQuery = $state('');

	const usersMap = $derived(new Map(usersData.map((u) => [u.id, u])));
	const modelsToDisplayName = $derived(new Map(modelsData.map((m) => [m.id, m])));

	onMount(async () => {
		usersData = await AdminService.listUsersIncludeDeleted();
		modelsData = await AdminService.listModels({ all: true });

		AdminService.listTotalTokenUsage()
			.then((response) => {
				totalTokensData = response;
			})
			.catch((error) => {
				errors.append(error);
			})
			.finally(() => {
				loadingTotalTokensData = false;
			});
	});

	let fetchAbortController: AbortController | null = null;

	const DEFER_DATA_THRESHOLD = 400;
	async function fetchData(start: Date, end: Date) {
		fetchAbortController?.abort();
		fetchAbortController = new AbortController();
		const signal = fetchAbortController.signal;

		loadingTableData = true;
		const timeRange = { start, end };
		AdminService.listTokenUsage(timeRange, { signal })
			.then((tokenUsage) => {
				if (signal.aborted) return;
				if (tokenUsage.length <= DEFER_DATA_THRESHOLD) {
					data = tokenUsage;
					return;
				}
				// Defer so the UI can paint (200, loading off) before heavy derivation. Safari lacks requestIdleCallback.
				const schedule =
					typeof requestIdleCallback !== 'undefined'
						? (fn: () => void) => requestIdleCallback(fn, { timeout: 120 })
						: (fn: () => void) => setTimeout(fn, 0);
				schedule(() => {
					if (!signal.aborted) data = tokenUsage;
				});
			})
			.finally(() => {
				if (!signal.aborted) loadingTableData = false;
			})
			.catch((error) => {
				if (error?.name === 'AbortError') return;
				errors.append(error);
			});
	}

	$effect(() => {
		if (start && end) {
			if (start !== lastStart || end !== lastEnd) {
				lastStart = start;
				lastEnd = end;
				fetchData(startDate, endDate);
			}
		}
	});

	onMount(() => {
		fetchData(startDate, endDate);
	});

	const duration = PAGE_TRANSITION_DURATION;

	const targetModelToDisplayName = $derived(
		new Map(modelsData.map((m) => [m.targetModel, m.displayName || m.name]))
	);

	function toTimelineItem(r: TokenUsage, category: string): TokenUsageWithCategory {
		return {
			...r,
			date: r.date,
			promptTokens: r.promptTokens ?? 0,
			completionTokens: r.completionTokens ?? 0,
			totalTokens: r.totalTokens ?? (r.promptTokens ?? 0) + (r.completionTokens ?? 0),
			category
		};
	}

	function computeMainTimelineData(
		filtered: TokenUsage[],
		group: string,
		users: Map<string, OrgUser>,
		modelToName: Map<string, string>
	): TokenUsageWithCategory[] {
		if (group === 'group_by_users') {
			const userKeys = [...new Set(filtered.map((r) => r.userID ?? r.runName ?? 'Unknown'))].sort();
			const userKeyToLabel = getUserLabels(users, userKeys);
			return filtered.map((r) =>
				toTimelineItem(
					r,
					userKeyToLabel.get(r.userID ?? r.runName ?? 'Unknown') ??
						r.userID ??
						r.runName ??
						'Unknown'
				)
			);
		}
		if (group === 'group_by_models') {
			return filtered.map((r) =>
				toTimelineItem(r, modelToName.get(r.model ?? '') ?? r.model ?? 'Unknown')
			);
		}
		return filtered.map((r) => toTimelineItem(r, 'Token usage'));
	}

	type PerModelRow = {
		modelKey: string;
		modelLabel: string;
		timelineData: TokenUsageWithCategory[];
	};
	type PerUserRow = { userKey: string; userLabel: string; timelineData: TokenUsageWithCategory[] };

	let perModelPromptData = $state<PerModelRow[]>([]);
	let perUserPromptData = $state<PerUserRow[]>([]);

	const TIMELINE_AGGREGATE_THRESHOLD = 500;

	$effect(() => {
		const filtered = filteredData;
		const users = usersMap;
		const modelToName = targetModelToDisplayName;
		const threshold = TIMELINE_AGGREGATE_THRESHOLD;

		function computePerModel(): PerModelRow[] {
			if (!filtered.length) return [];
			const byModel = new SvelteMap<string, TokenUsage[]>();
			for (const r of filtered) {
				const model = r.model;
				if (!model) continue;
				let rows = byModel.get(model);
				if (!rows) {
					rows = [];
					byModel.set(model, rows);
				}
				rows.push(r);
			}
			return [...byModel.entries()].map(([model, modelRows]) => {
				const modelLabel = modelToName.get(model) ?? model;
				return {
					modelKey: model,
					modelLabel,
					timelineData: modelRows.map((r) => toTimelineItem(r, modelLabel))
				};
			});
		}

		function computePerUser(): PerUserRow[] {
			if (!filtered.length) return [];
			const byUser = new SvelteMap<string, TokenUsage[]>();
			for (const r of filtered) {
				const userKey = r.userID ?? r.runName ?? 'Unknown';
				let rows = byUser.get(userKey);
				if (!rows) {
					rows = [];
					byUser.set(userKey, rows);
				}
				rows.push(r);
			}
			const userKeys = [...byUser.keys()].sort();
			const userKeyToLabel = getUserLabels(users, userKeys);
			return userKeys.map((userKey) => {
				const userRows = byUser.get(userKey)!;
				const userLabel = userKeyToLabel.get(userKey) ?? userKey;
				return {
					userKey,
					userLabel,
					timelineData: userRows.map((r) => toTimelineItem(r, userLabel))
				};
			});
		}

		if (filtered.length <= threshold) {
			perModelPromptData = computePerModel();
			perUserPromptData = computePerUser();
			return;
		}

		perModelPromptData = [];
		perUserPromptData = [];
		const ac = new AbortController();
		const signal = ac.signal;
		const schedule =
			typeof requestIdleCallback !== 'undefined'
				? (fn: () => void) => requestIdleCallback(fn, { timeout: 200 })
				: (fn: () => void) => setTimeout(fn, 0);
		schedule(() => {
			if (signal.aborted) return;
			perModelPromptData = computePerModel();
			perUserPromptData = computePerUser();
		});
		return () => ac.abort();
	});

	function timelineDataForChartWithRange(
		items: TokenUsageWithCategory[],
		start: Date,
		end: Date
	): TokenUsageWithCategory[] {
		if (items.length <= TIMELINE_AGGREGATE_THRESHOLD) return items;
		return aggregateTimelineDataByBucket(items, start, end) as TokenUsageWithCategory[];
	}

	let mainChartData = $state<TokenUsageWithCategory[]>([]);

	$effect(() => {
		const filtered = filteredData;
		const group = groupBy;
		const start = startDate;
		const end = endDate;
		const users = usersMap;
		const modelToName = targetModelToDisplayName;
		const threshold = TIMELINE_AGGREGATE_THRESHOLD;

		if (filtered.length <= threshold) {
			const timeline = computeMainTimelineData(filtered, group, users, modelToName);
			mainChartData = timeline;
			return;
		}

		const schedule =
			typeof requestIdleCallback !== 'undefined'
				? (fn: () => void) => requestIdleCallback(fn, { timeout: 150 })
				: (fn: () => void) => setTimeout(fn, 0);
		schedule(() => {
			const timeline = computeMainTimelineData(filtered, group, users, modelToName);
			mainChartData = timelineDataForChartWithRange(timeline, start, end);
		});
	});

	type GraphItem = { label: string; timelineData: TokenUsageWithCategory[] };
	const graphItems = $derived.by((): GraphItem[] => {
		if (selectedSubview === 'models') {
			return perModelPromptData.map(({ modelLabel, timelineData }) => ({
				label: modelLabel,
				timelineData
			}));
		}
		return perUserPromptData.map(({ userLabel, timelineData }) => ({
			label: userLabel,
			timelineData
		}));
	});

	const GRID_DEFER_ITEMS_THRESHOLD = 4;
	const GRID_CHUNK_SIZE = 3;
	let displayGraphItems = $state<GraphItem[]>([]);
	const INITIAL_VISIBLE_CHARTS = 6;
	const CHARTS_PER_FRAME = 6;
	let visibleChartCount = $state(INITIAL_VISIBLE_CHARTS);
	let gridDataReady = $state(true);

	function sortGraphItems(items: GraphItem[], sortBy: SubViewSortBy): GraphItem[] {
		const total = (item: GraphItem) => {
			const total = item.timelineData.reduce(
				(sum, r) => sum + (r.totalTokens ?? (r.promptTokens ?? 0) + (r.completionTokens ?? 0)),
				0
			);
			return total;
		};
		const byNameAsc = (a: GraphItem, b: GraphItem) => a.label.localeCompare(b.label);
		const byNameDesc = (a: GraphItem, b: GraphItem) => b.label.localeCompare(a.label);
		const byTotalTokensDesc = (a: GraphItem, b: GraphItem) => total(b) - total(a);
		const byTotalTokensAsc = (a: GraphItem, b: GraphItem) => total(a) - total(b);
		const cmp =
			sortBy === 'sort_by_name'
				? byNameAsc
				: sortBy === 'sort_by_name_reverse'
					? byNameDesc
					: sortBy === 'sort_by_total_tokens'
						? byTotalTokensDesc
						: byTotalTokensAsc;
		return [...items].sort(cmp);
	}

	function filterGraphItemsBySearch(items: GraphItem[], query: string): GraphItem[] {
		const q = query.trim().toLowerCase();
		if (!q) return items;
		return items.filter((item) => item.label.toLowerCase().includes(q));
	}

	function hasTokenData(item: GraphItem): boolean {
		const total = item.timelineData.reduce(
			(sum, r) => sum + (r.totalTokens ?? (r.promptTokens ?? 0) + (r.completionTokens ?? 0)),
			0
		);
		return total > 0;
	}

	$effect(() => {
		const items = graphItems;
		const start = startDate;
		const end = endDate;
		const sortBy = subViewSortBy;
		const searchQuery = subViewSearchQuery;
		const threshold = TIMELINE_AGGREGATE_THRESHOLD;

		const shouldDefer =
			items.length > GRID_DEFER_ITEMS_THRESHOLD ||
			items.some((item) => item.timelineData.length > threshold);

		if (!shouldDefer) {
			gridDataReady = true;
			const mapped = items.map((item) => ({
				label: item.label,
				timelineData: timelineDataForChartWithRange(item.timelineData, start, end)
			}));
			const sorted = sortGraphItems(mapped, sortBy).filter(hasTokenData);
			displayGraphItems = filterGraphItemsBySearch(sorted, searchQuery);
			return;
		}

		gridDataReady = false;
		displayGraphItems = [];
		const ac = new AbortController();
		const signal = ac.signal;
		const accumulated: GraphItem[] = [];

		function processChunk(fromIndex: number) {
			if (signal.aborted) return;
			const chunk = items.slice(fromIndex, fromIndex + GRID_CHUNK_SIZE);
			for (const item of chunk) {
				accumulated.push({
					label: item.label,
					timelineData: timelineDataForChartWithRange(item.timelineData, start, end)
				});
			}
			const nextIndex = fromIndex + GRID_CHUNK_SIZE;
			if (nextIndex < items.length) {
				requestAnimationFrame(() => processChunk(nextIndex));
			} else {
				if (signal.aborted) return;
				const sorted = sortGraphItems(accumulated, sortBy).filter(hasTokenData);
				displayGraphItems = filterGraphItemsBySearch(sorted, searchQuery);
				gridDataReady = true;
			}
		}

		requestAnimationFrame(() => processChunk(0));
		return () => ac.abort();
	});

	$effect(() => {
		const total = displayGraphItems.length;
		if (total <= INITIAL_VISIBLE_CHARTS) {
			visibleChartCount = total;
			return;
		}
		visibleChartCount = INITIAL_VISIBLE_CHARTS;
		let cancelled = false;

		function tick() {
			if (cancelled) return;
			visibleChartCount = Math.min(visibleChartCount + CHARTS_PER_FRAME, total);
			if (visibleChartCount < total) {
				requestAnimationFrame(tick);
			}
		}
		requestAnimationFrame(tick);
		return () => {
			cancelled = true;
		};
	});

	function handleDateRangeChange(range: DateRange) {
		const currentUrl = new URL(page.url);
		currentUrl.searchParams.set('start', range.start?.toISOString() ?? '');
		currentUrl.searchParams.set('end', range.end?.toISOString() ?? '');
		goto(currentUrl, { noScroll: true, keepFocus: true });
	}

	function handleRemoveUserFilter(userId: string) {
		const currentUrl = new URL(page.url);
		const users = currentUrl.searchParams.getAll('user').filter((id) => id !== userId);
		currentUrl.searchParams.delete('user');
		for (const id of users) {
			currentUrl.searchParams.append('user', id);
		}
		goto(currentUrl, { noScroll: true, keepFocus: true });
	}

	function handleRemoveAllUserFilters() {
		const currentUrl = new URL(page.url);
		currentUrl.searchParams.delete('user');
		goto(currentUrl, { noScroll: true, keepFocus: true });
	}

	function handleToggleUserFilter(userId: string) {
		if (userId === 'all_users') {
			const currentUrl = new URL(page.url);
			currentUrl.searchParams.delete('user');
			goto(currentUrl, { noScroll: true, keepFocus: true });
			return;
		}
		const currentUrl = new URL(page.url);
		const users = currentUrl.searchParams.getAll('user');
		if (users.includes(userId)) {
			handleRemoveUserFilter(userId);
		} else {
			users.push(userId);
			currentUrl.searchParams.delete('user');
			for (const id of users) {
				currentUrl.searchParams.append('user', id);
			}
			goto(currentUrl, { noScroll: true, keepFocus: true });
		}
	}

	function handleRemoveModelFilter(modelId: string) {
		const currentUrl = new URL(page.url);
		const models = currentUrl.searchParams.getAll('model').filter((id) => id !== modelId);
		currentUrl.searchParams.delete('model');
		for (const id of models) {
			currentUrl.searchParams.append('model', id);
		}
		goto(currentUrl, { noScroll: true, keepFocus: true });
	}

	function handleRemoveAllModelFilters() {
		const currentUrl = new URL(page.url);
		currentUrl.searchParams.delete('model');
		goto(currentUrl, { noScroll: true, keepFocus: true });
	}

	function handleToggleModelFilter(modelId: string) {
		if (modelId === 'all_models') {
			const currentUrl = new URL(page.url);
			currentUrl.searchParams.delete('model');
			goto(currentUrl, { noScroll: true, keepFocus: true });
			return;
		}
		const currentUrl = new URL(page.url);
		const models = currentUrl.searchParams.getAll('model');
		if (models.includes(modelId)) {
			handleRemoveModelFilter(modelId);
		} else {
			models.push(modelId);
			currentUrl.searchParams.delete('model');
			for (const id of models) {
				currentUrl.searchParams.append('model', id);
			}
			goto(currentUrl, { noScroll: true, keepFocus: true });
		}
	}

	function handleGroupByChange(groupBy: string) {
		const currentUrl = new URL(page.url);
		currentUrl.searchParams.set('group_by', groupBy);
		if (groupBy !== 'group_by_default') {
			currentUrl.searchParams.set('token_type', 'input');
		} else {
			currentUrl.searchParams.delete('token_type');
		}
		goto(currentUrl, { noScroll: true, keepFocus: true });
	}

	function handleTokenTypeChange(tokenType: 'input' | 'output') {
		const currentUrl = new URL(page.url);
		currentUrl.searchParams.set('token_type', tokenType);
		goto(currentUrl, { noScroll: true, keepFocus: true });
	}

	const usersOptions = $derived([
		{ label: 'All Users', id: 'all_users' },
		...usersData.map((user) => ({ label: getUserDisplayName(usersMap, user.id), id: user.id }))
	]);

	const modelsOptions = $derived([
		{ label: 'All Models', id: 'all_models' },
		...modelsData.map((model) => ({ label: model.name, id: model.id }))
	]);
</script>

<Layout
	title="Token Usage"
	classes={{
		container: 'md:px-0 px-0 pt-0',
		childrenContainer: 'max-w-none',
		noSidebarTitle: 'pl-4 md:pl-8 mx-auto md:max-w-(--breakpoint-xl) pt-4'
	}}
>
	{#if loadingTableData}
		<div
			class="absolute inset-0 z-20 flex items-center justify-center"
			in:fade={{ duration: 100 }}
			out:fade|global={{ duration: 300, delay: 500 }}
		>
			<div
				class="bg-surface3/50 border-surface3 text-primary dark:text-primary flex flex-col items-center gap-4 rounded-2xl border px-16 py-8 shadow-md backdrop-blur-[1px]"
			>
				<Loading class="size-32 stroke-1" />
				<div class="text-2xl font-semibold">Loading data...</div>
			</div>
		</div>
	{/if}

	<div class="mb-4 flex flex-col gap-4" transition:fade={{ duration }}>
		<div class="bg-surface2 dark:bg-surface1 w-full">
			<div class="m-auto w-full px-4 py-4 md:max-w-(--breakpoint-xl) md:px-8">
				<h4 class="font-semibold">Overall Stats</h4>
				<div class="flex flex-col flex-wrap items-stretch gap-4 md:flex-row">
					{@render summary('Total Tokens', totalTokensData?.totalTokens ?? 0)}
					<div class="divider-horizontal hidden md:block"></div>
					{@render summary('Total Prompt Tokens', totalTokensData?.promptTokens ?? 0)}
					<div class="divider-horizontal hidden md:block"></div>
					{@render summary('Total Completion Tokens', totalTokensData?.completionTokens ?? 0)}
				</div>
			</div>
		</div>
		<div
			class="m-auto flex w-full max-w-full flex-col gap-4 px-4 md:max-w-(--breakpoint-xl) md:px-8"
		>
			<div class="flex w-full flex-wrap items-center justify-end gap-4">
				<p class="text-on-surface1 w-full text-sm md:w-fit">Filter by:</p>
				<Select
					class="dark:border-surface3 border border-transparent"
					classes={{
						root: 'w-full md:flex-1 dark:border-surface3'
					}}
					options={usersOptions}
					selected={selectedUserIdsForSelect}
					onSelect={(option) => handleToggleUserFilter(option.id)}
					onClear={(option) => option && handleRemoveUserFilter(option.id)}
					onClearAll={selectedUserIdsForSelect !== 'all_users'
						? () => handleRemoveAllUserFilters()
						: undefined}
					id="user-select"
					multiple
					searchInDropdown
					placeholder="Filter by user..."
					buttonReadOnly
					buttonTitle="Users"
					displayCount={!!selectedUserIdsForSelect && selectedUserIdsForSelect !== 'all_users'}
				/>
				<Select
					class="dark:border-surface3 border border-transparent"
					classes={{
						root: 'w-full md:flex-1 dark:border-surface3'
					}}
					options={modelsOptions}
					selected={filteredByModel}
					onSelect={(option) => handleToggleModelFilter(option.id)}
					onClear={(option) => option && handleRemoveModelFilter(option.id)}
					onClearAll={filteredByModel !== 'all_models'
						? () => handleRemoveAllModelFilters()
						: undefined}
					id="model-select"
					multiple
					searchInDropdown
					placeholder="Filter by model..."
					buttonReadOnly
					buttonTitle="Models"
					displayCount={!!filteredByModel && filteredByModel !== 'all_models'}
				/>
				<div class="bg-surface3 hidden h-8 w-0.5 md:block"></div>
				<AuditLogCalendar start={startDate} end={endDate} onChange={handleDateRangeChange} />
			</div>
			{#if filteredByModel !== 'all_models' || selectedUserIdsForSelect !== 'all_users'}
				<div class="flex flex-wrap items-center gap-2" in:slide={{ axis: 'y', duration: 100 }}>
					{#if selectedUserIdsForSelect !== 'all_users'}
						{@const userPills = selectedUserIds.map((selectedUser) => ({
							id: selectedUser,
							label: getUserDisplayName(usersMap, selectedUser)
						}))}
						{#each userPills as userPill (userPill.id)}
							<div class="filter-primary">
								<span class="font-semibold">User:</span>{userPill.label}
								<button class="ml-1" onclick={() => handleRemoveUserFilter(userPill.id)}>
									<X class="size-3" />
								</button>
							</div>
						{/each}
					{/if}
					{#if filteredByModel !== 'all_models'}
						{@const modelPills = selectedModelIds.map((selectedModel) => ({
							id: selectedModel,
							label: modelsToDisplayName.get(selectedModel)?.name ?? selectedModel
						}))}
						{#each modelPills as modelPill (modelPill.id)}
							<div class="filter-primary">
								<span class="font-semibold">Model:</span>{modelPill.label}
								<button class="ml-1" onclick={() => handleRemoveModelFilter(modelPill.id)}>
									<X class="size-3" />
								</button>
							</div>
						{/each}
					{/if}
				</div>
			{/if}
			<div class="paper w-full gap-0 pt-4">
				<div class="mb-1 flex flex-wrap justify-between gap-2">
					<div class="flex flex-wrap items-center gap-4">
						<h4 class="flex items-center gap-2 font-semibold">
							Prompt & Completion Tokens
							{#if loadingTableData}
								<Loading class="size-4 animate-spin" />
							{/if}
						</h4>

						<div class="flex shrink-0">
							<button
								class={twMerge(
									'button-secondary rounded-r-none border border-r-0 text-xs',
									selectedTokenType === 'input' && 'bg-surface2 border-surface2'
								)}
								onclick={() => handleTokenTypeChange('input')}
							>
								Input Tokens
							</button>
							<button
								class={twMerge(
									'button-secondary rounded-l-none border text-xs',
									selectedTokenType === 'output' && 'bg-surface2 border-surface2'
								)}
								onclick={() => handleTokenTypeChange('output')}
							>
								Output Tokens
							</button>
						</div>
					</div>
					<Select
						class="bg-surface2 dark:bg-background dark:border-surface3 w-[50dvw] border border-transparent shadow-inner md:w-64"
						options={[
							{ label: 'Group by Token Type', id: 'group_by_default' },
							{ label: 'Group by Users', id: 'group_by_users' },
							{ label: 'Group by Models', id: 'group_by_models' }
						]}
						selected={groupBy}
						onSelect={(option) => handleGroupByChange(option.id)}
					/>
				</div>
				<div class="w-full pt-2">
					{#key groupBy}
						<StackedTimeline
							start={startDate}
							end={endDate}
							data={mainChartData}
							dateKey="date"
							primaryValueKey={selectedTokenType === 'input' ? 'promptTokens' : 'completionTokens'}
							categoryKey="category"
							class="h-96"
							legend={{
								showSecondaryLabel: false,
								primaryLabel:
									groupBy === 'group_by_default'
										? selectedTokenType === 'input'
											? 'input tokens'
											: 'output tokens'
										: '',
								hideCategoryLabel: groupBy === 'group_by_default'
							}}
						>
							{#snippet tooltipContent(item)}
								{@const value = item.primaryTotal ?? 0}
								<div class="flex flex-col gap-0 text-xs">
									<div class="text-sm font-light">{item.key}</div>
									<div class="text-on-surface1">{item.date}</div>
									<div class="tooltip-divider"></div>
								</div>
								<div class="flex flex-col gap-1">
									<div class="text-on-background flex flex-col">
										<div class="text-xl font-bold">{value.toLocaleString()}</div>
									</div>
								</div>
							{/snippet}
						</StackedTimeline>
					{/key}
				</div>
			</div>

			<div class="relative mt-2 flex flex-col">
				<div class="relative z-10 flex shrink-0 items-center justify-between">
					<div class="flex shrink-0">
						<button
							class={twMerge(
								'w-24 border-b-2 border-transparent px-4 py-2 transition-colors duration-400',
								selectedSubview === 'models' && 'border-primary'
							)}
							onclick={() => {
								selectedSubview = 'models';
								subViewSearchQuery = '';
							}}
						>
							Models
						</button>
						<button
							class={twMerge(
								'w-24 border-b-2 border-transparent px-4 py-2 transition-colors duration-400',
								selectedSubview === 'users' && 'border-primary'
							)}
							onclick={() => {
								selectedSubview = 'users';
								subViewSearchQuery = '';
							}}
						>
							Users
						</button>
					</div>
					<Select
						class="bg-surface1 hover:bg-surface2 dark:bg-background dark:hover:bg-surface1 mb-1.5 border border-transparent shadow-none md:w-64"
						options={[
							{ label: 'Sort by Name (A-Z)', id: 'sort_by_name' },
							{ label: 'Sort by Name (Z-A)', id: 'sort_by_name_reverse' },
							{
								label: 'Sort by Total Tokens (Highest to Lower)',
								id: 'sort_by_total_tokens'
							},
							{
								label: 'Sort by Total Tokens (Lowest to Highest)',
								id: 'sort_by_total_tokens_reverse'
							}
						]}
						selected={subViewSortBy}
						onSelect={(option) => {
							subViewSortBy = option.id as SubViewSortBy;
						}}
						id="sub-view-sort-by-select"
					/>
				</div>
				<div class="bg-surface3 h-0.5 w-full shrink-0 -translate-y-1"></div>

				<Search
					class="bg-background dark:border-surface3 mt-2 mb-3 border border-transparent"
					value={subViewSearchQuery}
					onChange={(value) => (subViewSearchQuery = value)}
					placeholder={`Search ${selectedSubview === 'models' ? 'models' : 'users'}...`}
				/>

				{#if graphItems.length > 0}
					<div class="min-h-[300px]">
						{#if !gridDataReady}
							<div
								class="text-on-surface1 flex items-center justify-center gap-2 py-12 text-sm"
								aria-live="polite"
							>
								<Loading class="size-4 animate-spin" />
								<span>Preparing charts…</span>
							</div>
						{:else if displayGraphItems.length > 0}
							<div class="grid grid-cols-1 gap-4 md:grid-cols-2">
								{#each displayGraphItems.slice(0, visibleChartCount) as item (item.label)}
									<div class="paper flex min-h-0 flex-col overflow-hidden">
										<h5 class="shrink-0 text-sm font-medium">{item.label}</h5>
										<div class="w-full shrink-0">
											<StackedTimeline
												start={startDate}
												end={endDate}
												data={item.timelineData}
												categoryKey="category"
												dateKey="date"
												primaryValueKey="promptTokens"
												secondaryValueKey="completionTokens"
												class="h-48"
												legend={{
													hideCategoryLabel: true,
													showSecondaryLabel: true,
													primaryLabel: 'input tokens',
													secondaryLabel: 'output tokens'
												}}
												classes={{
													legend: 'pt-4 justify-start'
												}}
											>
												{#snippet tooltipContent(item)}
													{@const value =
														item.hoveredPart === 'primary'
															? (item.primaryTotal ?? 0)
															: (item.secondaryTotal ?? 0)}
													<div class="flex flex-col gap-0 text-xs">
														<div class="text-sm font-light">
															{item.hoveredPart === 'primary' ? 'Input tokens' : 'Output tokens'}
														</div>
														<div class="text-on-surface1">{item.date}</div>
														<div class="tooltip-divider"></div>
													</div>
													<div class="flex flex-col gap-1">
														<div class="text-on-background flex flex-col">
															<div class="text-xl font-bold">{value.toLocaleString()}</div>
														</div>
													</div>
												{/snippet}
											</StackedTimeline>
										</div>
									</div>
								{/each}
							</div>
							{#if visibleChartCount < displayGraphItems.length}
								<div
									class="text-on-surface1 flex items-center justify-center gap-2 py-4 text-sm"
									aria-live="polite"
								>
									<Loading class="size-4 animate-spin" />
									<span>Loading charts… {visibleChartCount} of {displayGraphItems.length}</span>
								</div>
							{/if}
						{:else}
							<div class="text-on-surface1 mx-auto py-12 text-center text-sm font-light">
								No matches found.
							</div>
						{/if}
					</div>
				{:else}
					<div class="text-on-surface1 mx-auto py-12 text-sm font-light">No data available.</div>
				{/if}
			</div>
		</div>
	</div>
</Layout>

{#snippet summary(title: string, value: number)}
	<div class="flex min-w-0 flex-1 flex-col gap-1 py-2">
		<div class="text-on-background text-xs font-light">{title}</div>
		<div class="text-primary flex items-center gap-1 text-xl font-semibold">
			{#if loadingTotalTokensData}
				<div class="py-2">
					<Loading class="size-4 animate-spin" />
				</div>
			{:else}
				{value.toLocaleString()}
				<Coins class="size-4" />
			{/if}
		</div>
	</div>
{/snippet}

<svelte:head>
	<title>Obot | Token Usage</title>
</svelte:head>

<style lang="postcss">
	.divider-horizontal {
		width: 1px;
		height: auto;
		background-color: var(--color-surface3);
		margin-left: 1rem;
		margin-right: 1rem;
	}
</style>
