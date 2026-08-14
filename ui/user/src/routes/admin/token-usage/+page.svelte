<script lang="ts">
	import { page } from '$app/state';
	import type { DateRange } from '$lib/components/Calendar.svelte';
	import Layout from '$lib/components/Layout.svelte';
	import Search from '$lib/components/Search.svelte';
	import Select from '$lib/components/Select.svelte';
	import AuditLogCalendar from '$lib/components/admin/audit-logs/AuditLogCalendar.svelte';
	import TokenUsageTimelineCard from '$lib/components/admin/token-usage/TokenUsageTimelineCard.svelte';
	import {
		ALL_API_KEYS,
		ALL_MODELS,
		ALL_USERS,
		CHART_LABEL,
		DEFAULT_TOKEN_GROUP_BY,
		DEFAULT_TOKEN_TYPE,
		DEFAULT_USAGE_SUBVIEW,
		DEFAULT_USAGE_SUBVIEW_SORT_BY,
		GRAPH_METRIC,
		GRAPH_MODE,
		TOKEN_USAGE_CATEGORY,
		TOKEN_USAGE_PARAMS,
		USAGE_BUCKET_LABEL,
		USAGE_SUBVIEW,
		USAGE_SUBVIEW_SORT_BY,
		USAGE_SUBVIEW_SORT_BY_SPEND_OPTIONS,
		USAGE_SUBVIEW_SORT_BY_TOKEN_OPTIONS,
		usageSubViewSortByForView,
		type GraphMetric,
		type GraphMode,
		type TokenGroupBy,
		type TokenType,
		type UsageSubView,
		type UsageSubViewSortBy
	} from '$lib/components/admin/token-usage/constants';
	import {
		toBucketTimelineItems,
		toTimelineItem,
		timelineDataForChartWithRange,
		formatTokenUsageUSD as formatUSD,
		bucketTooltipValueKeys,
		TIMELINE_AGGREGATE_THRESHOLD,
		type TokenUsageTimelineItem
	} from '$lib/components/admin/token-usage/tokenUsageTimeline';
	import { getAPIKeyFilterOptions, getUserLabels } from '$lib/components/admin/token-usage/utils';
	import StackedTimeline from '$lib/components/graph/StackedTimeline.svelte';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants';
	import Loading from '$lib/icons/Loading.svelte';
	import {
		AdminService,
		type Model,
		type OrgUser,
		type TokenUsage,
		type TotalTokenUsage,
		UserService
	} from '$lib/services';
	import { errors, responsive } from '$lib/stores';
	import { goto } from '$lib/url';
	import { getUserDisplayName } from '$lib/utils';
	import { X } from '@lucide/svelte';
	import { subDays } from 'date-fns';
	import { onMount } from 'svelte';
	import { fade } from 'svelte/transition';
	import { slide } from 'svelte/transition';
	import { twMerge } from 'tailwind-merge';

	let loadingTableData = $state(true);
	let loadingTotalTokensData = $state(true);
	let usersData = $state<OrgUser[]>([]);
	let modelsData = $state<Model[]>([]);

	let end = $derived(page.url.searchParams.get(TOKEN_USAGE_PARAMS.END));
	let start = $derived(page.url.searchParams.get(TOKEN_USAGE_PARAMS.START));
	let lastStart = $state<string | null>(null);
	let lastEnd = $state<string | null>(null);

	let endDate = $derived(end ? new Date(end) : new Date());
	let startDate = $derived(start ? new Date(start) : subDays(endDate, 7));

	const selectedModelIds = $derived(page.url.searchParams.getAll(TOKEN_USAGE_PARAMS.MODEL));
	let filteredByModel = $derived(
		selectedModelIds.length > 0 ? selectedModelIds.join(',') : ALL_MODELS
	);
	const selectedUserIds = $derived(page.url.searchParams.getAll(TOKEN_USAGE_PARAMS.USER));
	const selectedUserIdsForSelect = $derived(
		selectedUserIds.length > 0 ? selectedUserIds.join(',') : ALL_USERS
	);
	const selectedAPIKeyIDs = $derived(page.url.searchParams.getAll(TOKEN_USAGE_PARAMS.API_KEY));
	const selectedAPIKeyIDsForSelect = $derived(
		selectedAPIKeyIDs.length > 0 ? selectedAPIKeyIDs.join(',') : ALL_API_KEYS
	);
	let selectedTokenType = $derived(
		(page.url.searchParams.get(TOKEN_USAGE_PARAMS.TOKEN_TYPE) as TokenType) ?? DEFAULT_TOKEN_TYPE
	);

	let totalTokensData = $state<TotalTokenUsage>();
	let data = $state<TokenUsage[]>([]);
	const selectedTargetModels = $derived.by(() => {
		const ids = selectedModelIds.filter((id) => id !== ALL_MODELS);
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
		const userIdsToFilter = selectedUserIds.filter((id) => id !== ALL_USERS);
		if (userIdsToFilter.length > 0) {
			const userSet = new Set(userIdsToFilter);
			result = result.filter((row) => row.userID != null && userSet.has(row.userID));
		}
		if (selectedTargetModels) {
			result = result.filter((row) => row.model != null && selectedTargetModels.has(row.model));
		}
		const apiKeyIDsToFilter = selectedAPIKeyIDs.filter((id) => id !== ALL_API_KEYS);
		if (apiKeyIDsToFilter.length > 0) {
			const apiKeySet = new Set(apiKeyIDsToFilter);
			result = result.filter(
				(row) => row.apiKeyID != null && apiKeySet.has(row.apiKeyID.toString())
			);
		}
		return result;
	});
	let groupBy = $derived(
		(page.url.searchParams.get(TOKEN_USAGE_PARAMS.GROUP_BY) as TokenGroupBy) ??
			DEFAULT_TOKEN_GROUP_BY
	);

	let selectedSubview = $state<UsageSubView>(DEFAULT_USAGE_SUBVIEW);
	let subViewSortBy = $state<UsageSubViewSortBy>(DEFAULT_USAGE_SUBVIEW_SORT_BY);
	let subViewSearchQuery = $state('');

	function selectSubview(view: UsageSubView) {
		selectedSubview = view;
		subViewSearchQuery = '';
		subViewSortBy = usageSubViewSortByForView(subViewSortBy, view);
	}

	const usersMap = $derived(new Map(usersData.map((u) => [u.id, u])));
	const modelsToDisplayName = $derived(new Map(modelsData.map((m) => [m.id, m])));

	onMount(async () => {
		usersData = await UserService.listUsersIncludeDeleted();
		modelsData = await AdminService.listModels({ all: true });
	});

	let fetchAbortController: AbortController | null = null;

	const DEFER_DATA_THRESHOLD = 400;
	async function fetchData(start: Date, end: Date) {
		fetchAbortController?.abort();
		fetchAbortController = new AbortController();
		const signal = fetchAbortController.signal;

		loadingTableData = true;
		loadingTotalTokensData = true;
		const timeRange = { start, end };

		AdminService.listTotalTokenUsage(timeRange, { signal })
			.then((response) => {
				if (signal.aborted) return;
				totalTokensData = response;
			})
			.catch((error) => {
				if (error?.name === 'AbortError') return;
				errors.append(error);
			})
			.finally(() => {
				if (!signal.aborted) loadingTotalTokensData = false;
			});

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

	type PerModelRow = {
		modelKey: string;
		modelLabel: string;
		timelineData: TokenUsageTimelineItem[];
	};
	type PerUserRow = {
		userKey: string;
		userLabel: string;
		timelineData: TokenUsageTimelineItem[];
	};
	type PerAPIKeyRow = {
		apiKeyID: string;
		apiKeyLabel: string;
		timelineData: TokenUsageTimelineItem[];
	};

	let perModelPromptData = $state<PerModelRow[]>([]);
	let perUserPromptData = $state<PerUserRow[]>([]);
	let perAPIKeyPromptData = $state<PerAPIKeyRow[]>([]);

	$effect(() => {
		const filtered = filteredData;
		const users = usersMap;
		const modelToName = targetModelToDisplayName;
		const threshold = TIMELINE_AGGREGATE_THRESHOLD;

		function computePerModel(): PerModelRow[] {
			if (!filtered.length) return [];
			// eslint-disable-next-line svelte/prefer-svelte-reactivity
			const byModel = new Map<string, TokenUsage[]>();
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
					timelineData: modelRows.flatMap(toBucketTimelineItems)
				};
			});
		}

		function computePerUser(): PerUserRow[] {
			if (!filtered.length) return [];
			// eslint-disable-next-line svelte/prefer-svelte-reactivity
			const byUser = new Map<string, TokenUsage[]>();
			for (const r of filtered) {
				const userKey = r.userID ?? TOKEN_USAGE_CATEGORY.UNKNOWN;
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

		function computePerAPIKey(): PerAPIKeyRow[] {
			if (!filtered.length) return [];
			// eslint-disable-next-line svelte/prefer-svelte-reactivity
			const byAPIKey = new Map<string, TokenUsage[]>();
			for (const row of filtered) {
				if (row.apiKeyID == null) continue;
				const apiKeyID = row.apiKeyID.toString();
				let rows = byAPIKey.get(apiKeyID);
				if (!rows) {
					rows = [];
					byAPIKey.set(apiKeyID, rows);
				}
				rows.push(row);
			}
			const labels = new Map(
				getAPIKeyFilterOptions(filtered, users).map((option) => [option.id, option.label])
			);
			return [...byAPIKey.entries()].map(([apiKeyID, rows]) => {
				const apiKeyLabel = labels.get(apiKeyID) ?? `API key #${apiKeyID}`;
				return {
					apiKeyID,
					apiKeyLabel,
					timelineData: rows.map((row) => toTimelineItem(row, apiKeyLabel))
				};
			});
		}

		if (filtered.length <= threshold) {
			perModelPromptData = computePerModel();
			perUserPromptData = computePerUser();
			perAPIKeyPromptData = computePerAPIKey();
			return;
		}

		perModelPromptData = [];
		perUserPromptData = [];
		perAPIKeyPromptData = [];
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
			perAPIKeyPromptData = computePerAPIKey();
		});
		return () => ac.abort();
	});

	type GraphItem = {
		key: string;
		label: string;
		timelineData: TokenUsageTimelineItem[];
		mode: GraphMode;
		metric: GraphMetric;
	};
	const graphItems = $derived.by((): GraphItem[] => {
		if (selectedSubview === USAGE_SUBVIEW.MODELS || selectedSubview === USAGE_SUBVIEW.SPEND) {
			const metric: GraphMetric =
				selectedSubview === USAGE_SUBVIEW.SPEND ? GRAPH_METRIC.SPEND : GRAPH_METRIC.TOKENS;
			return perModelPromptData.map(({ modelKey, modelLabel, timelineData }) => ({
				key: modelKey,
				label: modelLabel,
				timelineData,
				mode: GRAPH_MODE.BUCKET,
				metric
			}));
		}
		if (selectedSubview === USAGE_SUBVIEW.API_KEYS) {
			return perAPIKeyPromptData.map(({ apiKeyID, apiKeyLabel, timelineData }) => ({
				key: apiKeyID,
				label: apiKeyLabel,
				timelineData,
				mode: GRAPH_MODE.INPUT_OUTPUT,
				metric: GRAPH_METRIC.TOKENS
			}));
		}
		return perUserPromptData.map(({ userKey, userLabel, timelineData }) => ({
			key: userKey,
			label: userLabel,
			timelineData,
			mode: GRAPH_MODE.INPUT_OUTPUT,
			metric: GRAPH_METRIC.TOKENS
		}));
	});

	const GRID_DEFER_ITEMS_THRESHOLD = 4;
	const GRID_CHUNK_SIZE = 3;
	let displayGraphItems = $state<GraphItem[]>([]);
	const INITIAL_VISIBLE_CHARTS = 6;
	const CHARTS_PER_FRAME = 6;
	let visibleChartCount = $state(INITIAL_VISIBLE_CHARTS);
	let gridDataReady = $state(true);

	function graphItemTokens(item: GraphItem): number {
		return item.timelineData.reduce((sum, r) => {
			const rowTokens =
				item.mode === GRAPH_MODE.BUCKET
					? (r.bucketTokens ?? 0)
					: (r.totalTokens ?? (r.inputTokens ?? 0) + (r.outputTokens ?? 0));
			return sum + rowTokens;
		}, 0);
	}

	function graphItemSpend(item: GraphItem): number {
		return item.timelineData.reduce((sum, r) => {
			const rowSpend =
				item.mode === GRAPH_MODE.BUCKET
					? (r.bucketSpend ?? 0)
					: (r.totalSpend ?? (r.inputSpend ?? 0) + (r.outputSpend ?? 0));
			return sum + rowSpend;
		}, 0);
	}

	function sortGraphItems(items: GraphItem[], sortBy: UsageSubViewSortBy): GraphItem[] {
		const byNameAsc = (a: GraphItem, b: GraphItem) => a.label.localeCompare(b.label);
		const byNameDesc = (a: GraphItem, b: GraphItem) => b.label.localeCompare(a.label);
		const byTotalTokensDesc = (a: GraphItem, b: GraphItem) =>
			graphItemTokens(b) - graphItemTokens(a);
		const byTotalTokensAsc = (a: GraphItem, b: GraphItem) =>
			graphItemTokens(a) - graphItemTokens(b);
		const byTotalSpendDesc = (a: GraphItem, b: GraphItem) => graphItemSpend(b) - graphItemSpend(a);
		const byTotalSpendAsc = (a: GraphItem, b: GraphItem) => graphItemSpend(a) - graphItemSpend(b);
		const sortByFn = {
			[USAGE_SUBVIEW_SORT_BY.NAME]: byNameAsc,
			[USAGE_SUBVIEW_SORT_BY.NAME_REVERSE]: byNameDesc,
			[USAGE_SUBVIEW_SORT_BY.TOTAL_TOKENS]: byTotalTokensDesc,
			[USAGE_SUBVIEW_SORT_BY.TOTAL_TOKENS_REVERSE]: byTotalTokensAsc,
			[USAGE_SUBVIEW_SORT_BY.TOTAL_SPEND]: byTotalSpendDesc,
			[USAGE_SUBVIEW_SORT_BY.TOTAL_SPEND_REVERSE]: byTotalSpendAsc
		};
		return [...items].sort(sortByFn[sortBy]);
	}

	function filterGraphItemsBySearch(items: GraphItem[], query: string): GraphItem[] {
		const q = query.trim().toLowerCase();
		if (!q) return items;
		return items.filter((item) => item.label.toLowerCase().includes(q));
	}

	function hasGraphData(item: GraphItem): boolean {
		return item.metric === GRAPH_METRIC.SPEND
			? graphItemSpend(item) > 0
			: graphItemTokens(item) > 0;
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
				key: item.key,
				label: item.label,
				timelineData: timelineDataForChartWithRange(item.timelineData, start, end),
				mode: item.mode,
				metric: item.metric
			}));
			const sorted = sortGraphItems(mapped, sortBy).filter(hasGraphData);
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
					key: item.key,
					label: item.label,
					timelineData: timelineDataForChartWithRange(item.timelineData, start, end),
					mode: item.mode,
					metric: item.metric
				});
			}
			const nextIndex = fromIndex + GRID_CHUNK_SIZE;
			if (nextIndex < items.length) {
				requestAnimationFrame(() => processChunk(nextIndex));
			} else {
				if (signal.aborted) return;
				const sorted = sortGraphItems(accumulated, sortBy).filter(hasGraphData);
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
		currentUrl.searchParams.set(TOKEN_USAGE_PARAMS.START, range.start?.toISOString() ?? '');
		currentUrl.searchParams.set(TOKEN_USAGE_PARAMS.END, range.end?.toISOString() ?? '');
		goto(currentUrl, { noScroll: true, keepFocus: true });
	}

	function handleRemoveUserFilter(userId: string) {
		const currentUrl = new URL(page.url);
		const users = currentUrl.searchParams
			.getAll(TOKEN_USAGE_PARAMS.USER)
			.filter((id) => id !== userId);
		currentUrl.searchParams.delete(TOKEN_USAGE_PARAMS.USER);
		for (const id of users) {
			currentUrl.searchParams.append(TOKEN_USAGE_PARAMS.USER, id);
		}
		goto(currentUrl, { noScroll: true, keepFocus: true });
	}

	function handleRemoveAllUserFilters() {
		const currentUrl = new URL(page.url);
		currentUrl.searchParams.delete(TOKEN_USAGE_PARAMS.USER);
		goto(currentUrl, { noScroll: true, keepFocus: true });
	}

	function handleToggleUserFilter(userId: string) {
		if (userId === ALL_USERS) {
			const currentUrl = new URL(page.url);
			currentUrl.searchParams.delete(TOKEN_USAGE_PARAMS.USER);
			goto(currentUrl, { noScroll: true, keepFocus: true });
			return;
		}
		const currentUrl = new URL(page.url);
		const users = currentUrl.searchParams.getAll(TOKEN_USAGE_PARAMS.USER);
		if (users.includes(userId)) {
			handleRemoveUserFilter(userId);
		} else {
			users.push(userId);
			currentUrl.searchParams.delete(TOKEN_USAGE_PARAMS.USER);
			for (const id of users) {
				currentUrl.searchParams.append(TOKEN_USAGE_PARAMS.USER, id);
			}
			goto(currentUrl, { noScroll: true, keepFocus: true });
		}
	}

	function handleRemoveAPIKeyFilter(apiKeyID: string) {
		const currentUrl = new URL(page.url);
		const apiKeyIDs = currentUrl.searchParams
			.getAll(TOKEN_USAGE_PARAMS.API_KEY)
			.filter((id) => id !== apiKeyID);
		currentUrl.searchParams.delete(TOKEN_USAGE_PARAMS.API_KEY);
		for (const id of apiKeyIDs) {
			currentUrl.searchParams.append(TOKEN_USAGE_PARAMS.API_KEY, id);
		}
		goto(currentUrl, { noScroll: true, keepFocus: true });
	}

	function handleRemoveAllAPIKeyFilters() {
		const currentUrl = new URL(page.url);
		currentUrl.searchParams.delete(TOKEN_USAGE_PARAMS.API_KEY);
		goto(currentUrl, { noScroll: true, keepFocus: true });
	}

	function handleToggleAPIKeyFilter(apiKeyID: string) {
		if (apiKeyID === ALL_API_KEYS) {
			handleRemoveAllAPIKeyFilters();
			return;
		}
		const currentUrl = new URL(page.url);
		const apiKeyIDs = currentUrl.searchParams.getAll(TOKEN_USAGE_PARAMS.API_KEY);
		if (apiKeyIDs.includes(apiKeyID)) {
			handleRemoveAPIKeyFilter(apiKeyID);
		} else {
			apiKeyIDs.push(apiKeyID);
			currentUrl.searchParams.delete(TOKEN_USAGE_PARAMS.API_KEY);
			for (const id of apiKeyIDs) {
				currentUrl.searchParams.append(TOKEN_USAGE_PARAMS.API_KEY, id);
			}
			goto(currentUrl, { noScroll: true, keepFocus: true });
		}
	}

	function handleRemoveModelFilter(modelId: string) {
		const currentUrl = new URL(page.url);
		const models = currentUrl.searchParams
			.getAll(TOKEN_USAGE_PARAMS.MODEL)
			.filter((id) => id !== modelId);
		currentUrl.searchParams.delete(TOKEN_USAGE_PARAMS.MODEL);
		for (const id of models) {
			currentUrl.searchParams.append(TOKEN_USAGE_PARAMS.MODEL, id);
		}
		goto(currentUrl, { noScroll: true, keepFocus: true });
	}

	function handleRemoveAllModelFilters() {
		const currentUrl = new URL(page.url);
		currentUrl.searchParams.delete(TOKEN_USAGE_PARAMS.MODEL);
		goto(currentUrl, { noScroll: true, keepFocus: true });
	}

	function handleToggleModelFilter(modelId: string) {
		if (modelId === ALL_MODELS) {
			const currentUrl = new URL(page.url);
			currentUrl.searchParams.delete(TOKEN_USAGE_PARAMS.MODEL);
			goto(currentUrl, { noScroll: true, keepFocus: true });
			return;
		}
		const currentUrl = new URL(page.url);
		const models = currentUrl.searchParams.getAll(TOKEN_USAGE_PARAMS.MODEL);
		if (models.includes(modelId)) {
			handleRemoveModelFilter(modelId);
		} else {
			models.push(modelId);
			currentUrl.searchParams.delete(TOKEN_USAGE_PARAMS.MODEL);
			for (const id of models) {
				currentUrl.searchParams.append(TOKEN_USAGE_PARAMS.MODEL, id);
			}
			goto(currentUrl, { noScroll: true, keepFocus: true });
		}
	}

	function handleGroupByChange(groupBy: string) {
		const currentUrl = new URL(page.url);
		currentUrl.searchParams.set(TOKEN_USAGE_PARAMS.GROUP_BY, groupBy);
		goto(currentUrl, { noScroll: true, keepFocus: true });
	}

	function handleTokenTypeChange(tokenType: TokenType) {
		const currentUrl = new URL(page.url);
		currentUrl.searchParams.set(TOKEN_USAGE_PARAMS.TOKEN_TYPE, tokenType);
		goto(currentUrl, { noScroll: true, keepFocus: true });
	}

	const usersOptions = $derived([
		{ label: 'All Users', id: ALL_USERS },
		...usersData.map((user) => ({ label: getUserDisplayName(usersMap, user.id), id: user.id }))
	]);

	const modelsOptions = $derived([
		{ label: 'All Models', id: ALL_MODELS },
		...modelsData.map((model) => ({ label: model.name, id: model.id }))
	]);

	const apiKeyOptions = $derived([
		{ label: 'All API Keys', id: ALL_API_KEYS },
		...getAPIKeyFilterOptions(data, usersMap)
	]);
	const apiKeyOptionsMap = $derived(
		new Map(apiKeyOptions.map((option) => [option.id, option.label]))
	);

	const subViewSortByOptions = $derived(
		selectedSubview === USAGE_SUBVIEW.SPEND
			? USAGE_SUBVIEW_SORT_BY_SPEND_OPTIONS
			: USAGE_SUBVIEW_SORT_BY_TOKEN_OPTIONS
	);
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
				class="bg-base-400/50 border-base-400 text-primary dark:text-primary flex flex-col items-center gap-4 rounded-2xl border px-16 py-8 shadow-md backdrop-blur-[1px]"
			>
				<Loading class="size-32 stroke-1" />
				<div class="text-2xl font-semibold">Loading data...</div>
			</div>
		</div>
	{/if}

	<div class="mb-4 flex flex-col gap-4" transition:fade={{ duration }}>
		<div class="bg-base-300 dark:bg-base-200 w-full">
			<div class="m-auto w-full px-4 py-4 md:max-w-(--breakpoint-xl) md:px-8">
				<h4 class="font-semibold">Overall Stats</h4>
				<div class="flex flex-col flex-wrap items-stretch gap-4 md:flex-row">
					{@render summary('Total', totalTokensData?.totalTokens ?? 0)}
					<div class="divider-horizontal hidden md:block"></div>
					{@render summary('Input', totalTokensData?.inputTokens ?? 0)}
					<div class="divider-horizontal hidden md:block"></div>
					{@render summary('Output', totalTokensData?.outputTokens ?? 0)}
					<div class="divider-horizontal hidden md:block"></div>
					{@render summary(
						'Cached Input',
						(totalTokensData?.cacheReadTokens ?? 0) + (totalTokensData?.cacheWriteTokens ?? 0)
					)}
					<div class="divider-horizontal hidden md:block"></div>
					{@render spendSummary('Spend', totalTokensData?.totalSpend)}
				</div>
			</div>
		</div>
		<div
			class="m-auto flex w-full max-w-full flex-col gap-4 px-4 md:max-w-(--breakpoint-xl) md:px-8"
		>
			<div class="flex w-full flex-wrap items-center justify-end gap-4">
				<p class="text-muted-content w-full text-sm md:w-fit">Filter by:</p>
				<Select
					class="dark:border-base-400 border border-transparent"
					classes={{
						root: 'w-full md:min-w-72 md:flex-[2] dark:border-base-400'
					}}
					options={apiKeyOptions}
					selected={selectedAPIKeyIDsForSelect}
					onSelect={(option) => handleToggleAPIKeyFilter(option.id)}
					onClear={(option) => option && handleRemoveAPIKeyFilter(option.id)}
					onClearAll={selectedAPIKeyIDsForSelect !== ALL_API_KEYS
						? () => handleRemoveAllAPIKeyFilters()
						: undefined}
					id="api-key-select"
					multiple
					searchInDropdown
					placeholder="Filter by API key..."
					buttonReadOnly
					buttonTitle="API Keys"
					displayCount={!!selectedAPIKeyIDsForSelect && selectedAPIKeyIDsForSelect !== ALL_API_KEYS}
				/>
				<Select
					class="dark:border-base-400 border border-transparent"
					classes={{
						root: 'w-full md:flex-1 dark:border-base-400'
					}}
					options={usersOptions}
					selected={selectedUserIdsForSelect}
					onSelect={(option) => handleToggleUserFilter(option.id)}
					onClear={(option) => option && handleRemoveUserFilter(option.id)}
					onClearAll={selectedUserIdsForSelect !== ALL_USERS
						? () => handleRemoveAllUserFilters()
						: undefined}
					id="user-select"
					multiple
					searchInDropdown
					placeholder="Filter by user..."
					buttonReadOnly
					buttonTitle="Users"
					displayCount={!!selectedUserIdsForSelect && selectedUserIdsForSelect !== ALL_USERS}
				/>
				<Select
					class="dark:border-base-400 border border-transparent"
					classes={{
						root: 'w-full md:flex-1 dark:border-base-400'
					}}
					options={modelsOptions}
					selected={filteredByModel}
					onSelect={(option) => handleToggleModelFilter(option.id)}
					onClear={(option) => option && handleRemoveModelFilter(option.id)}
					onClearAll={filteredByModel !== ALL_MODELS
						? () => handleRemoveAllModelFilters()
						: undefined}
					id="model-select"
					multiple
					searchInDropdown
					placeholder="Filter by model..."
					buttonReadOnly
					buttonTitle="Models"
					displayCount={!!filteredByModel && filteredByModel !== ALL_MODELS}
				/>
				<div class="bg-base-400 hidden h-8 w-0.5 md:block"></div>
				<AuditLogCalendar start={startDate} end={endDate} onChange={handleDateRangeChange} />
			</div>
			{#if filteredByModel !== ALL_MODELS || selectedUserIdsForSelect !== ALL_USERS || selectedAPIKeyIDsForSelect !== ALL_API_KEYS}
				<div class="flex flex-wrap items-center gap-2" in:slide={{ axis: 'y', duration: 100 }}>
					{#if selectedUserIdsForSelect !== ALL_USERS}
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
					{#if selectedAPIKeyIDsForSelect !== ALL_API_KEYS}
						{#each selectedAPIKeyIDs as apiKeyID (apiKeyID)}
							<div class="filter-primary">
								<span class="font-semibold">API Key:</span>{apiKeyOptionsMap.get(apiKeyID) ??
									`API key #${apiKeyID}`}
								<button class="ml-1" onclick={() => handleRemoveAPIKeyFilter(apiKeyID)}>
									<X class="size-3" />
								</button>
							</div>
						{/each}
					{/if}
					{#if filteredByModel !== ALL_MODELS}
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
			<TokenUsageTimelineCard
				{startDate}
				{endDate}
				data={filteredData}
				loading={loadingTableData}
				users={usersData}
				models={modelsData}
				{selectedTokenType}
				{groupBy}
				onTokenTypeChange={handleTokenTypeChange}
				onGroupByChange={handleGroupByChange}
			/>

			<div class="relative mt-2 flex flex-col">
				<div class="relative z-10 flex shrink-0 items-center justify-between">
					<div class="flex shrink-0 min-h-12">
						<button
							class={twMerge(
								'w-28 whitespace-nowrap border-b-2 border-transparent px-4 py-2 transition-colors duration-400',
								selectedSubview === USAGE_SUBVIEW.API_KEYS
									? 'border-primary'
									: 'hover:border-primary/25 text-muted-content hover:text-base-content'
							)}
							onclick={() => selectSubview(USAGE_SUBVIEW.API_KEYS)}
						>
							API Keys
						</button>
						<button
							class={twMerge(
								'w-24 border-b-2 border-transparent px-4 py-2 transition-colors duration-400',
								selectedSubview === USAGE_SUBVIEW.MODELS
									? 'border-primary'
									: 'hover:border-primary/25 text-muted-content hover:text-base-content'
							)}
							onclick={() => selectSubview(USAGE_SUBVIEW.MODELS)}
						>
							Models
						</button>
						<button
							class={twMerge(
								'w-24 border-b-2 border-transparent px-4 py-2 transition-colors duration-400',
								selectedSubview === USAGE_SUBVIEW.USERS
									? 'border-primary'
									: 'hover:border-primary/25 text-muted-content hover:text-base-content'
							)}
							onclick={() => selectSubview(USAGE_SUBVIEW.USERS)}
						>
							Users
						</button>
						<button
							class={twMerge(
								'w-24 border-b-2 border-transparent px-4 py-2 transition-colors duration-400',
								selectedSubview === USAGE_SUBVIEW.SPEND
									? 'border-primary'
									: 'hover:border-primary/25 text-muted-content hover:text-base-content'
							)}
							onclick={() => selectSubview(USAGE_SUBVIEW.SPEND)}
						>
							Spend
						</button>
					</div>
					{#if !responsive.isMobile}
						{@render subViewSortBySelect()}
					{/if}
				</div>
				<div class="bg-base-400 h-0.5 w-full shrink-0 -translate-y-0.5"></div>

				<div class="flex flex-col gap-1 mt-2 mb-3">
					{#if responsive.isMobile}
						{@render subViewSortBySelect()}
					{/if}

					<Search
						class="bg-base-100 dark:border-base-400 border border-transparent"
						value={subViewSearchQuery}
						onChange={(value) => (subViewSearchQuery = value)}
						placeholder={`Search ${selectedSubview === USAGE_SUBVIEW.USERS ? 'users' : selectedSubview === USAGE_SUBVIEW.API_KEYS ? 'API keys' : 'models'}...`}
					/>
				</div>

				{#if graphItems.length > 0}
					<div class="min-h-75">
						{#if !gridDataReady}
							<div
								class="text-muted-content flex items-center justify-center gap-2 py-12 text-sm"
								aria-live="polite"
							>
								<Loading class="size-4 animate-spin" />
								<span>Preparing charts…</span>
							</div>
						{:else if displayGraphItems.length > 0}
							<div class="grid grid-cols-1 gap-4 md:grid-cols-2">
								{#each displayGraphItems.slice(0, visibleChartCount) as item (`${item.metric}-${item.key}`)}
									<div class="paper p-0 flex min-h-0 flex-col overflow-hidden">
										<h5
											class="shrink-0 text-xs font-medium uppercase border-b dark:border-base-400 border-base-300 px-4 py-2 rounded-t-md"
										>
											{item.label}
										</h5>
										<div class="w-full shrink-0 p-4">
											{#if item.mode === GRAPH_MODE.BUCKET}
												{@const isSpend = item.metric === GRAPH_METRIC.SPEND}
												<StackedTimeline
													start={startDate}
													end={endDate}
													data={item.timelineData}
													categoryKey="category"
													dateKey="date"
													primaryValueKey={isSpend ? 'bucketSpend' : 'bucketTokens'}
													tooltipValueKeys={bucketTooltipValueKeys}
													class="h-48"
													legend={{
														showSecondaryLabel: false,
														primaryLabel: isSpend ? CHART_LABEL.SPEND : CHART_LABEL.TOKENS
													}}
													classes={{
														legend: 'pt-4 justify-start'
													}}
												>
													{#snippet tooltipContent(item)}
														{@const value = item.primaryTotal ?? 0}
														<div class="flex flex-col gap-0 text-xs">
															<div class="text-sm font-light">{item.key}</div>
															<div class="text-muted-content">{item.date}</div>
															<div class="tooltip-divider"></div>
														</div>
														<div class="flex flex-col gap-1">
															<div class="text-base-content flex flex-col">
																{#if isSpend}
																	<div class="text-xl font-bold">{formatUSD(value)}</div>
																	{#if item.key === USAGE_BUCKET_LABEL.INPUT}
																		<div class="text-muted-content mt-1 text-xs">
																			Cache read: {formatUSD(item.details?.cacheReadSpend ?? 0)}
																		</div>
																		<div class="text-muted-content text-xs">
																			Cache write: {formatUSD(item.details?.cacheWriteSpend ?? 0)}
																		</div>
																	{/if}
																{:else}
																	{@const spend = item.details?.bucketSpend ?? 0}
																	<div class="text-xl font-bold">{value.toLocaleString()}</div>
																	<div class="text-muted-content text-xs">{formatUSD(spend)}</div>
																	{#if item.key === USAGE_BUCKET_LABEL.INPUT}
																		<div class="text-muted-content mt-1 text-xs">
																			Cache read: {(
																				item.details?.cacheReadTokens ?? 0
																			).toLocaleString()}
																			tokens, {formatUSD(item.details?.cacheReadSpend ?? 0)}
																		</div>
																		<div class="text-muted-content text-xs">
																			Cache write: {(
																				item.details?.cacheWriteTokens ?? 0
																			).toLocaleString()}
																			tokens, {formatUSD(item.details?.cacheWriteSpend ?? 0)}
																		</div>
																	{:else if item.key === USAGE_BUCKET_LABEL.OUTPUT && (item.details?.thinkingTokens ?? 0) > 0}
																		<div class="text-muted-content mt-1 text-xs">
																			Thinking: {(
																				item.details?.thinkingTokens ?? 0
																			).toLocaleString()}
																			tokens
																		</div>
																	{/if}
																{/if}
															</div>
														</div>
													{/snippet}
												</StackedTimeline>
											{:else}
												<StackedTimeline
													start={startDate}
													end={endDate}
													data={item.timelineData}
													categoryKey="category"
													dateKey="date"
													primaryValueKey="inputTokens"
													secondaryValueKey="outputTokens"
													class="h-48"
													legend={{
														hideCategoryLabel: true,
														showSecondaryLabel: true,
														primaryLabel: CHART_LABEL.INPUT_TOKENS,
														secondaryLabel: CHART_LABEL.OUTPUT_TOKENS
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
															<div class="text-muted-content">{item.date}</div>
															<div class="tooltip-divider"></div>
														</div>
														<div class="flex flex-col gap-1">
															<div class="text-base-content flex flex-col">
																<div class="text-xl font-bold">{value.toLocaleString()}</div>
															</div>
														</div>
													{/snippet}
												</StackedTimeline>
											{/if}
										</div>
									</div>
								{/each}
							</div>
							{#if visibleChartCount < displayGraphItems.length}
								<div
									class="text-muted-content flex items-center justify-center gap-2 py-4 text-sm"
									aria-live="polite"
								>
									<Loading class="size-4 animate-spin" />
									<span>Loading charts… {visibleChartCount} of {displayGraphItems.length}</span>
								</div>
							{/if}
						{:else}
							<div class="text-muted-content mx-auto py-12 text-center text-sm font-light">
								No matches found.
							</div>
						{/if}
					</div>
				{:else}
					<div class="text-muted-content mx-auto py-12 text-sm font-light">No data available.</div>
				{/if}
			</div>
		</div>
	</div>
</Layout>

{#snippet subViewSortBySelect()}
	<Select
		class="md:bg-base-200 md:hover:bg-base-300 md:dark:bg-base-100 md:dark:hover:bg-base-200 mb-1.5 md:border md:border-transparent md:shadow-none md:w-64!"
		options={subViewSortByOptions}
		selected={subViewSortBy}
		onSelect={(option) => {
			subViewSortBy = option.id as UsageSubViewSortBy;
		}}
		id="sub-view-sort-by-select"
	/>
{/snippet}

{#snippet summary(title: string, value: number)}
	<div class="flex min-w-0 flex-1 flex-col gap-1 py-2">
		<div class="text-base-content text-xs font-light">{title}</div>
		<div class="text-primary flex items-center gap-1 text-xl font-semibold">
			{#if loadingTotalTokensData}
				<div class="py-2">
					<Loading class="size-4 animate-spin" />
				</div>
			{:else}
				{value.toLocaleString()}
			{/if}
		</div>
	</div>
{/snippet}

{#snippet spendSummary(title: string, value: number | undefined)}
	<div class="flex min-w-0 flex-1 flex-col gap-1 py-2">
		<div class="text-base-content text-xs font-light">{title}</div>
		<div class="text-primary flex items-center gap-1 text-xl font-semibold">
			{#if loadingTotalTokensData}
				<div class="py-2">
					<Loading class="size-4 animate-spin" />
				</div>
			{:else}
				{formatUSD(value ?? 0)}
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
		background-color: var(--color-base-400);
		margin-left: 1rem;
		margin-right: 1rem;
	}
</style>
