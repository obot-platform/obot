<script lang="ts">
	import { resolve } from '$app/paths';
	import Layout from '$lib/components/Layout.svelte';
	import HorizontalBarGraph from '$lib/components/graph/HorizontalBarGraph.svelte';
	import { COMMON_AI_CLIENTS } from '$lib/constants.js';
	import { formatNumber } from '$lib/format';
	import { stripMarkdownToText } from '$lib/markdown';
	import { UserService, type MCPCatalogServer } from '$lib/services';
	import type {
		AvgToolCallResponseTimeRow,
		TopServerUsageRow,
		TopToolCallRow
	} from '$lib/services/dashboard/types';
	import {
		avgToolCallResponseTimeFromStats,
		topServersFromStats,
		topToolCallsFromStats
	} from '$lib/services/dashboard/utils';
	import { compileAvailableMcpServers, getMCPDisplayName } from '$lib/services/user/mcp';
	import { darkMode, errors, mcpServersAndEntries } from '$lib/stores';
	import { subMonths } from 'date-fns';
	import { ChevronRight, Server, Wrench } from 'lucide-svelte';
	import { onMount } from 'svelte';
	import { fade } from 'svelte/transition';
	import { twMerge } from 'tailwind-merge';

	let { data } = $props();
	let deviceScans = $derived(data.deviceScans);
	let latestDeviceScan = $derived(deviceScans?.items?.[0]);

	let loadingToolUsage = $state(true);

	let topToolCalls = $state<TopToolCallRow[]>([]);
	let topServerUsage = $state<TopServerUsageRow[]>([]);
	let avgToolCallResponseTime = $state<AvgToolCallResponseTimeRow[]>([]);
	let maxToolsToShow = 5;
	let maxServersToShow = 12;

	const end = new Date();
	const start = subMonths(end, 1);

	const serverAndEntries = $derived(mcpServersAndEntries.current);
	const sortedUserConfiguredServers = $derived(
		[...compileAvailableMcpServers([], serverAndEntries.userConfiguredServers)]
			.sort((a, b) => new Date(b.created).getTime() - new Date(a.created).getTime())
			.filter((server) => !server.id.startsWith('nba1'))
	);

	let deviceScanClients = $derived(
		(latestDeviceScan?.clients ?? []).map((client) => {
			const match = COMMON_AI_CLIENTS.find((c) =>
				client.name.toLowerCase().startsWith(c.id.toLowerCase())
			);
			return {
				...client,
				icon: match?.icon,
				iconDark: match?.iconDark
			};
		})
	);

	onMount(async () => {
		UserService.listAuditLogUsageStats({
			start_time: start.toISOString(),
			end_time: end.toISOString()
		})
			.then((stats) => {
				const statsToUse = (stats.items ?? []).filter(
					(s) =>
						!s.mcpID.startsWith('sms1') &&
						!s.mcpServerDisplayName.startsWith('nba1') &&
						!s.mcpServerDisplayName.startsWith('Obot ')
				);
				const adjustedStats = {
					...stats,
					items: statsToUse
				};
				topToolCalls = topToolCallsFromStats(adjustedStats);
				topServerUsage = topServersFromStats(adjustedStats);
				avgToolCallResponseTime = avgToolCallResponseTimeFromStats(adjustedStats);
			})
			.catch((error) => {
				if (error?.name === 'AbortError') return;
				errors.append(error);
			})
			.finally(() => {
				loadingToolUsage = false;
			});
	});

	function getUrl(item: MCPCatalogServer) {
		if ('manifest' in item) {
			return item.catalogEntryID && item.serverUserType === 'singleUser'
				? `/mcp-servers/c/${item.catalogEntryID}`
				: `/mcp-servers/s/${item.id}`;
		}
	}
</script>

<Layout title="Dashboard" classes={{ childrenContainer: 'max-w-none', container: '' }}>
	<div class="@container grid min-w-0 w-full max-w-full grid-cols-12 gap-4">
		<div class="flex flex-col gap-4 col-span-12">
			{#if latestDeviceScan}
				<div class="grid grid-cols-12 gap-4">
					<div class="col-span-12 @3xl:col-span-6">
						{@render recentlyCreatedServers()}
					</div>
					<div class="col-span-12 @3xl:col-span-6">
						<div class="paper gap-2 h-full">
							<div class="flex items-center justify-between gap-2 flex-wrap">
								<h4 class="font-semibold">Latest Device Scan</h4>
								<div class="flex items-center gap-2 text-xs">
									<div class="badge badge-secondary badge-sm">
										{latestDeviceScan.deviceID.slice(0, 12)}
									</div>
									<div class="badge badge-soft badge-primary badge-sm">
										{latestDeviceScan.os}/{latestDeviceScan.arch}
									</div>
								</div>
							</div>

							<div class="flex flex-col gap-2">
								<p class="text-sm font-medium">Device Clients</p>
								<ul class="flex flex-wrap gap-2">
									{#each deviceScanClients as client (client.name)}
										<li class="flex gap-2 items-center">
											<div class="tooltip" data-tip={client.name}>
												{#if darkMode.isDark && client.iconDark}
													<img src={client.iconDark} alt={client.name} class="size-5" />
												{:else}
													<img src={client.icon} alt={client.name} class="size-5" />
												{/if}
											</div>
										</li>
									{/each}
								</ul>
							</div>

							<div class="divider my-0"></div>

							<div class="flex flex-col gap-2">
								<p class="text-sm font-medium">Scan Overview</p>
								<div class="stats bg-base-200 dark:bg-base-300 shadow-inner">
									{#each ['mcpServers', 'skills', 'plugins'] as stat, i (i)}
										{@const property = latestDeviceScan[stat as keyof typeof latestDeviceScan] ?? 0}
										<div class="stat">
											<div class="stat-title capitalize">
												{stat === 'mcpServers' ? 'MCP Servers' : stat}
											</div>
											<div class="stat-value">
												{Array.isArray(property) ? property.length : 0}
											</div>
										</div>
									{/each}
								</div>

								<a
									href={resolve(`/devices/${latestDeviceScan.deviceID}`)}
									class="text-[11px] self-end bg-base-400/50 transition-colors duration-200 hover:bg-base-400 rounded-md py-0.5 w-fit px-2 flex items-center gap-1 mt-2"
								>
									See More <ChevronRight class="size-3" />
								</a>
							</div>
						</div>
					</div>
				</div>
			{:else}
				{@render recentlyCreatedServers()}
			{/if}
			{#if loadingToolUsage}
				<div class="skeleton rounded-md h-[400px]"></div>
			{:else}
				<div in:fade={{ duration: 150 }} class="paper gap-1 w-full min-h-72">
					<div class="flex flex-wrap items-center justify-between gap-4">
						<h4 class="flex items-center gap-1 font-semibold">
							Top Servers Used <span class="text-muted-content text-xs font-light"
								>(Last 30 Days)</span
							>
						</h4>
					</div>
					<HorizontalBarGraph
						data={topServerUsage.slice(0, maxServersToShow)}
						labelKey="serverName"
						valueKey="count"
						formatValue={(value) => Math.round(value).toString()}
						class="h-[400px]"
					>
						{#snippet tooltipContent(item)}
							<div class="flex flex-col gap-0 text-xs">
								<div class="text-muted-content text-xs">{item.label}</div>
							</div>
							<div class="text-base-content font-semibold">
								{item.value} calls
							</div>
						{/snippet}
					</HorizontalBarGraph>
				</div>
			{/if}
		</div>
		<div class="col-span-12 grid grid-cols-12 gap-4">
			{@render popularTools()}
			{@render toolAverageResponseTime()}
		</div>
	</div>
</Layout>

{#snippet recentlyCreatedServers()}
	<div
		in:fade={{ duration: 150 }}
		class={twMerge('paper gap-1 ', latestDeviceScan ? 'h-full' : '')}
	>
		<h4 class="flex items-center gap-2 font-semibold">Recently Connected Servers</h4>
		{#if mcpServersAndEntries.current.loading}
			<div class="pt-2 flex flex-col gap-4">
				{#each Array.from({ length: 5 }) as _, i (i)}
					<div class="flex gap-2 items-center w-full">
						<div class="size-8 rounded-md skeleton shrink-0"></div>
						<div class="flex flex-col gap-2 flex-1">
							<div class="h-4 w-full rounded-md skeleton"></div>
							<div class="h-3 w-full rounded-md skeleton"></div>
						</div>
					</div>
				{/each}
			</div>
		{:else if sortedUserConfiguredServers.length > 0}
			<div class="pt-2 flex flex-col gap-2">
				{#each sortedUserConfiguredServers.slice(0, 5) as server (server.id)}
					{@const url = server ? getUrl(server) : undefined}
					{#if server && url}
						<a
							class="flex gap-2 items-center dark:hover:bg-base-300 hover:bg-base-200 transition-colors duration-150 -mx-2 px-2 py-1 rounded-md"
							href={url ? resolve(url as `/${string}`) : undefined}
						>
							{@render serverItem(server)}
						</a>
					{:else if server}
						<div class="flex gap-2 items-center -mx-2 px-2 py-1 rounded-md">
							{@render serverItem(server)}
						</div>
					{/if}
				{/each}
			</div>
		{:else}
			<p
				class="text-xs text-muted-content pt-2 font-light text-center h-full flex items-center justify-center"
			>
				No servers have been deployed yet.
			</p>
		{/if}
	</div>
{/snippet}

{#snippet serverItem(server: MCPCatalogServer)}
	{@const icon = server?.manifest.icon}
	{@const displayName = getMCPDisplayName(server)}
	{@const description = server?.manifest.description}
	{#if icon}
		<img
			src={icon}
			alt={`${displayName} icon`}
			class="size-9 bg-base-200 dark:bg-base-300 rounded-md p-1"
		/>
	{:else}
		<Server class="size-9 opacity-65 bg-base-200 rounded-md p-1" />
	{/if}
	<div class="flex flex-col gap-0.5 max-w-[calc(100%-4.5rem)] grow">
		<p class="text-sm font-medium">{displayName}</p>
		{#if description}
			<p class="text-xs truncate line-clamp-1 break-all font-light">
				{stripMarkdownToText(description ?? '')}
			</p>
		{/if}
	</div>
	<ChevronRight class="size-5 shrink-0" />
{/snippet}

{#snippet popularTools()}
	<div class="paper gap-1 col-span-12 flex flex-col @3xl:col-span-6 min-h-72">
		<h4 class="flex items-center gap-2 font-semibold mb-1">
			Recently Popular Tools
			<span class="text-muted-content text-xs font-light">(Last 30 Days)</span>
		</h4>
		{#if loadingToolUsage}
			<div class="pt-2 flex flex-col gap-4 w-full">
				{#each Array.from({ length: maxToolsToShow }) as _, i (i)}
					<div class="flex gap-2 items-center w-full">
						<div class="size-8 rounded-md skeleton shrink-0"></div>
						<div class="flex flex-col gap-2 flex-1">
							<div class="h-4 w-full rounded-md skeleton"></div>
							<div class="h-3 w-full rounded-md skeleton"></div>
						</div>
					</div>
				{/each}
			</div>
		{:else if topToolCalls.length === 0}
			<p
				class="text-xs text-muted-content pt-2 font-light grow flex items-center justify-center h-full text-center"
			>
				No recent tool calls.
			</p>
		{:else}
			<ul class="pt-2 flex flex-col gap-2">
				{#each topToolCalls.slice(0, maxToolsToShow) as row (row.compositeKey)}
					<li class="flex gap-2 items-center">
						<div
							class="size-8 items-center justify-center shrink-0 bg-base-200 dark:bg-base-300 rounded-md p-1"
						>
							<Wrench class="size-6 opacity-65 shrink-0" />
						</div>
						<div class="flex flex-col gap-1 min-w-0">
							<p class="text-sm font-medium truncate">
								{row.toolLabel.split('.').slice(1).join('.') || row.compositeKey}
							</p>
							<p class="text-xs text-muted-content">
								{formatNumber(row.count)} calls · {row.serverDisplayName}
							</p>
						</div>
					</li>
				{/each}
			</ul>
		{/if}
		{#if !latestDeviceScan}
			<div class="flex grow min-h-0"></div>
		{/if}
		{#if topToolCalls.length > 0}
			<a
				href={resolve('/admin/usage')}
				class="text-[11px] translate-x-2 self-end bg-base-400/50 transition-colors duration-200 hover:bg-base-400 rounded-md py-0.5 w-fit px-2 flex items-center gap-1 mt-2"
			>
				See More <ChevronRight class="size-3" />
			</a>
		{/if}
	</div>
{/snippet}

{#snippet toolAverageResponseTime()}
	<div class="paper gap-1 col-span-12 flex flex-col @3xl:col-span-6 min-h-72 h-full">
		<h4 class="flex items-center gap-2 font-semibold mb-1">
			Tool Call Average Response Time
			<span class="text-muted-content text-xs font-light">(Last 30 Days)</span>
		</h4>
		{#if loadingToolUsage}
			<div class="pt-2 flex flex-col gap-4 w-full">
				{#each Array.from({ length: maxToolsToShow }) as _, i (i)}
					<div class="flex gap-2 items-center w-full">
						<div class="flex flex-col gap-2 flex-1">
							<div class="h-4 w-full rounded-md skeleton"></div>
							<div class="h-3 w-full rounded-md skeleton"></div>
						</div>
					</div>
				{/each}
			</div>
		{:else if avgToolCallResponseTime.length === 0}
			<p
				class="text-xs text-muted-content pt-2 font-light grow flex items-center justify-center h-full text-center"
			>
				No recent tool calls.
			</p>
		{:else}
			<div class="pt-2 flex flex-col gap-4 w-full">
				<ul class="flex flex-col gap-2">
					{#each avgToolCallResponseTime.slice(0, maxToolsToShow) as row (row.toolName)}
						<li class="flex gap-2 items-center">
							<div class="flex flex-col gap-1 min-w-0 grow pr-4">
								<p class="text-sm font-medium truncate">
									{row.toolName.split('.').slice(1).join('.')}
								</p>
								<p class="text-xs text-muted-content">
									{row.serverDisplayName}
								</p>
							</div>
							<div class="text-sm">
								{row.averageResponseTimeMs.toFixed(2)}ms
							</div>
						</li>
					{/each}
				</ul>
			</div>
		{/if}
		{#if !latestDeviceScan}
			<div class="flex grow min-h-0"></div>
		{/if}
		{#if avgToolCallResponseTime.length > 0}
			<a
				href={resolve('/admin/usage')}
				class="text-[11px] translate-x-2 self-end bg-base-400/50 transition-colors duration-200 hover:bg-base-400 rounded-md py-0.5 w-fit px-2 flex items-center gap-1 mt-2"
			>
				See More <ChevronRight class="size-3" />
			</a>
		{/if}
	</div>
{/snippet}

<svelte:head>
	<title>Obot | Dashboard</title>
</svelte:head>

<style lang="postcss">
	.skeleton-list-item .skeleton {
		:global(.dark) & {
			background-image: linear-gradient(
				105deg,
				var(--color-base-300) 0% 40%,
				var(--color-base-400) 50%,
				var(--color-base-300) 60% 100%
			);
		}
	}
</style>
