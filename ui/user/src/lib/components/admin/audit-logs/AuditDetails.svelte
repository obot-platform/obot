<script lang="ts">
	import {
		ArrowDownWideNarrow,
		ArrowUpNarrowWide,
		ChevronsLeft,
		ChevronsRight,
		ListFilter,
		LoaderCircle
	} from 'lucide-svelte';
	import Table from '../../Table.svelte';
	import { type Snippet } from 'svelte';
	import {
		AdminService,
		type AuditLog,
		type AuditLogFilters,
		type AuditLogUsageStats,
		type OrgUser
	} from '$lib/services';
	import type { PaginatedResponse } from '$lib/services/admin/operations';
	import StatBar from '../StatBar.svelte';
	import Select from '../../Select.svelte';
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import LineGraph from '../../graph/LineGraph.svelte';
	import BarGraph from '../../graph/BarGraph.svelte';
	import { twMerge } from 'tailwind-merge';
	import Search from '../../Search.svelte';
	import { clickOutside } from '$lib/actions/clickoutside';
	import { dialogAnimation } from '$lib/actions/dialogAnimation';
	import AuditLogDetails from './AuditLogDetails.svelte';
	import AuditFilters from './AuditFilters.svelte';
	import { goto } from '$app/navigation';

	interface Props {
		mcpId?: string;
		mcpCatalogEntryId?: string;
		mcpServerDisplayName?: string;
		users: OrgUser[];
		filters?: AuditLogFilters;
		sort?: {
			sortBy: string;
			sortOrder: 'asc' | 'desc';
		};
		emptyContent?: Snippet;
		allowPagination?: boolean;
	}

	let {
		mcpId,
		mcpCatalogEntryId,
		mcpServerDisplayName,
		filters,
		sort = {
			sortBy: 'created_at',
			sortOrder: 'desc'
		},
		users,
		emptyContent,
		allowPagination
	}: Props = $props();
	let listUsageStats = $state<Promise<AuditLogUsageStats>>();
	let listAuditLogs = $state<Promise<PaginatedResponse<AuditLog>>>();

	let currentPage = $state(0);
	let limit = $state(100);
	let search = $state('');

	let usersMap = $derived(new Map(users.map((u) => [u.id, u])));
	let graphView = $state<'calls' | 'tools' | 'resources' | 'prompts'>('calls');

	let showFilters = $state(false);
	let selectedAuditLog = $state<AuditLog & { user: string }>();
	let rightSidebar = $state<HTMLDialogElement>();

	let selectedSortOption = $derived(`${sort.sortBy}_${sort.sortOrder}`);

	async function reload() {
		currentPage = 0;
		const offset = currentPage * limit;
		fetchLogsAndUsers(filters, offset, limit);
	}

	$effect(() => {
		if (mcpId || mcpCatalogEntryId || mcpServerDisplayName || filters || sort) {
			reload();
		}
	});

	async function fetchLogsAndUsers(filters?: AuditLogFilters, offset?: number, limit?: number) {
		const sortAndFilters = {
			...filters,
			...sort,
			offset,
			limit
		};

		if (mcpId) {
			listAuditLogs = AdminService.listServerOrInstanceAuditLogs(mcpId, sortAndFilters);
			listUsageStats = AdminService.listServerOrInstanceAuditLogStats(mcpId, {
				startTime: filters?.startTime ?? '',
				endTime: filters?.endTime ?? ''
			});
		} else {
			listAuditLogs = AdminService.listAuditLogs({
				...sortAndFilters,
				mcpServerCatalogEntryName: mcpCatalogEntryId,
				mcpServerDisplayName
			});
			listUsageStats = AdminService.listAuditLogUsageStats({
				startTime: filters?.startTime ?? '',
				endTime: filters?.endTime ?? '',
				mcpServerCatalogEntryName: mcpCatalogEntryId,
				mcpServerDisplayName
			});
		}
	}

	async function nextPage() {
		currentPage = currentPage + 1;
		const offset = currentPage * limit;
		fetchLogsAndUsers(filters, offset, limit);
	}

	async function prevPage() {
		currentPage = currentPage - 1;
		const offset = currentPage * limit;
		fetchLogsAndUsers(filters, offset, limit);
	}

	function compileCallsGraphData(auditLogs: AuditLog[]) {
		if (auditLogs.length === 0) return [];

		// Determine the time range
		const dates = auditLogs.map((log) => new Date(log.createdAt));
		const minDate = new Date(Math.min(...dates.map((d) => d.getTime())));
		const maxDate = new Date(Math.max(...dates.map((d) => d.getTime())));
		const timeRangeMs = maxDate.getTime() - minDate.getTime();
		const timeRangeHours = timeRangeMs / (1000 * 60 * 60);
		const timeRangeDays = timeRangeHours / 24;

		// Determine the appropriate bucket size based on time range
		let bucketKey: (date: Date) => string;

		if (timeRangeDays > 7) {
			// More than a week: bucket by days
			bucketKey = (date) => {
				return new Date(date.getFullYear(), date.getMonth(), date.getDate()).toISOString();
			};
		} else if (timeRangeHours > 24) {
			// More than a day but less than a week: bucket by hours
			bucketKey = (date) => {
				return new Date(
					date.getFullYear(),
					date.getMonth(),
					date.getDate(),
					date.getHours()
				).toISOString();
			};
		} else {
			// Less than a day: bucket by minutes
			bucketKey = (date) => {
				return new Date(
					date.getFullYear(),
					date.getMonth(),
					date.getDate(),
					date.getHours(),
					date.getMinutes()
				).toISOString();
			};
		}

		// Bucket the data
		const buckets = auditLogs.reduce<Record<string, number>>((acc, log) => {
			const date = new Date(log.createdAt);
			const key = bucketKey(date);
			acc[key] = (acc[key] || 0) + 1;
			return acc;
		}, {});

		const sortedDates = Object.keys(buckets).sort(
			(a, b) => new Date(a).getTime() - new Date(b).getTime()
		);

		// Convert to the required format with Date objects
		const results = sortedDates.map((dateKey) => ({
			date: new Date(dateKey),
			value: buckets[dateKey]
		}));

		return results;
	}

	function compileBarGraphData(
		usageStats: AuditLogUsageStats | undefined,
		view: 'tools' | 'resources' | 'prompts'
	): Array<{ [key: string]: string | number; count: number }> {
		if (!usageStats) return [];
		if (view === 'tools') {
			// Aggregate tool call statistics across all usage stats
			const toolCounts = new Map<string, number>();

			usageStats.items.forEach((stat) => {
				stat.toolCalls?.forEach((toolCall) => {
					const currentCount = toolCounts.get(toolCall.toolName) || 0;
					toolCounts.set(toolCall.toolName, currentCount + toolCall.callCount);
				});
			});

			const results = Array.from(toolCounts.entries()).map(([toolName, count]) => ({
				toolName,
				count
			}));
			return results;
		} else if (view === 'resources') {
			// Aggregate resource read statistics across all usage stats
			const resourceCounts = new Map<string, number>();

			usageStats.items.forEach((stat) => {
				stat.resourceReads?.forEach((resourceRead) => {
					const currentCount = resourceCounts.get(resourceRead.resourceUri) || 0;
					resourceCounts.set(resourceRead.resourceUri, currentCount + resourceRead.readCount);
				});
			});

			return Array.from(resourceCounts.entries()).map(([resourceUri, count]) => ({
				resourceUri,
				count
			}));
		} else if (view === 'prompts') {
			// Aggregate prompt read statistics across all usage stats
			const promptCounts = new Map<string, number>();

			usageStats.items.forEach((stat) => {
				stat.promptReads?.forEach((promptRead) => {
					const currentCount = promptCounts.get(promptRead.promptName) || 0;
					promptCounts.set(promptRead.promptName, currentCount + promptRead.readCount);
				});
			});

			return Array.from(promptCounts.entries()).map(([promptName, count]) => ({
				promptName,
				count
			}));
		}

		return [];
	}

	function handleRightSidebarClose() {
		rightSidebar?.close();
		setTimeout(() => {
			showFilters = false;
			selectedAuditLog = undefined;
		}, 300);
	}

	const views = [
		{ id: 'calls' as const, label: 'Calls' },
		{ id: 'tools' as const, label: 'Tools' },
		{ id: 'resources' as const, label: 'Resources' },
		{ id: 'prompts' as const, label: 'Prompts' }
	];
</script>

{#await listAuditLogs}
	<div class="flex w-full justify-center">
		<LoaderCircle class="size-6 animate-spin" />
	</div>
{:then auditLogsResponse}
	{#await listUsageStats}
		<div class="flex w-full justify-center">
			<LoaderCircle class="size-6 animate-spin" />
		</div>
	{:then usageResponse}
		{@const totalCalls = usageResponse?.totalCalls ?? 0}
		{@const uniqueUsers = usageResponse?.uniqueUsers ?? 0}
		<div class="flex flex-col gap-4">
			<div class="flex w-full items-center justify-between gap-4">
				<div class="flex items-center gap-4">
					<StatBar
						{totalCalls}
						{uniqueUsers}
						startTime={filters?.startTime ?? ''}
						endTime={filters?.endTime ?? ''}
					/>
				</div>
				<div
					class="border-surface3 flex h-fit items-center overflow-hidden rounded-full border text-sm"
				>
					{#each views as view, i (view.id)}
						<button
							onclick={() => {
								graphView = view.id;
							}}
							class={twMerge(
								'border-surface3 border-r py-2 text-center transition-colors duration-200',
								i === 0 && 'pr-6 pl-8',
								i > 0 && i < views.length - 1 && 'px-6',
								i === views.length - 1 && 'pr-8 pl-6',
								view.id === graphView && 'dark:bg-surface3 bg-white',
								view.id !== graphView && 'hover:bg-surface2 dark:hover:bg-surface2'
							)}
						>
							{view.label}
						</button>
					{/each}
				</div>
			</div>

			<div
				class="dark:border-surface3 dark:bg-surface1 mb-8 w-full rounded-md border border-transparent bg-white p-4 shadow-sm"
			>
				{#if graphView === 'calls'}
					{@const callsGraphData = compileCallsGraphData(auditLogsResponse?.items ?? [])}
					{#if callsGraphData.length > 0}
						{@const timeRange = (() => {
							const dates = auditLogsResponse?.items?.map((log) => new Date(log.createdAt)) ?? [];
							if (dates.length === 0) return 'minute';

							const minDate = new Date(Math.min(...dates.map((d) => d.getTime())));
							const maxDate = new Date(Math.max(...dates.map((d) => d.getTime())));
							const timeRangeMs = maxDate.getTime() - minDate.getTime();
							const timeRangeHours = timeRangeMs / (1000 * 60 * 60);
							const timeRangeDays = timeRangeHours / 24;

							if (timeRangeDays > 7) return 'day';
							if (timeRangeHours > 24) return 'hour';
							return 'minute';
						})()}
						<LineGraph
							data={callsGraphData}
							x="date"
							y="value"
							padding={16}
							formatTooltipText={(d) => `${d.value} requests`}
							formatXLabel={(d) => {
								const date = new Date(d);
								if (timeRange === 'day') {
									return date.toLocaleDateString(undefined, {
										month: 'short',
										day: 'numeric'
									});
								} else if (timeRange === 'hour') {
									return date.toLocaleString(undefined, {
										month: 'short',
										day: 'numeric',
										hour: 'numeric'
									});
								} else {
									return date.toLocaleTimeString(undefined, {
										hour: 'numeric',
										minute: 'numeric'
									});
								}
							}}
						/>
					{:else}
						<div class="text-muted-foreground flex h-full items-center justify-center">
							No data available
						</div>
					{/if}
				{:else}
					{@const graphData = compileBarGraphData(usageResponse, graphView)}
					{#if graphData.length > 0}
						{@const config = {
							tools: { x: 'toolName', tooltip: 'calls' },
							resources: { x: 'resourceUri', tooltip: 'reads' },
							prompts: { x: 'promptName', tooltip: 'reads' }
						}[graphView]}
						<BarGraph
							data={graphData}
							x={config.x}
							y="count"
							padding={16}
							formatTooltipText={(d) => `${d.count} ${config.tooltip}`}
						/>
					{:else}
						<div
							class="flex h-[300px] items-center justify-center text-sm font-light text-gray-400 dark:text-gray-600"
						>
							No data available
						</div>
					{/if}
				{/if}
			</div>
		</div>
	{/await}

	{@const auditLogs = (auditLogsResponse?.items ?? [])
		.map((auditLog) => {
			const user = usersMap.get(auditLog.userID);
			return {
				...auditLog,
				user: user?.email ?? user?.username ?? 'Unknown'
			};
		})
		.filter((auditLog) => {
			if (search) {
				const searchLower = search.toLowerCase();
				const { callIdentifier, callType, mcpServerDisplayName, client, user } = auditLog;
				if (
					callIdentifier?.toLowerCase().includes(searchLower) ||
					callType?.toLowerCase().includes(searchLower) ||
					mcpServerDisplayName?.toLowerCase().includes(searchLower) ||
					user?.toLowerCase().includes(searchLower) ||
					client?.name?.toLowerCase().includes(searchLower)
				) {
					return true;
				}
				return false;
			}
			return true;
		})}

	<div class="flex flex-col gap-2">
		<div class="flex items-center gap-2">
			<Search
				class="dark:bg-surface1 dark:border-surface3 border border-transparent bg-white shadow-sm"
				onChange={(val) => (search = val)}
				placeholder="Search..."
			/>
			<div class="flex items-center gap-2">
				<Select
					class="dark:border-surface3 dark:bg-surface1 min-h-12 border border-transparent bg-white shadow-sm hover:outline-2 hover:outline-blue-500"
					classes={{
						root: 'w-64',
						buttonContent: 'flex items-center gap-1'
					}}
					options={[
						{
							label: 'Latest',
							id: 'created_at_desc',
							property: 'created_at',
							order: 'desc'
						},
						{
							label: 'Oldest',
							id: 'created_at_asc',
							property: 'created_at',
							order: 'asc'
						},
						{
							label: 'Server Name - A to Z',
							id: 'mcp_server_display_name_asc',
							property: 'mcp_server_display_name',
							order: 'asc'
						},
						{
							label: 'Server Name - Z to A',
							id: 'mcp_server_display_name_desc',
							property: 'mcp_server_display_name',
							order: 'desc'
						},
						{
							label: 'Call Type - A to Z',
							id: 'call_type_asc',
							property: 'call_type',
							order: 'asc'
						},
						{
							label: 'Call Type - Z to A',
							id: 'call_type_desc',
							property: 'call_type',
							order: 'desc'
						},
						{
							label: 'Processing Time - Low to High',
							id: 'processing_time_ms_asc',
							property: 'processing_time_ms',
							order: 'asc'
						},
						{
							label: 'Processing Time - High to Low',
							id: 'processing_time_ms_desc',
							property: 'processing_time_ms',
							order: 'desc'
						}
					]}
					selected={selectedSortOption}
					onSelect={(option) => {
						const urlParams = new URLSearchParams(window.location.search);
						urlParams.set('sortBy', option.property as string);
						urlParams.set('sortOrder', option.order as string);
						goto(`${window.location.pathname}?${urlParams.toString()}`);
					}}
				>
					{#snippet buttonStartContent()}
						<span class="flex flex-shrink-0">
							{#if sort.sortOrder === 'desc'}
								<ArrowDownWideNarrow class="size-5" />
							{:else}
								<ArrowUpNarrowWide class="size-5" />
							{/if}
						</span>
					{/snippet}
				</Select>
			</div>
			<button
				class="icon-button flex-shrink-0"
				onclick={() => {
					showFilters = true;
					selectedAuditLog = undefined;
					rightSidebar?.show();
				}}
				use:tooltip={'Filter Logs'}
			>
				<ListFilter class="size-6 flex-shrink-0" />
			</button>
		</div>
		{#if auditLogs.length > 0}
			<Table
				data={auditLogs}
				fields={[
					'createdAt',
					'callIdentifier',
					'callType',
					'mcpServerDisplayName',
					'user',
					'client'
				]}
				headers={[
					{ property: 'createdAt', title: 'Timestamp' },
					{
						property: 'callIdentifier',
						title: 'Identifier'
					},
					{
						property: 'callType',
						title: 'Type'
					},
					{
						property: 'mcpServerDisplayName',
						title: 'Server'
					},
					{
						property: 'client',
						title: 'Client or IP'
					}
				]}
				noDataMessage="No audit logs."
				onSelectRow={(d) => {
					selectedAuditLog = d;
					showFilters = false;
					rightSidebar?.show();
				}}
			>
				{#snippet onRenderColumn(property, d)}
					{#if property === 'client'}
						{#if d.client}
							{d.client.name}{#if d.client.version}/{d.client.version}{/if}
						{/if}
						{d.clientIP}
					{:else if property === 'createdAt'}
						{new Date(d.createdAt)
							.toLocaleString(undefined, {
								year: 'numeric',
								month: 'short',
								day: 'numeric',
								hour: '2-digit',
								minute: '2-digit',
								second: '2-digit',
								hour12: true,
								timeZoneName: 'short'
							})
							.replace(/,/g, '')}
					{:else}
						{d[property as keyof typeof d]}
					{/if}
				{/snippet}
			</Table>
		{:else}
			{@render emptyContent?.()}
		{/if}
	</div>

	{#if allowPagination}
		{@const total = auditLogsResponse?.total ?? 0}
		<div
			class="bg-surface1 sticky right-0 bottom-0 left-0 mt-auto flex w-full items-center justify-end gap-2 p-4 dark:bg-black"
		>
			<span class="text-sm font-light">Results per page</span>
			<Select
				class="dark:border-surface3 border border-transparent bg-white shadow-inner dark:bg-black"
				classes={{
					root: 'w-22'
				}}
				options={[
					{ label: '25', id: 25 },
					{ label: '50', id: 50 },
					{ label: '100', id: 100 },
					{ label: '200', id: 200 }
				]}
				selected={limit}
				onSelect={(option) => {
					limit = option.id as number;
					reload();
				}}
			/>
			<div class="flex items-center gap-2">
				<button
					class="icon-button disabled:opacity-50"
					onclick={prevPage}
					disabled={currentPage === 0}
					use:tooltip={'Next Page'}
				>
					<ChevronsLeft class="size-5" />
				</button>
				<button
					class="icon-button disabled:opacity-50"
					onclick={nextPage}
					disabled={(currentPage + 1) * limit >= total}
					use:tooltip={'Previous Page'}
				>
					<ChevronsRight class="size-5" />
				</button>
			</div>
		</div>
	{:else if (mcpId || mcpCatalogEntryId || mcpServerDisplayName) && auditLogs.length > 0}
		{@const param = mcpId
			? 'mcpId=' + mcpId
			: mcpCatalogEntryId
				? 'entryId=' + mcpCatalogEntryId
				: 'name=' + mcpServerDisplayName}
		<p class="mt-2 text-sm font-light text-gray-400 dark:text-gray-600">
			See more usage details in the server's <a
				href={`/v2/admin/audit-logs?${param}`}
				class="text-link"
			>
				Audit Logs
			</a>.
		</p>
	{/if}

	<dialog
		bind:this={rightSidebar}
		use:clickOutside={[handleRightSidebarClose, true]}
		use:dialogAnimation={{ type: 'drawer' }}
		class="dark:border-surface1 dark:bg-surface1 fixed! top-0! right-0! bottom-0! left-auto! z-40 h-screen w-auto max-w-none rounded-none border-0 bg-white shadow-lg outline-none!"
	>
		{#if selectedAuditLog}
			<AuditLogDetails onClose={handleRightSidebarClose} auditLog={selectedAuditLog} />
		{/if}
		{#if showFilters}
			<AuditFilters {auditLogs} onClose={handleRightSidebarClose} {filters} />
		{/if}
	</dialog>
{/await}
