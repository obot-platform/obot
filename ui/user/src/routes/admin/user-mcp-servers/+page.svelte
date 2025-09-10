<script lang="ts">
	import Layout from '$lib/components/Layout.svelte';
	import Table from '$lib/components/Table.svelte';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants';
	import { AdminService } from '$lib/services';
	import type {
		OrgUser,
		WorkspaceCatalogEntry,
		WorkspaceCatalogServer
	} from '$lib/services/admin/types';
	import { Server } from 'lucide-svelte';
	import { fade, fly } from 'svelte/transition';
	import Search from '$lib/components/Search.svelte';
	import { formatTimeAgo } from '$lib/time';
	import { openUrl } from '$lib/utils';
	import { onMount } from 'svelte';

	function convertEntriesToTableData(
		entries: WorkspaceCatalogEntry[] | undefined,
		usersMap: Map<string, OrgUser>
	) {
		if (!entries) {
			return [];
		}

		return entries
			.filter((entry) => !entry.deleted)
			.map((entry) => {
				const owner = usersMap.get(entry.workspaceUserID);
				return {
					id: entry.id,
					name: entry.manifest?.name ?? '',
					icon: entry.manifest?.icon,
					source: entry.sourceURL || 'manual',
					data: entry,
					users: entry.userCount ?? 0,
					editable: !entry.sourceURL,
					type: entry.manifest.runtime === 'remote' ? 'remote' : 'single',
					created: entry.created,
					owner: owner?.displayName ?? owner?.email ?? owner?.username ?? 'Unknown',
					workspaceId: entry.workspaceID
				};
			});
	}

	function convertServersToTableData(
		servers: WorkspaceCatalogServer[] | undefined,
		usersMap: Map<string, OrgUser>
	) {
		if (!servers) {
			return [];
		}

		return servers
			.filter((server) => !server.catalogEntryID && !server.deleted)
			.map((server) => {
				const owner = usersMap.get(server.workspaceUserID);
				return {
					id: server.id,
					name: server.manifest.name ?? '',
					icon: server.manifest.icon,
					source: 'manual',
					type: 'multi',
					data: server,
					users: server.mcpServerInstanceUserCount ?? 0,
					editable: true,
					created: server.created,
					owner: owner?.displayName ?? owner?.email ?? owner?.username ?? 'Unknown',
					workspaceId: server.workspaceID
				};
			});
	}

	function convertEntriesAndServersToTableData(
		entries: WorkspaceCatalogEntry[],
		servers: WorkspaceCatalogServer[],
		usersMap: Map<string, OrgUser>
	) {
		const entriesTableData = convertEntriesToTableData(entries, usersMap);
		const serversTableData = convertServersToTableData(servers, usersMap);
		return [...entriesTableData, ...serversTableData];
	}

	let { data } = $props();
	let search = $state('');
	let users = $state<OrgUser[]>([]);

	onMount(() => {
		AdminService.listUsers().then((response) => {
			users = response;
		});
	});

	let totalCount = $derived(data.entries.length + data.servers.length);
	let usersMap = $derived(new Map(users.map((user) => [user.id, user])));
	let tableData = $derived(
		convertEntriesAndServersToTableData(data.entries, data.servers, usersMap)
	);
	let filteredTableData = $derived(
		tableData
			.filter((d) => d.name.toLowerCase().includes(search.toLowerCase()))
			.sort((a, b) => {
				return a.name.localeCompare(b.name);
			})
	);

	const duration = PAGE_TRANSITION_DURATION;
</script>

<Layout>
	<div class="flex flex-col gap-8 pt-4 pb-8" in:fade>
		{@render mainContent()}
	</div>
</Layout>

{#snippet mainContent()}
	<div
		class="flex flex-col gap-4 md:gap-8"
		in:fly={{ x: 100, delay: duration, duration }}
		out:fly={{ x: -100, duration }}
	>
		<div class="flex flex-col items-center justify-start md:flex-row md:justify-between">
			<h1 class="flex w-full items-center gap-2 text-2xl font-semibold">
				User Published MCP Servers
			</h1>
		</div>

		<div class="flex flex-col gap-2">
			<Search
				class="dark:bg-surface1 dark:border-surface3 border border-transparent bg-white shadow-sm"
				onChange={(val) => (search = val)}
				placeholder="Search servers..."
			/>

			{#if totalCount === 0}
				<div class="mt-12 flex w-md flex-col items-center gap-4 self-center text-center">
					<Server class="size-24 text-gray-200 dark:text-gray-900" />
					<h4 class="text-lg font-semibold text-gray-400 dark:text-gray-600">
						No User Published MCP Servers
					</h4>
				</div>
			{:else}
				<Table
					data={filteredTableData}
					fields={['name', 'type', 'users', 'source', 'created', 'owner']}
					onSelectRow={(d, isCtrlClick) => {
						const url =
							d.type === 'single' || d.type === 'remote'
								? `/admin/user-mcp-servers/${d.workspaceId}/c/${d.id}`
								: `/admin/user-mcp-servers/${d.workspaceId}/s/${d.id}`;
						openUrl(url, isCtrlClick);
					}}
					sortable={['name', 'type', 'users', 'source', 'created']}
					noDataMessage="No catalog servers added."
				>
					{#snippet onRenderColumn(property, d)}
						{#if property === 'name'}
							<div class="flex flex-shrink-0 items-center gap-2">
								<div
									class="bg-surface1 flex items-center justify-center rounded-sm p-0.5 dark:bg-gray-600"
								>
									{#if d.icon}
										<img src={d.icon} alt={d.name} class="size-6" />
									{:else}
										<Server class="size-6" />
									{/if}
								</div>
								<p class="flex items-center gap-1">
									{d.name}
								</p>
							</div>
						{:else if property === 'type'}
							{d.type === 'single' ? 'Single User' : d.type === 'multi' ? 'Multi-User' : 'Remote'}
						{:else if property === 'source'}
							{d.source === 'manual' ? 'Web Console' : d.source}
						{:else if property === 'created'}
							{formatTimeAgo(d.created).relativeTime}
						{:else}
							{d[property as keyof typeof d]}
						{/if}
					{/snippet}
				</Table>
			{/if}
		</div>
	</div>
{/snippet}

<svelte:head>
	<title>Obot | User Published MCP Servers</title>
</svelte:head>
