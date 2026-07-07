<script lang="ts">
	import CopyField from '$lib/components/CopyField.svelte';
	import ResponsiveDialog from '$lib/components/ResponsiveDialog.svelte';
	import McpDeprecatedNotice from '$lib/components/mcp/McpDeprecatedNotice.svelte';
	import McpSelectServerDeployment from '$lib/components/mcp/McpSelectServerDeployment.svelte';
	import type { MCPCatalogEntry, MCPCatalogServer } from '$lib/services';
	import {
		hasEditableConfiguration,
		isDeprecatedMCPServer,
		isMultiUserCatalogEntry
	} from '$lib/services/user/mcp';
	import { mcpServersAndEntries } from '$lib/stores';
	import { Info, Plus } from '@lucide/svelte';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		onLaunchCatalogEntry?: (catalogEntry: MCPCatalogEntry) => void;
	}

	let { onLaunchCatalogEntry }: Props = $props();
	let connectUrlDialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let displayConnectUrl = $state<{
		url: string;
		catalogEntry?: MCPCatalogEntry;
		server?: MCPCatalogServer;
	}>();
	let displayConnectUrlDeprecated = $derived(
		isDeprecatedMCPServer(displayConnectUrl?.catalogEntry) ||
			isDeprecatedMCPServer(displayConnectUrl?.server)
	);
	let catalogEntry = $state<MCPCatalogEntry>();
	let selectServerDialog = $state<ReturnType<typeof McpSelectServerDeployment>>();

	function getMultiUserCatalogEntryServers(entry: MCPCatalogEntry) {
		return mcpServersAndEntries.current.servers.filter((s) => s.catalogEntryID === entry.id);
	}

	function getUserConfiguredCatalogEntryServers(entry: MCPCatalogEntry) {
		return mcpServersAndEntries.current.userConfiguredServers.filter(
			(s) => s.catalogEntryID === entry.id
		);
	}

	export function open(
		initEntry?: MCPCatalogEntry,
		urlToDisplay?: string,
		server?: MCPCatalogServer
	) {
		catalogEntry = initEntry;

		if (urlToDisplay) {
			displayConnectUrl = {
				url: urlToDisplay,
				catalogEntry,
				server
			};
			connectUrlDialog?.open();
			return;
		}

		if (catalogEntry) {
			const matchingServers = isMultiUserCatalogEntry(catalogEntry)
				? getMultiUserCatalogEntryServers(catalogEntry)
				: getUserConfiguredCatalogEntryServers(catalogEntry);
			if (matchingServers.length > 1) {
				selectServerDialog?.open(matchingServers);
			} else if (matchingServers[0]?.connectURL || catalogEntry?.connectURL) {
				displayConnectUrl = {
					url: matchingServers[0]?.connectURL || catalogEntry?.connectURL || '',
					catalogEntry,
					server: matchingServers[0]
				};

				connectUrlDialog?.open();
			} else {
				onLaunchCatalogEntry?.(catalogEntry);
			}
		}
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
	{#if displayConnectUrl?.url}
		<div
			class={twMerge(
				'flex flex-col gap-3 px-4',
				!isMultiUserCatalogEntry(displayConnectUrl?.catalogEntry) && 'pb-4'
			)}
		>
			<McpDeprecatedNotice deprecated={displayConnectUrlDeprecated} variant="notification" />
			<CopyField id="connect-url-dialog-connection-url" value={displayConnectUrl?.url ?? ''} />
		</div>
	{:else}
		<p class="px-4 text-muted-content text-sm text-center w-full">No connection URL available.</p>
	{/if}
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
				{#if getUserConfiguredCatalogEntryServers(displayConnectUrl!.catalogEntry!).length > 0}
					This connection URL uses your configured server instance.
				{:else}
					This server requires user configuration on connection with an MCP client.
				{/if}
			</p>
		</div>
	{/if}
</ResponsiveDialog>

<McpSelectServerDeployment
	bind:this={selectServerDialog}
	contextEntry={catalogEntry}
	onSelectServer={(d) => {
		selectServerDialog?.close();
		displayConnectUrl = {
			url: d.connectURL || catalogEntry?.connectURL || '',
			catalogEntry,
			server: d
		};
		connectUrlDialog?.open();
	}}
/>
