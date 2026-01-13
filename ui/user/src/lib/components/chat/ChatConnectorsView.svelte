<script lang="ts">
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import Table from '$lib/components/table/Table.svelte';
	import {
		type MCPCatalogEntry,
		type MCPCatalogServer,
		type MCPServerInstance,
		type OrgUser
	} from '$lib/services';
	import {
		convertEntriesAndServersToTableData,
		getServerTypeLabelByType,
		requiresUserUpdate
	} from '$lib/services/chat/mcp';
	import { mcpServersAndEntries } from '$lib/stores';
	import { formatTimeAgo } from '$lib/time';
	import ResponsiveDialog from '../ResponsiveDialog.svelte';
	import {
		CircleFadingArrowUp,
		LoaderCircle,
		Server,
		StepForward,
		TriangleAlert
	} from 'lucide-svelte';

	interface Props {
		usersMap?: Map<string, OrgUser>;
		query?: string;
		classes?: {
			tableHeader?: string;
		};
		onSelect: ({
			entry,
			instance,
			server
		}: {
			entry?: MCPCatalogEntry;
			instance?: MCPServerInstance;
			server?: MCPCatalogServer;
		}) => void;
		onConnect: ({
			server,
			instance,
			entry
		}: {
			server?: MCPCatalogServer;
			instance?: MCPServerInstance;
			entry?: MCPCatalogEntry;
		}) => void;
	}

	let { query, onSelect, onConnect }: Props = $props();

	let selectedConfiguredServers = $state<MCPCatalogServer[]>([]);
	let selectedEntry = $state<MCPCatalogEntry>();
	let selectServerDialog = $state<ReturnType<typeof ResponsiveDialog>>();

	let entriesMap = $derived(
		new Map(mcpServersAndEntries.current.entries.map((entry) => [entry.id, entry]))
	);

	let tableData = $derived(
		convertEntriesAndServersToTableData(
			mcpServersAndEntries.current.entries,
			mcpServersAndEntries.current.servers,
			undefined,
			mcpServersAndEntries.current.userConfiguredServers,
			mcpServersAndEntries.current.userInstances
		)
	);

	let filteredTableData = $derived.by(() => {
		const sorted = tableData
			.filter((d) => d.data.canConnect !== false)
			.sort((a, b) => {
				return a.name.localeCompare(b.name);
			});
		return query
			? sorted.filter(
					(d) =>
						d.name.toLowerCase().includes(query.toLowerCase()) ||
						d.registry.toLowerCase().includes(query.toLowerCase())
				)
			: sorted;
	});
</script>

<div class="flex flex-col gap-2">
	{#if mcpServersAndEntries.current.loading}
		<div class="my-2 flex items-center justify-center">
			<LoaderCircle class="size-6 animate-spin" />
		</div>
	{:else}
		<Table
			data={filteredTableData}
			classes={{
				root: 'rounded-none rounded-b-md shadow-none'
			}}
			fields={['name', 'connected', 'created']}
			filterable={['name', 'type', 'registry']}
			headers={[{ title: 'Status', property: 'connected' }]}
			onClickRow={(d) => {
				onSelect?.({
					entry:
						'isCatalogEntry' in d.data
							? d.data
							: d.data.catalogEntryID
								? entriesMap.get(d.data.catalogEntryID)
								: undefined,
					server: 'isCatalogEntry' in d.data ? undefined : d.data
				});
			}}
			sortable={['name', 'type', 'users', 'created', 'registry', 'connected']}
			noDataMessage="No catalog servers added."
			setRowClasses={(d) => ('needsUpdate' in d && d.needsUpdate ? 'bg-primary/10' : '')}
			disablePortal
		>
			{#snippet onRenderColumn(property, d)}
				{@const server =
					'isCatalogEntry' in d.data
						? mcpServersAndEntries.current.userConfiguredServers.find(
								(server) => server.catalogEntryID === d.data.id && !server.alias
							)
						: d.data}
				{#if property === 'name'}
					<div class="flex flex-shrink-0 items-center gap-2">
						<div class="icon">
							{#if d.icon}
								<img src={d.icon} alt={d.name} class="size-6" />
							{:else}
								<Server class="size-6" />
							{/if}
						</div>
						<p class="flex items-center gap-2">
							{d.name}
							{#if server && requiresUserUpdate(server)}
								<span
									class="text-yellow-500"
									use:tooltip={{
										text: 'Server requires an update.',
										disablePortal: true
									}}
								>
									<TriangleAlert class="size-4" />
								</span>
							{/if}
						</p>
					</div>
				{:else if property === 'connected'}
					{#if d.connected}
						<div class="pill-primary bg-primary">Connected</div>
					{/if}
				{:else if property === 'type'}
					{getServerTypeLabelByType(d.type)}
				{:else if property === 'created'}
					{formatTimeAgo(d.created).relativeTime}
				{:else}
					{d[property as keyof typeof d]}
				{/if}
			{/snippet}
			{#snippet actions(d)}
				<button
					class="icon-button hover:dark:bg-background/50"
					onclick={(e) => {
						e.stopPropagation();

						if ('isCatalogEntry' in d.data && d.connected) {
							selectedConfiguredServers = mcpServersAndEntries.current.userConfiguredServers.filter(
								(server) => server.catalogEntryID === d.data.id
							);
							selectedEntry = d.data;
							selectServerDialog?.open();
						} else {
							const entry =
								'isCatalogEntry' in d.data
									? d.data
									: d.data.catalogEntryID
										? entriesMap.get(d.data.catalogEntryID)
										: undefined;
							const server = 'isCatalogEntry' in d.data ? undefined : d.data;
							onConnect?.({
								entry,
								server
							});
						}
					}}
				>
					<StepForward class="size-4" />
				</button>
			{/snippet}
		</Table>
	{/if}
</div>

<ResponsiveDialog bind:this={selectServerDialog} title="Select Your Server">
	<Table
		data={selectedConfiguredServers || []}
		fields={['name', 'created']}
		onClickRow={(d) => {
			onConnect?.({
				entry: selectedEntry,
				server: d
			});
			selectServerDialog?.close();
		}}
	>
		{#snippet onRenderColumn(property, d)}
			{#if property === 'name'}
				<div class="flex flex-shrink-0 items-center gap-2">
					<div class="icon">
						{#if d.manifest.icon}
							<img src={d.manifest.icon} alt={d.manifest.name} class="size-6" />
						{:else}
							<Server class="size-6" />
						{/if}
					</div>
					<p class="flex items-center gap-2">
						{d.alias || d.manifest.name}
						{#if 'needsUpdate' in d && d.needsUpdate}
							<span
								use:tooltip={{
									classes: ['border-primary', 'bg-primary/10', 'dark:bg-primary/50'],
									text: 'An update requires your attention'
								}}
							>
								<CircleFadingArrowUp class="text-primary size-4" />
							</span>
						{/if}
					</p>
				</div>
			{:else if property === 'created'}
				{formatTimeAgo(d.created).relativeTime}
			{/if}
		{/snippet}
		{#snippet actions()}
			<button class="icon-button hover:dark:bg-background/50">
				<StepForward class="size-4" />
			</button>
		{/snippet}
	</Table>
	<p class="my-4 self-center text-center text-sm font-semibold">OR</p>
	<button
		class="button-primary"
		onclick={() => {
			selectServerDialog?.close();
			onConnect?.({
				entry: selectedEntry
			});
		}}>Connect New Server</button
	>
</ResponsiveDialog>
