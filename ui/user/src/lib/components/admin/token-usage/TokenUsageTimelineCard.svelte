<script lang="ts">
	import Select from '$lib/components/Select.svelte';
	import StackedTimeline from '$lib/components/graph/StackedTimeline.svelte';
	import Loading from '$lib/icons/Loading.svelte';
	import {
		AdminService,
		UserService,
		type Model,
		type OrgUser,
		type TokenUsage
	} from '$lib/services';
	import { errors, responsive } from '$lib/stores';
	import {
		DEFAULT_TOKEN_GROUP_BY,
		DEFAULT_TOKEN_TYPE,
		TOKEN_GROUP_BY,
		TOKEN_GROUP_BY_OPTIONS,
		TOKEN_TYPE,
		TOKEN_TYPE_OPTIONS,
		USAGE_BUCKET_LABEL,
		type TokenGroupBy,
		type TokenType
	} from './constants';
	import {
		buildMainChartData,
		formatTokenUsageUSD,
		mainChartTooltipValueKeys,
		mainChartUsesSpendBuckets,
		mainPrimaryLabel,
		mainPrimaryValueKey,
		targetModelToDisplayNameMap,
		TIMELINE_AGGREGATE_THRESHOLD,
		type TokenUsageTimelineItem
	} from './tokenUsageTimeline';
	import { twMerge } from 'tailwind-merge';

	type Props = {
		startDate: Date;
		endDate: Date;
		data?: TokenUsage[];
		loading?: boolean;
		users?: OrgUser[];
		models?: Model[];
		selectedTokenType?: TokenType;
		groupBy?: TokenGroupBy;
		onTokenTypeChange?: (tokenType: TokenType) => void;
		onGroupByChange?: (groupBy: string) => void;
		class?: string;
	};

	let {
		startDate,
		endDate,
		data: externalData,
		loading: externalLoading = false,
		users: externalUsers = [],
		models: externalModels = [],
		selectedTokenType: externalTokenType,
		groupBy: externalGroupBy,
		onTokenTypeChange,
		onGroupByChange,
		class: className
	}: Props = $props();

	let internalTokenType = $state<TokenType>(DEFAULT_TOKEN_TYPE);
	let internalGroupBy = $state<TokenGroupBy>(DEFAULT_TOKEN_GROUP_BY);
	let fetchedData = $state<TokenUsage[]>([]);
	let fetchedUsers = $state<OrgUser[]>([]);
	let fetchedModels = $state<Model[]>([]);
	let fetchLoading = $state(false);

	const selfFetch = $derived(externalData === undefined);
	const data = $derived(externalData ?? fetchedData);
	const users = $derived(externalUsers.length > 0 ? externalUsers : fetchedUsers);
	const models = $derived(externalModels.length > 0 ? externalModels : fetchedModels);
	const loading = $derived(selfFetch ? fetchLoading : externalLoading);

	const selectedTokenType = $derived(externalTokenType ?? internalTokenType);
	const groupBy = $derived(externalGroupBy ?? internalGroupBy);

	const usersMap = $derived(new Map(users.map((u) => [u.id, u])));
	const modelToDisplayName = $derived(targetModelToDisplayNameMap(models));

	const usesSpendBuckets = $derived(mainChartUsesSpendBuckets(selectedTokenType, groupBy));
	const primaryValueKey = $derived(mainPrimaryValueKey(selectedTokenType, usesSpendBuckets));
	const primaryLabel = $derived(mainPrimaryLabel(selectedTokenType, usesSpendBuckets));
	const tooltipValueKeys = $derived(mainChartTooltipValueKeys(usesSpendBuckets));

	let mainChartData = $state<TokenUsageTimelineItem[]>([]);

	let fetchAbortController: AbortController | null = null;

	async function fetchData(start: Date, end: Date) {
		fetchAbortController?.abort();
		fetchAbortController = new AbortController();
		const signal = fetchAbortController.signal;

		fetchLoading = true;
		const timeRange = { start, end };

		try {
			const [tokenUsage, usersList, modelsList] = await Promise.all([
				AdminService.listTokenUsage(timeRange, { signal }),
				externalUsers.length > 0
					? Promise.resolve(externalUsers)
					: UserService.listUsersIncludeDeleted(),
				externalModels.length > 0
					? Promise.resolve(externalModels)
					: AdminService.listModels({ all: true })
			]);
			if (signal.aborted) return;
			fetchedData = tokenUsage;
			if (externalUsers.length === 0) fetchedUsers = usersList;
			if (externalModels.length === 0) fetchedModels = modelsList;
		} catch (error) {
			if ((error as Error)?.name === 'AbortError') return;
			errors.append(error);
		} finally {
			if (!signal.aborted) fetchLoading = false;
		}
	}

	$effect(() => {
		if (!selfFetch) return;
		fetchData(startDate, endDate);
		return () => fetchAbortController?.abort();
	});

	$effect(() => {
		const filtered = data;
		const group = groupBy;
		const tokenType = selectedTokenType;
		const start = startDate;
		const end = endDate;
		const usersLookup = usersMap;
		const modelToName = modelToDisplayName;
		const threshold = TIMELINE_AGGREGATE_THRESHOLD;

		if (filtered.length <= threshold) {
			mainChartData = buildMainChartData(
				filtered,
				group,
				tokenType,
				start,
				end,
				usersLookup,
				modelToName
			);
			return;
		}

		const schedule =
			typeof requestIdleCallback !== 'undefined'
				? (fn: () => void) => requestIdleCallback(fn, { timeout: 150 })
				: (fn: () => void) => setTimeout(fn, 0);
		schedule(() => {
			mainChartData = buildMainChartData(
				filtered,
				group,
				tokenType,
				start,
				end,
				usersLookup,
				modelToName
			);
		});
	});

	function handleTokenTypeChange(tokenType: TokenType) {
		if (onTokenTypeChange) {
			onTokenTypeChange(tokenType);
			return;
		}
		internalTokenType = tokenType;
	}

	function handleGroupByChange(nextGroupBy: string) {
		if (onGroupByChange) {
			onGroupByChange(nextGroupBy);
			return;
		}
		internalGroupBy = nextGroupBy as TokenGroupBy;
	}
</script>

<div class={twMerge('paper w-full gap-0 pt-4', className)}>
	<div class="mb-1 flex flex-wrap justify-between gap-2">
		<div class="flex flex-wrap items-center gap-4">
			<h4 class="flex items-center gap-2 font-semibold">
				Token Usage
				{#if loading}
					<Loading class="size-4 animate-spin" />
				{/if}
			</h4>

			{#if !responsive.isMobile}
				<div class="flex shrink-0">
					<button
						class={twMerge(
							'btn btn-secondary bg-base-300 dark:hover:bg-base-400 border-base-300 rounded-r-none! border border-r-0 text-xs',
							selectedTokenType !== TOKEN_TYPE.INPUT &&
								'bg-base-100 dark:bg-base-200 hover:bg-base-400 '
						)}
						onclick={() => handleTokenTypeChange(TOKEN_TYPE.INPUT)}
					>
						Input Tokens
					</button>
					<button
						class={twMerge(
							'btn btn-secondary bg-base-300 dark:hover:bg-base-400 border-base-300 rounded-none! border border-r-0 text-xs',
							selectedTokenType !== TOKEN_TYPE.OUTPUT &&
								'bg-base-100 dark:bg-base-200 hover:bg-base-400'
						)}
						onclick={() => handleTokenTypeChange(TOKEN_TYPE.OUTPUT)}
					>
						Output Tokens
					</button>
					<button
						class={twMerge(
							'btn btn-secondary bg-base-300 dark:hover:bg-base-400 border-base-300 rounded-l-none! border text-xs',
							selectedTokenType !== TOKEN_TYPE.SPEND &&
								'bg-base-100 dark:bg-base-200 hover:bg-base-400'
						)}
						onclick={() => handleTokenTypeChange(TOKEN_TYPE.SPEND)}
					>
						Spend
					</button>
				</div>
			{/if}
		</div>
		{#if responsive.isMobile}
			<Select
				class="bg-base-300 dark:bg-base-100 dark:border-base-400 w-full border border-transparent shadow-inner md:w-64"
				classes={{
					root: 'w-full md:w-64'
				}}
				options={TOKEN_TYPE_OPTIONS}
				selected={selectedTokenType}
				onSelect={(option) => handleTokenTypeChange(option.id as TokenType)}
			/>
		{/if}
		<Select
			class="bg-base-300 dark:bg-base-100 dark:border-base-400 w-full border border-transparent shadow-inner md:w-64"
			classes={{
				root: 'w-full md:w-64'
			}}
			options={TOKEN_GROUP_BY_OPTIONS}
			selected={groupBy ?? DEFAULT_TOKEN_GROUP_BY}
			onSelect={(option) => handleGroupByChange(option.id)}
		/>
	</div>
	<div class="w-full pt-2">
		{#key `${groupBy}-${selectedTokenType}`}
			<StackedTimeline
				start={startDate}
				end={endDate}
				data={mainChartData}
				dateKey="date"
				{primaryValueKey}
				{tooltipValueKeys}
				categoryKey="category"
				class="h-96"
				legend={{
					showSecondaryLabel: false,
					primaryLabel: groupBy === TOKEN_GROUP_BY.DEFAULT ? primaryLabel : '',
					hideCategoryLabel: groupBy === TOKEN_GROUP_BY.DEFAULT && !usesSpendBuckets
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
							{#if selectedTokenType === TOKEN_TYPE.SPEND}
								<div class="text-xl font-bold">{formatTokenUsageUSD(value)}</div>
								{#if usesSpendBuckets}
									{#if item.key === USAGE_BUCKET_LABEL.INPUT}
										<div class="text-muted-content mt-1 text-xs">
											Cache read: {formatTokenUsageUSD(item.details?.cacheReadSpend ?? 0)}
										</div>
										<div class="text-muted-content text-xs">
											Cache write: {formatTokenUsageUSD(item.details?.cacheWriteSpend ?? 0)}
										</div>
									{:else if item.key === USAGE_BUCKET_LABEL.OUTPUT && (item.details?.thinkingTokens ?? 0) > 0}
										<div class="text-muted-content mt-1 text-xs">
											Thinking: {(item.details?.thinkingTokens ?? 0).toLocaleString()} tokens
										</div>
									{/if}
								{:else}
									<div class="text-muted-content mt-1 text-xs">
										Input: {formatTokenUsageUSD(item.details?.inputSpend ?? 0)}
									</div>
									<div class="text-muted-content text-xs">
										Output: {formatTokenUsageUSD(item.details?.outputSpend ?? 0)}
									</div>
									<div class="text-muted-content text-xs">
										Cache read: {formatTokenUsageUSD(item.details?.cacheReadSpend ?? 0)}
									</div>
									<div class="text-muted-content text-xs">
										Cache write: {formatTokenUsageUSD(item.details?.cacheWriteSpend ?? 0)}
									</div>
								{/if}
							{:else}
								{@const spend =
									selectedTokenType === TOKEN_TYPE.INPUT
										? (item.details?.inputSpend ?? 0)
										: (item.details?.outputSpend ?? 0)}
								<div class="text-xl font-bold">{value.toLocaleString()}</div>
								<div class="text-muted-content text-xs">{formatTokenUsageUSD(spend)}</div>
								{#if selectedTokenType === TOKEN_TYPE.INPUT}
									<div class="text-muted-content mt-1 text-xs">
										Cache read: {(item.details?.cacheReadTokens ?? 0).toLocaleString()} tokens,
										{formatTokenUsageUSD(item.details?.cacheReadSpend ?? 0)}
									</div>
									<div class="text-muted-content text-xs">
										Cache write: {(item.details?.cacheWriteTokens ?? 0).toLocaleString()} tokens,
										{formatTokenUsageUSD(item.details?.cacheWriteSpend ?? 0)}
									</div>
								{:else if (item.details?.thinkingTokens ?? 0) > 0}
									<div class="text-muted-content mt-1 text-xs">
										Thinking: {(item.details?.thinkingTokens ?? 0).toLocaleString()} tokens
									</div>
								{/if}
							{/if}
						</div>
					</div>
				{/snippet}
			</StackedTimeline>
		{/key}
	</div>
</div>
