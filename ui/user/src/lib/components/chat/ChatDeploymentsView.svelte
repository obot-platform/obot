<script lang="ts">
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import Table from '$lib/components/table/Table.svelte';
	import {
		ChatService,
		type MCPCatalogEntry,
		type MCPCatalogServer,
		type OrgUser,
		type MCPServerInstance
	} from '$lib/services';
	import { getServerTypeLabel, requiresUserUpdate } from '$lib/services/chat/mcp';
	import { profile, mcpServersAndEntries } from '$lib/stores';
	import { formatTimeAgo } from '$lib/time';
	import { getUserDisplayName } from '$lib/utils';
	import { CircleFadingArrowUp, LoaderCircle, Server, StepForward } from 'lucide-svelte';
	import { onMount } from 'svelte';

	interface Props {
		usersMap?: Map<string, OrgUser>;
		classes?: {
			tableHeader?: string;
		};
		query?: string;
		onSelect?: (args: {
			server: MCPCatalogServer;
			instance?: MCPServerInstance;
			entry?: MCPCatalogEntry;
		}) => void;
	}

	let { usersMap = new Map(), query, classes, onSelect }: Props = $props();
	let loading = $state(false);

	let serversData = $derived(
		mcpServersAndEntries.current.userConfiguredServers.filter((server) => !server.deleted)
	);

	let instances = $state<MCPServerInstance[]>([]);
	let instancesMap = $derived(
		new Map(instances.map((instance) => [instance.mcpServerID, instance]))
	);
	let tableRef = $state<ReturnType<typeof Table>>();

	let entriesMap = $derived(
		mcpServersAndEntries.current.entries.reduce<Record<string, MCPCatalogEntry>>((acc, entry) => {
			acc[entry.id] = entry;
			return acc;
		}, {})
	);

	let compositeMapping = $derived(
		serversData
			.filter((server) => 'compositeConfig' in server.manifest)
			.reduce<Record<string, MCPCatalogServer>>((acc, server) => {
				acc[server.id] = server;
				return acc;
			}, {})
	);

	let tableData = $derived.by(() => {
		function isCompositeDescendantDisabled(parent: MCPCatalogServer, id: string) {
			const match = parent.manifest.compositeConfig?.componentServers.find(
				(component) => component.catalogEntryID === id || component.mcpServerID === id
			);
			return match ? match.disabled : false;
		}

		const transformedData = serversData
			.map((deployment) => {
				const powerUserWorkspaceID =
					deployment.powerUserWorkspaceID ||
					(deployment.catalogEntryID
						? entriesMap[deployment.catalogEntryID]?.powerUserWorkspaceID
						: undefined);
				const powerUserID = deployment.catalogEntryID
					? entriesMap[deployment.catalogEntryID]?.powerUserID
					: powerUserWorkspaceID
						? deployment.userID
						: undefined;

				const compositeParent =
					deployment.compositeName && compositeMapping[deployment.compositeName];
				const compositeParentName = compositeParent
					? compositeParent.alias || compositeParent.manifest.name
					: '';
				return {
					...deployment,
					displayName: deployment.alias || deployment.manifest.name || '',
					userName: getUserDisplayName(usersMap, deployment.userID),
					registry: powerUserID ? getUserDisplayName(usersMap, powerUserID) : 'Global Registry',
					type: getServerTypeLabel(deployment),
					powerUserWorkspaceID,
					compositeParentName,
					disabled: compositeParent
						? isCompositeDescendantDisabled(
								compositeParent,
								deployment.catalogEntryID || deployment.mcpCatalogID || deployment.id
							)
						: false,
					isMyServer:
						(deployment.catalogEntryID && deployment.userID === profile.current.id) ||
						powerUserID === profile.current.id
				};
			})
			.filter((d) => !d.disabled && d.isMyServer);

		return query
			? transformedData.filter((d) => d.displayName.toLowerCase().includes(query.toLowerCase()))
			: transformedData;
	});

	onMount(() => {
		reload(true);
	});

	async function reload(isInitialLoad: boolean = false) {
		loading = true;
		mcpServersAndEntries.refreshAll();
		instances = await ChatService.listMcpServerInstances();
		loading = false;
	}
</script>

<div class="flex flex-col gap-2">
	{#if loading || mcpServersAndEntries.current.loading}
		<div class="my-2 flex items-center justify-center">
			<LoaderCircle class="size-6 animate-spin" />
		</div>
	{:else}
		<Table
			bind:this={tableRef}
			data={tableData}
			classes={{
				root: 'rounded-none rounded-b-md shadow-none',
				thead: classes?.tableHeader || 'top-31'
			}}
			fields={['displayName', 'type', 'deploymentStatus', 'created']}
			filterable={['displayName', 'type', 'deploymentStatus', 'userName']}
			headers={[
				{ title: 'Name', property: 'displayName' },
				{ title: 'User', property: 'userName' },
				{ title: 'Status', property: 'deploymentStatus' }
			]}
			onClickRow={(d) => {
				onSelect?.({
					server: d,
					instance: instancesMap.get(d.id),
					entry: d.catalogEntryID ? entriesMap[d.catalogEntryID] : undefined
				});
			}}
			sortable={['displayName', 'type', 'deploymentStatus', 'userName', 'registry', 'created']}
			noDataMessage="No catalog servers added."
			sectionedBy="isMyServer"
			sectionPrimaryTitle="My Servers"
			sectionSecondaryTitle="All Servers"
			disablePortal
		>
			{#snippet onRenderColumn(property, d)}
				{#if property === 'displayName'}
					<div class="flex flex-shrink-0 items-center gap-2">
						<div class="icon">
							{#if d.manifest.icon}
								<img src={d.manifest.icon} alt={d.manifest.name} class="size-6" />
							{:else}
								<Server class="size-6" />
							{/if}
						</div>
						<p class="flex flex-col">
							{d.displayName}
							{#if d.compositeParentName}
								<span class="text-on-surface1 text-xs">
									({d.compositeParentName})
								</span>
							{/if}
						</p>
					</div>
				{:else if property === 'created'}
					{formatTimeAgo(d.created).relativeTime}
				{:else if property === 'deploymentStatus'}
					<div class="flex items-center gap-2">
						{d.deploymentStatus || '--'}
						{#if d.needsUpdate && !d.compositeName}
							<div use:tooltip={'Upgrade available'}>
								<CircleFadingArrowUp class="text-primary size-4" />
							</div>
						{/if}
					</div>
				{:else}
					{d[property as keyof typeof d]}
				{/if}
			{/snippet}

			{#snippet actions(d)}
				<button class="icon-button hover:dark:bg-background/50">
					<StepForward class="size-4" />
				</button>
			{/snippet}
		</Table>
	{/if}
</div>
