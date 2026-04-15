<script lang="ts">
	import { resolve } from '$app/paths';
	import Layout from '$lib/components/Layout.svelte';
	import DonutGraph from '$lib/components/graph/DonutGraph.svelte';
	import StackedTimeline from '$lib/components/graph/StackedTimeline.svelte';
	import Loading from '$lib/icons/Loading.svelte';
	import {
		AdminService,
		type OrgUser,
		type TokenUsage,
		type TokenUsageWithCategory
	} from '$lib/services';
	import { errors } from '$lib/stores';
	import { aggregateTimelineDataByBucket, getUserLabels } from '../token-usage/utils';
	import { isWithinInterval, subDays } from 'date-fns';
	import { Activity, ChevronRight, Server, Users, Wrench } from 'lucide-svelte';
	import { onMount } from 'svelte';
	import { twMerge } from 'tailwind-merge';

	let loadingTableData = $state(true);
	let usersData = $state<OrgUser[]>([]);
	let selectedTokenType = $derived<'input' | 'output'>('input');
	let data = $state<TokenUsage[]>([]);

	let mainChartData = $state<TokenUsageWithCategory[]>([]);
	let usersMap = $derived(new Map(usersData.map((user) => [user.id, user])));

	const DEFER_DATA_THRESHOLD = 400;
	const TIMELINE_AGGREGATE_THRESHOLD = 500;
	const end = new Date();
	const start = subDays(end, 7);

	onMount(async () => {
		usersData = await AdminService.listUsersIncludeDeleted();

		const timeRange = { start, end };

		AdminService.listTokenUsage(timeRange)
			.then((tokenUsage) => {
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
					data = tokenUsage;
				});
			})
			.finally(() => {
				loadingTableData = false;
			})
			.catch((error) => {
				if (error?.name === 'AbortError') return;
				errors.append(error);
			});
	});

	function computeMainTimelineData(
		filtered: TokenUsage[],
		group: string,
		users: Map<string, OrgUser>
	): TokenUsageWithCategory[] {
		const userKeys = [...new Set(filtered.map((r) => r.userID ?? r.runName ?? 'Unknown'))].sort();
		const userKeyToLabel = getUserLabels(users, userKeys);
		return filtered.map((r) => ({
			...r,
			date: r.date,
			promptTokens: r.promptTokens ?? 0,
			completionTokens: r.completionTokens ?? 0,
			totalTokens: r.totalTokens ?? (r.promptTokens ?? 0) + (r.completionTokens ?? 0),
			category:
				userKeyToLabel.get(r.userID ?? r.runName ?? 'Unknown') ?? r.userID ?? r.runName ?? 'Unknown'
		}));
	}

	$effect(() => {
		if (data.length <= TIMELINE_AGGREGATE_THRESHOLD) {
			const timeline = computeMainTimelineData(data, 'group_by_users', usersMap);
			mainChartData = timeline;
			return;
		}

		const schedule =
			typeof requestIdleCallback !== 'undefined'
				? (fn: () => void) => requestIdleCallback(fn, { timeout: 150 })
				: (fn: () => void) => setTimeout(fn, 0);
		schedule(() => {
			const timeline = computeMainTimelineData(data, 'group_by_users', usersMap);
			if (timeline.length <= TIMELINE_AGGREGATE_THRESHOLD) return timeline;
			return aggregateTimelineDataByBucket(timeline, start, end) as TokenUsageWithCategory[];
		});
	});
</script>

<Layout title="Dashboard" classes={{ childrenContainer: 'max-w-none', container: '' }}>
	<div class="grid grid-cols-12 gap-4">
		<div class="flex flex-col md:col-span-8 col-span-12 gap-4">
			<!-- this token usage graph-->
			<div class="grid grid-cols-12 gap-4">
				<div class="md:col-span-4 col-span-12">
					<div class="paper gap-2">
						<div class="text-xs text-on-surface1">Total Users</div>
						<div class="flex w-full justify-between">
							<div class="text-3xl font-semibold">{usersData.length}</div>
							<Users class="size-8 text-primary" />
						</div>
						<a
							href={resolve('/admin/users')}
							class="text-[11px] bg-surface3 rounded-md py-0.5 w-fit px-2 flex items-center gap-1"
						>
							See All Users <ChevronRight class="size-3" />
						</a>
					</div>
				</div>
				<div class="md:col-span-4 col-span-12">
					<div class="paper gap-2">
						<div class="text-xs text-on-surface1">Monthly Active Users</div>
						<div class="flex w-full justify-between">
							<div class="text-3xl font-semibold">
								{usersData.filter(
									(user) =>
										user.lastActiveDay && isWithinInterval(user.lastActiveDay, { start, end })
								).length}
							</div>
							<Activity class="size-8 text-primary" />
						</div>
						<div class="text-xs text-on-surface1">Jan 1st - Feb 1st</div>
					</div>
				</div>
				<div class="md:col-span-4 col-span-12">
					<div class="paper gap-2">
						<div class="text-xs text-on-surface1">Active Server Connections</div>
						<div class="flex w-full justify-between">
							<div class="text-3xl font-semibold">100</div>
							<Server class="size-8 text-primary" />
						</div>
						<a
							href={resolve('/admin/mcp-servers?view=deployments')}
							class="text-[11px] bg-surface3 rounded-md py-0.5 w-fit px-2 flex items-center gap-1"
						>
							See All Deployments <ChevronRight class="size-3" />
						</a>
					</div>
				</div>
			</div>
			<div class="paper w-full min-h-72">
				<div class="flex flex-wrap items-center justify-between gap-4">
					<h4 class="flex items-center gap-2 font-semibold">
						Token Usage <span class="text-on-surface1 text-xs font-light">(Last 7 Days)</span>
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
							onclick={() => (selectedTokenType = 'input')}
						>
							Input Tokens
						</button>
						<button
							class={twMerge(
								'button-secondary rounded-l-none border text-xs',
								selectedTokenType === 'output' && 'bg-surface2 border-surface2'
							)}
							onclick={() => (selectedTokenType = 'output')}
						>
							Output Tokens
						</button>
					</div>
				</div>
				{#if !loadingTableData}
					<StackedTimeline
						{start}
						{end}
						data={mainChartData}
						dateKey="date"
						primaryValueKey={selectedTokenType === 'input' ? 'promptTokens' : 'completionTokens'}
						categoryKey="category"
						class="h-72"
						legend={{
							showSecondaryLabel: false,
							primaryLabel: selectedTokenType === 'input' ? 'input tokens' : 'output tokens'
						}}
					>
						{#snippet tooltipContent(item)}
							{@const value = item.primaryTotal ?? 0}
							<div class="flex flex-col gap-0 text-xs">
								<div class="text-sm font-light">{item.key}</div>
								<div class="text-on-surface1">{item.date}</div>
								<div class="divider"></div>
							</div>
							<div class="flex flex-col gap-1">
								<div class="text-on-background flex flex-col">
									<div class="text-xl font-bold">{value.toLocaleString()}</div>
								</div>
							</div>
						{/snippet}
					</StackedTimeline>
				{/if}
			</div>

			<div class="grid grid-cols-12 gap-4">
				<div class="paper gap-1 md:col-span-6 col-span-12">
					<h4 class="flex items-center gap-2 font-semibold">Most Popular Servers</h4>
					<ul class="py-2 flex flex-col gap-2">
						<li class="flex gap-2 items-center">
							<Server class="size-8 opacity-65" />
							<div class="flex flex-col gap-1">
								<p class="text-sm font-medium">Server Name</p>
								<p class="text-xs text-on-surface1">100 calls</p>
							</div>
						</li>
						<li class="flex gap-2 items-center">
							<Server class="size-8 opacity-65" />
							<div class="flex flex-col gap-1">
								<p class="text-sm font-medium">Server Name</p>
								<p class="text-xs text-on-surface1">100 calls</p>
							</div>
						</li>
						<li class="flex gap-2 items-center">
							<Server class="size-8 opacity-65" />
							<div class="flex flex-col gap-1">
								<p class="text-sm font-medium">Server Name</p>
								<p class="text-xs text-on-surface1">100 calls</p>
							</div>
						</li>
						<li class="flex gap-2 items-center">
							<Server class="size-8 opacity-65" />
							<div class="flex flex-col gap-1">
								<p class="text-sm font-medium">Server Name</p>
								<p class="text-xs text-on-surface1">100 calls</p>
							</div>
						</li>
						<li class="flex gap-2 items-center">
							<Server class="size-8 opacity-65" />
							<div class="flex flex-col gap-1">
								<p class="text-sm font-medium">Server Name</p>
								<p class="text-xs text-on-surface1">100 calls</p>
							</div>
						</li>
					</ul>
				</div>
				<div class="paper gap-1 md:col-span-6 col-span-12">
					<h4 class="flex items-center gap-2 font-semibold">Most Popular Tools</h4>
					<ul class="py-2 flex flex-col gap-2">
						<li class="flex gap-2 items-center">
							<Wrench class="size-8 opacity-65" />
							<div class="flex flex-col gap-1">
								<p class="text-sm font-medium">Tool Name</p>
								<p class="text-xs text-on-surface1">100 calls</p>
							</div>
						</li>
						<li class="flex gap-2 items-center">
							<Wrench class="size-8 opacity-65" />
							<div class="flex flex-col gap-1">
								<p class="text-sm font-medium">Tool Name</p>
								<p class="text-xs text-on-surface1">100 calls</p>
							</div>
						</li>
						<li class="flex gap-2 items-center">
							<Wrench class="size-8 opacity-65" />
							<div class="flex flex-col gap-1">
								<p class="text-sm font-medium">Tool Name</p>
								<p class="text-xs text-on-surface1">100 calls</p>
							</div>
						</li>
						<li class="flex gap-2 items-center">
							<Wrench class="size-8 opacity-65" />
							<div class="flex flex-col gap-1">
								<p class="text-sm font-medium">Tool Name</p>
								<p class="text-xs text-on-surface1">100 calls</p>
							</div>
						</li>
						<li class="flex gap-2 items-center">
							<Wrench class="size-8 opacity-65" />
							<div class="flex flex-col gap-1">
								<p class="text-sm font-medium">Tool Name</p>
								<p class="text-xs text-on-surface1">100 calls</p>
							</div>
						</li>
					</ul>
				</div>
			</div>
		</div>
		<div class="md:col-span-4 col-span-12">
			<div class="paper">
				<h4 class="font-semibold mb-1">Deployed Servers by Type</h4>

				<DonutGraph
					class="h-72"
					donutRatio={0.65}
					data={[
						{
							label: 'Remote',
							value: 25
						},
						{
							label: 'Single-User',
							value: 37
						},
						{
							label: 'Multi-User',
							value: 8
						}
					]}
				/>
			</div>
		</div>
	</div>
</Layout>
