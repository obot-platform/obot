<script lang="ts">
	import { page } from '$app/state';
	import AuditLogCalendar from '$lib/components/admin/audit-logs/AuditLogCalendar.svelte';
	import Layout from '$lib/components/Layout.svelte';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants';
	import { subDays } from 'date-fns';
	import { Coins, LoaderCircle } from 'lucide-svelte';
	import { fade } from 'svelte/transition';
	import Select from '$lib/components/Select.svelte';
	import { getUserDisplayName } from '$lib/utils';
	import {
		AdminService,
		type Model,
		type OrgUser,
		type TokenUsage,
		type TotalTokenUsage
	} from '$lib/services';
	import { onMount } from 'svelte';
	import type { DateRange } from '$lib/components/Calendar.svelte';
	import VirtualizedGrid from './VirtualizedGrid.svelte';
	import { errors, responsive } from '$lib/stores';
	import { buildPaletteFromPrimary, hslToHex, parseColorToHsl } from '$lib/colors';
	import { buildStackedSeriesColors, getUserLabels } from './utils';
	import { twMerge } from 'tailwind-merge';
	import { goto } from '$lib/url';
	import StackedBarsChart, {
		type TooltipArg,
		type StackedBarsChartProps,
		type StackTooltipArg
	} from '$lib/components/charts/StackedBarsChart.svelte';

	let loadingTableData = $state(true);
	let loadingTotalTokensData = $state(true);
	let end = $derived(
		page.url.searchParams.get('end') ? new Date(page.url.searchParams.get('end')!) : new Date()
	);
	let start = $derived(
		page.url.searchParams.get('start')
			? new Date(page.url.searchParams.get('start')!)
			: subDays(end, 7)
	);
	const selectedModelIds = $derived(page.url.searchParams.getAll('model'));
	let filteredByModel = $derived(
		selectedModelIds.length > 0 ? selectedModelIds.join(',') : 'all_models'
	);
	const selectedUserIds = $derived(page.url.searchParams.getAll('user'));
	const selectedUserIdsForSelect = $derived(
		selectedUserIds.length > 0 ? selectedUserIds.join(',') : 'all_users'
	);
	let totalTokensData = $state<TotalTokenUsage>();
	let data = $state<TokenUsage[]>([]);
	/** Token usage rows use provider target model (e.g. "gpt-4o"); filter by selected models' targetModel. */
	const selectedTargetModels = $derived.by(() => {
		const ids = selectedModelIds.filter((id) => id !== 'all_models');
		if (ids.length === 0) return null;
		const modelsMap = new Map(modelsData.map((m) => [m.id, m]));
		// eslint-disable-next-line svelte/prefer-svelte-reactivity
		const targetModels = new Set<string>();
		for (const id of ids) {
			const model = modelsMap.get(id);
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
	let usersData = $state<OrgUser[]>([]);
	let modelsData = $state<Model[]>([]);
	let groupBy = $derived(
		(page.url.searchParams.get('group_by') as 'group_by_users' | 'group_by_models' | null) ??
			'group_by_default'
	);

	let primaryColorCss = $state<string | null>(null);
	$effect(() => {
		if (typeof document === 'undefined') return;
		primaryColorCss =
			getComputedStyle(document.documentElement).getPropertyValue('--color-primary').trim() || null;
	});

	let selectedSubview = $state<'models' | 'users'>('models');

	const colors = $derived.by(() => {
		const defaultPrimaryHex = '#4f7ef3';
		const fallbackHsl = parseColorToHsl(defaultPrimaryHex)!;

		const primary = primaryColorCss ? parseColorToHsl(primaryColorCss) : null;
		const hsl = primary ?? fallbackHsl;
		return buildPaletteFromPrimary(hsl);
	});

	/** Neutral gray for "other" users/models; re-runs when theme (primary) changes. */
	const othersColor = $derived.by(() => {
		if (typeof document === 'undefined') {
			return '#A9AABC';
		}
		if (primaryColorCss) {
			const gray =
				getComputedStyle(document.documentElement).getPropertyValue('--color-gray-500').trim() ||
				'';
			const parsed = gray ? parseColorToHsl(gray) : null;
			if (parsed) return hslToHex(parsed.h, Math.min(40, parsed.s), parsed.l);
		}
		return '#A9AABC';
	});

	const usersMap = $derived(new Map(usersData.map((u) => [u.id, u])));

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

	async function fetchData(start: Date, end: Date) {
		fetchAbortController?.abort();
		fetchAbortController = new AbortController();
		const signal = fetchAbortController.signal;

		loadingTableData = true;
		const timeRange = { start, end };
		AdminService.listTokenUsage(timeRange, { signal })
			.then((tokenUsage) => {
				if (signal.aborted) return;
				data = tokenUsage;
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
		if (start || end) {
			fetchData(start, end);
		}
	});
	const duration = PAGE_TRANSITION_DURATION;

	const preparedData = $derived.by(() => {
		return filteredData.flatMap((row) => {
			const user = row.userID ?? row.runName;

			return [
				{
					date: new Date(row.date),
					tokenType: 'input_tokens',
					tokenValue: row.promptTokens ?? 0,
					user,
					promptTokens: row.promptTokens ?? 0,
					completionTokens: row.completionTokens ?? 0,
					runName: row.runName,
					model: row.model
				},
				{
					date: new Date(row.date),
					tokenType: 'output_tokens',
					tokenValue: row.completionTokens ?? 0,
					user,
					promptTokens: row.promptTokens ?? 0,
					completionTokens: row.completionTokens ?? 0,
					runName: row.runName,
					model: row.model
				}
			];
		});
	});

	const targetModelToDisplayName = $derived(
		new Map(modelsData.map((m) => [m.targetModel, m.displayName || m.name]))
	);

	const colorsByUsers = $derived.by(() => {
		const uniqueUsers = [...new Set(preparedData.map((d) => d.user))].filter(Boolean);
		// buildStackedSeriesColors expects keys with _input_tokens/_output_tokens suffixes
		const mockRow: Record<string, unknown> = { bucket: 'mock' };
		for (const user of uniqueUsers) {
			mockRow[`${user}_input_tokens`] = 0;
			mockRow[`${user}_output_tokens`] = 0;
		}

		return buildStackedSeriesColors([mockRow], colors, othersColor).reduce(
			(acc, { key, color }) => {
				// Extract the user ID by removing the suffix
				const user = key.replace(/_input_tokens$|_output_tokens$/, '');
				acc[user] = color;
				return acc;
			},
			{} as Record<string, string>
		);
	});

	const colorsByModels = $derived.by(() => {
		const uniqueModels = [...new Set(preparedData.map((d) => d.model))].filter(Boolean);
		// buildStackedSeriesColors expects keys with _input_tokens/_output_tokens suffixes
		const mockRow: Record<string, unknown> = { bucket: 'mock' };
		for (const model of uniqueModels) {
			mockRow[`${model}_input_tokens`] = 0;
			mockRow[`${model}_output_tokens`] = 0;
		}

		return buildStackedSeriesColors([mockRow], colors, othersColor).reduce(
			(acc, { key, color }) => {
				// Extract the model name by removing the suffix
				const model = key.replace(/_input_tokens$|_output_tokens$/, '');
				acc[model] = color;
				return acc;
			},
			{} as Record<string, string>
		);
	});

	const perModelPromptData = $derived.by(() => {
		if (!filteredData.length) return [];
		//eslint-disable-next-line svelte/prefer-svelte-reactivity
		const byModel = new Map<string, typeof filteredData>();
		for (const r of filteredData) {
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
			return {
				modelKey: model,
				modelLabel: targetModelToDisplayName.get(model) ?? model,
				data: modelRows.flatMap((row) => [
					{ date: new Date(row.date), tokenType: 'input_tokens', value: row.promptTokens ?? 0 },
					{ date: new Date(row.date), tokenType: 'output_tokens', value: row.completionTokens ?? 0 }
				])
			};
		});
	});

	const perUserPromptData = $derived.by(() => {
		if (!filteredData.length) return [];
		//eslint-disable-next-line svelte/prefer-svelte-reactivity
		const byUser = new Map<string, typeof filteredData>();
		for (const r of filteredData) {
			const userKey = r.userID ?? r.runName ?? 'Unknown';
			let rows = byUser.get(userKey);
			if (!rows) {
				rows = [];
				byUser.set(userKey, rows);
			}
			rows.push(r);
		}
		const userKeys = [...byUser.keys()].sort();
		const userKeyToLabel = getUserLabels(usersMap, userKeys);
		return userKeys.map((userKey) => {
			const userRows = byUser.get(userKey)!;
			return {
				userKey,
				userLabel: userKeyToLabel.get(userKey) ?? userKey,
				data: userRows.flatMap((row) => [
					{ date: new Date(row.date), tokenType: 'input_tokens', value: row.promptTokens ?? 0 },
					{ date: new Date(row.date), tokenType: 'output_tokens', value: row.completionTokens ?? 0 }
				])
			};
		});
	});

	type GraphItem = {
		label: string;
		data: { date: Date; tokenType: string; value: number }[];
	};
	const graphItems = $derived.by((): GraphItem[] => {
		if (selectedSubview === 'models') {
			return perModelPromptData.map(({ modelLabel, data }) => ({ label: modelLabel, data }));
		}
		return perUserPromptData.map(({ userLabel, data }) => ({ label: userLabel, data }));
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

	function handleAddUserFilter(userId: string) {
		if (userId === 'all_users') {
			const currentUrl = new URL(page.url);
			currentUrl.searchParams.delete('user');
			goto(currentUrl, { noScroll: true, keepFocus: true });
			return;
		}
		const currentUrl = new URL(page.url);
		const users = currentUrl.searchParams.getAll('user');
		if (users.includes(userId)) return;
		users.push(userId);
		currentUrl.searchParams.delete('user');
		for (const id of users) {
			currentUrl.searchParams.append('user', id);
		}
		goto(currentUrl, { noScroll: true, keepFocus: true });
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

	function handleAddModelFilter(modelId: string) {
		if (modelId === 'all_models') {
			const currentUrl = new URL(page.url);
			currentUrl.searchParams.delete('model');
			goto(currentUrl, { noScroll: true, keepFocus: true });
			return;
		}
		const currentUrl = new URL(page.url);
		const models = currentUrl.searchParams.getAll('model');
		if (models.includes(modelId)) return;
		models.push(modelId);
		currentUrl.searchParams.delete('model');
		for (const id of models) {
			currentUrl.searchParams.append('model', id);
		}
		goto(currentUrl, { noScroll: true, keepFocus: true });
	}

	function handleGroupByChange(groupBy: string) {
		const currentUrl = new URL(page.url);
		currentUrl.searchParams.set('group_by', groupBy);
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

{#snippet defaultChartTooltip(arg: TooltipArg)}
	{@const categoryLabel =
		arg.category === 'input_tokens'
			? 'Input Tokens'
			: arg.category === 'output_tokens'
				? 'Output Tokens'
				: arg.category}

	<div class="text-md text-base-content/50 flex flex-col gap-1">
		{#if arg?.date}
			<div class="text-xs">
				{arg.date.toLocaleDateString(undefined, {
					year: 'numeric',
					month: 'short',
					day: 'numeric',
					hour: '2-digit',
					minute: '2-digit'
				})}
			</div>
		{/if}

		{#if arg?.category}
			<div class="text-base-content text-lg font-semibold">{categoryLabel}</div>
		{/if}

		{#if arg?.value !== undefined}
			<div class="text-base-content text-xl">
				{arg.value.toLocaleString()} tokens
			</div>
		{/if}
	</div>
{/snippet}

{#snippet groupByUsersSegmentTooltip(arg: TooltipArg)}
	{@const items = arg.group ?? []}
	{@const input = items.reduce((s, i) => s + (i.promptTokens ?? 0), 0)}
	{@const output = items.reduce((s, i) => s + (i.completionTokens ?? 0), 0)}
	{@const total = input + output}
	{@const userDisplayName =
		usersMap.get(arg.category ?? '')?.displayName ?? arg.category ?? 'Unknown'}

	<div class="text-md text-base-content/50 flex flex-col gap-1">
		{#if arg?.date}
			<div class="text-xs">
				{arg.date.toLocaleDateString(undefined, {
					year: 'numeric',
					month: 'short',
					day: 'numeric',
					hour: '2-digit',
					minute: '2-digit'
				})}
			</div>
		{/if}

		{#if arg?.category}
			<div class="text-base-content text-xl font-semibold">User: {userDisplayName}</div>
		{/if}

		{#if arg?.value !== undefined}
			<div class="">
				Total: <span class="text-base-content text-lg">{total.toLocaleString()}</span> tokens
			</div>
		{/if}

		{#if items.length > 0}
			<div class="text-md">
				Input: <span class="text-base-content">{input.toLocaleString()}</span> | Output:
				<span class="text-base-content">{output.toLocaleString()}</span>
			</div>
		{/if}
	</div>
{/snippet}

{#snippet groupByUsersStackTooltip(arg: StackTooltipArg)}
	<div class="flex flex-col gap-2">
		{#if arg?.date}
			<div class="text-xs">
				{arg.date.toLocaleDateString(undefined, {
					year: 'numeric',
					month: 'short',
					day: 'numeric',
					hour: '2-digit',
					minute: '2-digit'
				})}
			</div>
		{/if}
		<div class="text-base-content/50 flex flex-col gap-1">
			{#each arg.segments as segment}
				{@const userDisplayName =
					usersMap.get(segment.category)?.displayName ?? segment.category ?? 'Unknown'}
				{@const items = segment.group ?? []}
				{@const input = items.reduce((s, i) => s + (i.promptTokens ?? 0), 0)}
				{@const output = items.reduce((s, i) => s + (i.completionTokens ?? 0), 0)}

				<div class="flex flex-col gap-1">
					<div class="flex items-center gap-2">
						<div class="h-3 w-3 rounded-sm" style="background-color: {segment.color}"></div>
						<div class="text-base-content text-sm font-semibold">{userDisplayName}</div>
						<div class="ml-auto font-semibold">
							<span class="text-base-content">{segment.value.toLocaleString()}</span> tokens
						</div>
					</div>
					<div class="ml-5 text-xs">
						Input: <span class="text-base-content">{input.toLocaleString()}</span> | Output:
						<span class="text-base-content">{output.toLocaleString()}</span>
					</div>
				</div>
			{/each}
			<div class="mt-1 flex items-center gap-2 border-t pt-1">
				<div class="text-sm font-semibold">Total</div>
				<div class="ml-auto text-lg font-bold">
					<span class="text-base-content">{arg.total.toLocaleString()}</span> tokens
				</div>
			</div>
		</div>
	</div>
{/snippet}

{#snippet promptCompletionStackedGraph()}
	{@const [categoryAccessor, groupAccessor, colorScheme, segmentTooltip, stackTooltip] = (() => {
		type Datum = (typeof preparedData)[number];
		type ChartProps = StackedBarsChartProps<Datum>;

		type CategoryAccessor = ChartProps['categoryAccessor'];
		type GroupAccessor = ChartProps['groupAccessor'];
		type SegmentTooltip = ChartProps['segmentTooltip'];
		type StackTooltip = ChartProps['stackTooltip'];

		type Result = [
			CategoryAccessor,
			GroupAccessor,
			Record<string, string>?,
			SegmentTooltip?,
			StackTooltip?
		];

		if (groupBy === 'group_by_users')
			return [
				(row) => row.user,
				(items) => items.reduce((sum, item) => sum + (item.tokenValue ?? 0), 0),
				colorsByUsers,
				groupByUsersSegmentTooltip,
				groupByUsersStackTooltip
			] as Result;
		if (groupBy === 'group_by_models')
			return [
				(row) => row.model,
				(items) => items.reduce((sum, item) => sum + (item.tokenValue ?? 0), 0),
				colorsByModels,
				defaultChartTooltip,
				undefined
			] as Result;

		return [
			(row) => row.tokenType,
			(items) => items.reduce((sum, item) => sum + (item.tokenValue ?? 0), 0),
			undefined,
			defaultChartTooltip,
			undefined
		] as Result;
	})()}

	<StackedBarsChart
		{start}
		{end}
		data={preparedData}
		padding={{ top: 32, right: 16, bottom: 32, left: 48 }}
		dateAccessor={(row) => row.date}
		{categoryAccessor}
		{groupAccessor}
		{colorScheme}
		{segmentTooltip}
		{stackTooltip}
	/>
{/snippet}

<!-- ============================================================================ -->
<!-- PAGE LAYOUT & CONTENT                                                        -->
<!-- ============================================================================ -->

<Layout
	title="Token Usage"
	classes={{
		container: 'md:px-0 px-0 pt-0',
		childrenContainer: 'max-w-none',
		noSidebarTitle: 'pl-4 md:pl-8 mx-auto md:max-w-(--breakpoint-xl) pt-4'
	}}
>
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
					onSelect={(option) => handleAddUserFilter(option.id)}
					onClear={(option) => option && handleRemoveUserFilter(option.id)}
					id="user-select"
					multiple
				/>
				<Select
					class="dark:border-surface3 border border-transparent"
					classes={{
						root: 'w-full md:flex-1 dark:border-surface3'
					}}
					options={modelsOptions}
					selected={filteredByModel}
					onSelect={(option) => handleAddModelFilter(option.id)}
					onClear={(option) => option && handleRemoveModelFilter(option.id)}
					id="model-select"
					multiple
				/>
				<div class="bg-surface3 hidden h-8 w-0.5 md:block"></div>
				<AuditLogCalendar {start} {end} onChange={handleDateRangeChange} />
			</div>
			<div class="paper w-full gap-4">
				<div class="mb-1 flex items-center justify-between gap-4">
					<h4 class="flex items-center gap-2 self-start font-semibold">
						Prompt & Completion Tokens
						{#if loadingTableData}
							<LoaderCircle class="size-4 animate-spin" />
						{/if}
					</h4>
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

				<div class="relative h-[500px] w-full">
					{@render promptCompletionStackedGraph()}
				</div>
			</div>
		</div>

		<div
			class="m-auto flex w-full max-w-full flex-col gap-4 px-4 md:max-w-(--breakpoint-xl) md:px-8"
		>
			<div class="relative mt-2 flex flex-col">
				<div class="relative z-10 flex shrink-0 items-center">
					<button
						class={twMerge(
							'w-24 border-b-2 border-transparent px-4 py-2 transition-colors duration-400',
							selectedSubview === 'models' && 'border-primary'
						)}
						onclick={() => (selectedSubview = 'models')}
					>
						Models
					</button>
					<button
						class={twMerge(
							'w-24 border-b-2 border-transparent px-4 py-2 transition-colors duration-400',
							selectedSubview === 'users' && 'border-primary'
						)}
						onclick={() => (selectedSubview = 'users')}
					>
						Users
					</button>
				</div>
				<div class="bg-surface3 h-0.5 w-full shrink-0 -translate-y-0.5"></div>

				{#if graphItems.length > 0}
					<VirtualizedGrid class="my-4" data={graphItems} columns={2} rowHeight={340} overscan={2}>
						{#snippet children({ item })}
							<div class="paper flex min-h-0 flex-col">
								<h5 class="text-sm font-medium">{item.label}</h5>
								<div class="relative" style="height: {responsive.isMobile ? 210 : 240}px;">
									<StackedBarsChart
										{start}
										{end}
										data={item.data}
										padding={{ top: 16, right: 16, bottom: 32, left: 48 }}
										dateAccessor={(row) => row.date}
										categoryAccessor={(row) => {
											if (groupBy === ' group_by_users') return row.user;

											return row.tokenType;
										}}
										groupAccessor={(items) =>
											items.reduce((sum, item) => sum + (item.value ?? 0), 0)}
										segmentTooltip={defaultChartTooltip}
									/>
								</div>
							</div>
						{/snippet}
					</VirtualizedGrid>
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
					<LoaderCircle class="size-4 animate-spin" />
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
