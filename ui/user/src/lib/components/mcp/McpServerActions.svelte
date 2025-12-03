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
		ServerCog,
		Trash2,
		Unplug
	} from 'lucide-svelte';
	import { mcpServersAndEntries, profile } from '$lib/stores';
	import ConnectToServer from './ConnectToServer.svelte';
	import EditExistingDeployment from './EditExistingDeployment.svelte';
	import Confirm from '../Confirm.svelte';
	import { DEFAULT_MCP_CATALOG_ID } from '$lib/constants';
	import McpMultiDeleteBlockedDialog from './McpMultiDeleteBlockedDialog.svelte';

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

	let instance = $derived(
		server && !server.catalogEntryID
			? mcpServersAndEntries.current.userInstances.find(
					(instance) => instance.mcpServerID === server.id
				)
			: undefined
	);
	let serverType = $derived(server && getServerType(server));
	let isSingleOrRemote = $derived(serverType === 'single' || serverType === 'remote');
	let requiresUpdate = $derived(server && requiresUserUpdate(server));
	let belongsToUser = $derived(
		server && (profile.current.hasAdminAccess?.() || server.userID === profile.current.id)
	);
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

{#if server && (!server.catalogEntryID || (server.catalogEntryID && server.userID === profile.current.id))}
	<button
		class="button-primary flex w-full items-center gap-1 text-sm md:w-fit"
		onclick={() => {
			connectToServerDialog?.open({
				entry,
				server,
				instance
			});
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
					{#if belongsToUser}
						<button
							class="menu-button-destructive"
							onclick={() => {
								deletingServer = server;
							}}
						>
							<Trash2 class="size-4" /> Delete Server
						</button>
					{/if}
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
