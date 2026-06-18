<script lang="ts">
	import CopyField from '$lib/components/CopyField.svelte';
	import ResponsiveDialog from '$lib/components/ResponsiveDialog.svelte';
	import IconButton from '$lib/components/primitives/IconButton.svelte';
	import Table from '$lib/components/table/Table.svelte';
	import type { MCPCatalogEntry, MCPCatalogServer } from '$lib/services';
	import {
		getMCPDisplayName,
		hasEditableConfiguration,
		isMultiUserCatalogEntry
	} from '$lib/services/user/mcp';
	import { mcpServersAndEntries } from '$lib/stores';
	import { formatTimeAgo } from '$lib/time';
	import { Info, Plus, Server, StepForward } from 'lucide-svelte';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		onLaunchCatalogEntry?: (catalogEntry: MCPCatalogEntry) => void;
	}

	let { onLaunchCatalogEntry }: Props = $props();
	let connectUrlDialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let displayConnectUrl = $state<{ url: string; catalogEntry?: MCPCatalogEntry }>();

	let selectServerForCatalogEntry = $state<{
		entry: MCPCatalogEntry;
		servers: MCPCatalogServer[];
	}>();
	let selectServerDialog = $state<ReturnType<typeof ResponsiveDialog>>();

	function getMultiUserCatalogEntryServers(entry: MCPCatalogEntry) {
		return mcpServersAndEntries.current.servers.filter((s) => s.catalogEntryID === entry.id);
	}

	function handleShowSelectServerDialog(entry: MCPCatalogEntry, servers: MCPCatalogServer[]) {
		selectServerForCatalogEntry = {
			entry,
			servers
		};
		selectServerDialog?.open();
	}

	export function open(catalogEntry?: MCPCatalogEntry, urlToDisplay?: string) {
		if (urlToDisplay) {
			displayConnectUrl = {
				url: urlToDisplay,
				catalogEntry
			};
			connectUrlDialog?.open();
			return;
		}

		if (isMultiUserCatalogEntry(catalogEntry)) {
			// multi user catalog requires configuration of at least one server to obtain connect URL
			const matchingServers = getMultiUserCatalogEntryServers(catalogEntry!);
			if (matchingServers.length > 1) {
				handleShowSelectServerDialog(catalogEntry!, matchingServers);
			} else if (matchingServers[0]?.connectURL) {
				displayConnectUrl = {
					url: matchingServers[0].connectURL,
					catalogEntry
				};
				connectUrlDialog?.open();
			} else {
				onLaunchCatalogEntry?.(catalogEntry!);
			}
			return;
		}

		// single user catalog entry contains baseline connect URL
		displayConnectUrl = {
			url: catalogEntry?.connectURL ?? '',
			catalogEntry
		};
		connectUrlDialog?.open();
	}

	function isConfigurableSingleUserCatalogEntry(entry?: MCPCatalogEntry) {
		return entry && !isMultiUserCatalogEntry(entry) && hasEditableConfiguration(entry);
	}
</script>

<ResponsiveDialog
	bind:this={connectUrlDialog}
	animate="slide"
	title="Connection URL"
	class="max-w-[95vw] md:max-w-2xl"
	classes={{ content: 'p-0', header: 'p-4 pb-0' }}
	disableMobileStyles
>
	<div class={twMerge('px-4', !isMultiUserCatalogEntry(displayConnectUrl?.catalogEntry) && 'pb-4')}>
		<CopyField id="connect-url-dialog-connection-url" value={displayConnectUrl?.url ?? ''} />
	</div>
	{#if isMultiUserCatalogEntry(displayConnectUrl?.catalogEntry)}
		<div class="mt-4 p-4 border-t border-base-300 dark:border-base-400">
			<p
				class="text-muted-content flex items-center justify-end gap-2 text-sm font-light md:px-0 px-4"
			>
				Need to set up a different instance?
				<button
					class="btn btn-sm btn-primary text-xs"
					onclick={() => {
						if (displayConnectUrl?.catalogEntry) {
							connectUrlDialog?.close();
							onLaunchCatalogEntry?.(displayConnectUrl.catalogEntry);
						}
					}}
				>
					<Plus class="size-3" />
					Launch New Server
				</button>
			</p>
		</div>
	{:else if isConfigurableSingleUserCatalogEntry(displayConnectUrl?.catalogEntry)}
		<div class="notification-info m-4 mt-0">
			<p class="flex items-center gap-2 text-xs">
				<Info class="size-4" />
				This server requires end-user configuration. Users will need to configure their instance of it
				here before they can connect to it.
			</p>
		</div>
	{/if}
</ResponsiveDialog>

<ResponsiveDialog
	class="bg-base-200 dark:bg-base-100"
	bind:this={selectServerDialog}
	title="Select Your Server"
>
	<Table
		data={selectServerForCatalogEntry?.servers || []}
		fields={['name', 'created']}
		onClickRow={async (d) => {
			selectServerDialog?.close();
			if (!d.connectURL) return;
			displayConnectUrl = {
				url: d.connectURL,
				catalogEntry: selectServerForCatalogEntry?.entry
			};
			connectUrlDialog?.open();
		}}
		disablePortal
	>
		{#snippet onRenderColumn(property, d)}
			{#if property === 'name'}
				<div class="flex shrink-0 items-center gap-2">
					<div class="icon">
						{#if d.manifest.icon}
							<img src={d.manifest.icon} alt={d.manifest.name} class="size-6" />
						{:else}
							<Server class="size-6" />
						{/if}
					</div>
					<p class="flex items-center gap-2">
						{getMCPDisplayName(d)}
					</p>
				</div>
			{:else if property === 'created'}
				{formatTimeAgo(d.created).relativeTime}
			{/if}
		{/snippet}
		{#snippet actions()}
			<IconButton class="hover:dark:bg-base-100/50">
				<StepForward class="size-4" />
			</IconButton>
		{/snippet}
	</Table>
</ResponsiveDialog>
