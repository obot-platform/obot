<script lang="ts">
	import {
		AdminService,
		ChatService,
		MCPCompositeDeletionDependencyError,
		type MCPCatalogEntry,
		type MCPCatalogServer,
		type MCPServerInstance
	} from '$lib/services';
	import {
		getServerType,
		hasEditableConfiguration,
		requiresUserUpdate
	} from '$lib/services/chat/mcp';
	import { twMerge } from 'tailwind-merge';
	import DotDotDot from '../DotDotDot.svelte';
	import {
		LoaderCircle,
		MessageCircle,
		PencilLine,
		Server,
		ServerCog,
		StepForward,
		Trash2,
		Unplug
	} from 'lucide-svelte';
	import { mcpServersAndEntries, profile } from '$lib/stores';
	import ConnectToServer from './ConnectToServer.svelte';
	import EditExistingDeployment from './EditExistingDeployment.svelte';
	import Confirm from '../Confirm.svelte';
	import { DEFAULT_MCP_CATALOG_ID } from '$lib/constants';
	import McpMultiDeleteBlockedDialog from './McpMultiDeleteBlockedDialog.svelte';
	import ResponsiveDialog from '../ResponsiveDialog.svelte';
	import Table from '../table/Table.svelte';
	import { formatTimeAgo } from '$lib/time';

	interface Props {
		server?: MCPCatalogServer;
		entry?: MCPCatalogEntry;
		onDelete?: () => void;
		onDeleteConflict?: (error: MCPCompositeDeletionDependencyError) => void;
		loading?: boolean;
		skipConnectDialog?: boolean;
		onConnect?: ({ server, entry }: { server?: MCPCatalogServer; entry?: MCPCatalogEntry }) => void;
	}

	let { server, entry, onDelete, loading, skipConnectDialog, onConnect }: Props = $props();
	let connectToServerDialog = $state<ReturnType<typeof ConnectToServer>>();
	let editExistingDialog = $state<ReturnType<typeof EditExistingDeployment>>();

	let deletingInstance = $state<MCPServerInstance>();
	let deletingServer = $state<MCPCatalogServer>();
	let deleteConflictError = $state<MCPCompositeDeletionDependencyError | undefined>();

	let selectedConfiguredServers = $state<MCPCatalogServer[]>([]);
	let selectedEntry = $state<MCPCatalogEntry>();
	let selectServerDialog = $state<ReturnType<typeof ResponsiveDialog>>();

	let instance = $derived(
		server && !server.catalogEntryID
			? mcpServersAndEntries.current.userInstances.find(
					(instance) => instance.mcpServerID === server.id
				)
			: undefined
	);
	let configuredServers = $derived(
		entry
			? mcpServersAndEntries.current.userConfiguredServers.filter(
					(server) => server.catalogEntryID === entry.id
				)
			: []
	);
	let serverType = $derived(server && getServerType(server));
	let isSingleOrRemote = $derived(serverType === 'single' || serverType === 'remote');
	let requiresUpdate = $derived(server && requiresUserUpdate(server));
	let belongsToUser = $derived(server && server.userID === profile.current.id);
	let canConfigure = $derived(
		entry && (entry.manifest.runtime === 'composite' || hasEditableConfiguration(entry))
	);
	let isConfigured = $derived((server && entry) || (server && instance));
	function refresh() {
		if (entry) {
			mcpServersAndEntries.refreshUserConfiguredServers();
		} else if (!server?.catalogEntryID) {
			mcpServersAndEntries.refreshUserInstances();
		}
	}

	function handleDeleteSuccess() {
		if (onDelete) {
			onDelete();
		} else {
			history.back();
		}
	}

	export function connect() {
		connectToServerDialog?.open({
			entry,
			server,
			instance
		});
	}
</script>

{#if (entry && !server) || (server && (!server.catalogEntryID || (server.catalogEntryID && server.userID === profile.current.id)))}
	<button
		class="button-primary flex w-full items-center gap-1 text-sm md:w-fit"
		onclick={() => {
			if (entry && !server && configuredServers.length > 0) {
				selectedConfiguredServers = configuredServers;
				selectedEntry = entry;
				selectServerDialog?.open();
			} else {
				connectToServerDialog?.open({
					entry,
					server,
					instance
				});
			}
		}}
		disabled={loading}
	>
		{#if loading}
			<LoaderCircle class="size-4 animate-spin" />
		{:else}
			Connect To Server
		{/if}
	</button>
{/if}

{#if !loading && server && isConfigured}
	<DotDotDot
		class="icon-button hover:bg-surface1 dark:hover:bg-surface2 hover:text-primary flex-shrink-0"
	>
		{#snippet children({ toggle })}
			<div class="default-dialog flex min-w-48 flex-col p-2">
				{#if isSingleOrRemote}
					{#if server.userID === profile.current.id}
						<button
							class="menu-button"
							onclick={async (e) => {
								e.stopPropagation();
								if (!server) return;
								connectToServerDialog?.handleSetupChat(server, instance);
								toggle(false);
							}}
						>
							<MessageCircle class="size-4" /> Chat
						</button>
					{/if}
					{#if belongsToUser}
						<button
							class="menu-button"
							onclick={() => {
								editExistingDialog?.rename({
									server,
									entry
								});
							}}
						>
							<PencilLine class="size-4" /> Rename
						</button>
					{/if}
					{#if belongsToUser && canConfigure}
						<button
							class={twMerge(
								'menu-button',
								requiresUpdate && 'bg-yellow-500/10 text-yellow-500 hover:bg-yellow-500/30'
							)}
							onclick={() => {
								editExistingDialog?.edit({
									server,
									entry
								});
							}}
						>
							<ServerCog class="size-4" /> Edit Configuration
						</button>
					{/if}
				{/if}
				{#if server && instance}
					<button
						class="menu-button"
						onclick={async () => {
							if (instance) {
								deletingInstance = instance;
							}
						}}
					>
						<Unplug class="size-4" /> Disconnect
					</button>
				{/if}
				{#if isConfigured && (belongsToUser || profile.current.hasAdminAccess?.())}
					<button
						class="menu-button-destructive"
						onclick={() => {
							deletingServer = server;
						}}
					>
						<Trash2 class="size-4" /> Delete Server
					</button>
				{/if}
			</div>
		{/snippet}
	</DotDotDot>
{/if}

<ConnectToServer
	bind:this={connectToServerDialog}
	userConfiguredServers={mcpServersAndEntries.current.userConfiguredServers}
	onConnect={(data) => {
		console.log({ data });
		onConnect?.(data);
		refresh();
	}}
	{skipConnectDialog}
/>

<EditExistingDeployment bind:this={editExistingDialog} onUpdateConfigure={refresh} />

<Confirm
	msg="Are you sure you want to disconnect from this server?"
	show={Boolean(deletingInstance)}
	onsuccess={async () => {
		if (deletingInstance) {
			await ChatService.deleteMcpServerInstance(deletingInstance.id);
			await refresh();
			handleDeleteSuccess();
		}
	}}
	oncancel={() => (deletingInstance = undefined)}
/>

<Confirm
	msg="Are you sure you want to delete this server?"
	show={Boolean(deletingServer)}
	onsuccess={async () => {
		if (!deletingServer) return;

		if (deletingServer.catalogEntryID) {
			await ChatService.deleteSingleOrRemoteMcpServer(deletingServer.id);
		} else {
			try {
				if (deletingServer.powerUserWorkspaceID) {
					await ChatService.deleteWorkspaceMCPCatalogServer(
						deletingServer.powerUserWorkspaceID,
						deletingServer.id
					);
				} else if (profile.current.hasAdminAccess?.()) {
					await AdminService.deleteMCPCatalogServer(DEFAULT_MCP_CATALOG_ID, deletingServer.id);
				}
				// Remove server from list
				mcpServersAndEntries.current.servers = mcpServersAndEntries.current.servers.filter(
					(s) => s.id !== deletingServer?.id
				);
			} catch (error) {
				if (error instanceof MCPCompositeDeletionDependencyError) {
					deleteConflictError = error;
					return;
				}

				throw error;
			}
		}
		await refresh();
		handleDeleteSuccess();
	}}
	oncancel={() => (deletingServer = undefined)}
/>

<McpMultiDeleteBlockedDialog
	show={!!deleteConflictError}
	error={deleteConflictError}
	onClose={() => {
		deleteConflictError = undefined;
	}}
/>

<ResponsiveDialog class="bg-surface1 dark:bg-background" bind:this={selectServerDialog} title="Select Your Server">
	<Table
		data={selectedConfiguredServers || []}
		fields={['name', 'created']}
		onClickRow={(d) => {
			connectToServerDialog?.open({
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
			connectToServerDialog?.open({
				entry: selectedEntry
			});
		}}>Connect New Server</button
	>
</ResponsiveDialog>
