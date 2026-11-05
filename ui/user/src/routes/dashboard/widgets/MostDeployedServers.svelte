<script lang="ts">
	import Skeleton from '$lib/components/Skeleton.svelte';
	import GraphTitle from '$lib/components/graph/GraphTitle.svelte';
	import { m } from '$lib/i18n';
	import { stripMarkdownToText } from '$lib/markdown';
	import type { MCPCatalogEntry, MCPCatalogServer } from '$lib/services';
	import { getMCPDisplayName } from '$lib/services/user/mcp';
	import { resolve } from '$lib/url';
	import { ChevronRight, Server } from '@lucide/svelte';
	import { fade } from 'svelte/transition';

	type PopularServer = {
		entry?: MCPCatalogEntry;
		server?: MCPCatalogServer;
		count: number;
		id: string;
	};

	type Props = {
		loading: boolean;
		servers: PopularServer[];
		isBootStrapUser: boolean;
	};

	let { loading, servers, isBootStrapUser }: Props = $props();

	const graphContainerClasses = 'flex h-full flex-col gap-4 px-6 py-4';

	function getServerUrl(server: MCPCatalogServer) {
		return `/mcp-servers/s/${server.id}?view=server-instances`;
	}

	function getEntryUrl(entry: MCPCatalogEntry) {
		return `/mcp-servers/c/${entry.id}?view=server-instances`;
	}
</script>

<div in:fade={{ duration: 150 }} class="paper h-full gap-0 p-0">
	<GraphTitle title={m.dashboard_most_deployed_servers()} element="h4" />
	<div class={graphContainerClasses}>
		{#if loading}
			<Skeleton type="list" />
		{:else if servers.length > 0}
			<div class="-ml-2 flex w-[calc(100%+1rem)] flex-col gap-2">
				{#each servers as info (info.id)}
					{@const icon = 'server' in info ? info.server?.manifest.icon : info.entry?.manifest.icon}
					{@const displayName =
						'server' in info ? getMCPDisplayName(info.server) : info.entry?.manifest.name}
					{@const description =
						'server' in info ? info.server?.manifest.description : info.entry?.manifest.description}
					{@const url = info.server
						? getServerUrl(info.server)
						: info.entry
							? getEntryUrl(info.entry)
							: undefined}
					<a
						class="hover:bg-base-200 dark:hover:bg-base-300 flex w-full items-center gap-2 rounded-md px-2 py-1 transition-colors duration-150"
						href={url ? resolve(url as `/${string}`) : undefined}
					>
						{#if icon}
							<img
								src={icon}
								alt={info.id}
								class="bg-base-200 dark:bg-base-300 size-9 rounded-md p-1"
							/>
						{:else}
							<Server class="bg-base-200 size-9 rounded-md p-1 opacity-65" />
						{/if}
						<div class="flex max-w-[calc(100%-4.5rem)] grow flex-col gap-0.5">
							<p class="text-sm font-medium">{displayName}</p>
							{#if description}
								<p class="line-clamp-1 truncate text-xs font-light break-all">
									{stripMarkdownToText(description ?? '')}
								</p>
							{/if}
							<p class="text-muted-content text-xs italic">
								{m.dashboard_deployed_times({ count: info.count })}
							</p>
						</div>
						<ChevronRight class="size-5 shrink-0" />
					</a>
				{/each}
			</div>
		{:else}
			<p
				class="text-muted-content flex h-full grow items-center justify-center pt-2 text-center text-xs font-light"
			>
				{m.dashboard_no_servers_deployed()}
			</p>
		{/if}
		<div class="flex grow"></div>
		{#if servers.length > 0 && !isBootStrapUser}
			<a
				href={resolve('/mcp-servers')}
				class="bg-base-400/50 hover:bg-base-400 flex w-fit translate-x-2 items-center gap-1 self-end justify-end rounded-md px-2 py-0.5 text-[11px] transition-colors duration-200"
			>
				{m.dashboard_see_more()}
				<ChevronRight class="size-3" />
			</a>
		{/if}
	</div>
</div>
