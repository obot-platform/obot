<script lang="ts">
	import { page } from '$app/state';
	import type { DateRange } from '$lib/components/Calendar.svelte';
	import Layout from '$lib/components/Layout.svelte';
	import AuditLogCalendar from '$lib/components/admin/audit-logs/AuditLogCalendar.svelte';
	import AuditLogsPageContent from '$lib/components/admin/audit-logs/AuditLogsPageContent.svelte';
	import LlmAuditLogsContent from '$lib/components/admin/audit-logs/LlmAuditLogsContent.svelte';
	import TokenUsageTimelineCard from '$lib/components/admin/token-usage/TokenUsageTimelineCard.svelte';
	import { TOKEN_USAGE_PARAMS } from '$lib/components/admin/token-usage/constants';
	import { formatTokenUsageUSD as formatUSD } from '$lib/components/admin/token-usage/tokenUsageTimeline';
	import VirtualPageRoot from '$lib/components/ui/virtual-page/virtual-page-viewport.svelte';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants';
	import { isAbortError } from '$lib/errors';
	import Loading from '$lib/icons/Loading.svelte';
	import {
		AdminService,
		UserService,
		type Model,
		type OrgUser,
		type TokenUsage,
		type TotalTokenUsage
	} from '$lib/services';
	import { errors, profile } from '$lib/stores';
	import { goto } from '$lib/url';
	import { Captions } from '@lucide/svelte';
	import { isValid, subDays } from 'date-fns';
	import { onMount, type Component } from 'svelte';
	import { fly } from 'svelte/transition';
	import { twMerge } from 'tailwind-merge';

	type LogTab = 'mcp' | 'llm';

	let { data } = $props();
	let { apiKey, apiKeyId, isAdmin } = $derived(data);
	let duration = PAGE_TRANSITION_DURATION;

	const scopeApiKeyId = $derived(apiKey?.id != null ? String(apiKey.id) : '');
	const canViewLlmLogs = $derived(Boolean(profile.current.hasAdminAccess?.()));
	const activeTab = $derived.by<LogTab>(() => {
		if (!canViewLlmLogs) return 'mcp';
		return page.url.searchParams.get('tab') === 'llm' ? 'llm' : 'mcp';
	});

	let loadingTableData = $state(true);
	let usersData = $state<OrgUser[]>([]);
	let modelsData = $state<Model[]>([]);
	let dataRows = $state<TokenUsage[]>([]);

	let end = $derived(page.url.searchParams.get(TOKEN_USAGE_PARAMS.END));
	let start = $derived(page.url.searchParams.get(TOKEN_USAGE_PARAMS.START));
	let lastStart = $state<string | null>(null);
	let lastEnd = $state<string | null>(null);

	function parseDateParam(value: string | null): Date | undefined {
		if (!value) return undefined;
		const parsed = new Date(value);
		return isValid(parsed) ? parsed : undefined;
	}

	let endDate = $derived(parseDateParam(end) ?? new Date());
	let startDate = $derived(parseDateParam(start) ?? subDays(endDate, 7));

	const filteredData = $derived(
		scopeApiKeyId
			? dataRows.filter((row) => row.apiKeyID != null && String(row.apiKeyID) === scopeApiKeyId)
			: []
	);

	const totalTokensData = $derived.by<TotalTokenUsage>(() => {
		return filteredData.reduce(
			(acc, row) => ({
				totalTokens: acc.totalTokens + (row.totalTokens ?? 0),
				inputTokens: (acc.inputTokens ?? 0) + (row.inputTokens ?? 0),
				outputTokens: (acc.outputTokens ?? 0) + (row.outputTokens ?? 0),
				cacheReadTokens: (acc.cacheReadTokens ?? 0) + (row.cacheReadTokens ?? 0),
				cacheWriteTokens: (acc.cacheWriteTokens ?? 0) + (row.cacheWriteTokens ?? 0),
				totalSpend: (acc.totalSpend ?? 0) + (row.totalSpend ?? 0)
			}),
			{
				totalTokens: 0,
				inputTokens: 0,
				outputTokens: 0,
				cacheReadTokens: 0,
				cacheWriteTokens: 0,
				totalSpend: 0
			}
		);
	});

	const views = $derived([
		{
			value: 'mcp' as const,
			label: 'MCP Server Logs'
		},
		...(canViewLlmLogs
			? [
					{
						value: 'llm' as const,
						label: 'LLM Gateway Logs'
					}
				]
			: [])
	] satisfies { value: LogTab; label: string }[]);

	onMount(async () => {
		try {
			const [users, models] = await Promise.all([
				UserService.listUsersIncludeDeleted(),
				isAdmin ? AdminService.listModels({ all: true }) : UserService.listModels()
			]);
			usersData = users;
			modelsData = models;
		} catch (error) {
			errors.append(error);
		}
	});

	let fetchAbortController: AbortController | null = null;

	const DEFER_DATA_THRESHOLD = 400;
	async function fetchData(rangeStart: Date, rangeEnd: Date) {
		fetchAbortController?.abort();
		fetchAbortController = new AbortController();
		const signal = fetchAbortController.signal;
		const userId = apiKey?.userId;

		loadingTableData = true;
		const timeRange = { start: rangeStart.toISOString(), end: rangeEnd.toISOString() };

		const tokenUsagePromise =
			isAdmin && userId == null
				? AdminService.listTokenUsage(timeRange, { signal })
				: userId != null
					? AdminService.listTokenUsageForUser(String(userId), timeRange, { signal })
					: Promise.resolve([]);

		tokenUsagePromise
			.then((tokenUsage) => {
				if (signal.aborted) return;
				if (tokenUsage.length <= DEFER_DATA_THRESHOLD) {
					dataRows = tokenUsage;
					return;
				}
				// Defer so the UI can paint before heavy derivation. Safari lacks requestIdleCallback.
				const schedule =
					typeof requestIdleCallback !== 'undefined'
						? (fn: () => void) => requestIdleCallback(fn, { timeout: 120 })
						: (fn: () => void) => setTimeout(fn, 0);
				schedule(() => {
					if (!signal.aborted) dataRows = tokenUsage;
				});
			})
			.finally(() => {
				if (!signal.aborted) loadingTableData = false;
			})
			.catch((error) => {
				if (isAbortError(error)) return;
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
		return () => fetchAbortController?.abort();
	});

	function handleDateRangeChange(range: DateRange) {
		const currentUrl = new URL(page.url);
		currentUrl.searchParams.set(TOKEN_USAGE_PARAMS.START, range.start?.toISOString() ?? '');
		currentUrl.searchParams.set(TOKEN_USAGE_PARAMS.END, range.end?.toISOString() ?? '');
		goto(currentUrl, { noScroll: true, keepFocus: true, replaceState: true });
	}

	function selectTab(tab: LogTab) {
		const url = new URL(page.url);
		if (tab === 'mcp') {
			url.searchParams.delete('tab');
		} else {
			url.searchParams.set('tab', tab);
		}
		goto(url, { noScroll: true, keepFocus: true, replaceState: true });
	}
</script>

<Layout
	title={`${apiKey?.name || 'Agent Auth Scope'} | ${apiKeyId}`}
	showBackButton
	classes={{ childrenContainer: 'max-w-none', container: 'pb-0' }}
	main={{
		component: VirtualPageRoot as unknown as Component,
		props: { class: '', as: 'main', itemHeight: 56, overscan: 5 }
	}}
>
	{#snippet rightNavActions()}
		<AuditLogCalendar start={startDate} end={endDate} onChange={handleDateRangeChange} />
	{/snippet}
	<div
		class="flex h-full w-full flex-col gap-8"
		in:fly={{ x: 100, duration }}
		out:fly={{ x: -100, duration }}
	>
		<section class="flex flex-col gap-4">
			<div class="paper flex flex-col flex-wrap items-stretch gap-4 p-4 md:flex-row">
				{@render summary('Total', totalTokensData.totalTokens ?? 0)}
				<div class="divider-horizontal hidden md:block"></div>
				{@render summary('Input', totalTokensData.inputTokens ?? 0)}
				<div class="divider-horizontal hidden md:block"></div>
				{@render summary('Output', totalTokensData.outputTokens ?? 0)}
				<div class="divider-horizontal hidden md:block"></div>
				{@render summary(
					'Cached Input',
					(totalTokensData.cacheReadTokens ?? 0) + (totalTokensData.cacheWriteTokens ?? 0)
				)}
				<div class="divider-horizontal hidden md:block"></div>
				{@render spendSummary('Spend', totalTokensData.totalSpend)}
			</div>
			<TokenUsageTimelineCard
				{startDate}
				{endDate}
				data={filteredData}
				loading={loadingTableData}
				users={usersData}
				models={modelsData}
			/>
		</section>

		<section class="flex min-h-0 flex-1 flex-col gap-4 pb-8">
			<div class="flex flex-1 flex-col">
				<div class="flex flex-1 relative z-10">
					{#each views as viewOption (viewOption.value)}
						<button
							id={`devices-tab-${viewOption.value}`}
							class={twMerge(
								'border-b-2 text-nowrap border-transparent px-8 py-2 transition-colors duration-400',
								activeTab === viewOption.value
									? 'border-primary'
									: 'hover:border-primary/25 text-muted-content hover:text-base-content'
							)}
							onclick={() => selectTab(viewOption.value)}
						>
							{viewOption.label}
						</button>
					{/each}
				</div>
				<div class="bg-base-400 h-0.5 w-full shrink-0 -translate-y-0.5"></div>
			</div>

			{#if activeTab === 'mcp'}
				<AuditLogsPageContent
					apiKeyId={scopeApiKeyId || null}
					startTime={startDate}
					endTime={endDate}
				>
					{#snippet emptyContent()}
						<div
							class="flex w-full flex-col items-center justify-center gap-4 px-6 py-16 text-center"
						>
							<Captions class="text-muted-content size-20 opacity-50" />
							<h4 class="text-muted-content text-lg font-semibold">No MCP server logs</h4>
							<p class="text-muted-content max-w-md text-sm font-light">
								There are no MCP server logs for this API key in the selected range.
							</p>
						</div>
					{/snippet}
				</AuditLogsPageContent>
			{:else if canViewLlmLogs}
				<LlmAuditLogsContent
					apiKeyId={scopeApiKeyId || null}
					startTime={startDate}
					endTime={endDate}
				/>
			{/if}
		</section>
	</div>
</Layout>

{#snippet summary(title: string, value: number)}
	<div class="flex min-w-0 flex-1 flex-col gap-1 py-2">
		<div class="text-base-content text-xs font-light">{title}</div>
		<div class="text-primary flex items-center gap-1 text-xl font-semibold">
			{#if loadingTableData}
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
			{#if loadingTableData}
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
	<title>Obot | {apiKey?.name || 'Agent Auth Scope'} | {apiKeyId}</title>
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
