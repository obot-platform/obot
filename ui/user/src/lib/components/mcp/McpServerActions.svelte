<script lang="ts">
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import { ChatService, type MCPCatalogEntry, type MCPCatalogServer } from '$lib/services';
	import { hasEditableConfiguration, requiresUserUpdate } from '$lib/services/chat/mcp';
	import { twMerge } from 'tailwind-merge';
	import DotDotDot from '../DotDotDot.svelte';
	import {
		LoaderCircle,
		MessageCircle,
		PencilLine,
		ReceiptText,
		SatelliteDish,
		Server,
		ServerCog,
		StepForward,
		Trash2,
		Unplug
	} from 'lucide-svelte';
	import { mcpServersAndEntries, profile } from '$lib/stores';
	import ConnectToServer from './ConnectToServer.svelte';
	import EditExistingDeployment from './EditExistingDeployment.svelte';
	import ResponsiveDialog from '../ResponsiveDialog.svelte';
	import Table from '../table/Table.svelte';
	import { formatTimeAgo } from '$lib/time';
	import { goto } from '$lib/url';
	import { resolve } from '$app/paths';
	import { page } from '$app/state';

	type ServerSelectMode = 'connect' | 'rename' | 'edit' | 'disconnect' | 'chat' | 'server-details';

	interface Props {
		server?: MCPCatalogServer;
		entry?: MCPCatalogEntry;
		loading?: boolean;
		skipConnectDialog?: boolean;
		onConnect?: ({ server, entry }: { server?: MCPCatalogServer; entry?: MCPCatalogEntry }) => void;
		promptInitialLaunch?: boolean;
		connectOnly?: boolean;
		isProjectMcp?: boolean;
	}

	let {
		server,
		entry,
		loading,
		skipConnectDialog,
		onConnect,
		promptInitialLaunch,
		connectOnly,
		isProjectMcp
	}: Props = $props();
	let connectToServerDialog = $state<ReturnType<typeof ConnectToServer>>();
	let editExistingDialog = $state<ReturnType<typeof EditExistingDeployment>>();

	let selectServerDialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let selectServerMode = $state<ServerSelectMode>('connect');

	let launchDialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let launchPromptHandled = $state(false);

	let disconnecting = $state(false);

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
	let requiresUpdate = $derived(server && requiresUserUpdate(server));
	let canConfigure = $derived(
		entry && (entry.manifest.runtime === 'composite' || hasEditableConfiguration(entry))
	);
	let belongsToComposite = $derived(Boolean(server && server.compositeName));
	let showServerDetails = $derived(entry && !server && configuredServers.length > 0);
	let hasActions = $derived((entry && server) || showServerDetails || (server && instance));
	let showDisconnectUser = $derived(
		entry && server && profile.current.isAdmin?.() && server.userID !== profile.current.id
	);
	// Look up canConnect from the store if not set on props (e.g., when entry/server loaded directly via API)
	let canConnect = $derived.by(() => {
		const entryCanConnect =
			entry?.canConnect ??
			mcpServersAndEntries.current.entries.find((e) => e.id === entry?.id)?.canConnect ??
			true;
		const serverCanConnect =
			server?.canConnect ??
			mcpServersAndEntries.current.servers.find((s) => s.id === server?.id)?.canConnect ??
			true;
		return entryCanConnect && serverCanConnect;
	});

	function refresh() {
		if (entry) {
			mcpServersAndEntries.refreshUserConfiguredServers();
		} else if (!server?.catalogEntryID) {
			mcpServersAndEntries.refreshUserInstances();
		}
	}

	export function connect() {
		connectToServerDialog?.open({
			entry,
			server,
			instance
		});
	}

	$effect(() => {
		if (promptInitialLaunch && !launchPromptHandled) {
			launchPromptHandled = true;
			launchDialog?.open();

			// clear out the launch param
			const url = new URL(page.url);
			url.searchParams.delete('launch');
			goto(url, { replaceState: true });
		}
	});

	function handleShowSelectServerDialog(mode: ServerSelectMode = 'connect') {
		selectServerDialog?.open();
		selectServerMode = mode;
	}
</script>

{#if !belongsToComposite}
	{#if (entry && !server) || (server && (!server.catalogEntryID || (server.catalogEntryID && server.userID === profile.current.id)))}
		<button
			class="button-primary flex w-full items-center gap-1 text-sm disabled:cursor-not-allowed disabled:opacity-50 md:w-fit"
			use:tooltip={{
				text: canConnect ? '' : 'See MCP Registries to grant connect access to this server'
			}}
			onclick={() => {
				if (entry && !server && configuredServers.length > 0) {
					if (configuredServers.length === 1) {
						connectToServerDialog?.open({
							entry,
							server: configuredServers[0]
						});
					} else {
						handleShowSelectServerDialog();
					}
				} else {
					connectToServerDialog?.open({
						entry,
						server,
						instance
					});
				}
			}}
			disabled={loading || !canConnect}
		>
			{#if loading}
				<LoaderCircle class="size-4 animate-spin" />
			{:else}
				Connect To Server
			{/if}
		</button>
	{/if}

	{#if !loading && hasActions}
		<DotDotDot
			class="icon-button hover:bg-surface1 dark:hover:bg-surface2 hover:text-primary flex-shrink-0"
			disablePortal={connectOnly}
		>
			{#snippet children({ toggle })}
				<div class="default-dialog flex min-w-48 flex-col">
					{#if connectOnly && entry}
						<button
							class="menu-button"
							onclick={async () => {
								connectToServerDialog?.open({
									entry,
									server: undefined
								});
								toggle(false);
							}}
						>
							<SatelliteDish class="size-4" /> Connect New Server
						</button>
					{:else}
						{@render serverActions(toggle)}
					{/if}
				</div>
			{/snippet}
		</DotDotDot>
	{/if}

	<ConnectToServer
		bind:this={connectToServerDialog}
		userConfiguredServers={mcpServersAndEntries.current.userConfiguredServers}
		onConnect={(data) => {
			onConnect?.(data);
			refresh();
		}}
		{skipConnectDialog}
		hideActions={isProjectMcp}
	/>

	<EditExistingDeployment bind:this={editExistingDialog} onUpdateConfigure={refresh} />

	<ResponsiveDialog
		class="bg-surface1 dark:bg-background"
		bind:this={selectServerDialog}
		title="Select Your Server"
	>
		<Table
			data={configuredServers || []}
			fields={['name', 'created']}
			onClickRow={(d) => {
				selectServerDialog?.close();
				switch (selectServerMode) {
					case 'chat': {
						connectToServerDialog?.handleSetupChat(d);
						break;
					}
					case 'server-details': {
						if (profile.current?.hasAdminAccess?.()) {
							goto(
								resolve(
									entry?.powerUserWorkspaceID
										? `/admin/mcp-servers/w/${d.powerUserWorkspaceID}/c/${d.catalogEntryID}/instance/${d.id}`
										: `/admin/mcp-servers/c/${d.catalogEntryID}/instance/${d.id}`
								),
								{ replaceState: true }
							);
						} else {
							goto(resolve(`/mcp-servers/c/${d.catalogEntryID}/instance/${d.id}`));
						}
						break;
					}
					case 'rename': {
						editExistingDialog?.rename({
							server: d,
							entry
						});
						break;
					}
					case 'edit': {
						editExistingDialog?.edit({
							server: d,
							entry
						});
						break;
					}
					case 'disconnect': {
						ChatService.deleteSingleOrRemoteMcpServer(d.id);
						mcpServersAndEntries.refreshUserConfiguredServers();
						break;
					}
					default:
						connectToServerDialog?.open({
							entry,
							server: d
						});
						break;
				}
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
	</ResponsiveDialog>

	<ResponsiveDialog bind:this={launchDialog} animate="slide" class="md:max-w-sm">
		{#snippet titleContent()}
			{#if entry || server}
				{@const name = entry?.manifest.name ?? server?.manifest.name ?? 'MCP Server'}
				{@const imageUrl = entry?.manifest.icon || server?.manifest.icon}
				<div class="icon">
					{#if imageUrl}
						<img
							src={imageUrl}
							alt={entry?.manifest.name ?? server?.manifest.name ?? 'MCP Server'}
							class="size-6"
						/>
					{:else}
						<Server class="size-6" />
					{/if}
				</div>
				{name}
			{/if}
		{/snippet}
		<div class="flex grow flex-col gap-2 p-4 pt-0 md:p-0">
			<p class="text-center">
				{#if entry && entry.manifest.runtime === 'remote'}
					Your proxy remote server details have been configured.
				{:else if entry}
					Your server details have been configured.
				{:else}
					Your server has been configured.
				{/if}
			</p>
			<p class="mb-2 text-center">Would you like to connect now?</p>
			<div class="flex grow"></div>
			<div class="flex flex-col gap-2">
				<button class="button" onclick={() => launchDialog?.close()}>Skip</button>
				<button
					class="button-primary"
					onclick={() => {
						launchDialog?.close();
						connectToServerDialog?.open({
							entry,
							server
						});
					}}>Connect To Server</button
				>
			</div>
		</div>
	</ResponsiveDialog>
{/if}

{#snippet serverActions(toggle: (value: boolean) => void)}
	{#if server && server.userID === profile.current.id}
		<div class="flex flex-col gap-1 p-2 {!isProjectMcp && 'bg-surface1 rounded-t-xl'}">
			{#if !isProjectMcp}
				<button
					class="menu-button"
					onclick={async () => {
						connectToServerDialog?.handleSetupChat(server, instance);
					}}
				>
					<MessageCircle class="size-4" /> Chat
				</button>
			{/if}
			{#if entry}
				{#if !isProjectMcp}
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
				{#if canConfigure}
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
					disabled={disconnecting}
					onclick={async (e) => {
						e.stopPropagation();
						disconnecting = true;
						await ChatService.deleteMcpServerInstance(instance.id);
						mcpServersAndEntries.refreshUserInstances();
						toggle(false);

						if (profile.current.hasAdminAccess?.()) {
							goto(
								resolve(
									entry?.powerUserWorkspaceID
										? `/admin/mcp-servers/w/${server.powerUserWorkspaceID}/s/${server.id}`
										: `/admin/mcp-servers/s/${server.id}`
								),
								{ replaceState: true }
							);
						} else {
							goto(resolve(`/mcp-servers/c/${server.id}`), { replaceState: true });
						}
						disconnecting = false;
					}}
				>
					{#if disconnecting}
						<LoaderCircle class="size-4 animate-spin" />
					{:else}
						<Unplug class="size-4" />
					{/if} Disconnect
				</button>
			{:else if entry && server && !isProjectMcp}
				<button
					class="menu-button"
					disabled={disconnecting}
					onclick={async (e) => {
						e.stopPropagation();
						disconnecting = true;
						await ChatService.deleteSingleOrRemoteMcpServer(server.id);
						mcpServersAndEntries.refreshUserConfiguredServers();
						toggle(false);

						if (profile.current.hasAdminAccess?.()) {
							goto(
								resolve(
									entry?.powerUserWorkspaceID
										? `/admin/mcp-servers/w/${entry.powerUserWorkspaceID}/c/${entry.id}`
										: `/admin/mcp-servers/c/${entry.id}`
								),
								{ replaceState: true }
							);
						} else {
							goto(resolve(`/mcp-servers/c/${entry.id}`), { replaceState: true });
						}
						disconnecting = false;
					}}
				>
					{#if disconnecting}
						<LoaderCircle class="size-4 animate-spin" />
					{:else}
						<Trash2 class="size-4" />
					{/if} Disconnect
				</button>
			{/if}
		</div>
	{:else if entry && configuredServers.length > 0}
		<div
			class="bg-background dark:bg-surface2 rounded-t-xl p-2 pl-4 text-[11px] font-semibold uppercase"
		>
			My Connection(s)
		</div>
		<div class="bg-surface1 flex flex-col gap-1 p-2">
			<button
				class="menu-button"
				onclick={() => {
					if (configuredServers.length === 1) {
						connectToServerDialog?.handleSetupChat(configuredServers[0]);
					} else {
						handleShowSelectServerDialog('chat');
					}
				}}
			>
				<MessageCircle class="size-4" /> Chat
			</button>
			{#if entry}
				<button
					class="menu-button"
					onclick={() => {
						if (configuredServers.length === 1) {
							editExistingDialog?.rename({
								server: configuredServers[0],
								entry
							});
						} else {
							handleShowSelectServerDialog('rename');
						}
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
							if (configuredServers.length === 1) {
								editExistingDialog?.edit({
									server: configuredServers[0],
									entry
								});
							} else {
								handleShowSelectServerDialog('edit');
							}
						}}
					>
						<ServerCog class="size-4" /> Edit Configuration
					</button>
				{/if}
			{/if}
			<button
				class="menu-button"
				onclick={() => {
					if (configuredServers.length === 1) {
						if (profile.current.hasAdminAccess?.()) {
							goto(
								resolve(
									entry?.powerUserWorkspaceID
										? `/admin/mcp-servers/w/${entry.powerUserWorkspaceID}/c/${entry.id}/instance/${configuredServers[0].id}`
										: `/admin/mcp-servers/c/${entry.id}/instance/${configuredServers[0].id}`
								),
								{ replaceState: true }
							);
						} else {
							goto(resolve(`/mcp-servers/c/${entry.id}/instance/${configuredServers[0].id}`), {
								replaceState: true
							});
						}
					} else {
						handleShowSelectServerDialog('server-details');
					}
				}}
			>
				<ReceiptText class="size-4" /> Server Details
			</button>
			<button
				class="menu-button"
				onclick={async (e) => {
					e.stopPropagation();
					if (configuredServers.length === 1) {
						await ChatService.deleteSingleOrRemoteMcpServer(configuredServers[0].id);
						mcpServersAndEntries.refreshUserConfiguredServers();
					} else {
						handleShowSelectServerDialog('disconnect');
					}
					toggle(false);
				}}
			>
				<Unplug class="size-4" /> Disconnect
			</button>
		</div>
	{/if}
	{#if showDisconnectUser && server}
		<div class="flex flex-col gap-2 p-2">
			<button
				class="menu-button text-red-500"
				onclick={async (e) => {
					e.stopPropagation();
					await ChatService.deleteSingleOrRemoteMcpServer(server.id);
					mcpServersAndEntries.refreshUserConfiguredServers();
					toggle(false);
				}}
			>
				<Trash2 class="size-4" /> Disconnect User
			</button>
		</div>
	{/if}
{/snippet}
