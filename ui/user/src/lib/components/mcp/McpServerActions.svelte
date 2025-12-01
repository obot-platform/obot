<script lang="ts">
	import {
		ChatService,
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
	import { LoaderCircle, MessageCircle, PencilLine, ServerCog, Trash2 } from 'lucide-svelte';
	import { mcpServersAndEntries, profile } from '$lib/stores';
	import ConnectToServer from './ConnectToServer.svelte';
	import EditExistingDeployment from './EditExistingDeployment.svelte';
	import Confirm from '../Confirm.svelte';

	interface Props {
		server?: MCPCatalogServer;
		entry?: MCPCatalogEntry;
		instance?: MCPServerInstance;
		onDelete?: () => void;
		loading?: boolean;
	}

	let { server, entry, instance, onDelete, loading }: Props = $props();
	let connectToServerDialog = $state<ReturnType<typeof ConnectToServer>>();
	let editExistingDialog = $state<ReturnType<typeof EditExistingDeployment>>();

	let deletingInstance = $state<MCPServerInstance>();
	let deletingServer = $state<MCPCatalogServer>();

	let serverType = $derived(server && getServerType(server));
	let isSingleOrRemote = $derived(serverType === 'single' || serverType === 'remote');
	let requiresUpdate = $derived(
		server &&
			requiresUserUpdate({
				connectURL: server?.connectURL ?? '',
				server: { ...server, categories: [] }
			})
	);
	let canConfigure = $derived(
		entry && (entry.manifest.runtime === 'composite' || hasEditableConfiguration(entry))
	);
	let isConfigured = $derived((server && entry) || (server && instance));

	async function refresh() {
		await mcpServersAndEntries.refreshUserConfiguredServers();
	}

	function handleDeleteSuccess() {
		if (onDelete) {
			onDelete();
		} else {
			history.back();
		}
	}
</script>

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

{#if !loading && isConfigured && server}
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
					<button
						class="menu-button"
						onclick={() => {
							editExistingDialog?.rename({
								server,
								entry,
								instance
							});
						}}
					>
						<PencilLine class="size-4" /> Rename
					</button>
					{#if canConfigure}
						<button
							class={twMerge(
								'menu-button',
								requiresUpdate && 'bg-yellow-500/10 text-yellow-500 hover:bg-yellow-500/30'
							)}
							onclick={() => {
								editExistingDialog?.edit({
									server,
									entry,
									instance
								});
							}}
						>
							<ServerCog class="size-4" /> Edit Configuration
						</button>
					{/if}
				{/if}
				<button
					class="menu-button-destructive"
					onclick={async () => {
						if (instance) {
							deletingInstance = instance;
						} else {
							deletingServer = server;
						}
					}}
				>
					<Trash2 class="size-4" /> Delete Server
				</button>
			</div>
		{/snippet}
	</DotDotDot>
{/if}

<ConnectToServer
	bind:this={connectToServerDialog}
	userConfiguredServers={mcpServersAndEntries.current.userConfiguredServers}
	onConnect={refresh}
/>

<EditExistingDeployment bind:this={editExistingDialog} onUpdateConfigure={refresh} />

<Confirm
	msg="Are you sure you want to delete this server?"
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
		if (deletingServer) {
			await ChatService.deleteSingleOrRemoteMcpServer(deletingServer.id);
			await refresh();
			handleDeleteSuccess();
		}
	}}
	oncancel={() => (deletingServer = undefined)}
/>
